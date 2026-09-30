package model

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/id"
)

const (
	RuleInclude = "include"
	RuleExclude = "exclude"
)

var (
	AudienceRoles     = []string{"Student", "Parent", "Staff"}
	AudienceRelations = []string{"Parents", "Children", "Siblings"}
)

type Rule struct {
	Kind       string   `json:"kind"`
	Roles      []string `json:"roles"`
	Search     string   `json:"search"`
	Classrooms []string `json:"classrooms"`
	Grades     []string `json:"grades"`
	Tags       []string `json:"tags"`
	Family     []string `json:"family"`
}

func (r Rule) Empty() bool {
	return len(r.Roles)+len(r.Classrooms)+len(r.Grades)+len(r.Tags) == 0 && r.Search == ""
}

func (r Rule) Same(o Rule) bool {
	return r.Kind == o.Kind && r.Search == o.Search &&
		slices.Equal(r.Roles, o.Roles) && slices.Equal(r.Classrooms, o.Classrooms) &&
		slices.Equal(r.Grades, o.Grades) && slices.Equal(r.Tags, o.Tags) && slices.Equal(r.Family, o.Family)
}

const (
	tagPrefix  = "tag:"
	deletedTag = "Deleted tag"
)

var magicTagKinds = []string{MagicTagParty, MagicTagActivity, MagicTagRoom, MagicTagGroup}

func TagKey(key string) string {
	return tagPrefix + key
}

func tagRef(tag string) (string, bool) {
	return strings.CutPrefix(tag, tagPrefix)
}

func (r Rule) CheckFacets() error {
	for _, role := range r.Roles {
		if !slices.Contains(AudienceRoles, role) {
			return fmt.Errorf("role %q is not one of %s", role, strings.Join(AudienceRoles, ", "))
		}
	}
	for _, relation := range r.Family {
		if !slices.Contains(AudienceRelations, relation) {
			return fmt.Errorf("family relation %q is not one of %s", relation, strings.Join(AudienceRelations, ", "))
		}
	}
	for _, tag := range r.Tags {
		if strings.Contains(tag, ",") {
			return fmt.Errorf("a tag with a comma in its name cannot be used in a rule")
		}
		if key, isTag := tagRef(tag); isTag {
			if _, ok := id.Parse(key); !ok {
				return fmt.Errorf("tag %q does not name a tag by its ID", tag)
			}
			continue
		}
		if kind, rest, _ := strings.Cut(tag, ":"); !slices.Contains(magicTagKinds, kind) || rest == "" {
			return fmt.Errorf("tag %q is not a tag's key or a Magic Tag's key", tag)
		}
	}
	return nil
}

type AudienceSources struct {
	Directory  *Directory
	MagicTags  func(holder string) []MagicTag
	EmailLists *EmailLists
}

func (s AudienceSources) managedLists(email string) []EmailList {
	out := []EmailList{}
	for _, g := range s.EmailLists.Groups {
		if g.Manages(email) && !s.EmailLists.Archived(g.ID, email) {
			out = append(out, g)
		}
	}
	return out
}

type reader struct {
	s          AudienceSources
	tags       map[string][]Tag
	shared     map[string][]Tag
	magicTags  map[string][]MagicTag
	members    map[string][]string
	evaluating map[string]bool
}

func (s AudienceSources) reader() *reader {
	return &reader{s: s, tags: map[string][]Tag{}, shared: map[string][]Tag{}, magicTags: map[string][]MagicTag{}, members: map[string][]string{}, evaluating: map[string]bool{}}
}

func listRef(tag string) (string, bool) {
	return strings.CutPrefix(tag, MagicTagGroup+":")
}

func (r *reader) managedList(key string, editors []string) *EmailList {
	g := r.s.EmailLists.Group(key)
	if g == nil || !slices.ContainsFunc(editors, g.Manages) {
		return nil
	}
	return g
}

func (r *reader) listMembers(g *EmailList) []string {
	if members, ok := r.members[g.ID]; ok {
		return members
	}
	if r.evaluating[g.ID] {
		return nil
	}
	r.evaluating[g.ID] = true
	members := []string{}
	for email := range g.audience().reasonsWith(r) {
		members = append(members, email)
	}
	sort.Strings(members)
	delete(r.evaluating, g.ID)
	r.members[g.ID] = members
	return members
}

