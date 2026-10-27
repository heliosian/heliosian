package db

import (
	"context"
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
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	photoLimit      = 30 << 20
	fileLimit       = 200 << 20
	formMemory      = 32 << 20
	thumbnailUpload = 20 << 20
)

type stored struct {
	Result []string `json:"result"`
	Hash   string   `json:"hash"`
}

func registerDo(mux *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens) {
	registerPDFPages(mux, s, pics, tokens)
	registerWiki(mux, s, pics, tokens)
	registerSubscription(mux, s, tokens)
	mux.HandleFunc("POST /api/do/photo", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoLimit)
		if err := r.ParseMultipartForm(photoLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the photo as multipart form data: %v", err))
			return
		}
		of := map[string]string{}
		for _, col := range []string{"person", "group", "document"} {
			if v := r.FormValue(col); v != "" {
				of[col] = v
			}
		}
		if len(of) != 1 {
			serve.Error(w, r, access.Invalid("a photo is of a person, a group or a wiki page: send one of them"))
			return
		}
		box, err := formBox(r)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		m := s.Model()
		row := store.Row{}
		maps.Copy(row, of)
		maps.Copy(row, box)
		if err := m.Authorize(env, Change{Table: "PHOTO", New: row}); err != nil {
			serve.Error(w, r, err)
			return
		}
		img, err := pictureOf(r, m, "photo")
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		if photo, hash, ok := m.pagePhoto(of["document"], img); ok {
			serve.Write(w, r, http.StatusOK, stored{Result: []string{photo}, Hash: hash})
			return
		}
		cells := map[string]any{}
		for k, v := range row {
			cells[k] = v
		}
		if of["document"] == "" {
			cells["order"] = m.firstOrder(of["person"], of["group"])
		}
		ids, hash, err := writeUpload(r.Context(), s, pics, actor, env, img, func(content string) Edit {
			cells["original"] = content
			return Edit{Insert: "PHOTO", Row: cells}
		})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a photo", "viewer", env.Viewer, "of", of, "photo", ids[len(ids)-1], "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: ids[len(ids)-1:], Hash: hash})
	})
	mux.HandleFunc("POST /api/do/flyer", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoLimit)
		if err := r.ParseMultipartForm(photoLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the flyer as multipart form data: %v", err))
			return
		}
		id := r.FormValue("group")
		ids, hash, err := setUpload(r, s, pics, actor, env, "GROUP", id, "flyer_original", func(m *Model) (upload, error) {
			return pictureOf(r, m, "flyer")
		})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "set a flyer", "viewer", env.Viewer, "group", id, "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: hash})
	})
	mux.HandleFunc("POST /api/do/pronunciation", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoLimit)
		if err := r.ParseMultipartForm(photoLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the recording as multipart form data: %v", err))
			return
		}
		person, group := r.FormValue("person"), r.FormValue("group")
		if (person == "") == (group == "") {
			serve.Error(w, r, access.Invalid("a recording is of a person or of a family: send one of them"))
			return
		}
		id, table := person, "PERSON"
		if group != "" {
			id, table = group, "GROUP"
		}
		ids, hash, err := setUpload(r, s, pics, actor, env, table, id, "pronunciation_original", func(*Model) (upload, error) {
			return readRecording(r)
		})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "recorded a pronunciation", "viewer", env.Viewer, "of", id, "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: hash})
	})
	mux.HandleFunc("POST /api/do/file", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
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
		document, hash, err := storeRoot(r.Context(), s, pics, actor, env, content, mimeType, root, sentTo, strings.TrimSpace(r.FormValue("supersedes")))
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a file", "viewer", env.Viewer, "document", document, "hash", hash, "kind", root["kind"], "mime", mimeType)
		serve.Write(w, r, http.StatusOK, stored{Result: []string{document}, Hash: hash})
	})
	mux.HandleFunc("POST /api/do/person-at", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		if _, _, ok := caller(w, r, m, tokens); !ok {
			return
		}
		var ask struct {
			Address string `json:"address"`
		}
		if !serve.Decode(w, r, &ask) {
			return
		}
		serve.Write(w, r, http.StatusOK, map[string]string{"person": m.personAt(ask.Address)})
	})
	mux.HandleFunc("POST /api/do/fetched", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
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
		answer, err := storeFetched(r, s, pics, actor, env, id, doc["relation"])
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "filled a linked document", "viewer", env.Viewer, "document", id, "hash", answer.Hash, "fetch", answer.Fetch, "why", answer.Why)
		serve.Write(w, r, http.StatusOK, answer)
	})
	mux.HandleFunc("POST /api/do/thumbnail", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), tokens)
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, thumbnailUpload+1<<20)
		if err := r.ParseMultipartForm(thumbnailUpload); err != nil {
			serve.Error(w, r, access.Invalid("send the picture as multipart form data: %v", err))
			return
		}
		id := r.FormValue("document")
		doc, found := s.Model().Table("DOCUMENT").Get(id)
		if !found || doc["content"] == "" || (doc["thumbnailed"] != "" && doc["thumbnail"] != "") {
			serve.Error(w, r, access.Invalid("%q is not a document still to thumbnail", id))
			return
		}
		if drawn := r.FormValue("content"); drawn != doc["content"] {
			serve.Error(w, r, access.Invalid("%q holds %q now, not the %q drawn", id, doc["content"], drawn))
			return
		}
		file, _, err := r.FormFile("picture")
		if err != nil {
			serve.Error(w, r, access.Invalid("the picture is required"))
			return
		}
		defer file.Close()
		picture, err := io.ReadAll(file)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the picture"))
			return
		}
		thumb, err := blob.Thumbnail(picture)
		if err != nil {
			serve.Error(w, r, access.Invalid("the picture: %v", err))
			return
		}
		if err := saveThumbnail(r.Context(), s, pics.bucket, actor, env, doc, thumb, s.Model().whole(env)); err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a thumbnail", "viewer", env.Viewer, "document", id, "bytes", len(thumb))
		serve.Write(w, r, http.StatusOK, stored{Result: []string{id}, Hash: blob.Hash(thumb)})
	})
}

