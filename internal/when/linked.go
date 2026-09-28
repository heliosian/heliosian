package when

import (
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"heliosian/internal/config"
	"heliosian/internal/who"
)

type Linked struct {
	Source       string
	ID           string
	Title        string
	Summary      string
	Description  string
	Location     string
	Start        string
	End          string
	Path         string
	Availability string
	Mine         string
	Who          []string
	People       []Standing
	Image        string
	Hosts        []string
}

type Standing struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
	Mine bool   `json:"mine,omitempty"`
}

var builtinTags = []Tag{
	{Name: TagCelebrate, Description: "Fun(d)raiser parties on Helios Celebrate.", Group: BuiltinGroup, Default: true, BuiltIn: true},
	{Name: TagHCA, Description: "Events the HCA runs, from HCA-Team.", Group: BuiltinGroup, Default: true, BuiltIn: true},
	{Name: TagMisc, Description: "Events the sheet has not filed under a category.", Group: BuiltinGroup, Default: true, BuiltIn: true},
	{Name: TagGoing, Description: "Events you said yes to, parties your household holds tickets to, and HCA events someone in it signed up for.", Group: BuiltinGroup, Default: true, BuiltIn: true},
	{Name: TagWaitlisted, Description: "Parties your household is on the waitlist for.", Group: BuiltinGroup, Default: true, BuiltIn: true},
}

const BuiltinGroup = "Other"

var tagBySource = map[string]string{SourceCelebrate: TagCelebrate, SourceTeam: TagHCA}

var tagByMine = map[string]string{MineGoing: TagGoing, MineWaitlisted: TagWaitlisted}

func linkedEvent(l Linked) *Event {
	start, end, allDay, err := parseWhen(l.Start, l.End)
	if err != nil {
		slog.Error("calendar: linked event skipped", "source", l.Source, "id", l.ID, "error", err)
		return nil
	}
	e := &Event{
		ID: l.Source + "/" + l.ID, Source: l.Source, Title: l.Title, Location: l.Location,
		Description: strings.TrimSpace(l.Summary + "\n\n" + l.Description),
		Start:       l.Start, End: l.End, AllDay: allDay, Tags: []string{tagBySource[l.Source]}, Classrooms: []string{},
		Link: l.Path, Availability: l.Availability, Mine: l.Mine, MineWho: l.Who, MinePeople: l.People, Image: l.Image, Hosts: l.Hosts, Sharing: SharingPublic, start: start, end: end,
	}
	if t := tagByMine[l.Mine]; t != "" {
		e.Tags = append(e.Tags, t)
	}
	if e.End == "" {
		e.End = e.Start
	}
	e.Dates = e.written()
	standing(e)
	return e
}

var noiseWords = map[string]bool{"hca": true, "helios": true, "the": true, "a": true, "an": true, "and": true, "of": true, "for": true, "at": true}

var yearForm = regexp.MustCompile(`^\d{4}$`)

func titleWords(title string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !noiseWords[w] && !yearForm.MatchString(w) {
			out[w] = true
		}
	}
	return out
}

func sameEvent(a, b *Event) bool {
	if a.start.Format(DateFormat) != b.start.Format(DateFormat) {
		return false
	}
	if !a.AllDay && !b.AllDay && (a.start.After(b.end) || b.start.After(a.end)) {
		return false
	}
	x, y := titleWords(a.Title), titleWords(b.Title)
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	shared := 0
	for w := range x {
		if y[w] {
			shared++
		}
	}
	return shared == len(x) || shared == len(y) || shared*2 >= len(x)+len(y)-shared
}

func folded(school, hca *Event) *Event {
	c := *school
	c.Tags = append(append([]string{}, school.Tags...), TagHCA)
	if t := tagByMine[hca.Mine]; t != "" {
		c.Tags = append(c.Tags, t)
	}
	c.Link, c.Availability, c.Mine, c.MineWho, c.MinePeople, c.Hosts = hca.Link, hca.Availability, hca.Mine, hca.MineWho, hca.MinePeople, hca.Hosts
	c.LinkedID = strings.TrimPrefix(hca.ID, SourceTeam+"/")
	if c.Image == "" {
		c.Image = hca.Image
	}
	if len(hca.Description) > len(c.Description) {
		c.Description = hca.Description
	}
	if c.Location == "" {
		c.Location = hca.Location
	}
	standing(&c)
	return &c
}

func (m *Model) eventsFor(directory *who.Model, email string, linked []Linked) []*Event {
	email = config.NormalizeEmail(email)
	answers := m.Answers[email]
	mine := m.mine(directory, email)
	events := m.Events
	for _, e := range m.Pending {
		if !e.Cancelled && (answers[e.ID] != "" || m.invitedAny(mine, e.ID)) {
			events = append(events[:len(events):len(events)], e)
		}
	}
	out := withLinked(events, linked)
	for i, e := range out {
		going := answers[e.ID] == AnswerYes && !slices.Contains(e.Tags, TagGoing)
		invitation := m.Invitations[e.ID] != nil && !e.Invitation
		invited := m.invitedAny(mine, e.ID) && !e.Invited
		if !going && !invitation && !invited {
			continue
		}
		if invited {
			e = m.invitedEvent(e)
		}
		c := *e
		if going {
			c.Tags = append(append([]string{}, e.Tags...), TagGoing)
		}
		c.Invitation = c.Invitation || invitation
		c.Invited = c.Invited || invited
		out[i] = &c
	}
	return out
}

func (e *Event) linked() bool {
	return e.Link != ""
}

func (e *Event) linkedID() string {
	if e.LinkedID != "" {
		return e.LinkedID
	}
	return strings.TrimPrefix(strings.TrimPrefix(e.ID, SourceCelebrate+"/"), SourceTeam+"/")
}

func withLinked(events []*Event, linked []Linked) []*Event {
	out := append([]*Event{}, events...)
	for _, l := range linked {
		e := linkedEvent(l)
		if e == nil {
			continue
		}
		if l.Source == SourceTeam {
			i := slices.IndexFunc(out, func(o *Event) bool { return o.Link == "" && o.Source != SourceCelebrate && sameEvent(o, e) })
			if i >= 0 {
				out[i] = folded(out[i], e)
				continue
			}
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, byStart)
	return out
}
