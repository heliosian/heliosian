package model

import (
	"net/url"
	"slices"
	"strings"
)

type OptStatus string

const (
	OptDefault OptStatus = "default"
	OptIn      OptStatus = "in"
	OptOut     OptStatus = "out"
)

type Photo struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	URL         string `json:"url"`
	OriginalURL string `json:"originalUrl"`
	cropName    string
	order       string
	stored      bool
}

type Person struct {
	ID                  string  `json:"id,omitempty"`
	Email               string  `json:"email"`
	FullName            string  `json:"fullName"`
	LegalName           string  `json:"legalName,omitempty"`
	PreferredName       string  `json:"preferredName,omitempty"`
	IsStaff             bool    `json:"isStaff"`
	IsParent            bool    `json:"isParent"`
	IsStudent           bool    `json:"isStudent"`
	IsNew               bool    `json:"isNew,omitempty"`
	Pronouns            string  `json:"pronouns,omitempty"`
	Facts               string  `json:"facts,omitempty"`
	FactsUpdated        string  `json:"factsUpdated,omitempty"`
	PronunciationURL    string  `json:"pronunciationUrl,omitempty"`
	HasOwnPronunciation bool    `json:"hasOwnPronunciation,omitempty"`
	PhotoURL            string  `json:"photoUrl,omitempty"`
	Photos              []Photo `json:"photos,omitempty"`
	veracrossPhoto      string
	websitePhoto        string
	pronunciation       string
	PhotoUpdated        string   `json:"photoUpdated,omitempty"`
	Grade               string   `json:"grade,omitempty"`
	Classroom           string   `json:"classroom,omitempty"`
	Crew                string   `json:"crew,omitempty"`
	Phone               string   `json:"phone,omitempty"`
	ParentContactEmails []string `json:"parentContactEmails,omitempty"`
	JobTitle            string   `json:"jobTitle,omitempty"`
	Department          string   `json:"department,omitempty"`
	GradeBand           string   `json:"gradeBand,omitempty"`

	OptStatus     OptStatus `json:"optStatus"`
	AddressMasked bool      `json:"addressMasked,omitempty"`
	PhoneMasked   bool      `json:"phoneMasked,omitempty"`
	EmailMasked   bool      `json:"emailMasked,omitempty"`
}

type Family struct {
	Key              string   `json:"key"`
	Name             string   `json:"name,omitempty"`
	ShortName        string   `json:"shortName,omitempty"`
	Address          string   `json:"address,omitempty"`
	Phone            string   `json:"phone,omitempty"`
	Lat              float64  `json:"lat,omitempty"`
	Lng              float64  `json:"lng,omitempty"`
	PhotoURL         string   `json:"photoUrl,omitempty"`
	OriginalPhotoURL string   `json:"originalPhotoUrl,omitempty"`
	PhotoCaption     string   `json:"photoCaption,omitempty"`
	PhotoUpdated     string   `json:"photoUpdated,omitempty"`
	PronunciationURL string   `json:"pronunciationUrl,omitempty"`
	AdultEmails      []string `json:"adultEmails,omitempty"`
	KidEmails        []string `json:"kidEmails,omitempty"`
	AddressMasked    bool     `json:"addressMasked,omitempty"`
	PhoneMasked      bool     `json:"phoneMasked,omitempty"`

	VeracrossAddress string `json:"veracrossAddress"`
	VeracrossPhone   string `json:"veracrossPhone"`

	photo, pronunciation, photoCropName, email string

	sheetRow map[string]string
}

type Classroom struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	HasCrews bool   `json:"hasCrews"`
}

type Crew struct {
	ID        string   `json:"id"`
	Classroom string   `json:"classroom"`
	Name      string   `json:"name,omitempty"`
	Teachers  []string `json:"teachers,omitempty"`
	GradeBand string   `json:"gradeBand,omitempty"`
}

