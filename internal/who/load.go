package who

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/config"
	"heliosian/internal/store"
)

const appName = "directory"

const (
	preferencesApp = "preferences"
	preferencesTab = "Sheet1"
)

const (
	studentsTab  = "Veracross Student Import"
	staffTab     = "Veracross Staff Import"
	namesTab     = "Name to Email"
	overridesTab = "Overrides"
	familiesTab  = "Families"
	photosTab    = "Photos"
	imagesTab    = "Images"
)

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

const managersTable = "Tag Managers"

const managerEmail = "Manager Email"

var managerColumns = []string{tagOwner, tagName, managerEmail}

var photoColumns = []string{"Email", "Photo Name", "Crop Name", store.OrderColumn}

const (
	imageKind  = "Kind"
	imageName  = "Name"
	imageImage = "Image"
)

var imageColumns = []string{imageKind, imageName, imageImage}

const (
	imageClassroom = "classroom"
	imageGrade     = "grade"
)

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
	"entry_sort_name", "person_full_name", "person_job_title", "person_classifications", "person_email",
	"person_phone_business", "person_photo",
}

var excludedFacultyTypes = map[string]bool{"Vendors": true}

const noEmailMarker = ".noemail"

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

const adminsTable = "Admins"

const geocodeTable = "Geocode"

const (
	geocodeAddress = "Address"
	geocodeLat     = "Lat"
	geocodeLng     = "Lng"
)

var geocodeColumns = []string{geocodeAddress, geocodeLat, geocodeLng}

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

type BlobChecker interface {
	Has(name string) (bool, error)
	Prefetch(ctx context.Context, names []string) error
}

type parsedName struct {
	display, legal, preferred string
}

var nameForm = regexp.MustCompile(`^(.+?) \((.+?)\) (.+)$`)

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

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

