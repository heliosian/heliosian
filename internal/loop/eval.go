package loop

import (
	"slices"

	"heliosian/internal/filter"
)

type (
	Rule    = filter.Rule
	Sources = filter.Sources
	Reason  = filter.Reason
)

const (
	KindInclude = filter.KindInclude
	KindExclude = filter.KindExclude
)

func list(g Group) filter.List {
	l := filter.List{Rules: g.Rules, Editors: g.Managers}
	for _, a := range g.Additions {
		l.Additions = append(l.Additions, a.Email)
	}
	for _, e := range g.Excluded {
		l.Excluded = append(l.Excluded, e.Email)
	}
	return l
}

func Reasons(g Group, s Sources) map[string][]Reason {
	return filter.Reasons(list(g), s)
}

func RuleCounts(g Group, s Sources) []int {
	return filter.RuleCounts(list(g), s)
}

func Members(g Group, s Sources) []string {
	return filter.Members(list(g), s)
}

func (g Group) VisibleTo(email string, admin bool, s Sources) bool {
	if admin || g.Manages(email) || g.Visibility == VisibilityEveryone {
		return true
	}
	return g.Visibility == VisibilityMembers && OnList(g, s, email)
}

func (g Group) MailReadableBy(email string, s Sources) bool {
	if g.Manages(email) {
		return true
	}
	return g.Visibility != VisibilityHidden && slices.Contains(Members(g, s), email)
}

func (g Group) PostableBy(email string, reply bool, s Sources) bool {
	audience := g.Posting
	if reply {
		audience = g.Replying
	}
	switch {
	case audience == PostingEveryone || g.Manages(email):
		return true
	case audience == PostingMembers:
		return OnList(g, s, email)
	}
	return false
}

func OnList(g Group, s Sources, email string) bool {
	l := list(g)
	l.Excluded = nil
	_, ok := filter.Reasons(l, s)[email]
	return ok
}
