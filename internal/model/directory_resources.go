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
	Path             string  `json:"path"`
	App              string  `json:"app"`
}

type classroomResource struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	HasCrews bool   `json:"hasCrews"`
	Slug     string `json:"slug"`
	Path     string `json:"path"`
	App      string `json:"app"`
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
	Me        tagMe  `json:"me"`
}

func DirectoryResources() []api.Type[*Directory] {
	return []api.Type[*Directory]{peopleType(), familiesType(), classroomsType(), gradesType(), crewsType(), departmentsType(), tagsType()}
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

func tagRelation(target string, many bool, list func(t Tag) []string) api.Relation[*Directory] {
	return api.Relation[*Directory]{Type: target, Many: many, List: func(m *Directory, q api.Query, key string) []string {
		t, ok := m.tagFor(key, q.Actor.Email)
		if !ok {
			return nil
		}
		return m.emailIDs(list(t))
	}}
}

func tagsType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "tags",
		Shape: tagResource{},
		Has:   func(m *Directory, key string) bool { return m.tagByKey(key) != nil },
		Get: func(m *Directory, q api.Query, key string) (any, bool) {
			t, ok := m.tagFor(key, q.Actor.Email)
			if !ok {
				return nil, false
			}
			return tagResource{Name: t.Name, OwnerName: t.OwnerName, Me: tagMe{Mine: t.Owner == q.Actor.Email}}, true
		},
		List: func(m *Directory, q api.Query) []string {
			out := []string{}
			for _, t := range append(m.Tags(q.Actor.Email), m.SharedTags(q.Actor.Email)...) {
				out = append(out, t.ID)
			}
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"owner":    tagRelation("people", false, func(t Tag) []string { return []string{t.Owner} }),
			"people":   tagRelation("people", true, func(t Tag) []string { return t.People }),
			"managers": tagRelation("people", true, func(t Tag) []string { return t.Managers }),
		},
	}
}

func departmentsType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "departments",
		Shape: departmentResource{},
		Has:   func(m *Directory, key string) bool { return slices.Contains(m.departmentIDs, key) },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			i := slices.Index(m.departmentIDs, key)
			if i < 0 {
				return nil, false
			}
			return departmentResource{Name: m.Departments[i]}, true
		},
		List: func(m *Directory, _ api.Query) []string { return slices.Clone(m.departmentIDs) },
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

func personRelation(list func(m *Directory, p *Person) []string) api.Relation[*Directory] {
	return api.Relation[*Directory]{Type: "people", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
		p := m.personByID(key)
		if p == nil {
			return nil
		}
		return list(m, p)
	}}
}

func personTo(target string, list func(m *Directory, p *Person) []string) api.Relation[*Directory] {
	rel := personRelation(list)
	rel.Type, rel.Many = target, false
	return rel
}

