package who

import (
	"net/url"
	"strings"

	"heliosian/internal/store"
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
	Email                string  `json:"email"`
	FullName             string  `json:"fullName"`
	LegalName            string  `json:"legalName,omitempty"`
	PreferredName        string  `json:"preferredName,omitempty"`
	IsStaff              bool    `json:"isStaff"`
	IsParent             bool    `json:"isParent"`
	IsStudent            bool    `json:"isStudent"`
	IsNew                bool    `json:"isNew,omitempty"`
	Pronouns             string  `json:"pronouns,omitempty"`
	Facts                string  `json:"facts,omitempty"`
	FactsUpdated         string  `json:"factsUpdated,omitempty"`
	PronunciationURL     string  `json:"pronunciationUrl,omitempty"`
	HasOwnPronunciation  bool    `json:"hasOwnPronunciation,omitempty"`
	PhotoURL             string  `json:"photoUrl,omitempty"`
	Photos               []Photo `json:"photos,omitempty"`
	primaryPhotoOverride string
	veracrossPhoto       string
	websitePhoto         string
	pronunciation        string
	overrideRow          map[string]string
	imported             map[string]string
	PhotoUpdated         string   `json:"photoUpdated,omitempty"`
	Grade                string   `json:"grade,omitempty"`
	Classroom            string   `json:"classroom,omitempty"`
	Crew                 string   `json:"crew,omitempty"`
	Phone                string   `json:"phone,omitempty"`
	ParentContactEmails  []string `json:"parentContactEmails,omitempty"`
	JobTitle             string   `json:"jobTitle,omitempty"`
	Department           string   `json:"department,omitempty"`
	GradeBand            string   `json:"gradeBand,omitempty"`

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

	importedAddress string
}

type Classroom struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	HasCrews bool   `json:"hasCrews"`
}

type Crew struct {
	Classroom string   `json:"classroom"`
	Name      string   `json:"name,omitempty"`
	Teachers  []string `json:"teachers,omitempty"`
	GradeBand string   `json:"gradeBand,omitempty"`
}

type Grade struct {
	Name     string `json:"name"`
	NextName string `json:"nextName,omitempty"`
	Band     string `json:"band,omitempty"`
	NextBand string `json:"nextBand,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type Model struct {
	People            []Person            `json:"people"`
	Families          map[string]Family   `json:"families"`
	Classrooms        []Classroom         `json:"classrooms"`
	Crews             []Crew              `json:"crews"`
	Grades            []Grade             `json:"grades"`
	RoomParents       map[string][]string `json:"roomParents"`
	Departments       []string            `json:"departments"`
	byEmail           map[string]int
	familyKeysByEmail map[string][]string
	hiddenEmails      []string
	aliases           Aliases
	tags              []store.Row
	managers          []store.Row
	admins            []string
	unlocated         []string
}

func (m *Model) Resolve(email string) string {
	if resolved, ok := m.aliases[email]; ok {
		return resolved
	}
	return email
}

func (m *Model) Person(email string) *Person {
	i, ok := m.byEmail[email]
	if !ok {
		return nil
	}
	return &m.People[i]
}

func (m *Model) FamilyKeysOf(email string) []string {
	return m.familyKeysByEmail[email]
}

func (m *Model) FamilyOf(email string) (Family, bool) {
	keys := m.FamilyKeysOf(m.Resolve(email))
	if len(keys) == 0 {
		return Family{}, false
	}
	family, ok := m.Families[keys[0]]
	return family, ok
}

func (m *Model) Members(key string) (adults, kids []*Person) {
	return m.members([]string{key}, "")
}

func (m *Model) Household(email string) (adults, kids []*Person) {
	email = m.Resolve(email)
	return m.members(m.FamilyKeysOf(email), email)
}

func (m *Model) Family(email string) map[string]bool {
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

func (m *Model) Parents(email string) []*Person {
	if p := m.Person(m.Resolve(email)); p == nil || !p.IsStudent {
		return nil
	}
	adults, _ := m.Household(email)
	return adults
}

func (m *Model) Children(email string) []*Person {
	if p := m.Person(m.Resolve(email)); p == nil || !p.IsParent {
		return nil
	}
	_, kids := m.Household(email)
	return kids
}

func (m *Model) members(keys []string, without string) (adults, kids []*Person) {
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

func (m *Model) HeroPhoto(email string) string {
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

func (m *Model) Member(email string) bool {
	return m.Person(email) != nil
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

func (m *Model) DisplayName(email string) string {
	if p := m.Person(email); p != nil {
		return p.FullName
	}
	return email
}
