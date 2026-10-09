package tools

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/db"
	"heliosian/internal/store"
)

var roleSlugs = map[string]string{"students": "student", "parents": "parent", "staff": "staff"}

var roleOrder = []string{"staff", "parent", "student"}

type member struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Href       string   `json:"href,omitempty"`
	Roles      []string `json:"roles,omitempty"`
	JobTitle   string   `json:"job_title,omitempty"`
	Grade      string   `json:"grade,omitempty"`
	Classroom  string   `json:"classroom,omitempty"`
	Crew       string   `json:"crew,omitempty"`
	Department string   `json:"department,omitempty"`
	Pronouns   string   `json:"pronouns,omitempty"`
	Email      string   `json:"email,omitempty"`
	Phone      string   `json:"phone,omitempty"`
	Lead       bool     `json:"lead,omitempty"`
	Reasons    string   `json:"reasons,omitempty"`
	Via        string   `json:"via,omitempty"`
	sort       string
}

func (c *call) roles(ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	found, res, err := c.rows(tree{"from": "EFFECTIVE_MEMBER", "where": []any{among("person", ids), eq("group.kind", "group"), among("group.slug", []string{"students", "parents", "staff"})}, "include": []any{"group"}})
	if err != nil {
		return nil, err
	}
	for _, r := range found {
		role := roleSlugs[res.Resources["GROUP"][r["group"]]["slug"]]
		if !slices.Contains(out[r["person"]], role) {
			out[r["person"]] = append(out[r["person"]], role)
		}
	}
	for id := range out {
		slices.SortFunc(out[id], func(a, b string) int { return slices.Index(roleOrder, a) - slices.Index(roleOrder, b) })
	}
	return out, nil
}

func rank(m member) int {
	for i, role := range roleOrder {
		if slices.Contains(m.Roles, role) {
			return i
		}
	}
	return len(roleOrder)
}

func (c *call) people(ids []string, contact bool) ([]member, error) {
	out := []member{}
	if len(ids) == 0 {
		return out, nil
	}
	found, res, err := c.rows(tree{"from": "PERSON", "where": []any{among("id", ids)}, "include": []any{"classroom", "crew", "department"}})
	if err != nil {
		return nil, err
	}
	roles, err := c.roles(ids)
	if err != nil {
		return nil, err
	}
	emails := map[string]string{}
	if contact {
		addresses, _, err := c.rows(tree{"from": "PERSON_EMAIL", "where": []any{among("person", ids), path("primary")}})
		if err != nil {
			return nil, err
		}
		for _, e := range addresses {
			emails[e["person"]] = e["address"]
		}
	}
	groups := res.Resources["GROUP"]
	for _, p := range found {
		m := member{ID: p["id"], Name: title(p), Href: c.href("PERSON", p), Roles: roles[p["id"]], JobTitle: p["job_title"], Grade: p["grade"], Pronouns: p["pronouns"], sort: p["name_sort"]}
		m.Classroom, m.Crew, m.Department = groups[p["classroom"]]["name"], groups[p["crew"]]["name"], groups[p["department"]]["name"]
		if contact {
			m.Email, m.Phone = emails[p["id"]], p["phone"]
		}
		out = append(out, m)
	}
	slices.SortFunc(out, func(a, b member) int {
		return cmp.Or(rank(a)-rank(b), strings.Compare(a.sort, b.sort), strings.Compare(a.Name, b.Name))
	})
	return out, nil
}

type family struct {
	named
	Address string   `json:"address,omitempty"`
	Phone   string   `json:"phone,omitempty"`
	Members []member `json:"members"`
}

func (c *call) familiesOf(groups []store.Row) ([]family, error) {
	out := []family{}
	for _, g := range groups {
		in, _, err := c.rows(tree{"from": "MEMBER", "where": []any{eq("group", g["id"]), eq("member", "yes")}})
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, m := range in {
			ids = append(ids, m["person"])
		}
		members, err := c.people(ids, true)
		if err != nil {
			return nil, err
		}
		out = append(out, family{named: c.named("GROUP", g), Address: g["address"], Phone: g["phone"], Members: members})
	}
	return out, nil
}

func (c *call) families(person string) ([]family, error) {
	groups, _, err := c.rows(tree{"from": "GROUP", "as": "f", "where": []any{eq("kind", "family"), tree{"exists": tree{"from": "MEMBER", "where": []any{eq("group", path("@f")), eq("person", person), eq("member", "yes")}}}}, "order": asc("name")})
	if err != nil {
		return nil, err
	}
	return c.familiesOf(groups)
}