type Grade struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NextName string `json:"nextName,omitempty"`
	Band     string `json:"band,omitempty"`
	NextBand string `json:"nextBand,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type Directory struct {
	People            []Person            `json:"people"`
	Families          map[string]Family   `json:"families"`
	Classrooms        []Classroom         `json:"classrooms"`
	Crews             []Crew              `json:"crews"`
	Grades            []Grade             `json:"grades"`
	RoomParents       map[string][]string `json:"roomParents"`
	Departments       []string            `json:"departments"`
	departmentIDs     []string
	byEmail           map[string]int
	byID              map[string]int
	familyKeysByEmail map[string][]string
	aliases           EmailAliases
	tags              map[string]*tagRecord
	admins            []string
	unlocated         []string
}

func (m *Directory) Resolve(email string) string {
	if resolved, ok := m.aliases[email]; ok {
		return resolved
	}
	return email
}

func (m *Directory) Person(email string) *Person {
	i, ok := m.byEmail[email]
	if !ok {
		return nil
	}
	return &m.People[i]
}

func (m *Directory) FamilyKeysOf(email string) []string {
	return m.familyKeysByEmail[email]
}

func (m *Directory) FamilyOf(email string) (Family, bool) {
	keys := m.FamilyKeysOf(m.Resolve(email))
	if len(keys) == 0 {
		return Family{}, false
	}
	family, ok := m.Families[keys[0]]
	return family, ok
}

func (m *Directory) Members(key string) (adults, kids []*Person) {
	return m.members([]string{key}, "")
}

func (m *Directory) Household(email string) (adults, kids []*Person) {
	email = m.Resolve(email)
	return m.members(m.FamilyKeysOf(email), email)
}

func (m *Directory) Family(email string) map[string]bool {
	out := map[string]bool{}
	if p := m.Person(m.Resolve(email)); p == nil || !p.IsParent {
		return out
	}
	adults, kids := m.Household(email)
	for _, p := range append(adults, kids...) {
		out[p.Email] = true
	}
	return out
}

func (m *Directory) Parents(email string) []*Person {
	if p := m.Person(m.Resolve(email)); p == nil || !p.IsStudent {
		return nil
	}
	adults, _ := m.Household(email)
	return adults
}

func (m *Directory) Children(email string) []*Person {
	if p := m.Person(m.Resolve(email)); p == nil || !p.IsParent {
		return nil
	}
	_, kids := m.Household(email)
	return kids
}

func (m *Directory) members(keys []string, without string) (adults, kids []*Person) {
	seen := map[string]bool{without: true}
	add := func(to []*Person, members []string) []*Person {
		for _, member := range members {
			if p := m.Person(m.Resolve(member)); p != nil && !seen[p.Email] {
				seen[p.Email] = true
				to = append(to, p)
			}
		}
		return to
	}
	for _, key := range keys {
		family := m.Families[key]
		adults = add(adults, family.AdultEmails)
		kids = add(kids, family.KidEmails)
	}
	return adults, kids
}

func (m *Directory) HeroPhoto(email string) string {
	person := m.Person(email)
	if person == nil {
		return ""
	}
	if person.PhotoURL != "" {
		return person.PhotoURL
	}
	for _, key := range m.FamilyKeysOf(email) {
		if family, ok := m.Families[key]; ok && family.PhotoURL != "" {
			return family.PhotoURL
		}
	}
	return ""
}

func (m *Directory) Member(email string) bool {
	return m.Person(email) != nil
}

func (p *Person) Words() string {
	switch {
	case p.IsStaff:
		if p.JobTitle == "" {
			return "Staff"
		}
		return p.JobTitle
	case p.IsStudent:
		if p.Grade == "" {
			return "Student"
		}
		return p.Grade
	case p.IsParent:
		return "Parent"
	}
	return ""
}

func (m *Directory) Listed() []*Person {
	out := []*Person{}
	for i := range m.People {
		if m.People[i].Email != "" && !m.People[i].EmailMasked {
			out = append(out, &m.People[i])
		}
	}
	slices.SortStableFunc(out, func(a, b *Person) int {
		return strings.Compare(strings.ToLower(a.FullName), strings.ToLower(b.FullName))
	})
	return out
}

func Slug(email string) string {
	local, _, _ := strings.Cut(email, "@")
	return local
}

func PersonPath(email string) string {
	return "/people/" + url.PathEscape(Slug(email))
}

func FamilyPath(key string) string {
	return "/families/" + url.PathEscape(key)
}

func ClassroomSlug(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "-")
}

func ClassroomPath(name string) string {
	return "/classrooms/" + url.PathEscape(ClassroomSlug(name))
}

func ListPath(key string) string {
	return "/people?list=" + url.QueryEscape(key)
}

func TagPath(key string) string {
	return "/people?tag=" + url.QueryEscape(key)
}

func (m *Directory) DisplayName(email string) string {
	if p := m.Person(email); p != nil {
		return p.FullName
	}
	return email
}

func (m *Directory) Facets(p *Person, classroom bool) []string {
	pick := func(q *Person) string {
		if classroom {
			return q.Classroom
		}
		return q.Grade
	}
	if p.IsStudent {
		if v := pick(p); v != "" {
			return []string{v}
		}
		return nil
	}
	out := []string{}
	for _, k := range m.Children(p.Email) {
		if pick(k) != "" {
			out = append(out, pick(k))
		}
	}
	return out
}

func (m *Directory) ClassroomsOf(p *Person) []string {
	out := []string{}
	for _, c := range m.Facets(p, true) {
		if !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func (m *Directory) ResolveAll(emails []string) []string {
	out := []string{}
	for _, e := range emails {
		if r := m.Resolve(e); !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

func (m *Directory) GradeNames() []string {
	out := []string{}
	for _, g := range m.Grades {
		out = append(out, g.Name)
	}
	return out
}

func (m *Directory) ClassroomNames() []string {
	out := []string{}
	for _, c := range m.Classrooms {
		out = append(out, c.Name)
	}
	return out
}

func (m *Directory) Teaches(author string) []string {
	name, _, _ := strings.Cut(author, "<")
	name = strings.Trim(strings.TrimSpace(name), `"`)
	if name == "" {
		return nil
	}
	out := []string{}
	for _, c := range m.Crews {
		for _, email := range c.Teachers {
			if p := m.Person(email); p != nil && strings.EqualFold(p.FullName, name) && !slices.Contains(out, c.Classroom) {
				out = append(out, c.Classroom)
			}
		}
	}
	return out
}
