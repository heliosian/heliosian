package db

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/claude"
)

const (
	SchoolCalendarID   = "heliosns.org_cidjj9plktli1gdm2hrkj7gqks@group.calendar.google.com"
	SchoolCalendarPage = "https://www.heliosschool.org/school-calendar"
	CalendarModel      = "claude-fable-5-1"
	DateLayout         = "2006-01-02"
	MomentLayout       = "2006-01-02 15:04"
	classifyBatch      = 10
	noDayType          = "None"
)

var School = func() *time.Location {
	l, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic("db: school time zone: " + err.Error())
	}
	return l
}()

type CalendarRows map[string][]map[string]string

type Classroom struct {
	ID     string
	Name   string
	Band   string
	Grades []string
	Crews  []string
}

type Category struct {
	ID          string
	Title       string
	Description string
}

type Vocabulary struct {
	Classrooms []Classroom
	Roles      map[string]string
	Events     []Category
	DayTypes   map[string]string
	Parts      map[string]string
}

func effectiveCell(p map[string]string, column string) string {
	if v := p[column]; v != "" {
		return v
	}
	return p["vc_"+column]
}

func NewVocabulary(rows CalendarRows) (*Vocabulary, error) {
	v := &Vocabulary{Roles: map[string]string{}, DayTypes: map[string]string{}, Parts: map[string]string{}}
	for _, c := range rows["CATEGORY"] {
		switch c["scope"] {
		case "event":
			v.Events = append(v.Events, Category{ID: c["id"], Title: c["title"], Description: c["description"]})
		case "day_type":
			v.DayTypes[c["title"]] = c["id"]
		case "day_part":
			v.Parts[c["title"]] = c["id"]
		}
	}
	slices.SortStableFunc(v.Events, func(a, b Category) int { return strings.Compare(a.Title, b.Title) })
	for name := range DayTemplates {
		if v.DayTypes[name] == "" {
			return nil, fmt.Errorf("no day_type category %s for its template", name)
		}
		for _, p := range DayTemplates[name] {
			if v.Parts[p.Part] == "" {
				return nil, fmt.Errorf("no day_part category %s for the %s template", p.Part, name)
			}
		}
	}
	if len(v.Events) == 0 {
		return nil, fmt.Errorf("no event categories to classify into")
	}
	groups := map[string]map[string]string{}
	for _, g := range rows["GROUP"] {
		groups[g["id"]] = g
	}
	byClassroom := map[string]*Classroom{}
	for _, g := range rows["GROUP"] {
		switch g["kind"] {
		case "classroom":
			byClassroom[g["id"]] = &Classroom{ID: g["id"], Name: g["title"]}
		case "group":
			if g["slug"] != "" {
				v.Roles[g["slug"]] = g["id"]
			}
		}
	}
	for _, slug := range []string{"everyone", "parents", "staff"} {
		if v.Roles[slug] == "" {
			return nil, fmt.Errorf("no role group %s", slug)
		}
	}
	for _, g := range rows["GROUP"] {
		if c, ok := byClassroom[g["parent"]]; ok && g["kind"] == "crew" && !slices.Contains(c.Crews, g["title"]) {
			c.Crews = append(c.Crews, g["title"])
		}
	}
	gradeGroups := map[string]map[string]string{}
	for _, g := range rows["GROUP"] {
		if g["kind"] == "grade" {
			gradeGroups[strings.ToLower(g["slug"])] = g
		}
	}
	for _, p := range rows["PERSON"] {
		c, ok := byClassroom[effectiveCell(p, "classroom")]
		grade := effectiveCell(p, "grade")
		if !ok || grade == "" || p["deactivated"] != "" || slices.Contains(c.Grades, grade) {
			continue
		}
		c.Grades = append(c.Grades, grade)
		if g, ok := gradeGroups["grade-"+strings.ToLower(grade)]; ok && c.Band == "" {
			if band, ok := groups[g["parent"]]; ok {
				c.Band = band["title"]
			}
		}
	}
	for _, c := range byClassroom {
		slices.SortFunc(c.Grades, func(a, b string) int { return cmp.Compare(slices.Index(grades, a), slices.Index(grades, b)) })
		slices.Sort(c.Crews)
		v.Classrooms = append(v.Classrooms, *c)
	}
	slices.SortFunc(v.Classrooms, func(a, b Classroom) int {
		return cmp.Or(cmp.Compare(firstGrade(a), firstGrade(b)), strings.Compare(a.Name, b.Name))
	})
	if len(v.Classrooms) == 0 {
		return nil, fmt.Errorf("no classrooms")
	}
	return v, nil
}

func firstGrade(c Classroom) int {
	if len(c.Grades) == 0 {
		return len(grades)
	}
	return slices.Index(grades, c.Grades[0])
}

