package model

import (
	"slices"
	"strings"
	"time"

	"heliosian/internal/api"
)

const widgetIssues = 2

type birthdayStep struct {
	Step string `json:"step"`
	Day  string `json:"day"`
}

func nextStep(sv StaffView) *birthdayStep {
	switch sv.Stage {
	case StageWait, StageOutreach:
		return &birthdayStep{Step: LateOutreach, Day: sv.RequestBy}
	case StageResponse:
		return &birthdayStep{Step: LateInfo, Day: sv.DueBy}
	case StageNewsletter:
		return &birthdayStep{Step: LateNewsletter, Day: sv.NewsletterDate}
	}
	return nil
}

type birthdayDue struct{ key, day string }

func byDue(found []birthdayDue) []string {
	slices.SortStableFunc(found, func(a, b birthdayDue) int { return strings.Compare(a.day, b.day) })
	out := []string{}
	for _, f := range found {
		out = append(out, f.key)
	}
	return out
}

func (m *Model) birthdayMine(q api.Query) []string {
	bs := m.Birthdays
	if !bs.Sees(q.Actor) {
		return []string{}
	}
	issues := bs.nextIssues(q.Now)
	found := []birthdayDue{}
	for _, key := range bs.staffOrder {
		sv, _ := m.birthdayStaff(key, q.Now)
		next := nextStep(sv)
		if next == nil || next.Day == "" || next.Step == LateNewsletter || sv.AssignedTo != q.Actor.Email || !slices.Contains(issues, sv.NewsletterDate) || !bs.InPipeline(sv.Email) {
			continue
		}
		found = append(found, birthdayDue{key, next.Day})
	}
	return byDue(found)
}

func (m *Birthdays) nextIssues(now time.Time) []string {
	today := birthdayDayOf(now)
	issues := slices.Sorted(slices.Values(m.issueDates()))
	issues = slices.DeleteFunc(issues, func(d string) bool { return d < today })
	return issues[:min(len(issues), widgetIssues)]
}

func (m *Model) birthdayIssues(q api.Query) []string {
	bs := m.Birthdays
	if !bs.Sees(q.Actor) || !q.Actor.May(ConfigureBirthdays) {
		return []string{}
	}
	issues := bs.nextIssues(q.Now)
	found := []birthdayDue{}
	for _, key := range bs.staffOrder {
		sv, _ := m.birthdayStaff(key, q.Now)
		if !slices.Contains(issues, sv.NewsletterDate) || !bs.InPipeline(sv.Email) {
			continue
		}
		found = append(found, birthdayDue{key, sv.NewsletterDate})
	}
	return byDue(found)
}
