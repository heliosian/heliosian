// Package loop serves Helios Loop: named email groups drawn from the directory by rules, each an address mail is received for and forwarded from.
package loop

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/data"
	"heliosian/internal/filter"
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
	changeLogTab  = "Change Log"

	prefixColumn = "Subject Prefix"
	prefixOff    = "off"

	visibleColumn = "Visible"

	VisibilityHidden   = "hidden"
	VisibilityMembers  = "members"
	VisibilityEveryone = "everyone"

	// Domain is where every group lives: a group named parents-k is
	// parents-k@loop.heliosian.com.
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
	GroupColumns     = []string{"Name", "Title", "Description", "Created By", "Created", prefixColumn, visibleColumn}
	ManagerColumns   = []string{"Group", "Email"}
	RuleColumns      = []string{"Group", "Kind", "Roles", "Search", "Classrooms", "Grades", "Tags", "Family", "Owner"}
	AdditionColumns  = []string{"Group", "Email", "Name"}
	ExcludedColumns  = []string{"Group", "Email", "Note", "Timestamp"}
	AliasColumns     = []string{"Group", "Alias"}
	MessageColumns   = []string{"ID", "Group", "Received", "From", "Subject", "State", "Recipients", "Object", "Detail", "Source", "Message ID"}
	DeliveryColumns  = []string{"Timestamp", "Group", "Email", "Event", "Message", "Detail"}
	AdminColumns     = []string{"Email"}
	ArchivedColumns  = []string{"Group", "Email"}
	ChangeLogColumns = []string{"Timestamp", "Actor", "Action", "Group", "Detail"}

	// Roles are the role facet's values, and Relations the Family facet's,
	// as the filter has them.
	Roles     = filter.Roles
	Relations = filter.Relations

	Visibilities = []string{VisibilityHidden, VisibilityMembers, VisibilityEveryone}
)

// nameForm is a group's name: the local part of its address, two to forty
// characters of lowercase letters, digits, dots and hyphens, neither end a
// dot or a hyphen.
var nameForm = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,38}[a-z0-9]$`)

var reservedNames = []string{"abuse", "admin", "administrator", "hostmaster", "noreply", "no-reply", "postmaster", "root", "webmaster"}

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Addition is someone on a group by hand rather than by rule: an address
// the directory does not hold, and the name a manager typed for it.
type Addition struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Excluded struct {
	Email string `json:"email"`
	Note  string `json:"note"`
	When  string `json:"when"`
}

// Group is one group: its name, which is its address's local part and never
// changes, the other local parts it answers as, what it is called and for,
// who manages it, its rules, the people added by hand from outside the
// directory, and the addresses kept off it whatever the rules say.
type Group struct {
	Name        string     `json:"name"`
	Aliases     []string   `json:"aliases"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	CreatedBy   string     `json:"createdBy,omitempty"`
	Created     string     `json:"created,omitempty"`
	Prefix      bool       `json:"prefix"`
	Visibility  string     `json:"visibility"`
	Managers    []string   `json:"managers"`
	Rules       []Rule     `json:"rules"`
	Additions   []Addition `json:"additions"`
	Excluded    []Excluded `json:"excluded"`
}

func (g Group) HasExcluded(email string) bool {
	return slices.ContainsFunc(g.Excluded, func(e Excluded) bool { return e.Email == email })
}

// Names is every local part the group answers as: its name, then its aliases.
func (g Group) Names() []string {
	return append([]string{g.Name}, g.Aliases...)
}

// Addition finds the group's addition at an address, nil for none.
func (g Group) Addition(email string) *Addition {
	for i := range g.Additions {
		if g.Additions[i].Email == email {
			return &g.Additions[i]
		}
	}
	return nil
}

// Address is the group's email address.
func (g Group) Address() string {
	return g.Name + "@" + Domain
}

func (g Group) Path() string {
	return "/groups/" + g.Name
}

// Manages says whether email is one of the group's managers.
func (g Group) Manages(email string) bool {
	return slices.Contains(g.Managers, email)
}

// Model is the sheet organized: every group by name, in name order, and
// every local part - name or alias - to the group it reaches.
type Model struct {
	Groups    []Group
	byName    map[string]int
	byAddress map[string]int
	// archived is who has put each group away for themselves, by the
	// group's name: a personal tidy that changes nothing about the group.
	archived map[string]map[string]bool
}

