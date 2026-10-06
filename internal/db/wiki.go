package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
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

var wikiImage = regexp.MustCompile(`^wiki-images/[0-9a-f]{64}\.(jpg|png|gif|webp)$`)

type wikiPage struct {
	Document string     `json:"document"`
	Parent   string     `json:"parent"`
	Name     string     `json:"name"`
	Body     string     `json:"body"`
	Sides    []wikiSide `json:"sides"`
}

type wikiSide struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Body string `json:"body"`
}

type markdown struct {
	hash string
	size int
}

func registerWiki(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, importKey []byte, now func() time.Time) {
	mux.HandleFunc("POST "+doPrefix+"wiki", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		var page wikiPage
		if !serve.Decode(w, r, &page) {
			return
		}
		id, hash, err := saveWiki(r.Context(), s, queue, pics, actor, env, page)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "saved a wiki page", "viewer", env.Viewer, "document", id, "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: []string{id}, Hash: hash})
	})
	mux.HandleFunc("GET /api/wiki/picture/{name}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := caller(w, r, s.Model(), importKey, now()); !ok {
			return
		}
		object := "wiki-images/" + r.PathValue("name")
		if !wikiImage.MatchString(object) {
			http.NotFound(w, r)
			return
		}
		data, mimeType, err := pics.bucket.Get(r.Context(), object)
		if errors.Is(err, blob.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		w.Header().Set("Content-Security-Policy", "sandbox")
		if _, err := w.Write(data); err != nil {
			slog.ErrorContext(r.Context(), "write a wiki picture", "object", object, "error", err)
		}
	})
}

func saveWiki(ctx context.Context, s *Store, queue *store.Queue, pics *Pictures, actor access.Actor, env Env, page wikiPage) (string, string, error) {
	name := strings.TrimSpace(page.Name)
	if name == "" {
		return "", "", access.Invalid("a wiki page needs a title")
	}
	if len(page.Body) > wikiLimit {
		return "", "", access.Invalid("a wiki page is at most %d bytes", wikiLimit)
	}
	for _, side := range page.Sides {
		if strings.TrimSpace(side.Name) == "" {
			return "", "", access.Invalid("a side card needs a title")
		}
		if len(side.Body) > wikiLimit {
			return "", "", access.Invalid("a side card is at most %d bytes", wikiLimit)
		}
	}
	stored := []string{}
	keep := func(body string) (markdown, error) {
		content := []byte(body)
		sum := sha256.Sum256(content)
		md := markdown{hash: hex.EncodeToString(sum[:]), size: len(content)}
		if _, held := s.Model().Table("CONTENT").Find(md.hash, wikiMime); held || slices.Contains(stored, contentFolder+"/"+md.hash) {
			return md, nil
		}
		if err := pics.bucket.Put(ctx, contentFolder+"/"+md.hash, wikiMime, content); err != nil {
			return md, err
		}
		stored = append(stored, contentFolder+"/"+md.hash)
		return md, nil
	}
	main, err := keep(page.Body)
	if err != nil {
		return "", "", err
	}
	sides := make([]markdown, len(page.Sides))
	for i, side := range page.Sides {
		if sides[i], err = keep(side.Body); err != nil {
			dropUnheld(s, pics.bucket, stored)
			return "", "", err
		}
	}
	var id string
	_, err = queue.Transact(ctx, actor, func(tx *store.Tx) error {
		m := s.In(tx)
		if err := m.wikiParentFits(page.Document, page.Parent); err != nil {
			return err
		}
		contents := map[string]string{}
		contentOf := func(md markdown) (string, error) {
			return stageContent(s, tx, stager(s, tx, wikiWhere), contents, md.hash, wikiMime, md.size)
		}
		apply := func(e Edit) (string, error) {
			written, c, err := stageWrite(s, tx, e, wikiWhere, nil, false, func(m *Model, c Change) error { return m.Authorize(env, c) })
			if err != nil {
				return "", err
			}
			return written, fire(s, tx, c, wikiWhere)
		}
		contentID, err := contentOf(main)
		if err != nil {
			return err
		}
		edit := Edit{Set: page.Document, Cells: map[string]any{"name": name, "content": contentID}}
		if page.Document == "" {
			edit = Edit{Insert: "DOCUMENT", Row: map[string]any{"kind": "wiki", "name": name, "content": contentID, "parent": page.Parent, "order": m.lastWikiOrder(page.Parent), "author": env.Viewer, "published": env.Now.In(School).Format(cells.StampFormat)}}
		} else if old, _ := m.Table("DOCUMENT").Get(page.Document); old["parent"] != page.Parent {
			edit.Cells["parent"], edit.Cells["order"] = page.Parent, m.lastWikiOrder(page.Parent)
		}
		if id, err = apply(edit); err != nil {
			return err
		}
		if page.Sides == nil {
			return nil
		}
		existing := map[string]store.Row{}
		for _, row := range s.In(tx).Table("DOCUMENT").Referencing("parent", id) {
			if row["relation"] == "side" {
				existing[row["id"]] = row
			}
		}
		orders := store.Order(make([]string, len(page.Sides)))
		for i, side := range page.Sides {
			cardContent, err := contentOf(sides[i])
			if err != nil {
				return err
			}
			cells := map[string]any{"name": strings.TrimSpace(side.Name), "content": cardContent, "order": orders[i]}
			if side.ID == "" {
				cells["parent"], cells["relation"] = id, "side"
				if _, err := apply(Edit{Insert: "DOCUMENT", Row: cells}); err != nil {
					return err
				}
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
			if _, err := apply(Edit{Set: side.ID, Cells: cells}); err != nil {
				return err
			}
		}
		for _, gone := range slices.Sorted(maps.Keys(existing)) {
			if _, err := apply(Edit{Delete: gone}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		dropUnheld(s, pics.bucket, stored)
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
