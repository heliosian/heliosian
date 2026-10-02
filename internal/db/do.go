package db

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	doPrefix   = "/api/do/"
	photoLimit = 30 << 20
	pdfLimit   = 30 << 20
)

type stored struct {
	Result []string `json:"result"`
	Hash   string   `json:"hash"`
}

func registerDo(mux *http.ServeMux, s *Store, queue *store.Queue, media *blob.Store, importKey []byte, now func() time.Time) {
	mux.HandleFunc("POST "+doPrefix+"person-photo", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, photoLimit)
		if err := r.ParseMultipartForm(photoLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the photo as multipart form data: %v", err))
			return
		}
		person := r.FormValue("person")
		file, _, err := r.FormFile("photo")
		if err != nil {
			serve.Error(w, r, access.Invalid("the photo is required"))
			return
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the photo"))
			return
		}
		mimeType := http.DetectContentType(content)
		ext, ok := blob.ImageExtensions[mimeType]
		if !ok {
			serve.Error(w, r, access.Invalid("%s is not a supported photo", mimeType))
			return
		}
		thumb, err := blob.Thumbnail(content)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the photo: %v", err))
			return
		}
		name := blob.Name(content, ext)
		thumbName := blob.Name(thumb, "jpg")
		m := s.Model()
		row := map[string]any{"person": person, "photo": name, "thumbnail": thumbName, "order": m.firstOrder(person)}
		if err := m.Authorize(env, Change{Table: "PERSON_PHOTO", New: store.Row{"person": person, "photo": name, "thumbnail": thumbName}}); err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := media.Put("photos", name, mimeType, content); err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := media.Put("photos", thumbName, "image/jpeg", thumb); err != nil {
			serve.Error(w, r, err)
			return
		}
		ids, err := Write(r.Context(), s, queue, actor, env, Batch{Batch: []Edit{{Insert: "PERSON_PHOTO", Row: row}}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a person's photo", "viewer", env.Viewer, "system", env.System, "person", person, "photo", name)
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: strings.TrimSuffix(name, "."+ext)})
	})
	mux.HandleFunc("POST "+doPrefix+"calendar-pdf", func(w http.ResponseWriter, r *http.Request) {
		env, actor, ok := caller(w, r, s.Model(), importKey, now())
		if !ok {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, pdfLimit)
		if err := r.ParseMultipartForm(pdfLimit); err != nil {
			serve.Error(w, r, access.Invalid("send the pdf as multipart form data: %v", err))
			return
		}
		file, _, err := r.FormFile("pdf")
		if err != nil {
			serve.Error(w, r, access.Invalid("the pdf is required"))
			return
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			serve.Error(w, r, access.Invalid("could not read the pdf"))
			return
		}
		if mimeType := http.DetectContentType(content); mimeType != "application/pdf" {
			serve.Error(w, r, access.Invalid("%s is not a pdf", mimeType))
			return
		}
		name := blob.Name(content, "pdf")
		hash := strings.TrimSuffix(name, ".pdf")
		m := s.Model()
		for _, row := range m.Table("DOCUMENT").All() {
			if row["kind"] == "calendar" && row["hash"] == hash {
				serve.Write(w, r, http.StatusOK, stored{Result: []string{row["id"]}, Hash: hash})
				return
			}
		}
		row := map[string]any{"kind": "calendar", "object": "calendar/" + name, "hash": hash, "url": r.FormValue("url"), "date": now().In(School).Format(DateLayout)}
		if err := m.Authorize(env, Change{Table: "DOCUMENT", New: store.Row{"kind": "calendar", "object": "calendar/" + name, "hash": hash}}); err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := media.Put("calendar", name, "application/pdf", content); err != nil {
			serve.Error(w, r, err)
			return
		}
		ids, err := Write(r.Context(), s, queue, actor, env, Batch{Batch: []Edit{{Insert: "DOCUMENT", Row: row}}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a version of the year calendar", "viewer", env.Viewer, "system", env.System, "document", ids[0], "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: hash})
	})
}

func (m *Model) firstOrder(person string) string {
	keys := []string{""}
	for _, row := range m.Table("PERSON_PHOTO").Referencing("person", person) {
		if row["order"] != "" {
			keys = append(keys, row["order"])
		}
	}
	slices.SortFunc(keys[1:], store.CompareKeys)
	return store.Order(keys)[0]
}