func NormName(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

type household struct {
	adults  []string
	kids    []string
	address string
	phone   string
}

func (l *loader) resolveImage(kind, name, staticKey string) (string, error) {
	if image := l.images[kind+"\n"+name]; image != "" && l.blobs != nil {
		return l.blobURL("photos", image, kind+" "+name)
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
	ctx    context.Context
	blobs  BlobChecker
	static BlobChecker
	idKey  []byte

	aliasRows      []store.Row
	importRows     []store.Row
	staffRows      []store.Row
	nameRows       []store.Row
	overrideRows   []store.Row
	familyRows     []store.Row
	photoRows      []store.Row
	preferenceRows []store.Row
	websiteRows    []store.Row
	imageRows      []store.Row
	geocodeRows    []store.Row
	adminRows      []store.Row
	nameToEmail    map[string]string
	images         map[string]string

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

type photoRef struct {
	Name     string
	CropName string
	order    string
	stored   bool
}

func familyID(idKey []byte, email string) string {
	mac := hmac.New(sha256.New, idKey)
	mac.Write([]byte(email))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

func BuildModel(ctx context.Context, tables store.Tables, blobs, static BlobChecker, idKey []byte) (*Model, error) {
	l := &loader{
		ctx:              ctx,
		blobs:            blobs,
		static:           static,
		idKey:            idKey,
		aliasRows:        tables[AliasesTable],
		importRows:       tables[studentsTab],
		staffRows:        tables[staffTab],
		nameRows:         tables[namesTab],
		overrideRows:     tables[overridesTab],
		familyRows:       tables[familiesTab],
		photoRows:        tables[photosTab],
		preferenceRows:   tables[preferencesTab],
		websiteRows:      tables[WebsiteTable],
		imageRows:        tables[imagesTab],
		geocodeRows:      tables[geocodeTable],
		adminRows:        tables[adminsTable],
		people:           map[string]*Person{},
		households:       map[string]*household{},
		personHouseholds: map[string][]string{},
		familyKeys:       map[string]string{},
		roomParents:      map[string][]string{},
		optedOut:         map[string]bool{},
		withheld:         map[string]bool{},
		excluded:         map[string]bool{},
		model: &Model{
			Families: map[string]Family{}, RoomParents: map[string][]string{},
			tags: tables[tagsTable], managers: tables[managersTable],
		},
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
		l.readImages,
		l.attachBlobs,
		l.sortPeople,
		l.deriveClassrooms,
		l.deriveStructure,
		l.locate,
		l.readAdmins,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return l.model, nil
}

type Aliases map[string]string

func ParseAliases(rows []store.Row) (Aliases, error) {
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

func (a Aliases) Rewrite(rows []store.Row, columns ...string) ([]store.Row, map[string]bool) {
	out := make([]store.Row, len(rows))
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

func (l *loader) applyEmailAliases() error {
	aliases, err := ParseAliases(l.aliasRows)
	if err != nil {
		return err
	}
	l.model.aliases = aliases
	used := map[string]bool{}
	rewrite := func(rows []store.Row, columns ...string) []store.Row {
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

func StaffByName(staffRows []store.Row) map[string][]string {
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

func WebsiteEmail(row store.Row, nameToEmail map[string]string, staffByName map[string][]string) (string, error) {
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

func (l *loader) applyNameToEmail() error {
	l.importRows = slices.Clone(l.importRows)
	l.staffRows = slices.Clone(l.staffRows)
	fill := func(rows []store.Row, nameColumn, emailColumn, name, email string) (int, error) {
		matches := 0
		for i, row := range rows {
			if NormName(row[nameColumn]) != name {
				continue
			}
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
		p.imported = map[string]string{
			"Full Name": p.FullName, "Legal Name": p.LegalName, "Preferred Name": p.PreferredName,
			"Grade": p.Grade, "Classroom": p.Classroom, "Crew": p.Crew,
			"Phone": p.Phone, "Job Title": p.JobTitle, "Facts": p.Facts,
			"Is Staff": map[bool]string{true: "TRUE", false: "FALSE"}[p.IsStaff],
		}

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

func (l *loader) buildFamilies() error {
	for _, setKey := range l.householdOrder {
		hh := l.households[setKey]
		email := slices.Min(hh.adults)
		key := familyID(l.idKey, email)
		l.familyKeys[setKey] = key
		l.model.Families[key] = Family{
			Key:             key,
			email:           email,
			Address:         hh.address,
			Phone:           hh.phone,
			AdultEmails:     hh.adults,
			KidEmails:       hh.kids,
			importedAddress: hh.address,
		}
	}
	return nil
}

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
		family := l.model.Families[key]
		if email != family.email {
			return fmt.Errorf("families row %s is not the family key %s", email, family.email)
		}
		if family.sheetRow != nil {
			return fmt.Errorf("families has duplicate rows for %s", email)
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

func (l *loader) indexFamilies() error {
	l.model.familyKeysByEmail = map[string][]string{}
	keys := make([]string, 0, len(l.model.Families))
	for key := range l.model.Families {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return l.model.Families[keys[i]].email < l.model.Families[keys[j]].email })
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

func parsePreference(row store.Row) (string, preference, error) {
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

func (l *loader) consentFamilies() map[string]string {
	linked := map[string][]string{}
	for _, sets := range l.personHouseholds {
		for _, a := range sets {
			for _, b := range sets {
				linked[l.familyKeys[a]] = append(linked[l.familyKeys[a]], l.familyKeys[b])
			}
		}
	}
	familyOf := map[string]string{}
	for _, set := range l.householdOrder {
		start := l.familyKeys[set]
		if _, seen := familyOf[start]; seen {
			continue
		}
		familyOf[start] = start
		members := []string{start}
		for i := 0; i < len(members); i++ {
			for _, next := range linked[members[i]] {
				if _, seen := familyOf[next]; !seen {
					familyOf[next] = start
					members = append(members, next)
				}
			}
		}
	}
	return familyOf
}

func (l *loader) applyPreferences() error {
	familyOf := l.consentFamilies()
	byConsentFamily := map[string]preference{}
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
			family := familyOf[l.familyKeys[set]]
			if current, ok := byConsentFamily[family]; !ok || pref.when.After(current.when) {
				byConsentFamily[family] = pref
			}
		}
	}
	byFamily := map[string]preference{}
	for key, family := range familyOf {
		if pref, ok := byConsentFamily[family]; ok {
			byFamily[key] = pref
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
		if p.OptStatus == OptOut || (!answered && !p.IsStaff) {
			l.withheld[email] = true
		}
	}
	return nil
}

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
		family.ShortName, family.Name = familyNameFor(family, l.people)
		l.model.Families[key] = family
	}
	for _, p := range l.people {
		p.ParentContactEmails = without(p.ParentContactEmails, gone)
	}
	for band, emails := range l.roomParents {
		l.roomParents[band] = without(emails, gone)
	}
}

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

func (l *loader) readImages() error {
	l.images = map[string]string{}
	for _, row := range l.imageRows {
		kind, name, image := row[imageKind], row[imageName], row[imageImage]
		if (kind != imageClassroom && kind != imageGrade) || name == "" || image == "" {
			return fmt.Errorf("images row %v needs a kind of %s or %s, a name and an image", row, imageClassroom, imageGrade)
		}
		if _, ok := l.images[kind+"\n"+name]; ok {
			return fmt.Errorf("images has two rows for %s %s", kind, name)
		}
		l.images[kind+"\n"+name] = image
	}
	return nil
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
		if err := store.CheckKey(row[store.OrderColumn]); err != nil {
			return fmt.Errorf("photos row %s %s: %w", email, name, err)
		}
		if l.people[email] == nil {
			continue
		}
		uploaded[email] = append(uploaded[email], photoRef{Name: name, CropName: row["Crop Name"], order: row[store.OrderColumn], stored: true})
	}
	for _, refs := range uploaded {
		slices.SortStableFunc(refs, func(a, b photoRef) int { return store.CompareKeys(a.order, b.order) })
	}

	if err := l.prefetchMedia(uploaded); err != nil {
		return err
	}
	for _, p := range l.people {
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
			photo := Photo{Name: ref.Name, Source: source, URL: url, OriginalURL: url, cropName: ref.CropName, order: ref.order, stored: ref.stored}
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
		photo, err := l.blobURL("photos", family.photo, family.email)
		if err != nil {
			return err
		}
		pronunciation, err := l.blobURL("pronunciation", family.pronunciation, family.email)
		if err != nil {
			return err
		}
		family.PhotoURL, family.OriginalPhotoURL, family.PronunciationURL = photo, photo, pronunciation
		if family.photoCropName != "" {
			if cropURL, err := l.blobURL("photos", family.photoCropName, family.email); err != nil {
				slog.Warn("resolve family photo crop", "family", family.email, "error", err)
			} else {
				family.PhotoURL = cropURL
			}
		}
		l.model.Families[key] = family
	}
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

func (l *loader) prefetchMedia(uploaded map[string][]photoRef) error {
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
	for _, image := range l.images {
		add("photos", image)
	}
	return l.blobs.Prefetch(l.ctx, names)
}

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
		imageURL, err := l.resolveImage(imageClassroom, name, "brand/classrooms/classroom-"+slug+".jpg")
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
		imageURL, err := l.resolveImage(imageGrade, grade, "brand/classrooms/grade-"+slug+".jpg")
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

func (l *loader) locate() error {
	known := map[string][2]float64{}
	for _, row := range l.geocodeRows {
		address := row[geocodeAddress]
		if address == "" {
			return fmt.Errorf("geocode row %v has no address", row)
		}
		lat, err := strconv.ParseFloat(row[geocodeLat], 64)
		if err != nil {
			return fmt.Errorf("geocode row %q has invalid %s %q", address, geocodeLat, row[geocodeLat])
		}
		lng, err := strconv.ParseFloat(row[geocodeLng], 64)
		if err != nil {
			return fmt.Errorf("geocode row %q has invalid %s %q", address, geocodeLng, row[geocodeLng])
		}
		known[address] = [2]float64{lat, lng}
	}
	unlocated := []string{}
	for key, family := range l.model.Families {
		if family.Address == "" {
			continue
		}
		point, ok := known[family.Address]
		if !ok {
			if !slices.Contains(unlocated, family.Address) {
				unlocated = append(unlocated, family.Address)
			}
			continue
		}
		family.Lat, family.Lng = point[0], point[1]
		l.model.Families[key] = family
	}
	sort.Strings(unlocated)
	l.model.unlocated = unlocated
	return nil
}

func (l *loader) readAdmins() error {
	emails := []string{}
	for _, row := range l.adminRows {
		emails = append(emails, row["Email"])
	}
	l.model.admins = config.NormalizeEmails(emails)
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

func familyNameFor(f Family, people map[string]*Person) (short, full string) {
	names := []string{}
	for _, email := range append(append([]string{}, f.KidEmails...), f.AdultEmails...) {
		p, ok := people[email]
		if !ok {
			continue
		}
		s := surname(p.FullName)
		if s == "" || slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, s) }) {
			continue
		}
		names = append(names, s)
	}
	kept := []string{}
	for _, s := range names {
		within := slices.ContainsFunc(names, func(other string) bool {
			return !strings.EqualFold(other, s) && slices.ContainsFunc(strings.Split(other, "-"), func(part string) bool { return strings.EqualFold(part, s) })
		})
		if !within {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return "", ""
	}
	short = strings.Join(kept, " & ")
	return short, short + " Family"
}
