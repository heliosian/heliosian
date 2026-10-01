package model

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/geocode"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

func setOverride(email string, cells store.Row) store.Op {
	return store.Upsert(overridesTab, store.Row{"Email": email}, cells)
}

func setFamily(key string, cells store.Row) store.Op {
	return store.Upsert(familiesTab, store.Row{"Email": key}, cells)
}

func photoOps(email string, before []photoRef, after []photoRef) []store.Op {
	keys := []string{}
	for _, ref := range after {
		keys = append(keys, ref.order)
	}
	keys = store.Order(keys)
	ops := []store.Op{}
	kept := map[string]bool{}
	for i, ref := range after {
		kept[ref.Name] = true
		if ref.stored {
			ops = append(ops, store.Update(photosTab, store.Row{"Email": email, "Photo Name": ref.Name}, store.Row{store.OrderColumn: keys[i], "Crop Name": ref.CropName}))
			continue
		}
		ops = append(ops, store.Insert(photosTab, store.Row{"Email": email, "Photo Name": ref.Name, store.OrderColumn: keys[i], "Crop Name": ref.CropName}))
	}
	for _, ref := range before {
		if ref.stored && !kept[ref.Name] {
			ops = append(ops, store.Delete(photosTab, store.Row{"Email": email, "Photo Name": ref.Name}))
		}
	}
	return ops
}

func (m *Directory) adminTarget(actor access.Actor, email string) (*Person, error) {
	if err := require(actor, Administer); err != nil {
		return nil, err
	}
	person := m.Person(strings.ToLower(strings.TrimSpace(email)))
	if person == nil {
		return nil, access.Invalid("no such person")
	}
	return person, nil
}

func recordFieldApplies(field string, person *Person) bool {
	switch field {
	case "fullName", "legalName", "preferredName":
		return true
	case "facts", "department", "jobTitle", "gradeBand":
		return person.IsStaff
	case "grade":
		return person.IsStudent
	case "classroom", "crew":
		return person.IsStaff || person.IsStudent
	case "phone", "roomParent", "address":
		return person.IsParent
	case "email", "isStudent", "isParent", "isStaff":
		return overrideBoolValue(person, "Added")
	}
	return false
}

func (m *Directory) checkRecordFields(person *Person, f recordFields, sent map[string]bool) error {
	for field := range sent {
		if !recordFieldApplies(field, person) {
			return access.Invalid("%s does not apply to %s", field, person.Email)
		}
	}
	classroom := overrideStringValue(person, "Classroom")
	if sent["classroom"] {
		classroom = f.Classroom
	}
	checks := []struct {
		field string
		bad   bool
		why   string
	}{
		{"legalName", len(f.LegalName) > maxNameLength, "bad legal name"},
		{"preferredName", len(f.PreferredName) > maxNameLength, "bad preferred name"},
		{"facts", len(f.Facts) > maxFactsLength, "bad facts"},
		{"jobTitle", len(f.JobTitle) > maxJobTitleLength, "job title too long"},
		{"department", f.Department != "" && !slices.Contains(m.Departments, f.Department), "unknown department"},
		{"gradeBand", f.GradeBand != "" && !gradeBandSet()[f.GradeBand], "unknown grade band"},
		{"grade", !validGrade(f.Grade), "unknown grade"},
		{"classroom", !validClassroom(m, f.Classroom), "unknown classroom"},
		{"crew", !validCrew(m, classroom, f.Crew), "unknown crew for that classroom"},
		{"phone", len(f.Phone) > maxPhoneLength, "bad phone number"},
		{"address", len(f.Address) > maxAddressLength, "bad address"},
		{"roomParent", f.RoomParent != "" && !gradeBandSet()[f.RoomParent], "unknown room parent band"},
	}
	for _, c := range checks {
		if sent[c.field] && c.bad {
			return access.Invalid("%s", c.why)
		}
	}
	if sent["fullName"] {
		if err := validFullName(f.FullName, person); err != nil {
			return err
		}
	}
	return nil
}