func (r *reader) tagsOf(owner string) []Tag {
	if _, ok := r.tags[owner]; !ok {
		r.tags[owner] = r.s.Directory.Tags(owner)
	}
	return r.tags[owner]
}

func (r *reader) sharedWith(email string) []Tag {
	if _, ok := r.shared[email]; !ok {
		r.shared[email] = r.s.Directory.SharedTags(email)
	}
	return r.shared[email]
}

func (r *reader) magicTagsOf(email string) []MagicTag {
	if _, ok := r.magicTags[email]; !ok {
		r.magicTags[email] = r.s.MagicTags(email)
	}
	return r.magicTags[email]
}

func (r *reader) managed(email, key string) (Tag, bool) {
	for _, t := range append(slices.Clone(r.tagsOf(email)), r.sharedWith(email)...) {
		if t.ID == key {
			return t, true
		}
	}
	return Tag{}, false
}

func (r *reader) people(tag string, editors []string) ([]string, bool) {
	if key, isTag := tagRef(tag); isTag {
		for _, e := range editors {
			if t, ok := r.managed(e, key); ok {
				return t.People, true
			}
		}
		return nil, false
	}
	if key, isList := listRef(tag); isList {
		g := r.managedList(key, editors)
		if g == nil {
			return nil, false
		}
		return r.listMembers(g), true
	}
	for _, e := range editors {
		for _, l := range r.magicTagsOf(e) {
			if l.Key == tag {
				return l.People, true
			}
		}
	}
	return nil, false
}

func (r *reader) tagged(rules []Rule, editors []string) map[string][]string {
	out := map[string][]string{}
	for _, rule := range rules {
		for _, tag := range rule.Tags {
			if _, done := out[tag]; done {
				continue
			}
			if people, ok := r.people(tag, editors); ok {
				out[tag] = people
			}
		}
	}
	return out
}

func (s AudienceSources) TagLabels(r Rule, editors []string, viewer string) []string {
	rd := s.reader()
	out := []string{}
	for _, tag := range r.Tags {
		if key, isList := listRef(tag); isList {
			label := tag + " (no longer a tag)"
			if g := rd.managedList(key, editors); g != nil {
				label = g.Title
			}
			out = append(out, label)
			continue
		}
		key, isTag := tagRef(tag)
		if !isTag {
			label := tag + " (no longer a tag)"
			for _, e := range editors {
				if i := slices.IndexFunc(rd.magicTagsOf(e), func(l MagicTag) bool { return l.Key == tag }); i >= 0 {
					label = rd.magicTagsOf(e)[i].Name
					break
				}
			}
			out = append(out, label)
			continue
		}
		t, found := s.Directory.Tag(key)
		if !found {
			out = append(out, deletedTag)
			continue
		}
		label := t.Name
		if t.Owner != viewer {
			label += " (" + t.OwnerName + "'s)"
		}
		if _, ok := rd.people(tag, editors); !ok {
			label += " (no longer shared)"
		}
		out = append(out, label)
	}
	return out
}

func (s AudienceSources) Writable(saver string, editors []string, existing, rules []Rule) error {
	rd := s.reader()
	for i, r := range rules {
		if slices.ContainsFunc(existing, r.Same) {
			continue
		}
		for _, tag := range r.Tags {
			if _, ok := rd.people(tag, []string{saver}); !ok {
				return fmt.Errorf("rule %d names a tag that is not yours", i+1)
			}
			if _, ok := rd.people(tag, editors); !ok {
				return fmt.Errorf("rule %d names a tag none of the people who edit this can read", i+1)
			}
		}
	}
	return nil
}

func anyIn(have, want []string) bool {
	for _, h := range have {
		if slices.Contains(want, h) {
			return true
		}
	}
	return false
}

type Reason struct {
	Rule    int    `json:"rule"`
	Through string `json:"through,omitempty"`
	Via     string `json:"via,omitempty"`
	ViaName string `json:"viaName,omitempty"`
	Added   bool   `json:"added,omitempty"`
}

func inRole(p *Person, roles []string) bool {
	return len(roles) == 0 || (p.IsStudent && slices.Contains(roles, "Student")) || (p.IsParent && slices.Contains(roles, "Parent")) || (p.IsStaff && slices.Contains(roles, "Staff"))
}

