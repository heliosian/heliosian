package directory

type OptStatus string

const (
	OptDefault OptStatus = "default"
	OptIn      OptStatus = "in"
	OptOut     OptStatus = "out"
)

// Every photo is one content-addressed object; which of a person's photos came from
// Veracross is recorded in the sheet, not in the object name. Source is "veracross"
// or "upload". A photo may also have a linked square crop, another content-addressed
// object recorded alongside it - cropName carries that object's name for reuse when
// a caller needs to rewrite a person's full photo list without dropping it (it isn't
// exported to JSON; the frontend only ever needs the two resolved URLs below).
type Photo struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	URL         string `json:"url"`         // the crop, if one exists, else the original
	OriginalURL string `json:"originalUrl"` // always the original, uncropped image
	cropName    string
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
	pronunciation        string
	// overrideRow is this person's raw Overrides sheet row, or nil if they don't have
	// one. Internal only: admin.go reads it (via overrideStringValue/overrideBoolValue)
	// to show and edit exactly what Overrides currently holds for a column, distinct
	// from the resolved value below - a plain nil-map lookup safely returns "" for
	// someone with no row at all, meaning "no override".
	overrideRow map[string]string
	// imported snapshots this person's Veracross-import-derived field values, captured
	// in applyOverrides (load.go) before overrideRow's cells could change any of them.
	// Internal only: nil for a person with no override row - their live field values
	// above already ARE the import values, since nothing here ever changed them, so
	// admin.go falls back to those directly in that case. Only holds columns that can
	// genuinely come from Veracross (Full Name, Legal Name, Preferred Name, Grade,
	// Classroom, Crew, Phone, Job Title, Is Staff); Department, Grade Band, Pronouns,
	// Facts and Room Parent have no import source for anyone.
	imported map[string]string
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
	// EmailMasked marks a Veracross-generated placeholder address (see noEmailMarker in
	// load.go) - unlike AddressMasked/PhoneMasked, which blank the field they mask
	// because the underlying data is sensitive, Email itself is left completely
	// untouched here: it's still this person's real identity/key everywhere in the app
	// (routing, Overrides, Tags, Photos). This only tells a viewer's client not to
	// render it - no visible address, no mailto: link, no copy button - since nobody
	// can actually reach the fake one.
	EmailMasked bool `json:"emailMasked,omitempty"`
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

	// VeracrossAddress and VeracrossPhone record what Veracross itself shows for this
	// family - "full"/"partial"/"hidden" and "visible"/"mixed"/"hidden" respectively -
	// captured before AddressMasked/PhoneMasked (the Helios Who opt-in override) can
	// blank the fields above. My Privacy uses them to warn a family whose Helios Who
	// override hides something Veracross still shows to the wider community.
	VeracrossAddress string `json:"veracrossAddress"`
	VeracrossPhone   string `json:"veracrossPhone"`

	photo, pronunciation string

	// sheetRow is this family's raw Families sheet row, or nil if it doesn't have one -
	// the same shape as Person.overrideRow, and read the same way: admin.go shows and
	// edits exactly what the tab currently holds for a column, distinct from the
	// resolved values above.
	sheetRow map[string]string

	// importedAddress snapshots the household's Veracross-import address, captured in
	// buildFamilies (load.go) before any adult's familyOverrides cells could change
	// Address above. Internal only: admin.go's Parent Overrides tab shows this as the
	// "Veracross" reference next to the (possibly different) value actually in
	// Overrides.
	importedAddress string
}

type Classroom struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl,omitempty"`
	HasCrews bool   `json:"hasCrews"`
	Color    string `json:"color,omitempty"`
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
	Color    string `json:"color,omitempty"`
}

// StaleYears is how old a photo or facts entry can get before the directory asks
// someone to refresh it. Admin-editable so the school can loosen or tighten the nag
// without a deploy.
type StaleYears struct {
	Photo       float64 `json:"photo"`
	Facts       float64 `json:"facts"`
	FamilyPhoto float64 `json:"familyPhoto"`
}

// PrivacyLinks are the two external URLs My Privacy sends someone to fix a mismatch
// between Veracross and their Helios Who opt-in. Admin-editable, since both belong to
// other systems (Veracross's own portal, the consent Google Form) this app doesn't
// control and can't guarantee will stay put.
type PrivacyLinks struct {
	VeracrossPreferences string `json:"veracrossPreferences"`
	HeliosWhoOptIn       string `json:"heliosWhoOptIn"`
}

type Model struct {
	People       []Person            `json:"people"`
	Families     map[string]Family   `json:"families"`
	Classrooms   []Classroom         `json:"classrooms"`
	Crews        []Crew              `json:"crews"`
	Grades       []Grade             `json:"grades"`
	RoomParents  map[string][]string `json:"roomParents"`
	Departments  []string            `json:"departments"`
	StaleYears   StaleYears          `json:"staleYears"`
	PrivacyLinks PrivacyLinks        `json:"privacyLinks"`
	StaffColor   string              `json:"staffColor,omitempty"`
	byEmail      map[string]int
	// familyKeysByEmail is internal only: the client derives the same index from
	// Families' own member lists, so serializing it would just duplicate them.
	familyKeysByEmail map[string][]string
	// hiddenEmails lists everyone removeOptedOut (load.go) just deleted from People -
	// captured there because that's the last point any of their data (even just their
	// email) is still reachable. Internal only, deliberately never serialized: this
	// app's one public API (the directory model JSON) must never reveal who opted out,
	// which is the whole point of opting out. admin.go's Hidden Overrides tab is the
	// only reader, and only for a caller already confirmed to be an admin.
	hiddenEmails []string
}

func (m *Model) Person(email string) *Person {
	i, ok := m.byEmail[email]
	if !ok {
		return nil
	}
	return &m.People[i]
}

// FamilyKeysOf lists every family this person belongs to, in sorted key order - one
// for a parent (an adult belongs to at most one household), one or more for a kid.
func (m *Model) FamilyKeysOf(email string) []string {
	return m.familyKeysByEmail[email]
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
