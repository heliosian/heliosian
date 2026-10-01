package model

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
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

type storedMedia struct {
	Name string `json:"name"`
}

func RegisterDirectoryMedia(mux *http.ServeMux, media *blob.Store) {
	mux.HandleFunc("POST /api/directory/media", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
		if err := r.ParseMultipartForm(30 << 20); err != nil {
			http.Error(w, "upload too large or malformed", http.StatusBadRequest)
			return
		}
		kind := r.FormValue("kind")
		if kind != "photo" && kind != "pronunciation" {
			serve.Error(w, r, access.Invalid("bad kind: must be photo or pronunciation"))
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
			serve.Error(w, r, err)
			return
		}
		folder := "photos"
		if kind != "photo" {
			folder = "pronunciation"
		}
		name := blob.Name(content, ext)
		if err := media.Put(folder, name, mimeType, content); err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "who: stored media", "actor", auth.Email(r), "kind", kind, "name", name)
		serve.Write(w, r, http.StatusOK, storedMedia{Name: name})
	})
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
