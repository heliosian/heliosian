package db

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/trace"
)

const blobPath = "/api/blob/"

func (m *Model) BlobCell(span *trace.Span, env Env, id, column string) (name, mimeType string, ok bool) {
	defer span.End()
	tableName, ok := TableOf(id)
	if !ok {
		return "", "", false
	}
	t, _ := Lookup(tableName)
	c, ok := t.Column(column)
	if !ok || t.Generated || !(c.Kind == Blob || c.Kind == Ref && c.Target == "CONTENT") {
		return "", "", false
	}
	r := m.newRun(env)
	r.policy = span.Tally("policy")
	row, ok := r.table(t.Name).Get(id)
	if !ok || !r.readable(t, row) {
		return "", "", false
	}
	if c.Kind == Ref {
		content := r.guardedCell(t, row, c)
		t, _ = Lookup("CONTENT")
		c, _ = t.Column("blob")
		if row, ok = r.table(t.Name).Get(content); !ok || !r.readable(t, row) {
			return "", "", false
		}
	}
	mime, _ := t.Column("mime")
	name, mimeType = r.guardedCell(t, row, c), r.guardedCell(t, row, mime)
	return name, mimeType, name != ""
}

func registerBlobs(mux *http.ServeMux, s *Store, pics *Pictures, tokens auth.Tokens) {
	mux.HandleFunc("GET "+blobPath+"{id}/{column}", func(w http.ResponseWriter, r *http.Request) {
		stream := r.Header.Get("Accept") == "text/event-stream"
		root := trace.New("request")
		defer func() {
			root.End()
			if !stream && root.Dur > 250 {
				slog.WarnContext(r.Context(), "slow blob", "path", r.URL.Path, "ms", root.Dur, "trace", root.Header())
			}
		}()
		waiting := root.Start("model")
		m := s.Model()
		waiting.End()
		signing := root.Start("caller")
		env, _, ok := caller(w, r, m, tokens)
		signing.End()
		if !ok {
			return
		}
		w.Header().Add("Vary", "Accept")
		if stream {
			id := r.PathValue("id")
			table, _ := TableOf(id)
			read := m.newRun(env)
			photo, exists := read.table("PHOTO").Get(id)
			t, _ := Lookup("PHOTO")
			if table != "PHOTO" || r.PathValue("column") != "image" || !exists || !read.readable(t, photo) {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			http.NewResponseController(w).Flush()
			err := pics.waitPhoto(r.Context(), env, id)
			if r.Context().Err() != nil {
				return
			}
			message := map[string]string{}
			if err != nil {
				message["error"] = err.Error()
			}
			w.Write([]byte("data: "))
			json.NewEncoder(w).Encode(message)
			w.Write([]byte("\n"))
			return
		}
		name, mimeType, ok := m.BlobCell(root.Start("cell"), env, r.PathValue("id"), r.PathValue("column"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		etag := strconv.Quote(name)
		cache := "private, no-cache"
		if table, _ := TableOf(r.PathValue("id")); table == "CONTENT" {
			cache = "private, max-age=31536000, immutable"
		}
		w.Header().Set("Cache-Control", cache)
		w.Header().Set("ETag", etag)
		w.Header().Add("Content-Security-Policy", "sandbox")
		if r.Header.Get("If-None-Match") == etag {
			root.End()
			w.Header().Set("Trace", root.Header())
			w.WriteHeader(http.StatusNotModified)
			return
		}
		fetching := root.Start("bucket")
		content, _, err := pics.bucket.Get(r.Context(), name)
		fetching.End()
		if errors.Is(err, blob.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		root.End()
		w.Header().Set("Trace", root.Header())
		w.Header().Set("Content-Type", mimeType)
		w.Write(content)
	})
}
