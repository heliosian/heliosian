package who

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/serve"
)

type admin struct {
	cache  *Cache
	writer data.Writer
	queue  *Queue
}

// RegisterAdmin wires up the admin tools: image management and the admin list itself.
// Routes are always registered — even in sample mode — so the page and settings are
// reachable for testing; PutImage is the only operation that actually needs a store.
// writer and queue back setPersonFields' writes to the Overrides sheet, the same
// data.Writer + *Queue pairing RegisterTags already uses - unlike PutImage's blob
// store, both are available in sample mode too (data.Dir satisfies data.Writer
// in-memory), so that handler needs no hasStore-style guard.
func RegisterAdmin(mux *http.ServeMux, cache *Cache, writer data.Writer, queue *Queue) {
	a := admin{cache: cache, writer: writer, queue: queue}
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /api/admin/state", a.state)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/images", a.setImage)
	mux.HandleFunc("POST /api/admin/super-edit", a.setSuperEdit)
	mux.HandleFunc("POST /api/admin/spoof", a.setSpoof)
	mux.HandleFunc("POST /api/admin/person-fields", a.setPersonFields)
	mux.HandleFunc("POST /api/admin/student-fields", a.setStudentFields)
	mux.HandleFunc("POST /api/admin/parent-fields", a.setParentFields)
	mux.HandleFunc("POST /api/admin/added-fields", a.setAddedFields)
	mux.HandleFunc("POST /api/admin/add-person", a.addPerson)
	mux.HandleFunc("POST /api/admin/delete-person", a.deletePerson)
	mux.HandleFunc("POST /api/admin/hide-person", a.hidePerson)
	mux.HandleFunc("POST /api/admin/unhide-person", a.unhidePerson)
}

// requireAdmin reports whether the effective identity — the spoof target, if the real
// admin has picked one to view as, otherwise the real admin themselves — is an admin
// (either tier), writing the appropriate error response itself when not. Spoofing is
// full parity, not a read-only preview: every admin handler but setSpoof runs as the
// effective identity, so it does exactly what that person could or couldn't do.
func (a admin) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := effectiveEmail(a.cache, r)
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

