package loop

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/store"
)

const (
	appName       = "groups"
	groupsTab     = "Groups"
	managersTab   = "Managers"
	rulesTab      = "Rules"
	additionsTab  = "Additions"
	excludedTab   = "Excluded"
	aliasesTab    = "Aliases"
	messagesTab   = "Messages"
	deliveriesTab = "Deliveries"
	adminsTab     = "Admins"
	archivedTab   = "Archived"

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

	Domain = "loop.heliosian.com"

	maxTitleLength       = 80
	maxDescriptionLength = 300
	maxSearchLength      = 80
	maxRules             = 40
	maxAdditions         = 200
	maxNameLength        = 80
	maxExcluded          = 200
	maxNoteLength        = 120
	maxAliases           = 10
)

var (
	GroupColumns    = []string{"Name", "Title", "Description", "Created By", "Created", prefixColumn, visibleColumn, postingColumn, replyingColumn}
	ManagerColumns  = []string{"Group", "Email"}
	RuleColumns     = append([]string{"Group"}, filter.RuleColumns...)
	AdditionColumns = []string{"Group", "Email", "Name"}
	ExcludedColumns = []string{"Group", "Email", "Note", "Timestamp"}
	AliasColumns    = []string{"Group", "Alias"}
	MessageColumns  = []string{"ID", "Group", "Received", "From", "Subject", "State", "Recipients", "Object", "Detail", "Message ID"}
	DeliveryColumns = []string{"Timestamp", "Group", "Email", "Event", "Message", "Detail"}
	AdminColumns    = []string{"Email"}
	ArchivedColumns = []string{"Group", "Email"}

	Roles     = filter.Roles
	Relations = filter.Relations

	Visibilities = []string{VisibilityHidden, VisibilityMembers, VisibilityEveryone}

	Postings = []string{PostingEveryone, PostingMembers, PostingManagers}
)

