package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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

func LoadModel(source data.Source, blobs BlobChecker) (*Model, error) {
	importHeader, importRows, err := source.Table(appName, "Veracross Import")
	if err != nil {
		return nil, err
	}
	if err := requireColumns("Veracross Import", importHeader, importColumns); err != nil {
		return nil, err
	}
	mapHeader, mapRows, err := source.Table(appName, "Name to Email")
	if err != nil {
		return nil, err
	}
	if err := requireColumns("Name to Email", mapHeader, []string{"Name", "Email"}); err != nil {
		return nil, err
	}
	overrideHeader, overrideRows, err := source.Table(appName, "Overrides")
	if err != nil {
		return nil, err
	}
	if err := requireColumns("Overrides", overrideHeader, overrideColumns); err != nil {
		return nil, err
	}

	nameToEmail := map[string]string{}
	for _, row := range mapRows {
		name, email := normName(row["Name"]), strings.ToLower(row["Email"])
		if name == "" || email == "" {
			return nil, fmt.Errorf("name to email row %v is incomplete", row)
		}
		if _, ok := nameToEmail[name]; ok {
			return nil, fmt.Errorf("name to email has duplicate name %q", row["Name"])
		}
		nameToEmail[name] = email
	}
	mappingUses := map[string]int{}

	people := map[string]*Person{}
	order := []string{}
	households := map[string]*household{}
	householdOrder := []string{}
	personHouseholds := map[string][]string{}

	addAdult := func(rawName, email, phone string) error {
		n := parseName(rawName)
		if p, ok := people[email]; ok {
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
		people[email] = &Person{
			Email: email, FullName: n.display, LegalName: n.legal, PreferredName: n.preferred,
			Phone: phone, IsParent: true,
		}
		order = append(order, email)
		return nil
	}

	for _, row := range importRows {
		rawName := row["student_full_name"]
		if rawName == "" {
			return nil, fmt.Errorf("import row %v has no student name", row)
		}
		var classifications struct {
			GradeLevel string `json:"grade_level"`
			Homeroom   string `json:"homeroom"`
		}
		if err := json.Unmarshal([]byte(row["student_classifications"]), &classifications); err != nil {
			return nil, fmt.Errorf("student %s classifications: %w", rawName, err)
		}
		if gradeBands[classifications.GradeLevel] == "" {
			return nil, fmt.Errorf("student %s has unknown grade %q", rawName, classifications.GradeLevel)
		}
		if classifications.Homeroom == "" {
			return nil, fmt.Errorf("student %s has no homeroom", rawName)
		}
		classroom, crew := splitHomeroom(classifications.Homeroom)

		email := strings.ToLower(row["student_email"])
		if mapped, ok := nameToEmail[normName(rawName)]; ok {
			email = mapped
			mappingUses[normName(rawName)]++
		}
		if email == "" {
			return nil, fmt.Errorf("student %s has no email and no name to email entry", rawName)
		}
		if _, ok := people[email]; ok {
			return nil, fmt.Errorf("student email %s appears twice", email)
		}
		n := parseName(rawName)
		student := &Person{
			Email: email, FullName: n.display, LegalName: n.legal, PreferredName: n.preferred,
			IsStudent: true, Grade: classifications.GradeLevel, Classroom: classroom, Section: crew,
			Phone: row["student_phone_mobile"],
		}
		people[email] = student
		order = append(order, email)

		for _, hn := range []string{"1", "2"} {
			adults := []string{}
			for _, pn := range []string{"1", "2"} {
				prefix := "household_" + hn + "_person_" + pn + "_"
				if row[prefix+"full_name"] == "" {
					continue
				}
				adultEmail := strings.ToLower(row[prefix+"email"])
				if adultEmail == "" {
					return nil, fmt.Errorf("adult %q of student %s has no email", row[prefix+"full_name"], rawName)
				}
				phone := row[prefix+"phone_mobile"]
				if phone == "" {
					phone = row[prefix+"phone_business"]
				}
				if err := addAdult(row[prefix+"full_name"], adultEmail, phone); err != nil {
					return nil, err
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
			hh, ok := households[setKey]
			if !ok {
				hh = &household{adults: adults, address: address, phone: phone}
				households[setKey] = hh
				householdOrder = append(householdOrder, setKey)
				for _, a := range adults {
					if len(personHouseholds[a]) > 0 {
						return nil, fmt.Errorf("adult %s belongs to more than one household", a)
					}
					personHouseholds[a] = append(personHouseholds[a], setKey)
				}
			} else if hh.address != address || hh.phone != phone {
				return nil, fmt.Errorf("household of %v has conflicting address or phone across rows", adults)
			}
			hh.kids = append(hh.kids, email)
			personHouseholds[email] = append(personHouseholds[email], setKey)
			student.ParentContactEmails = append(student.ParentContactEmails, adults...)
		}
	}

	for name := range nameToEmail {
		switch mappingUses[name] {
		case 0:
			return nil, fmt.Errorf("name to email entry %q matches no import row", name)
		case 1:
		default:
			return nil, fmt.Errorf("name to email entry %q matches %d import rows", name, mappingUses[name])
		}
	}

	type familyCells struct {
		address, phone, caption string
		hasAddress, hasPhone, hasCaption bool
	}
	familyOverrides := map[string]familyCells{}
	bandSet := map[string]bool{}
	for _, band := range gradeBands {
		bandSet[band] = true
	}
	roomParents := map[string][]string{}
	optedOut := map[string]bool{}

	seenOverride := map[string]bool{}
	for _, row := range overrideRows {
		email := strings.ToLower(row["Email"])
		if email == "" {
			return nil, fmt.Errorf("overrides row %v has no email", row)
		}
		if seenOverride[email] {
			return nil, fmt.Errorf("overrides has duplicate email %s", email)
		}
		seenOverride[email] = true
		added := row["Added"] == "TRUE"
		p, exists := people[email]
		if added && exists {
			return nil, fmt.Errorf("overrides row %s is flagged added but the import covers this person", email)
		}
		if !added && !exists {
			return nil, fmt.Errorf("overrides row %s matches no imported person", email)
		}
		if added {
			p = &Person{Email: email}
			people[email] = p
			order = append(order, email)
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
				return nil, err
			}
		}
		apply(row["Pronouns"], &p.Pronouns)
		apply(row["Facts"], &p.Facts)
		if cell := row["Grade"]; cell != "" && cell != "-" && !added && gradeBands[cell] == "" {
			return nil, fmt.Errorf("overrides row %s has unknown grade %q", email, cell)
		}
		apply(row["Grade"], &p.Grade)
		apply(row["Classroom"], &p.Classroom)
		apply(row["Crew"], &p.Section)
		apply(row["Phone"], &p.Phone)
		apply(row["Job Title"], &p.JobTitle)
		apply(row["Department"], &p.Department)
		if cell := row["Grade Band"]; cell != "" && cell != "-" && !bandSet[cell] {
			return nil, fmt.Errorf("overrides row %s has unknown grade band %q", email, cell)
		}
		apply(row["Grade Band"], &p.GradeBand)
		if cell := row["Room Parent"]; cell != "" && cell != "-" {
			if !bandSet[cell] {
				return nil, fmt.Errorf("overrides row %s has unknown room parent band %q", email, cell)
			}
			roomParents[cell] = append(roomParents[cell], email)
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
			familyOverrides[email] = cells
		}

		switch row["Opted Out"] {
		case "", "-", "FALSE":
		case "TRUE":
			optedOut[email] = true
		default:
			return nil, fmt.Errorf("overrides row %s has invalid Opted Out %q", email, row["Opted Out"])
		}

		if added {
			if p.FullName == "" {
				return nil, fmt.Errorf("added row %s has no full name", email)
			}
			if !p.IsStudent && !p.IsParent && !p.IsStaff {
				return nil, fmt.Errorf("added row %s has no role", email)
			}
		}
	}

	model := &Model{Families: map[string]Family{}, RoomParents: map[string][]string{}}

	familyKeys := map[string]string{}
	for _, setKey := range householdOrder {
		hh := households[setKey]
		members := append(append([]string{}, hh.adults...), hh.kids...)
		key := familyHash(members)
		familyKeys[setKey] = key
		model.Families[key] = Family{
			Key:         key,
			Address:     hh.address,
			Phone:       hh.phone,
			AdultEmails: hh.adults,
			KidEmails:   hh.kids,
		}
	}
	for email, sets := range personHouseholds {
		if p := people[email]; p.FamilyKey == "" {
			p.FamilyKey = familyKeys[sets[0]]
		}
	}
	for email, cells := range familyOverrides {
		p := people[email]
		if !p.IsParent {
			return nil, fmt.Errorf("overrides row %s has family cells but %s is not a parent", email, email)
		}
		sets := personHouseholds[email]
		if len(sets) != 1 {
			return nil, fmt.Errorf("overrides row %s has family cells but %s has no household", email, email)
		}
		key := familyKeys[sets[0]]
		family := model.Families[key]
		if cells.hasAddress {
			family.Address = cells.address
		}
		if cells.hasPhone {
			family.Phone = cells.phone
		}
		if cells.hasCaption {
			family.PhotoCaption = cells.caption
		}
		model.Families[key] = family
	}

	for email := range optedOut {
		delete(people, email)
	}
	kept := []string{}
	for _, email := range order {
		if !optedOut[email] {
			kept = append(kept, email)
		}
	}
	order = kept
	for key, family := range model.Families {
		family.AdultEmails = without(family.AdultEmails, optedOut)
		family.KidEmails = without(family.KidEmails, optedOut)
		if len(family.AdultEmails)+len(family.KidEmails) == 0 {
			delete(model.Families, key)
			continue
		}
		family.Name = familyNameFor(family, people)
		model.Families[key] = family
	}
	for _, p := range people {
		p.ParentContactEmails = without(p.ParentContactEmails, optedOut)
	}
	for band, emails := range roomParents {
		roomParents[band] = without(emails, optedOut)
	}

	if blobs != nil {
		for _, p := range people {
			local, _, _ := strings.Cut(p.Email, "@")
			if blobs.Has("people/" + local + "-photo") {
				p.PhotoURL = "/blob/people/" + local + "-photo"
			}
			if blobs.Has("people/" + local + "-pronunciation") {
				p.PronunciationURL = "/blob/people/" + local + "-pronunciation"
			}
		}
		for key, family := range model.Families {
			if blobs.Has("families/" + key + "-photo") {
				family.PhotoURL = "/blob/families/" + key + "-photo"
			}
			if blobs.Has("families/" + key + "-pronunciation") {
				family.PronunciationURL = "/blob/families/" + key + "-pronunciation"
			}
			model.Families[key] = family
		}
	}

	for _, email := range order {
		model.People = append(model.People, *people[email])
	}
	sort.Slice(model.People, func(i, j int) bool {
		si, sj := surname(model.People[i].FullName), surname(model.People[j].FullName)
		if si != sj {
			return si < sj
		}
		return model.People[i].FullName < model.People[j].FullName
	})

	type classroomInfo struct {
		crews    map[string]bool
		minGrade int
		bands    map[string]bool
	}
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
		if p.Section != "" {
			info.crews[p.Section] = true
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
			return nil, fmt.Errorf("classroom %s spans multiple grade bands", name)
		}
		imageURL := ""
		imagePath := "web/static/brand/classrooms/classroom-" + strings.ToLower(name) + ".jpg"
		if _, err := os.Stat(imagePath); err == nil {
			imageURL = "/static/brand/classrooms/classroom-" + strings.ToLower(name) + ".jpg"
		}
		model.Classrooms = append(model.Classrooms, Classroom{
			Name:        name,
			ImageURL:    imageURL,
			HasSections: len(info.crews) > 0,
		})
	}

	for _, p := range model.People {
		if !p.IsStaff || p.Classroom == "" {
			continue
		}
		info, ok := classrooms[p.Classroom]
		if !ok {
			return nil, fmt.Errorf("staff %s is assigned to unknown classroom %q", p.Email, p.Classroom)
		}
		if p.Section != "" && !info.crews[p.Section] {
			return nil, fmt.Errorf("staff %s is assigned to unknown crew %q of %s", p.Email, p.Section, p.Classroom)
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
		for _, crew := range crews {
			section := Section{Classroom: name, Name: crew, GradeBand: band}
			for _, p := range model.People {
				if p.IsStaff && p.Classroom == name && p.Section == crew {
					section.Teachers = append(section.Teachers, p.Email)
				}
			}
			model.Sections = append(model.Sections, section)
		}
	}

	for i, grade := range gradeOrder {
		g := Grade{Name: grade, Band: gradeBands[grade]}
		if i+1 < len(gradeOrder) {
			g.NextName = gradeOrder[i+1]
			g.NextBand = gradeBands[g.NextName]
		}
		model.Grades = append(model.Grades, g)
	}

	for band, emails := range roomParents {
		model.RoomParents[bandLabel(band)] = emails
	}

	model.Departments = append(model.Departments, departmentOrder...)

	return model, nil
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
