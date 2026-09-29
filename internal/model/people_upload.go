package model

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/serve"
)

const maxPhotos = 5

var audioExtensions = map[string]string{
	"audio/webm":  "webm",
	"video/webm":  "webm",
	"audio/mp4":   "m4a",
	"video/mp4":   "m4a",
	"audio/x-m4a": "m4a",
	"audio/mpeg":  "mp3",
	"audio/ogg":   "ogg",
	"audio/wav":   "wav",
}

type uploader struct {
	cache *DirectoryCache
	store *blob.Store
}

func RegisterDirectoryUpload(mux *http.ServeMux, cache *DirectoryCache, store *blob.Store) {
	u := uploader{cache: cache, store: store}
	mux.HandleFunc("POST /api/directory/upload", u.upload)
	mux.HandleFunc("POST /api/directory/facts", u.facts)
	mux.HandleFunc("POST /api/directory/edit", u.edit)
	mux.HandleFunc("POST /api/directory/reorder-photos", u.reorderPhotos)
	mux.HandleFunc("POST /api/directory/crop-photo", u.cropPhoto)
}

func today() string {
	return time.Now().UTC().Format(updatedFormat)
}

func clearable(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func refsOf(person *Person) []photoRef {
	refs := []photoRef{}
	for _, photo := range person.Photos {
		refs = append(refs, photoRef{Name: photo.Name, CropName: photo.cropName, order: photo.order, stored: photo.stored})
	}
	return refs
}

func (u uploader) edit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	field := r.FormValue("field")
	value := strings.TrimSpace(r.FormValue("value"))
	actor := requestActor(u.cache, r)
	ops, err := u.cache.Model().editField(actor, field, key, value)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "edit: set field", "actor", actor.Email, "field", field, "key", key)
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) facts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	facts := strings.TrimSpace(r.FormValue("facts"))
	actor := requestActor(u.cache, r)
	ops, err := u.cache.Model().setFacts(actor, key, facts)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "facts: set", "actor", actor.Email, "key", key, "chars", len(facts))
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	if err := r.ParseMultipartForm(30 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	target := r.FormValue("target")
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	kind := r.FormValue("kind")
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		http.Error(w, "unreadable file", http.StatusBadRequest)
		return
	}

	mimeType, ext, err := mediaType(kind, content, header.Header.Get("Content-Type"))
	if err != nil {
		serve.Error(w, r, err)
		return
	}

	folder := "photos"
	if kind != "photo" {
		folder = "pronunciation"
	}
	name := blob.Name(content, ext)
	actor := requestActor(u.cache, r)
	ops, err := u.cache.Model().upload(actor, target, kind, key, name)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.store.Put(folder, name, mimeType, content); err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	if kind == "photo" && target == "person" {
		slog.InfoContext(r.Context(), "upload: added photo", "actor", actor.Email, "name", name, "key", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	slog.InfoContext(r.Context(), "upload: set media", "actor", actor.Email, "target", target, "key", key, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) reorderPhotos(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	names := splitNonEmpty(r.FormValue("order"), ",")
	actor := requestActor(u.cache, r)
	ops, err := u.cache.Model().reorderPhotos(actor, key, names)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "reorder-photos: set photo list", "actor", actor.Email, "photos", len(names), "key", key)
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) cropPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	if err := r.ParseMultipartForm(30 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	target := r.FormValue("target")
	if target == "" {
		target = "person"
	}
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	name := r.FormValue("name")
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		http.Error(w, "unreadable file", http.StatusBadRequest)
		return
	}
	mimeType, ext, err := mediaType("photo", content, header.Header.Get("Content-Type"))
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	cropName := blob.Name(content, ext)
	actor := requestActor(u.cache, r)
	ops, err := u.cache.Model().cropPhoto(actor, target, key, name, cropName)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.store.Put("photos", cropName, mimeType, content); err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := u.cache.commit(r.Context(), actor, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	if target == "family" {
		slog.InfoContext(r.Context(), "crop-photo: set a crop on the family photo", "actor", actor.Email, "family", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	slog.InfoContext(r.Context(), "crop-photo: set a crop on a photo", "actor", actor.Email, "key", key, "photo", name)
	w.WriteHeader(http.StatusNoContent)
}

func splitNonEmpty(s, sep string) []string {
	var out []string
	for _, part := range strings.Split(s, sep) {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func isPhotoSubset(order []string, photos []Photo) bool {
	remaining := map[string]int{}
	for _, photo := range photos {
		remaining[photo.Name]++
	}
	seen := map[string]bool{}
	for _, name := range order {
		if seen[name] || remaining[name] == 0 {
			return false
		}
		seen[name] = true
	}
	return true
}

func mediaType(kind string, content []byte, declared string) (string, string, error) {
	if kind == "photo" {
		sniffed := http.DetectContentType(content)
		ext, ok := blob.ImageExtensions[sniffed]
		if !ok {
			return "", "", access.Invalid("unsupported photo type %s", sniffed)
		}
		return sniffed, ext, nil
	}
	base, _, _ := strings.Cut(declared, ";")
	base = strings.TrimSpace(strings.ToLower(base))
	ext, ok := audioExtensions[base]
	if !ok {
		return "", "", access.Invalid("unsupported audio type %s", declared)
	}
	if strings.HasPrefix(base, "video/") {
		base = "audio/" + strings.TrimPrefix(base, "video/")
	}
	return base, ext, nil
}
