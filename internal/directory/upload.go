package directory

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
)

const changeLogTable = "Change Log"

var changeLogHeader = []string{"Timestamp", "Actor", "Target", "Kind", "File", "Archived"}

var photoExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

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
	cache *Cache
	sheet *data.Sheet
	store *blob.Store
}

func RegisterUpload(mux *http.ServeMux, cache *Cache, sheet *data.Sheet, store *blob.Store) {
	u := uploader{cache: cache, sheet: sheet, store: store}
	mux.HandleFunc("POST /api/directory/upload", u.upload)
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
	if (target != "person" && target != "family") || (kind != "photo" && kind != "pronunciation") || key == "" {
		http.Error(w, "bad upload request", http.StatusBadRequest)
		return
	}

	me := auth.Email(r)
	model := u.cache.Model()
	if !mayEdit(model, me, target, key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}

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
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	folder := "people"
	keyColumn := "Email Lower"
	column := map[string]string{"photo": "Primary Photo", "pronunciation": "Pronunciation"}[kind]
	if target == "family" {
		folder = "families"
		keyColumn = "Family Key"
		column = map[string]string{"photo": "Family Photo", "pronunciation": "Family Pronunciation"}[kind]
	}
	local, _, _ := strings.Cut(key, "@")
	base := local + "-" + kind
	name := base + "." + ext

	archived, err := u.store.Upload(folder, base, ext, mimeType, content)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := u.sheet.SetColumn(appName, "Basic Directory", keyColumn, key, column, name); err != nil {
		serverError(w, fmt.Errorf("update sheet after upload of %s/%s: %w", folder, name, err))
		return
	}
	logRow := []string{time.Now().UTC().Format(time.RFC3339), me, key, target + " " + kind, name, archived}
	if err := u.sheet.Append(appName, changeLogTable, changeLogHeader, logRow); err != nil {
		serverError(w, fmt.Errorf("append change log after upload of %s/%s: %w", folder, name, err))
		return
	}
	if err := u.cache.Refresh(); err != nil {
		serverError(w, fmt.Errorf("refresh model after upload: %w", err))
		return
	}
	log.Printf("upload: %s set %s %s %s (archived %q)", me, target, key, name, archived)
	w.WriteHeader(http.StatusNoContent)
}

func mayEdit(model *Model, me, target, key string) bool {
	var mine *Person
	for i := range model.People {
		if model.People[i].Email == me {
			mine = &model.People[i]
			break
		}
	}
	if mine == nil {
		return false
	}
	if target == "family" {
		return mine.FamilyKey != "" && mine.FamilyKey == key
	}
	if key == me {
		return true
	}
	family, ok := model.Families[mine.FamilyKey]
	if !ok {
		return false
	}
	for _, kid := range family.KidEmails {
		if kid == key {
			return true
		}
	}
	return false
}

func mediaType(kind string, content []byte, declared string) (string, string, error) {
	if kind == "photo" {
		sniffed := http.DetectContentType(content)
		ext, ok := photoExtensions[sniffed]
		if !ok {
			return "", "", fmt.Errorf("unsupported photo type %s", sniffed)
		}
		return sniffed, ext, nil
	}
	base, _, _ := strings.Cut(declared, ";")
	base = strings.TrimSpace(strings.ToLower(base))
	ext, ok := audioExtensions[base]
	if !ok {
		return "", "", fmt.Errorf("unsupported audio type %s", declared)
	}
	if strings.HasPrefix(base, "video/") {
		base = "audio/" + strings.TrimPrefix(base, "video/")
	}
	return base, ext, nil
}
