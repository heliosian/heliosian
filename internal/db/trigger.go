package db

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type trigger struct {
	table string
	fire  func(m *Model, c Change) ([]Edit, error)
}

var triggers = []trigger{
	{table: "PERSON", fire: schoolGroups},
	{table: "PERSON", fire: personFamilyTitles},
	{table: "MEMBER", fire: memberFamilyTitle},
	{table: "PHOTO", fire: photoNotReady},
}

func personFamilyTitles(m *Model, c Change) ([]Edit, error) {
	p := c.New
	if p == nil {
		p = c.Old
	}
	families := []string{}
	for _, row := range m.Table("MEMBER").Referencing("person", p["id"]) {
		families = append(families, row["group"])
	}
	return m.retitleFamilies(families), nil
}

func memberFamilyTitle(m *Model, c Change) ([]Edit, error) {
	families := []string{}
	for _, row := range []store.Row{c.Old, c.New} {
		if row != nil {
			families = append(families, row["group"])
		}
	}
	return m.retitleFamilies(families), nil
}

func (m *Model) retitleFamilies(groups []string) []Edit {
	slices.Sort(groups)
	writes := []Edit{}
	for _, id := range slices.Compact(groups) {
		g, ok := m.Table("GROUP").Get(id)
		if !ok || g["kind"] != "family" {
			continue
		}
		if title := m.familyTitle(id); title != g["title"] {
			writes = append(writes, Edit{Set: id, Cells: map[string]any{"title": title}})
		}
	}
	return writes
}

func (m *Model) familyTitle(group string) string {
	rows := m.Table("MEMBER").Referencing("group", group)
	managers := map[string]bool{}
	for _, row := range rows {
		if row["role"] == "manager" {
			managers[row["person"]] = true
		}
	}
	order := []string{}
	for _, row := range rows {
		if row["role"] == "member" && !managers[row["person"]] {
			order = append(order, row["person"])
		}
	}
	for _, row := range rows {
		if row["role"] == "manager" {
			order = append(order, row["person"])
		}
	}
	found := []string{}
	for _, id := range order {
		p, ok := m.Table("PERSON").Get(id)
		if !ok || !named(p) {
			continue
		}
		fields := strings.Fields(p["name_long"])
		if len(fields) == 0 {
			continue
		}
		s := fields[len(fields)-1]
		if !slices.ContainsFunc(found, func(n string) bool { return strings.EqualFold(n, s) }) {
			found = append(found, s)
		}
	}
	kept := []string{}
	for _, s := range found {
		within := slices.ContainsFunc(found, func(other string) bool {
			return !strings.EqualFold(other, s) && slices.ContainsFunc(strings.Split(other, "-"), func(part string) bool { return strings.EqualFold(part, s) })
		})
		if !within {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " & ") + " Family"
}

func named(p store.Row) bool {
	hidden, _ := cells.YesNo(p["hidden"], false)
	return !hidden && p["deactivated"] == "" && strings.EqualFold(p["consent"], "listed")
}

func photoNotReady(_ *Model, c Change) ([]Edit, error) {
	if c.Old == nil || c.New == nil || c.New["ready"] == "" {
		return nil, nil
	}
	if !slices.ContainsFunc(photoInputs, func(col string) bool { return c.Old[col] != c.New[col] }) {
		return nil, nil
	}
	return []Edit{{Set: c.New["id"], Cells: map[string]any{"ready": ""}}}, nil
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
	schoolColumns = []string{"grade", "vc_grade", "classroom", "vc_classroom", "crew", "vc_crew", "department", "vc_department", "deactivated"}
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
	out[groups.All()[i]["id"]] = true
	return out, nil
}
