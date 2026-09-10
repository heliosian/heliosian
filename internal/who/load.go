package who

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
)

const appName = "directory"

const (
	preferencesApp = "preferences"
	preferencesTab = "Sheet1"
)

// The school's public staff page, as webexport writes it and cmd/import syncs it: the
// bio the school publishes, flattened to text on the way in, plus the title and
// portrait that go with it. Departments are carried for reference and never read -
// the school's own filing lives in Overrides. Exported so the import writes the tab
// this reads without a second copy of its shape.
const WebsiteTable = "Website Staff Import"

const (
	WebsiteID          = "constituent_id"
	WebsiteName        = "full_name"
	WebsiteTitle       = "title"
	WebsiteDepartments = "departments"
	WebsiteEmailColumn = "email"
	WebsiteBio         = "bio"
	websitePhotoName   = "photo"
)

var WebsiteColumns = []string{
	WebsiteID, WebsiteName, WebsiteTitle, WebsiteDepartments,
	WebsiteEmailColumn, WebsiteBio, websitePhotoName,
}

const AliasesTable = "Email Aliases"

const (
	AliasColumn      = "Alias"
	AliasEmailColumn = "Email"
)

var AliasColumns = []string{AliasColumn, AliasEmailColumn}

var importEmailColumns = []string{
	"student_email",
	"household_1_person_1_email", "household_1_person_2_email",
	"household_2_person_1_email", "household_2_person_2_email",
}

const tagsTable = "Tags"

const (
	tagOwner  = "Owner Email"
	tagName   = "Tag"
	tagPerson = "Person Email"
)

var tagColumns = []string{tagOwner, tagName, tagPerson}

var gradeOrder = []string{
	"Kindergarten", "Grade 1", "Grade 2", "Grade 3", "Grade 4",
	"Grade 5", "Grade 6", "Grade 7", "Grade 8",
}

var gradeBands = map[string]string{
	"Kindergarten": "Hummingbirds",
	"Grade 1":      "Halcons",
	"Grade 2":      "Halcons",
	"Grade 3":      "Jayvens",
	"Grade 4":      "Jayvens",
	"Grade 5":      "Cospreys",
	"Grade 6":      "Cospreys",
	"Grade 7":      "Hegrets",
	"Grade 8":      "Hegrets",
}

var departmentOrder = []string{
	"Admin and Office Staff",
	"Co-Curriculars and Specialists",
	"Classroom Teachers",
	"Facilities Staff",
}

var importColumns = []string{
	"entry_sort_name", "student_full_name", "student_classifications", "student_email",
	"student_photo",
}

var staffImportColumns = []string{
	"person_full_name", "person_job_title", "person_classifications", "person_email",
	"person_phone_business", "person_photo",
}

// Veracross faculty types belonging to people who are not school staff.
var excludedFacultyTypes = map[string]bool{"Vendors": true}

// noEmailMarker flags a Veracross-generated placeholder address, assigned to someone
// the school hasn't given a real email yet. The address is still that person's real
// identity/key everywhere in this app (routing, Overrides rows, Tags, Photos) - it
// must keep loading and resolving normally. All this marks is that the address itself
// is fake and must never be shown to a viewer (see personEmailDisplay in who.go
// and app.js's use of it): show no email at all for that person, rather than a
// mailto: link nobody can actually use.
const noEmailMarker = ".noemail"

// The permission column's header is misspelled in the form itself; it must match verbatim.
const (
	preferenceTimestamp  = "Timestamp"
	preferenceEmail      = "Email Address"
	preferenceStatus     = "Communication Opt-In Status"
	preferencePermission = "You have my permission to share the folllowing:"
)

const (
	optInAnswer  = "I agree to have family names and emails in the Helios Community Apps"
	optOutAnswer = "Please remove all family names and emails from the Helios Community Apps. " +
		"I understand that we will be unable to access the Helios Who directory, " +
		"the volunteer portal and Spring Celebration fun(d)raiser events."
	sharePhone   = "Adult Phone Number (if provided on Veracross)"
	shareAddress = "Home Address (if provided on Veracross)"
)

const preferenceTimeFormat = "1/2/2006 15:04:05"

var overrideColumns = []string{
	"Email", "Added", "Full Name", "Legal Name", "Preferred Name",
	"Is Student", "Is Parent", "Is Staff", "New to Helios", "Pronouns", "Facts",
	"Grade", "Classroom", "Crew", "Phone", "Job Title", "Department", "Grade Band", "Room Parent",
	"Opted Out", "Photo Updated", "Facts Updated",
	"Veracross Photo", "Primary Photo", "Pronunciation",
}

var familyColumns = []string{
	"Email", "Address", "Family Phone", "Family Photo Caption",
	"Family Photo Updated", "Family Photo", "Family Photo Crop", "Family Pronunciation",
}

const updatedFormat = "2006-01-02"

func checkUpdated(email, column, cell string) error {
	if cell == "" || cell == "-" {
		return nil
	}
	if _, err := time.Parse(updatedFormat, cell); err != nil {
		return fmt.Errorf("row %s has invalid %s %q", email, column, cell)
	}
	return nil
}

// BlobChecker answers for the media a sheet names: whether an object exists, and
// a chance to fetch many at once before they are asked for one by one.
type BlobChecker interface {
	Has(name string) (bool, error)
	Prefetch(names []string) error
}

type parsedName struct {
	display, legal, preferred string
}

var nameForm = regexp.MustCompile(`^(.+?) \((.+?)\) (.+)$`)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// leadingDigit distinguishes a street-level address ("123 Main St...") from a
// city-only one ("Dayton, OH") among the granularities Veracross hands us - see
// "household addresses arrive at whatever granularity each family chose" in
// docs/who/data.md. It's a heuristic, not a Veracross field: nothing in the export says
// which granularity a family picked, only the resulting string.
var leadingDigit = regexp.MustCompile(`^\s*\d`)

const (
	veracrossFull    = "full"
	veracrossPartial = "partial"
	veracrossVisible = "visible"
	veracrossMixed   = "mixed"
	veracrossHidden  = "hidden"
)

func parseName(raw string) parsedName {
	raw = strings.Join(strings.Fields(raw), " ")
	m := nameForm.FindStringSubmatch(raw)
	if m == nil {
		return parsedName{display: raw, legal: raw}
	}
	return parsedName{display: m[1] + " " + m[3], legal: m[2] + " " + m[3], preferred: m[1]}
}

func splitHomeroom(homeroom string) (classroom, crew string) {
	fields := strings.Fields(homeroom)
	if len(fields) == 1 {
		return homeroom, ""
	}
	return fields[len(fields)-1], strings.Join(fields[:len(fields)-1], " ")
}

