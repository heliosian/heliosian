package db

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/serve"
)

const blobPath = "/api/blob/"

func (m *Model) blobCell(env Env, id, column string) (string, bool) {
	tableName, ok := TableOf(id)
	if !ok {
		return "", false
	}
	t, _ := Lookup(tableName)
	c, ok := t.Column(column)
	if !ok || c.Kind != Blob || t.Generated {
		return "", false
	}
	r := m.newRun(env)
	row, ok := r.table(t.Name).Get(id)
	if !ok || !r.readable(t, row) {
		return "", false
	}
	name := r.guardedCell(t, row, c)
	return name, name != ""
}

func registerBlobs(mux *http.ServeMux, s *Store, pics *Pictures, importKey []byte, now func() time.Time) {
	mux.HandleFunc("GET "+blobPath+"{id}/{column}", func(w http.ResponseWriter, r *http.Request) {
		m := s.Model()
		env, _, ok := caller(w, r, m, importKey, now())
		if !ok {
			return
		}
		name, ok := m.blobCell(env, r.PathValue("id"), r.PathValue("column"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		etag := strconv.Quote(name)
		w.Header().Set("Cache-Control", "private, no-cache")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		content, mimeType, err := pics.bucket.Get(r.Context(), name)
		if errors.Is(err, blob.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		w.Header().Set("Content-Type", mimeType)
		w.Write(content)
	})
}
