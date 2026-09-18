package ask

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"heliosian/internal/who"
)

// card is a person as a tool answers: what the directory shows, with a
// masked email or phone left out rather than made up.
type card struct {
	Name        string   `json:"name"`
	Email       string   `json:"email,omitempty"`
	Roles       string   `json:"roles"`
	Pronouns    string   `json:"pronouns,omitempty"`
	Grade       string   `json:"grade,omitempty"`
	Classroom   string   `json:"classroom,omitempty"`
	Crew        string   `json:"crew,omitempty"`
	Teachers    []string `json:"teachers,omitempty"`
	JobTitle    string   `json:"jobTitle,omitempty"`
	Department  string   `json:"department,omitempty"`
	Phone       string   `json:"phone,omitempty"`
	NewToHelios bool     `json:"newToHelios,omitempty"`
	Kids        []string `json:"kids,omitempty"`
	Parents     []string `json:"parents,omitempty"`
	Link        string   `json:"link"`
}

func (v *viewer) card(p *who.Person) card {
	c := card{Name: p.FullName, Roles: rolesOf(p), Pronouns: p.Pronouns, Grade: p.Grade, Classroom: p.Classroom, Crew: p.Crew, JobTitle: p.JobTitle, Department: p.Department, Phone: p.Phone, NewToHelios: p.IsNew, Link: whoLink(p.Email)}
	if !p.EmailMasked {
		c.Email = p.Email
	}
	if p.IsStudent {
		c.Teachers = v.crewTeachers(p.Classroom, p.Crew)
		c.Parents = v.names(p.ParentContactEmails)
	}
	if p.IsParent {
		for _, key := range v.directory.FamilyKeysOf(p.Email) {
			for _, kid := range v.directory.Families[key].KidEmails {
				if k := v.directory.Person(kid); k != nil {
					words := k.FullName
					if k.Grade != "" {
						words += " (" + k.Grade + ")"
					}
					c.Kids = append(c.Kids, words)
				}
			}
		}
	}
	return c
}

type familyCard struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
	Phone   string `json:"phone,omitempty"`
	Adults  []card `json:"adults"`
	Kids    []card `json:"kids"`
	Caption string `json:"photoCaption,omitempty"`
	Link    string `json:"link"`
}

func (v *viewer) familyCard(key string) familyCard {
	family := v.directory.Families[key]
	f := familyCard{Name: family.Name, Address: family.Address, Phone: family.Phone, Adults: []card{}, Kids: []card{}, Caption: family.PhotoCaption, Link: whoBase + who.FamilyPath(key)}
	for _, email := range family.AdultEmails {
		if p := v.directory.Person(email); p != nil {
			f.Adults = append(f.Adults, v.card(p))
		}
	}
	for _, email := range family.KidEmails {
		if p := v.directory.Person(email); p != nil {
			f.Kids = append(f.Kids, v.card(p))
		}
	}
	return f
}