func (m *Directory) setRecordFields(actor access.Actor, email string, f recordFields, sent map[string]bool) ([]store.Op, error) {
	person, err := m.adminTarget(actor, email)
	if err != nil {
		return nil, err
	}
	if err := m.checkRecordFields(person, f, sent); err != nil {
		return nil, err
	}
	target := person.Email
	cells := store.Row{}
	text := func(field, column string, diff func(store.Row, string, string, string), value string) {
		if sent[field] {
			diff(cells, column, value, overrideStringValue(person, column))
		}
	}
	text("fullName", "Full Name", diffStringCell, f.FullName)
	text("legalName", "Legal Name", diffStringCell, f.LegalName)
	text("preferredName", "Preferred Name", diffStringCell, f.PreferredName)
	text("jobTitle", "Job Title", diffStringCell, f.JobTitle)
	text("department", "Department", diffStringCellNoBaseline, f.Department)
	text("gradeBand", "Grade Band", diffStringCellNoBaseline, f.GradeBand)
	text("grade", "Grade", diffStringCell, f.Grade)
	text("phone", "Phone", diffStringCell, f.Phone)
	text("roomParent", "Room Parent", diffStringCellNoBaseline, f.RoomParent)
	placement := diffStringCellNoBaseline
	if person.IsStudent {
		placement = diffStringCell
	}
	text("classroom", "Classroom", placement, f.Classroom)
	text("crew", "Crew", placement, f.Crew)
	if sent["facts"] {
		diffFacts(cells, f.Facts, person)
	}
	added := overrideBoolValue(person, "Added")
	if added {
		roles := map[string]bool{}
		for _, r := range []struct {
			field, column string
			value         bool
		}{{"isStudent", "Is Student", f.IsStudent}, {"isParent", "Is Parent", f.IsParent}, {"isStaff", "Is Staff", f.IsStaff}} {
			current := overrideBoolValue(person, r.column)
			roles[r.field] = current
			if sent[r.field] {
				roles[r.field] = r.value
				diffBoolCell(cells, r.column, r.value, current)
			}
		}
		if !roles["isStudent"] && !roles["isParent"] && !roles["isStaff"] {
			return nil, access.Invalid("choose at least one of Is Student, Is Parent, or Is Staff")
		}
		if sent["email"] {
			next := strings.ToLower(strings.TrimSpace(f.Email))
			if !strings.Contains(next, "@") {
				return nil, access.Invalid("bad email address")
			}
			if next != target && m.Person(next) != nil {
				return nil, access.Invalid("a person with this email already exists")
			}
			if next != target {
				cells["Email"] = next
			}
		}
	}
	ops := []store.Op{}
	if len(cells) > 0 && added {
		ops = append(ops, store.Update(overridesTab, store.Row{"Email": target}, cells))
	}
	if len(cells) > 0 && !added {
		ops = append(ops, setOverride(target, cells))
	}
	family, _ := m.FamilyOf(person.Email)
	if sent["address"] && family.Key != "" {
		familyCells := store.Row{}
		diffStringCell(familyCells, "Address", f.Address, familyStringValue(family, "Address"))
		if len(familyCells) > 0 {
			ops = append(ops, setFamily(family.email, familyCells))
		}
	}
	return ops, nil
}

type newPerson struct {
	Email     string `json:"email"`
	FullName  string `json:"fullName"`
	IsStudent bool   `json:"isStudent"`
	IsParent  bool   `json:"isParent"`
	IsStaff   bool   `json:"isStaff"`
}

func (m *Directory) addPerson(actor access.Actor, f newPerson) (string, string, []store.Op, error) {
	if err := require(actor, Administer); err != nil {
		return "", "", nil, err
	}
	email := strings.ToLower(strings.TrimSpace(f.Email))
	if !strings.Contains(email, "@") {
		return "", "", nil, access.Invalid("bad email address")
	}
	if m.Person(email) != nil {
		return "", "", nil, access.Invalid("a person with this email already exists")
	}
	fullName := strings.TrimSpace(f.FullName)
	if fullName == "" || len(fullName) > maxNameLength {
		return "", "", nil, access.Invalid("bad full name")
	}
	if !f.IsStudent && !f.IsParent && !f.IsStaff {
		return "", "", nil, access.Invalid("choose at least one of Is Student, Is Parent, or Is Staff")
	}
	row := store.Row{"Added": cells.YesNoCell(true), "Full Name": fullName}
	if f.IsStudent {
		row["Is Student"] = cells.YesNoCell(true)
	}
	if f.IsParent {
		row["Is Parent"] = cells.YesNoCell(true)
	}
	if f.IsStaff {
		row["Is Staff"] = cells.YesNoCell(true)
	}
	return email, fullName, []store.Op{setOverride(email, row)}, nil
}