func peopleType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "people",
		Shape: personResource{},
		Has:   func(m *Directory, key string) bool { return m.personByID(key) != nil },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			p := m.personByID(key)
			if p == nil {
				return nil, false
			}
			out := personResource{Person: p, HeroPhotoURL: m.HeroPhoto(p.Email), Words: p.Words(), Slug: Slug(p.Email), Path: PersonPath(p.Email), App: whoHost}
			if !p.EmailMasked {
				out.Email = p.Email
			}
			return out, true
		},
		List: func(m *Directory, _ api.Query) []string {
			out := []*Person{}
			for i := range m.People {
				if m.People[i].ID != "" {
					out = append(out, &m.People[i])
				}
			}
			slices.SortStableFunc(out, func(a, b *Person) int {
				return strings.Compare(strings.ToLower(a.FullName), strings.ToLower(b.FullName))
			})
			return ids(out)
		},
		Aliases: func(m *Directory) map[string]string {
			out := map[string]string{}
			for alias, p := range m.PersonAliases() {
				out[alias] = p.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"families": {Type: "families", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
				p := m.personByID(key)
				if p == nil {
					return nil
				}
				return slices.Clone(m.FamilyKeysOf(p.Email))
			}},
			"parents":  personRelation(func(m *Directory, p *Person) []string { return ids(m.Parents(p.Email)) }),
			"children": personRelation(func(m *Directory, p *Person) []string { return ids(m.Children(p.Email)) }),
			"partners": personRelation(func(m *Directory, p *Person) []string {
				if !p.IsParent {
					return nil
				}
				adults, _ := m.Household(p.Email)
				return ids(adults)
			}),
			"siblings": personRelation(func(m *Directory, p *Person) []string {
				if !p.IsStudent {
					return nil
				}
				_, kids := m.Household(p.Email)
				return ids(kids)
			}),
			"household": personRelation(func(m *Directory, p *Person) []string {
				adults, kids := m.Household(p.Email)
				return ids(append(adults, kids...))
			}),
			"classroom": personTo("classrooms", func(m *Directory, p *Person) []string {
				for _, c := range m.Classrooms {
					if c.Name == p.Classroom {
						return []string{c.ID}
					}
				}
				return nil
			}),
			"grade": personTo("grades", func(m *Directory, p *Person) []string {
				for _, g := range m.Grades {
					if g.Name == p.Grade {
						return []string{g.ID}
					}
				}
				return nil
			}),
			"crew": personTo("crews", func(m *Directory, p *Person) []string {
				for _, c := range m.Crews {
					if p.Classroom != "" && c.Classroom == p.Classroom && c.Name == p.Crew {
						return []string{c.ID}
					}
				}
				return nil
			}),
		},
		Filters: map[string]api.Filter[*Directory]{
			"listed": func(m *Directory, _ api.Query, value string) (func(string) bool, error) {
				if err := noValue("listed", value); err != nil {
					return nil, err
				}
				return func(key string) bool {
					p := m.personByID(key)
					return p != nil && !p.EmailMasked
				}, nil
			},
		},
	}
}

func familyMembers(adults bool) api.Relation[*Directory] {
	return api.Relation[*Directory]{Type: "people", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
		grown, kids := m.Members(key)
		if adults {
			return ids(grown)
		}
		return ids(kids)
	}}
}

func familiesType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "families",
		Shape: familyResource{},
		Has:   func(m *Directory, key string) bool { _, ok := m.Families[key]; return ok },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			f, ok := m.Families[key]
			if !ok {
				return nil, false
			}
			return familyResource{
				Name: f.Name, ShortName: f.ShortName, Address: f.Address, Phone: f.Phone, Lat: f.Lat, Lng: f.Lng,
				PhotoURL: f.PhotoURL, OriginalPhotoURL: f.OriginalPhotoURL, PhotoCaption: f.PhotoCaption, PhotoUpdated: f.PhotoUpdated,
				PronunciationURL: f.PronunciationURL, AddressMasked: f.AddressMasked, PhoneMasked: f.PhoneMasked,
				Path: FamilyPath(key), App: whoHost,
			}, true
		},
		List: func(m *Directory, _ api.Query) []string {
			out := []string{}
			for key := range m.Families {
				out = append(out, key)
			}
			slices.SortFunc(out, func(a, b string) int {
				if c := strings.Compare(m.Families[a].Name, m.Families[b].Name); c != 0 {
					return c
				}
				return strings.Compare(a, b)
			})
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"adults": familyMembers(true),
			"kids":   familyMembers(false),
		},
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

