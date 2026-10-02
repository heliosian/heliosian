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
	mu      sync.Mutex
	byGroup map[string][]store.Row
	sets    map[string]*generatedSet
}

type generatedSet struct {
	rows []store.Row
	by   map[string]map[string][]store.Row
}

var effectiveStatuses = []string{"invited", "pending", "yes", "maybe", "no"}

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
	m.derived.mu.Lock()
	set := m.derived.sets[t.Name]
	m.derived.mu.Unlock()
	if set != nil {
		return set
	}
	var rows []store.Row
	switch t.Name {
	case "EFFECTIVE_MEMBER":
		rows = m.effectiveAll()
	case "INBOX":
		rows = m.inbox()
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

func (m *Model) inbox() []store.Row {
	out := []store.Row{}
	for _, doc := range m.Shown("DOCUMENT").All() {
		if !slices.Contains([]string{"newsletter", "list", "post"}, doc["kind"]) {
			continue
		}
		members := map[string]bool{}
		for _, link := range m.Shown("DOCUMENT_GROUP").Referencing("document", doc["id"]) {
			if link["relation"] != "sent_to" && link["relation"] != "for" {
				continue
			}
			for _, row := range m.effectiveRows(link["group"]) {
				members[row["person"]] = true
			}
		}
		for _, person := range slices.Sorted(maps.Keys(m.expand(members, "household"))) {
			out = append(out, store.Row{"id": Derive(InboxPrefix, person, doc["id"]), "person": person, "document": doc["id"]})
		}
	}
	return out
}

func (m *Model) effectiveOf(group string) []store.Row {
	reasons := m.resolve(group, map[string]bool{})
	statuses := map[string]string{}
	for _, row := range m.Shown("MEMBER").Referencing("group", group) {
		status := strings.ToLower(strings.TrimSpace(row["status"]))
		if row["role"] == "member" && slices.Contains(effectiveStatuses, status) {
			statuses[row["person"]] = status
		}
	}
	out := []store.Row{}
	for _, person := range slices.Sorted(maps.Keys(reasons)) {
		out = append(out, store.Row{"id": Derive(EffectiveMemberPrefix, group, person), "group": group, "person": person, "status": statuses[person], "reasons": strings.Join(reasons[person], ", ")})
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
		if row["role"] != "member" {
			continue
		}
		switch status := strings.ToLower(strings.TrimSpace(row["status"])); {
		case status == "excluded":
			excluded[row["person"]] = true
		case status != "" && status != "cancelled":
			out[row["person"]] = append(out[row["person"]], "member")
		}
	}
	rules := m.Shown("RULE").Referencing("group", group)
	slices.SortStableFunc(rules, func(a, b store.Row) int { return store.CompareKeys(a["order"], b["order"]) })
	for _, rule := range rules {
		selected := m.selectRule(rule, stack)
		for person := range selected {
			if strings.EqualFold(rule["kind"], "exclude") {
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
		groups := []string{target}
		if descend, _ := cells.YesNo(rule["descend"], false); descend {
			groups = append(groups, m.descendants(target)...)
		}
		for _, g := range groups {
			for person := range m.resolve(g, stack) {
				s[person] = true
			}
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
	picked = m.expand(picked, strings.ToLower(strings.TrimSpace(rule["expand"])))
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

func (m *Model) descendants(group string) []string {
	out := []string{}
	seen := map[string]bool{group: true}
	queue := []string{group}
	for len(queue) > 0 {
		g := queue[0]
		queue = queue[1:]
		for _, child := range m.Shown("GROUP").Referencing("parent", g) {
			if seen[child["id"]] {
				continue
			}
			seen[child["id"]] = true
			out = append(out, child["id"])
			queue = append(queue, child["id"])
		}
	}
	return out
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

func (m *Model) familyRows(person, role string) []string {
	out := []string{}
	for _, row := range m.Shown("MEMBER").Referencing("person", person) {
		if row["role"] != role {
			continue
		}
		if g, ok := m.Shown("GROUP").Get(row["group"]); ok && g["kind"] == "family" {
			out = append(out, row["group"])
		}
	}
	return out
}

func (m *Model) familyPeople(family, role string) []string {
	out := []string{}
	for _, row := range m.Shown("MEMBER").Referencing("group", family) {
		if row["role"] == role {
			out = append(out, row["person"])
		}
	}
	return out
}

func (m *Model) expand(people map[string]bool, how string) map[string]bool {
	if how == "" || how == "self" {
		return people
	}
	out := map[string]bool{}
	for person := range people {
		switch how {
		case "parents":
			led := m.familyRows(person, "lead")
			for _, f := range m.familyRows(person, "member") {
				if slices.Contains(led, f) {
					continue
				}
				for _, p := range m.familyPeople(f, "lead") {
					out[p] = true
				}
			}
		case "children":
			for _, f := range m.familyRows(person, "lead") {
				leads := m.familyPeople(f, "lead")
				for _, p := range m.familyPeople(f, "member") {
					if !slices.Contains(leads, p) {
						out[p] = true
					}
				}
			}
		case "household":
			out[person] = true
			for _, f := range m.familyRows(person, "member") {
				for _, p := range m.familyPeople(f, "member") {
					out[p] = true
				}
			}
		}
	}
	return out
}
