package model

import (
	"slices"
	"strings"
)

const (
	inviteReminder = "reminder"
	inviteUpdate   = "update"
)

func (a calendarApp) recipients(e *Event, emails []string) ([]string, map[string][]string) {
	model := a.model()
	order := []string{}
	cc := map[string][]string{}
	for _, email := range emails {
		if model.InviteOf(e.ID, email) == nil || isGuestKey(email) || slices.Contains(order, email) {
			continue
		}
		with, reachable := a.ccFor(email)
		if !reachable {
			continue
		}
		cc[email] = with
		order = append(order, email)
	}
	return order, cc
}

func (a calendarApp) ccFor(email string) ([]string, bool) {
	directory := a.directory()
	p := directory.Person(email)
	if p == nil {
		return nil, true
	}
	if p.EmailMasked {
		return nil, false
	}
	cc := []string{}
	if !isAdult(p) {
		adults, kids := directory.Household(email)
		for _, member := range append(adults, kids...) {
			if isAdult(member) && !slices.Contains(cc, member.Email) {
				cc = append(cc, member.Email)
			}
		}
	}
	return cc, true
}

func (a calendarApp) reachable(e *Event, emails []string) int {
	order, _ := a.recipients(e, emails)
	return len(order)
}

func FirstWord(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}
