package model

import (
	"maps"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/cells"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	kindWhoSettings    = "who-settings"
	kindPersonRecord   = "person-record"
	whoSettingsType    = "who-settings"
	personRecordsType  = "person-records"
	greetingsType      = "greetings"
	inviteServicesType = "invite-services"
)

var uploadedName = regexp.MustCompile(`^[0-9a-f]{64}\.[a-z0-9]+$`)

func uploaded(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !uploadedName.MatchString(name) {
		return "", access.Invalid("%q is not an uploaded file", name)
	}
	return name, nil
}

func fieldsSent(wr api.Write[*Model]) []string {
	return slices.Sorted(maps.Keys(sent(wr.Body)))
}

type personEdit struct {
	PreferredName string `json:"preferredName"`
	Pronouns      string `json:"pronouns"`
	Facts         string `json:"facts"`
}

type familyEdit struct {
	PhotoCaption string `json:"photoCaption"`
}

type mediaBody struct {
	Name string `json:"name"`
}

type photoOrderBody struct {
	Names []string `json:"names"`
}

type photoCrop struct {
	Name string `json:"name"`
	Crop string `json:"crop"`
}

func editsPerson(m *Model, q api.Query, key string) bool {
	p := m.Directory.personByID(key)
	return p != nil && m.Directory.mayEdit(q.Actor, "person", p.Email) == nil
}

func editsFamily(m *Model, q api.Query, key string) bool {
	_, ok := m.Directory.Families[key]
	return ok && m.Directory.mayEdit(q.Actor, "family", key) == nil
}

