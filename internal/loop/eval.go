package loop

import "heliosian/internal/filter"

// The evaluator itself is internal/filter, shared with Heliosian's
// audiences; what is here reads a group's list of rules through it.

// The filter's types, as the sheet and the page have always named them.
type (
	Rule    = filter.Rule
	Sources = filter.Sources
	Reason  = filter.Reason
)

const (
	KindInclude = filter.KindInclude
	KindExclude = filter.KindExclude
)

// SharedKey is how a rule names a tag shared with its owner.
func SharedKey(owner, name string) string {
	return filter.SharedKey(owner, name)
}

// list is the group as the filter reads it: its rules, the additions on by
// hand, and the excluded addresses.
func list(g Group) filter.List {
	l := filter.List{Rules: g.Rules}
	for _, a := range g.Additions {
		l.Additions = append(l.Additions, a.Email)
	}
	for _, e := range g.Excluded {
		l.Excluded = append(l.Excluded, e.Email)
	}
	return l
}

// Reasons is everyone on a group and why (filter.Reasons).
func Reasons(g Group, s Sources) map[string][]Reason {
	return filter.Reasons(list(g), s)
}

// RuleCounts is how many people each of the group's rules touches
// (filter.RuleCounts).
func RuleCounts(g Group, s Sources) []int {
	return filter.RuleCounts(list(g), s)
}

// Members is Reasons' people alone, sorted.
func Members(g Group, s Sources) []string {
	return filter.Members(list(g), s)
}

// OnList says whether the rules or the additions place this person on the
// group, whatever the excluded list says - which is who sees a group
// visible to members.
func OnList(g Group, s Sources, email string) bool {
	l := list(g)
	l.Excluded = nil
	_, ok := filter.Reasons(l, s)[email]
	return ok
}
