package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	studentsFile = "All Students Directory.csv"
	staffFile    = "All Faculty & Staff Directory.csv"
	websiteFile  = "Staff Bios.csv"
)

type role string

const (
	student role = "student"
	parent  role = "parent"
	staff   role = "staff"
)

type entry struct {
	role      role
	name      string
	emails    []string
	grade     string
	classroom string
	crew      string
	jobTitle  string
	phone     string
	bio       string
	photos    []string
}

type householdRow struct {
	adults  []int
	kid     int
	address string
	phone   string
}

type export struct {
	entries    []entry
	households []householdRow
	photoDir   string
}

var gradeCodes = map[string]string{
	"Kindergarten": "K",
	"Grade 1":      "1",
	"Grade 2":      "2",
	"Grade 3":      "3",
	"Grade 4":      "4",
	"Grade 5":      "5",
	"Grade 6":      "6",
	"Grade 7":      "7",
	"Grade 8":      "8",
}

type adultColumns struct {
	name, email, email2, mobile, business string
}

var householdColumns = []struct {
	address, phone string
	adults         []adultColumns
}{
	{"household_1_address", "household_1_phone", []adultColumns{
		{"household_1_person_1_full_name", "household_1_person_1_email", "household_1_person_1_email_2", "household_1_person_1_phone_mobile", "household_1_person_1_phone_business"},
		{"household_1_person_2_full_name", "household_1_person_2_email", "household_1_person_2_email_2", "household_1_person_2_phone_mobile", "household_1_person_2_phone_business"},
	}},
	{"household_2_address", "household_2_phone", []adultColumns{
		{"household_2_person_1_full_name", "household_2_person_1_email", "household_2_person_1_email_2", "household_2_person_1_phone_mobile", "household_2_person_1_phone_business"},
		{"household_2_person_2_full_name", "household_2_person_2_email", "household_2_person_2_email_2", "household_2_person_2_phone_mobile", "household_2_person_2_phone_business"},
	}},
}

var excludedFacultyTypes = map[string]bool{"Vendors": true}

const noEmailMarker = ".noemail"

func clean(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func norm(s string) string {
	return strings.ToLower(clean(s))
}

func emails(cells ...string) []string {
	out := []string{}
	for _, cell := range cells {
		a := strings.ToLower(strings.TrimSpace(cell))
		if a == "" || strings.Contains(a, noEmailMarker) || slices.Contains(out, a) {
			continue
		}
		out = append(out, a)
	}
	return out
}

func photoFiles(cell string) []string {
	if name := strings.TrimSpace(cell); name != "" {
		return []string{name}
	}
	return []string{}
}

type names struct {
	long, legal, short, sort string
}

var nameForm = regexp.MustCompile(`^(.+?) \((.+?)\) (.+)$`)

func parseName(raw string) names {
	raw = clean(raw)
	if m := nameForm.FindStringSubmatch(raw); m != nil {
		return names{long: m[1] + " " + m[3], legal: m[2] + " " + m[3], short: m[1], sort: m[3] + ", " + m[1]}
	}
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return names{long: raw, legal: raw, short: raw, sort: raw}
	}
	last := fields[len(fields)-1]
	return names{long: raw, legal: raw, short: fields[0], sort: last + ", " + strings.Join(fields[:len(fields)-1], " ")}
}

func resolved(raw string) string {
	return norm(parseName(raw).long)
}

