package db

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type GoogleEvent struct {
	CalendarItem
	Series string
}

type YearEntry struct {
	Key        string
	Title      string
	Start      string
	End        string
	DayType    string
	Classrooms []string
	Marker     string
}

type ShadedDay struct {
	Date    string
	DayType string
}

type YearCalendar struct {
	Document string
	Year     string
	Hash     string
	Entries  []YearEntry
	Shaded   []ShadedDay
}

type calendarPlan struct {
	v        *Vocabulary
	groups   map[string]map[string]string
	sources  []map[string]string
	under    map[string]map[string][]map[string]string
	children map[string][]string
	deleted  map[string]bool
	edits    []Edit
	names    int
}

var underTables = []string{"RULE", "MEMBER", "DOCUMENT_GROUP", "GROUP_SOURCE"}

func newCalendarPlan(rows CalendarRows, v *Vocabulary) *calendarPlan {
	p := &calendarPlan{v: v, groups: map[string]map[string]string{}, under: map[string]map[string][]map[string]string{}, children: map[string][]string{}, deleted: map[string]bool{}}
	for _, g := range rows["GROUP"] {
		p.groups[g["id"]] = g
		if g["parent"] != "" {
			p.children[g["parent"]] = append(p.children[g["parent"]], g["id"])
		}
	}
	for _, t := range underTables {
		p.under[t] = map[string][]map[string]string{}
		for _, row := range rows[t] {
			p.under[t][row["group"]] = append(p.under[t][row["group"]], row)
		}
	}
	p.sources = rows["GROUP_SOURCE"]
	return p
}

func (p *calendarPlan) kind(source map[string]string) string {
	return p.groups[source["group"]]["kind"]
}

func (p *calendarPlan) name() (string, string) {
	p.names++
	n := "n" + strconv.Itoa(p.names)
	return n, "@" + n
}

func (p *calendarPlan) remove(id string) {
	if p.deleted[id] {
		return
	}
	p.deleted[id] = true
	p.edits = append(p.edits, Edit{Delete: id})
}

func (p *calendarPlan) deleteGroup(id string) {
	if p.deleted[id] {
		return
	}
	for _, child := range p.children[id] {
		p.deleteGroup(child)
	}
	for _, t := range underTables {
		for _, row := range p.under[t][id] {
			p.remove(row["id"])
		}
	}
	p.remove(id)
}

func (p *calendarPlan) liveSources(group string) []map[string]string {
	out := []map[string]string{}
	for _, s := range p.under["GROUP_SOURCE"][group] {
		if !p.deleted[s["id"]] {
			out = append(out, s)
		}
	}
	return out
}

func (p *calendarPlan) dropSource(source map[string]string) {
	p.remove(source["id"])
	if len(p.liveSources(source["group"])) == 0 {
		p.deleteGroup(source["group"])
	}
}

func itemCells(it CalendarItem) map[string]any {
	return map[string]any{
		"name": it.Title, "start": it.Start, "end": it.End, "all_day": it.AllDay,
		"location": it.Location, "description": it.Description,
	}
}

func sameCell(old string, want any) bool {
	if b, ok := want.(bool); ok {
		yes, err := cells.YesNo(old, false)
		return err == nil && yes == b
	}
	return strings.TrimSpace(old) == strings.TrimSpace(fmt.Sprint(want))
}

func (p *calendarPlan) setChanged(id string, old map[string]string, want map[string]any) {
	changed := map[string]any{}
	for _, column := range slices.Sorted(maps.Keys(want)) {
		if !sameCell(old[column], want[column]) {
			changed[column] = want[column]
		}
	}
	if len(changed) > 0 {
		p.edits = append(p.edits, Edit{Set: id, Cells: changed})
	}
}

func (p *calendarPlan) insert(table string, row map[string]any) string {
	n, ref := p.name()
	p.edits = append(p.edits, Edit{Insert: table, As: n, Row: row})
	return ref
}

func (p *calendarPlan) newGroup(kind string, row map[string]any) string {
	everyone := p.v.Roles["everyone"]
	row["kind"], row["listed"], row["status"], row["visible_to"], row["members_visible_to"] = kind, true, "open", everyone, everyone
	return p.insert("GROUP", row)
}

func (p *calendarPlan) everyClassroom(classrooms []string) bool {
	for _, c := range p.v.Classrooms {
		if !slices.Contains(classrooms, c.ID) {
			return false
		}
	}
	return true
}

