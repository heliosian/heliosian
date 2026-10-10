package model

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	emailListsAppName = "groups"
	groupsTab         = "Groups"
	managersTab       = "Managers"
	rulesTab          = "Rules"
	additionsTab      = "Additions"
	excludedTab       = "Excluded"
	messagesTab       = "Messages"
	deliveriesTab     = "Deliveries"
	archivedTab       = "Archived"

	idColumn     = "Group ID"
	prefixColumn = "Subject Prefix"
	prefixOff    = "off"

	visibleColumn = "Visible"

	VisibilityHidden   = "hidden"
	VisibilityMembers  = "members"
	VisibilityEveryone = "everyone"

	postingColumn  = "Posting"
	replyingColumn = "Replying"

	PostingEveryone = "everyone"
	PostingMembers  = "members"
	PostingManagers = "managers"

	ListDomain = "loop.heliosian.com"

	maxListTitleLength   = 80
	maxDescriptionLength = 300
	maxSearchLength      = 80
	maxRules             = 40
	maxAdditions         = 200
	maxListNameLength    = 80
	maxExcluded          = 200
	maxNoteLength        = 120
	maxAliases           = 10
)

var (
	GroupColumns       = []string{idColumn, "Name", "Title", "Description", "Created By", "Created", prefixColumn, visibleColumn, postingColumn, replyingColumn}
	ManagerColumns     = []string{"Group", "Email"}
	ListRuleColumns    = append([]string{"Group"}, RuleColumns...)
	AdditionColumns    = []string{"Group", "Email", "Name"}
	ExcludedColumns    = []string{"Group", "Email", "Note", "Timestamp"}
	ListMessageColumns = []string{"ID", "Group", "Received", "From", "Subject", "State", "Recipients", "Object", "Detail", "Message ID"}
	DeliveryColumns    = []string{"Timestamp", "Group", "Email", "Event", "Message", "Detail"}
	ArchivedColumns    = []string{"Group", "Email"}

	Visibilities = []string{VisibilityHidden, VisibilityMembers, VisibilityEveryone}

	Postings = []string{PostingEveryone, PostingMembers, PostingManagers}
)

var listNameForm = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,38}[a-z0-9]$`)

var reservedNames = []string{"abuse", "admin", "administrator", "hostmaster", "noreply", "no-reply", "postmaster", "root", "unsubscribe", "webmaster"}

type Addition struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Excluded struct {
	Email string `json:"email"`
	Note  string `json:"note"`
	When  string `json:"when"`
}

type EmailList struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Aliases     []string   `json:"aliases"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CreatedBy   string     `json:"createdBy,omitempty"`
	Created     string     `json:"created,omitempty"`
	Prefix      bool       `json:"prefix"`
	Visibility  string     `json:"visibility"`
	Posting     string     `json:"posting"`
	Replying    string     `json:"replying"`
	Managers    []string   `json:"managers"`
	Rules       []Rule     `json:"rules"`
	Additions   []Addition `json:"additions"`
	Excluded    []Excluded `json:"excluded"`
}

func (g EmailList) HasExcluded(email string) bool {
	return slices.ContainsFunc(g.Excluded, func(e Excluded) bool { return e.Email == email })
}

func (g EmailList) Names() []string {
	return append([]string{g.Name}, g.Aliases...)
}

func (g EmailList) Addition(email string) *Addition {
	for i := range g.Additions {
		if g.Additions[i].Email == email {
			return &g.Additions[i]
		}
	}
	return nil
}

func (g EmailList) Address() string {
	return g.Name + "@" + ListDomain
}

func (g EmailList) Path() string {
	return "/groups/" + g.Name
}

func (g EmailList) Manages(email string) bool {
	return slices.Contains(g.Managers, email)
}

type ListMessage struct {
	ID         string
	Group      string
	Received   string
	From       string
	Subject    string
	State      string
	Recipients string
	Object     string
	Detail     string
	MessageID  string
}

type ListDelivery struct {
	Timestamp string
	Group     string
	Email     string
	Event     string
	Message   string
	Detail    string
}

