package db

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	wikiMime  = "text/markdown"
	wikiLimit = 1 << 20
	wikiWhere = "wiki"
)

const (
	wikiSlugLimit  = 40
	wikiPathPrefix = "/p/"
)

var (
	wikiPicture = regexp.MustCompile(`/api/blob/(` + PhotoPrefix + `[0-9A-Za-z]{11})/image`)
	wikiSlug    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	wikiNameRun = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

func checkWikiSlug(slug, kind string) error {
	if slug == "" {
		return nil
	}
	if kind != "wiki" {
		return fmt.Errorf("only a wiki page has a slug")
	}
	if !wikiSlug.MatchString(slug) || len(slug) > wikiSlugLimit {
		return fmt.Errorf("a slug is lowercase letters, digits and single hyphens, at most %d, not %q", wikiSlugLimit, slug)
	}
	if _, isID := ParseID(slug); isID {
		return fmt.Errorf("%q is shaped like an id", slug)
	}
	return nil
}

type wikiPage struct {
	Document string     `json:"document"`
	Parent   string     `json:"parent"`
	Slug     string     `json:"slug"`
	Name     string     `json:"name"`
	Body     string     `json:"body"`
	Sides    []wikiSide `json:"sides"`
}

type wikiSide struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Body string `json:"body"`
}

func registerWiki(mux *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens) {
	mux.HandleFunc("POST /api/do/wiki", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		var page wikiPage
		if !serve.Decode(w, r, &page) {
			return
		}
		id, hash, err := saveWiki(r.Context(), s, pics, actor, env, page)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "saved a wiki page", "viewer", env.Viewer, "document", id, "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: []string{id}, Hash: hash})
	})
}

func wikiNamed(ctx context.Context, m *Model, bucket *blob.Bucket, page wikiPage) (map[string]bool, error) {
	bodies := []string{page.Body}
	for _, side := range page.Sides {
		bodies = append(bodies, side.Body)
	}
	if page.Sides == nil && page.Document != "" {
		for _, row := range m.Table("DOCUMENT").Referencing("parent", page.Document) {
			if row["relation"] != "side" {
				continue
			}
			raw, _, err := m.contentBytes(ctx, bucket, row["content"])
			if err != nil {
				return nil, err
			}
			bodies = append(bodies, string(raw))
		}
	}
	out := map[string]bool{}
	for _, body := range bodies {
		for _, found := range wikiPicture.FindAllStringSubmatch(body, -1) {
			out[found[1]] = true
		}
	}
	return out, nil
}

