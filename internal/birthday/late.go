package birthday

import (
	"slices"
	"sort"
	"strings"
)

// The steps a birthday can fall behind on, each with a day it is due by:
// outreach, the assignee's, by the day to ask; the birthday's information -
// the charity - the assignee's too, by the day it is due (Due By Lead Days
// before the newsletter); and the newsletter, the comms team's, by the issue
// it goes in.
const (
	LateOutreach   = "outreach"
	LateInfo       = "info"
	LateNewsletter = "newsletter"
)

// Late is one birthday step past its day and not done, as the shared
// toolbar's alert lists it in every app: whose birthday, which step, the
// day it was due, who it waits on (blank for an unassigned outreach), and
// the staff member's page, a path on the birthday app's host.
type Late struct {
	Name     string `json:"name"`
	Step     string `json:"step"`
	Due      string `json:"due"`
	Assignee string `json:"assignee,omitempty"`
	Path     string `json:"path"`
}

// Late is what the viewer is behind on: a birthday assigned to them whose
// charity is not in by its due-by day, or, before then, whose outreach is
// past its day to ask and not marked done; and - on the comms team - a
// birthday with its charity in, past its newsletter's day and not yet
// marked used. One step a birthday, the furthest along that is late. An
// admin gets everyone's, the unassigned included. Oldest first.
func (c *Cache) Late(directory Directory, email string) []Late {
	email = strings.ToLower(strings.TrimSpace(email))
	model := c.Model()
	admin := c.IsAdmin(email)
	comms := slices.ContainsFunc(model.Team, func(t TeamMember) bool { return t.Email == email && t.Role == RoleComms })
	if !admin && !comms && !model.OnTeam(email) {
		return []Late{}
	}
	v := viewer{directory: directory}
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
		// Someone the directory no longer lists has left: nobody's job, as
		// the app's own lists have it (Render).
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