// requireRealSuperAdmin is keyed on the real identity rather than the effective one:
// starting or stopping a spoof has to remain reachable by the real admin regardless
// of who they're currently viewing as, or the on-page banner's "stop" would be locked
// out by the very spoof it's meant to end.
func (a admin) requireRealSuperAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := realEmail(a.cache, r)
	if !a.cache.IsSuperAdmin(email) {
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

// overridableFields is every field one of the three Overrides tabs (Staff/Student/
// Parent) can show or edit, in one shape reused for two very different purposes on
// personOption: Override (what's literally in the Overrides sheet for this person
// right now - what each tab's form field is pre-filled with, and diffs against on
// save) and Veracross (what the Student/Staff Import actually supplies, shown as a
// read-only reference under each field). Booleans travel as "TRUE"/"FALSE"/"" strings
// like every other field here, rather than *bool, so the frontend's per-field loop
// doesn't need a special case for the checkbox fields (Is Staff, Is Student, Is
// Parent).
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

// personOption is the admin page's per-person picker entry. Beyond name/email (all
// the Admins/Super Admins/Spoof Mode pickers need), Override/Veracross carry every
// field setPersonFields/setStudentFields/setParentFields/setAddedFields can edit,
// pre-fetched here so the four Overrides tabs can populate their forms the instant a
// person is chosen rather than round-tripping for it. IsStaff/IsStudent/IsParent/
// IsAdded (the resolved, current state - never the override-only one) double as the
// four tabs' picker filters, so each only ever offers the kind of person its fields
// apply to.
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

// overrideStringValue returns column's literal current value in person's Overrides
// row - "" if they have no row, the row has no cell for this column, or the cell
// explicitly clears it ("-"). This is what every Overrides tab's text/select fields
// are pre-filled with and diff against on save, so an admin who never touches a field
// never introduces a new override cell for it, and one who clears a field that DOES
// have an active override removes exactly that override (reverting to whatever
// Veracross supplies, if anything).
func overrideStringValue(person *Person, column string) string {
	cell := person.overrideRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

// overrideBoolValue is overrideStringValue for a checkbox field (Is Staff): true only
// if the row's cell is the literal "TRUE".
func overrideBoolValue(person *Person, column string) bool {
	return person.overrideRow[column] == "TRUE"
}

// familyOf resolves a person's family - the first, for the rare kid with two - with
// the zero Family (nil sheetRow, empty fields) standing in for someone with none.
func familyOf(model *Model, email string) Family {
	keys := model.FamilyKeysOf(email)
	if len(keys) == 0 {
		return Family{}
	}
	return model.Families[keys[0]]
}

// familyStringValue is overrideStringValue for a Families-tab column.
func familyStringValue(family Family, column string) string {
	cell := family.sheetRow[column]
	if cell == "-" {
		return ""
	}
	return cell
}

// importedValue returns column's value as the Veracross Student/Staff Import actually
// supplies it, independent of any override - resolved is the fallback for a person
// with no Overrides row at all (nothing ever changed their fields, so the live value
// already IS the import value). Only meaningful for columns with a real import source;
// callers never invoke this for Department, Grade Band, Facts, or Room Parent, which
// have none for anyone.
func importedValue(person *Person, column, resolved string) string {
	if person.imported != nil {
		return person.imported[column]
	}
	return resolved
}

// fullNameFallback is the Veracross name an admin's cleared Full Name override would
// resolve back to - never empty for anyone with a real import row. The one person for
// whom it IS empty is one who exists purely via their own Overrides row (Overrides'
// "Added" flag - see applyOverrides in load.go), with no Veracross row backing them:
// nothing but their own Full Name override ever supplied them a name, so
// setPersonFields/setStudentFields/setParentFields refuse to let it go blank in that
// case.
func fullNameFallback(person *Person) string {
	return importedValue(person, "Full Name", person.FullName)
}

// crewOption is one named crew belonging to one classroom, offered to the Staff
// Assignments tab so it can restrict the Crew dropdown to what deriveClassrooms will
// actually accept for the chosen classroom. Classrooms with no crews of their own
// (deriveClassrooms falls back to a single unnamed crew for those) contribute nothing
// here - "no crew" is simply leaving the dropdown at its empty option.
type crewOption struct {
	Classroom string `json:"classroom"`
	Name      string `json:"name"`
}

func (a admin) state(w http.ResponseWriter, r *http.Request) {
	real := realEmail(a.cache, r)
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
				// Department, Grade Band, Facts, and Room Parent have no Veracross
				// source for anyone - left blank.
			},
		})
	}
	sort.Slice(people, func(i, j int) bool { return people[i].Name < people[j].Name })
	view := struct {
		Email        string         `json:"email"`
		HasStore     bool           `json:"hasStore"`
		Admins       []string       `json:"admins"`
		Classrooms   []imageInfo    `json:"classrooms"`
		Grades       []imageInfo    `json:"grades"`
		Bands        []string       `json:"bands"`
		Crews        []crewOption   `json:"crews"`
		Departments  []string       `json:"departments"`
		People       []personOption `json:"people"`
		HiddenEmails []string       `json:"hiddenEmails"`
		IsSuperAdmin bool           `json:"isSuperAdmin"`
		SpoofingAs   string         `json:"spoofingAs,omitempty"`
	}{
		Email: email, HasStore: a.cache.HasStore(), Admins: a.cache.Admins(),
		Classrooms: classrooms, Grades: grades, Bands: bands, Crews: crews, Departments: model.Departments,
		People: people, HiddenEmails: model.hiddenEmails,
	}
	// A regular admin's client learns of the super admin tier only through this flag,
	// and the list itself only from the config API, which answers a regular admin
	// with the same 403 a non-admin gets.
	view.IsSuperAdmin = a.cache.IsSuperAdmin(email)
	// The way back is keyed on the real identity and shown regardless of the
	// simulated tier — otherwise spoofing as a regular admin would strand the real
	// super admin on a page with no Spoof Mode tab and no way out.
	view.SpoofingAs = spoofDisplayName(a.cache, real)
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
	// The Admins tab shows every admin merged together, super admins included, so a
	// regular admin can't tell the two tiers apart. Submitting that merged list back
	// must not be able to write a super admin into the Admins tab (or drop them from
	// super admin by omission) — a super admin's presence here is display-only and
	// never round-trips into the tab. This is also why an empty result isn't
	// rejected the way it is for super admins: losing every regular admin can't lock
	// the tools, since the super admin list alone already guarantees access.
	admins := withoutSuperAdmins(config.NormalizeEmails(body.Admins), a.cache.superAdmins())
	current := a.cache.tabAdmins()
	added, removed := listDiff(current, admins)
	applied := make(chan struct{})
	a.queue.Add(func() {
		a.cache.applyAdmins(admins)
		close(applied)
		for _, e := range removed {
			if err := a.writer.Delete(appName, adminsTable, map[string]string{"Email": e}); err != nil {
				slog.ErrorContext(r.Context(), "remove admin", "email", e, "error", err)
			}
		}
		for _, e := range added {
			if err := a.writer.Append(appName, adminsTable, []string{e}); err != nil {
				slog.ErrorContext(r.Context(), "add admin", "email", e, "error", err)
			}
		}
	})
	<-applied
	slog.InfoContext(r.Context(), "admin: set the admin list", "actor", email, "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

// withoutSuperAdmins drops any email that's currently a super admin, so the merged
// list the Admins tab submits back can never write a super admin into the Admins tab
// (or, by their absence from an edited list, be mistaken for removed from the Super
// Admins tab of the Config sheet — that tab is untouched here regardless).
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

func listDiff(before, after []string) (added, removed []string) {
	was := map[string]bool{}
	for _, e := range before {
		was[e] = true
	}
	is := map[string]bool{}
	for _, e := range after {
		is[e] = true
		if !was[e] {
			added = append(added, e)
		}
	}
	for _, e := range before {
		if !is[e] {
			removed = append(removed, e)
		}
	}
	return added, removed
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
	a.cache.SetSuperEdit(email, body.Enabled)
	slog.InfoContext(r.Context(), "admin: set super edit mode", "actor", email, "enabled", body.Enabled)
	w.WriteHeader(http.StatusNoContent)
}

func (a admin) setSpoof(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireRealSuperAdmin(w, r)
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
	if target != "" && a.cache.Model().Person(target) == nil {
		http.Error(w, "no such person", http.StatusBadRequest)
		return
	}
	a.cache.SetSpoof(email, target)
	if target == "" {
		slog.InfoContext(r.Context(), "admin: stopped spoofing", "actor", email)
	} else {
		slog.InfoContext(r.Context(), "admin: started spoofing", "actor", email, "target", target)
	}
	w.WriteHeader(http.StatusNoContent)
}

const (
	maxJobTitleLength = 100
	maxNameLength     = 100
	maxFactsLength    = 4000
	maxPhoneLength    = 40
	maxAddressLength  = 200
)

// setPersonFields edits the Overrides-backed fields that have no self-service editor
// of their own: Classroom, Crew, Department, Job Title, and Grade Band are
// structural, school-filing decisions rather than something a person edits about
// themselves; Full Name, Legal Name, Preferred Name, and Facts do have self-service
// editors elsewhere (upload.go's edit()/facts()) - Pronouns does too, and is
// deliberately left off this form rather than duplicated here - but this gives an
// admin the same direct access to the rest for fixing a record on someone else's
// behalf.
//
// Every field here is diffed against overrideStringValue - what's literally in
// Overrides right now, the same values the form was pre-filled with, NOT the resolved
// value the rest of the app sees - so a field the admin never touches
// never introduces a new override cell, and clearing a field that DOES carry one
// removes exactly that override (falling back to whatever Veracross supplies, if
// anything). Only fields that actually changed are written at all: Overrides rejects
// a whole rebuild (see applyOverrides' "useless override" check in load.go) if a cell
// restates the value already in effect, and this form resubmits every field on every
// save, not just the one the admin touched.
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

	cells := map[string]string{}
	previous := map[string]string{}
	// Classroom, Crew, Department, and Grade Band have no Veracross baseline for STAFF
	// specifically (only students get Classroom/Crew from the import, via their
	// homeroom - see transformImport vs transformStaffImport in load.go), so clearing
	// one of these here needs the no-baseline treatment, not clearable()'s "-" (see
	// diffStringCellNoBaseline's doc comment).
	diffStringCellNoBaseline(cells, previous, "Classroom", body.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCellNoBaseline(cells, previous, "Crew", body.Crew, overrideStringValue(person, "Crew"))
	diffStringCellNoBaseline(cells, previous, "Department", body.Department, overrideStringValue(person, "Department"))
	diffStringCell(cells, previous, "Job Title", body.JobTitle, overrideStringValue(person, "Job Title"))
	diffStringCellNoBaseline(cells, previous, "Grade Band", body.GradeBand, overrideStringValue(person, "Grade Band"))
	diffStringCell(cells, previous, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, previous, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, previous, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffFacts(cells, previous, body.Facts, person)

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "person fields edit", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited person fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

// diffBoolCell and diffStringCell add a cell (and its previous value, for the change
// log) only when next actually differs from current - Overrides rejects a whole
// rebuild if a cell restates the value already in effect (applyOverrides' "useless
// override" check in load.go), and every setXFields handler resubmits its whole form
// on every save, not just the one field the admin actually touched. current is always
// the column's overrideStringValue/overrideBoolValue, never the resolved field -
// see setPersonFields' doc comment for why that distinction matters.
func diffBoolCell(cells, previous map[string]string, column string, next, current bool) {
	if next == current {
		return
	}
	previous[column] = boolCell(current)
	cells[column] = boolCell(next)
}

// diffStringCell uses a "reset to Veracross" convention deliberately different from
// clearable()'s self-service one (upload.go): emptying the field entirely removes the
// override outright (cell = "", which applyCells in load.go drops from the row
// entirely, so the next load sees no cell there at all and apply() falls through to
// whatever Veracross supplies) - matching what an admin means by "delete this
// override," which is to undo a correction that turned out to be unnecessary, not to
// force the field blank. Typing something that's non-empty but all whitespace (a
// single space is the simplest way) is the deliberate escape hatch for the rarer case
// where an admin actually wants Veracross's own value suppressed regardless of what it
// is (cell = "-") - checked before the no-op comparison below, since a field that's
// already empty (no override) and one that's already force-blanked look identical in
// the box, and the space is how the admin tells them apart.
func diffStringCell(cells, previous map[string]string, column string, next, current string) {
	trimmed := strings.TrimSpace(next)
	if next != "" && trimmed == "" {
		previous[column] = current
		cells[column] = "-"
		return
	}
	if trimmed == current {
		return
	}
	previous[column] = current
	cells[column] = trimmed
}

// diffStringCellNoBaseline is diffStringCell for a column with no Veracross import
// baseline (Room Parent; Classroom/Crew/Department/Grade/Grade Band for staff
// specifically) - a plain "" cell IS how those are cleared, unlike every field
// diffStringCell handles; sending clearable()'s "-" would hit applyOverrides' "clears
// a value that's already empty" check every time, since the pre-override baseline such
// a column sees on every rebuild is always "" (see upload.go's edit() "pronouns" case
// for the same reasoning, on a field this app no longer edits from the admin side).
// Room Parent doesn't actually go through that check at all (it has its own branch in
// applyOverrides, with no useless-override guard), so clearable() would technically be
// just as safe there, but this keeps every no-baseline column's handling visibly
// consistent.
func diffStringCellNoBaseline(cells, previous map[string]string, column string, next, current string) {
	next = strings.TrimSpace(next)
	if next == current {
		return
	}
	previous[column] = current
	cells[column] = next
}

// diffFacts is diffStringCellNoBaseline for Facts specifically: Facts Updated rides
// along with it, matching the self-service facts() handler (upload.go), so an
// admin-entered fact doesn't read as stale the moment it's saved.
func diffFacts(cells, previous map[string]string, next string, person *Person) {
	next = strings.TrimSpace(next)
	current := overrideStringValue(person, "Facts")
	if next == current {
		return
	}
	previous["Facts"] = current
	previous["Facts Updated"] = overrideStringValue(person, "Facts Updated")
	cells["Facts"] = next
	cells["Facts Updated"] = today()
}

// validClassroom, validCrew, and validGrade check a submitted value against what
// deriveClassrooms will actually accept, shared by every setXFields handler that
// exposes a Classroom/Crew/Grade field so their validation (and error wording) can't
// drift apart. Empty is always valid - it means "no opinion", not "clear to blank".
func validClassroom(model *Model, name string) bool {
	return name == "" || slices.ContainsFunc(model.Classrooms, func(c Classroom) bool { return c.Name == name })
}

// validCrew matches deriveClassrooms' actual rule, not a stricter one: a crew is only
// checked against a classroom when a classroom is actually given. deriveClassrooms
// skips that check entirely for a staff member with no classroom (load.go's "!p.IsStaff
// || p.Classroom == \"\"" guard) and never runs it for students at all - so a person
// can legitimately carry a Crew override with no Classroom override, and this must
// accept that instead of demanding the crew belong to an empty classroom.
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

// gradeBandSet is the distinct set of grade-band names (Hummingbirds, Halcons, ...),
// shared by every field that names a band directly: Grade Band and Room Parent.
func gradeBandSet() map[string]bool {
	set := map[string]bool{}
	for _, band := range gradeBands {
		set[band] = true
	}
	return set
}

// validFullName is shared by every setXFields handler that exposes Full Name. Unlike
// every other field here, an empty submission isn't simply "no override, leave
// Veracross's value alone" - a genuinely blank Full Name would break rendering
// everywhere it's used, so this only allows clearing the override when Veracross (via
// fullNameFallback) actually has a name to fall back to. The one person for whom it
// doesn't - someone who exists purely via their own Overrides row (Overrides' "Added"
// flag), with no Veracross row at all - can't clear their Full Name override, since
// nothing else supplies them one.
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

// setStudentFields is setPersonFields' student-side counterpart: Grade, Classroom, and
// Crew are structural, school-filing fields with no self-service editor; Full Name,
// Legal Name, and Preferred Name do have one (upload.go's edit()) - Pronouns does too,
// and is deliberately left off this form rather than duplicated here - but this gives
// an admin the same direct access to the rest on a student's behalf.
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

	cells := map[string]string{}
	previous := map[string]string{}
	diffStringCell(cells, previous, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, previous, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, previous, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, previous, "Grade", body.Grade, overrideStringValue(person, "Grade"))
	diffStringCell(cells, previous, "Classroom", body.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCell(cells, previous, "Crew", body.Crew, overrideStringValue(person, "Crew"))

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "student fields edit", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited student fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

// setParentFields edits a parent's name/phone fields plus the family-level Address and
// Room Parent. Address is written to the family's Families-tab row (a second write
// with its own change log entry, keyed by the family key), the same place the
// self-service "address" case in upload.go's edit() writes it. Room Parent has no
// Veracross import baseline (nothing but Overrides ever sets it, via l.roomParents),
// so it's diffed with the no-baseline treatment: a plain "" cell clears it.
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

	cells := map[string]string{}
	previous := map[string]string{}
	diffStringCell(cells, previous, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, previous, "Legal Name", body.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, previous, "Preferred Name", body.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, previous, "Phone", body.Phone, overrideStringValue(person, "Phone"))
	diffStringCellNoBaseline(cells, previous, "Room Parent", body.RoomParent, overrideStringValue(person, "Room Parent"))

	family := familyOf(model, person.Email)
	familyCells := map[string]string{}
	familyPrevious := map[string]string{}
	if family.Key != "" {
		diffStringCell(familyCells, familyPrevious, "Address", body.Address, familyStringValue(family, "Address"))
	}

	if len(cells) > 0 {
		if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "parent fields edit", cells, previous) {
			return
		}
	}
	if len(familyCells) > 0 {
		if !applyFamilyWrite(a.cache, a.writer, a.queue, w, r, actor, family.Key, "parent family edit", familyCells, familyPrevious) {
			return
		}
	}
	if len(cells) > 0 || len(familyCells) > 0 {
		slog.InfoContext(r.Context(), "admin: edited parent fields", "actor", actor, "target", target, "cells", cells, "familyCells", familyCells)
	}
	w.WriteHeader(http.StatusNoContent)
}

// setAddedFields edits an existing added-only person's Email, Full Name, Is Student,
// Is Parent, and Is Staff. Email is unlike every other field this app lets an admin
// edit: everywhere else it's just a lookup key, but for an added-only person (with no
// Veracross row to anchor them) it's their whole identity - so changing it goes
// through applyEmailRenameWrite instead of the usual applyOverrideWrite, renaming
// their Overrides row in place and carrying their Tags/Photos rows along with it
// rather than treating the new address as an unrelated new person. Same
// diff-against-overrideStringValue approach as every other tab for the rest of the
// fields, but there's no Veracross fallback to defend Full Name against going blank
// here (see validFullName) - this person's own override IS the only record of their
// name that exists anywhere.
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

	cells := map[string]string{}
	previous := map[string]string{}
	diffStringCell(cells, previous, "Full Name", body.FullName, overrideStringValue(person, "Full Name"))
	diffBoolCell(cells, previous, "Is Student", body.IsStudent, overrideBoolValue(person, "Is Student"))
	diffBoolCell(cells, previous, "Is Parent", body.IsParent, overrideBoolValue(person, "Is Parent"))
	diffBoolCell(cells, previous, "Is Staff", body.IsStaff, overrideBoolValue(person, "Is Staff"))

	if newEmail != target {
		if !applyEmailRenameWrite(a.cache, a.writer, a.queue, w, r, actor, target, newEmail, cells) {
			return
		}
		slog.InfoContext(r.Context(), "admin: renamed added person", "actor", actor, "target", target, "newEmail", newEmail, "cells", cells)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(cells) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "added person fields edit", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "admin: edited added-person fields", "actor", actor, "target", target, "cells", cells)
	w.WriteHeader(http.StatusNoContent)
}

// addPerson creates a brand new added-only person: someone Overrides is the sole
// record of, with no Veracross import row at all (see applyOverrides in load.go,
// which requires exactly this pairing - an "Added" row must not already exist in the
// import). Unlike the other setXFields handlers, there's no existing person to diff
// against, so every provided cell is written outright - except a false role checkbox,
// which is simply omitted rather than sent as "FALSE": a brand new row's fields all
// start at their Go zero value (false), so explicitly writing "FALSE" over an
// already-false baseline is exactly the useless-override case applyOverrides rejects
// (see diffBoolCell's doc comment).
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

	cells := map[string]string{"Added": "TRUE", "Full Name": fullName}
	if body.IsStudent {
		cells["Is Student"] = "TRUE"
	}
	if body.IsParent {
		cells["Is Parent"] = "TRUE"
	}
	if body.IsStaff {
		cells["Is Staff"] = "TRUE"
	}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, email, "add person", cells, map[string]string{}) {
		return
	}
	slog.InfoContext(r.Context(), "admin: added a new person", "actor", actor, "email", email, "name", fullName)
	w.WriteHeader(http.StatusNoContent)
}

