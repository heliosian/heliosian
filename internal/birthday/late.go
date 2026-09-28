package birthday

import (
	"slices"
	"sort"
	"strings"

	"heliosian/internal/who"
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

func (c *Cache) Late(directory func() *who.Model, email string) []Late {
	email = strings.ToLower(strings.TrimSpace(email))
	model := c.Model()
	admin := c.IsAdmin(email)
	comms := slices.ContainsFunc(model.Team, func(t TeamMember) bool { return t.Email == email && t.Role == RoleComms })
	if !model.Sees(directory().ActorOf(email, c.Held(email))) {
		return []Late{}
	}
	v := viewer{directory: directory()}
	month, day, _ := ParseMonthDay(model.Settings.YearStart)
	at := now()
	year := YearContaining(at, month, day)
	today := at.Format(DateFormat)
	out := []Late{}
	for i := range model.Birthdays {
		b := &model.Birthdays[i]
		if !model.InPipeline(b.Email) {
			continue
		}
		sv := v.staff(model, b, year, at)
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
