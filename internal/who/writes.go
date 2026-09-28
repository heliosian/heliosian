package who

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

func mayAdminister(actor access.Actor) error {
	if !actor.May(Administer) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func (m *Model) adminTarget(actor access.Actor, email string) (*Person, error) {
	if err := mayAdminister(actor); err != nil {
		return nil, err
	}
	person := m.Person(strings.ToLower(strings.TrimSpace(email)))
	if person == nil {
		return nil, access.Invalid("no such person")
	}
	return person, nil
}

func (m *Model) setPersonFields(actor access.Actor, f personFields) (string, store.Row, []store.Op, error) {
	person, err := m.adminTarget(actor, f.Email)
	if err != nil {
		return "", nil, nil, err
	}
	target := strings.ToLower(strings.TrimSpace(f.Email))
	if len(f.JobTitle) > maxJobTitleLength {
		return "", nil, nil, access.Invalid("job title too long")
	}
	if !validClassroom(m, f.Classroom) {
		return "", nil, nil, access.Invalid("unknown classroom")
	}
	if !validCrew(m, f.Classroom, f.Crew) {
		return "", nil, nil, access.Invalid("unknown crew for that classroom")
	}
	if f.Department != "" && !slices.Contains(m.Departments, f.Department) {
		return "", nil, nil, access.Invalid("unknown department")
	}
	if f.GradeBand != "" && !gradeBandSet()[f.GradeBand] {
		return "", nil, nil, access.Invalid("unknown grade band")
	}
	if err := validFullName(f.FullName, person); err != nil {
		return "", nil, nil, err
	}
	if len(f.LegalName) > maxNameLength {
		return "", nil, nil, access.Invalid("bad legal name")
	}
	if len(f.PreferredName) > maxNameLength {
		return "", nil, nil, access.Invalid("bad preferred name")
	}
	if len(f.Facts) > maxFactsLength {
		return "", nil, nil, access.Invalid("bad facts")
	}
	cells := store.Row{}
	diffStringCellNoBaseline(cells, "Classroom", f.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCellNoBaseline(cells, "Crew", f.Crew, overrideStringValue(person, "Crew"))
	diffStringCellNoBaseline(cells, "Department", f.Department, overrideStringValue(person, "Department"))
	diffStringCell(cells, "Job Title", f.JobTitle, overrideStringValue(person, "Job Title"))
	diffStringCellNoBaseline(cells, "Grade Band", f.GradeBand, overrideStringValue(person, "Grade Band"))
	diffStringCell(cells, "Full Name", f.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", f.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", f.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffFacts(cells, f.Facts, person)
	if len(cells) == 0 {
		return target, cells, nil, nil
	}
	return target, cells, []store.Op{setOverride(target, cells)}, nil
}

func (m *Model) setStudentFields(actor access.Actor, f studentFields) (string, store.Row, []store.Op, error) {
	person, err := m.adminTarget(actor, f.Email)
	if err != nil {
		return "", nil, nil, err
	}
	target := strings.ToLower(strings.TrimSpace(f.Email))
	if err := validFullName(f.FullName, person); err != nil {
		return "", nil, nil, err
	}
	if len(f.LegalName) > maxNameLength {
		return "", nil, nil, access.Invalid("bad legal name")
	}
	if len(f.PreferredName) > maxNameLength {
		return "", nil, nil, access.Invalid("bad preferred name")
	}
	if !validGrade(f.Grade) {
		return "", nil, nil, access.Invalid("unknown grade")
	}
	if !validClassroom(m, f.Classroom) {
		return "", nil, nil, access.Invalid("unknown classroom")
	}
	if !validCrew(m, f.Classroom, f.Crew) {
		return "", nil, nil, access.Invalid("unknown crew for that classroom")
	}
	cells := store.Row{}
	diffStringCell(cells, "Full Name", f.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", f.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", f.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, "Grade", f.Grade, overrideStringValue(person, "Grade"))
	diffStringCell(cells, "Classroom", f.Classroom, overrideStringValue(person, "Classroom"))
	diffStringCell(cells, "Crew", f.Crew, overrideStringValue(person, "Crew"))
	if len(cells) == 0 {
		return target, cells, nil, nil
	}
	return target, cells, []store.Op{setOverride(target, cells)}, nil
}

func (m *Model) setParentFields(actor access.Actor, f parentFields) (string, store.Row, store.Row, []store.Op, error) {
	person, err := m.adminTarget(actor, f.Email)
	if err != nil {
		return "", nil, nil, nil, err
	}
	target := strings.ToLower(strings.TrimSpace(f.Email))
	if !person.IsParent {
		return "", nil, nil, nil, access.Invalid("not a parent")
	}
	if err := validFullName(f.FullName, person); err != nil {
		return "", nil, nil, nil, err
	}
	if len(f.LegalName) > maxNameLength {
		return "", nil, nil, nil, access.Invalid("bad legal name")
	}
	if len(f.PreferredName) > maxNameLength {
		return "", nil, nil, nil, access.Invalid("bad preferred name")
	}
	if len(f.Phone) > maxPhoneLength {
		return "", nil, nil, nil, access.Invalid("bad phone number")
	}
	if len(f.Address) > maxAddressLength {
		return "", nil, nil, nil, access.Invalid("bad address")
	}
	if f.RoomParent != "" && !gradeBandSet()[f.RoomParent] {
		return "", nil, nil, nil, access.Invalid("unknown room parent band")
	}
	cells := store.Row{}
	diffStringCell(cells, "Full Name", f.FullName, overrideStringValue(person, "Full Name"))
	diffStringCell(cells, "Legal Name", f.LegalName, overrideStringValue(person, "Legal Name"))
	diffStringCell(cells, "Preferred Name", f.PreferredName, overrideStringValue(person, "Preferred Name"))
	diffStringCell(cells, "Phone", f.Phone, overrideStringValue(person, "Phone"))
	diffStringCellNoBaseline(cells, "Room Parent", f.RoomParent, overrideStringValue(person, "Room Parent"))
	family, _ := m.FamilyOf(person.Email)
	familyCells := store.Row{}
	if family.Key != "" {
		diffStringCell(familyCells, "Address", f.Address, familyStringValue(family, "Address"))
	}
	ops := []store.Op{}
	if len(cells) > 0 {
		ops = append(ops, setOverride(target, cells))
	}
	if len(familyCells) > 0 {
		ops = append(ops, setFamily(family.email, familyCells))
	}
	return target, cells, familyCells, ops, nil
}

func (m *Model) setAddedFields(actor access.Actor, f addedFields) (string, store.Row, []store.Op, error) {
	person, err := m.adminTarget(actor, f.Email)
	if err != nil {
		return "", nil, nil, err
	}
	target := strings.ToLower(strings.TrimSpace(f.Email))
	if !overrideBoolValue(person, "Added") {
		return "", nil, nil, access.Invalid("not an added-only person")
	}
	if err := validFullName(f.FullName, person); err != nil {
		return "", nil, nil, err
	}
	if !f.IsStudent && !f.IsParent && !f.IsStaff {
		return "", nil, nil, access.Invalid("choose at least one of Is Student, Is Parent, or Is Staff")
	}
	newEmail := strings.ToLower(strings.TrimSpace(f.NewEmail))
	if !strings.Contains(newEmail, "@") {
		return "", nil, nil, access.Invalid("bad email address")
	}
	if newEmail != target && m.Person(newEmail) != nil {
		return "", nil, nil, access.Invalid("a person with this email already exists")
	}
	cells := store.Row{}
	diffStringCell(cells, "Full Name", f.FullName, overrideStringValue(person, "Full Name"))
	diffBoolCell(cells, "Is Student", f.IsStudent, overrideBoolValue(person, "Is Student"))
	diffBoolCell(cells, "Is Parent", f.IsParent, overrideBoolValue(person, "Is Parent"))
	diffBoolCell(cells, "Is Staff", f.IsStaff, overrideBoolValue(person, "Is Staff"))
	if newEmail != target {
		cells["Email"] = newEmail
	}
	if len(cells) == 0 {
		return target, cells, nil, nil
	}
	return target, cells, []store.Op{store.Update(overridesTab, store.Row{"Email": target}, cells)}, nil
}

func (m *Model) addPerson(actor access.Actor, f newPerson) (string, string, []store.Op, error) {
	if err := mayAdminister(actor); err != nil {
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

func (m *Model) deletePerson(actor access.Actor, email string) (string, []store.Op, error) {
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

func (m *Model) hidePerson(actor access.Actor, email string) (string, []store.Op, error) {
	if _, err := m.adminTarget(actor, email); err != nil {
		return "", nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	return target, []store.Op{setOverride(target, store.Row{"Opted Out": cells.YesNoCell(true)})}, nil
}

func (m *Model) unhidePerson(actor access.Actor, email string) (string, []store.Op, error) {
	if err := mayAdminister(actor); err != nil {
		return "", nil, err
	}
	target := strings.ToLower(strings.TrimSpace(email))
	if !slices.Contains(m.hiddenEmails, target) {
		return "", nil, access.Invalid("not currently hidden")
	}
	return target, []store.Op{store.Update(overridesTab, store.Row{"Email": target}, store.Row{"Opted Out": ""})}, nil
}

func (m *Model) setImage(actor access.Actor, kind, name, image string) ([]store.Op, error) {
	if err := mayAdminister(actor); err != nil {
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

func (m *Model) mayEdit(actor access.Actor, target, key string) error {
	if actor.May(EditAnyone) || m.familyMayEdit(actor.Email, target, key) {
		return nil
	}
	return access.Forbidden("not allowed to edit this record")
}

func (m *Model) familyMayEdit(me, target, key string) bool {
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

func (m *Model) editField(actor access.Actor, field, key, value string) ([]store.Op, error) {
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

func (m *Model) setFacts(actor access.Actor, key, facts string) ([]store.Op, error) {
	if key == "" || len(facts) > 4000 {
		return nil, access.Invalid("bad facts request")
	}
	if err := m.mayEdit(actor, "person", key); err != nil {
		return nil, err
	}
	return []store.Op{setOverride(key, store.Row{"Facts": facts, "Facts Updated": today()})}, nil
}

func (m *Model) upload(actor access.Actor, target, kind, key, name string) ([]store.Op, error) {
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

func (m *Model) reorderPhotos(actor access.Actor, key string, names []string) ([]store.Op, error) {
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

func (m *Model) cropPhoto(actor access.Actor, target, key, name, cropName string) ([]store.Op, error) {
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

func (m *Model) ownTag(actor access.Actor, key string) (*tagRecord, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, access.Invalid("no such tag")
	}
	if t.owner != actor.Email {
		return nil, access.Forbidden("not your tag")
	}
	return t, nil
}

func (m *Model) managedTag(actor access.Actor, key string) (*tagRecord, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, access.Invalid("no such tag")
	}
	if t.owner != actor.Email && !slices.Contains(t.managers, actor.Email) {
		return nil, access.Forbidden("not your tag to manage")
	}
	return t, nil
}

func (m *Model) renameTag(actor access.Actor, key, to string) ([]store.Op, string, error) {
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

func (m *Model) copyTag(actor access.Actor, key, to string) ([]store.Op, string, int, error) {
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
	made := id.New(m.taken)
	ops := []store.Op{store.Insert(tagListTable, store.Row{tagID: made, tagOwner: actor.Email, tagName: to})}
	for _, person := range people {
		ops = append(ops, store.Insert(tagsTable, store.Row{tagID: made, tagPerson: person}))
	}
	return ops, made, len(people), nil
}

func (m *Model) shareTag(actor access.Actor, key, manager string, on bool) ([]store.Op, error) {
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

func (m *Model) leaveTag(actor access.Actor, key string) ([]store.Op, error) {
	t := m.tagByKey(key)
	if t == nil {
		return nil, nil
	}
	if t.owner == actor.Email {
		return nil, access.Invalid("you own this tag")
	}
	return []store.Op{store.Delete(managersTable, store.Row{tagID: t.id, managerEmail: actor.Email})}, nil
}

func (m *Model) dropTag(actor access.Actor, key string) ([]store.Op, int, error) {
	if m.tagByKey(key) == nil {
		return nil, 0, nil
	}
	t, err := m.ownTag(actor, key)
	if err != nil {
		return nil, 0, err
	}
	return []store.Op{store.Delete(tagListTable, store.Row{tagID: t.id})}, len(m.listed(t.people)), nil
}

func (m *Model) setTag(actor access.Actor, key, name, person string, on bool) ([]store.Op, string, error) {
	if m.Person(person) == nil {
		return nil, "", access.Invalid("no such person")
	}
	if key == "" {
		return m.tagNamed(actor, name, person, on)
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

func (m *Model) tagNamed(actor access.Actor, name, person string, on bool) ([]store.Op, string, error) {
	if !validTagName(name) || !on {
		return nil, "", access.Invalid("bad tag name")
	}
	if t := m.ownTagNamed(actor.Email, name); t != nil {
		return m.setTag(actor, t.id, "", person, true)
	}
	made := id.New(m.taken)
	return []store.Op{
		store.Insert(tagListTable, store.Row{tagID: made, tagOwner: actor.Email, tagName: name}),
		store.Insert(tagsTable, store.Row{tagID: made, tagPerson: person}),
	}, made, nil
}

func (i *Invites) saveGreeting(actor access.Actor, format, key string, grouped, individual bool) (string, []store.Op, error) {
	if format == "" {
		return "", nil, access.Invalid("format is required")
	}
	if len(format) > 200 {
		return "", nil, access.Invalid("format is too long")
	}
	row := store.Row{
		"Name":       format,
		"Format":     format,
		"Grouped":    cells.YesNoCell(grouped),
		"Individual": cells.YesNoCell(individual),
		"Email":      actor.Email,
	}
	if key == "" {
		row["Greeting ID"] = id.New(i.Model().taken)
		return row["Greeting ID"], []store.Op{store.Insert(greetingsTab, row)}, nil
	}
	have, ok := i.greeting(key)
	if !ok || have.CreatedBy != actor.Email {
		return "", nil, access.Forbidden("you can only edit greetings you created")
	}
	return have.ID, []store.Op{store.Update(greetingsTab, store.Row{"Greeting ID": have.ID}, row)}, nil
}

func (i *Invites) deleteGreeting(actor access.Actor, key string) ([]store.Op, error) {
	if key == "" {
		return nil, access.Invalid("id is required")
	}
	have, ok := i.greeting(key)
	if !ok || have.CreatedBy != actor.Email {
		return nil, access.Forbidden("you can only delete greetings you created")
	}
	return []store.Op{store.Delete(greetingsTab, store.Row{"Greeting ID": have.ID})}, nil
}

func (m *Model) locate(actor access.Actor, found map[string]geocode.Point) []store.Op {
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

var importedTabs = []string{studentsTab, staffTab, namesTab, WebsiteTable}

func importKey(tab string) (string, error) {
	if !slices.Contains(importedTabs, tab) {
		return "", access.Invalid("%s is not a tab the import writes", tab)
	}
	return Tabs[slices.IndexFunc(Tabs, func(t store.Tab) bool { return t.Name == tab })].Key[0], nil
}

func ImportInsert(actor access.Actor, tab string, cells store.Row) ([]store.Op, error) {
	if _, err := importKey(tab); err != nil {
		return nil, err
	}
	return []store.Op{store.Insert(tab, cells)}, nil
}

func ImportUpdate(actor access.Actor, tab, key string, cells store.Row) ([]store.Op, error) {
	column, err := importKey(tab)
	if err != nil {
		return nil, err
	}
	return []store.Op{store.Update(tab, store.Row{column: key}, cells)}, nil
}

func ImportDelete(actor access.Actor, tab, key string) ([]store.Op, error) {
	column, err := importKey(tab)
	if err != nil {
		return nil, err
	}
	return []store.Op{store.Delete(tab, store.Row{column: key})}, nil
}

func ClearCaughtUp(actor access.Actor, email string, cells store.Row) ([]store.Op, error) {
	for column := range cells {
		if column != "Facts" && column != "Facts Updated" && column != "Job Title" {
			return nil, access.Invalid("the import clears only facts and job titles, not %s", column)
		}
		if cells[column] != "" {
			return nil, access.Invalid("the import clears %s, it does not set it", column)
		}
	}
	return []store.Op{store.Update(overridesTab, store.Row{"Email": email}, cells)}, nil
}
