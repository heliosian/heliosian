package model

import (
	"slices"

	"heliosian/internal/api"
)

const (
	LateOutreach   = "outreach"
	LateInfo       = "info"
	LateNewsletter = "newsletter"
)

type late struct {
	Step string `json:"step"`
	Due  string `json:"due"`
}

func (m *Model) lateFor(q api.Query, sv StaffView) *late {
	bs := m.Birthdays
	email := q.Actor.Email
	if !bs.InPipeline(sv.Email) || !sv.InDirectory || !bs.Sees(q.Actor) {
		return nil
	}
	admin := q.Actor.May(ManageAdmins("birthday"))
	comms := slices.ContainsFunc(bs.Team, func(t TeamMember) bool { return t.Email == email && t.Role == RoleComms })
	mine := admin || sv.AssignedTo == email
	today := q.Now.Format(DateFormat)
	switch {
	case sv.Stage == StageComplete:
	case sv.Donation == nil && sv.DueBy != "" && sv.DueBy < today:
		if mine {
			return &late{Step: LateInfo, Due: sv.DueBy}
		}
	case sv.Stage == StageOutreach && sv.RequestBy != "" && sv.RequestBy < today:
		if mine {
			return &late{Step: LateOutreach, Due: sv.RequestBy}
		}
	case sv.Stage == StageNewsletter && sv.NewsletterDate != "" && sv.NewsletterDate < today:
		if admin || comms {
			return &late{Step: LateNewsletter, Due: sv.NewsletterDate}
		}
	}
	return nil
}