func saveWiki(ctx context.Context, s *Store, pics *Pictures, actor access.Actor, env Env, page wikiPage) (string, string, error) {
	name := strings.TrimSpace(page.Name)
	if name == "" {
		return "", "", access.Invalid("a wiki page needs a title")
	}
	if len(page.Body) > wikiLimit {
		return "", "", access.Invalid("a wiki page is at most %d bytes", wikiLimit)
	}
	slug := strings.TrimSpace(page.Slug)
	if err := checkWikiSlug(slug, "wiki"); err != nil {
		return "", "", access.Invalid("%v", err)
	}
	for _, side := range page.Sides {
		if strings.TrimSpace(side.Name) == "" {
			return "", "", access.Invalid("a side card needs a title")
		}
		if len(side.Body) > wikiLimit {
			return "", "", access.Invalid("a side card is at most %d bytes", wikiLimit)
		}
	}
	keep := func(body string) (madeBytes, error) {
		return keepBytes(ctx, s.Model(), pics.bucket, wikiMime, []byte(body))
	}
	main, err := keep(page.Body)
	if err != nil {
		return "", "", err
	}
	sides := make([]madeBytes, len(page.Sides))
	for i, side := range page.Sides {
		if sides[i], err = keep(side.Body); err != nil {
			return "", "", err
		}
	}
	named, err := wikiNamed(ctx, s.Model(), pics.bucket, page)
	if err != nil {
		return "", "", err
	}
	var id string
	_, err = s.Transact(ctx, actor, func(tx *store.Tx[Model]) error {
		m := s.In(tx)
		if err := m.wikiParentFits(page.Document, page.Parent); err != nil {
			return err
		}
		for photo := range named {
			if row, ok := m.Table("PHOTO").Get(photo); ok && (page.Document == "" || row["document"] != page.Document) {
				return access.Invalid("a picture on this page is another page's; insert it again here")
			}
		}
		w := &pendingWrites{}
		contents := map[string]string{}
		contentID, err := stageContent(m, w, contents, main)
		if err != nil {
			return err
		}
		sideContents := make([]string, len(page.Sides))
		for i := range page.Sides {
			if sideContents[i], err = stageContent(m, w, contents, sides[i]); err != nil {
				return err
			}
		}
		segment := wikiSegment(store.Row{"id": page.Document, "name": name, "slug": slug})
		if segment != "" && m.wikiSibling(page.Document, page.Parent, segment) != "" {
			return access.Invalid("another page here already has the address %s", m.wikiPath(page.Parent, segment))
		}
		edit := Edit{Set: page.Document, Cells: map[string]any{"name": name, "content": contentID}}
		if page.Document == "" {
			edit = Edit{Insert: "DOCUMENT", Row: map[string]any{"kind": "wiki", "name": name, "content": contentID, "parent": page.Parent, "slug": slug, "order": m.lastWikiOrder(page.Parent), "author": env.Viewer, "published": env.Now.In(School).Format(cells.StampFormat)}}
		} else {
			old, _ := m.Table("DOCUMENT").Get(page.Document)
			if old["parent"] != page.Parent {
				edit.Cells["parent"], edit.Cells["order"] = page.Parent, m.lastWikiOrder(page.Parent)
			}
			if old["slug"] != slug {
				edit.Cells["slug"] = slug
			}
		}
		at := len(w.edits)
		written := w.add(edit)
		commit := func() error {
			ids, err := stageEdits(s, tx, env, w.edits, wikiWhere, false)
			if err != nil {
				return err
			}
			id = ids[at]
			return nil
		}
		if page.Document != "" {
			for _, photo := range m.Table("PHOTO").Referencing("document", page.Document) {
				if !named[photo["id"]] {
					w.add(Edit{Delete: photo["id"]})
				}
			}
		}
		if page.Sides == nil {
			return commit()
		}
		existing := map[string]store.Row{}
		if page.Document != "" {
			for _, row := range m.Table("DOCUMENT").Referencing("parent", page.Document) {
				if row["relation"] == "side" {
					existing[row["id"]] = row
				}
			}
		}
		orders := store.Order(make([]string, len(page.Sides)))
		for i, side := range page.Sides {
			cells := map[string]any{"name": strings.TrimSpace(side.Name), "content": sideContents[i], "order": orders[i]}
			if side.ID == "" {
				cells["parent"], cells["relation"] = written, "side"
				w.add(Edit{Insert: "DOCUMENT", Row: cells})
				continue
			}
			old, ok := existing[side.ID]
			if !ok {
				return access.Invalid("%q is not one of this page's side cards", side.ID)
			}
			delete(existing, side.ID)
			for k, v := range cells {
				if old[k] == v {
					delete(cells, k)
				}
			}
			if len(cells) == 0 {
				continue
			}
			w.add(Edit{Set: side.ID, Cells: cells})
		}
		for _, gone := range slices.Sorted(maps.Keys(existing)) {
			w.add(Edit{Delete: gone})
		}
		return commit()
	})
	if err != nil {
		return "", "", err
	}
	return id, main.hash, nil
}

