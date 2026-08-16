package directory

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
	PronunciationURL    string   `json:"pronunciationUrl,omitempty"`
	PhotoURL            string   `json:"photoUrl,omitempty"`
	Grade               string   `json:"grade,omitempty"`
	Classroom           string   `json:"classroom,omitempty"`
	Section             string   `json:"section,omitempty"`
	Phone               string   `json:"phone,omitempty"`
	FamilyKey           string   `json:"familyKey,omitempty"`
	ParentContactEmails []string `json:"parentContactEmails,omitempty"`
	JobTitle            string   `json:"jobTitle,omitempty"`
	Department          string   `json:"department,omitempty"`
	GradeBand           string   `json:"gradeBand,omitempty"`
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
	PronunciationURL string   `json:"pronunciationUrl,omitempty"`
	AdultEmails      []string `json:"adultEmails,omitempty"`
	KidEmails        []string `json:"kidEmails,omitempty"`
}

type Classroom struct {
	Name        string `json:"name"`
	ImageURL    string `json:"imageUrl,omitempty"`
	HasSections bool   `json:"hasSections"`
}

type Section struct {
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

func (m *Model) DisplayName(email string) string {
	for _, p := range m.People {
		if p.Email == email {
			return p.FullName
		}
	}
	return email
}

type Model struct {
	People      []Person            `json:"people"`
	Families    map[string]Family   `json:"families"`
	Classrooms  []Classroom         `json:"classrooms"`
	Sections    []Section           `json:"sections"`
	Grades      []Grade             `json:"grades"`
	RoomParents map[string][]string `json:"roomParents"`
	Departments []string            `json:"departments"`
}
