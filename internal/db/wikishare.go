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
	markdownQuotes   = regexp.MustCompile(`^\s*(>\s?)+`)
	calloutMark      = regexp.MustCompile(`^\[![A-Za-z]+\]\s*`)
	questionMark     = regexp.MustCompile(`(?i)^\[!question\]\s*`)
)

type WikiShare struct {
	s     *Store
	pics  *Pictures
	style *sharecard.Style
}

type wikiShared struct {
	id, path, name, top, sentence, outlineIntro string
	outline                                     []string
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
	if len(page.outline) > 0 {
		desc = strings.TrimSpace(desc + " " + page.outlineIntro + ": " + strings.Join(page.outline, " · "))
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
	shown := page.outline
	if len(shown) > wikiShareSubPages {
		shown = append(shown[:wikiShareSubPages-1:wikiShareSubPages-1], fmt.Sprintf("and %d more", len(page.outline)-wikiShareSubPages+1))
	}
	for _, name := range shown {
		card.Lines = append(card.Lines, sharecard.Line{Icon: "dot", Text: name})
	}
	w.style.Serve(rw, r, card, slices.Concat([]string{page.top, page.name, page.sentence, header}, page.outline)...)
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
	page := wikiShared{id: id, path: m.WikiPath(row), name: row["name"], outlineIntro: "In this section", outline: []string{}}
	for at := row["parent"]; at != ""; {
		above, _ := docs.Get(at)
		page.top, at = above["name"], above["parent"]
	}
	for _, child := range m.wikiChildren(id) {
		page.outline = append(page.outline, child["name"])
	}
	markdown, err := w.markdown(ctx, row)
	if err != nil {
		return wikiShared{}, false, err
	}
	page.sentence = firstSentence(markdown)
	if len(page.outline) == 0 {
		page.outlineIntro, page.outline = "On this page", pageOutline(markdown)
	}
	if len(page.outline) > 0 && strings.HasPrefix(page.outline[0], page.sentence) {
		page.sentence = ""
	}
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

type markdownLine struct {
	text              string
	heading, question bool
}

func markdownLines(markdown string) []markdownLine {
	out := []markdownLine{}
	fenced := false
	for _, line := range strings.Split(withoutFrontMatter(markdown), "\n") {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		line = strings.TrimSpace(markdownQuotes.ReplaceAllString(line, ""))
		read := markdownLine{heading: markdownHeadLine.MatchString(line), question: questionMark.MatchString(line)}
		text := markdownImage.ReplaceAllString(calloutMark.ReplaceAllString(line, ""), "")
		text = markdownLink.ReplaceAllString(markdownLead.ReplaceAllString(text, ""), "$1")
		if read.text = strings.TrimSpace(markdownMarks.Replace(text)); read.text != "" {
			out = append(out, read)
		}
	}
	return out
}

func firstSentence(markdown string) string {
	for _, line := range markdownLines(markdown) {
		if line.heading {
			continue
		}
		if end := sentenceEnd.FindStringIndex(line.text); end != nil {
			return strings.TrimSpace(line.text[:end[0]+1])
		}
		return line.text
	}
	return ""
}

func pageOutline(markdown string) []string {
	out := []string{}
	for _, line := range markdownLines(markdown) {
		if line.heading || line.question {
			out = append(out, line.text)
		}
	}
	return out
}