func (m *Model) wikiParentFits(document, parent string) error {
	docs := m.Table("DOCUMENT")
	if parent == "" {
		return nil
	}
	if row, ok := docs.Get(parent); !ok || row["kind"] != "wiki" {
		return access.Invalid("%q is not a wiki page", parent)
	}
	for at := parent; at != ""; {
		if at == document {
			return access.Invalid("a page can't go under itself or one of its own sub-pages")
		}
		row, _ := docs.Get(at)
		at = row["parent"]
	}
	return nil
}

func wikiSegment(row store.Row) string {
	if row["slug"] != "" {
		return row["slug"]
	}
	if segment := strings.Trim(wikiNameRun.ReplaceAllString(row["name"], "-"), "-"); segment != "" {
		return segment
	}
	return row["id"]
}

func (m *Model) wikiPath(parent, segment string) string {
	docs := m.Table("DOCUMENT")
	return wikiPathIn(func(id string) store.Row {
		row, _ := docs.Get(id)
		return row
	}, parent, segment)
}

func wikiPathIn(docs func(id string) store.Row, parent, segment string) string {
	parts := []string{segment}
	seen := map[string]bool{}
	// Hrefs are generated before the load refuses a loop of parents.
	for at := parent; at != "" && !seen[at]; {
		seen[at] = true
		row := docs(at)
		parts = append([]string{wikiSegment(row)}, parts...)
		at = row["parent"]
	}
	return wikiPathPrefix + strings.Join(parts, "/")
}

func (m *Model) WikiPath(row store.Row) string {
	return m.wikiPath(row["parent"], wikiSegment(row))
}

func (m *Model) wikiChildren(parent string) []store.Row {
	docs := m.Table("DOCUMENT")
	rows := docs.Referencing("parent", parent)
	if parent == "" {
		rows = docs.All()
	}
	children := []store.Row{}
	for _, row := range rows {
		if row["kind"] == "wiki" && row["parent"] == parent {
			children = append(children, row)
		}
	}
	slices.SortFunc(children, func(a, b store.Row) int {
		if a["order"] != b["order"] {
			if a["order"] == "" || b["order"] == "" {
				return strings.Compare(b["order"], a["order"])
			}
			return store.CompareKeys(a["order"], b["order"])
		}
		return strings.Compare(a["name"], b["name"])
	})
	return children
}

func (m *Model) wikiSibling(document, parent, segment string) string {
	for _, row := range m.wikiChildren(parent) {
		if row["id"] != document && strings.EqualFold(wikiSegment(row), segment) {
			return row["id"]
		}
	}
	return ""
}

func (m *Model) wikiRedirect(path string) string {
	for _, row := range m.Table("REDIRECT").All() {
		if row["app"] != "wiki" || !strings.EqualFold(row["old"], path) {
			continue
		}
		if id, ok := strings.CutPrefix(row["new"], wikiPathPrefix); ok {
			if page, ok := m.Table("DOCUMENT").Get(id); ok && page["kind"] == "wiki" {
				return id
			}
		}
	}
	return ""
}

func (m *Model) wikiPageAt(key string) string {
	segments := strings.Split(strings.Trim(key, "/"), "/")
	if row, ok := m.Table("DOCUMENT").Get(segments[0]); ok && row["kind"] == "wiki" && len(segments) == 1 {
		return segments[0]
	}
	at := ""
	for i, segment := range segments {
		next := m.wikiSibling("", at, segment)
		if next == "" {
			next = m.wikiRedirect(wikiPathPrefix + strings.Join(segments[:i+1], "/"))
		}
		if next == "" {
			next = m.wikiRedirect(m.wikiPath(at, segment))
		}
		if next == "" {
			return ""
		}
		at = next
	}
	return at
}

func (m *Model) lastWikiOrder(parent string) string {
	keys := []string{}
	for _, row := range m.Table("DOCUMENT").All() {
		if row["kind"] == "wiki" && row["parent"] == parent && row["order"] != "" {
			keys = append(keys, row["order"])
		}
	}
	slices.SortFunc(keys, store.CompareKeys)
	return store.Order(append(keys, ""))[len(keys)]
}
