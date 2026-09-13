package calendar

import (
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// Linked is an event another app runs - a Helios Celebrate party, an
// HCA-Team event - as the calendar lists it for one viewer: what and when,
// its page on that site, what a reader can do there now, and Mine, where the
// viewer's household already stands with it - MineGoing, MineWaitlisted, or
// nothing.
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
}

// builtinTags are the tags the load and the linked events file under that no
// sheet row names: one per app, one per standing the viewer's household can
// have with a linked event, and Misc for an event the sheet gave an audience
// but no category. Every view carries them beside the sheet's own so the
// rail can switch each off.
var builtinTags = []Tag{
	{Name: TagCelebrate, Description: "Fun(d)raiser parties on Helios Celebrate."},
	{Name: TagHCA, Description: "Events the HCA runs, from HCA-Team."},
	{Name: TagMisc, Description: "Events the sheet has not filed under a category."},
	{Name: TagGoing, Description: "Parties your household holds tickets to, and HCA events someone in it signed up for."},
	{Name: TagWaitlisted, Description: "Parties your household is on the waitlist for."},
}

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
		Start: l.Start, End: l.End, AllDay: allDay, Tags: []string{tagBySource[l.Source]}, Classrooms: []string{},
		Link: l.Path, Availability: l.Availability, Mine: l.Mine, start: start, end: end,
	}
	if t := tagByMine[l.Mine]; t != "" {
		e.Tags = append(e.Tags, t)
	}
	if e.End == "" {
		e.End = e.Start
	}
	return e
}

var noiseWords = map[string]bool{"hca": true, "helios": true, "the": true, "a": true, "an": true, "and": true, "of": true, "for": true, "at": true}

var yearForm = regexp.MustCompile(`^\d{4}$`)

// titleWords is a title as the words that carry it: lower-cased, without
// punctuation, the association's and the school's names, filler, and years.
func titleWords(title string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(title), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if !noiseWords[w] && !yearForm.MatchString(w) {
			out[w] = true
		}
	}
	return out
}

// sameEvent says an HCA-Team event and a school event are one thing: the
// same day, hours that overlap (or either whole-day), and titles that agree
// once the noise is out - one's words all in the other's, or at least half
// of the words they have between them shared.
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

// folded is the school's event carrying the HCA event's way in - its link,
// its availability, the viewer's standing, and the HCA tag with the standing's
// - on a copy, so the model's own event stays as loaded.
func folded(school, hca *Event) *Event {
	c := *school
	c.Tags = append(append([]string{}, school.Tags...), TagHCA)
	if t := tagByMine[hca.Mine]; t != "" {
		c.Tags = append(c.Tags, t)
	}
	c.Link, c.Availability, c.Mine = hca.Link, hca.Availability, hca.Mine
	return &c
}

// withLinked is the sheet's events and the linked ones as one list in date
// order, the way every list and the search read it. An HCA-Team event the
// school's calendar also lists is folded into the school's, which keeps its
// tags and audience and gains the way to sign up; the rest stand alone.
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
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].start.Equal(out[j].start) {
			return out[i].start.Before(out[j].start)
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return out
}