func (m *Directory) deletePerson(actor access.Actor, email string) (string, []store.Op, error) {
	person, err := m.adminTarget(actor, email)
	if err != nil {
		return "", nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	if !overrideBoolValue(person, "Added") {
		return "", nil, access.Invalid("not an added-only person")
	}
	return target, []store.Op{store.Delete(overridesTab, store.Row{"Email": target})}, nil
}

func (m *Directory) hidePerson(actor access.Actor, email string) (string, []store.Op, error) {
	if _, err := m.adminTarget(actor, email); err != nil {
		return "", nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	return target, []store.Op{setOverride(target, store.Row{"Opted Out": cells.YesNoCell(true)})}, nil
}

func (m *Directory) unhidePerson(actor access.Actor, email string) (string, []store.Op, error) {
	if err := require(actor, Administer); err != nil {
		return "", nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	if !slices.Contains(m.hiddenEmails, target) {
		return "", nil, access.Invalid("not currently hidden")
	}
	return target, []store.Op{store.Update(overridesTab, store.Row{"Email": target}, store.Row{"Opted Out": ""})}, nil
}

func (m *Directory) setImage(actor access.Actor, kind, name, image string) ([]store.Op, error) {
	if err := require(actor, Administer); err != nil {
		return nil, err
	}
	switch kind {
	case imageClassroom:
		if !slices.ContainsFunc(m.Classrooms, func(c Classroom) bool { return c.Name == name }) {
			return nil, access.Invalid("no such classroom")
		}
	case imageGrade:
		if !slices.Contains(gradeOrder, name) {
			return nil, access.Invalid("no such grade")
		}
	default:
		return nil, access.Invalid("bad kind: must be classroom or grade")
	}
	return []store.Op{store.Upsert(imagesTab, store.Row{imageKind: kind, imageName: name}, store.Row{imageImage: image})}, nil
}

func (m *Directory) mayEdit(actor access.Actor, target, key string) error {
	if actor.May(EditAnyone) || m.familyMayEdit(actor.Email, target, key) {
		return nil
	}
	return access.Forbidden("not allowed to edit this record")
}

func (m *Directory) familyMayEdit(me, target, key string) bool {
	if target == "person" && key == me && m.Member(me) {
		return true
	}
	for _, familyKey := range m.FamilyKeysOf(me) {
		family := m.Families[familyKey]
		if !slices.Contains(family.AdultEmails, me) {
			continue
		}
		if target == "family" {
			if familyKey == key {
				return true
			}
		} else if slices.Contains(family.KidEmails, key) || slices.Contains(family.AdultEmails, key) {
			return true
		}
	}
	return false
}

func (m *Directory) editField(actor access.Actor, field, key, value string) ([]store.Op, error) {
	if field == "family-photo-caption" {
		if err := m.mayEdit(actor, "family", key); err != nil {
			return nil, err
		}
		family, ok := m.Families[key]
		if !ok {
			return nil, access.Invalid("no such family")
		}
		if len(value) > 200 {
			return nil, access.Invalid("bad caption")
		}
		return []store.Op{setFamily(family.email, store.Row{"Family Photo Caption": clearable(value)})}, nil
	}
	if field == "family-pronunciation" {
		if err := m.mayEdit(actor, "family", key); err != nil {
			return nil, err
		}
		if value != "" {
			return nil, access.Invalid("pronunciation can only be cleared through this field")
		}
		family, ok := m.Families[key]
		if !ok {
			return nil, access.Invalid("no such family")
		}
		return []store.Op{setFamily(family.email, store.Row{"Family Pronunciation": ""})}, nil
	}
	person := m.Person(key)
	if person == nil {
		return nil, access.Invalid("no such person")
	}
	cells := store.Row{}
	switch field {
	case "preferred-name":
		if err := m.mayEdit(actor, "person", key); err != nil {
			return nil, err
		}
		if value == "" || len(value) > 80 {
			return nil, access.Invalid("bad preferred name")
		}
		base := person.LegalName
		if base == "" {
			base = person.FullName
		}
		cells["Preferred Name"] = value
		cells["Full Name"] = value + " " + surname(base)
	case "pronouns":
		if err := m.mayEdit(actor, "person", key); err != nil {
			return nil, err
		}
		if len(value) > 40 {
			return nil, access.Invalid("bad pronouns")
		}
		cells["Pronouns"] = strings.ToLower(value)
	case "pronunciation":
		if err := m.mayEdit(actor, "person", key); err != nil {
			return nil, err
		}
		if value != "" {
			return nil, access.Invalid("pronunciation can only be cleared through this field")
		}
		cells["Pronunciation"] = ""
	default:
		return nil, access.Invalid("bad field")
	}
	return []store.Op{setOverride(key, cells)}, nil
}

func (m *Directory) setFacts(actor access.Actor, key, facts string) ([]store.Op, error) {
	if key == "" || len(facts) > 4000 {
		return nil, access.Invalid("bad facts request")
	}
	if err := m.mayEdit(actor, "person", key); err != nil {
		return nil, err
	}
	return []store.Op{setOverride(key, store.Row{"Facts": facts, "Facts Updated": today()})}, nil
}

func (m *Directory) upload(actor access.Actor, target, kind, key, name string) ([]store.Op, error) {
	if (target != "person" && target != "family") || (kind != "photo" && kind != "pronunciation") || key == "" {
		return nil, access.Invalid("bad upload request")
	}
	if err := m.mayEdit(actor, target, key); err != nil {
		return nil, err
	}
	if target == "family" {
		family, ok := m.Families[key]
		if !ok {
			return nil, access.Invalid("no such family")
		}
		cells := store.Row{"Family Pronunciation": name}
		if kind == "photo" {
			cells = store.Row{"Family Photo": name, "Family Photo Updated": today()}
		}
		return []store.Op{setFamily(family.email, cells)}, nil
	}
	person := m.Person(key)
	if person == nil {
		return nil, access.Invalid("no such person")
	}
	if kind != "photo" {
		return []store.Op{setOverride(key, store.Row{"Pronunciation": name})}, nil
	}
	if len(person.Photos) >= maxPhotos {
		return nil, access.Invalid("already has the maximum of %d photos", maxPhotos)
	}
	if isPhotoSubset([]string{name}, person.Photos) {
		return nil, access.Invalid("already has this photo")
	}
	before := refsOf(person)
	after := append(slices.Clone(before), photoRef{Name: name})
	return append(photoOps(key, before, after), setOverride(key, store.Row{"Photo Updated": today()})), nil
}

func (m *Directory) reorderPhotos(actor access.Actor, key string, names []string) ([]store.Op, error) {
	person := m.Person(key)
	if person == nil {
		return nil, access.Invalid("no such person")
	}
	if err := m.mayEdit(actor, "person", key); err != nil {
		return nil, err
	}
	if !isPhotoSubset(names, person.Photos) {
		return nil, access.Invalid("order must name only this person's current photos, with no duplicates")
	}
	before := refsOf(person)
	after := []photoRef{}
	for _, name := range names {
		after = append(after, before[slices.IndexFunc(before, func(ref photoRef) bool { return ref.Name == name })])
	}
	return photoOps(key, before, after), nil
}

func (m *Directory) cropPhoto(actor access.Actor, target, key, name, cropName string) ([]store.Op, error) {
	var person *Person
	var family Family
	switch target {
	case "person":
		person = m.Person(key)
		if person == nil {
			return nil, access.Invalid("no such person")
		}
	case "family":
		var ok bool
		family, ok = m.Families[key]
		if !ok {
			return nil, access.Invalid("no such family")
		}
	default:
		return nil, access.Invalid("bad crop request")
	}
	if err := m.mayEdit(actor, target, key); err != nil {
		return nil, err
	}
	if target == "family" {
		if family.photo == "" {
			return nil, access.Invalid("family has no photo to crop")
		}
		return []store.Op{setFamily(family.email, store.Row{"Family Photo Crop": cropName, "Family Photo Updated": today()})}, nil
	}
	if !isPhotoSubset([]string{name}, person.Photos) {
		return nil, access.Invalid("not one of this person's photos")
	}
	before := refsOf(person)
	after := slices.Clone(before)
	for i := range after {
		if after[i].Name == name {
			after[i].CropName = cropName
		}
	}
	return append(photoOps(key, before, after), setOverride(key, store.Row{"Photo Updated": today()})), nil
}

func validTagName(tag string) bool {
	return tag != "" && len(tag) <= maxTagLength
}

func (m *Directory) ownTag(actor access.Actor, key string) (*tagRecord, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, access.Invalid("no such tag")
	}
	if t.owner != actor.Email {
		return nil, access.Forbidden("not your tag")
	}
	return t, nil
}

func (m *Directory) managedTag(actor access.Actor, key string) (*tagRecord, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, access.Invalid("no such tag")
	}
	if t.owner != actor.Email && !slices.Contains(t.managers, actor.Email) {
		return nil, access.Forbidden("not your tag to manage")
	}
	return t, nil
}

func (m *Directory) renameTag(actor access.Actor, key, to string) ([]store.Op, string, error) {
	if !validTagName(to) {
		return nil, "", access.Invalid("bad tag name")
	}
	t, err := m.ownTag(actor, key)
	if err != nil {
		return nil, "", err
	}
	if other := m.ownTagNamed(actor.Email, to); other != nil && other != t {
		return nil, "", access.Refuse(http.StatusConflict, "you already have a tag called %s", to)
	}
	return []store.Op{store.Update(tagListTable, store.Row{tagID: t.id}, store.Row{tagName: to})}, t.name, nil
}

func (m *Directory) copyTag(actor access.Actor, key, to string, taken func(string) bool) ([]store.Op, string, int, error) {
	if !validTagName(to) {
		return nil, "", 0, access.Invalid("bad tag name")
	}
	t, err := m.managedTag(actor, key)
	if err != nil {
		return nil, "", 0, err
	}
	people := m.listed(t.people)
	if len(people) == 0 {
		return nil, "", 0, access.Invalid("no such tag")
	}
	if m.ownTagNamed(actor.Email, to) != nil {
		return nil, "", 0, access.Refuse(http.StatusConflict, "you already have a tag called %s", to)
	}
	made := id.New(taken)
	ops := []store.Op{store.Insert(tagListTable, store.Row{tagID: made, tagOwner: actor.Email, tagName: to})}
	for _, person := range people {
		ops = append(ops, store.Insert(tagsTable, store.Row{tagID: made, tagPerson: person}))
	}
	return ops, made, len(people), nil
}

func (m *Directory) shareTag(actor access.Actor, key, manager string, on bool) ([]store.Op, error) {
	if m.Person(manager) == nil || manager == actor.Email {
		return nil, access.Invalid("no such person")
	}
	if m.tagByKey(key) == nil && !on {
		return nil, nil
	}
	t, err := m.ownTag(actor, key)
	if err != nil {
		return nil, err
	}
	row := store.Row{tagID: t.id, managerEmail: manager}
	if on {
		return []store.Op{store.Upsert(managersTable, row, store.Row{})}, nil
	}
	return []store.Op{store.Delete(managersTable, row)}, nil
}

func (m *Directory) leaveTag(actor access.Actor, key string) ([]store.Op, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, nil
	}
	if t.owner == actor.Email {
		return nil, access.Invalid("you own this tag")
	}
	return []store.Op{store.Delete(managersTable, store.Row{tagID: t.id, managerEmail: actor.Email})}, nil
}