type place struct {
	named
	Kind string `json:"kind"`
	Lead bool   `json:"lead,omitempty"`
}

var whoami = define("helios_whoami", "Looking you up", "The person connected to Helios School's community data: who they are and their roles, their addresses, each family they are in with its members, the groups they belong to by name (classrooms, sign-ups, tickets, email lists, tags), the groups they manage and the Helios apps they are an admin of. Use it for questions about \"my family\", \"my kids\" or \"my classes\" at Helios.", func(c *call, _ none) (any, error) {
	me, err := c.people([]string{c.env.Viewer}, true)
	if err != nil {
		return nil, err
	}
	if len(me) == 0 {
		return nil, errors.New("the directory does not show you")
	}
	addresses, _, err := c.rows(tree{"from": "PERSON_EMAIL", "where": []any{eq("person", path("@viewer"))}, "order": asc("address")})
	if err != nil {
		return nil, err
	}
	emails := []string{}
	for _, a := range addresses {
		emails = append(emails, a["address"])
	}
	families, err := c.families(c.env.Viewer)
	if err != nil {
		return nil, err
	}
	in, res, err := c.rows(tree{"from": "MEMBER", "where": []any{eq("person", path("@viewer")), eq("member", "yes"), tree{"!=": []any{path("group.kind"), "family"}}}, "include": []any{"group"}})
	if err != nil {
		return nil, err
	}
	groups := []place{}
	for _, m := range in {
		g := res.Resources["GROUP"][m["group"]]
		if g == nil {
			continue
		}
		groups = append(groups, place{named: c.named("GROUP", g), Kind: g["kind"], Lead: m["lead"] == "Yes"})
	}
	slices.SortFunc(groups, func(a, b place) int { return cmp.Or(strings.Compare(a.Kind, b.Kind), strings.Compare(a.Name, b.Name)) })
	managed, _, err := c.rows(tree{"from": "GROUP", "as": "g", "where": []any{
		tree{"!=": []any{path("kind"), "family"}},
		tree{"!=": []any{path("managed_by"), path("@g")}},
		tree{"exists": tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("group", path("@g.managed_by")), eq("person", path("@viewer"))}}},
	}, "order": asc("name"), "limit": listRows})
	if err != nil {
		return nil, err
	}
	manages := []place{}
	for _, g := range managed {
		manages = append(manages, place{named: c.named("GROUP", g), Kind: g["kind"]})
	}
	apps, _, err := c.rows(tree{"from": "APP", "where": []any{tree{"admin_of": []any{path("key")}}}, "order": asc("key")})
	if err != nil {
		return nil, err
	}
	admin := []string{}
	for _, a := range apps {
		admin = append(admin, a["key"])
	}
	return map[string]any{"person": me[0], "emails": emails, "families": families, "groups": groups, "manages": manages, "adminOf": admin}, nil
})

type peopleIn struct {
	Words      string `json:"words,omitempty" jsonschema:"words of a name, or of what a person's profile says, matched by the search index"`
	Role       string `json:"role,omitempty" jsonschema:"student, parent or staff"`
	Grade      string `json:"grade,omitempty" jsonschema:"a student's grade: K or 1 to 8"`
	Classroom  string `json:"classroom,omitempty" jsonschema:"a student's or teacher's classroom, by name"`
	Department string `json:"department,omitempty" jsonschema:"a staff member's department, by name"`
}

var roleGroups = map[string]string{"student": "students", "parent": "parents", "staff": "staff"}

var findPeople = define("helios_find_people", "Looking in the directory", "People in the Helios School directory by any of words (a name, job title or the like), role (student, parent or staff), grade, classroom and department: each with their roles, grade, classroom, crew, department, job title, pronouns, primary email and phone as shared, and href.", func(c *call, in peopleIn) (any, error) {
	where := []any{}
	if words := strings.TrimSpace(in.Words); words != "" {
		found, err := c.deps.Search.Search(c.ctx, c.m, c.env, words, map[string]int{"GROUP": 0, "PERSON": listRows, "DOCUMENT": 0})
		if err != nil {
			return nil, err
		}
		ids := []string{}
		for _, h := range found["PERSON"] {
			ids = append(ids, h.ID)
		}
		if len(ids) == 0 {
			return []member{}, nil
		}
		where = append(where, among("id", ids))
	}
	if in.Role != "" {
		slug, ok := roleGroups[strings.ToLower(strings.TrimSpace(in.Role))]
		if !ok {
			return nil, fmt.Errorf("role is student, parent or staff, not %q", in.Role)
		}
		where = append(where, tree{"exists": tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("person", path("@p")), eq("group.slug", slug), eq("group.kind", "group")}}})
	}
	if in.Grade != "" {
		where = append(where, eq("grade", strings.TrimSpace(in.Grade)))
	}
	if in.Classroom != "" {
		where = append(where, eq("classroom.name", strings.TrimSpace(in.Classroom)))
	}
	if in.Department != "" {
		where = append(where, eq("department.name", strings.TrimSpace(in.Department)))
	}
	if len(where) == 0 {
		return nil, errors.New("name at least one of words, role, grade, classroom and department")
	}
	found, _, err := c.rows(tree{"from": "PERSON", "as": "p", "where": where, "limit": listRows})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, p := range found {
		ids = append(ids, p["id"])
	}
	return c.people(ids, true)
})

