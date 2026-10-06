package db

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

var (
	markdownHeading = regexp.MustCompile(`^#{1,6}\s+(.*)$`)
	markdownLink    = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	markdownMarks   = strings.NewReplacer("**", "", "*", "", "`", "")
)

type WikiShare struct {
	s     *Store
	pics  *Pictures
	style *sharecard.Style
}

func NewWikiShare(s *Store, pics *Pictures, name, tagline func() string) *WikiShare {
	return &WikiShare{s: s, pics: pics, style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: tagline, Lockup: "web/public/wiki/brand/logo-lockup-horizontal.png"}}
}

func (w *WikiShare) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /open/share/{id}", w.serveCard)
}

func (w *WikiShare) PreviewHead(r *http.Request) string {
	rest, ok := strings.CutPrefix(r.URL.Path, "/p/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(rest, "/")
	name, headings, found, err := w.page(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "read a wiki page for its preview", "document", id, "error", err)
		return ""
	}
	if !found {
		return ""
	}
	origin := "https://" + r.Host
	return w.style.PreviewTags(name, strings.Join(headings, " · "), origin+"/p/"+id, origin+"/open/share/"+id+".png")
}

func (w *WikiShare) serveCard(rw http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("id"), ".png")
	if !ok {
		http.NotFound(rw, r)
		return
	}
	name, headings, found, err := w.page(r.Context(), id)
	if err != nil {
		serve.Error(rw, r, err)
		return
	}
	if !found {
		http.NotFound(rw, r)
		return
	}
	w.style.Serve(rw, r, sharecard.Card{Title: name, Subtitle: strings.Join(headings, " · ")}, slices.Concat([]string{name}, headings)...)
}

func (w *WikiShare) page(ctx context.Context, id string) (string, []string, bool, error) {
	m := w.s.Model()
	row, ok := m.Table("DOCUMENT").Get(id)
	if !ok || row["kind"] != "wiki" {
		return "", nil, false, nil
	}
	headings := []string{}
	if row["content"] == "" {
		return row["name"], headings, true, nil
	}
	content, _ := m.Table("CONTENT").Get(row["content"])
	raw, _, err := w.pics.bucket.Get(ctx, content["blob"])
	if err != nil {
		return "", nil, false, err
	}
	fenced := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		heading := markdownHeading.FindStringSubmatch(line)
		if fenced || heading == nil {
			continue
		}
		if text := strings.TrimSpace(markdownMarks.Replace(markdownLink.ReplaceAllString(heading[1], "$1"))); text != "" {
			headings = append(headings, text)
		}
	}
	return row["name"], headings, true, nil
}