func (r Rule) matches(d *Directory, tagged map[string][]string) map[string]Reason {
	out := map[string]Reason{}
	for i := range d.People {
		p := &d.People[i]
		if r.Search != "" && !strings.Contains(strings.ToLower(p.FullName), r.Search) && !strings.Contains(strings.ToLower(p.Email), r.Search) {
			continue
		}
		if len(r.Grades) > 0 && !anyIn(d.Facets(p, false), r.Grades) {
			continue
		}
		if len(r.Classrooms) > 0 && !anyIn(d.Facets(p, true), r.Classrooms) {
			continue
		}
		if len(r.Tags) > 0 && !slices.ContainsFunc(r.Tags, func(tag string) bool { return slices.Contains(tagged[tag], p.Email) }) {
			continue
		}
		out[p.Email] = Reason{}
	}
	direct := map[string]bool{}
	for email := range out {
		direct[email] = true
	}
	reach := func(email, through, via string) {
		if _, ok := out[email]; !ok && email != via {
			out[email] = Reason{Through: through, Via: via, ViaName: d.DisplayName(via)}
		}
	}
	d.relatives(SortedKeys(direct), r.Family, reach)
	for email := range out {
		if !inRole(d.Person(email), r.Roles) {
			delete(out, email)
		}
	}
	return out
}

func (d *Directory) relatives(emails, family []string, reach func(email, through, via string)) {
	for _, email := range emails {
		if slices.Contains(family, "Parents") {
			for _, parent := range d.Parents(email) {
				reach(parent.Email, "Parents", email)
			}
		}
		if slices.Contains(family, "Children") {
			for _, kid := range d.Children(email) {
				reach(kid.Email, "Children", email)
			}
		}
		if slices.Contains(family, "Siblings") && d.Person(email).IsStudent {
			_, kids := d.Household(email)
			for _, kid := range kids {
				reach(kid.Email, "Siblings", email)
			}
		}
	}
}

func SortedKeys(m map[string]bool) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type Audience struct {
	Rules     []Rule
	Additions []string
	Excluded  []string
	Editors   []string
}

func (l Audience) Reasons(s AudienceSources) map[string][]Reason {
	return l.reasonsWith(s.reader())
}

func (l Audience) reasonsWith(rd *reader) map[string][]Reason {
	s := rd.s
	tagged := rd.tagged(l.Rules, l.Editors)
	in, out := map[string][]Reason{}, map[string]bool{}
	for i, r := range l.Rules {
		for email, reason := range r.matches(s.Directory, tagged) {
			if r.Kind == RuleExclude {
				out[email] = true
				continue
			}
			reason.Rule = i
			in[email] = append(in[email], reason)
		}
	}
	for _, reasons := range in {
		slices.SortFunc(reasons, func(a, b Reason) int { return a.Rule - b.Rule })
	}
	for email := range in {
		if p := s.Directory.Person(email); out[email] || p == nil || p.EmailMasked {
			delete(in, email)
		}
	}
	for _, email := range l.Additions {
		email = s.Directory.Resolve(email)
		if p := s.Directory.Person(email); p != nil && p.EmailMasked {
			continue
		}
		if _, ok := in[email]; !ok {
			in[email] = []Reason{{Added: true}}
		}
	}
	for _, email := range l.Excluded {
		delete(in, email)
	}
	return in
}

func (l Audience) RuleCounts(s AudienceSources) []int {
	tagged := s.reader().tagged(l.Rules, l.Editors)
	real := func(email string) bool {
		p := s.Directory.Person(email)
		return p != nil && !p.EmailMasked
	}
	in := map[string]bool{}
	matched := []map[string]Reason{}
	for _, r := range l.Rules {
		reasons := r.matches(s.Directory, tagged)
		matched = append(matched, reasons)
		if r.Kind == RuleInclude {
			for email := range reasons {
				if real(email) {
					in[email] = true
				}
			}
		}
	}
	counts := make([]int, len(l.Rules))
	for i, r := range l.Rules {
		for email := range matched[i] {
			if !real(email) {
				continue
			}
			if r.Kind == RuleInclude || in[email] {
				counts[i]++
			}
		}
	}
	return counts
}

func (l Audience) Members(s AudienceSources) []string {
	members := []string{}
	for email := range l.Reasons(s) {
		members = append(members, email)
	}
	sort.Strings(members)
	return members
}