func (p *calendarPlan) rules(group string, classrooms []string, who string) {
	type rule struct{ target, replaceWith string }
	want := []rule{}
	switch {
	case who == "staff":
		want = append(want, rule{p.v.Roles["staff"], ""})
	case p.everyClassroom(classrooms) && who == "parents":
		want = append(want, rule{p.v.Roles["parents"], ""})
	case p.everyClassroom(classrooms):
		want = append(want, rule{p.v.Roles["everyone"], ""})
	default:
		replaceWith := "household"
		if who == "parents" {
			replaceWith = "parents"
		}
		for _, c := range classrooms {
			want = append(want, rule{c, replaceWith})
		}
	}
	orders := store.Order(make([]string, len(want)))
	for i, r := range want {
		row := map[string]any{"group": group, "order": orders[i], "target": r.target}
		if r.replaceWith != "" {
			row["replace_with"] = r.replaceWith
		}
		p.edits = append(p.edits, Edit{Insert: "RULE", Row: row})
	}
}

func (p *calendarPlan) unclassify(group string) {
	for _, row := range p.under["RULE"][group] {
		p.remove(row["id"])
	}
}

func (p *calendarPlan) addDay(date, dayType string, classrooms []string, source map[string]any) {
	day := p.newGroup("day", map[string]any{"name": dayType, "parent": p.v.DayTypes[dayType], "start": date, "all_day": true})
	if !p.everyClassroom(classrooms) {
		orders := store.Order(make([]string, len(classrooms)))
		for i, c := range classrooms {
			p.edits = append(p.edits, Edit{Insert: "RULE", Row: map[string]any{"group": day, "order": orders[i], "target": c}})
		}
	}
	row := maps.Clone(source)
	row["group"], row["name"], row["start"], row["all_day"] = day, dayType, date, true
	p.edits = append(p.edits, Edit{Insert: "GROUP_SOURCE", Row: row})
	for _, part := range DayTemplates[dayType] {
		p.newGroup("day_part", map[string]any{"name": part.Part, "parent": day, "start": date + " " + part.Start, "end": date + " " + part.End})
	}
}

func weekdays(start, end string) []string {
	from, err := time.ParseInLocation(DateLayout, start[:min(len(start), len(DateLayout))], School)
	if err != nil {
		return nil
	}
	to, err := time.ParseInLocation(DateLayout, end[:min(len(end), len(DateLayout))], School)
	if err != nil || to.Before(from) {
		to = from
	}
	out := []string{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			out = append(out, d.Format(DateLayout))
		}
	}
	return out
}

func SchoolYear(date string) string {
	t, err := time.ParseInLocation(DateLayout, date[:min(len(date), len(DateLayout))], School)
	if err != nil {
		return ""
	}
	start := t.Year()
	if t.Month() < time.July {
		start--
	}
	return fmt.Sprintf("%d-%d", start, start+1)
}

func (p *calendarPlan) googleEvents(recurring bool) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, s := range p.sources {
		if s["calendar_event"] != "" && p.kind(s) == "event" && (p.groups[s["group"]]["start"] == "") == recurring {
			out[s["calendar_event"]] = s
		}
	}
	return out
}

func GoogleToClassify(rows CalendarRows, v *Vocabulary, feed []GoogleEvent) []CalendarItem {
	p := newCalendarPlan(rows, v)
	events := p.googleEvents(false)
	out := []CalendarItem{}
	for _, e := range feed {
		if s, ok := events[e.Key]; !ok || s["hash"] != v.InputHash(e.CalendarItem) {
			out = append(out, e.CalendarItem)
		}
	}
	return out
}

