package ask

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

//go:embed prompt.md
var school string

const listDomain = "loop.heliosian.com"

type document struct {
	ID        string
	Name      string
	Published string
	Lists     []string
}

type person struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Href      string   `json:"href"`
	Roles     []string `json:"roles"`
	JobTitle  string   `json:"job_title"`
	Grade     string   `json:"grade"`
	Classroom string   `json:"classroom"`
	Crew      string   `json:"crew"`
	Pronouns  string   `json:"pronouns"`
	Email     string   `json:"email"`
}

type family struct {
	Name    string   `json:"name"`
	Href    string   `json:"href"`
	Members []person `json:"members"`
}

type place struct {
	Name string `json:"name"`
	Href string `json:"href"`
	Kind string `json:"kind"`
	Lead bool   `json:"lead"`
}

type viewer struct {
	Person   person   `json:"person"`
	Emails   []string `json:"emails"`
	Families []family `json:"families"`
	Groups   []place  `json:"groups"`
	Manages  []place  `json:"manages"`
	AdminOf  []string `json:"adminOf"`
}

func (p person) is(role string) bool {
	return slices.Contains(p.Roles, role)
}

func (t *turn) viewer() (*viewer, error) {
	if t.env.Viewer == "" {
		return nil, nil
	}
	answer, err := t.sources.Tools.Run(t.ctx, t.m, t.env.Viewer, "helios_whoami", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}
	for href, id := range answer.Links {
		t.found.note(href, id)
	}
	v := &viewer{}
	if err := json.Unmarshal([]byte(answer.Text), v); err != nil {
		return nil, err
	}
	return v, nil
}

type schoolData struct {
	groups       map[string]store.Row
	classrooms   []store.Row
	teachers     map[string][]string
	studentGrade map[string][]string
}

func (s schoolData) named(kind, name string) store.Row {
	for _, g := range s.groups {
		if g["kind"] == kind && g["name"] == name {
			return g
		}
	}
	return nil
}

func (t *turn) school() (schoolData, error) {
	s := schoolData{groups: map[string]store.Row{}, teachers: map[string][]string{}, studentGrade: map[string][]string{}}
	groups, _, err := t.rows(`(from GROUP (where (in kind "band" "grade" "classroom" "crew" "department")) (order name asc))`)
	if err != nil {
		return s, err
	}
	for _, g := range groups {
		s.groups[g["id"]] = g
		if g["kind"] == "classroom" {
			s.classrooms = append(s.classrooms, g)
		}
	}
	staff, _, err := t.rows(`(from PERSON @p (where (or (not (blank classroom)) (not (blank crew))) (exists EFFECTIVE_MEMBER (= person @p) (= group.slug "staff") (= group.kind "group"))) (order name_sort asc))`)
	if err != nil {
		return s, err
	}
	for _, p := range staff {
		for _, col := range []string{"classroom", "crew"} {
			if p[col] != "" {
				s.teachers[p[col]] = append(s.teachers[p[col]], p["name_show"])
			}
		}
	}
	students, _, err := t.rows(`(from PERSON @p (where (not (blank classroom)) (not (blank grade)) (exists EFFECTIVE_MEMBER (= person @p) (= group.slug "students") (= group.kind "group"))))`)
	if err != nil {
		return s, err
	}
	for _, p := range students {
		if !slices.Contains(s.studentGrade[p["classroom"]], p["grade"]) {
			s.studentGrade[p["classroom"]] = append(s.studentGrade[p["classroom"]], p["grade"])
		}
	}
	for id := range s.studentGrade {
		slices.Sort(s.studentGrade[id])
	}
	return s, nil
}

func promptTexts(t *turn, s schoolData, v *viewer, listed []document) ([]string, error) {
	words, err := t.lingo(s)
	if err != nil {
		return nil, err
	}
	examples, err := t.linkExamples(s, v)
	if err != nil {
		return nil, err
	}
	return []string{school + "\n\n" + words, t.viewerBlock(s, v) + "\n" + recentBlock(t, listed) + "\n" + examples}, nil
}

func systemBlocks(texts []string, l *links) []anthropic.BetaTextBlockParam {
	out := []anthropic.BetaTextBlockParam{}
	for _, text := range texts {
		out = append(out, anthropic.BetaTextBlockParam{Text: l.shorten(text), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()})
	}
	return out
}

func dayWords(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}

