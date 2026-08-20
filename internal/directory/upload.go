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

var changeLogHeader = append([]string{"Timestamp", "Actor"}, overrideColumns...)

func changeLogRow(actor, email string, previous map[string]string) []string {
	row := make([]string, len(changeLogHeader))
	row[0] = time.Now().UTC().Format(time.RFC3339)
	row[1] = actor
	for i, column := range changeLogHeader {
		if column == "Email" {
			row[i] = email
		} else if value, ok := previous[column]; ok {
			if value == "" {
				value = "-"
			}
			row[i] = value
		}
	}
	return row
}

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
	mux.HandleFunc("POST /api/directory/facts", u.facts)
	mux.HandleFunc("POST /api/directory/optout", u.optOut)
	mux.HandleFunc("POST /api/directory/edit", u.edit)
}

func clearable(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func (u uploader) applyOverride(w http.ResponseWriter, actor, email, action string, cells, previous map[string]string) bool {
	for column, cell := range cells {
		if err := u.sheet.Upsert(appName, "Overrides", "Email", email, column, cell); err != nil {
			serverError(w, fmt.Errorf("set %s for %s: %w", column, email, err))
			return false
		}
	}
	logRow := changeLogRow(actor, email, previous)
	if err := u.sheet.Append(appName, changeLogTable, changeLogHeader, logRow); err != nil {
		serverError(w, fmt.Errorf("append change log after %s for %s: %w", action, email, err))
		return false
	}
	if err := u.cache.Refresh(); err != nil {
		serverError(w, fmt.Errorf("refresh model after %s: %w", action, err))
		return false
	}
	return true
}

func (u uploader) edit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	field := r.FormValue("field")
	value := strings.TrimSpace(r.FormValue("value"))
	me := auth.Email(r)
	model := u.cache.Model()
	person := model.Person(key)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}

	cells := map[string]string{}
	previous := map[string]string{}
	switch field {
	case "preferred-name":
		if !mayEdit(model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if value == "" || len(value) > 80 {
			http.Error(w, "bad preferred name", http.StatusBadRequest)
			return
		}
		base := person.LegalName
		if base == "" {
			base = person.FullName
		}
		cells["Preferred Name"] = value
		cells["Full Name"] = value + " " + surname(base)
		previous["Preferred Name"] = person.PreferredName
		previous["Full Name"] = person.FullName
	case "phone":
		if !mayEdit(model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if len(value) > 40 {
			http.Error(w, "bad phone number", http.StatusBadRequest)
			return
		}
		cells["Phone"] = clearable(value)
		previous["Phone"] = person.Phone
	case "address":
		if key != strings.ToLower(me) || !person.IsParent {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		family, ok := model.Families[person.FamilyKey]
		if !ok {
			http.Error(w, "no family record", http.StatusBadRequest)
			return
		}
		if len(value) > 200 {
			http.Error(w, "bad address", http.StatusBadRequest)
			return
		}
		cells["Address"] = clearable(value)
		previous["Address"] = family.Address
	default:
		http.Error(w, "bad field", http.StatusBadRequest)
		return
	}

	if !u.applyOverride(w, me, key, field+" edit", cells, previous) {
		return
	}
	log.Printf("edit: %s set %s on %s", me, field, key)
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) optOut(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	me := auth.Email(r)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	if key == "" {
		http.Error(w, "bad opt out request", http.StatusBadRequest)
		return
	}
	if !mayEdit(u.cache.Model(), me, "person", key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	if !u.applyOverride(w, me, key, "opt out", map[string]string{"Opted Out": "TRUE"}, map[string]string{"Opted Out": ""}) {
		return
	}
	log.Printf("optout: %s removed %s from the directory", me, key)
	w.WriteHeader(http.StatusNoContent)
}

func (u uploader) facts(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	facts := strings.TrimSpace(r.FormValue("facts"))
	if key == "" || len(facts) > 4000 {
		http.Error(w, "bad facts request", http.StatusBadRequest)
		return
	}
	me := auth.Email(r)
	model := u.cache.Model()
	if !mayEdit(model, me, "person", key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	old := ""
	if p := model.Person(key); p != nil {
		old = p.Facts
	}
	if !u.applyOverride(w, me, key, "facts update", map[string]string{"Facts": facts}, map[string]string{"Facts": old}) {
		return
	}
	log.Printf("facts: %s set %s (%d chars)", me, key, len(facts))
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
	if target == "family" {
		folder = "families"
	}
	local, _, _ := strings.Cut(key, "@")
	base := local + "-" + kind
	name := base + "." + ext

	superseded, err := u.store.Upload(folder, base, ext, mimeType, content)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := u.cache.Refresh(); err != nil {
		serverError(w, fmt.Errorf("refresh model after upload: %w", err))
		return
	}
	log.Printf("upload: %s set %s %s %s (superseded generation %q)", me, target, key, name, superseded)
	w.WriteHeader(http.StatusNoContent)
}

func mayEdit(model *Model, me, target, key string) bool {
	mine := model.Person(me)
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
