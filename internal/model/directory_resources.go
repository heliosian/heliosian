package model

import (
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
)

const whoHost = "who"

type personResource struct {
	*Person
	Email        string `json:"email,omitempty"`
	HeroPhotoURL string `json:"heroPhotoUrl,omitempty"`
	Words        string `json:"words,omitempty"`
	Slug         string `json:"slug"`
	Path         string `json:"path"`
	App          string `json:"app"`
}

type familyResource struct {
	Name             string  `json:"name,omitempty"`
	ShortName        string  `json:"shortName,omitempty"`
	Address          string  `json:"address,omitempty"`
	Phone            string  `json:"phone,omitempty"`
	Lat              float64 `json:"lat,omitempty"`
	Lng              float64 `json:"lng,omitempty"`
	PhotoURL         string  `json:"photoUrl,omitempty"`
	OriginalPhotoURL string  `json:"originalPhotoUrl,omitempty"`
	PhotoCaption     string  `json:"photoCaption,omitempty"`
	PhotoUpdated     string  `json:"photoUpdated,omitempty"`
	PronunciationURL string  `json:"pronunciationUrl,omitempty"`
	AddressMasked    bool    `json:"addressMasked,omitempty"`
	PhoneMasked      bool    `json:"phoneMasked,omitempty"`
	VeracrossAddress string  `json:"veracrossAddress"`
	VeracrossPhone   string  `json:"veracrossPhone"`
	Path             string  `json:"path"`
	App              string  `json:"app"`
}

type classroomResource struct {
	Name     string   `json:"name"`
	ImageURL string   `json:"imageUrl,omitempty"`
	HasCrews bool     `json:"hasCrews"`
	Band     string   `json:"band,omitempty"`
	Grades   []string `json:"grades"`
	Slug     string   `json:"slug"`
	Path     string   `json:"path"`
	App      string   `json:"app"`
}

