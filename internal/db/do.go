package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	doPrefix      = "/api/do/"
	photoLimit    = 30 << 20
	fileLimit     = 200 << 20
	formMemory    = 32 << 20
	contentFolder = "content"
)

type stored struct {
	Result []string `json:"result"`
	Hash   string   `json:"hash"`
}

func registerDo(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, importKey []byte, now func() time.Time) {
	registerWiki(mux, s, queue, pics, importKey, now)
	mux.HandleFunc("POST "+doPrefix+"photo", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoLimit)
		if err := r.ParseMultipartForm(photoLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the photo as multipart form data: %v", err))
			return
		}
		person, group := r.FormValue("person"), r.FormValue("group")
		if (person == "") == (group == "") {
			serve.Error(w, r, access.Invalid("a photo is of a person or of a group: send one of them"))
			return
		}
		img, err := readImage(r, "photo")
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		box, err := formBox(r)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		m := s.Model()
		row := store.Row{"person": person, "group": group, "original": pictureFolder + "/" + img.name}
		maps.Copy(row, box)
		if err := m.Authorize(env, Change{Table: "PHOTO", New: row}); err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := pics.bucket.Put(r.Context(), row["original"], img.mimeType, img.content); err != nil {
			serve.Error(w, r, err)
			return
		}
		cells := map[string]any{"order": m.firstOrder(person, group)}
		for k, v := range row {
			if v != "" {
				cells[k] = v
			}
		}
		ids, err := Write(r.Context(), s, queue, pics, actor, env, Batch{Batch: []Edit{{Insert: "PHOTO", Row: cells}}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a photo", "viewer", env.Viewer, "system", env.System, "person", person, "group", group, "original", row["original"])
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: strings.TrimSuffix(img.name, "."+img.ext)})
	})
	mux.HandleFunc("POST "+doPrefix+"file", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, fileLimit)
		if err := r.ParseMultipartForm(formMemory); err != nil {
			serve.Error(w, r, access.Invalid("send the file as multipart form data: %v", err))
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			serve.Error(w, r, access.Invalid("the file is required"))
			return
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the file"))
			return
		}
		mimeType := baseType(header.Header.Get("Content-Type"))
		if mimeType == "" || mimeType == "application/octet-stream" {
			mimeType = baseType(http.DetectContentType(content))
		}
		var root map[string]any
		var sentTo []string
		if mimeType == "message/rfc822" {
			root, sentTo, err = s.Model().mailRoot(content)
		} else {
			root, err = fileRoot(r, mimeType)
		}
		if err != nil {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		document, hash, err := storeRoot(r.Context(), s, queue, pics, actor, env, content, mimeType, root, sentTo)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a file", "viewer", env.Viewer, "system", env.System, "document", document, "hash", hash, "kind", root["kind"], "mime", mimeType)
		serve.Write(w, r, http.StatusOK, stored{Result: []string{document}, Hash: hash})
	})
	mux.HandleFunc("POST "+doPrefix+"fetched", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, fetchLimit+1<<20)
		if err := r.ParseMultipartForm(fetchLimit); err != nil {
			serve.Error(w, r, access.Invalid("send what was fetched as multipart form data: %v", err))
			return
		}
		id := r.FormValue("document")
		doc, found := s.Model().Table("DOCUMENT").Get(id)
		if !found || !slices.Contains(fetchedRelations, doc["relation"]) || doc["url"] == "" || doc["content"] != "" {
			serve.Error(w, r, access.Invalid("%q is not an image or a link still to fetch", id))
			return
		}
		answer, err := storeFetched(r, s, queue, pics, actor, env, id, doc["relation"])
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "filled a linked document", "viewer", env.Viewer, "system", env.System, "document", id, "hash", answer.Hash, "fetch", answer.Fetch, "why", answer.Why)
		serve.Write(w, r, http.StatusOK, answer)
	})
}

type fetchedAnswer struct {
	Result []string `json:"result"`
	Hash   string   `json:"hash,omitempty"`
	Fetch  string   `json:"fetch,omitempty"`
	Why    string   `json:"why,omitempty"`
}

