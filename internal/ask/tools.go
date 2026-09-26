package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const (
	maxToolOutput = 40000
	whoBase       = "https://who.heliosian.com"
	whenBase      = "https://when.heliosian.com"
	teamBase      = "https://team.heliosian.com"
	celebrateBase = "https://celebrate.heliosian.com"
	loopBase      = "https://loop.heliosian.com"
)

// A tool is one thing the model may look up: its name and description for
// the model, the words the page shows while it runs, its input's schema,
// and what it does for one viewer.
type tool struct {
	name        string
	description string
	words       string
	properties  map[string]any
	required    []string
	run         func(v *viewer, input json.RawMessage) (any, error)
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolean(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

// definitions is every tool as the API takes them.
func definitions() []anthropic.BetaToolUnionParam {
	out := []anthropic.BetaToolUnionParam{}
	for _, t := range tools {
		properties := t.properties
		if properties == nil {
			properties = map[string]any{}
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:        t.name,
			Description: anthropic.String(t.description),
			InputSchema: anthropic.BetaToolInputSchemaParam{Properties: properties, Required: t.required},
		}})
	}
	return out
}

// label is the words the page shows for a tool at work.
func label(name string) string {
	for _, t := range tools {
		if t.name == name {
			return t.words
		}
	}
	return "Looking something up"
}

// A viewer is one turn's reader: the person, and every model as it stood
// when the turn began, so a refresh mid-answer changes nothing under it.
type viewer struct {
	email     string
	me        *who.Person
	directory *who.Model
	calendar  *calendar.Model
	team      *team.Model
	celebrate *celebrate.Model
	loop      *loop.Model
	artifacts *artifacts.Model
	embedder  artifacts.Embedder
	sources   Sources
	now       time.Time
	teamAs    access.Viewer
	partyAs   access.Viewer
	loopAs    access.Viewer
	whenAs    access.Viewer
	homeAs    access.Viewer
	access    *groupAccess
	ctx       context.Context
}

func (a app) viewer(email string) *viewer {
	v := &viewer{
		email: email, directory: a.sources.Directory(), calendar: a.sources.Calendar(), team: a.sources.Team(), celebrate: a.sources.Celebrate(), loop: a.sources.Loop(),
		artifacts: a.sources.Artifacts(), embedder: a.sources.Embedder,
		sources: a.sources, now: time.Now().In(calendar.Location), access: &groupAccess{}, ctx: context.Background(),
	}
	v.me = v.directory.Person(email)
	household := v.directory.Family(email)
	as := func(admin func(string) bool) access.Viewer {
		return access.Viewer{Email: email, Admin: admin(email), Household: household}
	}
	v.teamAs, v.partyAs, v.loopAs, v.whenAs, v.homeAs = as(a.sources.Admins.Team), as(a.sources.Admins.Celebrate), as(a.sources.Admins.Loop), as(a.sources.Admins.Calendar), as(a.sources.Admins.Home)
	return v
}

// run answers one tool call for the viewer: the result as JSON, or an
// error the model reads. An answer too big to be useful is refused with
// a word on narrowing it.
func (v *viewer) run(ctx context.Context, name string, input json.RawMessage) (string, error) {
	i := slices.IndexFunc(tools, func(t tool) bool { return t.name == name })
	if i < 0 {
		return "", fmt.Errorf("there is no tool called %s", name)
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	scoped := *v
	scoped.ctx = ctx
	result, err := tools[i].run(&scoped, input)
	if err != nil {
		return "", err
	}
	encoded := &bytes.Buffer{}
	encoder := json.NewEncoder(encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return "", err
	}
	if encoded.Len() > maxToolOutput {
		return "", fmt.Errorf("that is too much at once (%d characters); narrow it with a name, a date range, a classroom or a smaller limit", encoded.Len())
	}
	return strings.TrimSpace(encoded.String()), nil
}

func decodeInput[T any](input json.RawMessage) (T, error) {
	var in T
	if err := json.Unmarshal(input, &in); err != nil {
		return in, fmt.Errorf("the input could not be read: %w", err)
	}
	return in, nil
}

// name is someone's name as the directory has it, else their address
// before the @.
func (v *viewer) name(email string) string {
	if p := v.directory.Person(v.directory.Resolve(email)); p != nil {
		return p.FullName
	}
	local, _, _ := strings.Cut(email, "@")
	return local
}

func (v *viewer) names(emails []string) []string {
	out := []string{}
	for _, e := range emails {
		out = append(out, v.name(e))
	}
	return out
}

func whoLink(email string) string {
	return whoBase + who.PersonPath(email)
}

var appBases = map[string]string{"calendar": whenBase, "team": teamBase, "celebrate": celebrateBase}

// classroomTeachers is the staff who teach in a classroom, by name: the
// crews' teachers, and any staff member placed in it.
func (v *viewer) classroomTeachers(classroom string) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(email string) {
		p := v.directory.Person(email)
		if p == nil || seen[p.Email] {
			return
		}
		seen[p.Email] = true
		out = append(out, p.FullName)
	}
	for _, c := range v.directory.Crews {
		if c.Classroom == classroom {
			for _, t := range c.Teachers {
				add(t)
			}
		}
	}
	for i := range v.directory.People {
		p := &v.directory.People[i]
		if p.IsStaff && p.Classroom == classroom {
			add(p.Email)
		}
	}
	return out
}

