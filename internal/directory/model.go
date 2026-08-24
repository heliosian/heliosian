package directory

type OptStatus string

const (
	OptDefault OptStatus = "default"
	OptIn      OptStatus = "in"
	OptOut     OptStatus = "out"
)

// Photo sources. The school portrait and a person's own upload are separate objects
// in the bucket, so uploading no longer destroys the portrait.
const (
	PhotoUpload    = "upload"
	PhotoVeracross = "veracross"
)

type Photo struct {
	Source string `json:"source"`
	URL    string `json:"url"`
}

type Person struct {
	Email               string   `json:"email"`
	FullName            string   `json:"fullName"`
	LegalName           string   `json:"legalName,omitempty"`
	PreferredName       string   `json:"preferredName,omitempty"`
	IsStaff             bool     `json:"isStaff"`
	IsParent            bool     `json:"isParent"`
	IsStudent           bool     `json:"isStudent"`
	IsNew               bool     `json:"isNew,omitempty"`
	Pronouns            string   `json:"pronouns,omitempty"`
	Facts               string   `json:"facts,omitempty"`
	FactsUpdated        string   `json:"factsUpdated,omitempty"`
	PronunciationURL    string   `json:"pronunciationUrl,omitempty"`
	PhotoURL            string   `json:"photoUrl,omitempty"`
	Photos              []Photo  `json:"photos,omitempty"`
	PrimaryPhoto        string   `json:"primaryPhoto,omitempty"`
	PhotoUpdated        string   `json:"photoUpdated,omitempty"`
	Grade               string   `json:"grade,omitempty"`
	Classroom           string   `json:"classroom,omitempty"`
	Crew                string   `json:"crew,omitempty"`
	Phone               string   `json:"phone,omitempty"`
	FamilyKey           string   `json:"familyKey,omitempty"`
	ParentContactEmails []string `json:"parentContactEmails,omitempty"`
	JobTitle            string   `json:"jobTitle,omitempty"`
	Department          string   `json:"department,omitempty"`
	GradeBand           string   `json:"gradeBand,omitempty"`

	OptStatus     OptStatus `json:"optStatus"`
	AddressMasked bool      `json:"addressMasked,omitempty"`
	PhoneMasked   bool      `json:"phoneMasked,omitempty"`
}

type Family struct {
	Key              string   `json:"key"`
	Name             string   `json:"name,omitempty"`
	Address          string   `json:"address,omitempty"`
	Phone            string   `json:"phone,omitempty"`
	Lat              float64  `json:"lat,omitempty"`
	Lng              float64  `json:"lng,omitempty"`
	PhotoURL         string   `json:"photoUrl,omitempty"`
	PhotoCaption     string   `json:"photoCaption,omitempty"`
	PhotoUpdated     string   `json:"photoUpdated,omitempty"`
	PronunciationURL string   `json:"pronunciationUrl,omitempty"`
	AdultEmails      []string `json:"adultEmails,omitempty"`
	KidEmails        []string `json:"kidEmails,omitempty"`
	AddressMasked    bool     `json:"addressMasked,omitempty"`
	PhoneMasked      bool     `json:"phoneMasked,omitempty"`
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
}

type Model struct {
	People      []Person            `json:"people"`
	Families    map[string]Family   `json:"families"`
	Classrooms  []Classroom         `json:"classrooms"`
	Crews       []Crew              `json:"crews"`
	Grades      []Grade             `json:"grades"`
	RoomParents map[string][]string `json:"roomParents"`
	Departments []string            `json:"departments"`
	byEmail     map[string]int
}

func (m *Model) Person(email string) *Person {
	i, ok := m.byEmail[email]
	if !ok {
		return nil
	}
	return &m.People[i]
}

func (m *Model) Member(email string) bool {
	return m.Person(email) != nil
}

func (m *Model) DisplayName(email string) string {
	if p := m.Person(email); p != nil {
		return p.FullName
	}
	return email
}