func (t *turn) documents(rows []store.Row) ([]document, error) {
	out := []document{}
	if len(rows) == 0 {
		return out, nil
	}
	ids := []string{}
	for _, r := range rows {
		ids = append(ids, fmt.Sprintf("%q", r["id"]))
	}
	links, res, err := t.rows(`(from DOCUMENT_GROUP (where (in document %s) (= relation "sent_to") group.mail) (include group))`, strings.Join(ids, " "))
	if err != nil {
		return nil, err
	}
	lists := map[string][]string{}
	for _, l := range links {
		lists[l["document"]] = append(lists[l["document"]], res.Resources["GROUP"][l["group"]]["name"])
	}
	for _, r := range rows {
		out = append(out, document{ID: r["id"], Name: r["name"], Published: r["published"], Lists: lists[r["id"]]})
	}
	return out, nil
}

func (t *turn) recentDocuments() ([]document, error) {
	since := t.today().AddDate(0, 0, -recentDays).Format(time.DateOnly)
	rows, _, err := t.rows(`(from DOCUMENT (where (= kind "mail") (>= published %q)) (order published desc) (limit %d))`, since, recentLimit)
	if err != nil {
		return nil, err
	}
	return t.documents(rows)
}

func (t *turn) documentsKnown(ids []string) ([]document, error) {
	quoted := []string{}
	for _, id := range ids {
		if table, ok := db.TableOf(id); ok && table == "DOCUMENT" {
			quoted = append(quoted, fmt.Sprintf("%q", id))
		}
	}
	if len(quoted) == 0 {
		return []document{}, nil
	}
	rows, _, err := t.rows(`(from DOCUMENT (where (in id %s)) (order published desc))`, strings.Join(quoted, " "))
	if err != nil {
		return nil, err
	}
	return t.documents(rows)
}

func (t *turn) timing(moment string) string {
	day, err := time.ParseInLocation(time.DateOnly, moment[:min(len(moment), len(time.DateOnly))], t.env.Now.Location())
	if err != nil {
		return ""
	}
	switch days := int(t.today().Sub(day).Hours() / 24); {
	case days == 0:
		return "today"
	case days > 0:
		return fmt.Sprintf("past (%d days ago)", days)
	case days == -1:
		return "tomorrow"
	default:
		return fmt.Sprintf("in %d days", -days)
	}
}

func recentBlock(t *turn, recent []document) string {
	if len(recent) == 0 {
		return fmt.Sprintf("## Recent documents\n\nNo mail has come in over the last %d days.\n", recentDays)
	}
	return fmt.Sprintf("## Recent documents\n\nThe newest mail of the last %d days, newest first, each with its id for helios_read_document:\n", recentDays) + documentLines(t, recent)
}

func arrivals(t *turn, fresh []document) string {
	return "New mail has come in since this conversation began, newest first, each with its id for helios_read_document:\n" + documentLines(t, fresh) +
		"\nNew mail can change what the calendar or the other apps say; read one that bears on what is being asked."
}

func newDay(t *turn, from string) string {
	return fmt.Sprintf("Today is now %s, no longer %s, the day the prompt above was written; reckon every date from today. The school year is %s.", dayWords(t.env.Now), from, db.SchoolYear(t.env.Now.Format(time.DateOnly)))
}