// crewTeachers is a student's teachers: their crew's when the classroom
// has crews and they are in one, else the classroom's.
func (v *viewer) crewTeachers(classroom, crew string) []string {
	if crew != "" {
		for _, c := range v.directory.Crews {
			if c.Classroom == classroom && c.Name == crew && len(c.Teachers) > 0 {
				return v.names(c.Teachers)
			}
		}
	}
	return v.classroomTeachers(classroom)
}

func contains(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func limitOf(n, fallback, ceiling int) int {
	if n <= 0 {
		return fallback
	}
	return min(n, ceiling)
}

// rolesOf are the words for a person's roles.
func rolesOf(p *who.Person) string {
	roles := []string{}
	if p.IsStudent {
		roles = append(roles, "student")
	}
	if p.IsParent {
		roles = append(roles, "parent")
	}
	if p.IsStaff {
		roles = append(roles, "staff")
	}
	return strings.Join(roles, ", ")
}

// daysAway is how many days from today a Start or End cell falls: negative
// for a day gone by, zero for today. A blank or unreadable cell gives no
// answer.
func (v *viewer) daysAway(cell string) (int, bool) {
	cell = strings.TrimSpace(cell)
	if len(cell) < len(calendar.DateFormat) {
		return 0, false
	}
	day, err := time.ParseInLocation(calendar.DateFormat, cell[:len(calendar.DateFormat)], calendar.Location)
	if err != nil {
		return 0, false
	}
	today := time.Date(v.now.Year(), v.now.Month(), v.now.Day(), 0, 0, 0, 0, calendar.Location)
	return int(day.Sub(today).Hours() / 24), true
}

// timing is the words for when a thing is against today, so the model
// never has to reckon dates itself: "past", "today", "in 3 days", or
// "12 days ago".
func (v *viewer) timing(start, end string) string {
	last := end
	if last == "" {
		last = start
	}
	until, ok := v.daysAway(last)
	if !ok {
		return ""
	}
	from, _ := v.daysAway(start)
	switch {
	case until < 0:
		return fmt.Sprintf("past (%d days ago)", -until)
	case from <= 0:
		return "today"
	case from == 1:
		return "tomorrow"
	}
	return fmt.Sprintf("in %d days", from)
}

// date reads a date the model gives, in the school's clock.
func date(cell string) (time.Time, error) {
	t, err := time.ParseInLocation(calendar.DateFormat, strings.TrimSpace(cell), calendar.Location)
	if err != nil {
		return t, fmt.Errorf("%q is not a date like 2026-09-24", cell)
	}
	return t, nil
}

func sortedByName[T any](list []T, name func(T) string) {
	sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(name(list[i])) < strings.ToLower(name(list[j])) })
}

var tools = []tool{findPeople, getPerson, getFamily, nearbyFamilies, getClassroom, calendarEvents, dayPlan, volunteerOpportunities, getActivity, parties, myGroups, myLists, communityLinks, searchDocuments, readDocument}