func splitHomeroom(homeroom string) (classroom, crew string) {
	fields := strings.Fields(homeroom)
	if len(fields) == 1 {
		return homeroom, ""
	}
	return fields[len(fields)-1], strings.Join(fields[:len(fields)-1], " ")
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("%s has no data rows", path)
	}
	rows := []map[string]string{}
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, name := range records[0] {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func readExport(dir string) (*export, error) {
	x := &export{photoDir: filepath.Join(dir, "photos")}
	if err := x.readStudents(filepath.Join(dir, studentsFile)); err != nil {
		return nil, err
	}
	if err := x.readStaff(filepath.Join(dir, staffFile)); err != nil {
		return nil, err
	}
	if err := x.readWebsite(filepath.Join(dir, websiteFile)); err != nil {
		return nil, err
	}
	return x, nil
}

func (x *export) readStudents(path string) error {
	rows, err := readCSV(path)
	if err != nil {
		return err
	}
	for _, row := range rows {
		name := clean(row["student_full_name"])
		if name == "" {
			return fmt.Errorf("student row %v has no name", row)
		}
		var classifications struct {
			GradeLevel string `json:"grade_level"`
			Homeroom   string `json:"homeroom"`
		}
		if err := json.Unmarshal([]byte(row["student_classifications"]), &classifications); err != nil {
			return fmt.Errorf("student %s classifications: %w", name, err)
		}
		grade, ok := gradeCodes[classifications.GradeLevel]
		if !ok {
			return fmt.Errorf("student %s has unknown grade %q", name, classifications.GradeLevel)
		}
		if classifications.Homeroom == "" {
			return fmt.Errorf("student %s has no homeroom", name)
		}
		classroom, crew := splitHomeroom(classifications.Homeroom)
		kid := len(x.entries)
		x.entries = append(x.entries, entry{
			role: student, name: name, emails: emails(row["student_email"]),
			grade: grade, classroom: classroom, crew: crew, photos: photoFiles(row["student_photo"]),
		})
		for _, hc := range householdColumns {
			adults := []int{}
			for _, ac := range hc.adults {
				adult := clean(row[ac.name])
				if adult == "" {
					continue
				}
				phone := strings.TrimSpace(row[ac.mobile])
				if phone == "" {
					phone = strings.TrimSpace(row[ac.business])
				}
				adults = append(adults, len(x.entries))
				x.entries = append(x.entries, entry{role: parent, name: adult, emails: emails(row[ac.email], row[ac.email2]), phone: phone})
			}
			if len(adults) == 0 {
				continue
			}
			x.households = append(x.households, householdRow{
				adults: adults, kid: kid,
				address: strings.TrimSpace(row[hc.address]), phone: strings.TrimSpace(row[hc.phone]),
			})
		}
	}
	return nil
}

func (x *export) readStaff(path string) error {
	rows, err := readCSV(path)
	if err != nil {
		return err
	}
	for _, row := range rows {
		name := clean(row["person_full_name"])
		if name == "" {
			return fmt.Errorf("staff row %v has no name", row)
		}
		var classifications struct {
			FacultyType string `json:"faculty_type"`
		}
		if raw := row["person_classifications"]; raw != "" {
			if err := json.Unmarshal([]byte(raw), &classifications); err != nil {
				return fmt.Errorf("staff %s classifications: %w", name, err)
			}
		}
		if excludedFacultyTypes[classifications.FacultyType] {
			continue
		}
		x.entries = append(x.entries, entry{
			role: staff, name: name, emails: emails(row["person_email"], row["person_email_2"]),
			jobTitle: clean(row["person_job_title"]), phone: strings.TrimSpace(row["person_phone_business"]),
			photos: photoFiles(row["person_photo"]),
		})
	}
	return nil
}

func (x *export) readWebsite(path string) error {
	rows, err := readCSV(path)
	if err != nil {
		return err
	}
	byEmail := map[string]int{}
	byName := map[string][]int{}
	for i, e := range x.entries {
		if e.role != staff {
			continue
		}
		for _, a := range e.emails {
			byEmail[a] = i
		}
		byName[resolved(e.name)] = append(byName[resolved(e.name)], i)
	}
	seen := map[int]bool{}
	unmatched := []string{}
	for _, row := range rows {
		name := clean(row["full_name"])
		i, ok := -1, false
		if found := emails(row["email"]); len(found) > 0 {
			i, ok = byEmail[found[0]]
		}
		if !ok {
			switch candidates := byName[norm(name)]; len(candidates) {
			case 0:
				unmatched = append(unmatched, name)
				continue
			case 1:
				i = candidates[0]
			default:
				return fmt.Errorf("the staff page's entry for %s has no email the staff export holds, and %d staff share the name", name, len(candidates))
			}
		}
		if seen[i] {
			return fmt.Errorf("the staff page has two entries for %s", x.entries[i].name)
		}
		seen[i] = true
		bio, err := flattenBio(row["bio_html"])
		if err != nil {
			return fmt.Errorf("flatten the bio of %s: %w", name, err)
		}
		x.entries[i].bio = bio
		x.entries[i].photos = append(x.entries[i].photos, photoFiles(row["photo"])...)
		if x.entries[i].jobTitle == "" {
			x.entries[i].jobTitle = clean(row["title"])
		}
	}
	slog.Info("website staff page", "entries", len(rows), "matched", len(seen), "unmatched", unmatched)
	return nil
}
