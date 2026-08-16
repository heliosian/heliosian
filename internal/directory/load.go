package directory

import (
	"sort"
	"strconv"
	"strings"

	"heliosian/internal/data"
)

const appName = "directory"

func LoadModel(source data.Source) (*Model, error) {
	tables := map[string][]map[string]string{}
	for _, name := range []string{"Basic Directory", "Staff Details", "Classrooms", "Schedules", "Grade Lookup", "Room Parents", "Departments"} {
		rows, err := source.Table(appName, name)
		if err != nil {
			return nil, err
		}
		tables[name] = rows
	}

	model := &Model{Families: map[string]Family{}, RoomParents: map[string][]string{}}

	staffByEmail := map[string]map[string]string{}
	for _, row := range tables["Staff Details"] {
		email := strings.ToLower(row["Email Lower"])
		if email != "" {
			staffByEmail[email] = row
		}
	}

	type familyAcc struct {
		family     Family
		hasParent  bool
		hasStudent bool
	}
	families := map[string]*familyAcc{}
	byEmail := map[string]Person{}
	for _, row := range tables["Basic Directory"] {
		email := strings.ToLower(row["Email Lower"])
		if email == "" {
			continue
		}
		p := Person{
			Email:            email,
			FullName:         row["Full Name"],
			LegalName:        row["Legal Name"],
			PreferredName:    row["Preferred Name"],
			IsStaff:          row["Is Staff?"] == "TRUE",
			IsParent:         row["Is Parent?"] == "TRUE",
			IsStudent:        row["Is Student?"] == "TRUE",
			Pronouns:         row["Pronouns"],
			Facts:            row["Facts"],
			PronunciationURL: blobURL("people", email, "pronunciation", row["Pronunciation"]),
			PhotoURL:         blobURL("people", email, "photo", row["Primary Photo"]),
			Grade:            row["Grade"],
			Classroom:        row["Class"],
			Section:          row["Section"],
			Phone:            row["Phone Number"],
			FamilyKey:        strings.ToLower(row["Family Key"]),
		}
		for _, contact := range strings.Split(row["Parent Contact Emails"], ",") {
			if contact = strings.ToLower(strings.TrimSpace(contact)); contact != "" {
				p.ParentContactEmails = append(p.ParentContactEmails, contact)
			}
		}
		if details, ok := staffByEmail[email]; ok {
			p.JobTitle = details["Job Title"]
			p.Department = details["Department"]
			p.GradeBand = details["Grade Band"]
		}
		model.People = append(model.People, p)
		byEmail[email] = p

		if p.FamilyKey == "" {
			continue
		}
		acc, ok := families[p.FamilyKey]
		if !ok {
			acc = &familyAcc{family: Family{Key: p.FamilyKey}}
			families[p.FamilyKey] = acc
		}
		if acc.family.Address == "" && row["Address 1"] != "" {
			acc.family.Address = row["Address 1"]
			if row["Address 2"] != "" {
				acc.family.Address += ", " + row["Address 2"]
			}
		}
		if acc.family.PhotoURL == "" {
			acc.family.PhotoURL = blobURL("families", p.FamilyKey, "photo", row["Family Photo"])
		}
		if acc.family.PhotoCaption == "" {
			acc.family.PhotoCaption = row["Family Photo Description"]
		}
		if acc.family.PronunciationURL == "" {
			acc.family.PronunciationURL = blobURL("families", p.FamilyKey, "pronunciation", row["Family Pronunciation"])
		}
		if p.IsParent {
			acc.hasParent = true
			acc.family.AdultEmails = append(acc.family.AdultEmails, email)
		}
		if p.IsStudent {
			acc.hasStudent = true
			acc.family.KidEmails = append(acc.family.KidEmails, email)
		}
	}
	for key, acc := range families {
		if !acc.hasParent && !acc.hasStudent {
			continue
		}
		acc.family.Name = familyName(acc.family, byEmail)
		model.Families[key] = acc.family
	}

	sort.Slice(model.People, func(i, j int) bool {
		si, sj := surname(model.People[i].FullName), surname(model.People[j].FullName)
		if si != sj {
			return si < sj
		}
		return model.People[i].FullName < model.People[j].FullName
	})

	for _, row := range tables["Classrooms"] {
		if row["Class"] == "" {
			continue
		}
		imageURL := ""
		if row["Classroom Image"] != "" {
			imageURL = "/static/brand/classrooms/classroom-" + strings.ToLower(row["Class"]) + ".jpg"
		}
		model.Classrooms = append(model.Classrooms, Classroom{
			Name:        row["Class"],
			ImageURL:    imageURL,
			HasSections: row["Has Sections"] == "TRUE",
		})
	}

	for _, row := range tables["Schedules"] {
		if row["Classroom"] == "" {
			continue
		}
		section := Section{Classroom: row["Classroom"], Name: row["Section"], GradeBand: row["Grade Band"]}
		for _, column := range []string{"Teacher 1", "Teacher 2", "Teacher 3"} {
			if row[column] != "" {
				section.Teachers = append(section.Teachers, row[column])
			}
		}
		model.Sections = append(model.Sections, section)
	}

	for _, row := range tables["Grade Lookup"] {
		if row["Current Grade"] == "" {
			continue
		}
		model.Grades = append(model.Grades, Grade{
			Name:     row["Current Grade"],
			NextName: row["Next Grade"],
			Band:     row["Current Gradeband"],
			NextBand: row["Next Gradeband"],
		})
	}

	for _, row := range tables["Room Parents"] {
		band, email := row["Gradeband"], strings.ToLower(row["Email Address"])
		if band == "" || email == "" {
			continue
		}
		model.RoomParents[band] = append(model.RoomParents[band], email)
	}

	type department struct {
		name  string
		order float64
	}
	departments := []department{}
	for _, row := range tables["Departments"] {
		if row["Department"] == "" {
			continue
		}
		order, err := strconv.ParseFloat(row["Order"], 64)
		if err != nil {
			order = float64(len(departments))
		}
		departments = append(departments, department{name: row["Department"], order: order})
	}
	sort.SliceStable(departments, func(i, j int) bool { return departments[i].order < departments[j].order })
	for _, d := range departments {
		model.Departments = append(model.Departments, d.name)
	}

	return model, nil
}

func blobURL(folder, email, kind, source string) string {
	if source == "" {
		return ""
	}
	local, _, _ := strings.Cut(email, "@")
	return "/blob/" + folder + "/" + local + "-" + kind
}

func surname(fullName string) string {
	fields := strings.Fields(fullName)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func familyName(f Family, byEmail map[string]Person) string {
	members := append(append([]string{}, f.KidEmails...), f.AdultEmails...)
	seen := map[string]bool{}
	names := []string{}
	for _, email := range members {
		s := surname(byEmail[email].FullName)
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
