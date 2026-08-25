// Command migrateblobs brings the media bucket into the layout the directory serves:
// it writes the thumbnails older objects are missing, then moves what still lives under
// the retired people/ and families/ prefixes into the content-addressed folders,
// recording every name in the sheet that indexes them.
//
// A person's legacy photo is whatever was in their single slot, which is their Veracross
// portrait unless they replaced it. The two are told apart by perceptual hash rather than
// by content: the portrait in the bucket today is a fresh download and differs from the
// old copy in resolution and encoding, so the bytes never match even when the photograph
// does.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/corona10/goimagehash"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
	storage "google.golang.org/api/storage/v1"

	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/sheetsync"
)

const app = "directory"

// At or below sameDistance the legacy photo is the portrait the sheet already names; at
// or above differentDistance it is a photo the person chose and has to be carried over.
// Between the two nothing is guessed: the run stops naming them, and somebody looks.
//
// The bucket puts every compared photo at 0 or at 14 and above, so the band sits in an
// empty gap rather than across real data. It is there for the rescaled copy that has not
// turned up yet, not for anything measured.
const (
	sameDistance      = 4
	differentDistance = 10
)

var legacyFolders = []string{"people", "families"}

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}

type legacyObject struct {
	name   string
	thumb  string
	folder string
	kind   string
	id     string
	mime   string
}

func base(name string) string {
	return strings.TrimSuffix(path.Base(name), path.Ext(name))
}

func legacyObjects(ctx context.Context, service *storage.Service) ([]legacyObject, error) {
	found := []legacyObject{}
	thumbs := map[string]string{}
	for _, folder := range legacyFolders {
		token := ""
		for {
			call := service.Objects.List(blob.Bucket).Prefix(folder+"/").
				Fields("nextPageToken", "items(name,contentType)").MaxResults(1000)
			if token != "" {
				call = call.PageToken(token)
			}
			page, err := call.Context(ctx).Do()
			if err != nil {
				return nil, fmt.Errorf("list %s: %w", folder, err)
			}
			for _, item := range page.Items {
				stem := base(item.Name)
				// Thumbnails are regenerated from the object they belong to rather than
				// carried across, and are only tracked so they can be swept with it.
				if strings.HasSuffix(stem, "-thumb") {
					thumbs[folder+"/"+strings.TrimSuffix(stem, "-thumb")] = item.Name
					continue
				}
				kind := ""
				switch {
				case strings.HasSuffix(stem, "-photo"):
					kind = "photo"
				case strings.HasSuffix(stem, "-pronunciation"):
					kind = "pronunciation"
				default:
					return nil, fmt.Errorf("object %s is neither a photo nor a pronunciation", item.Name)
				}
				found = append(found, legacyObject{
					name: item.Name, folder: folder, kind: kind, mime: item.ContentType,
					id: strings.TrimSuffix(stem, "-"+kind),
				})
			}
			if page.NextPageToken == "" {
				break
			}
			token = page.NextPageToken
		}
	}
	for i := range found {
		found[i].thumb = thumbs[found[i].folder+"/"+base(found[i].name)]
	}
	sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
	return found, nil
}

var photoColumns = []string{"Primary Photo", "Pronunciation", "Family Photo", "Family Pronunciation"}

type migration struct {
	service  *storage.Service
	uploader *blob.Uploader
	sheet    *data.Sheet
	svc      *sheets.Service
	sheetID  string
	model    *directory.Model

	byLocal     map[string]string
	familyOwner map[string]string
	portrait    map[string]string
	overrides   map[string]map[string]string
	photoRows   map[string]bool
	kept        map[string][][]byte

	cells     map[string]map[string]string
	newPhotos [][]string

	uploaded int
}

// index maps what a legacy name says — an email local part, or a family key — onto the
// person or household it belongs to now. A local part shared by two people is fatal:
// the old layout could not tell them apart either, and guessing would hand somebody
// else's photograph to the wrong person.
func (m *migration) index() error {
	m.byLocal = map[string]string{}
	m.portrait = map[string]string{}
	for i := range m.model.People {
		p := &m.model.People[i]
		local, _, _ := strings.Cut(p.Email, "@")
		if other, dup := m.byLocal[local]; dup {
			return fmt.Errorf("%s and %s share the local part %q", other, p.Email, local)
		}
		m.byLocal[local] = p.Email
		for _, photo := range p.Photos {
			if photo.Source == "veracross" {
				m.portrait[p.Email] = photo.Name
			}
		}
	}
	m.familyOwner = map[string]string{}
	for key, family := range m.model.Families {
		adults := append([]string{}, family.AdultEmails...)
		sort.Strings(adults)
		if len(adults) == 0 {
			continue
		}
		// A household's cells live on one parent's row, and buildFamilies reads them off
		// whichever it finds, so the choice has to be the same on every run.
		m.familyOwner[key] = adults[0]
	}
	return nil
}