type fetchedAnswer struct {
	Result []string `json:"result"`
	Hash   string   `json:"hash,omitempty"`
	Fetch  string   `json:"fetch,omitempty"`
	Why    string   `json:"why,omitempty"`
}

func storeFetched(r *http.Request, s *Store, pics *Pictures, actor access.Actor, env Env, id, relation string) (fetchedAnswer, error) {
	stop := func(why, reason string) (fetchedAnswer, error) {
		if _, err := Write(r.Context(), s, actor, env, Batch{Batch: []Edit{{Set: id, Cells: map[string]any{"fetch": why}}}}); err != nil {
			return fetchedAnswer{}, err
		}
		return fetchedAnswer{Result: []string{id}, Fetch: why, Why: reason}, nil
	}
	file, _, err := r.FormFile("body")
	if err != nil {
		return fetchedAnswer{}, access.Invalid("the body is required")
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
	made := madeOf(content, mimeType)
	m := s.Model()
	if _, found := m.Table("CONTENT").Find(made.hash, mimeType); !found {
		if err := m.Authorize(env, Change{Table: "CONTENT", New: made.row()}); err != nil {
			return fetchedAnswer{}, err
		}
	}
	kept, err := keepBytes(r.Context(), m, pics.bucket, mimeType, content)
	if err != nil {
		return fetchedAnswer{}, err
	}
	whole := m.whole(env)
	_, err = s.Transact(r.Context(), actor, func(tx *store.Tx[Model]) error {
		w := &pendingWrites{}
		held, err := stageContent(s.In(tx), w, map[string]string{}, kept)
		if err != nil {
			return err
		}
		w.add(Edit{Set: id, Cells: map[string]any{"fetch": "", "content": held}})
		_, err = stageEdits(s, tx, env, w.edits, "fetched", whole)
		return err
	})
	if err != nil {
		return fetchedAnswer{}, err
	}
	return fetchedAnswer{Result: []string{id}, Hash: made.hash}, nil
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

func storeRoot(ctx context.Context, s *Store, pics *Pictures, actor access.Actor, env Env, content []byte, mimeType string, root map[string]any, sentTo []string, supersedes string) (string, string, error) {
	made := madeOf(content, mimeType)
	hash := made.hash
	m := s.Model()
	if err := m.Authorize(env, Change{Table: "DOCUMENT", New: store.Row{"kind": root["kind"].(string)}}); err != nil {
		return "", "", err
	}
	if supersedes != "" {
		old, ok := m.Table("DOCUMENT").Get(supersedes)
		if !ok {
			return "", "", access.Invalid("no document %s to supersede", supersedes)
		}
		if old["kind"] != root["kind"] || old["parent"] != "" || mimeType == "message/rfc822" {
			return "", "", access.Invalid("supersedes must name a root of the same kind as the imported file")
		}
	}
	existing, found := m.Table("CONTENT").Find(hash, mimeType)
	if found {
		for _, row := range m.Table("DOCUMENT").Referencing("content", existing["id"]) {
			if row["parent"] != "" || row["kind"] != root["kind"] {
				continue
			}
			return row["id"], hash, addSentTo(ctx, s, actor, env, row["id"], sentTo, supersedes)
		}
	}
	edits := []Edit{}
	root["content"] = existing["id"]
	if !found {
		if err := m.Authorize(env, Change{Table: "CONTENT", New: made.row()}); err != nil {
			return "", "", err
		}
		if _, err := keepBytes(ctx, m, pics.bucket, mimeType, content); err != nil {
			return "", "", err
		}
		edits = append(edits, made.insert("content"))
		root["content"] = "@content"
	}
	edits = append(edits, Edit{Insert: "DOCUMENT", As: "document", Row: root})
	at := len(edits) - 1
	for _, group := range sentTo {
		edits = append(edits, Edit{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": "@document", "group": group, "relation": "sent_to"}})
	}
	if supersedes != "" {
		edits = append(edits, Edit{Set: supersedes, Cells: map[string]any{"superseded_by": "@document"}})
	}
	ids, err := Write(ctx, s, actor, env, Batch{Batch: edits})
	if err != nil {
		return "", "", err
	}
	return ids[at], hash, nil
}

func addSentTo(ctx context.Context, s *Store, actor access.Actor, env Env, document string, sentTo []string, supersedes string) error {
	held := map[string]bool{}
	for _, group := range s.Model().sentTo(document) {
		held[group] = true
	}
	edits := []Edit{}
	for _, group := range sentTo {
		if !held[group] {
			edits = append(edits, Edit{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": document, "group": group, "relation": "sent_to"}})
		}
	}
	if supersedes != "" && supersedes != document {
		previous, _ := s.Model().Table("DOCUMENT").Get(document)
		if previous["superseded_by"] == supersedes {
			edits = append(edits, Edit{Set: document, Cells: map[string]any{"superseded_by": ""}})
		}
		edits = append(edits, Edit{Set: supersedes, Cells: map[string]any{"superseded_by": document}})
	}
	if len(edits) == 0 {
		return nil
	}
	_, err := Write(ctx, s, actor, env, Batch{Batch: edits})
	return err
}

func FileMail(ctx context.Context, s *Store, pics *Pictures, raw []byte) (string, error) {
	m := s.Model()
	root, sentTo, err := m.mailRoot(raw)
	if errors.Is(err, errLoopMail) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	env := Env{System: "mail", Now: time.Now()}
	sender, err := senderOf(raw)
	if err != nil {
		return "", err
	}
	copied, err := m.copyOf(ctx, pics.bucket, root["name"].(string), root["published"].(string), sender)
	if err != nil {
		return "", err
	}
	if copied != "" {
		slog.InfoContext(ctx, "filed mail: a copy of mail already filed", "document", copied, "groups", len(sentTo))
		return copied, addSentTo(ctx, s, access.System(env.System), env, copied, sentTo, "")
	}
	document, hash, err := storeRoot(ctx, s, pics, access.System(env.System), env, raw, "message/rfc822", root, sentTo, "")
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "filed mail", "document", document, "hash", hash, "groups", len(sentTo))
	return document, nil
}

type upload struct {
	content  []byte
	mimeType string
	held     string
}

func pictureOf(r *http.Request, m *Model, field string) (upload, error) {
	id := r.FormValue("stock")
	if id == "" {
		return readImage(r, field)
	}
	row, ok := m.Table("STOCK").Get(id)
	if !ok || row["picture"] == "" {
		return upload{}, access.Invalid("%q is no stock picture someone picked", id)
	}
	return upload{held: row["picture"]}, nil
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
	if _, ok := blob.ImageExtensions[mimeType]; !ok {
		return upload{}, access.Invalid("%s is not a supported %s", mimeType, field)
	}
	if err := blob.Check(content); err != nil {
		return upload{}, access.Invalid("could not read the %s: %v", field, err)
	}
	return upload{content: content, mimeType: mimeType}, nil
}

func readRecording(r *http.Request) (upload, error) {
	file, header, err := r.FormFile("recording")
	if err != nil {
		return upload{}, access.Invalid("the recording is required")
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		return upload{}, access.Invalid("could not read the recording")
	}
	declared := header.Header.Get("Content-Type")
	mimeType, _, ok := blob.AudioType(declared)
	if !ok {
		return upload{}, access.Invalid("%s is not a supported recording", declared)
	}
	return upload{content: content, mimeType: mimeType}, nil
}

func (m *Model) pagePhoto(document string, up upload) (string, string, bool) {
	if document == "" {
		return "", "", false
	}
	content, ok := m.Table("CONTENT").Get(up.held)
	if !ok {
		content, ok = m.Table("CONTENT").Find(blob.Hash(up.content), up.mimeType)
	}
	if !ok {
		return "", "", false
	}
	photo, ok := m.Table("PHOTO").Find("", "", document, content["id"])
	return photo["id"], content["hash"], ok
}

func writeUpload(ctx context.Context, s *Store, pics *Pictures, actor access.Actor, env Env, up upload, edit func(content string) Edit) ([]string, string, error) {
	attach := func(content string) Edit {
		e := edit(content)
		e.uploaded = content
		return e
	}
	if up.held != "" {
		row, _ := s.Model().Table("CONTENT").Get(up.held)
		ids, err := Write(ctx, s, actor, env, Batch{Batch: []Edit{attach(up.held)}})
		return ids, row["hash"], err
	}
	kept, err := keepBytes(ctx, s.Model(), pics.bucket, up.mimeType, up.content)
	if err != nil {
		return nil, "", err
	}
	whole := s.Model().whole(env)
	var ids []string
	_, err = s.Transact(ctx, actor, func(tx *store.Tx[Model]) error {
		w := &pendingWrites{}
		content, err := stageContent(s.In(tx), w, map[string]string{}, kept)
		if err != nil {
			return err
		}
		w.add(attach(content))
		written, err := stageEdits(s, tx, env, w.edits, "batch", whole)
		ids = written
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return ids, kept.hash, nil
}

func setUpload(r *http.Request, s *Store, pics *Pictures, actor access.Actor, env Env, table, id, column string, read func(*Model) (upload, error)) ([]string, string, error) {
	m := s.Model()
	old, found := m.Shown(table).Get(id)
	if !found {
		return nil, "", access.Invalid("no %s %s", table, id)
	}
	updated := maps.Clone(old)
	updated[column] = "upload"
	if err := m.Authorize(env, Change{Table: table, Old: old, New: updated, uploaded: "upload"}); err != nil {
		return nil, "", err
	}
	up, err := read(m)
	if err != nil {
		return nil, "", err
	}
	ids, hash, err := writeUpload(r.Context(), s, pics, actor, env, up, func(content string) Edit {
		return Edit{Set: id, Cells: map[string]any{column: content}}
	})
	if err != nil {
		return nil, "", err
	}
	return ids[len(ids)-1:], hash, nil
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
