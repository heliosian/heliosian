package db

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
	if !ok || c.Kind != Blob || t.Generated {
		return "", "", false
	}
	r := m.newRun(env)
	r.policy = span.Tally("policy")
	row, ok := r.table(t.Name).Get(id)
	if !ok || !r.readable(t, row) {
		return "", "", false
	}
	name = r.guardedCell(t, row, c)
	if mime, ok := t.Column("mime"); ok {
		mimeType = r.guardedCell(t, row, mime)
	}
	return name, mimeType, name != ""
}

func registerBlobs(mux *http.ServeMux, s *Store, pics *Pictures, importKey []byte, now func() time.Time) {
	mux.HandleFunc("GET "+blobPath+"{id}/{column}", func(w http.ResponseWriter, r *http.Request) {
		root := trace.New("request")
		defer func() {
			root.End()
			if root.Dur > 250 {
				slog.WarnContext(r.Context(), "slow blob", "path", r.URL.Path, "ms", root.Dur, "trace", root.Header())
			}
		}()
		waiting := root.Start("model")
		m := s.Model()
		waiting.End()
		signing := root.Start("caller")
		env, _, ok := caller(w, r, m, importKey, now())
		signing.End()
		if !ok {
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
		content, stored, err := pics.bucket.Get(r.Context(), name)
		fetching.End()
		if errors.Is(err, blob.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		if mimeType == "" {
			mimeType = stored
		}
		root.End()
		w.Header().Set("Trace", root.Header())
		w.Header().Set("Content-Type", mimeType)
		w.Write(content)
	})
}