func (v *Vocabulary) ClassroomNames() []string {
	out := []string{}
	for _, c := range v.Classrooms {
		out = append(out, c.Name)
	}
	return out
}

func (v *Vocabulary) ClassroomID(name string) (string, bool) {
	for _, c := range v.Classrooms {
		if c.Name == name {
			return c.ID, true
		}
	}
	return "", false
}

func (v *Vocabulary) DayTypeNames() []string {
	return slices.Sorted(maps.Keys(DayTemplates))
}

func (v *Vocabulary) Glossary() string {
	b := &strings.Builder{}
	b.WriteString("Helios School is a K-8 school. Students belong to a homeroom classroom named for a bird. Two classrooms make a grade band whose name is a portmanteau of the two classroom names. Lower School is Kindergarten through Grade 4 and Middle School is Grade 5 through Grade 8.\n\nBands and their classrooms:\n")
	bands := []string{}
	for _, c := range v.Classrooms {
		if c.Band != "" && !slices.Contains(bands, c.Band) {
			bands = append(bands, c.Band)
		}
	}
	for _, band := range bands {
		parts := []string{}
		for _, c := range v.Classrooms {
			if c.Band == band {
				parts = append(parts, c.Name+" (grades "+strings.Join(c.Grades, ", ")+")")
			}
		}
		fmt.Fprintf(b, "- %s = %s\n", band, strings.Join(parts, " + "))
	}
	b.WriteString("\nClassrooms with crews (a crew is a smaller group within a classroom, written as crew then classroom):\n")
	for _, c := range v.Classrooms {
		if len(c.Crews) > 0 {
			fmt.Fprintf(b, "- %s: %s\n", c.Name, strings.Join(c.Crews, ", "))
		}
	}
	b.WriteString(`
Vocabulary seen in event titles:
- K or Kinder means Kindergarten. "1/2", "3/4", "5/6", "7/8" name grade pairs, which are bands. "8th Grade" means Grade 8. LS is Lower School, MS is Middle School.
- A singular band name (Cosprey, Hegret, Jayven, Halcon) means the band.
- CoL or COL is a Celebration of Learning, a showcase where students present their work to families.
- ILP is an Individual Learning Plan conference between a family and teachers.
- MAP is the MAP Growth assessment students take.
- BTSN or Back to School Night is an evening for parents.
- CAFE is a morning coffee for the parents of one band or classroom.
- Beacon Wellness is the school's counseling program; its chats are for parents.
- HCA is the Helios Community Association, the parent association; its events are for parents and families.
- PD or Professional Development is a day or half day staff work while students stay home or leave early.
- Intersession is a week of alternative programming, still a school day.
- Passion Projects are Middle School project weeks.
- Aftercare is the after-school care program.
`)
	return b.String()
}

func (v *Vocabulary) classifierSystem() string {
	described := &strings.Builder{}
	for _, c := range v.Events {
		fmt.Fprintf(described, "- %s: %s\n", c.Title, c.Description)
	}
	return v.Glossary() + `
You classify events from the school calendar for a family-facing app. For each event, using only its title, dates, location and description:

- classrooms: the classrooms the event concerns, as the narrowest set the event supports: a band's two classrooms when the title names a band, a grade pair, or both classrooms; one classroom when it names one; the Lower School or Middle School classrooms when it says LS or MS; every classroom when nothing narrows it.
- who: "families" when students and their families take part; "parents" when adults attend without students, such as a parent coffee, a parent education session or an HCA meeting; "staff" when it is for staff only.
- categories: every one of these that applies, possibly none:
` + described.String() + `- dayType: for an all-day event only, the day type it imposes on the students it applies to, or "None". "No School" for holidays, breaks and professional development days; "Early Dismissal" for early dismissal and half days; any other listed type only when the title says so plainly. An event with a time of day, or one that merely happens on a school day, is "None".

Answer under every event's id, repeating its title exactly as given.`
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum)[:16]
}

