package ask

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/access"
	"heliosian/internal/model"
)

//go:embed prompt.md
var school string

const placeFields = "fullName,email,link,grade.name,classroom.name,classroom.link,classroom.teachers.fullName,crew.name,crew.teachers.fullName"

type document = map[string]any

func systemBlocks(t *turn, listed []document, l *links) ([]anthropic.BetaTextBlockParam, error) {
	words, err := t.lingo()
	if err != nil {
		return nil, err
	}
	viewer, err := t.viewerBlock()
	if err != nil {
		return nil, err
	}
	examples, err := t.linkExamples()
	if err != nil {
		return nil, err
	}
	return []anthropic.BetaTextBlockParam{
		{Text: l.shorten(school + "\n\n" + words), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		{Text: l.shorten(viewer + "\n" + recentBlock(t, listed) + "\n" + examples), CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
	}, nil
}

func s(m map[string]any, name string) string {
	v, _ := m[name].(string)
	return v
}

func list(m map[string]any, name string) []any {
	v, _ := m[name].([]any)
	return v
}

func obj(m map[string]any, name string) map[string]any {
	v, _ := m[name].(map[string]any)
	return v
}

func words(items []any) []string {
	out := []string{}
	for _, item := range items {
		if w, ok := item.(string); ok {
			out = append(out, w)
		}
	}
	return out
}

func (t *turn) recentDocuments() ([]document, error) {
	since := today(t.clock()).AddDate(0, 0, -recentDays).Format(model.DateFormat)
	out, err := t.ask(query{name: "recent", path: collection("documents", params("since", since, "limit", "60")), fields: fields("id,title,date,kind,url,emailList")})
	if err != nil {
		return nil, err
	}
	recent := []document{}
	for _, d := range list(out, "recent") {
		if doc := d.(map[string]any); s(doc, "kind") != model.DocumentKindPortal && len(recent) < recentLimit {
			recent = append(recent, doc)
		}
	}
	return recent, nil
}

func (t *turn) documentsKnown(ids []string) ([]document, error) {
	out := []document{}
	for _, key := range ids {
		got, err := t.ask(query{name: "document", path: one("documents", key, params()), fields: fields("id,title,date,kind,url,emailList")})
		var refusal *access.Refusal
		if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, obj(got, "document"))
	}
	return out, nil
}

func (t *turn) timing(cell string) string {
	day, err := time.ParseInLocation(model.DateFormat, cell, model.Location)
	if err != nil {
		return ""
	}
	switch days := int(today(t.clock()).Sub(day).Hours() / 24); {
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
		return fmt.Sprintf("## Recent documents\n\nNo documents have come in over the last %d days.\n", recentDays)
	}
	return fmt.Sprintf("## Recent documents\n\nThe newest documents of the last %d days, newest first, each with its id for read_document:\n", recentDays) + documentLines(t, recent)
}

func arrivals(t *turn, fresh []document) string {
	return "New documents have come in since this conversation began, newest first, each with its id for read_document:\n" + documentLines(t, fresh) +
		"\nA new document can change what the calendar or the other apps say; read one that bears on what is being asked."
}

func documentLines(t *turn, docs []document) string {
	b := &strings.Builder{}
	for _, d := range docs {
		date := s(d, "date")
		if day, err := time.ParseInLocation(model.DateFormat, date, model.Location); err == nil {
			date = day.Format("Monday, January 2, 2006")
		}
		fmt.Fprintf(b, "- %s, %s: %s (id %s", date, t.timing(s(d, "date")), s(d, "title"), s(d, "id"))
		if url := s(d, "url"); url != "" {
			b.WriteString(", " + url)
		}
		if group := s(d, "emailList"); group != "" {
			b.WriteString(", mail to the email list " + group)
		}
		b.WriteString(")\n")
	}
	return b.String()
}

func (t *turn) lingo() (string, error) {
	out, err := t.ask(
		query{name: "grades", path: collection("grades", params()), fields: fields("name,band")},
		query{name: "classrooms", path: collection("classrooms", params("include", "teachers,crews")), fields: fields("name,band,grades,link,teachers.fullName,crews.name")},
		query{name: "departments", path: collection("departments", params()), fields: fields("name")},
		query{name: "tags", path: collection("calendar-tags", params()), fields: fields("id,name,description,builtIn")},
		query{name: "dayTypes", path: collection("day-types", params()), fields: fields("name,blocks.name,blocks.start,blocks.end")},
		query{name: "settings", path: collection("when-settings", params()), fields: fields("years.label,years.firstDay,years.lastDay")},
		query{name: "celebrations", path: collection("celebrations", params()), fields: fields("title,start,current")},
	)
	if err != nil {
		return "", err
	}
	b := &strings.Builder{}
	b.WriteString("## The school as the data has it\n\nGrades and their bands:\n")
	for _, g := range list(out, "grades") {
		fmt.Fprintf(b, "- %s: %s\n", s(g.(map[string]any), "name"), s(g.(map[string]any), "band"))
	}
	bands := []string{}
	bandGrades := map[string][]string{}
	bandClassrooms := map[string][]string{}
	for _, item := range list(out, "classrooms") {
		c := item.(map[string]any)
		band := s(c, "band")
		if _, seen := bandClassrooms[band]; !seen {
			bands = append(bands, band)
		}
		for _, g := range words(list(c, "grades")) {
			if !slices.Contains(bandGrades[band], g) {
				bandGrades[band] = append(bandGrades[band], g)
			}
		}
		bandClassrooms[band] = append(bandClassrooms[band], s(c, "name"))
	}
	b.WriteString("\nBands (the grades of its students; its classrooms):\n")
	for _, band := range bands {
		fmt.Fprintf(b, "- %s (%s; %s)\n", band, strings.Join(bandGrades[band], ", "), strings.Join(bandClassrooms[band], ", "))
	}
	b.WriteString("\nClassrooms (its link; band; the grades of its students; its teachers; its crews):\n")
	for _, item := range list(out, "classrooms") {
		c := item.(map[string]any)
		line := fmt.Sprintf("- %s (%s; %s; %s", s(c, "name"), s(c, "link"), s(c, "band"), strings.Join(words(list(c, "grades")), ", "))
		if teachers := words(list(c, "teachers")); len(teachers) > 0 {
			line += "; teachers " + strings.Join(teachers, ", ")
		}
		if crews := words(list(c, "crews")); len(crews) > 0 {
			line += "; crews " + strings.Join(crews, ", ")
		}
		b.WriteString(line + ")\n")
	}
	departments := []string{}
	for _, d := range list(out, "departments") {
		departments = append(departments, s(d.(map[string]any), "name"))
	}
	b.WriteString("\nStaff departments: " + strings.Join(departments, "; ") + "\n")
	b.WriteString("\nCalendar categories (what each files):\n")
	for _, item := range list(out, "tags") {
		tag := item.(map[string]any)
		if tag["builtIn"] == true && s(tag, "id") != model.TagCelebrate && s(tag, "id") != model.TagHCA {
			continue
		}
		fmt.Fprintf(b, "- %s: %s\n", s(tag, "name"), s(tag, "description"))
	}
	b.WriteString("\nDay types (the school day's hours):\n")
	for _, item := range list(out, "dayTypes") {
		d := item.(map[string]any)
		parts := []string{}
		for _, block := range list(d, "blocks") {
			bl := block.(map[string]any)
			parts = append(parts, fmt.Sprintf("%s %s-%s", strings.ToLower(s(bl, "name")), s(bl, "start"), s(bl, "end")))
		}
		if len(parts) == 0 {
			parts = []string{"no school"}
		}
		fmt.Fprintf(b, "- %s: %s\n", s(d, "name"), strings.Join(parts, ", "))
	}
	b.WriteString("\nSchool years the calendar holds:\n")
	for _, item := range list(first(out["settings"]), "years") {
		y := item.(map[string]any)
		fmt.Fprintf(b, "- %s: first day %s, last day %s\n", s(y, "label"), s(y, "firstDay"), s(y, "lastDay"))
	}
	for _, item := range list(out, "celebrations") {
		c := item.(map[string]any)
		if c["current"] != true {
			continue
		}
		fmt.Fprintf(b, "\nThe current celebration on Helios Celebrate is %s", s(c, "title"))
		if start := s(c, "start"); start != "" {
			fmt.Fprintf(b, " on %s", start)
		}
		b.WriteString(".\n")
	}
	fmt.Fprintf(b, "\nHelios Loop's addresses end in @%s.\n", model.ListDomain)
	return b.String(), nil
}

func (t *turn) viewer() (map[string]any, error) {
	email := t.reg.Actor(t.r).Email
	includes := "grade,classroom.teachers,crew.teachers,room-parent-for,families.adults,families.kids.grade,families.kids.classroom.teachers,families.kids.crew.teachers"
	spec := placeFields + ",heroPhotoUrl,isStaff,isParent,isStudent,pronouns,jobTitle,department,room-parent-for.name,families.name,families.link,families.adults.fullName,families.adults.email," + under("families.kids", placeFields)
	out, err := t.ask(query{name: "me", path: one("people", email, params("include", includes)), fields: fields(spec)})
	var refusal *access.Refusal
	if errors.As(err, &refusal) && refusal.Status == http.StatusNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return obj(out, "me"), nil
}

func (t *turn) viewerBlock() (string, error) {
	now := t.clock().In(model.Location)
	b := &strings.Builder{}
	fmt.Fprintf(b, "## Who is asking\n\nToday is %s. The school year is %s.\n\n", now.Format("Monday, January 2, 2006"), model.SchoolYear(now))
	p, err := t.viewer()
	if err != nil {
		return "", err
	}
	if p == nil {
		fmt.Fprintf(b, "The person signed in is %s, whom the directory does not list. Answer about the school and its apps; nothing about a family is known.\n", t.reg.Actor(t.r).Email)
		return b.String(), nil
	}
	fmt.Fprintf(b, "The person signed in is %s (%s), %s.", s(p, "fullName"), s(p, "email"), roleWords(p))
	if pronouns := s(p, "pronouns"); pronouns != "" {
		fmt.Fprintf(b, " Pronouns %s.", pronouns)
	}
	if p["isStudent"] == true {
		fmt.Fprintf(b, " %s.", placeWords(p))
	}
	if p["isStaff"] == true {
		if title := s(p, "jobTitle"); title != "" {
			fmt.Fprintf(b, " Job title: %s.", title)
		}
		if department := s(p, "department"); department != "" {
			fmt.Fprintf(b, " Department: %s.", department)
		}
		if classroom := s(obj(p, "classroom"), "name"); classroom != "" {
			fmt.Fprintf(b, " Teaches in %s.", classroom)
		}
	}
	b.WriteString("\n")
	for _, item := range list(p, "families") {
		f := item.(map[string]any)
		fmt.Fprintf(b, "\nFamily %q:\n", s(f, "name"))
		for _, a := range list(f, "adults") {
			fmt.Fprintf(b, "- Parent: %s (%s)\n", s(a.(map[string]any), "fullName"), s(a.(map[string]any), "email"))
		}
		for _, k := range list(f, "kids") {
			fmt.Fprintf(b, "- Student: %s, %s\n", s(k.(map[string]any), "fullName"), placeWords(k.(map[string]any)))
		}
	}
	if grades := words(list(p, "room-parent-for")); len(grades) > 0 {
		fmt.Fprintf(b, "\nRoom parent for: %s.\n", strings.Join(grades, ", "))
	}
	out, err := t.ask(
		query{name: "magicTags", path: collection("magic-tags", params("include", "people")), fields: fields("name,kind,archived,people.fullName")},
		query{name: "tags", path: collection("tags", params("mine", "true", "include", "people")), fields: fields("name,link,people.fullName")},
	)
	if err != nil {
		return "", err
	}
	roles := []string{}
	for _, item := range list(out, "magicTags") {
		l := item.(map[string]any)
		if l["archived"] != true {
			roles = append(roles, fmt.Sprintf("- %s: %s (%d people)\n", listKind(s(l, "kind")), s(l, "name"), len(list(l, "people"))))
		}
	}
	if len(roles) > 0 {
		b.WriteString("\nRoles in the apps (each is a Magic Tag in Helios Who?):\n" + strings.Join(roles, ""))
	}
	tags := []string{}
	for _, item := range list(out, "tags") {
		tag := item.(map[string]any)
		tags = append(tags, fmt.Sprintf("%s (%d, %s)", s(tag, "name"), len(list(tag, "people")), s(tag, "link")))
	}
	if len(tags) > 0 {
		fmt.Fprintf(b, "\nTheir own tags in Helios Who?: %s.\n", strings.Join(tags, ", "))
	}
	return b.String(), nil
}

func roleWords(p map[string]any) string {
	roles := []string{}
	if p["isStaff"] == true {
		roles = append(roles, "a staff member")
	}
	if p["isParent"] == true {
		roles = append(roles, "a parent")
	}
	if p["isStudent"] == true {
		roles = append(roles, "a student")
	}
	if len(roles) == 0 {
		return "a member of the community"
	}
	return strings.Join(roles, " and ")
}

func placeWords(p map[string]any) string {
	parts := []string{}
	if grade := s(p, "grade"); grade != "" {
		parts = append(parts, grade)
	}
	classroom := obj(p, "classroom")
	if name := s(classroom, "name"); name != "" {
		parts = append(parts, "in "+name)
	}
	crew := obj(p, "crew")
	if name := s(crew, "name"); name != "" {
		parts = append(parts, "crew "+name)
	}
	teachers := words(list(crew, "teachers"))
	if len(teachers) == 0 {
		teachers = words(list(classroom, "teachers"))
	}
	if len(teachers) > 0 {
		parts = append(parts, "taught by "+strings.Join(teachers, " and "))
	}
	if len(parts) == 0 {
		return "student"
	}
	return strings.Join(parts, ", ")
}

func listKind(kind string) string {
	switch kind {
	case model.MagicTagParty:
		return "hosts the party"
	case model.MagicTagActivity:
		return "co-chairs"
	case model.MagicTagRoom:
		return "room parent list"
	case model.MagicTagGroup:
		return "manages the Loop email list"
	}
	return kind
}

func (t *turn) exampleLinks() ([]string, error) {
	out := []string{}
	p, err := t.viewer()
	if err != nil {
		return nil, err
	}
	if p != nil {
		if families := list(p, "families"); len(families) > 0 {
			family := families[0].(map[string]any)
			out = append(out, s(family, "link"))
			if kids := list(family, "kids"); len(kids) > 0 {
				kid := kids[0].(map[string]any)
				out = append(out, s(kid, "link"))
				if classroom := s(obj(kid, "classroom"), "link"); classroom != "" {
					out = append(out, classroom)
				}
			}
		}
	}
	got, err := t.ask(
		query{name: "event", path: collection("events", params("from", today(t.clock()).Format(model.DateFormat), "app", "when", "limit", "1")), fields: fields("link")},
		query{name: "activity", path: collection("activities", params("year", "current", "limit", "1")), fields: fields("link")},
		query{name: "party", path: collection("parties", params("past", "false", "limit", "1")), fields: fields("link")},
		query{name: "group", path: collection("email-lists", params("limit", "1")), fields: fields("link")},
	)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"event", "activity", "party", "group"} {
		if link := s(first(got[name]), "link"); link != "" {
			out = append(out, link)
		}
	}
	return out, nil
}

func (t *turn) linkExamples() (string, error) {
	addresses, err := t.exampleLinks()
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

func (t *turn) starters() ([]string, error) {
	out := []string{"What's happening at school this week?", "When is the next day off?"}
	p, err := t.viewer()
	if err != nil {
		return nil, err
	}
	for _, family := range list(p, "families") {
		for _, item := range list(family.(map[string]any), "kids") {
			kid := item.(map[string]any)
			if classroom := s(obj(kid, "classroom"), "name"); classroom != "" && len(out) == 2 {
				out = append(out, fmt.Sprintf("Who teaches %s in %s?", model.FirstWord(s(kid, "fullName")), classroom))
			}
		}
	}
	return append(out, "What can I volunteer for?", "Which parties still have tickets?"), nil
}
