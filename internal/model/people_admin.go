package model

import (
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type admin struct {
	store *Store
	media *blob.Store
}

func registerPeopleAdmin(mux *http.ServeMux, s *Store, media *blob.Store) {
	a := admin{store: s, media: media}
	RegisterAdmins(mux, s, "who", a.state)
	mux.HandleFunc("POST /api/admin/images", a.setImage)
	mux.HandleFunc("POST /api/admin/person-fields", serve.JSON(a.setPersonFields))
	mux.HandleFunc("POST /api/admin/student-fields", serve.JSON(a.setStudentFields))
	mux.HandleFunc("POST /api/admin/parent-fields", serve.JSON(a.setParentFields))
	mux.HandleFunc("POST /api/admin/added-fields", serve.JSON(a.setAddedFields))
	mux.HandleFunc("POST /api/admin/add-person", serve.JSON(a.addPerson))
	mux.HandleFunc("POST /api/admin/delete-person", serve.JSON(a.deletePerson))
	mux.HandleFunc("POST /api/admin/hide-person", serve.JSON(a.hidePerson))
	mux.HandleFunc("POST /api/admin/unhide-person", serve.JSON(a.unhidePerson))
}

func requestActor(s *Store, r *http.Request) access.Actor {
	return s.Model().actor(r, "who")
}

type imageInfo struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type overridableFields struct {
	IsStaff       string `json:"isStaff,omitempty"`
	IsStudent     string `json:"isStudent,omitempty"`
	IsParent      string `json:"isParent,omitempty"`
	Classroom     string `json:"classroom,omitempty"`
	Crew          string `json:"crew,omitempty"`
	Department    string `json:"department,omitempty"`
	JobTitle      string `json:"jobTitle,omitempty"`
	Grade         string `json:"grade,omitempty"`
	GradeBand     string `json:"gradeBand,omitempty"`
	FullName      string `json:"fullName,omitempty"`
	LegalName     string `json:"legalName,omitempty"`
	PreferredName string `json:"preferredName,omitempty"`
	Facts         string `json:"facts,omitempty"`
	Phone         string `json:"phone,omitempty"`
	RoomParent    string `json:"roomParent,omitempty"`
	Address       string `json:"address,omitempty"`
}

type personOption struct {
	FullName  string            `json:"fullName"`
	Email     string            `json:"email"`
	IsStaff   bool              `json:"isStaff,omitempty"`
	IsStudent bool              `json:"isStudent,omitempty"`
	IsParent  bool              `json:"isParent,omitempty"`
	IsAdded   bool              `json:"isAdded,omitempty"`
	Override  overridableFields `json:"override"`
	Veracross overridableFields `json:"veracross"`
}

func overrideStringValue(person *Person, column string) string {
	cell := person.overrideRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

func overrideBoolValue(person *Person, column string) bool {
	value, _ := cells.YesNo(person.overrideRow[column], false)
	return value
}

func familyStringValue(family Family, column string) string {
	cell := family.sheetRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

func importedValue(person *Person, column, resolved string) string {
	if person.imported != nil {
		return person.imported[column]
	}
	return resolved
}

func fullNameFallback(person *Person) string {
	return importedValue(person, "Full Name", person.FullName)
}

type crewOption struct {
	Classroom string `json:"classroom"`
	Name      string `json:"name"`
}

func (a admin) state(m *Model, _ *http.Request, _ access.Actor) map[string]any {
	model := m.Directory
	classrooms := []imageInfo{}
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, imageInfo{Name: c.Name, ImageURL: c.ImageURL})
	}
	grades := []imageInfo{}
	bands := []string{}
	seenBand := map[string]bool{}
	for _, g := range model.Grades {
		grades = append(grades, imageInfo{Name: g.Name, ImageURL: g.ImageURL})
		if g.Band != "" && !seenBand[g.Band] {
			seenBand[g.Band] = true
			bands = append(bands, g.Band)
		}
	}
	crews := []crewOption{}
	for _, cr := range model.Crews {
		if cr.Name == "" {
			continue
		}
		crews = append(crews, crewOption{Classroom: cr.Classroom, Name: cr.Name})
	}
	people := []personOption{}
	for i := range model.People {
		p := &model.People[i]
		family, _ := model.FamilyOf(p.Email)
		people = append(people, personOption{
			FullName: p.FullName, Email: p.Email,
			IsStaff: p.IsStaff, IsStudent: p.IsStudent, IsParent: p.IsParent,
			IsAdded: overrideBoolValue(p, "Added"),
			Override: overridableFields{
				IsStaff:       cells.YesNoCell(overrideBoolValue(p, "Is Staff")),
				IsStudent:     cells.YesNoCell(overrideBoolValue(p, "Is Student")),
				IsParent:      cells.YesNoCell(overrideBoolValue(p, "Is Parent")),
				Classroom:     overrideStringValue(p, "Classroom"),
				Crew:          overrideStringValue(p, "Crew"),
				Department:    overrideStringValue(p, "Department"),
				JobTitle:      overrideStringValue(p, "Job Title"),
				Grade:         overrideStringValue(p, "Grade"),
				GradeBand:     overrideStringValue(p, "Grade Band"),
				FullName:      overrideStringValue(p, "Full Name"),
				LegalName:     overrideStringValue(p, "Legal Name"),
				PreferredName: overrideStringValue(p, "Preferred Name"),
				Facts:         overrideStringValue(p, "Facts"),
				Phone:         overrideStringValue(p, "Phone"),
				RoomParent:    overrideStringValue(p, "Room Parent"),
				Address:       familyStringValue(family, "Address"),
			},
			Veracross: overridableFields{
				IsStaff:       importedValue(p, "Is Staff", cells.YesNoCell(p.IsStaff)),
				Classroom:     importedValue(p, "Classroom", p.Classroom),
				Crew:          importedValue(p, "Crew", p.Crew),
				JobTitle:      importedValue(p, "Job Title", p.JobTitle),
				Grade:         importedValue(p, "Grade", p.Grade),
				FullName:      importedValue(p, "Full Name", p.FullName),
				LegalName:     importedValue(p, "Legal Name", p.LegalName),
				PreferredName: importedValue(p, "Preferred Name", p.PreferredName),
				Phone:         importedValue(p, "Phone", p.Phone),
				Address:       family.importedAddress,
			},
		})
	}
	sort.Slice(people, func(i, j int) bool { return people[i].FullName < people[j].FullName })
	return map[string]any{
		"classrooms": classrooms, "grades": grades, "bands": bands, "crews": crews, "departments": model.Departments,
		"people": people, "hiddenEmails": model.hiddenEmails,
	}
}

const (
	maxJobTitleLength = 100
	maxNameLength     = 100
	maxFactsLength    = 4000
	maxPhoneLength    = 40
	maxAddressLength  = 200
)

type personFields struct {
	Email         string `json:"email"`
	Classroom     string `json:"classroom"`
	Crew          string `json:"crew"`
	Department    string `json:"department"`
	JobTitle      string `json:"jobTitle"`
	GradeBand     string `json:"gradeBand"`
	FullName      string `json:"fullName"`
	LegalName     string `json:"legalName"`
	PreferredName string `json:"preferredName"`
	Facts         string `json:"facts"`
}

func (a admin) setPersonFields(r *http.Request, body personFields) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, cells, ops, err := a.store.Model().Directory.setPersonFields(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:edited person fields", "actor", actor.Email, "target", target, "cells", cells)
	return serve.None{}, nil
}

func diffBoolCell(row store.Row, column string, next, current bool) {
	if next == current {
		return
	}
	row[column] = cells.YesNoCell(next)
}

func diffStringCell(cells store.Row, column string, next, current string) {
	trimmed := strings.TrimSpace(next)
	if next != "" && trimmed == "" {
		cells[column] = "-"
		return
	}
	if trimmed == current {
		return
	}
	cells[column] = trimmed
}

func diffStringCellNoBaseline(cells store.Row, column string, next, current string) {
	next = strings.TrimSpace(next)
	if next == current {
		return
	}
	cells[column] = next
}

func diffFacts(cells store.Row, next string, person *Person) {
	next = strings.TrimSpace(next)
	if next == overrideStringValue(person, "Facts") {
		return
	}
	cells["Facts"] = next
	cells["Facts Updated"] = today()
}

func validClassroom(model *Directory, name string) bool {
	return name == "" || slices.ContainsFunc(model.Classrooms, func(c Classroom) bool { return c.Name == name })
}

func validCrew(model *Directory, classroom, crew string) bool {
	if crew == "" {
		return true
	}
	if classroom == "" {
		return slices.ContainsFunc(model.Crews, func(c Crew) bool { return c.Name == crew })
	}
	return slices.ContainsFunc(model.Crews, func(c Crew) bool { return c.Classroom == classroom && c.Name == crew })
}

func validGrade(grade string) bool {
	return grade == "" || slices.Contains(gradeOrder, grade)
}

func gradeBandSet() map[string]bool {
	set := map[string]bool{}
	for _, band := range gradeBands {
		set[band] = true
	}
	return set
}

func validFullName(fullName string, person *Person) error {
	if strings.TrimSpace(fullName) == "" {
		if fullNameFallback(person) == "" {
			return access.Invalid("full name required: this person has no Veracross record to fall back to")
		}
		return nil
	}
	if len(fullName) > maxNameLength {
		return access.Invalid("full name too long")
	}
	return nil
}

type studentFields struct {
	Email         string `json:"email"`
	FullName      string `json:"fullName"`
	LegalName     string `json:"legalName"`
	PreferredName string `json:"preferredName"`
	Grade         string `json:"grade"`
	Classroom     string `json:"classroom"`
	Crew          string `json:"crew"`
}

func (a admin) setStudentFields(r *http.Request, body studentFields) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, cells, ops, err := a.store.Model().Directory.setStudentFields(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:edited student fields", "actor", actor.Email, "target", target, "cells", cells)
	return serve.None{}, nil
}

type parentFields struct {
	Email         string `json:"email"`
	FullName      string `json:"fullName"`
	LegalName     string `json:"legalName"`
	PreferredName string `json:"preferredName"`
	Phone         string `json:"phone"`
	RoomParent    string `json:"roomParent"`
	Address       string `json:"address"`
}

func (a admin) setParentFields(r *http.Request, body parentFields) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, cells, familyCells, ops, err := a.store.Model().Directory.setParentFields(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:edited parent fields", "actor", actor.Email, "target", target, "cells", cells, "familyCells", familyCells)
	return serve.None{}, nil
}

type addedFields struct {
	Email     string `json:"email"`
	NewEmail  string `json:"newEmail"`
	FullName  string `json:"fullName"`
	IsStudent bool   `json:"isStudent"`
	IsParent  bool   `json:"isParent"`
	IsStaff   bool   `json:"isStaff"`
}

func (a admin) setAddedFields(r *http.Request, body addedFields) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, cells, ops, err := a.store.Model().Directory.setAddedFields(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:edited added-person fields", "actor", actor.Email, "target", target, "cells", cells)
	return serve.None{}, nil
}

type newPerson struct {
	Email     string `json:"email"`
	FullName  string `json:"fullName"`
	IsStudent bool   `json:"isStudent"`
	IsParent  bool   `json:"isParent"`
	IsStaff   bool   `json:"isStaff"`
}

func (a admin) addPerson(r *http.Request, body newPerson) (serve.None, error) {
	actor := requestActor(a.store, r)
	email, fullName, ops, err := a.store.Model().Directory.addPerson(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:added a new person", "actor", actor.Email, "email", email, "name", fullName)
	return serve.None{}, nil
}

type personEmail struct {
	Email string `json:"email"`
}

func (a admin) deletePerson(r *http.Request, body personEmail) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, ops, err := a.store.Model().Directory.deletePerson(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:deleted added person", "actor", actor.Email, "target", target)
	return serve.None{}, nil
}

func (a admin) hidePerson(r *http.Request, body personEmail) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, ops, err := a.store.Model().Directory.hidePerson(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:hid person from the directory", "actor", actor.Email, "target", target)
	return serve.None{}, nil
}

func (a admin) unhidePerson(r *http.Request, body personEmail) (serve.None, error) {
	actor := requestActor(a.store, r)
	target, ops, err := a.store.Model().Directory.unhidePerson(actor, body.Email)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "who:unhid person from the directory", "actor", actor.Email, "target", target)
	return serve.None{}, nil
}

func (a admin) setImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	name := strings.TrimSpace(r.FormValue("name"))
	file, _, err := r.FormFile("file")
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
	sniffed := http.DetectContentType(content)
	ext, ok := blob.ImageExtensions[sniffed]
	if !ok {
		http.Error(w, "unsupported image type "+sniffed, http.StatusBadRequest)
		return
	}
	image := blob.Name(content, ext)
	actor := requestActor(a.store, r)
	ops, err := a.store.Model().Directory.setImage(actor, kind, name, image)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := a.media.Put("photos", image, sniffed, content); err != nil {
		serve.Error(w, r, err)
		return
	}
	if err := a.store.Commit(r.Context(), actor, DirectoryApp, ops...); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "who:replaced image", "actor", actor.Email, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}
