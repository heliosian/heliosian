package who

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/data"
)

const changeLogTable = "Change Log"

// maxPhotos caps a person's photo gallery, the Veracross school portrait counting
// as one of the slots like any other photo.
const maxPhotos = 5

// changeLogHeader is a fixed list rather than derived from overrideColumns: rows are
// appended positionally, so it must match the Change Log tab's actual column order,
// which still interleaves the family columns from when family fields lived on person
// Overrides rows. One log covers both tabs - a family edit's row is keyed by the
// family key, which is itself a parent email.
var changeLogHeader = []string{
	"Timestamp", "Actor",
	"Email", "Added", "Full Name", "Legal Name", "Preferred Name",
	"Is Student", "Is Parent", "Is Staff", "New to Helios", "Pronouns", "Facts",
	"Grade", "Classroom", "Crew", "Phone", "Job Title", "Department", "Grade Band", "Room Parent",
	"Address", "Family Phone", "Family Photo Caption", "Opted Out",
	"Photo Updated", "Facts Updated", "Family Photo Updated",
	"Veracross Photo", "Primary Photo", "Pronunciation",
	"Family Photo", "Family Pronunciation",
}

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

func (u uploader) applyOverride(w http.ResponseWriter, r *http.Request, actor, email, action string, cells, previous map[string]string) bool {
	return applyOverrideWrite(u.cache, u.sheet, u.queue, w, r, actor, email, action, cells, previous)
}

func (u uploader) applyFamily(w http.ResponseWriter, r *http.Request, actor, key, action string, cells, previous map[string]string) bool {
	return applyFamilyWrite(u.cache, u.sheet, u.queue, w, r, actor, key, action, cells, previous)
}