var nameForm = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,38}[a-z0-9]$`)

var reservedNames = []string{"abuse", "admin", "administrator", "hostmaster", "noreply", "no-reply", "postmaster", "root", unsubscribeLocal, "webmaster"}

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type Addition struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Excluded struct {
	Email string `json:"email"`
	Note  string `json:"note"`
	When  string `json:"when"`
}

type Group struct {
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

func (g Group) HasExcluded(email string) bool {
	return slices.ContainsFunc(g.Excluded, func(e Excluded) bool { return e.Email == email })
}

func (g Group) Names() []string {
	return append([]string{g.Name}, g.Aliases...)
}

func (g Group) Addition(email string) *Addition {
	for i := range g.Additions {
		if g.Additions[i].Email == email {
			return &g.Additions[i]
		}
	}
	return nil
}

func (g Group) Address() string {
	return g.Name + "@" + Domain
}

func (g Group) Path() string {
	return "/groups/" + g.Name
}

func (g Group) Manages(email string) bool {
	return slices.Contains(g.Managers, email)
}

type Message struct {
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

type Delivery struct {
	Timestamp string
	Group     string
	Email     string
	Event     string
	Message   string
	Detail    string
}

type Model struct {
	Groups     []Group
	Messages   []Message
	Deliveries []Delivery
	admins     []string
	byName     map[string]int
	byAddress  map[string]int
	archived   map[string]map[string]bool
}

func (m *Model) Archived(name, email string) bool {
	return m.archived[name][email]
}

func (m *Model) Group(name string) *Group {
	i, ok := m.byName[name]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

func (m *Model) Resolve(local string) *Group {
	i, ok := m.byAddress[local]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

func SplitList(cell string) []string {
	out := []string{}
	for _, item := range strings.Split(cell, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func JoinList(items []string) string {
	return strings.Join(items, ", ")
}

func cleanEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func cleanEmails(raw []string) []string {
	out := []string{}
	for _, e := range raw {
		e = cleanEmail(e)
		if e != "" && !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	return out
}

func CheckName(name string) error {
	if !nameForm.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("a group's name is two to forty lowercase letters, digits, dots and hyphens, starting and ending with a letter or digit, with no two dots together")
	}
	if slices.Contains(reservedNames, name) {
		return fmt.Errorf("%s is reserved", name)
	}
	return nil
}

func CheckRule(r Rule) error {
	if r.Kind != KindInclude && r.Kind != KindExclude {
		return fmt.Errorf("kind %q is not %s or %s", r.Kind, KindInclude, KindExclude)
	}
	if err := filter.CheckFacets(r); err != nil {
		return err
	}
	if len(r.Search) > maxSearchLength {
		return fmt.Errorf("the search words are too long")
	}
	if r.Empty() {
		return fmt.Errorf("a rule needs a role, some words, a classroom, a grade or a tag")
	}
	return nil
}

func CheckGroup(g Group) error {
	if err := CheckName(g.Name); err != nil {
		return err
	}
	if len(g.Aliases) > maxAliases {
		return fmt.Errorf("group %s has too many aliases", g.Name)
	}
	for _, alias := range g.Aliases {
		if err := CheckName(alias); err != nil {
			return fmt.Errorf("group %s, alias %q: %w", g.Name, alias, err)
		}
		if alias == g.Name {
			return fmt.Errorf("group %s: an alias cannot be the group's own name", g.Name)
		}
	}
	if strings.TrimSpace(g.Title) == "" || len(g.Title) > maxTitleLength {
		return fmt.Errorf("group %s needs a short title", g.Name)
	}
	if len(g.Description) > maxDescriptionLength {
		return fmt.Errorf("group %s: the description is too long", g.Name)
	}
	if !slices.Contains(Visibilities, g.Visibility) {
		return fmt.Errorf("group %s: visibility %q is not one of %s", g.Name, g.Visibility, JoinList(Visibilities))
	}
	if !slices.Contains(Postings, g.Posting) {
		return fmt.Errorf("group %s: posting %q is not one of %s", g.Name, g.Posting, JoinList(Postings))
	}
	if !slices.Contains(Postings, g.Replying) {
		return fmt.Errorf("group %s: replying %q is not one of %s", g.Name, g.Replying, JoinList(Postings))
	}
	if len(g.Managers) == 0 {
		return fmt.Errorf("group %s needs at least one manager", g.Name)
	}
	for _, m := range g.Managers {
		if !emailForm.MatchString(m) {
			return fmt.Errorf("group %s: manager %q is not an email address", g.Name, m)
		}
	}
	if len(g.Rules) > maxRules {
		return fmt.Errorf("group %s has too many rules", g.Name)
	}
	includes := 0
	for i, r := range g.Rules {
		if err := CheckRule(r); err != nil {
			return fmt.Errorf("group %s, rule %d: %w", g.Name, i+1, err)
		}
		if r.Kind == KindInclude {
			includes++
		}
	}
	if includes == 0 {
		return fmt.Errorf("group %s needs at least one include rule", g.Name)
	}
	if len(g.Additions) > maxAdditions {
		return fmt.Errorf("group %s has too many people added by hand", g.Name)
	}
	for _, a := range g.Additions {
		if !emailForm.MatchString(a.Email) {
			return fmt.Errorf("group %s: %q is not an email address", g.Name, a.Email)
		}
		if len(a.Name) > maxNameLength {
			return fmt.Errorf("group %s: the name for %s is too long", g.Name, a.Email)
		}
	}
	if len(g.Excluded) > maxExcluded {
		return fmt.Errorf("group %s has too many excluded addresses", g.Name)
	}
	for _, e := range g.Excluded {
		if !emailForm.MatchString(e.Email) {
			return fmt.Errorf("group %s: excluded %q is not an email address", g.Name, e.Email)
		}
		if len(e.Note) > maxNoteLength {
			return fmt.Errorf("group %s: the note for %s is too long", g.Name, e.Email)
		}
	}
	return nil
}

func Normalize(g Group) Group {
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
	g.Managers = cleanEmails(g.Managers)
	rules := make([]Rule, 0, len(g.Rules))
	for _, r := range g.Rules {
		rules = append(rules, filter.Clean(r))
	}
	g.Rules = rules
	additions := make([]Addition, 0, len(g.Additions))
	for _, a := range g.Additions {
		a.Email = cleanEmail(a.Email)
		a.Name = strings.Join(strings.Fields(a.Name), " ")
		if a.Email != "" && !slices.ContainsFunc(additions, func(b Addition) bool { return b.Email == a.Email }) {
			additions = append(additions, a)
		}
	}
	g.Additions = additions
	excluded := make([]Excluded, 0, len(g.Excluded))
	for _, e := range g.Excluded {
		e.Email = cleanEmail(e.Email)
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

func ruleCells(name string, r Rule) store.Row {
	cells := filter.RuleCells(r)
	cells["Group"] = name
	return cells
}

func prefixCell(on bool) string {
	if on {
		return ""
	}
	return prefixOff
}

func groupCells(g Group) store.Row {
	return store.Row{"Name": g.Name, "Title": g.Title, "Description": g.Description, "Created By": g.CreatedBy, "Created": g.Created, prefixColumn: prefixCell(g.Prefix), visibleColumn: g.Visibility, postingColumn: g.Posting, replyingColumn: g.Replying}
}

func BuildModel(tables store.Tables) (*Model, error) {
	model := &Model{Groups: []Group{}, Messages: []Message{}, Deliveries: []Delivery{}, byName: map[string]int{}, archived: map[string]map[string]bool{}}
	admins := []string{}
	for _, row := range tables[adminsTab] {
		admins = append(admins, row["Email"])
	}
	model.admins = config.NormalizeEmails(admins)
	for _, row := range tables[groupsTab] {
		g := Normalize(Group{Name: row["Name"], Title: row["Title"], Description: row["Description"], CreatedBy: row["Created By"], Created: row["Created"], Prefix: strings.ToLower(strings.TrimSpace(row[prefixColumn])) != prefixOff, Visibility: row[visibleColumn], Posting: row[postingColumn], Replying: row[replyingColumn]})
		if _, dup := model.byName[g.Name]; dup {
			return nil, fmt.Errorf("%s has two rows named %q", groupsTab, g.Name)
		}
		g.Aliases = []string{}
		g.Managers = []string{}
		g.Rules = []Rule{}
		g.Additions = []Addition{}
		g.Excluded = []Excluded{}
		model.byName[g.Name] = len(model.Groups)
		model.Groups = append(model.Groups, g)
	}
	owner := func(tab string, row store.Row) (*Group, error) {
		g := model.Group(strings.ToLower(strings.TrimSpace(row["Group"])))
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", tab, row["Group"], groupsTab)
		}
		return g, nil
	}
	for _, row := range tables[aliasesTab] {
		g, err := owner(aliasesTab, row)
		if err != nil {
			return nil, err
		}
		g.Aliases = append(g.Aliases, row["Alias"])
	}
	for _, row := range tables[managersTab] {
		g, err := owner(managersTab, row)
		if err != nil {
			return nil, err
		}
		email := cleanEmail(row["Email"])
		if !slices.Contains(g.Managers, email) {
			g.Managers = append(g.Managers, email)
		}
	}
	for _, row := range tables[rulesTab] {
		g, err := owner(rulesTab, row)
		if err != nil {
			return nil, err
		}
		g.Rules = append(g.Rules, filter.RuleFromRow(row))
	}
	for _, row := range tables[additionsTab] {
		g, err := owner(additionsTab, row)
		if err != nil {
			return nil, err
		}
		g.Additions = append(g.Additions, Addition{Email: row["Email"], Name: row["Name"]})
	}
	for _, row := range tables[excludedTab] {
		g, err := owner(excludedTab, row)
		if err != nil {
			return nil, err
		}
		g.Excluded = append(g.Excluded, Excluded{Email: row["Email"], Note: row["Note"], When: row["Timestamp"]})
	}
	for _, row := range tables[archivedTab] {
		g, err := owner(archivedTab, row)
		if err != nil {
			return nil, err
		}
		if model.archived[g.Name] == nil {
			model.archived[g.Name] = map[string]bool{}
		}
		model.archived[g.Name][cleanEmail(row["Email"])] = true
	}
	for _, row := range tables[messagesTab] {
		model.Messages = append(model.Messages, Message{
			ID: row["ID"], Group: strings.ToLower(strings.TrimSpace(row["Group"])), Received: row["Received"], From: row["From"], Subject: row["Subject"],
			State: row["State"], Recipients: row["Recipients"], Object: row["Object"], Detail: row["Detail"], MessageID: row["Message ID"],
		})
	}
	for _, row := range tables[deliveriesTab] {
		model.Deliveries = append(model.Deliveries, Delivery{
			Timestamp: row["Timestamp"], Group: strings.ToLower(strings.TrimSpace(row["Group"])), Email: cleanEmail(row["Email"]),
			Event: row["Event"], Message: row["Message"], Detail: row["Detail"],
		})
	}
	for i, g := range model.Groups {
		g = Normalize(g)
		if err := CheckGroup(g); err != nil {
			return nil, err
		}
		slices.SortStableFunc(g.Excluded, compareWhen)
		model.Groups[i] = g
	}
	sort.SliceStable(model.Groups, func(i, j int) bool { return model.Groups[i].Name < model.Groups[j].Name })
	model.byName = map[string]int{}
	model.byAddress = map[string]int{}
	for i, g := range model.Groups {
		model.byName[g.Name] = i
		for _, local := range g.Names() {
			if j, taken := model.byAddress[local]; taken {
				return nil, fmt.Errorf("%s@%s reaches both %s and %s", local, Domain, model.Groups[j].Name, g.Name)
			}
			model.byAddress[local] = i
		}
	}
	return model, nil
}