// NormName is how every name in the sheet is matched against a name in the Veracross
// export. Exported so the import tool finds exactly the rows this package would.
func NormName(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

type household struct {
	adults  []string
	kids    []string
	address string
	phone   string
}

// resolveImage prefers an admin-uploaded image in the bucket over the bundled default,
// so a school that never touches the admin page keeps the illustrations it shipped
// with, and one that does sees its replacement without a deploy.
func (l *loader) resolveImage(blobFolder, slug, staticKey string) (string, error) {
	if l.blobs != nil {
		uploaded, err := l.blobs.Has(blobFolder + "/" + slug)
		if err != nil {
			return "", err
		}
		if uploaded {
			return "/" + blobFolder + "/" + slug + ".jpg", nil
		}
	}
	bundled, err := l.static.Has(staticKey)
	if err != nil {
		return "", err
	}
	if bundled {
		return "/" + staticKey, nil
	}
	return "", nil
}

func gradeSlug(name string) string {
	if name == "Kindergarten" {
		return "k"
	}
	return strings.TrimPrefix(name, "Grade ")
}

type loader struct {
	blobs  BlobChecker
	static BlobChecker

	aliasRows      []map[string]string
	importRows     []map[string]string
	staffRows      []map[string]string
	nameRows       []map[string]string
	overrideRows   []map[string]string
	familyRows     []map[string]string
	photoRows      []map[string]string
	preferenceRows []map[string]string
	websiteRows    []map[string]string
	nameToEmail    map[string]string

	people           map[string]*Person
	order            []string
	households       map[string]*household
	householdOrder   []string
	personHouseholds map[string][]string
	familyKeys       map[string]string
	roomParents      map[string][]string
	optedOut         map[string]bool
	withheld         map[string]bool
	excluded         map[string]bool
	useless          []string

	model *Model
}

// Tables are the parsed source tables a model is built from. Tags are per-user
// and deliberately never reach Model, which is served to every member.
type Tables struct {
	Aliases     []map[string]string
	Imports     []map[string]string
	Staff       []map[string]string
	Names       []map[string]string
	Overrides   []map[string]string
	Families    []map[string]string
	Preferences []map[string]string
	Website     []map[string]string
	Tags        []map[string]string
	Photos      []map[string]string
	Admins      []map[string]string
}

const adminsTable = "Admins"

// withOverride mirrors what data.Sheet.Upsert just wrote, copying the rows it
// touches so the tables the current model was built from stay intact.
func (t *Tables) withOverride(email string, cells map[string]string) *Tables {
	rows := make([]map[string]string, len(t.Overrides))
	copy(rows, t.Overrides)
	found := false
	for i, row := range rows {
		if !strings.EqualFold(row["Email"], email) {
			continue
		}
		next := maps.Clone(row)
		applyCells(next, cells)
		rows[i] = next
		found = true
	}
	if !found {
		row := map[string]string{"Email": email}
		applyCells(row, cells)
		rows = append(rows, row)
	}
	out := *t
	out.Overrides = rows
	return &out
}

// withFamily is withOverride for the Families tab, keyed by the family key (the
// alphabetically first parent email, which is also the row's Email cell).
func (t *Tables) withFamily(key string, cells map[string]string) *Tables {
	rows := make([]map[string]string, len(t.Families))
	copy(rows, t.Families)
	found := false
	for i, row := range rows {
		if !strings.EqualFold(row["Email"], key) {
			continue
		}
		next := maps.Clone(row)
		applyCells(next, cells)
		rows[i] = next
		found = true
	}
	if !found {
		row := map[string]string{"Email": key}
		applyCells(row, cells)
		rows = append(rows, row)
	}
	out := *t
	out.Families = rows
	return &out
}

// withEmailRenamed is withOverride for a person whose email itself is changing -
// admin.go's setAddedFields, for an added-only person, is the only caller (everyone
// else's email comes from Veracross and isn't renameable through this app at all). It
// renames the Overrides row in place (folding cells into the same pass, same as
// withOverride) and also renames every Tags row where this person is the owner or the
// tagged person, and every Photos row that belongs to them, so a rename doesn't
// silently strand their tags or photos under the old address.
func (t *Tables) withEmailRenamed(oldEmail, newEmail string, cells map[string]string) *Tables {
	overrides := make([]map[string]string, len(t.Overrides))
	copy(overrides, t.Overrides)
	found := false
	for i, row := range overrides {
		if !strings.EqualFold(row["Email"], oldEmail) {
			continue
		}
		next := maps.Clone(row)
		next["Email"] = newEmail
		applyCells(next, cells)
		overrides[i] = next
		found = true
	}
	if !found {
		row := map[string]string{"Email": newEmail}
		applyCells(row, cells)
		overrides = append(overrides, row)
	}

	renameColumn := func(rows []map[string]string, column string) []map[string]string {
		next := make([]map[string]string, len(rows))
		for i, row := range rows {
			if strings.EqualFold(row[column], oldEmail) {
				clone := maps.Clone(row)
				clone[column] = newEmail
				next[i] = clone
			} else {
				next[i] = row
			}
		}
		return next
	}

	out := *t
	out.Overrides = overrides
	out.Tags = renameColumn(renameColumn(t.Tags, tagOwner), tagPerson)
	out.Photos = renameColumn(t.Photos, "Email")
	return &out
}

// withoutPerson removes an added-only person's Overrides row, every Tags row where
// they're the owner or the tagged person, and every Photos row that belongs to them -
// admin.go's setAddedFields (the Added Overrides tab's delete) is the only caller.
// Since an added-only person exists solely because their Overrides row says so (no
// Veracross import row backs them), removing that row is enough on its own to make
// them vanish from the next rebuild; the Tags/Photos cleanup is just good hygiene, so
// their old email doesn't keep orphaned rows around forever.
func (t *Tables) withoutPerson(email string) *Tables {
	without := func(rows []map[string]string, column string) []map[string]string {
		next := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			if !strings.EqualFold(row[column], email) {
				next = append(next, row)
			}
		}
		return next
	}
	withoutTags := func(rows []map[string]string) []map[string]string {
		next := make([]map[string]string, 0, len(rows))
		for _, row := range rows {
			if strings.EqualFold(row[tagOwner], email) || strings.EqualFold(row[tagPerson], email) {
				continue
			}
			next = append(next, row)
		}
		return next
	}
	out := *t
	out.Overrides = without(t.Overrides, "Email")
	out.Tags = withoutTags(t.Tags)
	out.Photos = without(t.Photos, "Email")
	return &out
}

