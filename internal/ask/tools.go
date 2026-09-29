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
	"heliosian/internal/model"
)

const (
	maxToolOutput = 40000
	whoBase       = "https://who.heliosian.com"
	whenBase      = "https://when.heliosian.com"
	teamBase      = "https://team.heliosian.com"
	celebrateBase = "https://celebrate.heliosian.com"
	loopBase      = "https://loop.heliosian.com"
)

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

func label(name string) string {
	for _, t := range tools {
		if t.name == name {
			return t.words
		}
	}
	return "Looking something up"
}

type viewer struct {
	email     string
	me        *model.Person
	directory *model.Directory
	calendar  *model.Calendar
	team      *model.Activities
	celebrate *model.Parties
	loop      *model.EmailLists
	library   *model.Documents
	embedder  *artifacts.Vertex
	all       *model.Model
	now       time.Time
	teamAs    access.Actor
	partyAs   access.Actor
	loopAs    access.Actor
	whenAs    access.Actor
	homeAs    access.Actor
	access    *groupAccess
	ctx       context.Context
}

func (a app) viewer(email string) *viewer {
	m := a.sources.Store.Model()
	v := &viewer{
		email: email, directory: m.Directory, calendar: m.Calendar, team: m.Activities, celebrate: m.Parties, loop: m.EmailLists,
		library: m.Documents, embedder: a.sources.Embedder,
		all: m, now: a.sources.Now().In(model.Location), access: &groupAccess{}, ctx: context.Background(),
	}
	v.me = v.directory.Person(email)
	as := func(app string) access.Actor {
		return v.directory.ActorOf(email, m.AdminList(app).Held(email))
	}
	v.teamAs, v.partyAs, v.loopAs, v.whenAs, v.homeAs = as("team"), as("celebrate"), as("loop"), as("when"), as("home")
	return v
}

func (v *viewer) linked() []model.Linked {
	return v.all.LinkedEvents(v.email, v.now)
}

func (v *viewer) audience() model.AudienceSources {
	return v.all.EmailListAudience(v.now)
}

func (v *viewer) lists(email string) []model.MagicTag {
	return append(v.directory.RoomParentTags(email), v.all.ManagedMagicTags(email, v.now)...)
}

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
	return whoBase + model.PersonPath(email)
}

var appBases = map[string]string{"when": whenBase, "team": teamBase, "celebrate": celebrateBase}

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

func rolesOf(p *model.Person) string {
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

func (v *viewer) daysAway(cell string) (int, bool) {
	cell = strings.TrimSpace(cell)
	if len(cell) < len(model.DateFormat) {
		return 0, false
	}
	day, err := time.ParseInLocation(model.DateFormat, cell[:len(model.DateFormat)], model.Location)
	if err != nil {
		return 0, false
	}
	today := time.Date(v.now.Year(), v.now.Month(), v.now.Day(), 0, 0, 0, 0, model.Location)
	return int(day.Sub(today).Hours() / 24), true
}

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

func date(cell string) (time.Time, error) {
	t, err := time.ParseInLocation(model.DateFormat, strings.TrimSpace(cell), model.Location)
	if err != nil {
		return t, fmt.Errorf("%q is not a date like 2026-09-24", cell)
	}
	return t, nil
}

func sortedByName[T any](list []T, name func(T) string) {
	sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(name(list[i])) < strings.ToLower(name(list[j])) })
}

var tools = []tool{findPeople, getPerson, getFamily, nearbyFamilies, getClassroom, calendarEvents, dayPlan, volunteerOpportunities, getActivity, parties, myGroups, myLists, communityLinks, searchDocuments, readDocument}
