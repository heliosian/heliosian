package who

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type admin struct {
	cache *Cache
	media *blob.Store
}

func RegisterAdmin(mux *http.ServeMux, cache *Cache, media *blob.Store) {
	a := admin{cache: cache, media: media}
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /api/admin/state", a.state)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/images", a.setImage)
	mux.HandleFunc("POST /api/admin/super-edit", a.setSuperEdit)
	mux.HandleFunc("POST /api/admin/person-fields", a.setPersonFields)
	mux.HandleFunc("POST /api/admin/student-fields", a.setStudentFields)
	mux.HandleFunc("POST /api/admin/parent-fields", a.setParentFields)
	mux.HandleFunc("POST /api/admin/added-fields", a.setAddedFields)
	mux.HandleFunc("POST /api/admin/add-person", a.addPerson)
	mux.HandleFunc("POST /api/admin/delete-person", a.deletePerson)
	mux.HandleFunc("POST /api/admin/hide-person", a.hidePerson)
	mux.HandleFunc("POST /api/admin/unhide-person", a.unhidePerson)
}

func (a admin) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func (a admin) page(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	serve.File(w, r, "web/who/admin.html")
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
	Name      string            `json:"name"`
	Email     string            `json:"email"`
	IsStaff   bool              `json:"isStaff,omitempty"`
	IsStudent bool              `json:"isStudent,omitempty"`
	IsParent  bool              `json:"isParent,omitempty"`
	IsAdded   bool              `json:"isAdded,omitempty"`
	Override  overridableFields `json:"override"`
	Veracross overridableFields `json:"veracross"`
}

func boolCell(b bool) string {
	return map[bool]string{true: "TRUE", false: "FALSE"}[b]
}