func applyCells(row, cells map[string]string) {
	for column, value := range cells {
		// parseTable drops blank cells, so a cleared column vanishes rather than holding "".
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}

// photoRef names one of a person's photos and, if set, the blob name of its linked
// square crop - the version shown wherever this photo renders as a square (grid
// tile, hero, roster avatar). The original itself is untouched and is what
// "View photo" always shows.
type photoRef struct {
	Name     string
	CropName string
}

// withPhotos mirrors what a Photos-sheet rewrite just wrote: replace this email's
// rows wholesale with refs, in order. Every mutation of a person's photo list
// (upload, reorder, delete, crop) rewrites the full list this way, so this is the
// only shape a change to Photos ever takes.
func (t *Tables) withPhotos(email string, refs []photoRef) *Tables {
	rows := make([]map[string]string, 0, len(t.Photos)+len(refs))
	for _, row := range t.Photos {
		if !strings.EqualFold(row["Email"], email) {
			rows = append(rows, row)
		}
	}
	for _, ref := range refs {
		row := map[string]string{"Email": email, "Photo Name": ref.Name}
		if ref.CropName != "" {
			row["Crop Name"] = ref.CropName
		}
		rows = append(rows, row)
	}
	out := *t
	out.Photos = rows
	return &out
}

func LoadModel(source data.Source, blobs, static BlobChecker) (*Model, error) {
	tables, err := ReadTables(source)
	if err != nil {
		return nil, err
	}
	return BuildModel(tables, blobs, static)
}

func BuildModel(tables *Tables, blobs, static BlobChecker) (*Model, error) {
	l := &loader{
		blobs:            blobs,
		static:           static,
		aliasRows:        tables.Aliases,
		importRows:       tables.Imports,
		staffRows:        tables.Staff,
		nameRows:         tables.Names,
		overrideRows:     tables.Overrides,
		familyRows:       tables.Families,
		photoRows:        tables.Photos,
		preferenceRows:   tables.Preferences,
		websiteRows:      tables.Website,
		people:           map[string]*Person{},
		households:       map[string]*household{},
		personHouseholds: map[string][]string{},
		familyKeys:       map[string]string{},
		roomParents:      map[string][]string{},
		optedOut:         map[string]bool{},
		withheld:         map[string]bool{},
		excluded:         map[string]bool{},
		model:            &Model{Families: map[string]Family{}, RoomParents: map[string][]string{}},
	}
	steps := []func() error{
		l.applyEmailAliases,
		l.buildNameToEmail,
		l.applyNameToEmail,
		l.transformImport,
		l.transformStaffImport,
		l.applyWebsite,
		l.applyOverrides,
		l.maskFakeEmails,
		l.hideStudentPhones,
		l.buildFamilies,
		l.applyFamilies,
		l.classifyVeracrossVisibility,
		l.applyPreferences,
		l.removeOptedOut,
		l.removeWithheld,
		l.indexFamilies,
		l.attachBlobs,
		l.sortPeople,
		l.deriveClassrooms,
		l.deriveStructure,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return l.model, nil
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		app    string
		name   string
		header []string
		rows   []map[string]string
		err    error
	}
	aliases := &table{app: appName, name: AliasesTable}
	imports := &table{app: appName, name: "Veracross Student Import"}
	staff := &table{app: appName, name: "Veracross Staff Import"}
	names := &table{app: appName, name: "Name to Email"}
	overrides := &table{app: appName, name: "Overrides"}
	families := &table{app: appName, name: "Families"}
	preferences := &table{app: preferencesApp, name: preferencesTab}
	website := &table{app: appName, name: WebsiteTable}
	tags := &table{app: appName, name: tagsTable}
	photos := &table{app: appName, name: "Photos"}
	admins := &table{app: appName, name: adminsTable}
	// Header only: the change log is never read into the model, and it gains a row per
	// member edit forever. Nothing else compares its columns against what the app
	// writes, and a column missing here truncates every audit row that reaches it.
	changeLog := &table{app: appName, name: changeLogTable}
	ordered := []*table{aliases, imports, staff, names, overrides, families, preferences, website, tags, photos, admins}
	var wg sync.WaitGroup
	for _, t := range ordered {
		wg.Go(func() {
			t.header, t.rows, t.err = source.Table(t.app, t.name)
		})
	}
	wg.Go(func() {
		changeLog.header, changeLog.err = source.Header(changeLog.app, changeLog.name)
	})
	wg.Wait()
	for _, t := range append(ordered, changeLog) {
		if t.err != nil {
			return nil, t.err
		}
	}
	if err := data.CheckColumns(aliases.name, aliases.header, AliasColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(imports.name, imports.header, importColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(staff.name, staff.header, staffImportColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(names.name, names.header, []string{"Name", "Email"}); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(overrides.name, overrides.header, overrideColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(families.name, families.header, familyColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(preferences.name, preferences.header, []string{
		preferenceTimestamp, preferenceEmail, preferenceStatus, preferencePermission,
	}); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(website.name, website.header, WebsiteColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(tags.name, tags.header, tagColumns); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(photos.name, photos.header, []string{"Email", "Photo Name"}); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(admins.name, admins.header, []string{"Email"}); err != nil {
		return nil, err
	}
	if err := data.CheckColumns(changeLog.name, changeLog.header, changeLogHeader); err != nil {
		return nil, err
	}
	return &Tables{
		Aliases:     aliases.rows,
		Imports:     imports.rows,
		Staff:       staff.rows,
		Names:       names.rows,
		Overrides:   overrides.rows,
		Families:    families.rows,
		Preferences: preferences.rows,
		Website:     website.rows,
		Tags:        tags.rows,
		Photos:      photos.rows,
		Admins:      admins.rows,
	}, nil
}

// Aliases maps every other address a source publishes for somebody onto the one the
// directory keys them by, which is the address Veracross exports.
type Aliases map[string]string

func ParseAliases(rows []map[string]string) (Aliases, error) {
	aliases := Aliases{}
	for _, row := range rows {
		alias, email := strings.ToLower(strings.TrimSpace(row[AliasColumn])), strings.ToLower(strings.TrimSpace(row[AliasEmailColumn]))
		if !emailForm.MatchString(alias) || !emailForm.MatchString(email) {
			return nil, fmt.Errorf("email aliases row %v needs an alias and an email", row)
		}
		if alias == email {
			return nil, fmt.Errorf("email aliases row %s is an alias of itself", alias)
		}
		if _, ok := aliases[alias]; ok {
			return nil, fmt.Errorf("email aliases has duplicate alias %s", alias)
		}
		aliases[alias] = email
	}
	for alias, email := range aliases {
		if _, ok := aliases[email]; ok {
			return nil, fmt.Errorf("email aliases maps %s to %s, which is itself an alias", alias, email)
		}
	}
	return aliases, nil
}

// Rewrite copies rows, replacing an alias in any of the columns with the address it
// stands for, and reports which aliases it found.
func (a Aliases) Rewrite(rows []map[string]string, columns ...string) ([]map[string]string, map[string]bool) {
	out := make([]map[string]string, len(rows))
	used := map[string]bool{}
	for i, row := range rows {
		out[i] = row
		cloned := false
		for _, column := range columns {
			alias := strings.ToLower(strings.TrimSpace(row[column]))
			email, ok := a[alias]
			if !ok {
				continue
			}
			if !cloned {
				out[i] = maps.Clone(row)
				cloned = true
			}
			out[i][column] = email
			used[alias] = true
		}
	}
	return out, used
}

// applyEmailAliases rewrites the sources authored outside this app before anything
// matches on an address. The app's own tabs are keyed by the resolved address and are
// left alone, so a row written by the app never drifts from the key it was written
// under.
func (l *loader) applyEmailAliases() error {
	aliases, err := ParseAliases(l.aliasRows)
	if err != nil {
		return err
	}
	l.model.aliases = aliases
	used := map[string]bool{}
	rewrite := func(rows []map[string]string, columns ...string) []map[string]string {
		next, found := aliases.Rewrite(rows, columns...)
		maps.Copy(used, found)
		return next
	}
	l.importRows = rewrite(l.importRows, importEmailColumns...)
	l.staffRows = rewrite(l.staffRows, "person_email")
	l.websiteRows = rewrite(l.websiteRows, WebsiteEmailColumn)
	l.preferenceRows = rewrite(l.preferenceRows, preferenceEmail)
	for alias := range aliases {
		if !used[alias] {
			return fmt.Errorf("email alias %s matches no row in any import: drop the row", alias)
		}
	}
	return nil
}

func (l *loader) buildNameToEmail() error {
	l.nameToEmail = map[string]string{}
	for _, row := range l.nameRows {
		// A name with no address is an affirmative decision to leave that person out,
		// which is why the tab needs a row for them at all: Veracross carries people
		// the community directory does not, and silence would be indistinguishable
		// from nobody having looked.
		name, email := NormName(row["Name"]), strings.ToLower(row["Email"])
		if name == "" {
			return fmt.Errorf("name to email row %v has no name", row)
		}
		if _, ok := l.nameToEmail[name]; ok {
			return fmt.Errorf("name to email has duplicate name %q", row["Name"])
		}
		l.nameToEmail[name] = email
	}
	return nil
}

// transformStaffImport adds the people Veracross carries as faculty and staff. It
// supplies only what the export knows: name, job title, email and business phone.
// Department, grade band, classroom and crew stay in Overrides, because Veracross's
// own department field disagrees with the school's filing often enough that importing
// it would silently refile people.
//
// A staff member with no email is skipped rather than fatal. Veracross genuinely has
// none for several of them, and every record here is keyed by address.
func (l *loader) transformStaffImport() error {
	for _, row := range l.staffRows {
		rawName := row["person_full_name"]
		if rawName == "" {
			return fmt.Errorf("staff import row %v has no name", row)
		}
		var classifications struct {
			FacultyType string `json:"faculty_type"`
			Department  string `json:"department"`
		}
		if raw := row["person_classifications"]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &classifications); err != nil {
				return fmt.Errorf("staff %s classifications: %w", rawName, err)
			}
		}
		if excludedFacultyTypes[classifications.FacultyType] || l.excluded[NormName(rawName)] {
			continue
		}
		email := strings.ToLower(row["person_email"])
		if email == "" {
			return fmt.Errorf("staff %s has no email and no name to email entry", rawName)
		}
		n := parseName(rawName)
		if p, ok := l.people[email]; ok {
			// Staff who are also parents arrive twice. The household copy of a name
			// often carries a redundant parenthetical the staff export omits, so only
			// the resolved names have to agree.
			if p.FullName != n.display || p.LegalName != n.legal {
				return fmt.Errorf("staff %s has conflicting names %q and %q", email, p.FullName, rawName)
			}
			p.IsStaff = true
			p.JobTitle = row["person_job_title"]
			p.veracrossPhoto = row["person_photo"]
			if p.Phone == "" {
				p.Phone = row["person_phone_business"]
			}
			continue
		}
		l.people[email] = &Person{
			Email: email, FullName: n.display, LegalName: n.legal, PreferredName: n.preferred,
			IsStaff: true, JobTitle: row["person_job_title"], Phone: row["person_phone_business"],
			veracrossPhoto: row["person_photo"],
		}
		l.order = append(l.order, email)
	}
	return nil
}

// StaffByName indexes the staff import by resolved name, for the entries the staff
// page publishes no address for. A name is keyed to every address it belongs to, so
// a name two staff share can be refused rather than guessed at.
func StaffByName(staffRows []map[string]string) map[string][]string {
	byName := map[string][]string{}
	for _, row := range staffRows {
		email := strings.ToLower(row["person_email"])
		if email == "" {
			continue
		}
		name := NormName(parseName(row["person_full_name"]).display)
		byName[name] = append(byName[name], email)
	}
	return byName
}

// WebsiteEmail is the address a staff page entry belongs to. The page publishes none
// for a few people: Name to Email reaches the ones Veracross has no address for
// either, and the staff import itself, by name, reaches the ones it has. The import
// resolves entries the same way when it tidies the overrides the page has caught up
// with.
func WebsiteEmail(row map[string]string, nameToEmail map[string]string, staffByName map[string][]string) (string, error) {
	if email := strings.ToLower(strings.TrimSpace(row[WebsiteEmailColumn])); email != "" {
		return email, nil
	}
	name := NormName(row[WebsiteName])
	if email, ok := nameToEmail[name]; ok {
		return email, nil
	}
	emails := staffByName[name]
	if len(emails) > 1 {
		return "", fmt.Errorf("the staff page's entry for %s has no address, and %d staff share the name", row[WebsiteName], len(emails))
	}
	if len(emails) == 1 {
		return emails[0], nil
	}
	return "", nil
}

// applyWebsite folds in what the school publishes about its staff on its own site:
// the bio, which nothing else supplies, and a title and portrait for the people
// Veracross has none for. It is an import like any other, running before Overrides:
// a Veracross title still stands, an override still wins, an override that clears a
// field still clears it, and an override that only restates what the page publishes
// is still the dead weight the load refuses to carry.
//
// The site is not the directory's roster. It carries vendors the directory drops and
// people who have left, and it publishes no address for a few, so a row that matches
// nobody is counted and skipped rather than fatal - and the count is logged, because
// a match key that quietly stops working looks exactly like a page nobody has updated.
func (l *loader) applyWebsite() error {
	unmatched := []string{}
	seen := map[string]bool{}
	staffByName := StaffByName(l.staffRows)
	for _, row := range l.websiteRows {
		name := row[WebsiteName]
		if name == "" {
			return fmt.Errorf("website import row %v has no name", row)
		}
		email, err := WebsiteEmail(row, l.nameToEmail, staffByName)
		if err != nil {
			return err
		}
		if email != "" && seen[email] {
			return fmt.Errorf("the staff page has two entries for %s", email)
		}
		seen[email] = true
		p := l.people[email]
		if p == nil {
			unmatched = append(unmatched, name)
			continue
		}
		if p.Facts == "" {
			p.Facts = row[WebsiteBio]
		}
		if p.JobTitle == "" {
			p.JobTitle = row[WebsiteTitle]
		}
		p.websitePhoto = row[websitePhotoName]
	}
	if len(unmatched) > 0 {
		slog.Info("website import: entries match nobody in the directory", "unmatched", len(unmatched), "of", len(l.websiteRows), "names", unmatched)
	}
	return nil
}

func (l *loader) addAdult(rawName, email, phone string) error {
	n := parseName(rawName)
	if p, ok := l.people[email]; ok {
		if p.FullName != n.display || p.LegalName != n.legal || p.PreferredName != n.preferred {
			return fmt.Errorf("adult %s has conflicting names %q and %q", email, p.FullName, rawName)
		}
		if p.Phone != "" && phone != "" && p.Phone != phone {
			return fmt.Errorf("adult %s has conflicting phones %q and %q", email, p.Phone, phone)
		}
		if p.Phone == "" {
			p.Phone = phone
		}
		p.IsParent = true
		return nil
	}
	l.people[email] = &Person{
		Email: email, FullName: n.display, LegalName: n.legal, PreferredName: n.preferred,
		Phone: phone, IsParent: true,
	}
	l.order = append(l.order, email)
	return nil
}

// applyNameToEmail fills in the addresses Veracross omits, for students and staff
// alike, so everything downstream can just read the email column. Matching each entry
// against the rows, rather than the other way round, is what makes an entry that fits
// nothing — or fits two people — fall out here instead of needing to be counted later.
// Rows are copied rather than patched, leaving the cached tables as they were read.
func (l *loader) applyNameToEmail() error {
	l.importRows = slices.Clone(l.importRows)
	l.staffRows = slices.Clone(l.staffRows)
	fill := func(rows []map[string]string, nameColumn, emailColumn, name, email string) (int, error) {
		matches := 0
		for i, row := range rows {
			if NormName(row[nameColumn]) != name {
				continue
			}
			// The tab supplies what Veracross omits, so it never gets to override what
			// Veracross has: an entry carrying a value for somebody the export already
			// has an address for would win silently, right or wrong.
			if email != "" && row[emailColumn] != "" {
				return 0, fmt.Errorf("name to email entry %q carries %s, but veracross exports %s for them: drop the row",
					row[nameColumn], email, row[emailColumn])
			}
			next := maps.Clone(row)
			next[emailColumn] = email
			rows[i] = next
			matches++
		}
		return matches, nil
	}
	for name, email := range l.nameToEmail {
		matches, err := fill(l.importRows, "student_full_name", "student_email", name, email)
		if err != nil {
			return err
		}
		staffMatches, err := fill(l.staffRows, "person_full_name", "person_email", name, email)
		if err != nil {
			return err
		}
		matches += staffMatches
		if matches != 1 {
			return fmt.Errorf("name to email entry %q matches %d import rows", name, matches)
		}
		if email == "" {
			l.excluded[name] = true
		}
	}
	return nil
}

func (l *loader) transformImport() error {
	for _, row := range l.importRows {
		rawName := row["student_full_name"]
		if rawName == "" {
			return fmt.Errorf("import row %v has no student name", row)
		}
		if l.excluded[NormName(rawName)] {
			continue
		}
		var classifications struct {
			GradeLevel string `json:"grade_level"`
			Homeroom   string `json:"homeroom"`
		}
		if err := json.Unmarshal([]byte(row["student_classifications"]), &classifications); err != nil {
			return fmt.Errorf("student %s classifications: %w", rawName, err)
		}
		if gradeBands[classifications.GradeLevel] == "" {
			return fmt.Errorf("student %s has unknown grade %q", rawName, classifications.GradeLevel)
		}
		if classifications.Homeroom == "" {
			return fmt.Errorf("student %s has no homeroom", rawName)
		}
		classroom, crew := splitHomeroom(classifications.Homeroom)

		email := strings.ToLower(row["student_email"])
		if email == "" {
			return fmt.Errorf("student %s has no email and no name to email entry", rawName)
		}
		if _, ok := l.people[email]; ok {
			return fmt.Errorf("student email %s appears twice", email)
		}
		n := parseName(rawName)
		student := &Person{
			Email: email, FullName: n.display, LegalName: n.legal, PreferredName: n.preferred,
			IsStudent: true, Grade: classifications.GradeLevel, Classroom: classroom, Crew: crew,
			veracrossPhoto: row["student_photo"],
		}
		l.people[email] = student
		l.order = append(l.order, email)

		for _, hn := range []string{"1", "2"} {
			adults := []string{}
			for _, pn := range []string{"1", "2"} {
				prefix := "household_" + hn + "_person_" + pn + "_"
				if row[prefix+"full_name"] == "" {
					continue
				}
				adultEmail := strings.ToLower(row[prefix+"email"])
				if adultEmail == "" {
					return fmt.Errorf("adult %q of student %s has no email", row[prefix+"full_name"], rawName)
				}
				phone := row[prefix+"phone_mobile"]
				if phone == "" {
					phone = row[prefix+"phone_business"]
				}
				if err := l.addAdult(row[prefix+"full_name"], adultEmail, phone); err != nil {
					return err
				}
				adults = append(adults, adultEmail)
			}
			if len(adults) == 0 {
				continue
			}
			address, phone := row["household_"+hn+"_address"], row["household_"+hn+"_phone"]
			setKey := strings.Join(func() []string {
				s := append([]string{}, adults...)
				sort.Strings(s)
				return s
			}(), "\n")
			hh, ok := l.households[setKey]
			if !ok {
				hh = &household{adults: adults, address: address, phone: phone}
				l.households[setKey] = hh
				l.householdOrder = append(l.householdOrder, setKey)
				for _, a := range adults {
					if len(l.personHouseholds[a]) > 0 {
						return fmt.Errorf("adult %s belongs to more than one household", a)
					}
					l.personHouseholds[a] = append(l.personHouseholds[a], setKey)
				}
			} else if hh.address != address || hh.phone != phone {
				return fmt.Errorf("household of %v has conflicting address or phone across rows", adults)
			}
			hh.kids = append(hh.kids, email)
			l.personHouseholds[email] = append(l.personHouseholds[email], setKey)
			student.ParentContactEmails = append(student.ParentContactEmails, adults...)
		}
	}

	return nil
}

func (l *loader) applyOverrides() error {
	bandSet := map[string]bool{}
	for _, band := range gradeBands {
		bandSet[band] = true
	}
	seen := map[string]bool{}
	for _, row := range l.overrideRows {
		email := strings.ToLower(row["Email"])
		if email == "" {
			return fmt.Errorf("overrides row %v has no email", row)
		}
		if seen[email] {
			return fmt.Errorf("overrides has duplicate email %s", email)
		}
		seen[email] = true
		added := row["Added"] == "TRUE"
		p, exists := l.people[email]
		if added && exists {
			return fmt.Errorf("overrides row %s is flagged added but the import covers this person", email)
		}
		if !added && !exists {
			return fmt.Errorf("overrides row %s matches no imported person", email)
		}
		if added {
			p = &Person{Email: email}
			l.people[email] = p
			l.order = append(l.order, email)
		}
		p.overrideRow = row
		// Snapshot every field that can genuinely come from Veracross before the
		// apply()/applyBool() calls below have a chance to change any of them - this is
		// the only place in the whole load that ever mutates them, so whatever's here
		// right now is still exactly what the import supplied.
		p.imported = map[string]string{
			"Full Name": p.FullName, "Legal Name": p.LegalName, "Preferred Name": p.PreferredName,
			"Grade": p.Grade, "Classroom": p.Classroom, "Crew": p.Crew,
			"Phone": p.Phone, "Job Title": p.JobTitle, "Facts": p.Facts,
			"Is Staff": map[bool]string{true: "TRUE", false: "FALSE"}[p.IsStaff],
		}

		// An override that restates what the import already says is dead weight: it
		// survives long after the import starts supplying the value, and hides the
		// corrections that matter. Every one is collected and reported together, since
		// finding them one failed load at a time would be miserable.
		useless := func(column, why string) {
			l.useless = append(l.useless, fmt.Sprintf("%s: %s %s", email, column, why))
		}
		apply := func(column string, field *string) {
			switch cell := row[column]; cell {
			case "":
			case "-":
				if *field == "" {
					useless(column, "clears a value that is already empty")
					return
				}
				*field = ""
			default:
				if *field == cell {
					useless(column, fmt.Sprintf("repeats the value the record already has, %q", cell))
					return
				}
				*field = cell
			}
		}
		applyBool := func(column string, field *bool) error {
			switch row[column] {
			case "":
			case "-", "FALSE":
				if !*field {
					useless(column, "is already false")
					return nil
				}
				*field = false
			case "TRUE":
				if *field {
					useless(column, "is already true")
					return nil
				}
				*field = true
			default:
				return fmt.Errorf("overrides row %s has invalid %s %q", email, column, row[column])
			}
			return nil
		}
		apply("Full Name", &p.FullName)
		apply("Legal Name", &p.LegalName)
		apply("Preferred Name", &p.PreferredName)
		for column, field := range map[string]*bool{
			"Is Student": &p.IsStudent, "Is Parent": &p.IsParent, "Is Staff": &p.IsStaff, "New to Helios": &p.IsNew,
		} {
			if err := applyBool(column, field); err != nil {
				return err
			}
		}
		apply("Pronouns", &p.Pronouns)
		apply("Facts", &p.Facts)
		if err := checkUpdated(email, "Facts Updated", row["Facts Updated"]); err != nil {
			return err
		}
		apply("Facts Updated", &p.FactsUpdated)
		if err := checkUpdated(email, "Photo Updated", row["Photo Updated"]); err != nil {
			return err
		}
		apply("Photo Updated", &p.PhotoUpdated)
		apply("Veracross Photo", &p.veracrossPhoto)
		apply("Primary Photo", &p.primaryPhotoOverride)
		apply("Pronunciation", &p.pronunciation)
		if cell := row["Grade"]; cell != "" && cell != "-" && !added && gradeBands[cell] == "" {
			return fmt.Errorf("overrides row %s has unknown grade %q", email, cell)
		}
		apply("Grade", &p.Grade)
		apply("Classroom", &p.Classroom)
		apply("Crew", &p.Crew)
		apply("Phone", &p.Phone)
		apply("Job Title", &p.JobTitle)
		apply("Department", &p.Department)
		if cell := row["Grade Band"]; cell != "" && cell != "-" && !bandSet[cell] {
			return fmt.Errorf("overrides row %s has unknown grade band %q", email, cell)
		}
		apply("Grade Band", &p.GradeBand)
		if cell := row["Room Parent"]; cell != "" && cell != "-" {
			if !bandSet[cell] {
				return fmt.Errorf("overrides row %s has unknown room parent band %q", email, cell)
			}
			l.roomParents[cell] = append(l.roomParents[cell], email)
		}

		switch row["Opted Out"] {
		case "", "-", "FALSE":
		case "TRUE":
			l.optedOut[email] = true
		default:
			return fmt.Errorf("overrides row %s has invalid Opted Out %q", email, row["Opted Out"])
		}

		if added {
			if p.FullName == "" {
				return fmt.Errorf("added row %s has no full name", email)
			}
			if !p.IsStudent && !p.IsParent && !p.IsStaff {
				return fmt.Errorf("added row %s has no role", email)
			}
		}
	}
	if len(l.useless) > 0 {
		return fmt.Errorf("%d useless override cells, delete them from the Overrides tab:\n  %s",
			len(l.useless), strings.Join(l.useless, "\n  "))
	}
	return nil
}

// maskFakeEmails flags every person whose email is a Veracross-generated placeholder
// (see noEmailMarker) with EmailMasked, so viewers never see it, mailto: it, or copy
// it - the address itself stays completely untouched everywhere else this app uses
// it, since it's still that person's real identity/key (routing, Overrides, Tags,
// Photos).
func (l *loader) maskFakeEmails() error {
	for _, p := range l.people {
		if strings.Contains(p.Email, noEmailMarker) {
			p.EmailMasked = true
		}
	}
	return nil
}

func (l *loader) hideStudentPhones() error {
	for _, p := range l.people {
		if p.IsStudent {
			p.Phone = ""
		}
	}
	return nil
}

// buildFamilies keys each family by its alphabetically first adult email - a real
// assertion, not a convenience: an adult belongs to at most one household
// (transformImport enforces it), so the key is unique, and the Families tab is keyed
// by exactly the same email (applyFamilies enforces that too).
func (l *loader) buildFamilies() error {
	for _, setKey := range l.householdOrder {
		hh := l.households[setKey]
		key := slices.Min(hh.adults)
		l.familyKeys[setKey] = key
		l.model.Families[key] = Family{
			Key:             key,
			Address:         hh.address,
			Phone:           hh.phone,
			AdultEmails:     hh.adults,
			KidEmails:       hh.kids,
			importedAddress: hh.address,
		}
	}
	return nil
}

// applyFamilies folds the Families tab in: one row per family, keyed by the family
// key itself. Family-level fields live only here - person Overrides rows carry none.
func (l *loader) applyFamilies() error {
	for _, row := range l.familyRows {
		email := strings.ToLower(row["Email"])
		if email == "" {
			return fmt.Errorf("families row %v has no email", row)
		}
		p, ok := l.people[email]
		if !ok || !p.IsParent {
			return fmt.Errorf("families row %s does not name a parent", email)
		}
		sets := l.personHouseholds[email]
		if len(sets) != 1 {
			return fmt.Errorf("families row %s names a parent with no household", email)
		}
		key := l.familyKeys[sets[0]]
		if email != key {
			return fmt.Errorf("families row %s is not the family key %s", email, key)
		}
		family := l.model.Families[key]
		if family.sheetRow != nil {
			return fmt.Errorf("families has duplicate rows for %s", key)
		}
		family.sheetRow = row
		if cell := row["Address"]; cell != "" {
			family.Address = ""
			if cell != "-" {
				family.Address = cell
			}
		}
		if cell := row["Family Phone"]; cell != "" {
			family.Phone = ""
			if cell != "-" {
				family.Phone = cell
			}
		}
		if cell := row["Family Photo Caption"]; cell != "" && cell != "-" {
			family.PhotoCaption = cell
		}
		if cell := row["Family Photo Updated"]; cell != "" && cell != "-" {
			if err := checkUpdated(email, "Family Photo Updated", cell); err != nil {
				return err
			}
			family.PhotoUpdated = cell
		}
		family.photo = row["Family Photo"]
		family.photoCropName = row["Family Photo Crop"]
		family.pronunciation = row["Family Pronunciation"]
		l.model.Families[key] = family
	}
	return nil
}

// indexFamilies runs after removeOptedOut so the index covers exactly the people the
// model still carries; keys are visited in sorted order so a two-household kid's
// families list is deterministic everywhere it's read.
func (l *loader) indexFamilies() error {
	l.model.familyKeysByEmail = map[string][]string{}
	keys := make([]string, 0, len(l.model.Families))
	for key := range l.model.Families {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		family := l.model.Families[key]
		for _, email := range append(append([]string{}, family.AdultEmails...), family.KidEmails...) {
			l.model.familyKeysByEmail[email] = append(l.model.familyKeysByEmail[email], key)
		}
	}
	return nil
}

type preference struct {
	when    time.Time
	status  OptStatus
	address bool
	phone   bool
}

func parsePreference(row map[string]string) (string, preference, error) {
	email := strings.ToLower(row[preferenceEmail])
	if !emailForm.MatchString(email) {
		return "", preference{}, fmt.Errorf("preferences row %v has invalid email %q", row, row[preferenceEmail])
	}
	when, err := time.Parse(preferenceTimeFormat, row[preferenceTimestamp])
	if err != nil {
		return "", preference{}, fmt.Errorf("preferences row %s has invalid timestamp %q", email, row[preferenceTimestamp])
	}
	pref := preference{when: when}
	switch row[preferenceStatus] {
	case optInAnswer:
		pref.status = OptIn
	case optOutAnswer:
		pref.status = OptOut
	default:
		return "", preference{}, fmt.Errorf("preferences row %s has unknown opt-in status %q", email, row[preferenceStatus])
	}
	if cell := row[preferencePermission]; cell != "" {
		for _, item := range strings.Split(cell, ", ") {
			switch {
			case item == shareAddress && !pref.address:
				pref.address = true
			case item == sharePhone && !pref.phone:
				pref.phone = true
			default:
				return "", preference{}, fmt.Errorf("preferences row %s grants invalid permission %q", email, item)
			}
		}
	}
	return email, pref, nil
}

func (l *loader) applyPreferences() error {
	byFamily := map[string]preference{}
	byPerson := map[string]preference{}
	for _, row := range l.preferenceRows {
		email, pref, err := parsePreference(row)
		if err != nil {
			return err
		}
		if _, ok := l.people[email]; !ok {
			continue
		}
		sets := l.personHouseholds[email]
		if len(sets) == 0 {
			if current, ok := byPerson[email]; !ok || pref.when.After(current.when) {
				byPerson[email] = pref
			}
			continue
		}
		for _, set := range sets {
			key := l.familyKeys[set]
			if current, ok := byFamily[key]; !ok || pref.when.After(current.when) {
				byFamily[key] = pref
			}
		}
	}

	for key, pref := range byFamily {
		family := l.model.Families[key]
		if !pref.address {
			family.Address = ""
			family.AddressMasked = true
		}
		if !pref.phone {
			family.Phone = ""
			family.PhoneMasked = true
		}
		l.model.Families[key] = family
	}

	for _, email := range l.order {
		p := l.people[email]
		p.OptStatus = OptDefault
		governing := []preference{}
		answered := true
		for _, set := range l.personHouseholds[email] {
			pref, ok := byFamily[l.familyKeys[set]]
			if !ok {
				answered = false
				continue
			}
			governing = append(governing, pref)
		}
		if pref, ok := byPerson[email]; ok {
			governing = append(governing, pref)
		}
		for _, pref := range governing {
			if pref.status == OptOut || p.OptStatus == OptDefault {
				p.OptStatus = pref.status
			}
			if !pref.address {
				p.AddressMasked = true
			}
			if !pref.phone && !p.IsStudent {
				p.PhoneMasked = true
				p.Phone = ""
			}
		}
		if len(governing) == 0 {
			answered = false
		}
		// Staff reach the family consent form only through being a parent too, so silence
		// leaves them listed - but an answer of their own that opts out still removes them.
		if p.OptStatus == OptOut || (!answered && !p.IsStaff) {
			l.withheld[email] = true
		}
	}
	return nil
}

// classifyVeracrossVisibility records what Veracross itself shows for each family,
// before applyPreferences can blank Address/Phone under a Helios Who opt-out. It must
// run after buildFamilies (so manual Overrides corrections are folded in) and before
// applyPreferences (so the Helios Who override hasn't erased anything yet).
func (l *loader) classifyVeracrossVisibility() error {
	for key, family := range l.model.Families {
		family.VeracrossAddress = classifyAddress(family.Address)
		family.VeracrossPhone = classifyPhone(family, l.people)
		l.model.Families[key] = family
	}
	return nil
}

func classifyAddress(address string) string {
	switch {
	case address == "":
		return veracrossHidden
	case leadingDigit.MatchString(address):
		return veracrossFull
	default:
		return veracrossPartial
	}
}

// classifyPhone treats each adult's individual phone cell as that adult's own
// Veracross visibility choice - present means they share it, blank means they don't -
// the same way an empty address cell is read as "nothing" rather than "unknown" in
// docs/who/data.md. A family is "mixed" when its adults disagree.
func classifyPhone(family Family, people map[string]*Person) string {
	visible, hidden := 0, 0
	for _, email := range family.AdultEmails {
		if p := people[email]; p != nil && p.Phone != "" {
			visible++
		} else {
			hidden++
		}
	}
	switch {
	case visible > 0 && hidden > 0:
		return veracrossMixed
	case visible > 0:
		return veracrossVisible
	default:
		return veracrossHidden
	}
}

func (l *loader) removeOptedOut() error {
	hidden := make([]string, 0, len(l.optedOut))
	for email := range l.optedOut {
		hidden = append(hidden, email)
	}
	sort.Strings(hidden)
	l.model.hiddenEmails = hidden
	l.removePeople(l.optedOut)
	return nil
}

// removeWithheld drops everyone the consent form does not affirmatively put in the
// directory, which is the same removal an Overrides opt-out performs - but no part of
// Hidden Overrides, since there is no cell for an admin to clear to bring them back.
func (l *loader) removeWithheld() error {
	l.removePeople(l.withheld)
	return nil
}

func (l *loader) removePeople(gone map[string]bool) {
	for email := range gone {
		delete(l.people, email)
	}
	kept := []string{}
	for _, email := range l.order {
		if !gone[email] {
			kept = append(kept, email)
		}
	}
	l.order = kept
	for key, family := range l.model.Families {
		family.AdultEmails = without(family.AdultEmails, gone)
		family.KidEmails = without(family.KidEmails, gone)
		if len(family.AdultEmails)+len(family.KidEmails) == 0 {
			delete(l.model.Families, key)
			continue
		}
		family.Name = familyNameFor(family, l.people)
		l.model.Families[key] = family
	}
	for _, p := range l.people {
		p.ParentContactEmails = without(p.ParentContactEmails, gone)
	}
	for band, emails := range l.roomParents {
		l.roomParents[band] = without(emails, gone)
	}
}

// reconcileLegacyPrimary moves whichever photo the retired Primary Photo override
// still names to the front of the just-built order, so shipping order-is-primary
// doesn't silently change what someone who already made an explicit choice sees.
// Unlike the mechanism this replaces, it never errors - an override naming a photo
// that no longer exists is just ignored, since this is display cosmetics now, not
// data integrity.
func (l *loader) reconcileLegacyPrimary(p *Person) {
	if p.primaryPhotoOverride == "" {
		return
	}
	for i, photo := range p.Photos {
		if photo.Name == p.primaryPhotoOverride {
			if i != 0 {
				p.Photos[0], p.Photos[i] = p.Photos[i], p.Photos[0]
			}
			return
		}
	}
}

func (l *loader) attachBlobs() error {
	if l.blobs == nil {
		return nil
	}
	uploaded := map[string][]photoRef{}
	for _, row := range l.photoRows {
		email := strings.ToLower(row["Email"])
		name := row["Photo Name"]
		if email == "" || name == "" {
			return fmt.Errorf("photos row %v is incomplete", row)
		}
		// A row for somebody no longer in the directory is skipped rather than fatal:
		// people leave, and their photos outlive them in the sheet until touched.
		if l.people[email] == nil {
			continue
		}
		uploaded[email] = append(uploaded[email], photoRef{Name: name, CropName: row["Crop Name"]})
	}

	if err := l.prefetchMedia(uploaded); err != nil {
		return err
	}
	for _, p := range l.people {
		// The implicit veracross-first entry only applies when it isn't already one
		// of this person's own Photos-sheet rows: most people have never explicitly
		// reordered or deleted it, so it's synthesized in front as before, but the
		// moment a row exists for it (any mutation - upload, reorder, delete -
		// persists a complete snapshot, veracross included if it should still be
		// there), that explicit row is authoritative and it's never synthesized a
		// second time. This - not "has this person touched the gallery at all" -
		// is what lets deleting it actually stick.
		refs := uploaded[p.Email]
		named := func(name string) bool {
			if name == "" {
				return true
			}
			for _, ref := range refs {
				if ref.Name == name {
					return true
				}
			}
			return false
		}
		var ordered []photoRef
		if !named(p.veracrossPhoto) {
			ordered = append(ordered, photoRef{Name: p.veracrossPhoto})
		}
		// The staff page's headshot sits behind the school portrait, so importing it
		// never changes the photo the directory already shows for someone.
		if p.websitePhoto != p.veracrossPhoto && !named(p.websitePhoto) {
			ordered = append(ordered, photoRef{Name: p.websitePhoto})
		}
		ordered = append(ordered, refs...)
		for _, ref := range ordered {
			source := "upload"
			switch ref.Name {
			case p.veracrossPhoto:
				source = "veracross"
			case p.websitePhoto:
				source = "website"
			}
			url, err := l.blobURL("photos", ref.Name, p.Email)
			if err != nil {
				return err
			}
			photo := Photo{Name: ref.Name, Source: source, URL: url, OriginalURL: url, cropName: ref.CropName}
			// A crop that fails to resolve (e.g. its object went missing) just falls
			// back to the original rather than failing the whole model load - unlike
			// the original name above, a crop is display cosmetics, not data integrity.
			if ref.CropName != "" {
				if cropURL, err := l.blobURL("photos", ref.CropName, p.Email); err != nil {
					slog.Warn("resolve photo crop", "email", p.Email, "photo", ref.Name, "error", err)
				} else {
					photo.URL = cropURL
				}
			}
			p.Photos = append(p.Photos, photo)
		}
		l.reconcileLegacyPrimary(p)
		if len(p.Photos) > 0 {
			p.PhotoURL = p.Photos[0].URL
		}
		url, err := l.blobURL("pronunciation", p.pronunciation, p.Email)
		if err != nil {
			return err
		}
		p.PronunciationURL = url
		p.HasOwnPronunciation = url != ""
	}
	for key, family := range l.model.Families {
		photo, err := l.blobURL("photos", family.photo, key)
		if err != nil {
			return err
		}
		pronunciation, err := l.blobURL("pronunciation", family.pronunciation, key)
		if err != nil {
			return err
		}
		family.PhotoURL, family.OriginalPhotoURL, family.PronunciationURL = photo, photo, pronunciation
		// Same graceful fallback as a person's photo crop above: a crop that fails
		// to resolve just leaves the original in place rather than failing the load.
		if family.photoCropName != "" {
			if cropURL, err := l.blobURL("photos", family.photoCropName, key); err != nil {
				slog.Warn("resolve family photo crop", "family", key, "error", err)
			} else {
				family.PhotoURL = cropURL
			}
		}
		l.model.Families[key] = family
	}
	// A person with no recording of their own hears their family's - a display-time
	// fallback, not a stored value, so a personal recording (set via the person-keyed
	// upload) overrides it and clearing that reveals the family's again. A person with
	// no photos gets no such fallback: they show the initials placeholder, since kids
	// generally have photos and a family photo standing in read as an error.
	for _, p := range l.people {
		if p.PronunciationURL != "" {
			continue
		}
		for _, key := range l.model.familyKeysByEmail[p.Email] {
			if url := l.model.Families[key].PronunciationURL; url != "" {
				p.PronunciationURL = url
				break
			}
		}
	}
	return nil
}

// prefetchMedia hands the bucket every object name the sheets record in one
// batch, so the fetches run side by side instead of one per row below.
func (l *loader) prefetchMedia(uploaded map[string][]photoRef) error {
	if l.blobs == nil {
		return nil
	}
	names := []string{}
	add := func(kind, name string) {
		if name != "" {
			names = append(names, kind+"/"+name)
		}
	}
	for _, refs := range uploaded {
		for _, ref := range refs {
			add("photos", ref.Name)
			add("photos", ref.CropName)
		}
	}
	for _, p := range l.people {
		add("photos", p.veracrossPhoto)
		add("photos", p.websitePhoto)
		add("pronunciation", p.pronunciation)
	}
	for _, family := range l.model.Families {
		add("photos", family.photo)
		add("photos", family.photoCropName)
		add("pronunciation", family.pronunciation)
	}
	return l.blobs.Prefetch(names)
}

// blobURL turns a recorded object name into the path clients fetch. A name with no
// object behind it is fatal: the sheet is the index, so a miss means the two have
// drifted rather than that the blob is merely absent.
func (l *loader) blobURL(kind, name, owner string) (string, error) {
	if name == "" {
		return "", nil
	}
	found, err := l.blobs.Has(kind + "/" + name)
	if err != nil {
		return "", fmt.Errorf("%s names %s %q: %w", owner, kind, name, err)
	}
	if !found {
		return "", fmt.Errorf("%s names %s %q, which is not in the bucket", owner, kind, name)
	}
	return "/" + kind + "/" + name, nil
}

func (l *loader) sortPeople() error {
	for _, email := range l.order {
		l.model.People = append(l.model.People, *l.people[email])
	}
	sort.Slice(l.model.People, func(i, j int) bool {
		si, sj := surname(l.model.People[i].FullName), surname(l.model.People[j].FullName)
		if si != sj {
			return si < sj
		}
		return l.model.People[i].FullName < l.model.People[j].FullName
	})
	l.model.byEmail = map[string]int{}
	for i, p := range l.model.People {
		l.model.byEmail[p.Email] = i
	}
	return nil
}

type classroomInfo struct {
	crews    map[string]bool
	minGrade int
	bands    map[string]bool
}

func (l *loader) deriveClassrooms() error {
	model := l.model
	classrooms := map[string]*classroomInfo{}
	for _, p := range model.People {
		if p.Classroom == "" || !p.IsStudent || gradeBands[p.Grade] == "" {
			continue
		}
		info, ok := classrooms[p.Classroom]
		if !ok {
			info = &classroomInfo{crews: map[string]bool{}, minGrade: len(gradeOrder), bands: map[string]bool{}}
			classrooms[p.Classroom] = info
		}
		if p.Crew != "" {
			info.crews[p.Crew] = true
		}
		for i, g := range gradeOrder {
			if g == p.Grade && i < info.minGrade {
				info.minGrade = i
			}
		}
		info.bands[gradeBands[p.Grade]] = true
	}
	classroomNames := []string{}
	for name := range classrooms {
		classroomNames = append(classroomNames, name)
	}
	sort.Slice(classroomNames, func(i, j int) bool {
		ci, cj := classrooms[classroomNames[i]], classrooms[classroomNames[j]]
		if ci.minGrade != cj.minGrade {
			return ci.minGrade < cj.minGrade
		}
		return classroomNames[i] < classroomNames[j]
	})
	for _, name := range classroomNames {
		info := classrooms[name]
		if len(info.bands) != 1 {
			return fmt.Errorf("classroom %s spans multiple grade bands", name)
		}
		slug := strings.ToLower(name)
		imageURL, err := l.resolveImage("classroom-images", slug, "brand/classrooms/classroom-"+slug+".jpg")
		if err != nil {
			return err
		}
		model.Classrooms = append(model.Classrooms, Classroom{
			Name:     name,
			ImageURL: imageURL,
			HasCrews: len(info.crews) > 0,
		})
	}

	for _, p := range model.People {
		if !p.IsStaff || p.Classroom == "" {
			continue
		}
		info, ok := classrooms[p.Classroom]
		if !ok {
			return fmt.Errorf("staff %s is assigned to unknown classroom %q", p.Email, p.Classroom)
		}
		if p.Crew != "" && !info.crews[p.Crew] {
			return fmt.Errorf("staff %s is assigned to unknown crew %q of %s", p.Email, p.Crew, p.Classroom)
		}
	}
	for _, name := range classroomNames {
		info := classrooms[name]
		band := ""
		for b := range info.bands {
			band = b
		}
		crews := []string{}
		for crew := range info.crews {
			crews = append(crews, crew)
		}
		sort.Strings(crews)
		if len(crews) == 0 {
			crews = []string{""}
		}
		for _, crewName := range crews {
			crew := Crew{Classroom: name, Name: crewName, GradeBand: band}
			for _, p := range model.People {
				if p.IsStaff && p.Classroom == name && p.Crew == crewName {
					crew.Teachers = append(crew.Teachers, p.Email)
				}
			}
			model.Crews = append(model.Crews, crew)
		}
	}
	return nil
}

func (l *loader) deriveStructure() error {
	for i, grade := range gradeOrder {
		slug := gradeSlug(grade)
		imageURL, err := l.resolveImage("grade-images", slug, "brand/classrooms/grade-"+slug+".jpg")
		if err != nil {
			return err
		}
		g := Grade{Name: grade, Band: gradeBands[grade], ImageURL: imageURL}
		if i+1 < len(gradeOrder) {
			g.NextName = gradeOrder[i+1]
			g.NextBand = gradeBands[g.NextName]
		}
		l.model.Grades = append(l.model.Grades, g)
	}
	for band, emails := range l.roomParents {
		l.model.RoomParents[bandLabel(band)] = emails
	}
	l.model.Departments = append(l.model.Departments, departmentOrder...)
	return nil
}

func bandLabel(band string) string {
	if band == "Hummingbirds" {
		return "K"
	}
	labels := []string{}
	for _, grade := range gradeOrder {
		if gradeBands[grade] != band {
			continue
		}
		number := strings.TrimPrefix(grade, "Grade ")
		switch number {
		case "1":
			labels = append(labels, "1st")
		case "2":
			labels = append(labels, "2nd")
		case "3":
			labels = append(labels, "3rd")
		default:
			labels = append(labels, number+"th")
		}
	}
	return strings.Join(labels, " / ")
}

func without(list []string, drop map[string]bool) []string {
	kept := []string{}
	for _, item := range list {
		if !drop[item] {
			kept = append(kept, item)
		}
	}
	return kept
}

func surname(fullName string) string {
	fields := strings.Fields(fullName)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func familyNameFor(f Family, people map[string]*Person) string {
	members := append(append([]string{}, f.KidEmails...), f.AdultEmails...)
	seen := map[string]bool{}
	names := []string{}
	for _, email := range members {
		p, ok := people[email]
		if !ok {
			continue
		}
		s := surname(p.FullName)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		names = append(names, s)
	}
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, " & ") + " Family"
}
