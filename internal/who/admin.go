package who

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

type admin struct {
	cache *Cache
	media *blob.Store
}

func RegisterAdmin(mux *http.ServeMux, cache *Cache, media *blob.Store) {
	a := admin{cache: cache, media: media}
	mux.HandleFunc("GET /api/admin/state", a.state)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/images", a.setImage)
	mux.HandleFunc("POST /api/admin/person-fields", a.setPersonFields)
	mux.HandleFunc("POST /api/admin/student-fields", a.setStudentFields)
	mux.HandleFunc("POST /api/admin/parent-fields", a.setParentFields)
	mux.HandleFunc("POST /api/admin/added-fields", a.setAddedFields)
	mux.HandleFunc("POST /api/admin/add-person", a.addPerson)
	mux.HandleFunc("POST /api/admin/delete-person", a.deletePerson)
	mux.HandleFunc("POST /api/admin/hide-person", a.hidePerson)
	mux.HandleFunc("POST /api/admin/unhide-person", a.unhidePerson)
}

func actorOf(cache *Cache, email string) access.Actor {
	return access.Actor{Email: email, Admin: cache.IsAdmin(email), Household: cache.Model().Family(email)}
}

func requestActor(cache *Cache, r *http.Request) access.Actor {
	return actorOf(cache, effectiveEmail(cache, r))
}

func (a admin) requireAdmin(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	v := requestActor(a.cache, r)
	if err := mayAdminister(v); err != nil {
		refuse(w, err)
		return v, false
	}
	return v, true
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
	v, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	email := v.Email
	model := a.cache.Model()
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

func decodeBody(w http.ResponseWriter, r *http.Request, limit int64, body any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, limit)).Decode(body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (a admin) setAdmins(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decodeBody(w, r, 8<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	ops, admins, err := a.cache.setAdmins(actor, body.Admins)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:set the admin list", "actor", actor.Email, "admins", admins)
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

const superEditCookie = "heliosian-super-edit"

func superEditOn(r *http.Request) bool {
	cookie, err := r.Cookie(superEditCookie)
	return err == nil && cookie.Value == "1"
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

func (a admin) setPersonFields(w http.ResponseWriter, r *http.Request) {
	var body personFields
	if !decodeBody(w, r, 8<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, cells, ops, err := a.cache.Model().setPersonFields(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:edited person fields", "actor", actor.Email, "target", target, "cells", cells)
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

func (a admin) setStudentFields(w http.ResponseWriter, r *http.Request) {
	var body studentFields
	if !decodeBody(w, r, 8<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, cells, ops, err := a.cache.Model().setStudentFields(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:edited student fields", "actor", actor.Email, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
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

func (a admin) setParentFields(w http.ResponseWriter, r *http.Request) {
	var body parentFields
	if !decodeBody(w, r, 8<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, cells, familyCells, ops, err := a.cache.Model().setParentFields(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:edited parent fields", "actor", actor.Email, "target", target, "cells", cells, "familyCells", familyCells)
	w.WriteHeader(http.StatusNoContent)
}

type addedFields struct {
	Email     string `json:"email"`
	NewEmail  string `json:"newEmail"`
	FullName  string `json:"fullName"`
	IsStudent bool   `json:"isStudent"`
	IsParent  bool   `json:"isParent"`
	IsStaff   bool   `json:"isStaff"`
}

func (a admin) setAddedFields(w http.ResponseWriter, r *http.Request) {
	var body addedFields
	if !decodeBody(w, r, 4<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, cells, ops, err := a.cache.Model().setAddedFields(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:edited added-person fields", "actor", actor.Email, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

type newPerson struct {
	Email     string `json:"email"`
	FullName  string `json:"fullName"`
	IsStudent bool   `json:"isStudent"`
	IsParent  bool   `json:"isParent"`
	IsStaff   bool   `json:"isStaff"`
}

func (a admin) addPerson(w http.ResponseWriter, r *http.Request) {
	var body newPerson
	if !decodeBody(w, r, 4<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	email, fullName, ops, err := a.cache.Model().addPerson(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:added a new person", "actor", actor.Email, "email", email, "name", fullName)
	w.WriteHeader(http.StatusNoContent)
}

type personEmail struct {
	Email string `json:"email"`
}

func (a admin) deletePerson(w http.ResponseWriter, r *http.Request) {
	var body personEmail
	if !decodeBody(w, r, 1<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, ops, err := a.cache.Model().deletePerson(actor, body.Email)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:deleted added person", "actor", actor.Email, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) hidePerson(w http.ResponseWriter, r *http.Request) {
	var body personEmail
	if !decodeBody(w, r, 1<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, ops, err := a.cache.Model().hidePerson(actor, body.Email)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:hid person from the directory", "actor", actor.Email, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) unhidePerson(w http.ResponseWriter, r *http.Request) {
	var body personEmail
	if !decodeBody(w, r, 1<<10, &body) {
		return
	}
	actor := requestActor(a.cache, r)
	target, ops, err := a.cache.Model().unhidePerson(actor, body.Email)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:unhid person from the directory", "actor", actor.Email, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

var imageExtensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
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
	ext, ok := imageExtensions[sniffed]
	if !ok {
		http.Error(w, "unsupported image type "+sniffed, http.StatusBadRequest)
		return
	}
	image := fmt.Sprintf("%x.%s", sha256.Sum256(content), ext)
	actor := requestActor(a.cache, r)
	ops, err := a.cache.Model().setImage(actor, kind, name, image)
	if err != nil {
		refuse(w, err)
		return
	}
	if err := a.media.Put("photos", image, sniffed, content); err != nil {
		serverError(w, r, err)
		return
	}
	if !a.cache.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "who:replaced image", "actor", actor.Email, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}