func classroomsType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "classrooms",
		Shape: classroomResource{},
		Has:   func(m *Directory, key string) bool { return m.classroomByID(key) != nil },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			c := m.classroomByID(key)
			if c == nil {
				return nil, false
			}
			return classroomResource{Name: c.Name, ImageURL: c.ImageURL, HasCrews: c.HasCrews, Slug: ClassroomSlug(c.Name), Path: ClassroomPath(c.Name), App: whoHost}, true
		},
		List: func(m *Directory, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Classrooms {
				out = append(out, c.ID)
			}
			return out
		},
		Aliases: func(m *Directory) map[string]string {
			out := map[string]string{}
			for _, c := range m.Classrooms {
				out[ClassroomSlug(c.Name)] = c.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"students": {Type: "people", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
				c := m.classroomByID(key)
				if c == nil {
					return nil
				}
				return m.peopleWhere(func(p *Person) bool { return p.IsStudent && p.Classroom == c.Name })
			}},
			"crews": {Type: "crews", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
				c := m.classroomByID(key)
				if c == nil {
					return nil
				}
				out := []string{}
				for _, crew := range m.Crews {
					if crew.Classroom == c.Name {
						out = append(out, crew.ID)
					}
				}
				return out
			}},
		},
	}
}

func gradesType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "grades",
		Shape: gradeResource{},
		Has:   func(m *Directory, key string) bool { return m.gradeByID(key) != nil },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			g := m.gradeByID(key)
			if g == nil {
				return nil, false
			}
			return gradeResource{Name: g.Name, NextName: g.NextName, Band: g.Band, NextBand: g.NextBand, ImageURL: g.ImageURL, Slug: ClassroomSlug(g.Name), Path: GradePath(g.Name), App: whoHost}, true
		},
		List: func(m *Directory, _ api.Query) []string {
			out := []string{}
			for _, g := range m.Grades {
				out = append(out, g.ID)
			}
			return out
		},
		Aliases: func(m *Directory) map[string]string {
			out := map[string]string{}
			for _, g := range m.Grades {
				out[ClassroomSlug(g.Name)] = g.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"students": {Type: "people", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
				g := m.gradeByID(key)
				if g == nil {
					return nil
				}
				return m.peopleWhere(func(p *Person) bool { return p.IsStudent && p.Grade == g.Name })
			}},
		},
		Filters: map[string]api.Filter[*Directory]{
			"enrolled": func(m *Directory, _ api.Query, value string) (func(string) bool, error) {
				if err := noValue("enrolled", value); err != nil {
					return nil, err
				}
				enrolled := map[string]bool{}
				for _, p := range m.People {
					if p.IsStudent && p.Grade != "" {
						enrolled[p.Grade] = true
					}
				}
				return func(key string) bool {
					g := m.gradeByID(key)
					return g != nil && enrolled[g.Name]
				}, nil
			},
		},
	}
}

func crewsType() api.Type[*Directory] {
	return api.Type[*Directory]{
		Name:  "crews",
		Shape: crewResource{},
		Has:   func(m *Directory, key string) bool { return m.crewByID(key) != nil },
		Get: func(m *Directory, _ api.Query, key string) (any, bool) {
			c := m.crewByID(key)
			if c == nil {
				return nil, false
			}
			return crewResource{Name: c.Name, GradeBand: c.GradeBand, Path: ClassroomPath(c.Classroom), App: whoHost}, true
		},
		List: func(m *Directory, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Crews {
				out = append(out, c.ID)
			}
			return out
		},
		Relations: map[string]api.Relation[*Directory]{
			"classroom": {Type: "classrooms", List: func(m *Directory, _ api.Query, key string) []string {
				c := m.crewByID(key)
				if c == nil {
					return nil
				}
				for _, room := range m.Classrooms {
					if room.Name == c.Classroom {
						return []string{room.ID}
					}
				}
				return nil
			}},
			"teachers": {Type: "people", Many: true, List: func(m *Directory, _ api.Query, key string) []string {
				c := m.crewByID(key)
				if c == nil {
					return nil
				}
				out := []*Person{}
				for _, email := range c.Teachers {
					if p := m.Person(email); p != nil {
						out = append(out, p)
					}
				}
				return ids(out)
			}},
		},
	}
}