func (m *migration) readSheet() error {
	_, rows, err := m.sheet.Table(app, "Overrides")
	if err != nil {
		return err
	}
	m.overrides = map[string]map[string]string{}
	for _, row := range rows {
		m.overrides[strings.ToLower(row["Email"])] = row
	}
	_, photos, err := m.sheet.Table(app, "Photos")
	if err != nil {
		return err
	}
	m.photoRows = map[string]bool{}
	for _, row := range photos {
		m.photoRows[strings.ToLower(row["Email"])+"|"+row["Photo Name"]] = true
	}
	return nil
}

func (m *migration) read(ctx context.Context, name string) ([]byte, error) {
	resp, err := m.service.Objects.Get(blob.Bucket, name).Context(ctx).Download()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	defer resp.Body.Close()
	content := &bytes.Buffer{}
	if _, err := content.ReadFrom(resp.Body); err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return content.Bytes(), nil
}

// owner resolves what a legacy name refers to. The column it returns is the whole
// decision: Primary Photo means the object is one person's photo, whichever folder it
// was filed under, and some family objects are named for a person rather than a
// household.
func (m *migration) owner(o legacyObject) (string, string) {
	if o.folder == "families" {
		if owner := m.familyOwner[o.id]; owner != "" {
			if o.kind == "photo" {
				return owner, "Family Photo"
			}
			return owner, "Family Pronunciation"
		}
	}
	email := m.byLocal[o.id]
	if email == "" {
		return "", ""
	}
	if o.kind == "photo" {
		return email, "Primary Photo"
	}
	return email, "Pronunciation"
}

func hash(content []byte) (*goimagehash.ImageHash, error) {
	img, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	return goimagehash.PerceptionHash(img)
}

func distance(a, b []byte) (int, error) {
	first, err := hash(a)
	if err != nil {
		return 0, err
	}
	second, err := hash(b)
	if err != nil {
		return 0, err
	}
	return first.Distance(second)
}

// classify weighs a person's legacy photo against every photo they already have: the
// portrait Veracross supplies today, and anything kept for them earlier in this run,
// since the same picture can be filed under both folders. It reports the closest
// distance found, so a whole run's worth can be read together.
func (m *migration) classify(ctx context.Context, email string, o legacyObject, content []byte) (string, int, error) {
	candidates := [][]byte{}
	if portrait := m.portrait[email]; portrait != "" {
		original, err := m.read(ctx, "photos/"+portrait)
		if err != nil {
			return "", 0, err
		}
		candidates = append(candidates, original)
	}
	candidates = append(candidates, m.kept[email]...)
	if len(candidates) == 0 {
		return "keep", -1, nil
	}
	closest := 64
	for _, candidate := range candidates {
		d, err := distance(content, candidate)
		if err != nil {
			return "", 0, fmt.Errorf("compare %s for %s: %w", o.name, email, err)
		}
		if d < closest {
			closest = d
		}
	}
	switch {
	case closest <= sameDistance:
		return "drop", closest, nil
	case closest >= differentDistance:
		return "keep", closest, nil
	}
	return "ambiguous", closest, nil
}

func (m *migration) record(email, column, name string) error {
	current := m.overrides[email][column]
	if current == name {
		return nil
	}
	if current != "" {
		return fmt.Errorf("%s already names %s %q, which is not %q", email, column, current, name)
	}
	if m.cells[email] == nil {
		m.cells[email] = map[string]string{}
	}
	// Somebody with more than one legacy photo has all of them listed, and the first in
	// bucket order is the one the directory shows.
	if staged, ok := m.cells[email][column]; ok {
		if staged != name {
			log.Printf("%s shows %s, and %s joins it as another photo", email, staged, name)
		}
		return nil
	}
	m.cells[email][column] = name
	return nil
}

func (m *migration) put(folder, name, mime string, content []byte) error {
	written, err := m.uploader.Put(folder, name, mime, content)
	if err != nil {
		return err
	}
	if written {
		m.uploaded++
	}
	return nil
}

type decision struct {
	object   legacyObject
	email    string
	column   string
	name     string
	folder   string
	content  []byte
	action   string
	distance int
}