func GooglePlan(rows CalendarRows, v *Vocabulary, feed []GoogleEvent, from, to time.Time, classified map[string]Classification) []Edit {
	p := newCalendarPlan(rows, v)
	events := p.googleEvents(false)
	series := p.googleEvents(true)
	days := map[string][]string{}
	for _, s := range p.sources {
		if s["calendar_event"] != "" && p.kind(s) == "day" {
			days[s["calendar_event"]] = append(days[s["calendar_event"]], s["group"])
		}
	}
	reclassified := func(e GoogleEvent) (Classification, bool) {
		c, have := classified[e.Key]
		s, ok := events[e.Key]
		return c, have && (!ok || s["hash"] != v.InputHash(e.CalendarItem))
	}
	seriesCategory := map[string]string{}
	for _, e := range feed {
		if c, ok := reclassified(e); ok && e.Series != "" {
			seriesCategory[e.Series] = c.Category
		}
	}
	seen := map[string]bool{}
	seriesRef := map[string]string{}
	for _, e := range feed {
		seen[e.Key] = true
		parent := ""
		if e.Series != "" {
			seen[e.Series] = true
			if seriesRef[e.Series] == "" {
				category, recategorized := seriesCategory[e.Series]
				if s, ok := series[e.Series]; ok {
					seriesRef[e.Series] = s["group"]
					if recategorized {
						p.setChanged(s["group"], p.groups[s["group"]], map[string]any{"parent": category})
					}
				} else {
					row := map[string]any{"name": e.Title}
					if category != "" {
						row["parent"] = category
					}
					ref := p.newGroup("event", row)
					p.edits = append(p.edits, Edit{Insert: "GROUP_SOURCE", Row: map[string]any{"group": ref, "calendar_event": e.Series, "name": e.Title}})
					seriesRef[e.Series] = ref
				}
			}
			parent = seriesRef[e.Series]
		}
		hash := v.InputHash(e.CalendarItem)
		c, reclassify := reclassified(e)
		want := itemCells(e.CalendarItem)
		switch {
		case parent != "":
			want["parent"] = parent
		case reclassify:
			want["parent"] = c.Category
		}
		source := map[string]any{"calendar_event": e.Key, "hash": hash}
		maps.Copy(source, itemCells(e.CalendarItem))
		s, ok := events[e.Key]
		if !ok {
			if !reclassify {
				continue
			}
			if want["parent"] == "" {
				delete(want, "parent")
			}
			group := p.newGroup("event", want)
			source["group"] = group
			p.edits = append(p.edits, Edit{Insert: "GROUP_SOURCE", Row: source})
			p.rules(group, c.Classrooms, c.Who)
			p.googleDays(e, c, hash)
			continue
		}
		p.setChanged(s["group"], p.groups[s["group"]], want)
		delete(source, "calendar_event")
		if !reclassify {
			delete(source, "hash")
			p.setChanged(s["id"], s, source)
			continue
		}
		p.setChanged(s["id"], s, source)
		p.unclassify(s["group"])
		p.rules(s["group"], c.Classrooms, c.Who)
		for _, day := range days[e.Key] {
			p.deleteGroup(day)
		}
		p.googleDays(e, c, hash)
	}
	inWindow := func(s map[string]string) bool {
		start, err := time.ParseInLocation(DateLayout, s["start"][:min(len(s["start"]), len(DateLayout))], School)
		return err == nil && !start.Before(from) && start.Before(to)
	}
	for key, s := range events {
		if seen[key] || !inWindow(s) {
			continue
		}
		p.dropSource(s)
		for _, day := range days[key] {
			p.deleteGroup(day)
		}
	}
	for key, s := range series {
		if seen[key] {
			continue
		}
		live := slices.ContainsFunc(p.children[s["group"]], func(child string) bool { return !p.deleted[child] })
		if !live {
			p.deleteGroup(s["group"])
		}
	}
	return p.edits
}

func (p *calendarPlan) googleDays(e GoogleEvent, c Classification, hash string) {
	if c.DayType == "" || !e.AllDay {
		return
	}
	for _, d := range weekdays(e.Start, e.End) {
		p.addDay(d, c.DayType, c.Classrooms, map[string]any{"calendar_event": e.Key, "hash": hash})
	}
}

func matchKey(date, title string) string {
	return date[:min(len(date), len(DateLayout))] + "\x00" + strings.ToLower(Collapse(title))
}

func (p *calendarPlan) pdfSourcesOf(year string) []map[string]string {
	out := []map[string]string{}
	for _, s := range p.sources {
		if s["document"] != "" && SchoolYear(s["start"]) == year {
			out = append(out, s)
		}
	}
	return out
}

func (p *calendarPlan) pdfEvents(cal YearCalendar) map[string][]map[string]string {
	pdf := map[string][]map[string]string{}
	for _, s := range p.pdfSourcesOf(cal.Year) {
		if p.kind(s) == "event" {
			key := matchKey(s["start"], s["name"])
			pdf[key] = append(pdf[key], s)
		}
	}
	return pdf
}

func dates(start, end string) (string, string) {
	from := start[:min(len(start), len(DateLayout))]
	to := end[:min(len(end), len(DateLayout))]
	if to < from {
		to = from
	}
	return from, to
}

func (p *calendarPlan) googleOn(e YearEntry) []map[string]string {
	from, to := dates(e.Start, e.End)
	out := []map[string]string{}
	seen := map[string]bool{}
	for _, s := range p.sources {
		g := p.groups[s["group"]]
		if s["calendar_event"] == "" || g["kind"] != "event" || g["start"] == "" || seen[g["id"]] {
			continue
		}
		gFrom, gTo := dates(g["start"], g["end"])
		if gFrom <= to && from <= gTo {
			seen[g["id"]] = true
			out = append(out, g)
		}
	}
	return out
}