func (l Audience) Includes(s AudienceSources, email string) bool {
	_, ok := l.Reasons(s)[s.Directory.Resolve(email)]
	return ok
}

type TagOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type MagicTagOption struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Parent string `json:"parent,omitempty"`
}

type AudienceOptions struct {
	Classrooms []string         `json:"classrooms"`
	Grades     []string         `json:"grades"`
	Tags       []TagOption      `json:"tags"`
	Lists      []MagicTagOption `json:"lists"`
	Roles      []string         `json:"roles"`
	Relations  []string         `json:"relations"`
}

func (s AudienceSources) Options(viewer string) AudienceOptions {
	d := s.Directory
	classrooms := []string{}
	for _, c := range d.Classrooms {
		classrooms = append(classrooms, c.Name)
	}
	present := map[string]bool{}
	for _, p := range d.People {
		if p.IsStudent && p.Grade != "" {
			present[p.Grade] = true
		}
	}
	grades := []string{}
	for _, g := range d.Grades {
		if present[g.Name] {
			grades = append(grades, g.Name)
		}
	}
	tags := []TagOption{}
	for _, t := range d.Tags(viewer) {
		tags = append(tags, TagOption{Key: TagKey(t.ID), Name: t.Name})
	}
	for _, t := range d.SharedTags(viewer) {
		tags = append(tags, TagOption{Key: TagKey(t.ID), Name: t.Name + " (" + t.OwnerName + "'s)"})
	}
	lists := []MagicTagOption{}
	for _, l := range s.MagicTags(viewer) {
		if l.Archived {
			continue
		}
		lists = append(lists, MagicTagOption{Key: l.Key, Name: l.Name, Kind: l.Kind, Parent: l.Parent})
	}
	for _, g := range s.managedLists(viewer) {
		lists = append(lists, MagicTagOption{Key: MagicTagGroup + ":" + g.ID, Name: g.Title, Kind: MagicTagGroup})
	}
	slices.SortFunc(lists, func(x, y MagicTagOption) int { return strings.Compare(x.Name, y.Name) })
	return AudienceOptions{Classrooms: classrooms, Grades: grades, Tags: tags, Lists: lists, Roles: AudienceRoles, Relations: AudienceRelations}
}

var RuleColumns = []string{"Kind", "Roles", "Search", "Classrooms", "Grades", "Tags", "Family"}

func RuleFromRow(row map[string]string) Rule {
	return Rule{
		Kind: row["Kind"], Roles: cells.SplitList(row["Roles"]), Search: row["Search"],
		Classrooms: cells.SplitList(row["Classrooms"]), Grades: cells.SplitList(row["Grades"]),
		Tags: cells.SplitList(row["Tags"]), Family: cells.SplitList(row["Family"]),
	}
}

func (r Rule) Cells() map[string]string {
	return map[string]string{
		"Kind": r.Kind, "Roles": cells.JoinList(r.Roles), "Search": r.Search,
		"Classrooms": cells.JoinList(r.Classrooms), "Grades": cells.JoinList(r.Grades),
		"Tags": cells.JoinList(r.Tags), "Family": cells.JoinList(r.Family),
	}
}

func (r Rule) Clean() Rule {
	tags := []string{}
	for _, t := range r.Tags {
		t = strings.TrimSpace(t)
		if key, isTag := tagRef(t); isTag {
			if parsed, ok := id.Parse(key); ok {
				t = TagKey(parsed)
			}
		}
		if t != "" && !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return Rule{
		Kind:       strings.ToLower(strings.TrimSpace(r.Kind)),
		Roles:      cells.SplitList(cells.JoinList(r.Roles)),
		Search:     strings.Join(strings.Fields(strings.ToLower(r.Search)), " "),
		Classrooms: cells.SplitList(cells.JoinList(r.Classrooms)),
		Grades:     cells.SplitList(cells.JoinList(r.Grades)),
		Tags:       tags,
		Family:     cells.SplitList(cells.JoinList(r.Family)),
	}
}

func (r Rule) Check() error {
	if r.Kind != RuleInclude && r.Kind != RuleExclude {
		return fmt.Errorf("kind %q is not %s or %s", r.Kind, RuleInclude, RuleExclude)
	}
	if err := r.CheckFacets(); err != nil {
		return err
	}
	if r.Empty() {
		return fmt.Errorf("a rule needs a role, some words, a classroom, a grade or a tag")
	}
	return nil
}