type CalendarItem struct {
	Key         string `json:"-"`
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      bool   `json:"allDay"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
}

type Classification struct {
	Categories []string
	Classrooms []string
	Who        string
	DayType    string
}

func (v *Vocabulary) InputHash(it CalendarItem) string {
	names := []string{}
	for _, c := range v.Classrooms {
		names = append(names, c.ID+"="+c.Name)
	}
	return digest(it.Title, it.Start, it.End, strconv.FormatBool(it.AllDay), it.Location, it.Description, v.classifierSystem(), strings.Join(names, ","), strings.Join(v.DayTypeNames(), ","))
}

func enumOf(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

type classifyAnswer struct {
	Title      string   `json:"title"`
	Categories []string `json:"categories"`
	Classrooms []string `json:"classrooms"`
	Who        string   `json:"who"`
	DayType    string   `json:"dayType"`
}

func Ask(ctx context.Context, client anthropic.Client, system string, content []anthropic.ContentBlockParamUnion, schema map[string]any, out any) (string, error) {
	return claude.JSON(ctx, client, anthropic.MessageNewParams{
		Model:     CalendarModel,
		MaxTokens: 64000,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort("max"), Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, out)
}

func FanOut[T any](n int, f func(i int) (T, error)) ([]T, []error) {
	results := make([]T, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { results[i], errs[i] = f(i) })
	}
	wg.Wait()
	return results, errs
}

func (v *Vocabulary) Classify(ctx context.Context, client anthropic.Client, items []CalendarItem) map[string]Classification {
	out := map[string]Classification{}
	batches := [][]CalendarItem{}
	for start := 0; start < len(items); start += classifyBatch {
		batches = append(batches, items[start:min(start+classifyBatch, len(items))])
	}
	results, errs := FanOut(len(batches), func(i int) (map[string]Classification, error) {
		return v.classifyBatch(ctx, client, batches[i])
	})
	for i := range batches {
		if errs[i] != nil {
			slog.ErrorContext(ctx, "calendar import: classify events", "batch", i+1, "of", len(batches), "error", errs[i])
			continue
		}
		maps.Copy(out, results[i])
	}
	slog.InfoContext(ctx, "calendar import: classified", "events", len(out), "of", len(items))
	return out
}

func classifyRequest(items []CalendarItem) (string, map[string]CalendarItem, error) {
	type handled struct {
		ID string `json:"id"`
		CalendarItem
	}
	sent := map[string]CalendarItem{}
	list := []handled{}
	for i, it := range items {
		handle := "e" + strconv.Itoa(i+1)
		sent[handle] = it
		list = append(list, handled{ID: handle, CalendarItem: it})
	}
	encoded, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return "", nil, err
	}
	return "Classify each of these events:\n\n" + string(encoded), sent, nil
}

func (v *Vocabulary) classifyBatch(ctx context.Context, client anthropic.Client, items []CalendarItem) (map[string]Classification, error) {
	categories := []string{}
	byTitle := map[string]string{}
	for _, c := range v.Events {
		categories = append(categories, c.Title)
		byTitle[c.Title] = c.ID
	}
	one := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"title", "categories", "classrooms", "who", "dayType"},
		"properties": map[string]any{
			"title":      map[string]any{"type": "string", "description": "the event's title, exactly as given"},
			"categories": map[string]any{"type": "array", "items": enumOf(categories)},
			"classrooms": map[string]any{"type": "array", "minItems": 1, "items": enumOf(v.ClassroomNames())},
			"who":        enumOf([]string{"families", "parents", "staff"}),
			"dayType":    enumOf(append([]string{noDayType}, v.DayTypeNames()...)),
		},
	}
	properties := map[string]any{}
	required := []string{}
	for i := range items {
		handle := "e" + strconv.Itoa(i+1)
		properties[handle] = map[string]any{"$ref": "#/$defs/answer"}
		required = append(required, handle)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": properties,
		"$defs": map[string]any{"answer": one},
	}
	request, sent, err := classifyRequest(items)
	if err != nil {
		return nil, err
	}
	content := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(request)}
	for attempt := 1; ; attempt++ {
		answers := map[string]classifyAnswer{}
		raw, err := Ask(ctx, client, v.classifierSystem(), content, schema, &answers)
		if err != nil {
			return nil, err
		}
		out, err := v.classified(sent, answers, byTitle)
		if err == nil {
			return out, nil
		}
		if attempt == 2 {
			return nil, fmt.Errorf("%w; the answer was %s", err, raw)
		}
		slog.WarnContext(ctx, "calendar import: asking for the classification again", "error", err)
	}
}

func (v *Vocabulary) classified(sent map[string]CalendarItem, answers map[string]classifyAnswer, byTitle map[string]string) (map[string]Classification, error) {
	out := map[string]Classification{}
	for handle, it := range sent {
		a := answers[handle]
		if Collapse(a.Title) != Collapse(it.Title) {
			return nil, fmt.Errorf("claude answered %s with the title %q, not %q", handle, a.Title, it.Title)
		}
		c := Classification{Who: a.Who}
		for _, title := range a.Categories {
			c.Categories = append(c.Categories, byTitle[title])
		}
		for _, name := range a.Classrooms {
			id, _ := v.ClassroomID(name)
			c.Classrooms = append(c.Classrooms, id)
		}
		if a.DayType != noDayType && it.AllDay {
			c.DayType = a.DayType
		}
		out[it.Key] = c
	}
	return out, nil
}

func Collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
