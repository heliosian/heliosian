package db

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

const (
	wikiShareSubPages  = 4
	wikiBackgrounds    = "web/wiki/backgrounds/"
	wikiPicturePrefix  = "/api/wiki/picture/"
	wikiHeaderImageKey = "header_image:"
)

var wikiBackgroundNames = []string{"backpacking", "building", "camping", "library", "math", "music", "nature", "science", "writing"}

var (
	markdownImage    = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	markdownLink     = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	markdownLead     = regexp.MustCompile(`^\s*(#{1,6}\s|>\s?|[-*]\s+|\d+[.)]\s+)`)
	markdownMarks    = strings.NewReplacer("**", "", "*", "", "`", "")
	sentenceEnd      = regexp.MustCompile(`[.!?](\s|$)`)
	markdownHeadLine = regexp.MustCompile(`^\s*#{1,6}\s`)
)

type WikiShare struct {
	s     *Store
	pics  *Pictures
	style *sharecard.Style
}

type wikiShared struct {
	id, path, name, top, sentence string
	subPages                      []string
}

func NewWikiShare(s *Store, pics *Pictures, name func() string) *WikiShare {
	return &WikiShare{s: s, pics: pics, style: &sharecard.Style{Palette: sharecard.Standard, Name: name, Tagline: func() string { return "" }, Lockup: "web/public/wiki/brand/logo-lockup-horizontal.png"}}
}

func (w *WikiShare) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /open/share/{id}", w.serveCard)
}

func (w *WikiShare) PreviewHead(r *http.Request) string {
	rest, ok := strings.CutPrefix(r.URL.Path, "/p/")
	if !ok {
		return ""
	}
	key := strings.TrimSuffix(rest, "/edit")
	if w.s.Model().wikiPageAt(rest) != "" {
		key = rest
	}
	page, found, err := w.page(r.Context(), key)
	if err != nil {
		slog.ErrorContext(r.Context(), "read a wiki page for its preview", "page", key, "error", err)
		return ""
	}
	if !found {
		return ""
	}
	desc := page.sentence
	if len(page.subPages) > 0 {
		desc = strings.TrimSpace(desc + " In this section: " + strings.Join(page.subPages, " · "))
	}
	origin := "https://" + r.Host
	return w.style.PreviewTags(page.name, desc, origin+(&url.URL{Path: page.path}).EscapedPath(), origin+"/open/share/"+page.id+".png")
}

func (w *WikiShare) serveCard(rw http.ResponseWriter, r *http.Request) {
	id, ok := strings.CutSuffix(r.PathValue("id"), ".png")
	if !ok {
		http.NotFound(rw, r)
		return
	}
	page, found, err := w.page(r.Context(), id)
	if err != nil {
		serve.Error(rw, r, err)
		return
	}
	if !found {
		http.NotFound(rw, r)
		return
	}
	header, picture, err := w.header(r.Context(), page.id)
	if err != nil {
		serve.Error(rw, r, err)
		return
	}
	card := sharecard.Card{Section: page.top, Title: page.name, Subtitle: page.sentence, Picture: picture}
	shown := page.subPages
	if len(shown) > wikiShareSubPages {
		shown = append(shown[:wikiShareSubPages-1:wikiShareSubPages-1], fmt.Sprintf("and %d more", len(page.subPages)-wikiShareSubPages+1))
	}
	for _, name := range shown {
		card.Lines = append(card.Lines, sharecard.Line{Icon: "dot", Text: name})
	}
	w.style.Serve(rw, r, card, slices.Concat([]string{page.top, page.name, page.sentence, header}, page.subPages)...)
}

func (w *WikiShare) header(ctx context.Context, id string) (string, []byte, error) {
	m := w.s.Model()
	docs := m.Table("DOCUMENT")
	top := id
	for at := id; at != ""; {
		row, _ := docs.Get(at)
		markdown, err := w.markdown(ctx, row)
		if err != nil {
			return "", nil, err
		}
		if object := headerImage(markdown); object != "" {
			picture, _, err := w.pics.bucket.Get(ctx, object)
			return object, picture, err
		}
		top, at = at, row["parent"]
	}
	file := wikiBackgrounds + wikiBackgroundNames[backgroundHash(top)%uint32(len(wikiBackgroundNames))] + ".jpg"
	picture, err := os.ReadFile(file)
	return file, picture, err
}

func (w *WikiShare) markdown(ctx context.Context, row map[string]string) (string, error) {
	if row["content"] == "" {
		return "", nil
	}
	content, _ := w.s.Model().Table("CONTENT").Get(row["content"])
	raw, _, err := w.pics.bucket.Get(ctx, content["blob"])
	return string(raw), err
}

func backgroundHash(id string) uint32 {
	sum := uint32(0)
	for _, c := range id {
		sum = sum*31 + uint32(c)
	}
	return sum
}

func headerImage(markdown string) string {
	rest, ok := strings.CutPrefix(markdown, "---\n")
	if !ok {
		return ""
	}
	front, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return ""
	}
	for _, line := range strings.Split(front, "\n") {
		value, ok := strings.CutPrefix(line, wikiHeaderImageKey)
		if !ok {
			continue
		}
		name, ok := strings.CutPrefix(strings.TrimSpace(value), wikiPicturePrefix)
		if ok && wikiImage.MatchString("wiki-images/"+name) {
			return "wiki-images/" + name
		}
	}
	return ""
}

func (w *WikiShare) page(ctx context.Context, key string) (wikiShared, bool, error) {
	m := w.s.Model()
	docs := m.Table("DOCUMENT")
	id := m.wikiPageAt(key)
	if id == "" {
		return wikiShared{}, false, nil
	}
	row, _ := docs.Get(id)
	page := wikiShared{id: id, path: m.WikiPath(row), name: row["name"], subPages: []string{}}
	for at := row["parent"]; at != ""; {
		above, _ := docs.Get(at)
		page.top, at = above["name"], above["parent"]
	}
	for _, child := range m.wikiChildren(id) {
		page.subPages = append(page.subPages, child["name"])
	}
	markdown, err := w.markdown(ctx, row)
	if err != nil {
		return wikiShared{}, false, err
	}
	page.sentence = firstSentence(markdown)
	return page, true, nil
}

func withoutFrontMatter(markdown string) string {
	rest, ok := strings.CutPrefix(markdown, "---\n")
	if !ok {
		return markdown
	}
	if _, body, ok := strings.Cut(rest, "\n---\n"); ok {
		return body
	}
	return markdown
}

func firstSentence(markdown string) string {
	fenced := false
	for _, line := range strings.Split(withoutFrontMatter(markdown), "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced || markdownHeadLine.MatchString(line) {
			continue
		}
		text := markdownImage.ReplaceAllString(line, "")
		text = markdownLink.ReplaceAllString(markdownLead.ReplaceAllString(text, ""), "$1")
		text = strings.TrimSpace(markdownMarks.Replace(text))
		if text == "" {
			continue
		}
		if end := sentenceEnd.FindStringIndex(text); end != nil {
			return strings.TrimSpace(text[:end[0]+1])
		}
		return text
	}
	return ""
}