func (c *call) managers(self store.Row) ([]member, error) {
	managing := []string{}
	for cur, depth := self, 0; depth < maxAncestors; depth++ {
		if cur["managed_by"] != "" && !slices.Contains(managing, cur["managed_by"]) {
			managing = append(managing, cur["managed_by"])
		}
		if cur["parent"] == "" {
			break
		}
		next, ok, err := c.row("GROUP", cur["parent"])
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		cur = next
	}
	if len(managing) == 0 {
		return []member{}, nil
	}
	found, res, err := c.rows(tree{"from": "EFFECTIVE_MEMBER", "where": []any{among("group", managing)}, "include": []any{"group"}})
	if err != nil {
		return nil, err
	}
	via := map[string]string{}
	ids := []string{}
	for _, m := range found {
		if _, seen := via[m["person"]]; !seen {
			ids = append(ids, m["person"])
			via[m["person"]] = title(res.Resources["GROUP"][m["group"]])
		}
	}
	out, err := c.people(ids, false)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Via = via[out[i].ID]
	}
	return out, nil
}

func (c *call) membersOf(group string) ([]member, int, error) {
	found, _, err := c.rows(tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("group", group)}})
	if err != nil {
		return nil, 0, err
	}
	reasons := map[string]string{}
	ids := []string{}
	for _, m := range found {
		reasons[m["person"]] = m["reasons"]
		ids = append(ids, m["person"])
	}
	leads, _, err := c.rows(tree{"from": "MEMBER", "where": []any{eq("group", group), path("lead")}})
	if err != nil {
		return nil, 0, err
	}
	led := map[string]bool{}
	for _, m := range leads {
		led[m["person"]] = true
	}
	out, err := c.people(ids, false)
	if err != nil {
		return nil, 0, err
	}
	for i := range out {
		out[i].Reasons, out[i].Lead = reasons[out[i].ID], led[out[i].ID]
	}
	slices.SortStableFunc(out, func(a, b member) int {
		if a.Lead != b.Lead {
			if a.Lead {
				return -1
			}
			return 1
		}
		return 0
	})
	count := len(out)
	return out[:min(count, listRows)], count, nil
}

var group = define("helios_group", "Looking at a group", "A Helios School group by ID in depth - a family, classroom, grade, event, volunteer activity, party, email list or any other group: who manages it and through which group, its members with why each is in, each marked student, parent or staff with a staff member's job title, leads first and then staff, parents and students, its rules, and the groups under it.", func(c *call, in idIn) (any, error) {
	if _, err := tableOf(in.ID, "GROUP"); err != nil {
		return nil, err
	}
	id := strings.TrimSpace(in.ID)
	self, ok, err := c.row("GROUP", id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no group %s that you can see", id)
	}
	managers, err := c.managers(self)
	if err != nil {
		return nil, err
	}
	members, count, err := c.membersOf(id)
	if err != nil {
		return nil, err
	}
	rules, rulesQuery, err := c.run(tree{"from": "RULE", "where": []any{eq("group", id)}, "order": asc("order"), "include": []any{"target", "person", "within"}})
	if err != nil {
		return nil, err
	}
	under, underQuery, err := c.run(tree{"from": "GROUP", "where": []any{eq("parent", id)}, "order": asc("order"), "limit": listRows})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"group":        c.compact("GROUP", self),
		"managers":     managers,
		"memberCount":  count,
		"members":      members,
		"membersShown": len(members),
		"rules":        c.shape(rules, rulesQuery),
		"under":        c.shape(under, underQuery),
	}, nil
})