func storeFetched(r *http.Request, s *Store, queue *store.Queue, pics *Pictures, actor access.Actor, env Env, id, relation string) (fetchedAnswer, error) {
	stop := func(why, reason string) (fetchedAnswer, error) {
		if _, err := Write(r.Context(), s, queue, pics, actor, env, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"fetch": why}}}}); err != nil {
			return fetchedAnswer{}, err
		}
		return fetchedAnswer{Result: []string{id}, Fetch: why, Why: reason}, nil
	}
	if why := r.FormValue("stop"); why != "" {
		return stop(why, "the fetcher stopped")
	}
	file, _, err := r.FormFile("body")
	if err != nil {
		return fetchedAnswer{}, access.Invalid("the body, or a stop, is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return fetchedAnswer{}, access.Invalid("could not read the body")
	}
	if len(content) > fetchLimit {
		return stop("refused", fmt.Sprintf("larger than %d bytes", fetchLimit))
	}
	mimeType, err := keptBody(relation, content)
	if err != nil {
		return stop("refused", err.Error())
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	name := contentFolder + "/" + hash
	m := s.Model()
	_, found := m.Table("CONTENT").Find(hash, mimeType)
	if !found {
		if err := m.Authorize(env, Change{Table: "CONTENT", New: store.Row{"hash": hash, "blob": name, "mime": mimeType, "size": strconv.Itoa(len(content))}}); err != nil {
			return fetchedAnswer{}, err
		}
		if err := pics.bucket.Put(r.Context(), name, mimeType, content); err != nil {
			return fetchedAnswer{}, err
		}
	}
	whole := env.System == importReader
	authorize := func(m *Model, c Change) error {
		return m.Authorize(env, c)
	}
	_, err = queue.Transact(r.Context(), actor, func(tx *store.Tx) error {
		stage := func(e Edit) (string, error) {
			written, c, err := stageWrite(s, tx, e, "fetched", nil, whole, authorize)
			if err != nil {
				return "", err
			}
			if err := fire(s, tx, c, "fetched"); err != nil {
				return "", err
			}
			pics.watch(tx, c)
			return written, nil
		}
		held, err := stageContent(s, tx, stage, map[string]string{}, hash, mimeType, len(content))
		if err != nil {
			return err
		}
		_, err = stage(Edit{Set: id, Cells: map[string]any{"fetch": "", "content": held}})
		return err
	})
	if err != nil {
		if !found {
			dropUnheld(s, pics.bucket, []string{name})
		}
		return fetchedAnswer{}, err
	}
	return fetchedAnswer{Result: []string{id}, Hash: hash}, nil
}

func fileRoot(r *http.Request, mimeType string) (map[string]any, error) {
	kind := strings.TrimSpace(r.FormValue("kind"))
	switch kind {
	case "":
		kind = "file"
	case "calendar":
		if mimeType != pdfType {
			return nil, fmt.Errorf("the year calendar is a pdf, not %s", mimeType)
		}
	default:
		return nil, fmt.Errorf("kind %q is not calendar", kind)
	}
	address, published := strings.TrimSpace(r.FormValue("url")), strings.TrimSpace(r.FormValue("published"))
	if address == "" {
		return nil, fmt.Errorf("a file needs the url it came from")
	}
	if _, err := time.ParseInLocation(cells.StampFormat, published, School); err != nil {
		return nil, fmt.Errorf("published %q is not a moment like %s", published, cells.StampFormat)
	}
	root := map[string]any{"kind": kind, "url": address, "published": published}
	if name := strings.TrimSpace(r.FormValue("name")); name != "" {
		root["name"] = name
	}
	return root, nil
}

func storeRoot(ctx context.Context, s *Store, queue *store.Queue, pics *Pictures, actor access.Actor, env Env, content []byte, mimeType string, root map[string]any, sentTo []string) (string, string, error) {
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	m := s.Model()
	if err := m.Authorize(env, Change{Table: "DOCUMENT", New: store.Row{"kind": root["kind"].(string)}}); err != nil {
		return "", "", err
	}
	existing, found := m.Table("CONTENT").Find(hash, mimeType)
	if found {
		for _, row := range m.Table("DOCUMENT").Referencing("content", existing["id"]) {
			if row["parent"] != "" || row["kind"] != root["kind"] {
				continue
			}
			held := map[string]bool{}
			for _, link := range m.Table("DOCUMENT_GROUP").Referencing("document", row["id"]) {
				if link["relation"] == "sent_to" {
					held[link["group"]] = true
				}
			}
			edits := []Edit{}
			for _, group := range sentTo {
				if !held[group] {
					edits = append(edits, Edit{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": row["id"], "group": group, "relation": "sent_to"}})
				}
			}
			if len(edits) > 0 {
				if _, err := Write(ctx, s, queue, pics, actor, env, Batch{Batch: edits}); err != nil {
					return "", "", err
				}
			}
			return row["id"], hash, nil
		}
	}
	edits := []Edit{}
	root["content"] = existing["id"]
	if !found {
		name, size := contentFolder+"/"+hash, strconv.Itoa(len(content))
		if err := m.Authorize(env, Change{Table: "CONTENT", New: store.Row{"hash": hash, "blob": name, "mime": mimeType, "size": size}}); err != nil {
			return "", "", err
		}
		if err := pics.bucket.Put(ctx, name, mimeType, content); err != nil {
			return "", "", err
		}
		edits = append(edits, Edit{Insert: "CONTENT", As: "content", Row: map[string]any{"hash": hash, "blob": name, "mime": mimeType, "size": size}})
		root["content"] = "@content"
	}
	edits = append(edits, Edit{Insert: "DOCUMENT", As: "document", Row: root})
	at := len(edits) - 1
	for _, group := range sentTo {
		edits = append(edits, Edit{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": "@document", "group": group, "relation": "sent_to"}})
	}
	ids, err := Write(ctx, s, queue, pics, actor, env, Batch{Batch: edits})
	if err != nil {
		if !found {
			dropUnheld(s, pics.bucket, []string{contentFolder + "/" + hash})
		}
		return "", "", err
	}
	return ids[at], hash, nil
}

func FileMail(ctx context.Context, s *Store, queue *store.Queue, pics *Pictures, raw []byte) (string, error) {
	root, sentTo, err := s.Model().mailRoot(raw)
	if errors.Is(err, errLoopMail) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	env := Env{System: importReader, Now: time.Now()}
	document, hash, err := storeRoot(ctx, s, queue, pics, access.System(importReader), env, raw, "message/rfc822", root, sentTo)
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "filed mail", "document", document, "hash", hash, "groups", len(sentTo))
	return document, nil
}

type upload struct {
	content       []byte
	mimeType, ext string
	name          string
}

func readImage(r *http.Request, field string) (upload, error) {
	file, _, err := r.FormFile(field)
	if err != nil {
		return upload{}, access.Invalid("the %s is required", field)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return upload{}, access.Invalid("could not read the %s", field)
	}
	mimeType := http.DetectContentType(content)
	ext, ok := blob.ImageExtensions[mimeType]
	if !ok {
		return upload{}, access.Invalid("%s is not a supported %s", mimeType, field)
	}
	if err := blob.Check(content); err != nil {
		return upload{}, access.Invalid("could not read the %s: %v", field, err)
	}
	return upload{content: content, mimeType: mimeType, ext: ext, name: blob.Name(content, ext)}, nil
}

func formBox(r *http.Request) (store.Row, error) {
	names := cropColumns
	values := []string{}
	for _, name := range names {
		values = append(values, strings.TrimSpace(r.FormValue(name)))
	}
	if !slices.ContainsFunc(values, func(v string) bool { return v != "" }) {
		return store.Row{}, nil
	}
	out := store.Row{}
	for i, v := range values {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || (i >= 2 && n == 0) {
			return nil, access.Invalid("a crop box is %s, whole numbers with a width and height above zero", strings.Join(names, ", "))
		}
		out[names[i]] = v
	}
	return out, nil
}

func (m *Model) firstOrder(person, group string) string {
	keys := []string{""}
	of, id := "person", person
	if group != "" {
		of, id = "group", group
	}
	for _, row := range m.Table("PHOTO").Referencing(of, id) {
		if row["order"] != "" {
			keys = append(keys, row["order"])
		}
	}
	slices.SortFunc(keys[1:], store.CompareKeys)
	return store.Order(keys)[0]
}
