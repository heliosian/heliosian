package model

import (
	"slices"
	"sort"
	"strings"
)

const (
	LateOutreach   = "outreach"
	LateInfo       = "info"
	LateNewsletter = "newsletter"
)

type Late struct {
	Name     string `json:"name"`
	Step     string `json:"step"`
	Due      string `json:"due"`
	Assignee string `json:"assignee,omitempty"`
	Path     string `json:"path"`
}

func (m *Model) Late(email string) []Late {
	email = strings.ToLower(strings.TrimSpace(email))
	bs := m.Birthdays
	admins := m.AdminList("birthday")
	admin := admins.IsAdmin(email)
	comms := slices.ContainsFunc(bs.Team, func(t TeamMember) bool { return t.Email == email && t.Role == RoleComms })
	if !bs.Sees(m.Directory.ActorOf(email, admins.Held(email))) {
		return []Late{}
	}
	v := birthdayViewer{directory: m.Directory}
	month, day, _ := ParseMonthDay(bs.Settings.YearStart)
	at := now()
	year := BirthdayYearContaining(at, month, day)
	today := at.Format(DateFormat)
	out := []Late{}
	for i := range bs.Birthdays {
		b := &bs.Birthdays[i]
		if !bs.InPipeline(b.Email) {
			continue
		}
		sv := v.staff(bs, b, year, at)
		if !sv.InDirectory {
			continue
		}
		mine := admin || sv.AssignedTo == email
		switch {
		case sv.Stage == StageComplete:
		case sv.Donation == nil && sv.DueBy != "" && sv.DueBy < today:
			if mine {
				out = append(out, Late{Name: sv.Name, Step: LateInfo, Due: sv.DueBy, Assignee: sv.AssignedToName, Path: staffPath(sv.Email)})
			}
		case sv.Stage == StageOutreach && sv.RequestBy != "" && sv.RequestBy < today:
			if mine {
				out = append(out, Late{Name: sv.Name, Step: LateOutreach, Due: sv.RequestBy, Assignee: sv.AssignedToName, Path: staffPath(sv.Email)})
			}
		case sv.Stage == StageNewsletter && sv.NewsletterDate != "" && sv.NewsletterDate < today:
			if admin || comms {
				out = append(out, Late{Name: sv.Name, Step: LateNewsletter, Due: sv.NewsletterDate, Path: staffPath(sv.Email)})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Due < out[j].Due })
	return out
}
