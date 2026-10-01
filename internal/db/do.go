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
)

type addedPhoto struct {
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
		ids, err := Write(r.Context(), s, queue, actor, env, Batch{Batch: []write{{Insert: "PERSON_PHOTO", Row: row}}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a person's photo", "viewer", env.Viewer, "system", env.System, "person", person, "photo", name)
		serve.Write(w, r, http.StatusOK, addedPhoto{Result: ids, Hash: strings.TrimSuffix(name, "."+ext)})
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
