package filter

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/who"
)

const (
	KindInclude = "include"
	KindExclude = "exclude"
)

var (
	Roles     = []string{"Student", "Parent", "Staff"}
	Relations = []string{"Parents", "Children", "Siblings"}
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

var listKinds = []string{who.ListParty, who.ListActivity, who.ListRoom, who.ListGroup}

func TagKey(key string) string {
	return tagPrefix + key
}

func tagRef(tag string) (string, bool) {
	return strings.CutPrefix(tag, tagPrefix)
}

func CheckFacets(r Rule) error {
	for _, role := range r.Roles {
		if !slices.Contains(Roles, role) {
			return fmt.Errorf("role %q is not one of %s", role, strings.Join(Roles, ", "))
		}
	}
	for _, relation := range r.Family {
		if !slices.Contains(Relations, relation) {
			return fmt.Errorf("family relation %q is not one of %s", relation, strings.Join(Relations, ", "))
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
		if kind, rest, _ := strings.Cut(tag, ":"); !slices.Contains(listKinds, kind) || rest == "" {
			return fmt.Errorf("tag %q is not a tag's key or a Magic Tag's key", tag)
		}
	}
	return nil
}

type Sources struct {
	Directory *who.Model
	Tags      func(owner string) []who.Tag
	Lists     func(owner string) []who.List
	Shared    func(email string) []who.Tag
}

type reader struct {
	s      Sources
	tags   map[string][]who.Tag
	shared map[string][]who.Tag
	lists  map[string][]who.List
}

func (s Sources) reader() *reader {
	return &reader{s: s, tags: map[string][]who.Tag{}, shared: map[string][]who.Tag{}, lists: map[string][]who.List{}}
}

func (r *reader) tagsOf(owner string) []who.Tag {
	if _, ok := r.tags[owner]; !ok {
		if r.s.Tags != nil {
			r.tags[owner] = r.s.Tags(owner)
		} else {
			r.tags[owner] = nil
		}
	}
	return r.tags[owner]
}

func (r *reader) sharedWith(email string) []who.Tag {
	if _, ok := r.shared[email]; !ok {
		if r.s.Shared != nil {
			r.shared[email] = r.s.Shared(email)
		} else {
			r.shared[email] = nil
		}
	}
	return r.shared[email]
}

func (r *reader) listsOf(email string) []who.List {
	if _, ok := r.lists[email]; !ok {
		if r.s.Lists != nil {
			r.lists[email] = r.s.Lists(email)
		} else {
			r.lists[email] = nil
		}
	}
	return r.lists[email]
}

func (r *reader) managed(email, key string) (who.Tag, bool) {
	for _, t := range append(slices.Clone(r.tagsOf(email)), r.sharedWith(email)...) {
		if t.ID == key {
			return t, true
		}
	}
	return who.Tag{}, false
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
	for _, e := range editors {
		for _, l := range r.listsOf(e) {
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

func (s Sources) TagLabels(r Rule, editors []string, viewer string) []string {
	rd := s.reader()
	out := []string{}
	for _, tag := range r.Tags {
		key, isTag := tagRef(tag)
		if !isTag {
			label := tag + " (no longer a tag)"
			for _, e := range editors {
				if i := slices.IndexFunc(rd.listsOf(e), func(l who.List) bool { return l.Key == tag }); i >= 0 {
					label = rd.listsOf(e)[i].Name
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

func Writable(s Sources, saver string, editors []string, existing, rules []Rule) error {
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

func InRole(p *who.Person, roles []string) bool {
	return len(roles) == 0 || (p.IsStudent && slices.Contains(roles, "Student")) || (p.IsParent && slices.Contains(roles, "Parent")) || (p.IsStaff && slices.Contains(roles, "Staff"))
}

func Matches(r Rule, s Sources, tagged map[string][]string) map[string]Reason {
	model := s.Directory
	out := map[string]Reason{}
	for i := range model.People {
		p := &model.People[i]
		if r.Search != "" && !strings.Contains(strings.ToLower(p.FullName), r.Search) && !strings.Contains(strings.ToLower(p.Email), r.Search) {
			continue
		}
		if len(r.Grades) > 0 && !anyIn(model.Facets(p, false), r.Grades) {
			continue
		}
		if len(r.Classrooms) > 0 && !anyIn(model.Facets(p, true), r.Classrooms) {
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
			out[email] = Reason{Through: through, Via: via, ViaName: model.DisplayName(via)}
		}
	}
	for _, email := range SortedKeys(direct) {
		if slices.Contains(r.Family, "Parents") {
			for _, parent := range model.Parents(email) {
				reach(parent.Email, "Parents", email)
			}
		}
		if slices.Contains(r.Family, "Children") {
			for _, kid := range model.Children(email) {
				reach(kid.Email, "Children", email)
			}
		}
		if slices.Contains(r.Family, "Siblings") && model.Person(email).IsStudent {
			_, kids := model.Household(email)
			for _, kid := range kids {
				reach(kid.Email, "Siblings", email)
			}
		}
	}
	for email := range out {
		if !InRole(model.Person(email), r.Roles) {
			delete(out, email)
		}
	}
	return out
}

func SortedKeys(m map[string]bool) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type List struct {
	Rules     []Rule
	Additions []string
	Excluded  []string
	Editors   []string
}

func Reasons(l List, s Sources) map[string][]Reason {
	tagged := s.reader().tagged(l.Rules, l.Editors)
	in, out := map[string][]Reason{}, map[string]bool{}
	for i, r := range l.Rules {
		for email, reason := range Matches(r, s, tagged) {
			if r.Kind == KindExclude {
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
		if _, ok := in[email]; !ok {
			in[email] = []Reason{{Added: true}}
		}
	}
	for _, email := range l.Excluded {
		delete(in, email)
	}
	return in
}

func RuleCounts(l List, s Sources) []int {
	tagged := s.reader().tagged(l.Rules, l.Editors)
	real := func(email string) bool {
		p := s.Directory.Person(email)
		return p != nil && !p.EmailMasked
	}
	in := map[string]bool{}
	matched := []map[string]Reason{}
	for _, r := range l.Rules {
		reasons := Matches(r, s, tagged)
		matched = append(matched, reasons)
		if r.Kind == KindInclude {
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
			if r.Kind == KindInclude || in[email] {
				counts[i]++
			}
		}
	}
	return counts
}

func Members(l List, s Sources) []string {
	members := []string{}
	for email := range Reasons(l, s) {
		members = append(members, email)
	}
	sort.Strings(members)
	return members
}

func OnList(l List, s Sources, email string) bool {
	_, ok := Reasons(l, s)[s.Directory.Resolve(email)]
	return ok
}

type TagOption struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type ListOption struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Parent string `json:"parent,omitempty"`
}

type Options struct {
	Classrooms []string     `json:"classrooms"`
	Grades     []string     `json:"grades"`
	Tags       []TagOption  `json:"tags"`
	Lists      []ListOption `json:"lists"`
	Roles      []string     `json:"roles"`
	Relations  []string     `json:"relations"`
}

func OptionsFor(s Sources, viewer string) Options {
	model := s.Directory
	classrooms := []string{}
	for _, c := range model.Classrooms {
		classrooms = append(classrooms, c.Name)
	}
	present := map[string]bool{}
	for _, p := range model.People {
		if p.IsStudent && p.Grade != "" {
			present[p.Grade] = true
		}
	}
	grades := []string{}
	for _, g := range model.Grades {
		if present[g.Name] {
			grades = append(grades, g.Name)
		}
	}
	tags := []TagOption{}
	if s.Tags != nil {
		for _, t := range s.Tags(viewer) {
			tags = append(tags, TagOption{Key: TagKey(t.ID), Name: t.Name})
		}
	}
	if s.Shared != nil {
		for _, t := range s.Shared(viewer) {
			tags = append(tags, TagOption{Key: TagKey(t.ID), Name: t.Name + " (" + t.OwnerName + "'s)"})
		}
	}
	lists := []ListOption{}
	if s.Lists != nil {
		for _, l := range s.Lists(viewer) {
			lists = append(lists, ListOption{Key: l.Key, Name: l.Name, Kind: l.Kind, Parent: l.Parent})
		}
	}
	slices.SortFunc(lists, func(x, y ListOption) int { return strings.Compare(x.Name, y.Name) })
	return Options{Classrooms: classrooms, Grades: grades, Tags: tags, Lists: lists, Roles: Roles, Relations: Relations}
}

var RuleColumns = []string{"Kind", "Roles", "Search", "Classrooms", "Grades", "Tags", "Family"}

func RuleFromRow(row map[string]string) Rule {
	return Rule{
		Kind: row["Kind"], Roles: cells.SplitList(row["Roles"]), Search: row["Search"],
		Classrooms: cells.SplitList(row["Classrooms"]), Grades: cells.SplitList(row["Grades"]),
		Tags: cells.SplitList(row["Tags"]), Family: cells.SplitList(row["Family"]),
	}
}

func RuleCells(r Rule) map[string]string {
	return map[string]string{
		"Kind": r.Kind, "Roles": cells.JoinList(r.Roles), "Search": r.Search,
		"Classrooms": cells.JoinList(r.Classrooms), "Grades": cells.JoinList(r.Grades),
		"Tags": cells.JoinList(r.Tags), "Family": cells.JoinList(r.Family),
	}
}

func Clean(r Rule) Rule {
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

func Check(r Rule) error {
	if r.Kind != KindInclude && r.Kind != KindExclude {
		return fmt.Errorf("kind %q is not %s or %s", r.Kind, KindInclude, KindExclude)
	}
	if err := CheckFacets(r); err != nil {
		return err
	}
	if r.Empty() {
		return fmt.Errorf("a rule needs a role, some words, a classroom, a grade or a tag")
	}
	return nil
}