// applyFamilyWrite is applyOverrideWrite for the Families tab, keyed by the family
// key (the alphabetically first parent email, which is the row's Email cell).
func applyFamilyWrite(cache *Cache, writer data.Writer, queue *Queue, w http.ResponseWriter, r *http.Request, actor, key, action string, cells, previous map[string]string) bool {
	logRow := changeLogRow(actor, key, previous)
	applied := make(chan error, 1)
	queue.Add(func() {
		err := cache.applyFamily(key, cells)
		applied <- err
		if err != nil {
			return
		}
		if err := writer.Upsert(appName, "Families", "Email", key, cells); err != nil {
			slog.ErrorContext(r.Context(), "set families row", "key", key, "error", err)
			return
		}
		if err := writer.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after %s for %s: %v", action, key, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, r, fmt.Errorf("rebuild model after %s: %w", action, err))
		return false
	}
	return true
}

// applyOverrideWrite folds an Overrides change into the cache and, once the rebuild
// accepts it, persists the same cells to the real sheet plus a change log row. Shared
// by uploader (self-service field edits, keyed on *data.Sheet) and admin (structural
// field edits, keyed on the narrower data.Writer) so the write path - and its
// reject-before-persist ordering - can't drift between the two.
func applyOverrideWrite(cache *Cache, writer data.Writer, queue *Queue, w http.ResponseWriter, r *http.Request, actor, email, action string, cells, previous map[string]string) bool {
	logRow := changeLogRow(actor, email, previous)
	applied := make(chan error, 1)
	queue.Add(func() {
		// If the in-memory rebuild rejects this change, don't write it to the real
		// sheet either - otherwise the sheet ends up holding a value the model can
		// never load, and every future rebuild (including the next server start)
		// fails the same way until someone finds and fixes the cell by hand.
		err := cache.applyOverride(email, cells)
		applied <- err
		if err != nil {
			return
		}
		if err := writer.Upsert(appName, "Overrides", "Email", email, cells); err != nil {
			slog.ErrorContext(r.Context(), "set overrides", "email", email, "error", err)
			return
		}
		if err := writer.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after %s for %s: %v", action, email, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, r, fmt.Errorf("rebuild model after %s: %w", action, err))
		return false
	}
	return true
}

// applyEmailRenameWrite is applyOverrideWrite for an added-only person's email change
// (admin.go's setAddedFields is the only caller): besides the Overrides row itself, it
// renames every Tags row where they're the owner or the tagged person, and every
// Photos row that belongs to them (see Tables.withEmailRenamed), so the rename doesn't
// silently strand their tags or photos under the old address.
//
// The existence checks before each Tags/Photos Upsert matter: data.Writer's Upsert
// contract inserts a new row when nothing matches its key, which is exactly right for
// "this person has exactly one Overrides row" but wrong here - a person with zero tags
// or photos (the common case) would otherwise get a garbage row invented for them, one
// with only the renamed column set and everything else blank. Checking first against
// the tables snapshot from just before the rename (still keyed under oldEmail) means
// each Upsert only ever fires when a real row is there to rename.
func applyEmailRenameWrite(cache *Cache, writer data.Writer, queue *Queue, w http.ResponseWriter, r *http.Request, actor, oldEmail, newEmail string, cells map[string]string) bool {
	logRow := changeLogRow(actor, newEmail, map[string]string{"Email": oldEmail})
	applied := make(chan error, 1)
	queue.Add(func() {
		before := cache.currentTables()
		hasTagOwner := slices.ContainsFunc(before.Tags, func(row map[string]string) bool {
			return strings.EqualFold(row[tagOwner], oldEmail)
		})
		hasTagPerson := slices.ContainsFunc(before.Tags, func(row map[string]string) bool {
			return strings.EqualFold(row[tagPerson], oldEmail)
		})
		hasPhotos := slices.ContainsFunc(before.Photos, func(row map[string]string) bool {
			return strings.EqualFold(row["Email"], oldEmail)
		})

		err := cache.applyEmailRename(oldEmail, newEmail, cells)
		applied <- err
		if err != nil {
			return
		}
		overrideCells := map[string]string{"Email": newEmail}
		for column, value := range cells {
			overrideCells[column] = value
		}
		if err := writer.Upsert(appName, "Overrides", "Email", oldEmail, overrideCells); err != nil {
			slog.ErrorContext(r.Context(), "rename overrides row", "from", oldEmail, "to", newEmail, "error", err)
			return
		}
		if hasTagOwner {
			if err := writer.Upsert(appName, tagsTable, tagOwner, oldEmail, map[string]string{tagOwner: newEmail}); err != nil {
				slog.ErrorContext(r.Context(), "rename tag owner", "from", oldEmail, "to", newEmail, "error", err)
			}
		}
		if hasTagPerson {
			if err := writer.Upsert(appName, tagsTable, tagPerson, oldEmail, map[string]string{tagPerson: newEmail}); err != nil {
				slog.ErrorContext(r.Context(), "rename tag person", "from", oldEmail, "to", newEmail, "error", err)
			}
		}
		if hasPhotos {
			if err := writer.Upsert(appName, "Photos", "Email", oldEmail, map[string]string{"Email": newEmail}); err != nil {
				slog.ErrorContext(r.Context(), "rename photos", "from", oldEmail, "to", newEmail, "error", err)
			}
		}
		if err := writer.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after email rename %s -> %s: %v", oldEmail, newEmail, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, r, fmt.Errorf("rebuild model after email rename: %w", err))
		return false
	}
	return true
}

// applyDeletePersonWrite is applyOverrideWrite for permanently removing an added-only
// person (admin.go's setAddedFields, the Added Overrides tab's delete, is the only
// caller): besides their Overrides row, it deletes every Tags row where they're the
// owner or the tagged person, and every Photos row that belongs to them (see
// Tables.withoutPerson). Unlike applyEmailRenameWrite's Upsert calls, data.Writer's
// Delete is safe to call even when nothing matches - both implementations just delete
// zero rows and return no error - so this doesn't need the existence checks that
// rename does.
func applyDeletePersonWrite(cache *Cache, writer data.Writer, queue *Queue, w http.ResponseWriter, r *http.Request, actor, email string, previous map[string]string) bool {
	logRow := changeLogRow(actor, email, previous)
	applied := make(chan error, 1)
	queue.Add(func() {
		err := cache.applyDeletePerson(email)
		applied <- err
		if err != nil {
			return
		}
		if err := writer.Delete(appName, "Overrides", map[string]string{"Email": email}); err != nil {
			slog.ErrorContext(r.Context(), "delete overrides row", "email", email, "error", err)
			return
		}
		if err := writer.Delete(appName, tagsTable, map[string]string{tagOwner: email}); err != nil {
			slog.ErrorContext(r.Context(), "delete tags owned", "email", email, "error", err)
		}
		if err := writer.Delete(appName, tagsTable, map[string]string{tagPerson: email}); err != nil {
			slog.ErrorContext(r.Context(), "delete tags naming", "email", email, "error", err)
		}
		if err := writer.Delete(appName, "Photos", map[string]string{"Email": email}); err != nil {
			slog.ErrorContext(r.Context(), "delete photos", "email", email, "error", err)
		}
		if err := writer.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after deleting %s: %v", email, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, r, fmt.Errorf("rebuild model after delete: %w", err))
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
	// super-edit mode editing a family that isn't the caller's own.
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
		// clearable's "-" convention, same as every Families-tab column: applyFamilies
		// reads "-" as an explicit clear and "" as no cell at all.
		cells := map[string]string{"Family Photo Caption": clearable(value)}
		previous := map[string]string{"Family Photo Caption": family.PhotoCaption}
		if !u.applyFamily(w, r, me, family.Key, field+" edit", cells, previous) {
			return
		}
		slog.InfoContext(r.Context(), "edit: set field", "actor", me, "field", field, "key", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// A pronunciation recording can only be set through the upload endpoint (it's a
	// file), but there was no way to clear one once set. This is that: the value is
	// always empty, since it only ever deletes. Family Pronunciation, like Family
	// Photo Caption above, is keyed on a family rather than the caller's own email,
	// for the same super-edit reason.
	if field == "family-pronunciation" {
		if !u.mayEdit(model, me, "family", key) {
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
		cells := map[string]string{"Family Pronunciation": ""}
		previous := map[string]string{"Family Pronunciation": family.pronunciation}
		if !u.applyFamily(w, r, me, family.Key, field+" edit", cells, previous) {
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
		// Lowercased so the Pronouns filter (case-sensitive on stored values) stays
		// one option per pronoun set instead of splitting on casing.
		cells["Pronouns"] = strings.ToLower(value)
		previous["Pronouns"] = person.Pronouns
	case "pronunciation":
		if !u.mayEdit(model, me, "person", key) {
			http.Error(w, "not allowed to edit this record", http.StatusForbidden)
			return
		}
		if value != "" {
			http.Error(w, "pronunciation can only be cleared through this field", http.StatusBadRequest)
			return
		}
		// Same no-baseline "" convention as Pronouns above, not clearable()'s "-" -
		// Pronunciation is only ever set by the upload endpoint, so there's nothing
		// for apply() to see as a pre-existing baseline value.
		cells["Pronunciation"] = ""
		previous["Pronunciation"] = person.pronunciation
	default:
		http.Error(w, "bad field", http.StatusBadRequest)
		return
	}

	if !u.applyOverride(w, r, me, key, field+" edit", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "edit: set field", "actor", me, "field", field, "key", key)
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
	if !u.applyOverride(w, r, me, key, "opt out", map[string]string{"Opted Out": "TRUE"}, map[string]string{"Opted Out": ""}) {
		return
	}
	slog.InfoContext(r.Context(), "optout: removed from the directory", "actor", me, "key", key)
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
	if !u.applyOverride(w, r, me, key, "facts update", cells, previous) {
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
		serverError(w, r, err)
		return
	}

	// A person's photos are a list; every other kind is a single slot, named on the
	// owner's Overrides or Families row.
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
		if !u.setPhotos(w, r, me, key, order, cells, previous, "photo upload") {
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
		cells, previous := map[string]string{}, map[string]string{}
		if kind == "photo" {
			cells["Family Photo"], cells["Family Photo Updated"] = name, today()
			previous["Family Photo"] = family.photo
			previous["Family Photo Updated"] = family.PhotoUpdated
		} else {
			cells["Family Pronunciation"] = name
			previous["Family Pronunciation"] = family.pronunciation
		}
		if !u.applyFamily(w, r, me, family.Key, kind+" upload", cells, previous) {
			return
		}
		slog.InfoContext(r.Context(), "upload: set media", "actor", me, "target", target, "key", key, "kind", kind, "name", name)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	cells := map[string]string{"Pronunciation": name}
	previous := map[string]string{"Pronunciation": model.Person(key).pronunciation}
	if !u.applyOverride(w, r, me, key, kind+" upload", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "upload: set media", "actor", me, "target", target, "key", key, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

// setPhotos replaces a person's complete photo list and folds the change into the
// running model before responding, so the caller's very next model fetch sees it.
// Uploading (append), drag-reorder (permute), deleting (remove one), and cropping
// (attach a crop to one) all funnel through this one path rather than four ad hoc
// ones, since each is really just "this person's photo list is now exactly order" -
// one place to get the sheet-write-then-rebuild interaction right instead of four.
func (u uploader) setPhotos(w http.ResponseWriter, r *http.Request, me, key string, order []photoRef, cells, previous map[string]string, changeAction string) bool {
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
		serverError(w, r, fmt.Errorf("rewrite photo list for %s: %w", key, err))
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
			slog.ErrorContext(r.Context(), "set overrides", "email", key, "error", err)
			return
		}
		if err := u.sheet.Append(appName, changeLogTable, logRow); err != nil {
			log.Fatalf("[ERROR] append change log after %s for %s: %v", changeAction, key, err)
		}
	})
	if err := <-applied; err != nil {
		serverError(w, r, fmt.Errorf("rebuild model after %s: %w", changeAction, err))
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
	if !u.setPhotos(w, r, me, key, order, cells, previous, "photo reorder") {
		return
	}
	slog.InfoContext(r.Context(), "reorder-photos: set photo list", "actor", me, "photos", len(order), "key", key)
	w.WriteHeader(http.StatusNoContent)
}

// cropPhoto attaches a crop to an existing photo - a square crop for one of a
// person's photos (name identifies which one), or an arbitrary-shape crop for
// a family's single photo (families only ever have one, so name is unused) -
// replacing any crop it already had.
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
	if !u.mayEdit(model, me, target, key) {
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
		cells := map[string]string{"Family Photo Crop": cropName, "Family Photo Updated": today()}
		previous := map[string]string{"Family Photo Crop": family.photoCropName, "Family Photo Updated": family.PhotoUpdated}
		if !u.applyFamily(w, r, me, family.Key, "family photo crop", cells, previous) {
			return
		}
		slog.InfoContext(r.Context(), "crop-photo: set a crop on the family photo", "actor", me, "family", key)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	order := make([]photoRef, len(person.Photos))
	for i, photo := range person.Photos {
		order[i] = photoRef{Name: photo.Name, CropName: photo.cropName}
	}
	for i := range order {
		if order[i].Name == name {
			order[i].CropName = cropName
		}
	}

	cells := map[string]string{"Photo Updated": today()}
	previous := map[string]string{"Photo Updated": person.PhotoUpdated}
	if !u.setPhotos(w, r, me, key, order, cells, previous, "photo crop") {
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
		return slices.Contains(model.FamilyKeysOf(mine.Email), key)
	}
	if key == me {
		return true
	}
	for _, familyKey := range model.FamilyKeysOf(mine.Email) {
		family := model.Families[familyKey]
		if slices.Contains(family.KidEmails, key) || slices.Contains(family.AdultEmails, key) {
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