type classroomIn struct {
	Name string `json:"name,omitempty" jsonschema:"the classroom's name, such as Falcons; leave it out for every classroom in brief"`
}

type crewOut struct {
	named
	Teachers []member `json:"teachers"`
	Students []member `json:"students"`
}

type bandGroup struct {
	named
	Members []member `json:"members"`
}

var classroom = define("helios_classroom", "Looking at a classroom", "One Helios School classroom by name: its band, color and grades, its teachers with their job titles, its crews each with its teachers and students, its students with their grade and crew, and its band's own groups with their members - the band's room parents. Without a name, every classroom in brief with its band and teachers.", func(c *call, in classroomIn) (any, error) {
	name := strings.TrimSpace(in.Name)
	where := []any{eq("kind", "classroom")}
	if name != "" {
		where = append(where, eq("name", name))
	}
	rooms, res, err := c.rows(tree{"from": "GROUP", "where": where, "order": asc("name"), "include": []any{"parent"}})
	if err != nil {
		return nil, err
	}
	if len(rooms) == 0 {
		return nil, fmt.Errorf("no classroom named %q; helios_classroom with no name lists them", name)
	}
	if name == "" {
		out := []map[string]any{}
		for _, room := range rooms {
			teachers, err := c.inClassroom("classroom", room["id"], "staff")
			if err != nil {
				return nil, err
			}
			out = append(out, map[string]any{"classroom": c.named("GROUP", room), "band": res.Resources["GROUP"][room["parent"]]["name"], "color": room["color"], "teachers": teachers})
		}
		return out, nil
	}
	room := rooms[0]
	band := res.Resources["GROUP"][room["parent"]]
	teachers, err := c.inClassroom("classroom", room["id"], "staff")
	if err != nil {
		return nil, err
	}
	students, err := c.inClassroom("classroom", room["id"], "student")
	if err != nil {
		return nil, err
	}
	grades := []string{}
	for _, s := range students {
		if s.Grade != "" && !slices.Contains(grades, s.Grade) {
			grades = append(grades, s.Grade)
		}
	}
	slices.Sort(grades)
	crewRows, _, err := c.rows(tree{"from": "GROUP", "where": []any{eq("kind", "crew"), eq("parent", room["id"])}, "order": asc("name")})
	if err != nil {
		return nil, err
	}
	crews := []crewOut{}
	for _, crew := range crewRows {
		crewTeachers, err := c.inClassroom("crew", crew["id"], "staff")
		if err != nil {
			return nil, err
		}
		crewStudents, err := c.inClassroom("crew", crew["id"], "student")
		if err != nil {
			return nil, err
		}
		crews = append(crews, crewOut{named: c.named("GROUP", crew), Teachers: crewTeachers, Students: crewStudents})
	}
	bandGroups := []bandGroup{}
	if band != nil {
		under, _, err := c.rows(tree{"from": "GROUP", "where": []any{eq("parent", band["id"]), eq("kind", "group")}, "order": asc("name")})
		if err != nil {
			return nil, err
		}
		for _, g := range under {
			members, _, err := c.membersOf(g["id"])
			if err != nil {
				return nil, err
			}
			bandGroups = append(bandGroups, bandGroup{named: c.named("GROUP", g), Members: members})
		}
	}
	out := map[string]any{"classroom": c.named("GROUP", room), "color": room["color"], "grades": grades, "teachers": teachers, "crews": crews, "students": students, "bandGroups": bandGroups}
	if band != nil {
		out["band"] = band["name"]
	}
	return out, nil
})

func (c *call) inClassroom(column, group, role string) ([]member, error) {
	found, _, err := c.rows(tree{"from": "PERSON", "as": "p", "where": []any{eq(column, group), tree{"exists": tree{"from": "EFFECTIVE_MEMBER", "where": []any{eq("person", path("@p")), eq("group.slug", roleGroups[role]), eq("group.kind", "group")}}}}})
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, p := range found {
		ids = append(ids, p["id"])
	}
	return c.people(ids, role == "staff")
}

type nearbyIn struct {
	Family      string  `json:"family,omitempty" jsonschema:"the ID of the family to measure from; the viewer's own when left out"`
	Classroom   string  `json:"classroom,omitempty" jsonschema:"only families with a student in this classroom, by name"`
	Grade       string  `json:"grade,omitempty" jsonschema:"only families with a student in this grade: K or 1 to 8"`
	WithinMiles float64 `json:"within_miles,omitempty" jsonschema:"only families within this many miles"`
	Limit       int     `json:"limit,omitempty" jsonschema:"how many families to answer, 10 when left out, 30 at most"`
}