// deletePerson permanently removes an added-only person - the Added Overrides tab's
// delete. Restricted to added-only people for the same reason setAddedFields is:
// anyone else's record comes from Veracross, and deleting their Overrides row would
// just leave them showing up again next rebuild with whatever the import supplies,
// not actually remove them from the directory.
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
	previous := maps.Clone(person.overrideRow)
	if !applyDeletePersonWrite(a.cache, a.writer, a.queue, w, r, actor, target, previous) {
		return
	}
	slog.InfoContext(r.Context(), "admin: deleted added person", "actor", actor, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

// hidePerson sets Opted Out for an existing person - the Hidden Overrides tab's "hide"
// action. This is the same mechanism the self-service privacy opt-out flow uses
// (upload.go's optOut), just admin-initiated for any person rather than only the
// caller's own record: removeOptedOut (load.go) drops them from People entirely on
// the next rebuild, which is what actually hides them from the whole directory.
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
	cells := map[string]string{"Opted Out": "TRUE"}
	previous := map[string]string{"Opted Out": ""}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "hide person", cells, previous) {
		return
	}
	slog.InfoContext(r.Context(), "admin: hid person from the directory", "actor", actor, "target", target)
	w.WriteHeader(http.StatusNoContent)
}

// unhidePerson clears Opted Out for a currently-hidden person, so they reappear in the
// directory on the next rebuild. Unlike every other handler here, the target is never
// in model.Person - that's the whole point of being hidden - so this checks against
// Model.hiddenEmails instead. Opted Out has no Veracross import baseline (nothing but
// Overrides ever sets it) but, unlike Pronouns/Facts/Room Parent, it isn't run through
// applyOverrides' generic apply()/applyBool() closures at all (it has its own inline
// switch in load.go with no useless-override guard), so clearing it is always safe -
// no diffStringCellNoBaseline-style special-casing needed.
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
	cells := map[string]string{"Opted Out": ""}
	previous := map[string]string{"Opted Out": "TRUE"}
	if !applyOverrideWrite(a.cache, a.writer, a.queue, w, r, actor, target, "unhide person", cells, previous) {
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
	if !a.cache.HasStore() {
		http.Error(w, "image uploads require real-data mode", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "upload too large or malformed", http.StatusBadRequest)
		return
	}
	kind := r.FormValue("kind")
	name := strings.TrimSpace(r.FormValue("name"))
	var folder, slug string
	switch kind {
	case "classroom":
		folder = "classroom-images"
		slug = strings.ToLower(name)
	case "grade":
		folder = "grade-images"
		slug = gradeSlug(name)
	default:
		http.Error(w, "bad kind: must be classroom or grade", http.StatusBadRequest)
		return
	}
	if slug == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
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

	if err := a.cache.PutImage(folder, slug+"."+ext, sniffed, content); err != nil {
		serverError(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "admin: replaced image", "actor", email, "kind", kind, "name", name)
	w.WriteHeader(http.StatusNoContent)
}
