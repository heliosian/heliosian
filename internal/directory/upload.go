package directory

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/data"
)

const changeLogTable = "Change Log"

// maxPhotos caps a person's photo gallery, the Veracross school portrait counting
// as one of the slots like any other photo.
const maxPhotos = 5

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
	mux.HandleFunc("POST /api/directory/reorder-photos", u.reorderPhotos)
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
		// If the in-memory rebuild rejects this change, don't write it to the real
		// sheet either - otherwise the sheet ends up holding a value the model can
		// never load, and every future rebuild (including the next server start)
		// fails the same way until someone finds and fixes the cell by hand.
		err := u.cache.applyOverride(email, cells)
		applied <- err
		if err != nil {
			return
		}
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
	me := effectiveEmail(u.cache, r)
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
		if !u.mayEdit(model, me, "person", key) {
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
		if !u.mayEdit(model, me, "person", key) {
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
	me := effectiveEmail(u.cache, r)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	if key == "" {
		http.Error(w, "bad opt out request", http.StatusBadRequest)
		return
	}
	if !u.mayEdit(u.cache.Model(), me, "person", key) {
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
	me := effectiveEmail(u.cache, r)
	model := u.cache.Model()
	if !u.mayEdit(model, me, "person", key) {
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

	me := effectiveEmail(u.cache, r)
	model := u.cache.Model()
	if !u.mayEdit(model, me, target, key) {
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

	// A person's photos are a list; every other kind is a single slot, named on the
	// owner's Overrides row.
	if kind == "photo" && target == "person" {
		person := model.Person(key)
		if len(person.Photos) >= maxPhotos {
			http.Error(w, fmt.Sprintf("already has the maximum of %d photos", maxPhotos), http.StatusBadRequest)
			return
		}
		order := make([]string, len(person.Photos), len(person.Photos)+1)
		for i, photo := range person.Photos {
			order[i] = photo.Name
		}
		order = append(order, name)
		cells := map[string]string{"Photo Updated": today()}
		previous := map[string]string{"Photo Updated": person.PhotoUpdated}
		if !u.setPhotos(w, me, key, order, cells, previous, "photo upload") {
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

// setPhotos replaces a person's complete photo list and folds the change into the
// running model before responding, so the caller's very next model fetch sees it.
// Uploading (append), drag-reorder (permute), and deleting (remove one) all funnel
// through this one path rather than three ad hoc ones, since each is really just
// "this person's photo list is now exactly order" - one place to get the
// sheet-write-then-rebuild interaction right instead of three.
func (u uploader) setPhotos(w http.ResponseWriter, me, key string, order []string, cells, previous map[string]string, changeAction string) bool {
	rows := make([][]string, len(order))
	for i, name := range order {
		rows[i] = []string{key, name}
	}
	rewritten := make(chan error, 1)
	u.queue.Add(func() {
		if err := u.sheet.Delete(appName, "Photos", map[string]string{"Email": key}); err != nil {
			rewritten <- err
			return
		}
		rewritten <- u.sheet.AppendAll(appName, "Photos", rows)
	})
	if err := <-rewritten; err != nil {
		serverError(w, fmt.Errorf("rewrite photo list for %s: %w", key, err))
		return false
	}
	logRow := changeLogRow(me, key, previous)
	applied := make(chan error, 1)
	u.queue.Add(func() {
		// If the in-memory rebuild rejects this change, don't write it to the real
		// sheet either - otherwise the sheet ends up holding a value the model can
		// never load, and every future rebuild (including the next server start)
		// fails the same way until someone finds and fixes the cell by hand.
		err := u.cache.applyPhotos(key, order, cells)
		applied <- err
		if err != nil || len(cells) == 0 {
			return
		}
		if err := u.sheet.Upsert(appName, "Overrides", "Email", key, cells); err != nil {
			log.Printf("[ERROR] set overrides for %s: %v", key, err)
			return
		}
		if err := u.sheet.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after %s for %s: %v", changeAction, key, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, fmt.Errorf("rebuild model after %s: %w", changeAction, err))
		return false
	}
	return true
}

// reorderPhotos handles both dragging photos into a new order and deleting one: a
// delete is just "the same list minus one name," so both are the same request.
func (u uploader) reorderPhotos(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	me := effectiveEmail(u.cache, r)
	model := u.cache.Model()
	person := model.Person(key)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if !u.mayEdit(model, me, "person", key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	order := splitNonEmpty(r.FormValue("order"), ",")
	if !isPhotoSubset(order, person.Photos) {
		http.Error(w, "order must name only this person's current photos, with no duplicates", http.StatusBadRequest)
		return
	}
	cells, previous := map[string]string{}, map[string]string{}
	if person.primaryPhotoOverride != "" {
		// Retire the legacy pointer now that order alone decides primary - otherwise
		// it would resurface and override this reorder on the next model load. An
		// empty string, not "-": unlike Phone/Address, Primary Photo has no import
		// baseline to distinguish "no override" from "overridden to blank", so
		// applyOverrides always starts it at "" and a literal "-" here would just be
		// flagged as clearing an already-empty value.
		cells["Primary Photo"] = ""
		previous["Primary Photo"] = person.primaryPhotoOverride
	}
	if !u.setPhotos(w, me, key, order, cells, previous, "photo reorder") {
		return
	}
	log.Printf("reorder-photos: %s set %d photos for %s", me, len(order), key)
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

// isPhotoSubset reports whether order names only photos this person actually has,
// each at most once - a full permutation for a reorder, one name short for a
// delete. Anything else (an unknown name, a duplicate) is rejected outright rather
// than silently ignored, since order is about to become the sheet's source of truth.
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

func (u uploader) mayEdit(model *Model, me, target, key string) bool {
	admin := strings.ToLower(strings.TrimSpace(me))
	if u.cache.IsAdmin(admin) && u.cache.SuperEditEnabled(admin) {
		return true
	}
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
	for _, adult := range family.AdultEmails {
		if adult == key {
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
