package directory

import (
	"crypto/sha256"
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
	queue *Queue
}

func RegisterUpload(mux *http.ServeMux, cache *Cache, sheet *data.Sheet, store *blob.Store, queue *Queue) {
	u := uploader{cache: cache, sheet: sheet, store: store, queue: queue}
	mux.HandleFunc("POST /api/directory/upload", u.upload)
	mux.HandleFunc("POST /api/directory/facts", u.facts)
	mux.HandleFunc("POST /api/directory/optout", u.optOut)
	mux.HandleFunc("POST /api/directory/edit", u.edit)
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

func (u uploader) applyOverride(w http.ResponseWriter, actor, email, action string, cells, previous map[string]string) bool {
	logRow := changeLogRow(actor, email, previous)
	applied := make(chan error, 1)
	u.queue.Add(func() {
		applied <- u.cache.applyOverride(email, cells)
		if err := u.sheet.Upsert(appName, "Overrides", "Email", email, cells); err != nil {
			log.Printf("[ERROR] set overrides for %s: %v", email, err)
			return
		}
		if err := u.sheet.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after %s for %s: %v", action, email, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, fmt.Errorf("rebuild model after %s: %w", action, err))
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
	case "primary-photo":
		if !mayEdit(model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		// Only a photo they already have: the column decides which one the directory
		// shows, and a name from anywhere else would fail the next model load.
		chosen := false
		for _, photo := range person.Photos {
			if photo.Name == value {
				chosen = true
			}
		}
		if !chosen {
			http.Error(w, "not one of this person's photos", http.StatusBadRequest)
			return
		}
		cells["Primary Photo"] = value
		previous["Primary Photo"] = person.PrimaryPhoto
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
	old, oldUpdated := "", ""
	if p := model.Person(key); p != nil {
		old, oldUpdated = p.Facts, p.FactsUpdated
	}
	cells := map[string]string{"Facts": facts, "Facts Updated": today()}
	previous := map[string]string{"Facts": old, "Facts Updated": oldUpdated}
	if !u.applyOverride(w, me, key, "facts update", cells, previous) {
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

	// Content-addressed: the same image uploaded twice is one object, and an object is
	// never overwritten, so nobody's photo can be destroyed by somebody else's upload.
	folder := "photos"
	if kind != "photo" {
		folder = "pronunciation"
	}
	name := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)
	if err := u.store.Put(folder, name, mimeType, content); err != nil {
		serverError(w, err)
		return
	}

	// A person's photos are a list, so a new one is a row plus a pointer at it. Every
	// other kind is a single slot, named on the owner's Overrides row.
	if kind == "photo" && target == "person" {
		if !u.addPhoto(w, me, key, name, model.Person(key).PhotoUpdated) {
			return
		}
		log.Printf("upload: %s added photo %s for %s", me, name, key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	row, cells, previous := key, map[string]string{}, map[string]string{}
	switch {
	case kind == "photo":
		row = strings.ToLower(me)
		cells["Family Photo"], cells["Family Photo Updated"] = name, today()
		previous["Family Photo"] = model.Families[key].photo
		previous["Family Photo Updated"] = model.Families[key].PhotoUpdated
	case target == "family":
		row = strings.ToLower(me)
		cells["Family Pronunciation"] = name
		previous["Family Pronunciation"] = model.Families[key].pronunciation
	default:
		cells["Pronunciation"] = name
		previous["Pronunciation"] = model.Person(key).pronunciation
	}
	if !u.applyOverride(w, me, row, kind+" upload", cells, previous) {
		return
	}
	log.Printf("upload: %s set %s %s %s to %s", me, target, key, kind, name)
	w.WriteHeader(http.StatusNoContent)
}

// addPhoto appends the Photos row and makes the new photo primary, since uploading
// one is a statement that it should be the one people see.
func (u uploader) addPhoto(w http.ResponseWriter, me, key, name, previousUpdated string) bool {
	appended := make(chan error, 1)
	u.queue.Add(func() {
		appended <- u.sheet.Append(appName, "Photos", []string{key, name})
	})
	if err := <-appended; err != nil {
		serverError(w, fmt.Errorf("record photo %s for %s: %w", name, key, err))
		return false
	}
	return u.applyOverride(w, me, key, "photo upload",
		map[string]string{"Primary Photo": name, "Photo Updated": today()},
		map[string]string{"Primary Photo": "", "Photo Updated": previousUpdated})
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