func documentLines(t *turn, docs []document) string {
	b := &strings.Builder{}
	for _, d := range docs {
		date := d.Published
		if day, err := time.Parse(time.DateOnly, d.Published[:min(len(d.Published), len(time.DateOnly))]); err == nil {
			date = day.Format("Monday, January 2, 2006")
		}
		fmt.Fprintf(b, "- %s, %s: %s (id %s", date, t.timing(d.Published), d.Name, d.ID)
		for _, list := range d.Lists {
			b.WriteString(", mail to the email list " + list)
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func (t *turn) lingo(s schoolData) (string, error) {
	b := &strings.Builder{}
	b.WriteString("## The school as the data has it\n\nGrades and their bands:\n")
	bands := []store.Row{}
	for _, g := range sortedGroups(s.groups) {
		switch g["kind"] {
		case "grade":
			fmt.Fprintf(b, "- %s: %s\n", g["name"], s.groups[g["parent"]]["name"])
		case "band":
			bands = append(bands, g)
		}
	}
	b.WriteString("\nBands (its grades; its classrooms):\n")
	for _, band := range bands {
		grades, rooms := []string{}, []string{}
		for _, g := range sortedGroups(s.groups) {
			if g["parent"] != band["id"] {
				continue
			}
			switch g["kind"] {
			case "grade":
				grades = append(grades, g["name"])
			case "classroom":
				rooms = append(rooms, g["name"])
			}
		}
		fmt.Fprintf(b, "- %s (%s; %s)\n", band["name"], strings.Join(grades, ", "), strings.Join(rooms, ", "))
	}
	b.WriteString("\nClassrooms (its link; band; the grades of its students; its teachers; its crews and their teachers):\n")
	for _, room := range s.classrooms {
		line := fmt.Sprintf("- %s (%s; %s; %s", room["name"], t.href("GROUP", room), s.groups[room["parent"]]["name"], strings.Join(s.studentGrade[room["id"]], ", "))
		if teachers := s.teachers[room["id"]]; len(teachers) > 0 {
			line += "; teachers " + strings.Join(teachers, ", ")
		}
		crews := []string{}
		for _, g := range sortedGroups(s.groups) {
			if g["kind"] == "crew" && g["parent"] == room["id"] {
				crew := g["name"]
				if teachers := s.teachers[g["id"]]; len(teachers) > 0 {
					crew += " (" + strings.Join(teachers, ", ") + ")"
				}
				crews = append(crews, crew)
			}
		}
		if len(crews) > 0 {
			line += "; crews " + strings.Join(crews, ", ")
		}
		b.WriteString(line + ")\n")
	}
	departments := []string{}
	for _, g := range sortedGroups(s.groups) {
		if g["kind"] == "department" {
			departments = append(departments, g["name"])
		}
	}
	b.WriteString("\nStaff departments: " + strings.Join(departments, "; ") + "\n")
	categories, _, err := t.rows(`(from GROUP @c (where (= kind "category") (blank parent) (not (exists GROUP @t (= parent @c) (= kind "category") (exists GROUP (= parent @t) (= kind "day"))))) (order order asc))`)
	if err != nil {
		return "", err
	}
	b.WriteString("\nCalendar categories (what each files):\n")
	for _, c := range categories {
		fmt.Fprintf(b, "- %s: %s\n", c["name"], c["description"])
	}
	dayTypes, _, err := t.rows(`(from GROUP @c (where (= kind "category") (exists GROUP (= parent @c) (= kind "day"))) (order order asc))`)
	if err != nil {
		return "", err
	}
	b.WriteString("\nDay types (helios_days gives a date's plan and hours):\n")
	for _, d := range dayTypes {
		fmt.Fprintf(b, "- %s: %s\n", d["name"], d["description"])
	}
	markers, res, err := t.rows(`(from GROUP_SOURCE (where (not (blank marker))) (include group))`)
	if err != nil {
		return "", err
	}
	years := map[string][2]string{}
	for _, m := range markers {
		start := res.Resources["GROUP"][m["group"]]["start"]
		year := db.SchoolYear(start)
		bounds := years[year]
		if m["marker"] == "first_day" {
			bounds[0] = start
		} else {
			bounds[1] = start
		}
		years[year] = bounds
	}
	if len(years) > 0 {
		b.WriteString("\nSchool years the calendar holds:\n")
		for _, year := range slices.Sorted(maps.Keys(years)) {
			fmt.Fprintf(b, "- %s: first day %s, last day %s\n", year, years[year][0], years[year][1])
		}
	}
	settings, _, err := t.rows(`(from SETTING (where (= app "celebrate") (= key "Current")))`)
	if err != nil {
		return "", err
	}
	for _, setting := range settings {
		if table, ok := db.TableOf(setting["value"]); !ok || table != "GROUP" {
			continue
		}
		current, _, err := t.rows(`(from GROUP (where (= id %q)))`, setting["value"])
		if err != nil {
			return "", err
		}
		for _, c := range current {
			fmt.Fprintf(b, "\nThe current celebration on Helios Celebrate is %s.\n", c["name"])
		}
	}
	fmt.Fprintf(b, "\nHelios Loop's addresses end in @%s.\n", listDomain)
	return b.String(), nil
}

func sortedGroups(groups map[string]store.Row) []store.Row {
	out := []store.Row{}
	for _, g := range groups {
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b store.Row) int {
		if c := strings.Compare(a["slug"], b["slug"]); a["kind"] == "grade" && b["kind"] == "grade" && c != 0 {
			return c
		}
		return strings.Compare(a["name"], b["name"])
	})
	return out
}

func roleWords(p person) string {
	roles := []string{}
	for _, r := range []struct{ role, words string }{{"staff", "a staff member"}, {"parent", "a parent"}, {"student", "a student"}} {
		if p.is(r.role) {
			roles = append(roles, r.words)
		}
	}
	if len(roles) == 0 {
		return "a member of the community"
	}
	return strings.Join(roles, " and ")
}

func placeWords(s schoolData, p person) string {
	parts := []string{}
	if p.Grade != "" {
		parts = append(parts, "Grade "+p.Grade)
	}
	if p.Classroom != "" {
		parts = append(parts, "in "+p.Classroom)
	}
	if p.Crew != "" {
		parts = append(parts, "crew "+p.Crew)
	}
	teachers := []string{}
	if crew := s.named("crew", p.Crew); crew != nil {
		teachers = s.teachers[crew["id"]]
	}
	if room := s.named("classroom", p.Classroom); len(teachers) == 0 && room != nil {
		teachers = s.teachers[room["id"]]
	}
	if len(teachers) > 0 {
		parts = append(parts, "taught by "+strings.Join(teachers, " and "))
	}
	if len(parts) == 0 {
		return "student"
	}
	return strings.Join(parts, ", ")
}

func (t *turn) viewerBlock(s schoolData, v *viewer) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "## Who is asking\n\nToday is %s. The school year is %s.\n\n", dayWords(t.env.Now), db.SchoolYear(t.env.Now.Format(time.DateOnly)))
	if v == nil {
		fmt.Fprintf(b, "The person signed in is %s, whom the directory does not list. Answer about the school and its apps; nothing about a family is known.\n", t.email)
		return b.String()
	}
	p := v.Person
	fmt.Fprintf(b, "The person signed in is %s (%s), %s.", p.Name, t.email, roleWords(p))
	if p.Pronouns != "" {
		fmt.Fprintf(b, " Pronouns %s.", p.Pronouns)
	}
	if p.is("student") {
		fmt.Fprintf(b, " %s.", placeWords(s, p))
	}
	if p.is("staff") {
		if p.JobTitle != "" {
			fmt.Fprintf(b, " Job title: %s.", p.JobTitle)
		}
		if p.Classroom != "" {
			fmt.Fprintf(b, " Teaches in %s.", p.Classroom)
		}
	}
	b.WriteString("\n")
	for _, f := range v.Families {
		fmt.Fprintf(b, "\nFamily %q (%s):\n", f.Name, f.Href)
		for _, m := range f.Members {
			if m.is("student") {
				fmt.Fprintf(b, "- Student: %s, %s\n", m.Name, placeWords(s, m))
				continue
			}
			fmt.Fprintf(b, "- Parent: %s (%s)\n", m.Name, m.Email)
		}
	}
	roles := []string{}
	for _, g := range v.Groups {
		switch g.Kind {
		case "classroom", "crew", "grade", "band", "department":
			continue
		}
		line := fmt.Sprintf("- in %s (%s", g.Name, g.Kind)
		if g.Lead {
			line += ", a lead"
		}
		roles = append(roles, line+")\n")
	}
	for _, g := range v.Manages {
		roles = append(roles, fmt.Sprintf("- manages %s (%s)\n", g.Name, g.Kind))
	}
	if len(roles) > 0 {
		b.WriteString("\nTheir groups and roles in the apps:\n" + strings.Join(roles, ""))
	}
	if len(v.AdminOf) > 0 {
		fmt.Fprintf(b, "\nAn admin of: %s.\n", strings.Join(v.AdminOf, ", "))
	}
	return b.String()
}