func (v *viewer) familyKeys(email, name string) []string {
	keys := []string{}
	for _, p := range v.findByEmailOrName(email, name) {
		for _, key := range v.directory.FamilyKeysOf(p.Email) {
			if !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	if name = strings.TrimSpace(name); name != "" {
		for key, family := range v.directory.Families {
			if contains(family.Name, name) && !slices.Contains(keys, key) {
				keys = append(keys, key)
			}
		}
	}
	slices.Sort(keys)
	return keys
}

// findByEmailOrName is the people an email or a name picks out: the one
// the address keys, else everyone whose name holds the words.
func (v *viewer) findByEmailOrName(email, name string) []*who.Person {
	if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
		if p := v.directory.Person(v.directory.Resolve(email)); p != nil {
			return []*who.Person{p}
		}
	}
	out := []*who.Person{}
	if name = strings.TrimSpace(name); name == "" {
		return out
	}
	for i := range v.directory.People {
		p := &v.directory.People[i]
		if contains(p.FullName, name) || contains(p.LegalName, name) || contains(p.PreferredName, name) {
			out = append(out, p)
		}
	}
	return out
}

var findPeople = tool{
	name:        "find_people",
	description: "Search the school directory (Helios Who?) for people: students, parents and staff. Each query is searched on its own, so every person a document names is found in one call. Every filter narrows every query; a parent matches a grade or classroom through their children. Returns, for each query, at most a page of people with what places them, their contact details as shared, and a link to each one's page.",
	words:       "Looking in the directory",
	properties: map[string]any{
		"queries":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Words to find in a name, an email or a job title, one entry per person or search. Leave it out to list everyone the filters pick."},
		"role":       map[string]any{"type": "string", "enum": []string{"student", "parent", "staff"}, "description": "Only people with this role."},
		"grade":      str("A grade as the school names it, such as Grade 3 or Kindergarten."),
		"classroom":  str("A classroom name."),
		"department": str("A staff department, for staff."),
		"limit":      integer("How many to return, 25 unless said, 50 at most."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct {
			Queries                            []string
			Role, Grade, Classroom, Department string
			Limit                              int
		}](input)
		if err != nil {
			return nil, err
		}
		limit := limitOf(in.Limit, 25, 50)
		queries := in.Queries
		if len(queries) == 0 {
			queries = []string{""}
		}
		results := []map[string]any{}
		for _, query := range queries {
			matches := []card{}
			total := 0
			for i := range v.directory.People {
				p := &v.directory.People[i]
				if !v.matches(p, strings.TrimSpace(query), in.Role, in.Grade, in.Classroom, in.Department) {
					continue
				}
				total++
				if len(matches) < limit {
					matches = append(matches, v.card(p))
				}
			}
			sortedByName(matches, func(c card) string { return c.Name })
			results = append(results, map[string]any{"query": query, "people": matches, "matched": total, "shown": len(matches)})
		}
		return map[string]any{"results": results}, nil
	},
}

// matches is Who?'s filters as the tool reads them: a parent's grade and
// classroom are their children's.
func (v *viewer) matches(p *who.Person, query, role, grade, classroom, department string) bool {
	if query != "" && !contains(p.FullName, query) && !contains(p.Email, query) && !contains(p.JobTitle, query) && !contains(p.PreferredName, query) {
		return false
	}
	switch role {
	case "student":
		if !p.IsStudent {
			return false
		}
	case "parent":
		if !p.IsParent {
			return false
		}
	case "staff":
		if !p.IsStaff {
			return false
		}
	}
	if department != "" && !contains(p.Department, department) {
		return false
	}
	if grade != "" && !slices.ContainsFunc(v.facets(p, false), func(g string) bool { return strings.EqualFold(g, grade) }) {
		return false
	}
	if classroom != "" && !slices.ContainsFunc(v.facets(p, true), func(c string) bool { return strings.EqualFold(c, classroom) }) {
		return false
	}
	return true
}

// facets is a person's grade or classroom as the filters read it: a
// student's own, a staff member's classroom, a parent's children's.
func (v *viewer) facets(p *who.Person, classroom bool) []string {
	pick := func(q *who.Person) string {
		if classroom {
			return q.Classroom
		}
		return q.Grade
	}
	out := []string{}
	if own := pick(p); own != "" && (p.IsStudent || p.IsStaff) {
		out = append(out, own)
	}
	if p.IsParent {
		for _, key := range v.directory.FamilyKeysOf(p.Email) {
			for _, kid := range v.directory.Families[key].KidEmails {
				if k := v.directory.Person(kid); k != nil && pick(k) != "" {
					out = append(out, pick(k))
				}
			}
		}
	}
	return out
}

var getPerson = tool{
	name:        "get_person",
	description: "One person in full from the directory, by email or by name: their card, their About Me, and each family they belong to with its members, address and phone as shared. Several people matching a name are all returned as cards to choose from.",
	words:       "Reading a directory page",
	properties: map[string]any{
		"email": str("The person's email address, when known."),
		"name":  str("Words of the person's name, when the address is not known."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Email, Name string }](input)
		if err != nil {
			return nil, err
		}
		people := v.findByEmailOrName(in.Email, in.Name)
		if len(people) == 0 {
			return nil, fmt.Errorf("nobody in the directory matches that")
		}
		if len(people) > 1 {
			cards := []card{}
			for _, p := range people {
				cards = append(cards, v.card(p))
			}
			sortedByName(cards, func(c card) string { return c.Name })
			return map[string]any{"several": cards}, nil
		}
		p := people[0]
		families := []familyCard{}
		for _, key := range v.directory.FamilyKeysOf(p.Email) {
			families = append(families, v.familyCard(key))
		}
		bands := []string{}
		for label, parents := range v.directory.RoomParents {
			if slices.Contains(parents, p.Email) {
				bands = append(bands, label)
			}
		}
		slices.Sort(bands)
		return map[string]any{"person": v.card(p), "aboutMe": p.Facts, "families": families, "roomParentFor": bands}, nil
	},
}

