package who

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/store"
)

const maxPhotos = 5

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
	store *blob.Store
}

func RegisterUpload(mux *http.ServeMux, cache *Cache, store *blob.Store) {
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

func setOverride(email string, cells store.Row) store.Op {
	return store.Set(overridesTab, store.Row{"Email": email}, cells)
}

func setFamily(key string, cells store.Row) store.Op {
	return store.Set(familiesTab, store.Row{"Email": key}, cells)
}

func refsOf(person *Person) []photoRef {
	refs := make([]photoRef, len(person.Photos))
	for i, photo := range person.Photos {
		refs[i] = photoRef{Name: photo.Name, CropName: photo.cropName, order: photo.order, stored: photo.stored}
	}
	return refs
}

func photoOps(email string, before []photoRef, after []photoRef) []store.Op {
	keys := make([]string, len(after))
	for i, ref := range after {
		keys[i] = ref.order
	}
	keys = store.Order(keys)
	ops := []store.Op{}
	kept := map[string]bool{}
	for i, ref := range after {
		kept[ref.Name] = true
		if ref.stored {
			ops = append(ops, store.Update(photosTab, store.Row{"Email": email, "Photo Name": ref.Name}, store.Row{store.OrderColumn: keys[i], "Crop Name": ref.CropName}))
			continue
		}
		ops = append(ops, store.Insert(photosTab, store.Row{"Email": email, "Photo Name": ref.Name, store.OrderColumn: keys[i], "Crop Name": ref.CropName}))
	}
	for _, ref := range before {
		if ref.stored && !kept[ref.Name] {
			ops = append(ops, store.Delete(photosTab, store.Row{"Email": email, "Photo Name": ref.Name}))
		}
	}
	return ops
}

func (u uploader) edit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	key := strings.ToLower(strings.TrimSpace(r.FormValue("key")))
	field := r.FormValue("field")
	value := strings.TrimSpace(r.FormValue("value"))
	me := effectiveEmail(u.cache, r)
	model := u.cache.Model()

	if field == "family-photo-caption" {
		if !u.mayEdit(r, model, me, "family", key) {
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
		if !u.cache.commit(w, r, me, setFamily(family.email, store.Row{"Family Photo Caption": clearable(value)})) {
			return
		}
		slog.InfoContext(r.Context(), "edit: set field", "actor", me, "field", field, "key", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if field == "family-pronunciation" {
		if !u.mayEdit(r, model, me, "family", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if value != "" {
			http.Error(w, "pronunciation can only be cleared through this field", http.StatusBadRequest)
			return
		}
		family, ok := model.Families[key]
		if !ok {
			http.Error(w, "no such family", http.StatusBadRequest)
			return
		}
		if !u.cache.commit(w, r, me, setFamily(family.email, store.Row{"Family Pronunciation": ""})) {
			return
		}
		slog.InfoContext(r.Context(), "edit: set field", "actor", me, "field", field, "key", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	person := model.Person(key)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}

	cells := store.Row{}
	switch field {
	case "preferred-name":
		if !u.mayEdit(r, model, me, "person", key) {
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
	case "pronouns":
		if !u.mayEdit(r, model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if len(value) > 40 {
			http.Error(w, "bad pronouns", http.StatusBadRequest)
			return
		}
		cells["Pronouns"] = strings.ToLower(value)
	case "pronunciation":
		if !u.mayEdit(r, model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if value != "" {
			http.Error(w, "pronunciation can only be cleared through this field", http.StatusBadRequest)
			return
		}
		cells["Pronunciation"] = ""
	default:
		http.Error(w, "bad field", http.StatusBadRequest)
		return
	}

	if !u.cache.commit(w, r, me, setOverride(key, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "edit: set field", "actor", me, "field", field, "key", key)
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
	if !u.mayEdit(r, u.cache.Model(), me, "person", key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	if !u.cache.commit(w, r, me, setOverride(key, store.Row{"Facts": facts, "Facts Updated": today()})) {
		return
	}
	slog.InfoContext(r.Context(), "facts: set", "actor", me, "key", key, "chars", len(facts))
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
	if !u.mayEdit(r, model, me, target, key) {
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

	folder := "photos"
	if kind != "photo" {
		folder = "pronunciation"
	}
	name := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)

	if kind == "photo" && target == "person" {
		person := model.Person(key)
		if len(person.Photos) >= maxPhotos {
			http.Error(w, fmt.Sprintf("already has the maximum of %d photos", maxPhotos), http.StatusBadRequest)
			return
		}
		if isPhotoSubset([]string{name}, person.Photos) {
			http.Error(w, "already has this photo", http.StatusBadRequest)
			return
		}
		if err := u.store.Put(folder, name, mimeType, content); err != nil {
			serverError(w, r, err)
			return
		}
		before := refsOf(person)
		after := append(slices.Clone(before), photoRef{Name: name})
		ops := append(photoOps(key, before, after), setOverride(key, store.Row{"Photo Updated": today()}))
		if !u.cache.commit(w, r, me, ops...) {
			return
		}
		slog.InfoContext(r.Context(), "upload: added photo", "actor", me, "name", name, "key", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if target == "family" {
		family, ok := model.Families[key]
		if !ok {
			http.Error(w, "no such family", http.StatusBadRequest)
			return
		}
		if err := u.store.Put(folder, name, mimeType, content); err != nil {
			serverError(w, r, err)
			return
		}
		cells := store.Row{"Family Pronunciation": name}
		if kind == "photo" {
			cells = store.Row{"Family Photo": name, "Family Photo Updated": today()}
		}
		if !u.cache.commit(w, r, me, setFamily(family.email, cells)) {
			return
		}
		slog.InfoContext(r.Context(), "upload: set media", "actor", me, "target", target, "key", key, "kind", kind, "name", name)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if model.Person(key) == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if err := u.store.Put(folder, name, mimeType, content); err != nil {
		serverError(w, r, err)
		return
	}
	if !u.cache.commit(w, r, me, setOverride(key, store.Row{"Pronunciation": name})) {
		return
	}
	slog.InfoContext(r.Context(), "upload: set media", "actor", me, "target", target, "key", key, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

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
	if !u.mayEdit(r, model, me, "person", key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	names := splitNonEmpty(r.FormValue("order"), ",")
	if !isPhotoSubset(names, person.Photos) {
		http.Error(w, "order must name only this person's current photos, with no duplicates", http.StatusBadRequest)
		return
	}
	before := refsOf(person)
	after := make([]photoRef, len(names))
	for i, name := range names {
		after[i] = before[slices.IndexFunc(before, func(ref photoRef) bool { return ref.Name == name })]
	}
	ops := photoOps(key, before, after)
	if person.primaryPhotoOverride != "" {
		ops = append(ops, setOverride(key, store.Row{"Primary Photo": ""}))
	}
	if !u.cache.commit(w, r, me, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "reorder-photos: set photo list", "actor", me, "photos", len(after), "key", key)
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
	me := effectiveEmail(u.cache, r)
	model := u.cache.Model()

	var person *Person
	var family Family
	switch target {
	case "person":
		person = model.Person(key)
		if person == nil {
			http.Error(w, "no such person", http.StatusBadRequest)
			return
		}
	case "family":
		var ok bool
		family, ok = model.Families[key]
		if !ok {
			http.Error(w, "no such family", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "bad crop request", http.StatusBadRequest)
		return
	}
	if !u.mayEdit(r, model, me, target, key) {
		http.Error(w, "not allowed to edit this record", http.StatusForbidden)
		return
	}
	if target == "person" && !isPhotoSubset([]string{name}, person.Photos) {
		http.Error(w, "not one of this person's photos", http.StatusBadRequest)
		return
	}
	if target == "family" && family.photo == "" {
		http.Error(w, "family has no photo to crop", http.StatusBadRequest)
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
	mimeType, ext, err := mediaType("photo", content, header.Header.Get("Content-Type"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cropName := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)
	if err := u.store.Put("photos", cropName, mimeType, content); err != nil {
		serverError(w, r, err)
		return
	}

	if target == "family" {
		if !u.cache.commit(w, r, me, setFamily(family.email, store.Row{"Family Photo Crop": cropName, "Family Photo Updated": today()})) {
			return
		}
		slog.InfoContext(r.Context(), "crop-photo: set a crop on the family photo", "actor", me, "family", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	before := refsOf(person)
	after := slices.Clone(before)
	for i := range after {
		if after[i].Name == name {
			after[i].CropName = cropName
		}
	}
	ops := append(photoOps(key, before, after), setOverride(key, store.Row{"Photo Updated": today()}))
	if !u.cache.commit(w, r, me, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "crop-photo: set a crop on a photo", "actor", me, "key", key, "photo", name)
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

func (u uploader) mayEdit(r *http.Request, model *Model, me, target, key string) bool {
	return superEdit(r, viewerOf(u.cache, me)) || mayEdit(model, me, target, key)
}

func mayEdit(model *Model, me, target, key string) bool {
	if target == "person" && key == me && model.Member(me) {
		return true
	}
	for _, familyKey := range model.FamilyKeysOf(me) {
		family := model.Families[familyKey]
		if !slices.Contains(family.AdultEmails, me) {
			continue
		}
		if target == "family" {
			if familyKey == key {
				return true
			}
		} else if slices.Contains(family.KidEmails, key) || slices.Contains(family.AdultEmails, key) {
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
