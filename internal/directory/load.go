package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	"Address", "Family Phone", "Family Photo Caption", "Opted Out",
	"Photo Updated", "Facts Updated", "Family Photo Updated",
	"Veracross Photo", "Primary Photo", "Pronunciation",
	"Family Photo", "Family Pronunciation",
}

const updatedFormat = "2006-01-02"

func checkUpdated(email, column, cell string) error {
	if cell == "" || cell == "-" {
		return nil
	}
	if _, err := time.Parse(updatedFormat, cell); err != nil {
		return fmt.Errorf("overrides row %s has invalid %s %q", email, column, cell)
	}
	return nil
}

type BlobChecker interface {
	Has(key string) bool
}

type parsedName struct {
	display, legal, preferred string
}

var nameForm = regexp.MustCompile(`^(.+?) \((.+?)\) (.+)$`)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

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

func normName(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

type household struct {
	adults  []string
	kids    []string
	address string
	phone   string
}

type familyCells struct {
	address, phone, caption, photoUpdated             string
	hasAddress, hasPhone, hasCaption, hasPhotoUpdated bool
	photo, pronunciation                              string
}

func exactColumns(table string, header, wanted []string) error {
	if err := requireColumns(table, header, wanted); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, w := range wanted {
		known[w] = true
	}
	for _, h := range header {
		if !known[h] {
			return fmt.Errorf("table %s has unexpected column %q", table, h)
		}
	}
	return nil
}

func requireColumns(table string, header, wanted []string) error {
	present := map[string]bool{}
	for _, h := range header {
		present[h] = true
	}
	for _, w := range wanted {
		if !present[w] {
			return fmt.Errorf("table %s is missing column %q", table, w)
		}
	}
	return nil
}

func familyHash(members []string) string {
	sorted := append([]string{}, members...)
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

type loader struct {
	blobs  BlobChecker
	static BlobChecker

	importRows     []map[string]string
	staffRows      []map[string]string
	nameRows       []map[string]string
	overrideRows   []map[string]string
	photoRows      []map[string]string
	preferenceRows []map[string]string
	nameToEmail    map[string]string

	people           map[string]*Person
	order            []string
	households       map[string]*household
	householdOrder   []string
	personHouseholds map[string][]string
	familyKeys       map[string]string
	familyOverrides  map[string]familyCells
	roomParents      map[string][]string
	optedOut         map[string]bool
	excluded         map[string]bool
	useless          []string

	model *Model
}

// Tables are the parsed source tables a model is built from. Tags are per-user
// and deliberately never reach Model, which is served to every member.
type Tables struct {
	Imports     []map[string]string
	Staff       []map[string]string
	Names       []map[string]string
	Overrides   []map[string]string
	Preferences []map[string]string
	Tags        []map[string]string
	Photos      []map[string]string
}

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
		importRows:       tables.Imports,
		staffRows:        tables.Staff,
		nameRows:         tables.Names,
		overrideRows:     tables.Overrides,
		photoRows:        tables.Photos,
		preferenceRows:   tables.Preferences,
		people:           map[string]*Person{},
		households:       map[string]*household{},
		personHouseholds: map[string][]string{},
		familyKeys:       map[string]string{},
		familyOverrides:  map[string]familyCells{},
		roomParents:      map[string][]string{},
		optedOut:         map[string]bool{},
		excluded:         map[string]bool{},
		model:            &Model{Families: map[string]Family{}, RoomParents: map[string][]string{}},
	}
	steps := []func() error{
		l.buildNameToEmail,
		l.applyNameToEmail,
		l.transformImport,
		l.transformStaffImport,
		l.applyOverrides,
		l.hideStudentPhones,
		l.buildFamilies,
		l.applyPreferences,
		l.removeOptedOut,
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
	imports := &table{app: appName, name: "Veracross Student Import"}
	staff := &table{app: appName, name: "Veracross Staff Import"}
	names := &table{app: appName, name: "Name to Email"}
	overrides := &table{app: appName, name: "Overrides"}
	preferences := &table{app: preferencesApp, name: preferencesTab}
	tags := &table{app: appName, name: tagsTable}
	photos := &table{app: appName, name: "Photos"}
	// Header only: the change log is never read into the model, and it gains a row per
	// member edit forever. Nothing else compares its columns against what the app
	// writes, and a column missing here truncates every audit row that reaches it.
	changeLog := &table{app: appName, name: changeLogTable}
	ordered := []*table{imports, staff, names, overrides, preferences, tags, photos}
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
	if err := requireColumns(imports.name, imports.header, importColumns); err != nil {
		return nil, err
	}
	if err := requireColumns(staff.name, staff.header, staffImportColumns); err != nil {
		return nil, err
	}
	if err := requireColumns(names.name, names.header, []string{"Name", "Email"}); err != nil {
		return nil, err
	}
	if err := requireColumns(overrides.name, overrides.header, overrideColumns); err != nil {
		return nil, err
	}
	if err := exactColumns(preferences.name, preferences.header, []string{
		preferenceTimestamp, preferenceEmail, preferenceStatus, preferencePermission,
	}); err != nil {
		return nil, err
	}
	if err := requireColumns(tags.name, tags.header, tagColumns); err != nil {
		return nil, err
	}
	if err := requireColumns(photos.name, photos.header, []string{"Email", "Photo Name"}); err != nil {
		return nil, err
	}
	if err := exactColumns(changeLog.name, changeLog.header, changeLogHeader); err != nil {
		return nil, err
	}
	return &Tables{
		Imports:     imports.rows,
		Staff:       staff.rows,
		Names:       names.rows,
		Overrides:   overrides.rows,
		Preferences: preferences.rows,
		Tags:        tags.rows,
		Photos:      photos.rows,
	}, nil
}

func (l *loader) buildNameToEmail() error {
	l.nameToEmail = map[string]string{}
	for _, row := range l.nameRows {
		// A name with no address is an affirmative decision to leave that person out,
		// which is why the tab needs a row for them at all: Veracross carries people
		// the community directory does not, and silence would be indistinguishable
		// from nobody having looked.
		name, email := normName(row["Name"]), strings.ToLower(row["Email"])
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
		if excludedFacultyTypes[classifications.FacultyType] || l.excluded[normName(rawName)] {
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
	fill := func(rows []map[string]string, nameColumn, emailColumn, name, email string) int {
		matches := 0
		for i, row := range rows {
			if normName(row[nameColumn]) != name {
				continue
			}
			next := maps.Clone(row)
			next[emailColumn] = email
			rows[i] = next
			matches++
		}
		return matches
	}
	for name, email := range l.nameToEmail {
		matches := fill(l.importRows, "student_full_name", "student_email", name, email)
		matches += fill(l.staffRows, "person_full_name", "person_email", name, email)
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
		if l.excluded[normName(rawName)] {
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
		apply("Primary Photo", &p.PrimaryPhoto)
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

		cells := familyCells{}
		if cell := row["Address"]; cell != "" {
			cells.hasAddress = true
			if cell != "-" {
				cells.address = cell
			}
		}
		if cell := row["Family Phone"]; cell != "" {
			cells.hasPhone = true
			if cell != "-" {
				cells.phone = cell
			}
		}
		if cell := row["Family Photo Caption"]; cell != "" {
			cells.hasCaption = true
			if cell != "-" {
				cells.caption = cell
			}
		}
		if cell := row["Family Photo Updated"]; cell != "" {
			if err := checkUpdated(email, "Family Photo Updated", cell); err != nil {
				return err
			}
			cells.hasPhotoUpdated = true
			if cell != "-" {
				cells.photoUpdated = cell
			}
		}
		cells.photo = row["Family Photo"]
		cells.pronunciation = row["Family Pronunciation"]
		if cells.hasAddress || cells.hasPhone || cells.hasCaption || cells.hasPhotoUpdated ||
			cells.photo != "" || cells.pronunciation != "" {
			l.familyOverrides[email] = cells
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

func (l *loader) hideStudentPhones() error {
	for _, p := range l.people {
		if p.IsStudent {
			p.Phone = ""
		}
	}
	return nil
}

func (l *loader) buildFamilies() error {
	for _, setKey := range l.householdOrder {
		hh := l.households[setKey]
		members := append(append([]string{}, hh.adults...), hh.kids...)
		key := familyHash(members)
		l.familyKeys[setKey] = key
		l.model.Families[key] = Family{
			Key:         key,
			Address:     hh.address,
			Phone:       hh.phone,
			AdultEmails: hh.adults,
			KidEmails:   hh.kids,
		}
	}
	for email, sets := range l.personHouseholds {
		if p := l.people[email]; p.FamilyKey == "" {
			p.FamilyKey = l.familyKeys[sets[0]]
		}
	}
	for email, cells := range l.familyOverrides {
		p := l.people[email]
		if !p.IsParent {
			return fmt.Errorf("overrides row %s has family cells but %s is not a parent", email, email)
		}
		sets := l.personHouseholds[email]
		if len(sets) != 1 {
			return fmt.Errorf("overrides row %s has family cells but %s has no household", email, email)
		}
		key := l.familyKeys[sets[0]]
		family := l.model.Families[key]
		if cells.hasAddress {
			family.Address = cells.address
		}
		if cells.hasPhone {
			family.Phone = cells.phone
		}
		if cells.hasCaption {
			family.PhotoCaption = cells.caption
		}
		if cells.hasPhotoUpdated {
			family.PhotoUpdated = cells.photoUpdated
		}
		if cells.photo != "" {
			family.photo = cells.photo
		}
		if cells.pronunciation != "" {
			family.pronunciation = cells.pronunciation
		}
		l.model.Families[key] = family
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
		for _, set := range l.personHouseholds[email] {
			if pref, ok := byFamily[l.familyKeys[set]]; ok {
				governing = append(governing, pref)
			}
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
	}
	return nil
}

func (l *loader) removeOptedOut() error {
	for email := range l.optedOut {
		delete(l.people, email)
	}
	kept := []string{}
	for _, email := range l.order {
		if !l.optedOut[email] {
			kept = append(kept, email)
		}
	}
	l.order = kept
	for key, family := range l.model.Families {
		family.AdultEmails = without(family.AdultEmails, l.optedOut)
		family.KidEmails = without(family.KidEmails, l.optedOut)
		if len(family.AdultEmails)+len(family.KidEmails) == 0 {
			delete(l.model.Families, key)
			continue
		}
		family.Name = familyNameFor(family, l.people)
		l.model.Families[key] = family
	}
	for _, p := range l.people {
		p.ParentContactEmails = without(p.ParentContactEmails, l.optedOut)
	}
	for band, emails := range l.roomParents {
		l.roomParents[band] = without(emails, l.optedOut)
	}
	return nil
}

func named(names []string, source string) []Photo {
	photos := make([]Photo, len(names))
	for i, name := range names {
		photos[i] = Photo{Name: name, Source: source}
	}
	return photos
}

// setPrimaryPhoto resolves which photo the directory shows. An explicit choice wins,
// otherwise the school portrait does — a person who uploads without choosing has the
// upload made primary at upload time, so falling back to the portrait here only
// affects people who never chose at all.
func (l *loader) setPrimaryPhoto(p *Person) error {
	if p.PrimaryPhoto != "" {
		for _, photo := range p.Photos {
			if photo.Name == p.PrimaryPhoto {
				p.PhotoURL = photo.URL
				return nil
			}
		}
		return fmt.Errorf("%s has primary photo %q, which is not one of their photos", p.Email, p.PrimaryPhoto)
	}
	for _, photo := range p.Photos {
		if photo.Source == "veracross" {
			p.PhotoURL = photo.URL
			p.PrimaryPhoto = photo.Name
			return nil
		}
	}
	return nil
}

func (l *loader) attachBlobs() error {
	if l.blobs == nil {
		return nil
	}
	uploaded := map[string][]string{}
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
		uploaded[email] = append(uploaded[email], name)
	}

	for _, p := range l.people {
		// The school portrait first, then a person's own uploads in sheet order, so a
		// viewer flipping through them starts where the directory does.
		for _, photo := range append(
			[]Photo{{Name: p.veracrossPhoto, Source: "veracross"}},
			named(uploaded[p.Email], "upload")...,
		) {
			if photo.Name == "" {
				continue
			}
			url, err := l.blobURL("photos", photo.Name, p.Email)
			if err != nil {
				return err
			}
			photo.URL = url
			p.Photos = append(p.Photos, photo)
		}
		if err := l.setPrimaryPhoto(p); err != nil {
			return err
		}
		url, err := l.blobURL("pronunciation", p.pronunciation, p.Email)
		if err != nil {
			return err
		}
		p.PronunciationURL = url
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
		family.PhotoURL, family.PronunciationURL = photo, pronunciation
		l.model.Families[key] = family
	}
	return nil
}

// blobURL turns a recorded object name into the path clients fetch. A name with no
// object behind it is fatal: the sheet is the index, so a miss means the two have
// drifted rather than that the blob is merely absent.
func (l *loader) blobURL(kind, name, owner string) (string, error) {
	if name == "" {
		return "", nil
	}
	if !l.blobs.Has(kind + "/" + name) {
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
		imageURL := ""
		imageKey := "brand/classrooms/classroom-" + strings.ToLower(name) + ".jpg"
		if l.static.Has(imageKey) {
			imageURL = "/static/" + imageKey
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
		g := Grade{Name: grade, Band: gradeBands[grade]}
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