type nearby struct {
	family
	Miles float64 `json:"milesAway"`
}

const earthMiles = 3958.8

func miles(lat1, lng1, lat2, lng2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return math.Round(2*earthMiles*math.Asin(math.Sqrt(a))*10) / 10
}

var nearbyFamilies = define("helios_nearby_families", "Looking for families nearby", "Helios School families who live near a family - the viewer's own unless another is named by ID - nearest first, by straight-line distance between the street addresses families share in Helios Who?, as its map shows them: for carpools, walking groups and playdates. Each comes with milesAway, its address as shared, and its members marked student, parent or staff with each student's grade and classroom. Narrow them to a classroom, a grade or a distance. A family that shares no street address, or whose address has not been placed on the map yet, is never listed.", func(c *call, in nearbyIn) (any, error) {
	points := map[string][2]float64{}
	geocodes, _, err := c.rows(tree{"from": "GEOCODE"})
	if err != nil {
		return nil, err
	}
	for _, g := range geocodes {
		if !db.Placeable(g["address"]) {
			continue
		}
		lat, err1 := strconv.ParseFloat(g["lat"], 64)
		lng, err2 := strconv.ParseFloat(g["lng"], 64)
		if err1 == nil && err2 == nil {
			points[g["address"]] = [2]float64{lat, lng}
		}
	}
	var from store.Row
	if id := strings.TrimSpace(in.Family); id != "" {
		row, ok, err := c.row("GROUP", id)
		if err != nil {
			return nil, err
		}
		if !ok || row["kind"] != "family" {
			return nil, fmt.Errorf("no family %s that you can see", id)
		}
		from = row
	} else {
		own, _, err := c.rows(tree{"from": "GROUP", "as": "f", "where": []any{eq("kind", "family"), tree{"exists": tree{"from": "MEMBER", "where": []any{eq("group", path("@f")), eq("person", path("@viewer")), eq("member", "yes")}}}}, "order": asc("name")})
		if err != nil {
			return nil, err
		}
		for _, f := range own {
			if _, ok := points[f["address"]]; ok {
				from = f
				break
			}
		}
		if from == nil {
			return nil, errors.New("none of your families shares a street address placed on Helios Who?'s map, so there is nothing to measure from")
		}
	}
	origin, ok := points[from["address"]]
	if !ok {
		return nil, fmt.Errorf("%s shares no street address placed on Helios Who?'s map, only a city or nothing, so there is nothing to measure from", title(from))
	}
	where := []any{eq("kind", "family"), tree{"!=": []any{path("id"), from["id"]}}, tree{"not": tree{"blank": path("address")}}}
	student := []any{eq("group", path("@f")), eq("member", "yes")}
	if in.Classroom != "" {
		student = append(student, eq("person.classroom.name", strings.TrimSpace(in.Classroom)))
	}
	if in.Grade != "" {
		student = append(student, eq("person.grade", strings.TrimSpace(in.Grade)))
	}
	if in.Classroom != "" || in.Grade != "" {
		where = append(where, tree{"exists": tree{"from": "MEMBER", "where": student}})
	}
	candidates, _, err := c.rows(tree{"from": "GROUP", "as": "f", "where": where})
	if err != nil {
		return nil, err
	}
	type scored struct {
		row   store.Row
		miles float64
	}
	ranked := []scored{}
	for _, f := range candidates {
		p, ok := points[f["address"]]
		if !ok {
			continue
		}
		d := miles(origin[0], origin[1], p[0], p[1])
		if in.WithinMiles > 0 && d > in.WithinMiles {
			continue
		}
		ranked = append(ranked, scored{row: f, miles: d})
	}
	slices.SortFunc(ranked, func(a, b scored) int {
		return cmp.Or(cmp.Compare(a.miles, b.miles), strings.Compare(a.row["name"], b.row["name"]))
	})
	limit := 10
	if in.Limit > 0 {
		limit = min(in.Limit, 30)
	}
	ranked = ranked[:min(len(ranked), limit)]
	out := []nearby{}
	for _, s := range ranked {
		fams, err := c.familiesOf([]store.Row{s.row})
		if err != nil {
			return nil, err
		}
		out = append(out, nearby{family: fams[0], Miles: s.miles})
	}
	return map[string]any{"from": c.named("GROUP", from), "families": out, "note": "Distances are straight lines between the addresses families share, not driving routes."}, nil
})
