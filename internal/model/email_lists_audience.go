package model

import (
	"slices"

	"heliosian/internal/access"
)

func (g EmailList) audience() Audience {
	l := Audience{Rules: g.Rules, Editors: g.Managers}
	for _, a := range g.Additions {
		l.Additions = append(l.Additions, a.Email)
	}
	for _, e := range g.Excluded {
		l.Excluded = append(l.Excluded, e.Email)
	}
	return l
}

func (g EmailList) Reasons(s AudienceSources) map[string][]Reason {
	return g.audience().Reasons(s)
}

func (g EmailList) RuleCounts(s AudienceSources) []int {
	return g.audience().RuleCounts(s)
}

func (g EmailList) Members(s AudienceSources) []string {
	return g.audience().Members(s)
}

var (
	SeeAllLists  = access.Named("loop.see-all")
	ActAsManager = access.Named("loop.act-as-manager")
)

var EmailListsAdminAllowances = []access.Allowance{SeeAllLists, ActAsManager}

func (g EmailList) Edits(v access.Actor) bool {
	return v.May(ActAsManager) || g.Manages(v.Email)
}

func (g EmailList) Sees(v access.Actor) bool {
	return v.May(SeeAllLists) || g.Manages(v.Email)
}

func (g EmailList) VisibleTo(v access.Actor, s AudienceSources) bool {
	return g.visibleWith(v, func() bool { return g.OnList(s, v.Email) })
}

func (g EmailList) visibleWith(v access.Actor, onList func() bool) bool {
	if g.Sees(v) || g.Visibility == VisibilityEveryone {
		return true
	}
	return g.Visibility == VisibilityMembers && onList()
}

func (g EmailList) For(v access.Actor, s AudienceSources) *EmailList {
	if !g.VisibleTo(v, s) {
		return nil
	}
	if !g.Sees(v) {
		g.Rules = []Rule{}
		g.Excluded = []Excluded{}
	}
	return &g
}

func (g EmailList) MailReadableBy(email string, s AudienceSources) bool {
	if g.Manages(email) {
		return true
	}
	return g.Visibility != VisibilityHidden && slices.Contains(g.Members(s), email)
}

func (g EmailList) PostableBy(email string, reply bool, s AudienceSources) bool {
	audience := g.Posting
	if reply {
		audience = g.Replying
	}
	switch {
	case audience == PostingEveryone || g.Manages(email):
		return true
	case audience == PostingMembers:
		return g.OnList(s, email)
	}
	return false
}

func (g EmailList) OnList(s AudienceSources, email string) bool {
	l := g.audience()
	l.Excluded = nil
	_, ok := l.Reasons(s)[email]
	return ok
}