// decide settles what becomes of one legacy object without writing anything, so a run
// can be read whole before any of it is acted on.
func (m *migration) decide(ctx context.Context, o legacyObject) (decision, error) {
	d := decision{object: o, distance: -1}
	email, column := m.owner(o)
	if email == "" {
		// People leave and households change shape, and a family's key is a hash of its
		// members, so an object nobody claims is expected rather than wrong.
		d.action = "orphan"
		return d, nil
	}
	content, err := m.read(ctx, o.name)
	if err != nil {
		return d, err
	}
	d.email, d.column = email, column
	d.name = fmt.Sprintf("%x%s", sha256.Sum256(content), path.Ext(o.name))
	d.folder = "photos"
	if o.kind == "pronunciation" {
		d.folder = "pronunciation"
	}
	d.action = "keep"
	if column == "Primary Photo" {
		d.action, d.distance, err = m.classify(ctx, email, o, content)
		if err != nil {
			return d, err
		}
		// The same picture in different bytes is still the only copy of those bytes.
		// Dropping it would leave it indexed by nothing and swept away, so it is
		// attached to the person it belongs to instead.
		if d.action == "drop" && !m.uploader.Has(d.folder+"/"+d.name) {
			log.Printf("attach %s for %s: the portrait's picture, but not the portrait's file", o.name, email)
			d.action = "keep"
		}
	}
	if d.action == "keep" {
		d.content = content
		if column == "Primary Photo" {
			m.kept[email] = append(m.kept[email], content)
		}
	}
	return d, nil
}

func (m *migration) commit(d decision) error {
	if err := m.put(d.folder, d.name, d.object.mime, d.content); err != nil {
		return err
	}
	// A person's photos are a list, so the row lists it and the cell points at it, the
	// pair the upload path writes. Without the pointer the photo is in the bucket but
	// nothing shows it.
	if d.column == "Primary Photo" {
		listed := m.photoRows[d.email+"|"+d.name]
		// Already listed and already pointing somewhere: this photo has been through
		// here before, and whichever of their photos is primary was settled then.
		if listed && m.overrides[d.email]["Primary Photo"] != "" {
			return nil
		}
		if !listed {
			m.newPhotos = append(m.newPhotos, []string{d.email, d.name})
			m.photoRows[d.email+"|"+d.name] = true
		}
	}
	return m.record(d.email, d.column, d.name)
}

// apply writes the index only once every object it names is in the bucket, so the sheet
// never points at something that is not there. Both writes are batched: a cell at a time
// would be hundreds of requests against a quota of sixty a minute.
func (m *migration) apply() error {
	if err := m.sheet.AppendAll(app, "Photos", m.newPhotos); err != nil {
		return fmt.Errorf("append %d photos rows: %w", len(m.newPhotos), err)
	}
	log.Printf("photos tab: %d rows appended", len(m.newPhotos))
	if len(m.cells) == 0 {
		return nil
	}
	rows := []map[string]string{}
	for email, cells := range m.cells {
		existing := m.overrides[email]
		// The tab's own spelling of the address, because Sync matches keys literally and
		// a second row for one person is what makes the model refuse to load.
		key := email
		if existing["Email"] != "" {
			key = existing["Email"]
		}
		row := map[string]string{"Email": key}
		// Every column carried at its current value, so setting one never clears another.
		for _, column := range photoColumns {
			row[column] = existing[column]
		}
		for column, value := range cells {
			row[column] = value
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i]["Email"] < rows[j]["Email"] })
	result, err := sheetsync.Sync(m.svc, m.sheetID, "Overrides",
		append([]string{"Email"}, photoColumns...), rows, "Email", sheetsync.Merge, true)
	if err != nil {
		return err
	}
	log.Printf("overrides: %d cells updated, %d rows added", len(result.Edits), len(result.Added))
	return nil
}

func describe(d decision) string {
	switch {
	case d.action == "orphan":
		return fmt.Sprintf("%q belongs to nobody in the directory", d.object.id)
	case d.distance < 0:
		return fmt.Sprintf("%s, nothing to compare it against", d.email)
	default:
		return fmt.Sprintf("%s, distance %d", d.email, d.distance)
	}
}