type gradeResource struct {
	Name     string `json:"name"`
	NextName string `json:"nextName,omitempty"`
	Band     string `json:"band,omitempty"`
	NextBand string `json:"nextBand,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Slug     string `json:"slug"`
	Path     string `json:"path"`
	App      string `json:"app"`
}

type crewResource struct {
	Name      string `json:"name,omitempty"`
	GradeBand string `json:"gradeBand,omitempty"`
	Path      string `json:"path"`
	App       string `json:"app"`
}

func GradePath(name string) string {
	return "/grades/" + ClassroomSlug(name)
}

type departmentResource struct {
	Name string `json:"name"`
}

type tagMe struct {
	Mine bool `json:"mine"`
}

type tagResource struct {
	Name      string `json:"name"`
	OwnerName string `json:"ownerName"`
	Path      string `json:"path"`
	App       string `json:"app"`
	Me        tagMe  `json:"me"`
}

func DirectoryResources() []api.Type[*Model] {
	return []api.Type[*Model]{peopleType(), familiesType(), classroomsType(), gradesType(), crewsType(), departmentsType(), tagsType()}
}

func (m *Directory) tagFor(key string, viewer string) (Tag, bool) {
	t, ok := m.Tag(key)
	if !ok || (t.Owner != viewer && !slices.Contains(t.Managers, viewer)) {
		return Tag{}, false
	}
	return t, true
}

func (m *Directory) emailIDs(emails []string) []string {
	out := []*Person{}
	for _, email := range emails {
		if p := m.Person(email); p != nil {
			out = append(out, p)
		}
	}
	return ids(out)
}

func tagRelation(target string, many bool, list func(t Tag) []string) api.Relation[*Model] {
	return api.Relation[*Model]{Type: target, Many: many, List: func(m *Model, q api.Query, key string) []string {
		t, ok := m.Directory.tagFor(key, q.Actor.Email)
		if !ok {
			return nil
		}
		return m.Directory.emailIDs(list(t))
	}}
}

func tagsType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "tags",
		Shape: tagResource{},
		Has:   func(m *Model, key string) bool { return m.Directory.tagByKey(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			t, ok := m.Directory.tagFor(key, q.Actor.Email)
			if !ok {
				return nil, false
			}
			return tagResource{Name: t.Name, OwnerName: t.OwnerName, Path: TagPath(t.ID), App: whoHost, Me: tagMe{Mine: t.Owner == q.Actor.Email}}, true
		},
		List: func(m *Model, q api.Query) []string {
			d := m.Directory
			out := []string{}
			for _, t := range append(d.Tags(q.Actor.Email), d.SharedTags(q.Actor.Email)...) {
				out = append(out, t.ID)
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"owner":    tagRelation("people", false, func(t Tag) []string { return []string{t.Owner} }),
			"people":   tagRelation("people", true, func(t Tag) []string { return t.People }),
			"managers": tagRelation("people", true, func(t Tag) []string { return t.Managers }),
		},
	}
}

func departmentsType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "departments",
		Shape: departmentResource{},
		Has:   func(m *Model, key string) bool { return slices.Contains(m.Directory.departmentIDs, key) },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			i := slices.Index(m.Directory.departmentIDs, key)
			if i < 0 {
				return nil, false
			}
			return departmentResource{Name: m.Directory.Departments[i]}, true
		},
		List: func(m *Model, _ api.Query) []string { return slices.Clone(m.Directory.departmentIDs) },
	}
}

func (m *Directory) personByID(key string) *Person {
	i, ok := m.byID[key]
	if !ok {
		return nil
	}
	return &m.People[i]
}

func ids(people []*Person) []string {
	out := []string{}
	for _, p := range people {
		if p.ID != "" {
			out = append(out, p.ID)
		}
	}
	return out
}

func (m *Directory) peopleWhere(keep func(p *Person) bool) []string {
	out := []*Person{}
	for i := range m.People {
		if keep(&m.People[i]) {
			out = append(out, &m.People[i])
		}
	}
	return ids(out)
}

func noValue(name, value string) error {
	if value != "" && value != "true" {
		return access.Invalid("%s takes no value", name)
	}
	return nil
}

func (m *Directory) PersonAliases() map[string]*Person {
	out := map[string]*Person{}
	slugs := map[string]int{}
	for i := range m.People {
		p := &m.People[i]
		if p.ID == "" {
			continue
		}
		out[p.Email] = p
		slugs[Slug(p.Email)]++
	}
	for i := range m.People {
		p := &m.People[i]
		if p.ID != "" && slugs[Slug(p.Email)] == 1 {
			out[Slug(p.Email)] = p
		}
	}
	for alias, email := range m.aliases {
		if p := m.Person(email); p != nil && p.ID != "" {
			out[alias] = p
		}
	}
	return out
}

func personRelation(list func(d *Directory, p *Person) []string) api.Relation[*Model] {
	return api.Relation[*Model]{Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
		p := m.Directory.personByID(key)
		if p == nil {
			return nil
		}
		return list(m.Directory, p)
	}}
}

func personTo(target string, list func(d *Directory, p *Person) []string) api.Relation[*Model] {
	rel := personRelation(list)
	rel.Type, rel.Many = target, false
	return rel
}

func peopleType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "people",
		Shape: personResource{},
		Has:   func(m *Model, key string) bool { return m.Directory.personByID(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			d := m.Directory
			p := d.personByID(key)
			if p == nil {
				return nil, false
			}
			out := personResource{Person: p, HeroPhotoURL: d.HeroPhoto(p.Email), Words: p.Words(), Slug: Slug(p.Email), Path: PersonPath(p.Email), App: whoHost}
			if !p.EmailMasked {
				out.Email = p.Email
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string {
			d := m.Directory
			out := []*Person{}
			for i := range d.People {
				if d.People[i].ID != "" {
					out = append(out, &d.People[i])
				}
			}
			slices.SortStableFunc(out, func(a, b *Person) int {
				return strings.Compare(strings.ToLower(a.FullName), strings.ToLower(b.FullName))
			})
			return ids(out)
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for alias, p := range m.Directory.PersonAliases() {
				out[alias] = p.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"families": {Type: "families", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				p := m.Directory.personByID(key)
				if p == nil {
					return nil
				}
				return slices.Clone(m.Directory.FamilyKeysOf(p.Email))
			}},
			"parents":  personRelation(func(d *Directory, p *Person) []string { return ids(d.Parents(p.Email)) }),
			"children": personRelation(func(d *Directory, p *Person) []string { return ids(d.Children(p.Email)) }),
			"partners": personRelation(func(d *Directory, p *Person) []string {
				if !p.IsParent {
					return nil
				}
				adults, _ := d.Household(p.Email)
				return ids(adults)
			}),
			"siblings": personRelation(func(d *Directory, p *Person) []string {
				if !p.IsStudent {
					return nil
				}
				_, kids := d.Household(p.Email)
				return ids(kids)
			}),
			"household": personRelation(func(d *Directory, p *Person) []string {
				adults, kids := d.Household(p.Email)
				return ids(append(adults, kids...))
			}),
			"classroom": personTo("classrooms", func(d *Directory, p *Person) []string {
				for _, c := range d.Classrooms {
					if c.Name == p.Classroom {
						return []string{c.ID}
					}
				}
				return nil
			}),
			"grade": personTo("grades", func(d *Directory, p *Person) []string {
				for _, g := range d.Grades {
					if g.Name == p.Grade {
						return []string{g.ID}
					}
				}
				return nil
			}),
			"crew": personTo("crews", func(d *Directory, p *Person) []string {
				for _, c := range d.Crews {
					if p.Classroom != "" && c.Classroom == p.Classroom && c.Name == p.Crew {
						return []string{c.ID}
					}
				}
				return nil
			}),
			"room-parent-for": {Type: "grades", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				d := m.Directory
				p := d.personByID(key)
				if p == nil {
					return nil
				}
				out := []string{}
				for _, g := range d.Grades {
					if g.Band != "" && slices.Contains(d.RoomParentsOf(g.Band), p.Email) {
						out = append(out, g.ID)
					}
				}
				return out
			}},
		},
		Filters: map[string]api.Filter[*Model]{
			"listed": func(m *Model, _ api.Query, value string) (func(string) bool, error) {
				if err := noValue("listed", value); err != nil {
					return nil, err
				}
				d := m.Directory
				return func(key string) bool {
					p := d.personByID(key)
					return p != nil && !p.EmailMasked
				}, nil
			},
			"q": personWhere(func(_ *Directory, p *Person, value string) bool {
				email := p.Email
				if p.EmailMasked {
					email = ""
				}
				return mentions(value, p.FullName, p.PreferredName, p.JobTitle, email)
			}),
			"role": func(m *Model, _ api.Query, value string) (func(string) bool, error) {
				is := map[string]func(p *Person) bool{
					"student": func(p *Person) bool { return p.IsStudent },
					"parent":  func(p *Person) bool { return p.IsParent },
					"staff":   func(p *Person) bool { return p.IsStaff },
				}[value]
				if is == nil {
					return nil, access.Invalid("role takes student, parent or staff")
				}
				d := m.Directory
				return func(key string) bool {
					p := d.personByID(key)
					return p != nil && is(p)
				}, nil
			},
			"grade": personWhere(func(d *Directory, p *Person, value string) bool {
				return slices.ContainsFunc(d.placesOf(p, func(q *Person) string { return q.Grade }), func(g string) bool { return strings.EqualFold(g, value) })
			}),
			"classroom": personWhere(func(d *Directory, p *Person, value string) bool {
				return slices.ContainsFunc(d.placesOf(p, func(q *Person) string { return q.Classroom }), func(c string) bool { return strings.EqualFold(c, value) })
			}),
			"department": personWhere(func(_ *Directory, p *Person, value string) bool {
				return mentions(value, p.Department)
			}),
		},
	}
}

func personWhere(keep func(d *Directory, p *Person, value string) bool) api.Filter[*Model] {
	return func(m *Model, _ api.Query, value string) (func(string) bool, error) {
		value = strings.TrimSpace(value)
		d := m.Directory
		return func(key string) bool {
			p := d.personByID(key)
			return p != nil && keep(d, p, value)
		}, nil
	}
}

func (m *Directory) placesOf(p *Person, place func(*Person) string) []string {
	out := []string{}
	if own := place(p); own != "" && (p.IsStudent || p.IsStaff) {
		out = append(out, own)
	}
	for _, k := range m.Children(p.Email) {
		if place(k) != "" {
			out = append(out, place(k))
		}
	}
	return out
}

func familyMembers(adults bool) api.Relation[*Model] {
	return api.Relation[*Model]{Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
		grown, kids := m.Directory.Members(key)
		if adults {
			return ids(grown)
		}
		return ids(kids)
	}}
}

func familiesType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "families",
		Shape: familyResource{},
		Has:   func(m *Model, key string) bool { _, ok := m.Directory.Families[key]; return ok },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			f, ok := m.Directory.Families[key]
			if !ok {
				return nil, false
			}
			return familyResource{
				Name: f.Name, ShortName: f.ShortName, Address: f.Address, Phone: f.Phone, Lat: f.Lat, Lng: f.Lng,
				PhotoURL: f.PhotoURL, OriginalPhotoURL: f.OriginalPhotoURL, PhotoCaption: f.PhotoCaption, PhotoUpdated: f.PhotoUpdated,
				PronunciationURL: f.PronunciationURL, AddressMasked: f.AddressMasked, PhoneMasked: f.PhoneMasked,
				VeracrossAddress: f.VeracrossAddress, VeracrossPhone: f.VeracrossPhone,
				Path: FamilyPath(key), App: whoHost,
			}, true
		},
		List: func(m *Model, _ api.Query) []string {
			d := m.Directory
			out := []string{}
			for key := range d.Families {
				out = append(out, key)
			}
			slices.SortFunc(out, func(a, b string) int {
				if c := strings.Compare(d.Families[a].Name, d.Families[b].Name); c != 0 {
					return c
				}
				return strings.Compare(a, b)
			})
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"adults": familyMembers(true),
			"kids":   familyMembers(false),
		},
		Filters: familyFilters,
	}
}

func (m *Directory) classroomByID(key string) *Classroom {
	for i := range m.Classrooms {
		if m.Classrooms[i].ID == key {
			return &m.Classrooms[i]
		}
	}
	return nil
}

func (m *Directory) gradeByID(key string) *Grade {
	for i := range m.Grades {
		if m.Grades[i].ID == key {
			return &m.Grades[i]
		}
	}
	return nil
}

func (m *Directory) crewByID(key string) *Crew {
	for i := range m.Crews {
		if m.Crews[i].ID == key {
			return &m.Crews[i]
		}
	}
	return nil
}

func classroomsType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "classrooms",
		Shape: classroomResource{},
		Has:   func(m *Model, key string) bool { return m.Directory.classroomByID(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			c := m.Directory.classroomByID(key)
			if c == nil {
				return nil, false
			}
			out := classroomResource{Name: c.Name, ImageURL: c.ImageURL, HasCrews: c.HasCrews, Grades: []string{}, Slug: ClassroomSlug(c.Name), Path: ClassroomPath(c.Name), App: whoHost}
			if r := m.Directory.Roster().byID(key); r != nil {
				out.Band, out.Grades = r.Band, r.Grades
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Directory.Classrooms {
				out = append(out, c.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, c := range m.Directory.Classrooms {
				out[ClassroomSlug(c.Name)] = c.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"students": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				c := m.Directory.classroomByID(key)
				if c == nil {
					return nil
				}
				return m.Directory.peopleWhere(func(p *Person) bool { return p.IsStudent && p.Classroom == c.Name })
			}},
			"crews": {Type: "crews", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				c := m.Directory.classroomByID(key)
				if c == nil {
					return nil
				}
				out := []string{}
				for _, crew := range m.Directory.Crews {
					if crew.Classroom == c.Name {
						out = append(out, crew.ID)
					}
				}
				return out
			}},
			"teachers": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				c := m.Directory.classroomByID(key)
				if c == nil {
					return nil
				}
				return ids(m.Directory.teachersOf(c.Name))
			}},
			"room-parents": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				r := m.Directory.Roster().byID(key)
				if r == nil || r.Band == "" {
					return nil
				}
				return m.Directory.emailIDs(m.Directory.RoomParentsOf(r.Band))
			}},
		},
	}
}

func (m *Directory) teachersOf(classroom string) []*Person {
	out := []*Person{}
	add := func(p *Person) {
		if p != nil && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, c := range m.Crews {
		if c.Classroom == classroom {
			for _, t := range c.Teachers {
				add(m.Person(t))
			}
		}
	}
	for i := range m.People {
		if m.People[i].IsStaff && m.People[i].Classroom == classroom {
			add(&m.People[i])
		}
	}
	return out
}

func gradesType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "grades",
		Shape: gradeResource{},
		Has:   func(m *Model, key string) bool { return m.Directory.gradeByID(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			g := m.Directory.gradeByID(key)
			if g == nil {
				return nil, false
			}
			return gradeResource{Name: g.Name, NextName: g.NextName, Band: g.Band, NextBand: g.NextBand, ImageURL: g.ImageURL, Slug: ClassroomSlug(g.Name), Path: GradePath(g.Name), App: whoHost}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, g := range m.Directory.Grades {
				out = append(out, g.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, g := range m.Directory.Grades {
				out[ClassroomSlug(g.Name)] = g.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"students": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				g := m.Directory.gradeByID(key)
				if g == nil {
					return nil
				}
				return m.Directory.peopleWhere(func(p *Person) bool { return p.IsStudent && p.Grade == g.Name })
			}},
			"room-parents": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				g := m.Directory.gradeByID(key)
				if g == nil || g.Band == "" {
					return nil
				}
				return m.Directory.emailIDs(m.Directory.RoomParentsOf(g.Band))
			}},
		},
		Filters: map[string]api.Filter[*Model]{
			"enrolled": func(m *Model, _ api.Query, value string) (func(string) bool, error) {
				if err := noValue("enrolled", value); err != nil {
					return nil, err
				}
				d := m.Directory
				enrolled := map[string]bool{}
				for _, p := range d.People {
					if p.IsStudent && p.Grade != "" {
						enrolled[p.Grade] = true
					}
				}
				return func(key string) bool {
					g := d.gradeByID(key)
					return g != nil && enrolled[g.Name]
				}, nil
			},
		},
	}
}

func crewsType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "crews",
		Shape: crewResource{},
		Has:   func(m *Model, key string) bool { return m.Directory.crewByID(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			c := m.Directory.crewByID(key)
			if c == nil {
				return nil, false
			}
			return crewResource{Name: c.Name, GradeBand: c.GradeBand, Path: ClassroomPath(c.Classroom), App: whoHost}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Directory.Crews {
				out = append(out, c.ID)
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"classroom": {Type: "classrooms", List: func(m *Model, _ api.Query, key string) []string {
				c := m.Directory.crewByID(key)
				if c == nil {
					return nil
				}
				for _, room := range m.Directory.Classrooms {
					if room.Name == c.Classroom {
						return []string{room.ID}
					}
				}
				return nil
			}},
			"teachers": {Type: "people", Many: true, List: func(m *Model, _ api.Query, key string) []string {
				c := m.Directory.crewByID(key)
				if c == nil {
					return nil
				}
				out := []*Person{}
				for _, email := range c.Teachers {
					if p := m.Directory.Person(email); p != nil {
						out = append(out, p)
					}
				}
				return ids(out)
			}},
		},
	}
}