func personActions(s *Store) map[string]api.Action[*Model] {
	return map[string]api.Action[*Model]{
		"edit": api.Do(editsPerson, func(wr api.Write[*Model], body personEdit) error {
			d := wr.S.Directory
			email := d.personByID(wr.ID).Email
			fields := sent(wr.Body)
			ops := []store.Op{}
			for _, f := range []struct{ name, field, value string }{{"preferredName", "preferred-name", body.PreferredName}, {"pronouns", "pronouns", body.Pronouns}} {
				if !fields[f.name] {
					continue
				}
				more, err := d.editField(wr.Query.Actor, f.field, email, strings.TrimSpace(f.value))
				if err != nil {
					return err
				}
				ops = append(ops, more...)
			}
			if fields["facts"] {
				more, err := d.setFacts(wr.Query.Actor, email, strings.TrimSpace(body.Facts))
				if err != nil {
					return err
				}
				ops = append(ops, more...)
			}
			logAfter(wr, "who: edited a person", "person", email, "fields", fieldsSent(wr))
			return s.stage(wr, DirectoryApp, ops, nil)
		}),
		"add-photo": api.Do(editsPerson, func(wr api.Write[*Model], body mediaBody) error {
			name, err := uploaded(body.Name)
			if err != nil {
				return err
			}
			email := wr.S.Directory.personByID(wr.ID).Email
			ops, err := wr.S.Directory.upload(wr.Query.Actor, "person", "photo", email, name)
			logAfter(wr, "who: added a photo", "person", email, "name", name)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"order-photos": api.Do(editsPerson, func(wr api.Write[*Model], body photoOrderBody) error {
			email := wr.S.Directory.personByID(wr.ID).Email
			ops, err := wr.S.Directory.reorderPhotos(wr.Query.Actor, email, body.Names)
			logAfter(wr, "who: set the photo list", "person", email, "photos", len(body.Names))
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"crop-photo": api.Do(editsPerson, func(wr api.Write[*Model], body photoCrop) error {
			crop, err := uploaded(body.Crop)
			if err != nil {
				return err
			}
			email := wr.S.Directory.personByID(wr.ID).Email
			ops, err := wr.S.Directory.cropPhoto(wr.Query.Actor, "person", email, body.Name, crop)
			logAfter(wr, "who: cropped a photo", "person", email, "photo", body.Name)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"pronunciation": api.Do(editsPerson, func(wr api.Write[*Model], body mediaBody) error {
			d := wr.S.Directory
			email := d.personByID(wr.ID).Email
			if strings.TrimSpace(body.Name) == "" {
				ops, err := d.editField(wr.Query.Actor, "pronunciation", email, "")
				logAfter(wr, "who: cleared a pronunciation", "person", email)
				return s.stage(wr, DirectoryApp, ops, err)
			}
			name, err := uploaded(body.Name)
			if err != nil {
				return err
			}
			ops, err := d.upload(wr.Query.Actor, "person", "pronunciation", email, name)
			logAfter(wr, "who: set a pronunciation", "person", email, "name", name)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
	}
}

func familyActions(s *Store) map[string]api.Action[*Model] {
	return map[string]api.Action[*Model]{
		"edit": api.Do(editsFamily, func(wr api.Write[*Model], body familyEdit) error {
			ops := []store.Op{}
			if sent(wr.Body)["photoCaption"] {
				more, err := wr.S.Directory.editField(wr.Query.Actor, "family-photo-caption", wr.ID, strings.TrimSpace(body.PhotoCaption))
				if err != nil {
					return err
				}
				ops = more
			}
			logAfter(wr, "who: edited a family", "family", wr.ID, "fields", fieldsSent(wr))
			return s.stage(wr, DirectoryApp, ops, nil)
		}),
		"photo": api.Do(editsFamily, func(wr api.Write[*Model], body mediaBody) error {
			name, err := uploaded(body.Name)
			if err != nil {
				return err
			}
			ops, err := wr.S.Directory.upload(wr.Query.Actor, "family", "photo", wr.ID, name)
			logAfter(wr, "who: set a family photo", "family", wr.ID, "name", name)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"crop-photo": api.Do(editsFamily, func(wr api.Write[*Model], body photoCrop) error {
			crop, err := uploaded(body.Crop)
			if err != nil {
				return err
			}
			ops, err := wr.S.Directory.cropPhoto(wr.Query.Actor, "family", wr.ID, "", crop)
			logAfter(wr, "who: cropped a family photo", "family", wr.ID)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"pronunciation": api.Do(editsFamily, func(wr api.Write[*Model], body mediaBody) error {
			d := wr.S.Directory
			if strings.TrimSpace(body.Name) == "" {
				ops, err := d.editField(wr.Query.Actor, "family-pronunciation", wr.ID, "")
				logAfter(wr, "who: cleared a family pronunciation", "family", wr.ID)
				return s.stage(wr, DirectoryApp, ops, err)
			}
			name, err := uploaded(body.Name)
			if err != nil {
				return err
			}
			ops, err := d.upload(wr.Query.Actor, "family", "pronunciation", wr.ID, name)
			logAfter(wr, "who: set a family pronunciation", "family", wr.ID, "name", name)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
	}
}

type tagCreate struct {
	Name   string `json:"name"`
	Person string `json:"person"`
}

type tagPersonBody struct {
	Person string `json:"person"`
}

type tagNameBody struct {
	Name string `json:"name"`
}

func (m *Directory) personEmail(key string) (string, error) {
	p := m.personByID(strings.TrimSpace(key))
	if p == nil {
		return "", access.Invalid("no such person")
	}
	return p.Email, nil
}

func ownsTag(m *Model, q api.Query, key string) bool {
	t := m.Directory.tagByKey(key)
	return t != nil && t.owner == q.Actor.Email
}

func managesTag(m *Model, q api.Query, key string) bool {
	t := m.Directory.tagByKey(key)
	return t != nil && (t.owner == q.Actor.Email || slices.Contains(t.managers, q.Actor.Email))
}

func leavesTag(m *Model, q api.Query, key string) bool {
	t := m.Directory.tagByKey(key)
	return t != nil && t.owner != q.Actor.Email && slices.Contains(t.managers, q.Actor.Email)
}

func tagMember(s *Store, on bool) func(wr api.Write[*Model], body tagPersonBody) error {
	return func(wr api.Write[*Model], body tagPersonBody) error {
		d := wr.S.Directory
		person, err := d.personEmail(body.Person)
		if err != nil {
			return err
		}
		ops, key, err := d.setTag(wr.Query.Actor, wr.ID, "", person, on, wr.Taken)
		logAfter(wr, "tag: changed", "on", on, "tag", key, "person", person)
		return s.stage(wr, DirectoryApp, ops, err)
	}
}

func tagManager(s *Store, on bool) func(wr api.Write[*Model], body tagPersonBody) error {
	return func(wr api.Write[*Model], body tagPersonBody) error {
		d := wr.S.Directory
		manager, err := d.personEmail(body.Person)
		if err != nil {
			return err
		}
		ops, err := d.shareTag(wr.Query.Actor, wr.ID, manager, on)
		logAfter(wr, "tag: shared", "on", on, "tag", wr.ID, "manager", manager)
		return s.stage(wr, DirectoryApp, ops, err)
	}
}

func tagCreator(s *Store) api.Maker[*Model] {
	return api.Make(func(wr api.Write[*Model], body tagCreate) (string, error) {
		d := wr.S.Directory
		person, err := d.personEmail(body.Person)
		if err != nil {
			return "", err
		}
		ops, key, err := d.setTag(wr.Query.Actor, "", strings.TrimSpace(body.Name), person, true, wr.Taken)
		if err := s.stage(wr, DirectoryApp, ops, err); err != nil {
			return "", err
		}
		logAfter(wr, "tag: changed", "on", true, "tag", key, "person", person)
		return key, nil
	})
}

func tagActions(s *Store) map[string]api.Action[*Model] {
	return map[string]api.Action[*Model]{
		"add":     api.Do(managesTag, tagMember(s, true)),
		"remove":  api.Do(managesTag, tagMember(s, false)),
		"share":   api.Do(ownsTag, tagManager(s, true)),
		"unshare": api.Do(ownsTag, tagManager(s, false)),
		"rename": api.Do(ownsTag, func(wr api.Write[*Model], body tagNameBody) error {
			to := strings.TrimSpace(body.Name)
			ops, from, err := wr.S.Directory.renameTag(wr.Query.Actor, wr.ID, to)
			logAfter(wr, "tag: renamed", "tag", wr.ID, "from", from, "to", to)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"copy": api.DoMaking(managesTag, func(wr api.Write[*Model], body tagNameBody) (string, error) {
			to := strings.TrimSpace(body.Name)
			ops, made, people, err := wr.S.Directory.copyTag(wr.Query.Actor, wr.ID, to, wr.Taken)
			if err := s.stage(wr, DirectoryApp, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "tag: copied", "from", wr.ID, "to", made, "name", to, "people", people)
			return made, nil
		}),
		"leave": api.Do(leavesTag, func(wr api.Write[*Model], _ serve.None) error {
			ops, err := wr.S.Directory.leaveTag(wr.Query.Actor, wr.ID)
			logAfter(wr, "tag: left", "tag", wr.ID)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
		"delete": api.Do(ownsTag, func(wr api.Write[*Model], _ serve.None) error {
			ops, people, err := wr.S.Directory.dropTag(wr.Query.Actor, wr.ID)
			logAfter(wr, "tag: deleted", "tag", wr.ID, "people", people)
			return s.stage(wr, DirectoryApp, ops, err)
		}),
	}
}

func administers(_ *Model, q api.Query, _ string) bool {
	return q.Actor.May(Administer)
}

func imageAction(s *Store, kind string, nameOf func(m *Model, key string) string) api.Action[*Model] {
	return api.Do(administers, func(wr api.Write[*Model], body mediaBody) error {
		image, err := uploaded(body.Name)
		if err != nil {
			return err
		}
		name := nameOf(wr.S, wr.ID)
		ops, err := wr.S.Directory.setImage(wr.Query.Actor, kind, name, image)
		logAfter(wr, "who: replaced an image", "kind", kind, "name", name)
		return s.stage(wr, DirectoryApp, ops, err)
	})
}

type recordValues struct {
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

type personRecordResource struct {
	Email     string        `json:"email"`
	FullName  string        `json:"fullName,omitempty"`
	IsStaff   bool          `json:"isStaff,omitempty"`
	IsStudent bool          `json:"isStudent,omitempty"`
	IsParent  bool          `json:"isParent,omitempty"`
	Added     bool          `json:"added,omitempty"`
	Hidden    bool          `json:"hidden,omitempty"`
	Override  *recordValues `json:"override,omitempty"`
	Veracross *recordValues `json:"veracross,omitempty"`
}

type recordFields struct {
	Email         string `json:"email"`
	FullName      string `json:"fullName"`
	LegalName     string `json:"legalName"`
	PreferredName string `json:"preferredName"`
	Facts         string `json:"facts"`
	Department    string `json:"department"`
	JobTitle      string `json:"jobTitle"`
	GradeBand     string `json:"gradeBand"`
	Grade         string `json:"grade"`
	Classroom     string `json:"classroom"`
	Crew          string `json:"crew"`
	Phone         string `json:"phone"`
	RoomParent    string `json:"roomParent"`
	Address       string `json:"address"`
	IsStudent     bool   `json:"isStudent"`
	IsParent      bool   `json:"isParent"`
	IsStaff       bool   `json:"isStaff"`
}

func (m *Model) records() map[string]string {
	s := m.scope
	s.recordsOnce.Do(func() {
		s.records = map[string]string{}
		for _, p := range m.Directory.People {
			if p.Email != "" {
				s.records[derived(m, kindPersonRecord, p.Email)] = p.Email
			}
		}
		for _, email := range m.Directory.hiddenEmails {
			s.records[derived(m, kindPersonRecord, email)] = email
		}
	})
	return s.records
}

func (m *Model) recordEmail(key string) (string, bool) {
	email, ok := m.records()[key]
	return email, ok
}

func (m *Directory) hidden(email string) bool {
	return slices.Contains(m.hiddenEmails, email)
}

func personRecord(d *Directory, email string) personRecordResource {
	p := d.Person(email)
	if p == nil {
		return personRecordResource{Email: email, Hidden: d.hidden(email)}
	}
	family, _ := d.FamilyOf(p.Email)
	return personRecordResource{
		Email: p.Email, FullName: p.FullName, IsStaff: p.IsStaff, IsStudent: p.IsStudent, IsParent: p.IsParent,
		Added: overrideBoolValue(p, "Added"),
		Override: &recordValues{
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
		Veracross: &recordValues{
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
	}
}

func recordWith(when func(d *Directory, email string) bool) func(m *Model, q api.Query, key string) bool {
	return func(m *Model, q api.Query, key string) bool {
		email, ok := m.recordEmail(key)
		return ok && q.Actor.May(Administer) && when(m.Directory, email)
	}
}

func listedRecord(d *Directory, email string) bool {
	return d.Person(email) != nil
}

func personRecordResources(s *Store) api.Type[*Model] {
	return api.Type[*Model]{
		Name:  personRecordsType,
		Shape: personRecordResource{},
		Has: func(m *Model, key string) bool {
			_, ok := m.recordEmail(key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			email, ok := m.recordEmail(key)
			if !ok || !q.Actor.May(Administer) {
				return nil, false
			}
			return personRecord(m.Directory, email), true
		},
		List: func(m *Model, q api.Query) []string {
			if !q.Actor.May(Administer) {
				return []string{}
			}
			out := []string{}
			for _, p := range m.Directory.People {
				if p.Email != "" {
					out = append(out, derived(m, kindPersonRecord, p.Email))
				}
			}
			for _, email := range m.Directory.hiddenEmails {
				out = append(out, derived(m, kindPersonRecord, email))
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"person": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				email, ok := m.recordEmail(key)
				if !ok {
					return nil
				}
				return m.personID(email)
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], body newPerson) (string, error) {
			email, fullName, ops, err := wr.S.Directory.addPerson(wr.Query.Actor, body)
			if err := s.stage(wr, DirectoryApp, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "who: added a new person", "email", email, "name", fullName)
			return derived(wr.S, kindPersonRecord, email), nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(recordWith(listedRecord), func(wr api.Write[*Model], body recordFields) error {
				email, _ := wr.S.recordEmail(wr.ID)
				ops, err := wr.S.Directory.setRecordFields(wr.Query.Actor, email, body, sent(wr.Body))
				logAfter(wr, "who: edited a person's record", "target", email, "fields", fieldsSent(wr))
				return s.stage(wr, DirectoryApp, ops, err)
			}),
			"hide": api.Do(recordWith(listedRecord), func(wr api.Write[*Model], _ serve.None) error {
				email, _ := wr.S.recordEmail(wr.ID)
				target, ops, err := wr.S.Directory.hidePerson(wr.Query.Actor, email)
				logAfter(wr, "who: hid a person from the directory", "target", target)
				return s.stage(wr, DirectoryApp, ops, err)
			}),
			"unhide": api.Do(recordWith((*Directory).hidden), func(wr api.Write[*Model], _ serve.None) error {
				email, _ := wr.S.recordEmail(wr.ID)
				target, ops, err := wr.S.Directory.unhidePerson(wr.Query.Actor, email)
				logAfter(wr, "who: unhid a person", "target", target)
				return s.stage(wr, DirectoryApp, ops, err)
			}),
			"delete": api.Do(recordWith(func(d *Directory, email string) bool {
				p := d.Person(email)
				return p != nil && overrideBoolValue(p, "Added")
			}), func(wr api.Write[*Model], _ serve.None) error {
				email, _ := wr.S.recordEmail(wr.ID)
				target, ops, err := wr.S.Directory.deletePerson(wr.Query.Actor, email)
				logAfter(wr, "who: deleted an added person", "target", target)
				return s.stage(wr, DirectoryApp, ops, err)
			}),
		},
	}
}

type whoSettingsResource struct {
	MapsKey         string            `json:"mapsKey"`
	StaleYears      StaleYears        `json:"staleYears"`
	PrivacyLinks    PrivacyLinks      `json:"privacyLinks"`
	StaffColor      string            `json:"staffColor"`
	GradeColors     map[string]string `json:"gradeColors"`
	ClassroomColors map[string]string `json:"classroomColors"`
}

type colorBody struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func configures(_ *Model, q api.Query, _ string) bool {
	return q.Actor.May(Configure)
}

func whoSettingsResources(s *Store, mapsKey string) api.Type[*Model] {
	return api.Type[*Model]{
		Name:  whoSettingsType,
		Shape: whoSettingsResource{},
		Has:   func(m *Model, key string) bool { return key == derived(m, kindWhoSettings, "") },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			if key != derived(m, kindWhoSettings, "") {
				return nil, false
			}
			c := m.Config
			return whoSettingsResource{MapsKey: mapsKey, StaleYears: c.StaleYears, PrivacyLinks: c.PrivacyLinks, StaffColor: c.StaffColor, GradeColors: c.GradeColors, ClassroomColors: c.ClassroomColors}, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{derived(m, kindWhoSettings, "")} },
		Relations: map[string]api.Relation[*Model]{
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
		},
		Actions: map[string]api.Action[*Model]{
			"stale-years": api.Do(configures, func(wr api.Write[*Model], body StaleYears) error {
				ops, err := wr.S.Config.setStaleYears(wr.Query.Actor, body)
				logAfter(wr, "config: set stale-years thresholds", "photo", body.Photo, "facts", body.Facts, "familyPhoto", body.FamilyPhoto)
				return s.stage(wr, ConfigApp, ops, err)
			}),
			"privacy-links": api.Do(configures, func(wr api.Write[*Model], body PrivacyLinks) error {
				links, ops, err := wr.S.Config.setPrivacyLinks(wr.Query.Actor, body)
				logAfter(wr, "config: set privacy links", "veracrossPreferences", links.VeracrossPreferences, "heliosWhoOptIn", links.HeliosWhoOptIn)
				return s.stage(wr, ConfigApp, ops, err)
			}),
			"color": api.Do(configures, func(wr api.Write[*Model], body colorBody) error {
				name, ops, err := wr.S.Config.setColor(wr.Query.Actor, body.Kind, body.Name, body.Color)
				logAfter(wr, "config: set color", "kind", body.Kind, "name", name, "color", body.Color)
				return s.stage(wr, ConfigApp, ops, err)
			}),
		},
	}
}

type greetingResource struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	Grouped    bool   `json:"grouped"`
	Individual bool   `json:"individual"`
	Role       string `json:"role,omitempty"`
	Me         tagMe  `json:"me"`
}

type greetingBody struct {
	Format     string `json:"format"`
	Grouped    bool   `json:"grouped"`
	Individual bool   `json:"individual"`
}

var greetingRoles = map[string]string{
	builtinGreetings.Default:     "default",
	builtinGreetings.WholeFamily: "wholeFamily",
	builtinGreetings.Kids:        "kids",
	builtinGreetings.Adults:      "adults",
	builtinGreetings.FullName:    "fullName",
	builtinGreetings.FirstName:   "firstName",
}

func (m *Model) greetingFor(q api.Query, key string) (GreetingTemplate, bool) {
	g, ok := m.Invites.greeting(key)
	if !ok || (g.CreatedBy != "" && g.CreatedBy != q.Actor.Email) {
		return GreetingTemplate{}, false
	}
	return g, true
}

func ownsGreeting(m *Model, q api.Query, key string) bool {
	g, ok := m.Invites.greeting(key)
	return ok && g.CreatedBy == q.Actor.Email
}

func greetingResources(s *Store) api.Type[*Model] {
	return api.Type[*Model]{
		Name:  greetingsType,
		Shape: greetingResource{},
		Has: func(m *Model, key string) bool {
			_, ok := m.Invites.greeting(key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			g, ok := m.greetingFor(q, key)
			if !ok {
				return nil, false
			}
			return greetingResource{Name: g.Name, Format: g.Format, Grouped: g.Grouped, Individual: g.Individual, Role: greetingRoles[g.ID], Me: tagMe{Mine: g.CreatedBy == q.Actor.Email}}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, g := range visibleGreetings(m.Invites.Greetings, q.Actor.Email) {
				out = append(out, g.ID)
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body greetingBody) (string, error) {
			format := strings.TrimSpace(body.Format)
			key, ops, err := wr.S.Invites.saveGreeting(wr.Query.Actor, format, "", body.Grouped, body.Individual, wr.Taken)
			if err := s.stage(wr, invitesApp, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "greeting: saved", "id", key, "name", format)
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(ownsGreeting, func(wr api.Write[*Model], body greetingBody) error {
				format := strings.TrimSpace(body.Format)
				key, ops, err := wr.S.Invites.saveGreeting(wr.Query.Actor, format, wr.ID, body.Grouped, body.Individual, wr.Taken)
				logAfter(wr, "greeting: saved", "id", key, "name", format)
				return s.stage(wr, invitesApp, ops, err)
			}),
			"delete": api.Do(ownsGreeting, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Invites.deleteGreeting(wr.Query.Actor, wr.ID)
				logAfter(wr, "greeting: deleted", "id", wr.ID)
				return s.stage(wr, invitesApp, ops, err)
			}),
		},
	}
}

type inviteServiceResource struct {
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	HeaderRow      bool             `json:"headerRow"`
	SupportsGroups bool             `json:"supportsGroups"`
	Columns        []TemplateColumn `json:"columns"`
}

func (m *InviteTemplates) service(key string) (InviteTemplate, bool) {
	for _, s := range m.Systems {
		if s.ID == key {
			return s, true
		}
	}
	return InviteTemplate{}, false
}

func inviteServiceResources() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  inviteServicesType,
		Shape: inviteServiceResource{},
		Has: func(m *Model, key string) bool {
			_, ok := m.Invites.service(key)
			return ok
		},
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			s, ok := m.Invites.service(key)
			if !ok {
				return nil, false
			}
			return inviteServiceResource{Name: s.Name, Description: s.Description, HeaderRow: s.HeaderRow, SupportsGroups: s.SupportsGroups, Columns: s.Columns}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, s := range m.Invites.Systems {
				out = append(out, s.ID)
			}
			return out
		},
	}
}

func WhoResources(s *Store, mapsKey string) []api.Type[*Model] {
	return []api.Type[*Model]{whoSettingsResources(s, mapsKey), personRecordResources(s), greetingResources(s), inviteServiceResources()}
}