func overrideStringValue(person *Person, column string) string {
	cell := person.overrideRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

func overrideBoolValue(person *Person, column string) bool {
	return person.overrideRow[column] == "TRUE"
}

func familyOf(model *Model, email string) Family {
	keys := model.FamilyKeysOf(email)
	if len(keys) == 0 {
		return Family{}
	}
	return model.Families[keys[0]]
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

func (a admin) state(w http.ResponseWriter, r *http.Request) {
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return
	}
	model := a.cache.Model()
	classrooms := make([]imageInfo, 0, len(model.Classrooms))
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, imageInfo{Name: c.Name, ImageURL: c.ImageURL})
	}
	grades := make([]imageInfo, 0, len(model.Grades))
	bands := make([]string, 0, len(model.Grades))
	seenBand := map[string]bool{}
	for _, g := range model.Grades {
		grades = append(grades, imageInfo{Name: g.Name, ImageURL: g.ImageURL})
		if g.Band != "" && !seenBand[g.Band] {
			seenBand[g.Band] = true
			bands = append(bands, g.Band)
		}
	}
	crews := make([]crewOption, 0, len(model.Crews))
	for _, cr := range model.Crews {
		if cr.Name == "" {
			continue
		}
		crews = append(crews, crewOption{Classroom: cr.Classroom, Name: cr.Name})
	}
	people := make([]personOption, 0, len(model.People))
	for i := range model.People {
		p := &model.People[i]
		family := familyOf(model, p.Email)
		people = append(people, personOption{
			Name: p.FullName, Email: p.Email,
			IsStaff: p.IsStaff, IsStudent: p.IsStudent, IsParent: p.IsParent,
			IsAdded: overrideBoolValue(p, "Added"),
			Override: overridableFields{
				IsStaff:       boolCell(overrideBoolValue(p, "Is Staff")),
				IsStudent:     boolCell(overrideBoolValue(p, "Is Student")),
				IsParent:      boolCell(overrideBoolValue(p, "Is Parent")),
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
				IsStaff:       importedValue(p, "Is Staff", boolCell(p.IsStaff)),
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
	sort.Slice(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	view := struct {
		Email        string         `json:"email"`
		Admins       []string       `json:"admins"`
		Classrooms   []imageInfo    `json:"classrooms"`
		Grades       []imageInfo    `json:"grades"`
		Bands        []string       `json:"bands"`
		Crews        []crewOption   `json:"crews"`
		Departments  []string       `json:"departments"`
		People       []personOption `json:"people"`
		HiddenEmails []string       `json:"hiddenEmails"`
		IsSuperAdmin bool           `json:"isSuperAdmin"`
	}{
		Email: email, Admins: a.cache.Admins(),
		Classrooms: classrooms, Grades: grades, Bands: bands, Crews: crews, Departments: model.Departments,
		People: people, HiddenEmails: model.hiddenEmails,
	}
	view.IsSuperAdmin = a.cache.IsSuperAdmin(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode admin state", "error", err)
	}
}

func (a admin) setAdmins(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Admins []string `json:"admins"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	admins := withoutSuperAdmins(config.NormalizeEmails(body.Admins), a.cache.superAdmins())
	current := a.cache.Model().admins
	ops := []store.Op{}
	for _, e := range current {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTable, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(current, e) {
			ops = append(ops, store.Insert(adminsTable, store.Row{"Email": e}))
		}
	}
	if !a.cache.commit(w, r, email, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "admin: set the admin list", "actor", email, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func withoutSuperAdmins(emails, superAdmins []string) []string {
	super := map[string]bool{}
	for _, e := range superAdmins {
		super[e] = true
	}
	out := []string{}
	for _, e := range emails {
		if !super[e] {
			out = append(out, e)
		}
	}
	return out
}

const (
	superEditCookie = "who-super-edit"
	superEditLength = 400 * 24 * 60 * 60
)

func superEdit(cache *Cache, r *http.Request, email string) bool {
	if !cache.IsAdmin(email) {
		return false
	}
	cookie, err := r.Cookie(superEditCookie)
	return err == nil && cookie.Value == "1"
}

func (a admin) setSuperEdit(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	cookie := &http.Cookie{
		Name:     superEditCookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	if body.Enabled {
		cookie.Value, cookie.MaxAge = "1", superEditLength
	}
	http.SetCookie(w, cookie)
	slog.InfoContext(r.Context(), "admin: set super edit mode", "actor", email, "enabled", body.Enabled)
	w.WriteHeader(http.StatusNoContent)
}

const (
	maxJobTitleLength = 100
	maxNameLength     = 100
	maxFactsLength    = 4000
	maxPhoneLength    = 40
	maxAddressLength  = 200
)

func (a admin) setPersonFields(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	model := a.cache.Model()
	person := model.Person(target)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if len(body.JobTitle) > maxJobTitleLength {
		http.Error(w, "job title too long", http.StatusBadRequest)
		return
	}
	if !validClassroom(model, body.Classroom) {
		http.Error(w, "unknown classroom", http.StatusBadRequest)
		return
	}
	if !validCrew(model, body.Classroom, body.Crew) {
		http.Error(w, "unknown crew for that classroom", http.StatusBadRequest)
		return
	}
	if body.Department != "" && !slices.Contains(model.Departments, body.Department) {
		http.Error(w, "unknown department", http.StatusBadRequest)
		return
	}
	if body.GradeBand != "" && !gradeBandSet()[body.GradeBand] {
		http.Error(w, "unknown grade band", http.StatusBadRequest)
		return
	}
	if err := validFullName(body.FullName, person); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.LegalName) > maxNameLength {
		http.Error(w, "bad legal name", http.StatusBadRequest)
		return
	}
	if len(body.PreferredName) > maxNameLength {
		http.Error(w, "bad preferred name", http.StatusBadRequest)
		return
	}
	if len(body.Facts) > maxFactsLength {
		http.Error(w, "bad facts", http.StatusBadRequest)
		return
	}

	cells := store.Row{}
	diffStringCellNoBaseline(cells, "Classroom", body.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCellNoBaseline(cells, "Crew", body.Crew, overrideStringValue(person, "Crew"))
	diffStringCellNoBaseline(cells, "Department", body.Department, overrideStringValue(person, "Department"))
	diffStringCell(cells, "Job Title", body.JobTitle, overrideStringValue(person, "Job Title"))
	diffStringCellNoBaseline(cells, "Grade Band", body.GradeBand, overrideStringValue(person, "Grade Band"))
	diffStringCell(cells, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffFacts(cells, body.Facts, person)

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, setOverride(target, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited person fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

func diffBoolCell(cells store.Row, column string, next, current bool) {
	if next == current {
		return
	}
	cells[column] = boolCell(next)
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

func validClassroom(model *Model, name string) bool {
	return name == "" || slices.ContainsFunc(model.Classrooms, func(c Classroom) bool { return c.Name == name })
}

func validCrew(model *Model, classroom, crew string) bool {
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
			return errors.New("full name required: this person has no Veracross record to fall back to")
		}
		return nil
	}
	if len(fullName) > maxNameLength {
		return errors.New("full name too long")
	}
	return nil
}

func (a admin) setStudentFields(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email         string `json:"email"`
		FullName      string `json:"fullName"`
		LegalName     string `json:"legalName"`
		PreferredName string `json:"preferredName"`
		Grade         string `json:"grade"`
		Classroom     string `json:"classroom"`
		Crew          string `json:"crew"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	model := a.cache.Model()
	person := model.Person(target)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if err := validFullName(body.FullName, person); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.LegalName) > maxNameLength {
		http.Error(w, "bad legal name", http.StatusBadRequest)
		return
	}
	if len(body.PreferredName) > maxNameLength {
		http.Error(w, "bad preferred name", http.StatusBadRequest)
		return
	}
	if !validGrade(body.Grade) {
		http.Error(w, "unknown grade", http.StatusBadRequest)
		return
	}
	if !validClassroom(model, body.Classroom) {
		http.Error(w, "unknown classroom", http.StatusBadRequest)
		return
	}
	if !validCrew(model, body.Classroom, body.Crew) {
		http.Error(w, "unknown crew for that classroom", http.StatusBadRequest)
		return
	}

	cells := store.Row{}
	diffStringCell(cells, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, "Grade", body.Grade, overrideStringValue(person, "Grade"))
	diffStringCell(cells, "Classroom", body.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCell(cells, "Crew", body.Crew, overrideStringValue(person, "Crew"))

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, setOverride(target, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited student fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setParentFields(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email         string `json:"email"`
		FullName      string `json:"fullName"`
		LegalName     string `json:"legalName"`
		PreferredName string `json:"preferredName"`
		Phone         string `json:"phone"`
		RoomParent    string `json:"roomParent"`
		Address       string `json:"address"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	model := a.cache.Model()
	person := model.Person(target)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if !person.IsParent {
		http.Error(w, "not a parent", http.StatusBadRequest)
		return
	}
	if err := validFullName(body.FullName, person); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.LegalName) > maxNameLength {
		http.Error(w, "bad legal name", http.StatusBadRequest)
		return
	}
	if len(body.PreferredName) > maxNameLength {
		http.Error(w, "bad preferred name", http.StatusBadRequest)
		return
	}
	if len(body.Phone) > maxPhoneLength {
		http.Error(w, "bad phone number", http.StatusBadRequest)
		return
	}
	if len(body.Address) > maxAddressLength {
		http.Error(w, "bad address", http.StatusBadRequest)
		return
	}
	if body.RoomParent != "" && !gradeBandSet()[body.RoomParent] {
		http.Error(w, "unknown room parent band", http.StatusBadRequest)
		return
	}

	cells := store.Row{}
	diffStringCell(cells, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, "Phone", body.Phone, overrideStringValue(person, "Phone"))
	diffStringCellNoBaseline(cells, "Room Parent", body.RoomParent, overrideStringValue(person, "Room Parent"))

	family := familyOf(model, person.Email)
	familyCells := store.Row{}
	if family.Key != "" {
		diffStringCell(familyCells, "Address", body.Address, familyStringValue(family, "Address"))
	}

	ops := []store.Op{}
	if len(cells) > 0 {
		ops = append(ops, setOverride(target, cells))
	}
	if len(familyCells) > 0 {
		ops = append(ops, setFamily(family.email, familyCells))
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited parent fields", "actor", actor, "target", target, "cells", cells, "familyCells", familyCells)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setAddedFields(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email     string `json:"email"`
		NewEmail  string `json:"newEmail"`
		FullName  string `json:"fullName"`
		IsStudent bool   `json:"isStudent"`
		IsParent  bool   `json:"isParent"`
		IsStaff   bool   `json:"isStaff"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	model := a.cache.Model()
	person := model.Person(target)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if !overrideBoolValue(person, "Added") {
		http.Error(w, "not an added-only person", http.StatusBadRequest)
		return
	}
	if err := validFullName(body.FullName, person); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !body.IsStudent && !body.IsParent && !body.IsStaff {
		http.Error(w, "choose at least one of Is Student, Is Parent, or Is Staff", http.StatusBadRequest)
		return
	}
	newEmail := strings.ToLower(strings.TrimSpace(body.NewEmail))
	if !strings.Contains(newEmail, "@") {
		http.Error(w, "bad email address", http.StatusBadRequest)
		return
	}
	if newEmail != target && model.Person(newEmail) != nil {
		http.Error(w, "a person with this email already exists", http.StatusBadRequest)
		return
	}

	cells := store.Row{}
	diffStringCell(cells, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffBoolCell(cells, "Is Student", body.IsStudent, overrideBoolValue(person, "Is Student"))
	diffBoolCell(cells, "Is Parent", body.IsParent, overrideBoolValue(person, "Is Parent"))
	diffBoolCell(cells, "Is Staff", body.IsStaff, overrideBoolValue(person, "Is Staff"))
	if newEmail != target {
		cells["Email"] = newEmail
	}

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, store.Update(overridesTab, store.Row{"Email": target}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited added-person fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) addPerson(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email     string `json:"email"`
		FullName  string `json:"fullName"`
		IsStudent bool   `json:"isStudent"`
		IsParent  bool   `json:"isParent"`
		IsStaff   bool   `json:"isStaff"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if !strings.Contains(email, "@") {
		http.Error(w, "bad email address", http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	if model.Person(email) != nil {
		http.Error(w, "a person with this email already exists", http.StatusBadRequest)
		return
	}
	fullName := strings.TrimSpace(body.FullName)
	if fullName == "" || len(fullName) > maxNameLength {
		http.Error(w, "bad full name", http.StatusBadRequest)
		return
	}
	if !body.IsStudent && !body.IsParent && !body.IsStaff {
		http.Error(w, "choose at least one of Is Student, Is Parent, or Is Staff", http.StatusBadRequest)
		return
	}

	cells := store.Row{"Added": "TRUE", "Full Name": fullName}
	if body.IsStudent {
		cells["Is Student"] = "TRUE"
	}
	if body.IsParent {
		cells["Is Parent"] = "TRUE"
	}
	if body.IsStaff {
		cells["Is Staff"] = "TRUE"
	}
	if !a.cache.commit(w, r, actor, setOverride(email, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "admin: added a new person", "actor", actor, "email", email, "name", fullName)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) deletePerson(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	person := a.cache.Model().Person(target)
	if person == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if !overrideBoolValue(person, "Added") {
		http.Error(w, "not an added-only person", http.StatusBadRequest)
		return
	}
	if !a.cache.commit(w, r, actor, store.Delete(overridesTab, store.Row{"Email": target})) {
		return
	}
	slog.InfoContext(r.Context(), "admin: deleted added person", "actor", actor, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) hidePerson(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	if a.cache.Model().Person(target) == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	if !a.cache.commit(w, r, actor, setOverride(target, store.Row{"Opted Out": "TRUE"})) {
		return
	}
	slog.InfoContext(r.Context(), "admin: hid person from the directory", "actor", actor, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) unhidePerson(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	target := strings.ToLower(strings.TrimSpace(body.Email))
	if !slices.Contains(a.cache.Model().hiddenEmails, target) {
		http.Error(w, "not currently hidden", http.StatusBadRequest)
		return
	}
	if !a.cache.commit(w, r, actor, store.Update(overridesTab, store.Row{"Email": target}, store.Row{"Opted Out": ""})) {
		return
	}
	slog.InfoContext(r.Context(), "admin: unhid person from the directory", "actor", actor, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

var imageExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

func (a admin) setImage(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	name := strings.TrimSpace(r.FormValue("name"))
	model := a.cache.Model()
	switch kind {
	case imageClassroom:
		if !slices.ContainsFunc(model.Classrooms, func(c Classroom) bool { return c.Name == name }) {
			http.Error(w, "no such classroom", http.StatusBadRequest)
			return
		}
	case imageGrade:
		if !slices.Contains(gradeOrder, name) {
			http.Error(w, "no such grade", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, "bad kind: must be classroom or grade", http.StatusBadRequest)
		return
	}

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
	ext, ok := imageExtensions[sniffed]
	if !ok {
		http.Error(w, "unsupported image type "+sniffed, http.StatusBadRequest)
		return
	}
	image := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)
	if err := a.media.Put("photos", image, sniffed, content); err != nil {
		serverError(w, r, err)
		return
	}
	if !a.cache.commit(w, r, email, store.Set(imagesTab, store.Row{imageKind: kind, imageName: name}, store.Row{imageImage: image})) {
		return
	}
	slog.InfoContext(r.Context(), "admin: replaced image", "actor", email, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}