// Archived reports whether email has archived the named group: it then
// sits under Archived in their rail rather than among their groups, and
// its Magic Tag stays off Who?'s lists for them, though its pages in both
// apps still open.
func (m *Model) Archived(name, email string) bool {
	return m.archived[name][email]
}

// Group finds a group by name, nil for none.
func (m *Model) Group(name string) *Group {
	i, ok := m.byName[name]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

// Resolve finds the group a local part reaches, by its name or an alias,
// nil for none.
func (m *Model) Resolve(local string) *Group {
	i, ok := m.byAddress[local]
	if !ok {
		return nil
	}
	return &m.Groups[i]
}

type Tables struct {
	Groups     []map[string]string
	Managers   []map[string]string
	Rules      []map[string]string
	Additions  []map[string]string
	Excluded   []map[string]string
	Aliases    []map[string]string
	Messages   []map[string]string
	Deliveries []map[string]string
	Admins     []map[string]string
	Archived   []map[string]string
}

func ReadTables(source data.Source) (*Tables, error) {
	type table struct {
		name string
		want []string
		rows []map[string]string
	}
	groups := &table{name: groupsTab, want: GroupColumns}
	managers := &table{name: managersTab, want: ManagerColumns}
	rules := &table{name: rulesTab, want: RuleColumns}
	additions := &table{name: additionsTab, want: AdditionColumns}
	excluded := &table{name: excludedTab, want: ExcludedColumns}
	aliases := &table{name: aliasesTab, want: AliasColumns}
	messages := &table{name: messagesTab, want: MessageColumns}
	deliveries := &table{name: deliveriesTab, want: DeliveryColumns}
	admins := &table{name: adminsTab, want: AdminColumns}
	archived := &table{name: archivedTab, want: ArchivedColumns}
	changeLog := &table{name: changeLogTab, want: ChangeLogColumns}
	read := []*table{groups, managers, rules, additions, excluded, aliases, messages, deliveries, admins, archived}
	names := []string{}
	for _, t := range read {
		names = append(names, t.name)
	}
	tabs, err := source.Tabs(appName, names, []string{changeLog.name})
	if err != nil {
		return nil, err
	}
	for _, t := range append(read, changeLog) {
		t.rows = tabs[t.name].Rows
		if err := data.CheckColumns(t.name, tabs[t.name].Header, t.want); err != nil {
			return nil, err
		}
	}
	return &Tables{Groups: groups.rows, Managers: managers.rows, Rules: rules.rows, Additions: additions.rows, Excluded: excluded.rows, Aliases: aliases.rows, Messages: messages.rows, Deliveries: deliveries.rows, Admins: admins.rows, Archived: archived.rows}, nil
}

// SplitList reads a list cell: comma-separated, trimmed, without repeats.
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

// CheckName refuses a name that cannot be a group's: not the address form,
// or one mail systems reserve.
func CheckName(name string) error {
	if !nameForm.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("a group's name is two to forty lowercase letters, digits, dots and hyphens, starting and ending with a letter or digit, with no two dots together")
	}
	if slices.Contains(reservedNames, name) {
		return fmt.Errorf("%s is reserved", name)
	}
	return nil
}

// CheckRule refuses a rule that says nothing, or says something the facets
// cannot mean.
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
	if !emailForm.MatchString(r.Owner) {
		return fmt.Errorf("a rule needs an owner")
	}
	if r.Empty() {
		return fmt.Errorf("a rule needs a role, some words, a classroom, a grade or a tag")
	}
	return nil
}

// CheckGroup refuses a group the sheet's rules do not allow, so one never
// reaches the sheet and refuses the next load.
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

// Normalize trims and lowercases what the sheet and the editor may have
// spelled loosely, so every check and comparison sees one form.
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
	g.Managers = cleanEmails(g.Managers)
	rules := make([]Rule, 0, len(g.Rules))
	for _, r := range g.Rules {
		r.Kind = strings.ToLower(strings.TrimSpace(r.Kind))
		r.Roles = SplitList(JoinList(r.Roles))
		r.Search = strings.Join(strings.Fields(strings.ToLower(r.Search)), " ")
		r.Classrooms = SplitList(JoinList(r.Classrooms))
		r.Grades = SplitList(JoinList(r.Grades))
		r.Tags = trimmed(r.Tags)
		r.Family = SplitList(JoinList(r.Family))
		r.Owner = cleanEmail(r.Owner)
		rules = append(rules, r)
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

func trimmed(items []string) []string {
	out := []string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(out, item) {
			out = append(out, item)
		}
	}
	return out
}

