package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

type trigger struct {
	table string
	fire  func(m *Model, c Change) ([]Edit, error)
}

var triggers = []trigger{
	{table: "PERSON", fire: schoolGroups},
}

func unchecked(*Model, Change) error {
	return nil
}

func fire(s *Store, tx *store.Tx, c Change, where string) error {
	for pending := []Change{c}; len(pending) > 0; {
		next := pending[0]
		pending = pending[1:]
		for _, t := range triggers {
			if t.table != next.Table {
				continue
			}
			writes, err := t.fire(s.In(tx), next)
			if err != nil {
				return access.Invalid("%s: %v", where, err)
			}
			for _, w := range writes {
				_, c, err := stageWrite(s, tx, w, where, nil, true, unchecked)
				if err != nil {
					return err
				}
				pending = append(pending, c)
			}
		}
	}
	return nil
}

var (
	schoolKinds   = []string{"classroom", "crew", "grade", "band", "department"}
	schoolColumns = []string{"grade", "vc_grade", "classroom", "vc_classroom", "crew", "vc_crew", "department", "deactivated"}
)

func schoolGroups(m *Model, c Change) ([]Edit, error) {
	if c.New == nil {
		return nil, nil
	}
	if c.Old != nil && !slices.ContainsFunc(schoolColumns, func(col string) bool { return c.Old[col] != c.New[col] }) {
		return nil, nil
	}
	want, err := m.schoolGroupsOf(c.New)
	if err != nil {
		return nil, err
	}
	person := c.New["id"]
	groups := m.Table("GROUP")
	writes := []Edit{}
	for _, row := range m.Table("MEMBER").Referencing("person", person) {
		g, _ := groups.Get(row["group"])
		if !slices.Contains(schoolKinds, g["kind"]) {
			continue
		}
		if want[row["group"]] && row["role"] == "member" && row["status"] == "yes" {
			delete(want, row["group"])
			continue
		}
		writes = append(writes, Edit{Delete: row["id"]})
	}
	for _, group := range slices.Sorted(maps.Keys(want)) {
		writes = append(writes, Edit{Insert: "MEMBER", Row: map[string]any{"group": group, "person": person, "role": "member", "status": "yes"}})
	}
	return writes, nil
}

func (m *Model) schoolGroupsOf(p store.Row) (map[string]bool, error) {
	out := map[string]bool{}
	if p["deactivated"] != "" {
		return out, nil
	}
	effective := func(column string) string {
		if v := p[column]; v != "" {
			return v
		}
		return p["vc_"+column]
	}
	for _, column := range []string{"classroom", "crew", "department"} {
		if g := effective(column); g != "" {
			out[g] = true
		}
	}
	grade := effective("grade")
	if grade == "" {
		return out, nil
	}
	groups := m.Table("GROUP")
	slug := "grade-" + strings.ToLower(grade)
	i := slices.IndexFunc(groups.All(), func(g store.Row) bool { return g["kind"] == "grade" && strings.EqualFold(g["slug"], slug) })
	if i < 0 {
		return nil, fmt.Errorf("%s is in grade %s, and no grade group has the slug %s", p["id"], grade, slug)
	}
	g := groups.All()[i]
	out[g["id"]] = true
	if band, ok := groups.Get(g["parent"]); ok && band["kind"] == "band" {
		out[band["id"]] = true
	}
	return out, nil
}
