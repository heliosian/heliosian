package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"heliosian/internal/data"
)

const appName = "directory"

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
	"entry_sort_name", "student_full_name", "student_classifications", "student_email", "student_phone_mobile",
}

var overrideColumns = []string{
	"Email", "Added", "Full Name", "Legal Name", "Preferred Name",
	"Is Student", "Is Parent", "Is Staff", "New to Helios", "Pronouns", "Facts",
	"Grade", "Classroom", "Crew", "Phone", "Job Title", "Department", "Grade Band", "Room Parent",
	"Address", "Family Phone", "Family Photo Caption", "Opted Out",
}

type BlobChecker interface {
	Has(key string) bool
}

type parsedName struct {
	display, legal, preferred string
}

var nameForm = regexp.MustCompile(`^(.+?) \((.+?)\) (.+)$`)

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
	address, phone, caption          string
	hasAddress, hasPhone, hasCaption bool
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

	importRows   []map[string]string
	overrideRows []map[string]string
	nameToEmail  map[string]string

	people           map[string]*Person
	order            []string
	households       map[string]*household
	householdOrder   []string
	personHouseholds map[string][]string
	familyKeys       map[string]string
	familyOverrides  map[string]familyCells
	roomParents      map[string][]string
	optedOut         map[string]bool

	model *Model
}

func LoadModel(source data.Source, blobs, static BlobChecker) (*Model, error) {
	l := &loader{
		blobs:            blobs,
		static:           static,
		people:           map[string]*Person{},
		households:       map[string]*household{},
		personHouseholds: map[string][]string{},
		familyKeys:       map[string]string{},
		familyOverrides:  map[string]familyCells{},
		roomParents:      map[string][]string{},
		optedOut:         map[string]bool{},
		model:            &Model{Families: map[string]Family{}, RoomParents: map[string][]string{}},
	}
	steps := []func() error{
		func() error { return l.readTables(source) },
		l.transformImport,
		l.applyOverrides,
		l.buildFamilies,
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

func (l *loader) readTables(source data.Source) error {
	importHeader, importRows, err := source.Table(appName, "Veracross Import")
	if err != nil {
		return err
	}
	if err := requireColumns("Veracross Import", importHeader, importColumns); err != nil {
		return err
	}
	l.importRows = importRows
	mapHeader, mapRows, err := source.Table(appName, "Name to Email")
	if err != nil {
		return err
	}
	if err := requireColumns("Name to Email", mapHeader, []string{"Name", "Email"}); err != nil {
		return err
	}
	overrideHeader, overrideRows, err := source.Table(appName, "Overrides")
	if err != nil {
		return err
	}
	if err := requireColumns("Overrides", overrideHeader, overrideColumns); err != nil {
		return err
	}
	l.overrideRows = overrideRows

	l.nameToEmail = map[string]string{}
	for _, row := range mapRows {
		name, email := normName(row["Name"]), strings.ToLower(row["Email"])
		if name == "" || email == "" {
			return fmt.Errorf("name to email row %v is incomplete", row)
		}
		if _, ok := l.nameToEmail[name]; ok {
			return fmt.Errorf("name to email has duplicate name %q", row["Name"])
		}
		l.nameToEmail[name] = email
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

func (l *loader) transformImport() error {
	mappingUses := map[string]int{}
	for _, row := range l.importRows {
		rawName := row["student_full_name"]
		if rawName == "" {
			return fmt.Errorf("import row %v has no student name", row)
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
		if mapped, ok := l.nameToEmail[normName(rawName)]; ok {
			email = mapped
			mappingUses[normName(rawName)]++
		}
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
			Phone: row["student_phone_mobile"],
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

	for name := range l.nameToEmail {
		switch mappingUses[name] {
		case 0:
			return fmt.Errorf("name to email entry %q matches no import row", name)
		case 1:
		default:
			return fmt.Errorf("name to email entry %q matches %d import rows", name, mappingUses[name])
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

		apply := func(cell string, field *string) {
			switch cell {
			case "":
			case "-":
				*field = ""
			default:
				*field = cell
			}
		}
		applyBool := func(column string, field *bool) error {
			switch row[column] {
			case "":
			case "-", "FALSE":
				*field = false
			case "TRUE":
				*field = true
			default:
				return fmt.Errorf("overrides row %s has invalid %s %q", email, column, row[column])
			}
			return nil
		}
		apply(row["Full Name"], &p.FullName)
		apply(row["Legal Name"], &p.LegalName)
		apply(row["Preferred Name"], &p.PreferredName)
		for column, field := range map[string]*bool{
			"Is Student": &p.IsStudent, "Is Parent": &p.IsParent, "Is Staff": &p.IsStaff, "New to Helios": &p.IsNew,
		} {
			if err := applyBool(column, field); err != nil {
				return err
			}
		}
		apply(row["Pronouns"], &p.Pronouns)
		apply(row["Facts"], &p.Facts)
		if cell := row["Grade"]; cell != "" && cell != "-" && !added && gradeBands[cell] == "" {
			return fmt.Errorf("overrides row %s has unknown grade %q", email, cell)
		}
		apply(row["Grade"], &p.Grade)
		apply(row["Classroom"], &p.Classroom)
		apply(row["Crew"], &p.Crew)
		apply(row["Phone"], &p.Phone)
		apply(row["Job Title"], &p.JobTitle)
		apply(row["Department"], &p.Department)
		if cell := row["Grade Band"]; cell != "" && cell != "-" && !bandSet[cell] {
			return fmt.Errorf("overrides row %s has unknown grade band %q", email, cell)
		}
		apply(row["Grade Band"], &p.GradeBand)
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
		if cells.hasAddress || cells.hasPhone || cells.hasCaption {
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
		l.model.Families[key] = family
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

func (l *loader) attachBlobs() error {
	if l.blobs == nil {
		return nil
	}
	for _, p := range l.people {
		local, _, _ := strings.Cut(p.Email, "@")
		if l.blobs.Has("people/" + local + "-photo") {
			p.PhotoURL = "/blob/people/" + local + "-photo"
		}
		if l.blobs.Has("people/" + local + "-pronunciation") {
			p.PronunciationURL = "/blob/people/" + local + "-pronunciation"
		}
	}
	for key, family := range l.model.Families {
		if l.blobs.Has("families/" + key + "-photo") {
			family.PhotoURL = "/blob/families/" + key + "-photo"
		}
		if l.blobs.Has("families/" + key + "-pronunciation") {
			family.PronunciationURL = "/blob/families/" + key + "-pronunciation"
		}
		l.model.Families[key] = family
	}
	return nil
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