func ruleFromRow(row map[string]string) Rule {
	return Rule{
		Kind: row["Kind"], Roles: SplitList(row["Roles"]), Search: row["Search"],
		Classrooms: SplitList(row["Classrooms"]), Grades: SplitList(row["Grades"]),
		Tags: SplitList(row["Tags"]), Family: SplitList(row["Family"]), Owner: row["Owner"],
	}
}

func ruleCells(name string, r Rule) map[string]string {
	return map[string]string{
		"Group": name, "Kind": r.Kind, "Roles": JoinList(r.Roles), "Search": r.Search,
		"Classrooms": JoinList(r.Classrooms), "Grades": JoinList(r.Grades),
		"Tags": JoinList(r.Tags), "Family": JoinList(r.Family), "Owner": r.Owner,
	}
}

func additionCells(name string, a Addition) map[string]string {
	return map[string]string{"Group": name, "Email": a.Email, "Name": a.Name}
}

func excludedCells(name string, e Excluded) map[string]string {
	return map[string]string{"Group": name, "Email": e.Email, "Note": e.Note, "Timestamp": e.When}
}

func aliasCells(name, alias string) map[string]string {
	return map[string]string{"Group": name, "Alias": alias}
}

func prefixCell(on bool) string {
	if on {
		return ""
	}
	return prefixOff
}

func groupCells(g Group) map[string]string {
	return map[string]string{"Name": g.Name, "Title": g.Title, "Description": g.Description, "Created By": g.CreatedBy, "Created": g.Created, prefixColumn: prefixCell(g.Prefix), visibleColumn: g.Visibility}
}

// BuildModel validates every row and refuses the whole set on the first
// problem, the stance every app takes: a sheet edit that breaks a rule
// surfaces as a refused load, never as a group quietly matching nobody.
func BuildModel(tables *Tables) (*Model, error) {
	model := &Model{Groups: []Group{}, byName: map[string]int{}, archived: map[string]map[string]bool{}}
	for _, row := range tables.Groups {
		g := Normalize(Group{Name: row["Name"], Title: row["Title"], Description: row["Description"], CreatedBy: row["Created By"], Created: row["Created"], Prefix: strings.ToLower(strings.TrimSpace(row[prefixColumn])) != prefixOff, Visibility: row[visibleColumn]})
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
	for _, row := range tables.Aliases {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		g := model.Group(name)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", aliasesTab, row["Group"], groupsTab)
		}
		g.Aliases = append(g.Aliases, row["Alias"])
	}
	for _, row := range tables.Managers {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		g := model.Group(name)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", managersTab, row["Group"], groupsTab)
		}
		email := cleanEmail(row["Email"])
		if !slices.Contains(g.Managers, email) {
			g.Managers = append(g.Managers, email)
		}
	}
	for _, row := range tables.Rules {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		g := model.Group(name)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", rulesTab, row["Group"], groupsTab)
		}
		g.Rules = append(g.Rules, ruleFromRow(row))
	}
	for _, row := range tables.Additions {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		g := model.Group(name)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", additionsTab, row["Group"], groupsTab)
		}
		g.Additions = append(g.Additions, Addition{Email: row["Email"], Name: row["Name"]})
	}
	for _, row := range tables.Excluded {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		g := model.Group(name)
		if g == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", excludedTab, row["Group"], groupsTab)
		}
		g.Excluded = append(g.Excluded, Excluded{Email: row["Email"], Note: row["Note"], When: row["Timestamp"]})
	}
	for _, row := range tables.Archived {
		name := strings.ToLower(strings.TrimSpace(row["Group"]))
		if model.Group(name) == nil {
			return nil, fmt.Errorf("%s names %q, which %s does not have", archivedTab, row["Group"], groupsTab)
		}
		if model.archived[name] == nil {
			model.archived[name] = map[string]bool{}
		}
		model.archived[name][cleanEmail(row["Email"])] = true
	}
	for i, g := range model.Groups {
		g = Normalize(g)
		if err := CheckGroup(g); err != nil {
			return nil, err
		}
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

func cloneRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, len(rows))
	for i, row := range rows {
		out[i] = maps.Clone(row)
	}
	return out
}

