package db

import (
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

func registerDo(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, importKey []byte, now func() time.Time) {
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
		row := store.Row{"person": person, "group": group, "photo": pictureFolder + "/" + img.name}
		maps.Copy(row, box)
		if err := m.Authorize(env, Change{Table: "PHOTO", New: row}); err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := pics.bucket.Put(r.Context(), row["photo"], img.mimeType, img.content); err != nil {
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
		slog.InfoContext(r.Context(), "added a photo", "viewer", env.Viewer, "system", env.System, "person", person, "group", group, "photo", row["photo"])
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: strings.TrimSuffix(img.name, "."+img.ext)})
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
		if err := pics.bucket.Put(r.Context(), "calendar/"+name, "application/pdf", content); err != nil {
			serve.Error(w, r, err)
			return
		}
		ids, err := Write(r.Context(), s, queue, pics, actor, env, Batch{Batch: []Edit{{Insert: "DOCUMENT", Row: row}}})
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "added a version of the year calendar", "viewer", env.Viewer, "system", env.System, "document", ids[0], "hash", hash)
		serve.Write(w, r, http.StatusOK, stored{Result: ids, Hash: hash})
	})
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