type EmailLists struct {
	Groups     []EmailList
	Messages   []ListMessage
	Deliveries []ListDelivery
	admins     []string
	idKey      []byte
	byID       map[string]int
	byName     map[string]int
	byAddress  map[string]int
	archived   map[string]map[string]bool
	sent       map[string][]*Sent
	sentByID   map[string]*Sent
	copyByID   map[string]*Copy
}

func (m *EmailLists) Archived(groupID, email string) bool {
	return m.archived[groupID][email]
}

func (m *EmailLists) Group(groupID string) *EmailList {
	i, ok := m.byID[strings.ToLower(strings.TrimSpace(groupID))]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

func (m *EmailLists) Named(name string) *EmailList {
	i, ok := m.byName[name]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

func (m *EmailLists) Resolve(local string) *EmailList {
	i, ok := m.byAddress[local]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

func CheckListName(name string) error {
	if !listNameForm.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("an email list's name is two to forty lowercase letters, digits, dots and hyphens, starting and ending with a letter or digit, with no two dots together")
	}
	if slices.Contains(reservedNames, name) {
		return fmt.Errorf("%s is reserved", name)
	}
	if _, ok := id.Parse(name); ok {
		return fmt.Errorf("%s reads as an id", name)
	}
	return nil
}

func CheckListRule(r Rule) error {
	if err := r.Check(); err != nil {
		return err
	}
	if len(r.Search) > maxSearchLength {
		return fmt.Errorf("the search words are too long")
	}
	return nil
}

func CheckList(g EmailList) error {
	if err := CheckListName(g.Name); err != nil {
		return err
	}
	if len(g.Aliases) > maxAliases {
		return fmt.Errorf("email list %s has too many aliases", g.Name)
	}
	for _, alias := range g.Aliases {
		if err := CheckListName(alias); err != nil {
			return fmt.Errorf("email list %s, alias %q: %w", g.Name, alias, err)
		}
		if alias == g.Name {
			return fmt.Errorf("email list %s: an alias cannot be the email list's own name", g.Name)
		}
	}
	if strings.TrimSpace(g.Title) == "" || len(g.Title) > maxListTitleLength {
		return fmt.Errorf("email list %s needs a short title", g.Name)
	}
	if len(g.Description) > maxDescriptionLength {
		return fmt.Errorf("email list %s: the description is too long", g.Name)
	}
	if !slices.Contains(Visibilities, g.Visibility) {
		return fmt.Errorf("email list %s: visibility %q is not one of %s", g.Name, g.Visibility, cells.JoinList(Visibilities))
	}
	if !slices.Contains(Postings, g.Posting) {
		return fmt.Errorf("email list %s: posting %q is not one of %s", g.Name, g.Posting, cells.JoinList(Postings))
	}
	if !slices.Contains(Postings, g.Replying) {
		return fmt.Errorf("email list %s: replying %q is not one of %s", g.Name, g.Replying, cells.JoinList(Postings))
	}
	if len(g.Managers) == 0 {
		return fmt.Errorf("email list %s needs at least one manager", g.Name)
	}
	for _, m := range g.Managers {
		if !emailForm.MatchString(m) {
			return fmt.Errorf("email list %s: manager %q is not an email address", g.Name, m)
		}
	}
	if len(g.Rules) > maxRules {
		return fmt.Errorf("email list %s has too many rules", g.Name)
	}
	includes := 0
	for i, r := range g.Rules {
		if err := CheckListRule(r); err != nil {
			return fmt.Errorf("email list %s, rule %d: %w", g.Name, i+1, err)
		}
		if r.Kind == RuleInclude {
			includes++
		}
	}
	if includes == 0 {
		return fmt.Errorf("email list %s needs at least one include rule", g.Name)
	}
	if len(g.Additions) > maxAdditions {
		return fmt.Errorf("email list %s has too many people added by hand", g.Name)
	}
	for _, a := range g.Additions {
		if !emailForm.MatchString(a.Email) {
			return fmt.Errorf("email list %s: %q is not an email address", g.Name, a.Email)
		}
		if len(a.Name) > maxListNameLength {
			return fmt.Errorf("email list %s: the name for %s is too long", g.Name, a.Email)
		}
	}
	if len(g.Excluded) > maxExcluded {
		return fmt.Errorf("email list %s has too many excluded addresses", g.Name)
	}
	for _, e := range g.Excluded {
		if !emailForm.MatchString(e.Email) {
			return fmt.Errorf("email list %s: excluded %q is not an email address", g.Name, e.Email)
		}
		if len(e.Note) > maxNoteLength {
			return fmt.Errorf("email list %s: the note for %s is too long", g.Name, e.Email)
		}
	}
	return nil
}

func NormalizeList(g EmailList) EmailList {
	g.ID = strings.ToLower(strings.TrimSpace(g.ID))
	g.Name = strings.ToLower(strings.TrimSpace(g.Name))
	aliases := []string{}
	for _, alias := range g.Aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias != "" && !slices.Contains(aliases, alias) {
			aliases = append(aliases, alias)
		}
	}
	g.Aliases = aliases
	g.Title = strings.TrimSpace(g.Title)
	g.Description = strings.TrimSpace(g.Description)
	g.Visibility = strings.ToLower(strings.TrimSpace(g.Visibility))
	if g.Visibility == "" {
		g.Visibility = VisibilityHidden
	}
	g.Posting = strings.ToLower(strings.TrimSpace(g.Posting))
	if g.Posting == "" {
		g.Posting = PostingEveryone
	}
	g.Replying = strings.ToLower(strings.TrimSpace(g.Replying))
	if g.Replying == "" {
		g.Replying = PostingEveryone
	}
	g.Managers = mail.NormalizeAll(g.Managers)
	rules := []Rule{}
	for _, r := range g.Rules {
		rules = append(rules, r.Clean())
	}
	g.Rules = rules
	additions := []Addition{}
	for _, a := range g.Additions {
		a.Email = mail.Normalize(a.Email)
		a.Name = strings.Join(strings.Fields(a.Name), " ")
		if a.Email != "" && !slices.ContainsFunc(additions, func(b Addition) bool { return b.Email == a.Email }) {
			additions = append(additions, a)
		}
	}
	g.Additions = additions
	excluded := []Excluded{}
	for _, e := range g.Excluded {
		e.Email = mail.Normalize(e.Email)
		e.Note = strings.Join(strings.Fields(e.Note), " ")
		e.When = strings.TrimSpace(e.When)
		if e.Email != "" && !slices.ContainsFunc(excluded, func(v Excluded) bool { return v.Email == e.Email }) {
			excluded = append(excluded, e)
		}
	}
	g.Excluded = excluded
	return g
}

func compareWhen(x, y Excluded) int {
	switch {
	case x.When == y.When:
		return 0
	case x.When == "":
		return 1
	case y.When == "":
		return -1
	}
	return strings.Compare(x.When, y.When)
}

func ruleCells(groupID string, r Rule) store.Row {
	cells := r.Cells()
	cells["Group"] = groupID
	return cells
}

func prefixCell(on bool) string {
	if on {
		return ""
	}
	return prefixOff
}

func groupCells(g EmailList) store.Row {
	return store.Row{idColumn: g.ID, "Name": g.Name, "Title": g.Title, "Description": g.Description, "Created By": g.CreatedBy, "Created": g.Created, prefixColumn: prefixCell(g.Prefix), visibleColumn: g.Visibility, postingColumn: g.Posting, replyingColumn: g.Replying}
}

func BuildEmailLists(tables store.Tables, idKey []byte) (*EmailLists, error) {
	m := &EmailLists{Groups: []EmailList{}, Messages: []ListMessage{}, Deliveries: []ListDelivery{}, idKey: idKey, byID: map[string]int{}, byName: map[string]int{}, archived: map[string]map[string]bool{}}
	m.admins = ReadAdmins(tables)
	for _, row := range tables[groupsTab] {
		groupID, ok := id.Parse(row[idColumn])
		if !ok {
			return nil, fmt.Errorf("%s row %q has no valid %s: %q", groupsTab, row["Name"], idColumn, row[idColumn])
		}
		g := NormalizeList(EmailList{ID: groupID, Name: row["Name"], Title: row["Title"], Description: row["Description"], CreatedBy: row["Created By"], Created: row["Created"], Prefix: strings.ToLower(strings.TrimSpace(row[prefixColumn])) != prefixOff, Visibility: row[visibleColumn], Posting: row[postingColumn], Replying: row[replyingColumn]})
		if _, dup := m.byID[g.ID]; dup {
			return nil, fmt.Errorf("%s has two rows with %s %q", groupsTab, idColumn, g.ID)
		}
		if _, dup := m.byName[g.Name]; dup {
			return nil, fmt.Errorf("%s has two rows named %q", groupsTab, g.Name)
		}
		g.Aliases = []string{}
		g.Managers = []string{}
		g.Rules = []Rule{}
		g.Additions = []Addition{}
		g.Excluded = []Excluded{}
		m.byID[g.ID] = len(m.Groups)
		m.byName[g.Name] = len(m.Groups)
		m.Groups = append(m.Groups, g)
	}
	owner := func(tab, groupID string) (*EmailList, error) {
		g := m.Group(groupID)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", tab, groupID, groupsTab)
		}
		return g, nil
	}
	aliases, err := id.ParseAliases(tables[id.AliasesTab])
	if err != nil {
		return nil, err
	}
	for _, row := range tables[id.AliasesTab] {
		alias := strings.ToLower(strings.TrimSpace(row[id.AliasColumn]))
		g, err := owner(id.AliasesTab, aliases[alias])
		if err != nil {
			return nil, err
		}
		g.Aliases = append(g.Aliases, alias)
	}
	for _, row := range tables[managersTab] {
		g, err := owner(managersTab, row["Group"])
		if err != nil {
			return nil, err
		}
		email := mail.Normalize(row["Email"])
		if !slices.Contains(g.Managers, email) {
			g.Managers = append(g.Managers, email)
		}
	}
	for _, row := range tables[rulesTab] {
		g, err := owner(rulesTab, row["Group"])
		if err != nil {
			return nil, err
		}
		g.Rules = append(g.Rules, RuleFromRow(row))
	}
	for _, row := range tables[additionsTab] {
		g, err := owner(additionsTab, row["Group"])
		if err != nil {
			return nil, err
		}
		g.Additions = append(g.Additions, Addition{Email: row["Email"], Name: row["Name"]})
	}
	for _, row := range tables[excludedTab] {
		g, err := owner(excludedTab, row["Group"])
		if err != nil {
			return nil, err
		}
		g.Excluded = append(g.Excluded, Excluded{Email: row["Email"], Note: row["Note"], When: row["Timestamp"]})
	}
	for _, row := range tables[archivedTab] {
		g, err := owner(archivedTab, row["Group"])
		if err != nil {
			return nil, err
		}
		if m.archived[g.ID] == nil {
			m.archived[g.ID] = map[string]bool{}
		}
		m.archived[g.ID][mail.Normalize(row["Email"])] = true
	}
	for _, row := range tables[messagesTab] {
		m.Messages = append(m.Messages, ListMessage{
			ID: row["ID"], Group: strings.ToLower(strings.TrimSpace(row["Group"])), Received: row["Received"], From: row["From"], Subject: row["Subject"],
			State: row["State"], Recipients: row["Recipients"], Object: row["Object"], Detail: row["Detail"], MessageID: row["Message ID"],
		})
	}
	for _, row := range tables[deliveriesTab] {
		m.Deliveries = append(m.Deliveries, ListDelivery{
			Timestamp: row["Timestamp"], Group: strings.ToLower(strings.TrimSpace(row["Group"])), Email: mail.Normalize(row["Email"]),
			Event: row["Event"], Message: row["Message"], Detail: row["Detail"],
		})
	}
	for i, g := range m.Groups {
		g = NormalizeList(g)
		if err := CheckList(g); err != nil {
			return nil, err
		}
		slices.SortStableFunc(g.Excluded, compareWhen)
		m.Groups[i] = g
	}
	sort.SliceStable(m.Groups, func(i, j int) bool { return m.Groups[i].Name < m.Groups[j].Name })
	m.byID = map[string]int{}
	m.byName = map[string]int{}
	m.byAddress = map[string]int{}
	for i, g := range m.Groups {
		m.byID[g.ID] = i
		m.byName[g.Name] = i
		for _, local := range g.Names() {
			if j, taken := m.byAddress[local]; taken {
				return nil, fmt.Errorf("%s@%s reaches both %s and %s", local, ListDomain, m.Groups[j].Name, g.Name)
			}
			m.byAddress[local] = i
		}
	}
	m.indexHistory()
	return m, nil
}