var getFamily = tool{
	name:        "get_family",
	description: "A family from the directory, found through any of its members by email or name: the family's name, address and phone as shared, and its adults and students.",
	words:       "Reading a family page",
	properties: map[string]any{
		"email": str("A member's email address, when known."),
		"name":  str("Words of a member's name or the family's name."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Email, Name string }](input)
		if err != nil {
			return nil, err
		}
		keys := v.familyKeys(in.Email, in.Name)
		if len(keys) == 0 {
			return nil, fmt.Errorf("no family in the directory matches that")
		}
		families := []familyCard{}
		for _, key := range keys {
			families = append(families, v.familyCard(key))
		}
		return map[string]any{"families": families}, nil
	},
}

var getClassroom = tool{
	name:        "get_classroom",
	description: "One classroom: its band and grades, its teachers, its crews with each crew's teachers and students, the rest of its students, and the room parents of its band. Without a name, every classroom in brief.",
	words:       "Looking at a classroom",
	properties: map[string]any{
		"classroom": str("The classroom's name."),
	},
	run: func(v *viewer, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Classroom string }](input)
		if err != nil {
			return nil, err
		}
		want := strings.TrimSpace(in.Classroom)
		if want == "" {
			out := []map[string]any{}
			for _, c := range v.calendar.Roster.Classrooms {
				out = append(out, map[string]any{"name": c.Name, "band": c.Band, "grades": c.Grades, "teachers": v.classroomTeachers(c.Name), "students": v.count(c.Name), "link": whoBase + who.ClassroomPath(c.Name)})
			}
			return map[string]any{"classrooms": out}, nil
		}
		i := slices.IndexFunc(v.calendar.Roster.Classrooms, func(c calendarClassroom) bool { return contains(c.Name, want) })
		if i < 0 {
			return nil, fmt.Errorf("there is no classroom called %q", want)
		}
		c := v.calendar.Roster.Classrooms[i]
		crews := []map[string]any{}
		inCrew := map[string]bool{}
		for _, crew := range v.directory.Crews {
			if crew.Classroom != c.Name || crew.Name == "" {
				continue
			}
			students := []card{}
			for j := range v.directory.People {
				p := &v.directory.People[j]
				if p.IsStudent && p.Classroom == c.Name && p.Crew == crew.Name {
					students = append(students, v.card(p))
					inCrew[p.Email] = true
				}
			}
			sortedByName(students, func(c card) string { return c.Name })
			crews = append(crews, map[string]any{"name": crew.Name, "teachers": v.names(crew.Teachers), "students": students})
		}
		others := []card{}
		for j := range v.directory.People {
			p := &v.directory.People[j]
			if p.IsStudent && p.Classroom == c.Name && !inCrew[p.Email] {
				others = append(others, v.card(p))
			}
		}
		sortedByName(others, func(c card) string { return c.Name })
		roomParents := []string{}
		for label, parents := range v.directory.RoomParents {
			if bandMatches(label, c.Grades) {
				roomParents = append(roomParents, v.names(parents)...)
			}
		}
		slices.Sort(roomParents)
		return map[string]any{
			"name": c.Name, "band": c.Band, "grades": c.Grades, "teachers": v.classroomTeachers(c.Name), "crews": crews, "students": others,
			"roomParents": roomParents, "link": whoBase + who.ClassroomPath(c.Name),
		}, nil
	},
}

func (v *viewer) count(classroom string) int {
	n := 0
	for i := range v.directory.People {
		if v.directory.People[i].IsStudent && v.directory.People[i].Classroom == classroom {
			n++
		}
	}
	return n
}

// bandMatches says whether a room parent band label - K, 1st/2nd, 3rd/4th
// - names any of the grades.
func bandMatches(label string, grades []string) bool {
	for _, g := range grades {
		if g == "Kindergarten" && strings.HasPrefix(label, "K") {
			return true
		}
		number := strings.TrimPrefix(g, "Grade ")
		if strings.Contains(label, number+"st") || strings.Contains(label, number+"nd") || strings.Contains(label, number+"rd") || strings.Contains(label, number+"th") {
			return true
		}
	}
	return false
}
