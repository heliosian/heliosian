package ask

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/artifacts"
	"heliosian/internal/calendar"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

//go:embed prompt.md
var school string

func systemBlocks(v *viewer, recent []*artifacts.Document, l *links) []anthropic.BetaTextBlockParam {
	return []anthropic.BetaTextBlockParam{
		{Text: l.shorten(school + "\n\n" + lingo(v)), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		{Text: l.shorten(viewerBlock(v) + "\n" + recentBlock(v, recent) + "\n" + linkExamples(v)), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
	}
}

func recentDocuments(v *viewer) []*artifacts.Document {
	since := v.now.AddDate(0, 0, -recentDays).Format(calendar.DateFormat)
	out := []*artifacts.Document{}
	for _, d := range v.documents().Documents {
		if len(out) >= recentLimit || d.Date < since {
			break
		}
		if d.Kind == artifacts.KindPortal {
			continue
		}
		out = append(out, d)
	}
	return out
}

func linkExamples(v *viewer) string {
	b := &strings.Builder{}
	b.WriteString("## How links look\n\nThe page draws these links, from this person's own data, as chips:\n")
	for _, address := range v.exampleLinks() {
		card, ok := v.linkCard(address)
		if !ok {
			continue
		}
		shown := "a small mark"
		switch {
		case card.Badge != "":
			shown = fmt.Sprintf("a badge reading %q", card.Badge)
		case card.Image != "":
			shown = "a picture"
		}
		fmt.Fprintf(b, "- [%s](%s) shows %s, then the words %q.\n", card.Name, address, shown, card.Name)
	}
	return b.String()
}

func (v *viewer) exampleLinks() []string {
	out := []string{}
	keys := v.directory.FamilyKeysOf(v.email)
	if len(keys) > 0 {
		family := v.directory.Families[keys[0]]
		out = append(out, whoBase+who.FamilyPath(keys[0]))
		for _, email := range family.KidEmails {
			if k := v.directory.Person(email); k != nil {
				out = append(out, whoLink(k.Email))
				if k.Classroom != "" {
					out = append(out, whoBase+who.ClassroomPath(k.Classroom))
				}
				break
			}
		}
	}
	today := v.now.Format(calendar.DateFormat)
	for _, e := range v.calendar.EventsFor(v.email, v.sources.Linked(v.email)) {
		if app, _ := calendar.Page(e); app == "calendar" && e.Start >= today {
			out = append(out, eventLink(e))
			break
		}
	}
	for _, a := range v.team.Activities {
		if a.Year == team.SchoolYear(v.now) && v.team.VisibleTo(a, v.email, false) {
			out = append(out, teamBase+v.team.PathOf(a))
			break
		}
	}
	for _, p := range v.celebrate.SortedParties("") {
		if p.VisibleTo(v.email, false) && !p.Past(v.now) {
			out = append(out, celebrateBase+v.celebrate.PathOf(p))
			break
		}
	}
	sources := v.sources.LoopSources()
	for _, g := range v.loop.Groups {
		if g.VisibleTo(v.email, false, sources) {
			out = append(out, loopBase+g.Path())
			break
		}
	}
	return out
}

func recentBlock(v *viewer, recent []*artifacts.Document) string {
	if len(recent) == 0 {
		return fmt.Sprintf("## Recent documents\n\nNo documents have come in over the last %d days.\n", recentDays)
	}
	return fmt.Sprintf("## Recent documents\n\nThe newest documents of the last %d days, newest first, each with its key for read_document:\n", recentDays) + documentLines(v, recent)
}

func arrivals(v *viewer, fresh []*artifacts.Document) string {
	return "New documents have come in since this conversation began, newest first, each with its key for read_document:\n" + documentLines(v, fresh) +
		"\nA new document can change what the calendar or the other apps say; read one that bears on what is being asked."
}

func documentLines(v *viewer, docs []*artifacts.Document) string {
	b := &strings.Builder{}
	for _, d := range docs {
		when := d.Date
		if day, err := time.ParseInLocation(calendar.DateFormat, d.Date, calendar.Location); err == nil {
			when = day.Format("Monday, January 2, 2006")
		}
		fmt.Fprintf(b, "- %s, %s: %s (key %s", when, v.timing(d.Date, ""), d.Title, d.Key)
		if url := d.URL(); url != "" {
			b.WriteString(", " + url)
		}
		if group := v.groupOf(d); group != "" {
			b.WriteString(", mail to the group " + group)
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func lingo(v *viewer) string {
	b := &strings.Builder{}
	b.WriteString("## The school as the data has it\n\nGrades and their bands:\n")
	for _, g := range v.directory.Grades {
		fmt.Fprintf(b, "- %s: %s\n", g.Name, g.Band)
	}
	bands := []string{}
	bandGrades := map[string][]string{}
	bandClassrooms := map[string][]string{}
	for _, c := range v.calendar.Roster.Classrooms {
		if !slices.Contains(bands, c.Band) {
			bands = append(bands, c.Band)
		}
		for _, g := range c.Grades {
			if !slices.Contains(bandGrades[c.Band], g) {
				bandGrades[c.Band] = append(bandGrades[c.Band], g)
			}
		}
		bandClassrooms[c.Band] = append(bandClassrooms[c.Band], c.Name)
	}
	b.WriteString("\nBands (the grades of its students; its classrooms):\n")
	for _, band := range bands {
		fmt.Fprintf(b, "- %s (%s; %s)\n", band, strings.Join(bandGrades[band], ", "), strings.Join(bandClassrooms[band], ", "))
	}
	b.WriteString("\nClassrooms (its link; band; the grades of its students; its teachers; its crews):\n")
	for _, c := range v.calendar.Roster.Classrooms {
		line := fmt.Sprintf("- %s (%s; %s; %s", c.Name, whoBase+who.ClassroomPath(c.Name), c.Band, strings.Join(c.Grades, ", "))
		if teachers := v.classroomTeachers(c.Name); len(teachers) > 0 {
			line += "; teachers " + strings.Join(teachers, ", ")
		}
		if len(c.Crews) > 0 {
			line += "; crews " + strings.Join(c.Crews, ", ")
		}
		b.WriteString(line + ")\n")
	}
	b.WriteString("\nStaff departments: " + strings.Join(v.directory.Departments, "; ") + "\n")
	b.WriteString("\nCalendar categories (what each files):\n")
	for _, t := range v.calendar.Tags {
		fmt.Fprintf(b, "- %s: %s\n", t.Name, t.Description)
	}
	b.WriteString("- Celebrate: fun(d)raiser parties from Helios Celebrate.\n- HCA: events the HCA runs, from HCA-Team.\n")
	b.WriteString("\nDay types (the school day's hours):\n")
	for _, d := range v.calendar.DayTypes {
		parts := []string{}
		for _, block := range d.Blocks {
			parts = append(parts, fmt.Sprintf("%s %s-%s", strings.ToLower(block.Name), block.Start, block.End))
		}
		if len(parts) == 0 {
			parts = []string{"no school"}
		}
		fmt.Fprintf(b, "- %s: %s\n", d.Name, strings.Join(parts, ", "))
	}
	b.WriteString("\nSchool years the calendar holds:\n")
	for _, y := range v.calendar.Years {
		fmt.Fprintf(b, "- %s: first day %s, last day %s\n", y.Label, y.FirstDay, y.LastDay)
	}
	if c := v.celebrate.Current(); c != nil {
		fmt.Fprintf(b, "\nThe current celebration on Helios Celebrate is %s", c.Title)
		if c.Start != "" {
			fmt.Fprintf(b, " on %s", c.Start)
		}
		b.WriteString(".\n")
	}
	fmt.Fprintf(b, "\nHelios Loop's addresses end in @%s.\n", loop.Domain)
	return b.String()
}

func viewerBlock(v *viewer) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "## Who is asking\n\nToday is %s. The school year is %s.\n\n", v.now.Format("Monday, January 2, 2006"), calendar.SchoolYear(v.now))
	p := v.me
	if p == nil {
		fmt.Fprintf(b, "The person signed in is %s, whom the directory does not list. Answer about the school and its apps; nothing about a family is known.\n", v.email)
		return b.String()
	}
	fmt.Fprintf(b, "The person signed in is %s (%s), %s.", p.FullName, p.Email, roleWords(p))
	if p.Pronouns != "" {
		fmt.Fprintf(b, " Pronouns %s.", p.Pronouns)
	}
	if p.IsStudent {
		fmt.Fprintf(b, " %s.", placeWords(v, p))
	}
	if p.IsStaff {
		if p.JobTitle != "" {
			fmt.Fprintf(b, " Job title: %s.", p.JobTitle)
		}
		if p.Department != "" {
			fmt.Fprintf(b, " Department: %s.", p.Department)
		}
		if p.Classroom != "" {
			fmt.Fprintf(b, " Teaches in %s.", p.Classroom)
		}
	}
	b.WriteString("\n")
	for _, key := range v.directory.FamilyKeysOf(p.Email) {
		family := v.directory.Families[key]
		fmt.Fprintf(b, "\nFamily %q:\n", family.Name)
		for _, email := range family.AdultEmails {
			if a := v.directory.Person(email); a != nil {
				fmt.Fprintf(b, "- Parent: %s (%s)\n", a.FullName, a.Email)
			}
		}
		for _, email := range family.KidEmails {
			if k := v.directory.Person(email); k != nil {
				fmt.Fprintf(b, "- Student: %s, %s\n", k.FullName, placeWords(v, k))
			}
		}
	}
	bands := []string{}
	for label, parents := range v.directory.RoomParents {
		if slices.Contains(parents, p.Email) {
			bands = append(bands, label)
		}
	}
	slices.Sort(bands)
	if len(bands) > 0 {
		fmt.Fprintf(b, "\nRoom parent for: %s.\n", strings.Join(bands, ", "))
	}
	lists := v.sources.Lists(p.Email)
	if len(lists) > 0 {
		b.WriteString("\nRoles in the apps (each is a Magic Tag in Helios Who?):\n")
		for _, l := range lists {
			fmt.Fprintf(b, "- %s: %s (%d people)\n", listKind(l.Kind), l.Name, len(l.People))
		}
	}
	tags := v.sources.Tags(p.Email)
	if len(tags) > 0 {
		names := []string{}
		for name, people := range tags {
			names = append(names, fmt.Sprintf("%s (%d)", name, len(people)))
		}
		slices.Sort(names)
		fmt.Fprintf(b, "\nTheir own tags in Helios Who?: %s.\n", strings.Join(names, ", "))
	}
	return b.String()
}

func roleWords(p *who.Person) string {
	roles := []string{}
	if p.IsStaff {
		roles = append(roles, "a staff member")
	}
	if p.IsParent {
		roles = append(roles, "a parent")
	}
	if p.IsStudent {
		roles = append(roles, "a student")
	}
	if len(roles) == 0 {
		return "a member of the community"
	}
	return strings.Join(roles, " and ")
}

func placeWords(v *viewer, p *who.Person) string {
	parts := []string{}
	if p.Grade != "" {
		parts = append(parts, p.Grade)
	}
	if p.Classroom != "" {
		parts = append(parts, "in "+p.Classroom)
	}
	if p.Crew != "" {
		parts = append(parts, "crew "+p.Crew)
	}
	if teachers := v.crewTeachers(p.Classroom, p.Crew); len(teachers) > 0 {
		parts = append(parts, "taught by "+strings.Join(teachers, " and "))
	}
	if len(parts) == 0 {
		return "student"
	}
	return strings.Join(parts, ", ")
}

func listKind(kind string) string {
	switch kind {
	case who.ListParty:
		return "hosts the party"
	case who.ListActivity:
		return "co-chairs"
	case who.ListRoom:
		return "room parent list"
	case who.ListGroup:
		return "manages the Loop group"
	}
	return kind
}

func (v *viewer) starters() []string {
	out := []string{"What's happening at school this week?", "When is the next day off?"}
	if v.me != nil {
		for _, key := range v.directory.FamilyKeysOf(v.me.Email) {
			for _, email := range v.directory.Families[key].KidEmails {
				if k := v.directory.Person(email); k != nil && k.Classroom != "" && k.Email != v.me.Email {
					out = append(out, fmt.Sprintf("Who teaches %s in %s?", firstName(k.FullName), k.Classroom))
					break
				}
			}
			if len(out) > 2 {
				break
			}
		}
	}
	return append(out, "What can I volunteer for?", "Which parties still have tickets?")
}

func firstName(name string) string {
	if words := strings.Fields(name); len(words) > 0 {
		return words[0]
	}
	return name
}
