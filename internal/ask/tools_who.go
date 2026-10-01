package ask

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"heliosian/internal/model"
)

const (
	personIncludes = "parents,children.grade,grade,classroom.teachers,crew.teachers"
	personFields   = "fullName,email,pronouns,isStudent,isParent,isStaff,isNew,jobTitle,department,phone,grade.name,classroom.name,classroom.teachers.fullName,crew.name,crew.teachers.fullName,parents.fullName,children.fullName,children.grade.name,link"
	familyIncludes = "adults,kids.grade,kids.classroom"
	familyFields   = "name,address,phone,photoCaption,link,adults.fullName,adults.email,adults.phone,kids.fullName,kids.grade.name,kids.classroom.name"
)

var findPeople = tool{
	name:        "find_people",
	description: "Search the school directory (Helios Who?) for people: students, parents and staff. Each query is searched on its own, so every person a document names is found in one call. Every filter narrows every query; a parent matches a grade or classroom through their children. Returns, for each query, at most a page of people with what places them, their contact details as shared, and a link to each one's page; more says the page was full.",
	words:       "Looking in the directory",
	properties: map[string]any{
		"queries":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Words to find in a name, an email or a job title, one entry per person or search. Leave it out to list everyone the filters pick."},
		"role":       map[string]any{"type": "string", "enum": []string{"student", "parent", "staff"}, "description": "Only people with this role."},
		"grade":      str("A grade as the school names it, such as Grade 3 or Kindergarten."),
		"classroom":  str("A classroom name."),
		"department": str("A staff department, for staff."),
		"limit":      integer("How many to return, 25 unless said, 50 at most."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
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
		asked := []query{}
		for i, words := range queries {
			v := params("q", words, "role", in.Role, "grade", in.Grade, "classroom", in.Classroom, "department", in.Department, "include", personIncludes, "limit", shown(limit))
			asked = append(asked, query{name: "q" + strconv.Itoa(i), path: collection("people", v), fields: fields(personFields), limit: limit})
		}
		out, err := t.ask(asked...)
		if err != nil {
			return nil, err
		}
		results := []map[string]any{}
		for i, words := range queries {
			name := "q" + strconv.Itoa(i)
			results = append(results, map[string]any{"query": words, "people": out[name], "more": out[name+"More"]})
		}
		return map[string]any{"results": results}, nil
	},
}

func (t *turn) peopleNamed(email, name string) ([]string, error) {
	if email != "" {
		return []string{email}, nil
	}
	if name == "" {
		return nil, fmt.Errorf("say whose")
	}
	env, err := t.read(map[string]string{"people": collection("people", params("q", name, "limit", "11"))})
	if err != nil {
		return nil, err
	}
	return env.Result.(map[string]any)["people"].([]string), nil
}

var getPerson = tool{
	name:        "get_person",
	description: "One person in full from the directory, by email or by name: who they are and what places them, their About Me, each family they belong to with its members, address and phone as shared, and the grades they are a room parent for. Several people matching a name are all returned in brief to choose from.",
	words:       "Reading a directory page",
	properties: map[string]any{
		"email": str("The person's email address, when known."),
		"name":  str("Words of the person's name, when the address is not known."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Email, Name string }](input)
		if err != nil {
			return nil, err
		}
		people, err := t.peopleNamed(in.Email, in.Name)
		if err != nil {
			return nil, err
		}
		switch len(people) {
		case 0:
			return nil, fmt.Errorf("nobody in the directory matches that")
		case 1:
			v := params("include", personIncludes+","+under("families", familyIncludes)+",room-parent-for")
			spec := personFields + ",facts,room-parent-for.name," + under("families", familyFields)
			return t.ask(query{name: "person", path: one("people", people[0], v), fields: fields(spec)})
		}
		v := params("q", in.Name, "include", personIncludes, "limit", "11")
		return t.ask(query{name: "several", path: collection("people", v), fields: fields(personFields), limit: 10})
	},
}

func under(relation, spec string) string {
	parts := strings.Split(spec, ",")
	for i := range parts {
		parts[i] = relation + "." + parts[i]
	}
	return strings.Join(parts, ",")
}

var getFamily = tool{
	name:        "get_family",
	description: "A family from the directory, found through any of its members by email, or by words of a member's or the family's name: the family's name, address and phone as shared, and its adults and students.",
	words:       "Reading a family page",
	properties: map[string]any{
		"email": str("A member's email address, when known."),
		"name":  str("Words of a member's name or the family's name."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Email, Name string }](input)
		if err != nil {
			return nil, err
		}
		var out map[string]any
		var families any
		if in.Email != "" {
			v := params("include", under("families", familyIncludes))
			out, err = t.ask(query{name: "person", path: one("people", in.Email, v), fields: fields(under("families", familyFields))})
			if err != nil {
				return nil, err
			}
			families = out["person"].(map[string]any)["families"]
		} else {
			out, err = t.ask(query{name: "families", path: collection("families", params("q", in.Name, "include", familyIncludes, "limit", "11")), fields: fields(familyFields), limit: 10})
			if err != nil {
				return nil, err
			}
			families = out["families"]
		}
		if empty(families) {
			return nil, fmt.Errorf("no family in the directory matches that")
		}
		return map[string]any{"families": families, "more": out["familiesMore"]}, nil
	},
}

var getClassroom = tool{
	name:        "get_classroom",
	description: "One classroom: its band and grades, its teachers, the room parents of its band, its crews with each crew's teachers, and its students with their crews. Without a name, every classroom in brief.",
	words:       "Looking at a classroom",
	properties: map[string]any{
		"classroom": str("The classroom's name."),
	},
	run: func(t *turn, input json.RawMessage) (any, error) {
		in, err := decodeInput[struct{ Classroom string }](input)
		if err != nil {
			return nil, err
		}
		if in.Classroom == "" {
			return t.ask(query{name: "classrooms", path: collection("classrooms", params("include", "teachers")), fields: fields("name,band,grades,link,teachers.fullName")})
		}
		v := params("include", "teachers,room-parents,crews.teachers,students.crew")
		spec := "name,band,grades,link,teachers.fullName,room-parents.fullName,crews.name,crews.teachers.fullName,students.fullName,students.crew.name"
		return t.ask(query{name: "classroom", path: one("classrooms", model.ClassroomSlug(in.Classroom), v), fields: fields(spec)})
	},
}