func withoutGroupRows(rows []map[string]string, column, name string) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row[column]), name) {
			out = append(out, row)
		}
	}
	return out
}

// withGroup mirrors what saving a group writes: its Groups row set or
// added, and its Managers, Rules and Additions rows replaced whole.
func (t *Tables) withGroup(g Group) *Tables {
	out := *t
	out.Groups = cloneRows(t.Groups)
	found := false
	for _, row := range out.Groups {
		if strings.EqualFold(strings.TrimSpace(row["Name"]), g.Name) {
			for column, value := range groupCells(g) {
				if value == "" {
					delete(row, column)
				} else {
					row[column] = value
				}
			}
			found = true
		}
	}
	if !found {
		out.Groups = append(out.Groups, groupCells(g))
	}
	out.Managers = withoutGroupRows(t.Managers, "Group", g.Name)
	for _, m := range g.Managers {
		out.Managers = append(out.Managers, map[string]string{"Group": g.Name, "Email": m})
	}
	out.Rules = withoutGroupRows(t.Rules, "Group", g.Name)
	for _, r := range g.Rules {
		out.Rules = append(out.Rules, ruleCells(g.Name, r))
	}
	out.Additions = withoutGroupRows(t.Additions, "Group", g.Name)
	for _, a := range g.Additions {
		out.Additions = append(out.Additions, additionCells(g.Name, a))
	}
	out.Excluded = withoutGroupRows(t.Excluded, "Group", g.Name)
	for _, e := range g.Excluded {
		out.Excluded = append(out.Excluded, excludedCells(g.Name, e))
	}
	out.Aliases = withoutGroupRows(t.Aliases, "Group", g.Name)
	for _, alias := range g.Aliases {
		out.Aliases = append(out.Aliases, aliasCells(g.Name, alias))
	}
	return &out
}

func (t *Tables) withoutGroup(name string) *Tables {
	out := *t
	out.Groups = withoutGroupRows(t.Groups, "Name", name)
	out.Managers = withoutGroupRows(t.Managers, "Group", name)
	out.Rules = withoutGroupRows(t.Rules, "Group", name)
	out.Additions = withoutGroupRows(t.Additions, "Group", name)
	out.Excluded = withoutGroupRows(t.Excluded, "Group", name)
	out.Aliases = withoutGroupRows(t.Aliases, "Group", name)
	out.Archived = withoutGroupRows(t.Archived, "Group", name)
	return &out
}

// withArchived is the group put away for one person, or brought back: the
// Archived row for the pair added or removed, once each.
func (t *Tables) withArchived(name, email string, archived bool) *Tables {
	out := *t
	out.Archived = make([]map[string]string, 0, len(t.Archived)+1)
	for _, row := range t.Archived {
		if !strings.EqualFold(strings.TrimSpace(row["Group"]), name) || cleanEmail(row["Email"]) != email {
			out.Archived = append(out.Archived, row)
		}
	}
	if archived {
		out.Archived = append(out.Archived, map[string]string{"Group": name, "Email": email})
	}
	return &out
}

func (t *Tables) withMessage(id, group string, cells map[string]string) *Tables {
	out := *t
	out.Messages = cloneRows(t.Messages)
	for _, row := range out.Messages {
		if row["ID"] != id || !strings.EqualFold(row["Group"], group) {
			continue
		}
		for column, value := range cells {
			row[column] = value
		}
		return &out
	}
	row := map[string]string{"ID": id, "Group": group}
	for column, value := range cells {
		row[column] = value
	}
	out.Messages = append(out.Messages, row)
	return &out
}

func (t *Tables) withDelivery(row map[string]string) *Tables {
	out := *t
	out.Deliveries = append(cloneRows(t.Deliveries), row)
	return &out
}

func (t *Tables) withAdmins(emails []string) *Tables {
	out := *t
	out.Admins = make([]map[string]string, 0, len(emails))
	for _, email := range emails {
		out.Admins = append(out.Admins, map[string]string{"Email": email})
	}
	return &out
}