func (t *turn) exampleLinks(s schoolData, v *viewer) ([]string, error) {
	out := []string{}
	if v != nil && len(v.Families) > 0 {
		f := v.Families[0]
		out = append(out, f.Href)
		for _, m := range f.Members {
			if !m.is("student") {
				continue
			}
			out = append(out, m.Href)
			if room := s.named("classroom", m.Classroom); room != nil {
				out = append(out, t.href("GROUP", room))
			}
			break
		}
	}
	today := t.today().Format(time.DateOnly)
	for _, src := range []string{
		`(from GROUP (where (= kind "event") (>= start %q)) (order start asc) (limit 1))`,
		`(from GROUP (where (= kind "activity") (>= start %q)) (order start asc) (limit 1))`,
		`(from GROUP (where (= kind "party") (>= start %q)) (order start asc) (limit 1))`,
	} {
		found, _, err := t.rows(src, today)
		if err != nil {
			return nil, err
		}
		for _, g := range found {
			out = append(out, t.href("GROUP", g))
		}
	}
	lists, _, err := t.rows(`(from GROUP (where mail) (order name asc) (limit 1))`)
	if err != nil {
		return nil, err
	}
	for _, g := range lists {
		out = append(out, t.href("GROUP", g))
	}
	return out, nil
}

func (t *turn) linkExamples(s schoolData, v *viewer) (string, error) {
	addresses, err := t.exampleLinks(s, v)
	if err != nil {
		return "", err
	}
	b := &strings.Builder{}
	b.WriteString("## How links look\n\nThe page draws these links, from this person's own data, as chips:\n")
	for _, address := range addresses {
		card, ok := t.linkCard(address)
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
	return b.String(), nil
}

func starters(v *viewer) []string {
	out := []string{"What's happening at school this week?", "When is the next day off?"}
	if v != nil {
		for _, f := range v.Families {
			for _, m := range f.Members {
				if m.is("student") && m.Classroom != "" && len(out) == 2 {
					out = append(out, fmt.Sprintf("Who teaches %s in %s?", strings.Fields(m.Name)[0], m.Classroom))
				}
			}
		}
	}
	return append(out, "What can I volunteer for?", "Which parties still have tickets?")
}
