package db

import (
	"maps"
	"slices"
	"strings"
	"sync"

	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type derived struct {
	mu            sync.Mutex
	byGroup       map[string][]store.Row
	sets          map[string]*generatedSet
	searchVersion int
}

type generatedSet struct {
	rows []store.Row
	by   map[string]map[string][]store.Row
}

func memberAs(row store.Row) string {
	return strings.ToLower(strings.TrimSpace(row["member"]))
}

func (m *Model) effectiveRows(group string) []store.Row {
	m.derived.mu.Lock()
	rows, ok := m.derived.byGroup[group]
	m.derived.mu.Unlock()
	if ok {
		return rows
	}
	rows = m.effectiveOf(group)
	m.derived.mu.Lock()
	m.derived.byGroup[group] = rows
	m.derived.mu.Unlock()
	return rows
}

func (m *Model) generated(t *Table) *generatedSet {
	version := m.index.current()
	m.derived.mu.Lock()
	set := m.derived.sets[t.Name]
	stale := t.Name == "SEARCH" && m.derived.searchVersion != version
	m.derived.mu.Unlock()
	if set != nil && !stale {
		return set
	}
	var rows []store.Row
	switch t.Name {
	case "EFFECTIVE_MEMBER":
		rows = m.effectiveAll()
	case "INBOX":
		rows = []store.Row{}
	case "SEARCH":
		rows, version = m.index.generatedRows()
		m.derived.mu.Lock()
		m.derived.searchVersion = version
		m.derived.mu.Unlock()
	default:
		panic("db: no builder for " + t.Name)
	}
	set = &generatedSet{rows: rows, by: map[string]map[string][]store.Row{}}
	for _, c := range t.Columns {
		if c.Kind != ID && c.Kind != Ref {
			continue
		}
		index := map[string][]store.Row{}
		for _, row := range rows {
			index[row[c.Name]] = append(index[row[c.Name]], row)
		}
		set.by[c.Name] = index
	}
	m.derived.mu.Lock()
	m.derived.sets[t.Name] = set
	m.derived.mu.Unlock()
	return set
}

func (m *Model) effectiveAll() []store.Row {
	out := []store.Row{}
	for _, g := range m.Shown("GROUP").All() {
		out = append(out, m.effectiveRows(g["id"])...)
	}
	return out
}

func (m *Model) effectiveOf(group string) []store.Row {
	reasons := m.resolve(group, map[string]bool{})
	out := []store.Row{}
	for _, person := range slices.Sorted(maps.Keys(reasons)) {
		out = append(out, store.Row{"id": Derive(EffectiveMemberPrefix, group, person), "group": group, "person": person, "reasons": strings.Join(reasons[person], ", ")})
	}
	return out
}

func (m *Model) resolve(group string, stack map[string]bool) map[string][]string {
	out := map[string][]string{}
	if stack[group] {
		return out
	}
	stack[group] = true
	defer delete(stack, group)
	excluded := map[string]bool{}
	for _, row := range m.Shown("MEMBER").Referencing("group", group) {
		switch memberAs(row) {
		case "excluded":
			excluded[row["person"]] = true
		case "yes":
			out[row["person"]] = append(out[row["person"]], "member")
		}
	}
	rules := m.Shown("RULE").Referencing("group", group)
	slices.SortStableFunc(rules, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
	for _, rule := range rules {
		selected := m.selectRule(rule, stack)
		for person := range selected {
			if exclude, _ := cells.YesNo(rule["exclude"], false); exclude {
				excluded[person] = true
				continue
			}
			out[person] = append(out[person], "rule "+rule["order"])
		}
	}
	people := m.Shown("PERSON")
	for person := range out {
		row, ok := people.Get(person)
		if excluded[person] || !ok || strings.TrimSpace(row["deactivated"]) != "" {
			delete(out, person)
		}
	}
	return out
}

func (m *Model) selectRule(rule store.Row, stack map[string]bool) map[string]bool {
	var picked map[string]bool
	narrow := func(s map[string]bool) {
		if picked == nil {
			picked = s
			return
		}
		for person := range picked {
			if !s[person] {
				delete(picked, person)
			}
		}
	}
	if target := rule["target"]; target != "" {
		s := map[string]bool{}
		for person := range m.resolve(target, stack) {
			s[person] = true
		}
		narrow(s)
	}
	if person := rule["person"]; person != "" {
		narrow(map[string]bool{person: true})
	}
	if search := strings.ToLower(strings.TrimSpace(rule["search"])); search != "" {
		narrow(m.search(search))
	}
	if property := strings.TrimSpace(rule["property"]); property != "" {
		want := strings.TrimSpace(rule["value"])
		s := map[string]bool{}
		for _, row := range m.Shown("PERSON").All() {
			if strings.EqualFold(strings.TrimSpace(row[property]), want) {
				s[row["id"]] = true
			}
		}
		narrow(s)
	}
	if picked == nil {
		return map[string]bool{}
	}
	picked = m.replaceWith(picked, strings.ToLower(strings.TrimSpace(rule["replace_with"])))
	if within := rule["within"]; within != "" {
		in := m.resolve(within, stack)
		for person := range picked {
			if _, ok := in[person]; !ok {
				delete(picked, person)
			}
		}
	}
	return picked
}

func (m *Model) search(text string) map[string]bool {
	out := map[string]bool{}
	for _, row := range m.Shown("PERSON").All() {
		if strings.Contains(strings.ToLower(row["name_show"]), text) {
			out[row["id"]] = true
		}
	}
	for _, row := range m.Shown("PERSON_EMAIL").All() {
		if strings.Contains(strings.ToLower(row["address"]), text) {
			out[row["person"]] = true
		}
	}
	return out
}

func (m *Model) students() map[string]bool {
	out := map[string]bool{}
	for _, g := range m.Table("GROUP").All() {
		if g["kind"] != "group" || g["slug"] != "students" {
			continue
		}
		for _, row := range m.Table("MEMBER").Referencing("group", g["id"]) {
			if memberAs(row) == "yes" {
				out[row["person"]] = true
			}
		}
	}
	return out
}

func (m *Model) familiesOf(person string) []string {
	out := []string{}
	for _, row := range m.Shown("MEMBER").Referencing("person", person) {
		if memberAs(row) != "yes" {
			continue
		}
		if g, ok := m.Shown("GROUP").Get(row["group"]); ok && g["kind"] == "family" {
			out = append(out, row["group"])
		}
	}
	return out
}

func (m *Model) familyPeople(family string) []string {
	out := []string{}
	for _, row := range m.Shown("MEMBER").Referencing("group", family) {
		if memberAs(row) == "yes" {
			out = append(out, row["person"])
		}
	}
	return out
}

func (m *Model) replaceWith(people map[string]bool, how string) map[string]bool {
	if how == "" {
		return people
	}
	students := m.students()
	out := map[string]bool{}
	for person := range people {
		if how == "household" {
			out[person] = true
		}
		if how == "parents" && !students[person] || how == "children" && students[person] {
			continue
		}
		for _, f := range m.familiesOf(person) {
			for _, p := range m.familyPeople(f) {
				if how == "household" || how == "parents" && !students[p] || how == "children" && students[p] {
					out[p] = true
				}
			}
		}
	}
	return out
}