// sweep removes the legacy layout once its content is stored under its own hash. An
// object whose content is nowhere else is left exactly where it is: unreferenced is not
// the same as unwanted, and the bucket is the only copy of it.
func (m *migration) sweep(ctx context.Context, decisions []decision) error {
	deleted, left := 0, 0
	for _, d := range decisions {
		if d.action == "orphan" || !m.uploader.Has(d.folder+"/"+d.name) {
			log.Printf("leave %s: its content is stored nowhere else", d.object.name)
			left++
			continue
		}
		for _, name := range []string{d.object.name, d.object.thumb} {
			if name == "" {
				continue
			}
			if err := m.service.Objects.Delete(blob.Bucket, name).Context(ctx).Do(); err != nil {
				return fmt.Errorf("delete %s: %w", name, err)
			}
			deleted++
		}
	}
	log.Printf("%d legacy objects deleted, %d left in place", deleted, left)
	return nil
}

func main() {
	sheetID := flag.String("sheet", "", "spreadsheet id")
	preferencesID := flag.String("preferences", "", "preferences spreadsheet id")
	flag.Parse()
	// This tool's output is its report, so it belongs on stdout where it can be read
	// and captured without merging streams.
	log.SetOutput(os.Stdout)
	if *sheetID == "" || *preferencesID == "" {
		log.Fatal("[ERROR] -sheet and -preferences are required")
	}
	ctx := context.Background()

	uploader, err := blob.NewUploader()
	if err != nil {
		log.Fatalf("[ERROR] storage: %v", err)
	}
	repaired, err := uploader.Repair()
	if err != nil {
		log.Fatalf("[ERROR] repair thumbnails: %v", err)
	}
	log.Printf("repaired %d thumbnails", repaired)

	sheet, err := data.NewSheet(map[string]string{"directory": *sheetID, "preferences": *preferencesID})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(sheet, uploader, staticFiles{})
	if err != nil {
		log.Fatalf("[ERROR] load model: %v", err)
	}
	service, err := storage.NewService(ctx, option.WithScopes(storage.DevstorageReadWriteScope))
	if err != nil {
		log.Fatalf("[ERROR] storage client: %v", err)
	}
	svc, err := sheets.NewService(ctx, option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] sheets client: %v", err)
	}

	m := &migration{
		service: service, uploader: uploader, sheet: sheet, model: model,
		svc: svc, sheetID: *sheetID,
		cells: map[string]map[string]string{},
		kept:  map[string][][]byte{},
	}
	if err := m.index(); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	if err := m.readSheet(); err != nil {
		log.Fatalf("[ERROR] read sheet: %v", err)
	}
	objects, err := legacyObjects(ctx, service)
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("%d objects under %v", len(objects), legacyFolders)
	decisions := []decision{}
	for _, o := range objects {
		d, err := m.decide(ctx, o)
		if err != nil {
			log.Fatalf("[ERROR] %s: %v", o.name, err)
		}
		log.Printf("%s %s: %s", d.action, o.name, describe(d))
		decisions = append(decisions, d)
	}

	// Every distance the run judged on, so the thresholds can be read off real data
	// rather than assumed, and so a gap that is not there shows up as a gap that is not
	// there.
	counts := map[int]int{}
	byAction := map[string]int{}
	ambiguous := []decision{}
	compared := 0
	for _, d := range decisions {
		byAction[d.action]++
		if d.distance >= 0 {
			counts[d.distance]++
			compared++
		}
		if d.action == "ambiguous" {
			ambiguous = append(ambiguous, d)
		}
	}
	distances := []int{}
	for d := range counts {
		distances = append(distances, d)
	}
	sort.Ints(distances)
	log.Printf("distance distribution over %d compared photos:", compared)
	for _, d := range distances {
		log.Printf("  %2d: %d", d, counts[d])
	}
	log.Printf("keep %d, drop %d, ambiguous %d, orphan %d",
		byAction["keep"], byAction["drop"], byAction["ambiguous"], byAction["orphan"])

	if len(ambiguous) > 0 {
		for _, d := range ambiguous {
			log.Printf("[ERROR] ambiguous: %s is distance %d from %s's portrait", d.object.name, d.distance, d.email)
		}
		log.Fatalf("[ERROR] %d objects fall between %d and %d: look at them and settle the thresholds",
			len(ambiguous), sameDistance, differentDistance)
	}

	for _, d := range decisions {
		if d.action != "keep" {
			continue
		}
		if err := m.commit(d); err != nil {
			log.Fatalf("[ERROR] %s: %v", d.object.name, err)
		}
	}
	log.Printf("%d objects uploaded", m.uploaded)
	if err := m.apply(); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	log.Printf("%d photos rows appended, %d overrides rows updated", len(m.newPhotos), len(m.cells))
	if err := m.sweep(ctx, decisions); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
}