func (m *Directory) dropTag(actor access.Actor, key string) ([]store.Op, int, error) {
	if m.tagByKey(key) == nil {
		return nil, 0, nil
	}
	t, err := m.ownTag(actor, key)
	if err != nil {
		return nil, 0, err
	}
	return []store.Op{store.Delete(tagListTable, store.Row{tagID: t.id})}, len(m.listed(t.people)), nil
}

func (m *Directory) setTag(actor access.Actor, key, name, person string, on bool, taken func(string) bool) ([]store.Op, string, error) {
	if m.Person(person) == nil {
		return nil, "", access.Invalid("no such person")
	}
	if key == "" {
		return m.tagNamed(actor, name, person, on, taken)
	}
	t, err := m.managedTag(actor, key)
	if err != nil {
		return nil, "", err
	}
	if slices.Contains(t.people, person) == on {
		return nil, t.id, nil
	}
	row := store.Row{tagID: t.id, tagPerson: person}
	if on {
		return []store.Op{store.Insert(tagsTable, row)}, t.id, nil
	}
	return []store.Op{store.Delete(tagsTable, row)}, t.id, nil
}

func (m *Directory) tagNamed(actor access.Actor, name, person string, on bool, taken func(string) bool) ([]store.Op, string, error) {
	if !validTagName(name) || !on {
		return nil, "", access.Invalid("bad tag name")
	}
	if t := m.ownTagNamed(actor.Email, name); t != nil {
		return m.setTag(actor, t.id, "", person, true, taken)
	}
	made := id.New(taken)
	return []store.Op{
		store.Insert(tagListTable, store.Row{tagID: made, tagOwner: actor.Email, tagName: name}),
		store.Insert(tagsTable, store.Row{tagID: made, tagPerson: person}),
	}, made, nil
}

func (m *Directory) locate(actor access.Actor, found map[string]geocode.Point) []store.Op {
	ops := []store.Op{}
	for _, address := range m.unlocated {
		point, ok := found[address]
		if !ok {
			continue
		}
		ops = append(ops, store.Insert(geocodeTable, store.Row{
			geocodeAddress: address,
			geocodeLat:     strconv.FormatFloat(point.Lat, 'f', -1, 64),
			geocodeLng:     strconv.FormatFloat(point.Lng, 'f', -1, 64),
		}))
	}
	return ops
}