func entryItem(e YearEntry) CalendarItem {
	return CalendarItem{Key: e.Key, Title: e.Title, Start: e.Start, End: e.End, AllDay: true}
}

func PDFToClassify(rows CalendarRows, v *Vocabulary, cal YearCalendar, matched map[string][]string) []CalendarItem {
	p := newCalendarPlan(rows, v)
	pdf := p.pdfEvents(cal)
	out := []CalendarItem{}
	for _, e := range cal.Entries {
		if len(pdf[matchKey(e.Start, e.Title)]) == 0 && len(matched[e.Key]) == 0 {
			out = append(out, entryItem(e))
		}
	}
	return out
}

func PDFPlan(rows CalendarRows, v *Vocabulary, cal YearCalendar, classified map[string]Classification, matched map[string][]string) ([]Edit, error) {
	p := newCalendarPlan(rows, v)
	pdf := p.pdfEvents(cal)
	kept := map[string]bool{}
	first, last := "", ""
	for _, e := range cal.Entries {
		switch e.Marker {
		case "first_day":
			first = e.Start
		case "last_day":
			last = e.Start
		}
		item := entryItem(e)
		source := map[string]any{"document": cal.Document, "hash": cal.Hash}
		maps.Copy(source, itemCells(item))
		key := matchKey(e.Start, e.Title)
		if groups := matched[e.Key]; len(groups) > 0 {
			for _, group := range groups {
				i := slices.IndexFunc(pdf[key], func(s map[string]string) bool { return s["group"] == group && !kept[s["id"]] })
				if i >= 0 {
					kept[pdf[key][i]["id"]] = true
					p.setChanged(pdf[key][i]["id"], pdf[key][i], source)
					continue
				}
				row := maps.Clone(source)
				row["group"] = group
				p.edits = append(p.edits, Edit{Insert: "GROUP_SOURCE", Row: row})
			}
			continue
		}
		if i := slices.IndexFunc(pdf[key], func(s map[string]string) bool { return !kept[s["id"]] }); i >= 0 {
			s := pdf[key][i]
			kept[s["id"]] = true
			p.setChanged(s["id"], s, source)
			if len(p.under["GROUP_SOURCE"][s["group"]]) == 1 {
				p.setChanged(s["group"], p.groups[s["group"]], itemCells(item))
			}
			continue
		}
		c, ok := classified[e.Key]
		if !ok {
			return nil, fmt.Errorf("%q on %s was not classified", e.Title, e.Start)
		}
		row := itemCells(item)
		if c.Category != "" {
			row["parent"] = c.Category
		}
		group := p.newGroup("event", row)
		source["group"] = group
		p.edits = append(p.edits, Edit{Insert: "GROUP_SOURCE", Row: source})
		p.rules(group, e.Classrooms, c.Who)
	}
	if first == "" || last == "" {
		return nil, fmt.Errorf("the year calendar has no first or last day of school")
	}
	for _, s := range p.pdfSourcesOf(cal.Year) {
		switch {
		case kept[s["id"]]:
		case p.kind(s) == "day":
			p.deleteGroup(s["group"])
		default:
			p.dropSource(s)
		}
	}
	every := []string{}
	for _, c := range v.Classrooms {
		every = append(every, c.ID)
	}
	for _, d := range weekdays(first, last) {
		types := map[string]string{}
		for _, c := range every {
			types[c] = "Regular"
		}
		for _, s := range cal.Shaded {
			if s.Date == d {
				for _, c := range every {
					types[c] = s.DayType
				}
			}
		}
		for _, e := range cal.Entries {
			if e.DayType != "" && slices.Contains(weekdays(e.Start, e.End), d) {
				for _, c := range e.Classrooms {
					types[c] = e.DayType
				}
			}
		}
		byType := map[string][]string{}
		for _, c := range every {
			byType[types[c]] = append(byType[types[c]], c)
		}
		marker := ""
		switch d {
		case first:
			marker = "first_day"
		case last:
			marker = "last_day"
		}
		for _, t := range slices.Sorted(maps.Keys(byType)) {
			source := map[string]any{"document": cal.Document, "hash": cal.Hash}
			if marker != "" {
				source["marker"] = marker
			}
			p.addDay(d, t, byType[t], source)
		}
	}
	return p.edits, nil
}
