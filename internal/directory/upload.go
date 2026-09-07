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

	// Every other field below is keyed on a person - key is that person's own
	// email, even for the family-level "address" case (self-service only, so the
	// caller and the family member being written to are always the same person).
	// This one is keyed on a family instead, since it's reachable from
	// super-edit mode editing a family that isn't the caller's own, the same
	// reason the family photo/pronunciation upload path needed familyRow
	// instead of just writing the caller's own row.
	if field == "family-photo-caption" {
		if !u.mayEdit(model, me, "family", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		family, ok := model.Families[key]
		if !ok {
			http.Error(w, "no such family", http.StatusBadRequest)
			return
		}
		if len(value) > 200 {
			http.Error(w, "bad caption", http.StatusBadRequest)
			return
		}
		row := familyRow(family)
		if row == "" {
			http.Error(w, "no such family", http.StatusBadRequest)
			return
		}
		// Unlike Pronouns/Primary Photo, Family Photo Caption is resolved through
		// the same family-fields-split-across-parent-rows merge as Address/Family
		// Phone (buildFamilies' familyCells), which treats "-" as "this row
		// explicitly says no caption" and "" as "this row has no opinion" - so it
		// needs clearable(value)'s "-" convention, not a plain "".
		cells := map[string]string{"Family Photo Caption": clearable(value)}
		previous := map[string]string{"Family Photo Caption": family.PhotoCaption}
		if !u.applyOverride(w, me, row, field+" edit", cells, previous) {
			return
		}
		log.Printf("edit: %s set %s on %s", me, field, key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

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
	case "pronouns":
		if !u.mayEdit(model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if len(value) > 40 {
			http.Error(w, "bad pronouns", http.StatusBadRequest)
			return
		}
		// Unlike Phone/Address, Pronouns has no import baseline (nothing ever sets
		// it outside Overrides), so it's cleared with a plain "" - the same reason
		// Primary Photo needed "" instead of "-" before it was retired - not
		// clearable(value): applyOverrides always starts it at "" and would flag a
		// literal "-" as clearing an already-empty value, failing the whole load.
		cells["Pronouns"] = value
		previous["Pronouns"] = person.Pronouns
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
		order := make([]photoRef, len(person.Photos), len(person.Photos)+1)
		for i, photo := range person.Photos {
			order[i] = photoRef{Name: photo.Name, CropName: photo.cropName}
		}
		order = append(order, photoRef{Name: name})
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
		family := model.Families[key]
		row = familyRow(family)
		cells["Family Photo"], cells["Family Photo Updated"] = name, today()
		previous["Family Photo"] = family.photo
		previous["Family Photo Updated"] = family.PhotoUpdated
	case target == "family":
		family := model.Families[key]
		row = familyRow(family)
		cells["Family Pronunciation"] = name
		previous["Family Pronunciation"] = family.pronunciation
	default:
		cells["Pronunciation"] = name
		previous["Pronunciation"] = model.Person(key).pronunciation
	}
	if row == "" {
		http.Error(w, "no such family", http.StatusBadRequest)
		return
	}
	if !u.applyOverride(w, me, row, kind+" upload", cells, previous) {
		return
	}
	log.Printf("upload: %s set %s %s %s to %s", me, target, key, kind, name)
	w.WriteHeader(http.StatusNoContent)
}

// setPhotos replaces a person's complete photo list and folds the change into the
// running model before responding, so the caller's very next model fetch sees it.
// Uploading (append), drag-reorder (permute), deleting (remove one), and cropping
// (attach a crop to one) all funnel through this one path rather than four ad hoc
// ones, since each is really just "this person's photo list is now exactly order" -
// one place to get the sheet-write-then-rebuild interaction right instead of four.
func (u uploader) setPhotos(w http.ResponseWriter, me, key string, order []photoRef, cells, previous map[string]string, changeAction string) bool {
	rows := make([][]string, len(order))
	for i, ref := range order {
		rows[i] = []string{key, ref.Name, ref.CropName}
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
	names := splitNonEmpty(r.FormValue("order"), ",")
	if !isPhotoSubset(names, person.Photos) {
		http.Error(w, "order must name only this person's current photos, with no duplicates", http.StatusBadRequest)
		return
	}
	// A reorder or delete must carry each photo's existing crop link forward - the
	// client only ever sends names, never crops, so those are looked up here rather
	// than trusted from the request.
	cropOf := map[string]string{}
	for _, photo := range person.Photos {
		cropOf[photo.Name] = photo.cropName
	}
	order := make([]photoRef, len(names))
	for i, name := range names {
		order[i] = photoRef{Name: name, CropName: cropOf[name]}
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

// cropPhoto attaches a square crop to one of a person's existing photos - name
// identifies which one, and the uploaded file becomes its crop, replacing any
// crop it already had. name may instead be empty, meaning "this person has no
// photos of their own and is looking at their family's photo as a stand-in
// (attachBlobs falls back to it) - adopt that family photo as their first
// personal photo, with this crop attached," since there's no existing entry to
// attach a crop to otherwise.
func (u uploader) cropPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 30<<20)
	if err := r.ParseMultipartForm(30 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	name := r.FormValue("name")
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

	var order []photoRef
	if name == "" {
		if len(person.Photos) != 0 {
			http.Error(w, "name is required once this person has photos of their own", http.StatusBadRequest)
			return
		}
		family, ok := model.Families[person.FamilyKey]
		if !ok || family.photo == "" {
			http.Error(w, "no family photo to adopt", http.StatusBadRequest)
			return
		}
		name = family.photo
		order = []photoRef{{Name: name}}
	} else {
		if !isPhotoSubset([]string{name}, person.Photos) {
			http.Error(w, "not one of this person's photos", http.StatusBadRequest)
			return
		}
		order = make([]photoRef, len(person.Photos))
		for i, photo := range person.Photos {
			order[i] = photoRef{Name: photo.Name, CropName: photo.cropName}
		}
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
	mimeType, ext, err := mediaType("photo", content, header.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cropName := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)
	if err := u.store.Put("photos", cropName, mimeType, content); err != nil {
		serverError(w, err)
		return
	}
	for i := range order {
		if order[i].Name == name {
			order[i].CropName = cropName
		}
	}

	cells := map[string]string{"Photo Updated": today()}
	previous := map[string]string{"Photo Updated": person.PhotoUpdated}
	if !u.setPhotos(w, me, key, order, cells, previous, "photo crop") {
		return
	}
	log.Printf("crop-photo: %s set a crop on %s's photo %s", me, key, name)
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

// familyRow returns an email belonging to family whose Overrides row can carry
// a family-level cell (Family Photo, Family Pronunciation) - family data lives
// on a member's own row (see buildFamilies' merge across parents), so writing
// it always has to name an actual member of the family being edited. In
// self-service mode the acting user already is one, but in super-edit mode an
// admin editing someone else's family isn't - using the admin's own email there
// would silently attribute the change to the admin's own family instead.
// Prefers an adult; falls back to a kid for a family with none (empty only for
// a family key that doesn't actually exist).
func familyRow(family Family) string {
	if len(family.AdultEmails) > 0 {
		return strings.ToLower(family.AdultEmails[0])
	}
	if len(family.KidEmails) > 0 {
		return strings.ToLower(family.KidEmails[0])
	}
	return ""
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
