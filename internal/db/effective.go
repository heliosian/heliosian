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
	byGroup map[string]effectiveEntry
	sets    map[string]*generatedSet
}

type effectiveEntry struct {
	rows  []store.Row
	reads reads
}

type readKind byte

const (
	readGroup readKind = iota
	readPerson
	readPeople
	readHouseholds
)

type readKey struct {
	kind readKind
	id   string
}

type reads map[readKey]bool

func newDerived() *derived {
	return &derived{byGroup: map[string]effectiveEntry{}, sets: map[string]*generatedSet{}}
}

var generatedInputs = map[string][]string{"BIRTHDAY": {"PERSON", "GROUP", "SETTING", "BIRTHDAY_YEAR"}}

func (d *derived) carry(was, is map[string]*View) *derived {
	if d == nil || was == nil {
		return newDerived()
	}
	changed := changedReads(was, is)
	stale := map[string]bool{}
	for name, tables := range generatedInputs {
		stale[name] = slices.ContainsFunc(tables, func(t string) bool { return viewChanged(was[t], is[t]) })
	}
	if len(changed) == 0 && !slices.Contains(slices.Collect(maps.Values(stale)), true) {
		return d
	}
	keys := slices.Collect(maps.Keys(changed))
	out := newDerived()
	d.mu.Lock()
	defer d.mu.Unlock()
	for group, e := range d.byGroup {
		if !slices.ContainsFunc(keys, func(k readKey) bool { return e.reads[k] }) {
			out.byGroup[group] = e
		}
	}
	for name, set := range d.sets {
		if !stale[name] && (name != "EFFECTIVE_MEMBER" || len(changed) == 0) {
			out.sets[name] = set
		}
	}
	return out
}

func viewChanged(a, b *View) bool {
	changed := false
	viewChanges(a, b, func(store.Row, store.Row) { changed = true })
	return changed
}

func changedReads(was, is map[string]*View) reads {
	out := reads{}
	household := func(id string) bool {
		for _, v := range []*View{was["GROUP"], is["GROUP"]} {
			if i, ok := v.rows.byID[id]; ok {
				g := v.rows.rows[i]
				if g["kind"] == "family" || g["kind"] == "group" && g["slug"] == "students" {
					return true
				}
			}
		}
		return false
	}
	each := func(table string, row store.Row) {
		switch table {
		case "MEMBER":
			out[readKey{readGroup, row["group"]}] = true
			if household(row["group"]) {
				out[readKey{kind: readHouseholds}] = true
			}
		case "RULE":
			out[readKey{readGroup, row["group"]}] = true
		case "GROUP":
			out[readKey{readGroup, row["id"]}] = true
			out[readKey{readGroup, row["parent"]}] = true
			if household(row["id"]) {
				out[readKey{kind: readHouseholds}] = true
			}
		case "PERSON":
			out[readKey{readPerson, row["id"]}] = true
			out[readKey{kind: readPeople}] = true
		case "PERSON_EMAIL":
			out[readKey{kind: readPeople}] = true
		}
	}
	for _, table := range []string{"MEMBER", "RULE", "GROUP", "PERSON", "PERSON_EMAIL"} {
		viewChanges(was[table], is[table], func(old, new store.Row) {
			for _, row := range []store.Row{old, new} {
				if row != nil {
					each(table, row)
				}
			}
		})
	}
	return out
}

func viewChanges(a, b *View, each func(old, new store.Row)) {
	if a == b {
		return
	}
	if a.rows == b.rows {
		for i, row := range a.rows.rows {
			if (a.shown[i] == nil) != (b.shown[i] == nil) {
				each(row, row)
			}
		}
		return
	}
	for i, row := range b.rows.rows {
		j, ok := a.rows.byID[row["id"]]
		if !ok {
			each(nil, row)
			continue
		}
		if (a.shown[j] == nil) != (b.shown[i] == nil) || !maps.Equal(a.rows.rows[j], row) {
			each(a.rows.rows[j], row)
		}
	}
	for _, row := range a.rows.rows {
		if _, ok := b.rows.byID[row["id"]]; !ok {
			each(row, nil)
		}
	}
}

type generatedSet struct {
	once    sync.Once
	version int
	rows    []store.Row
	by      map[string]map[string][]store.Row
}

func memberAs(row store.Row) string {
	return strings.ToLower(strings.TrimSpace(row["member"]))
}

func (m *Model) directMemberCount(group string) int {
	count := 0
	for _, row := range m.Table("MEMBER").Referencing("group", group) {
		if memberAs(row) == "yes" {
			count++
		}
	}
	return count
}

func (m *Model) effectiveRows(group string) []store.Row {
	m.derived.mu.Lock()
	e, ok := m.derived.byGroup[group]
	m.derived.mu.Unlock()
	if ok {
		return e.rows
	}
	r := reads{}
	rows := m.effectiveOf(group, r)
	m.derived.mu.Lock()
	m.derived.byGroup[group] = effectiveEntry{rows: rows, reads: r}
	m.derived.mu.Unlock()
	return rows
}

func (m *Model) generated(t *Table) *generatedSet {
	version := m.index.current()
	m.derived.mu.Lock()
	set := m.derived.sets[t.Name]
	if set == nil || (t.Name == "SEARCH" && set.version != version) {
		set = &generatedSet{version: version}
		m.derived.sets[t.Name] = set
	}
	m.derived.mu.Unlock()
	set.once.Do(func() { set.build(m, t) })
	return set
}

func StartWarmer(s *Store) {
	in := s.Model().services.inbox(warmInbox)
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	go func() {
		for range in.wake {
			in.take()
			s.Model().generated(effective)
		}
	}()
	in.wake.poke()
}

func (m *Model) searchRow(id string) (store.Row, bool) {
	t, _ := Lookup("SEARCH")
	c, _ := t.Column("id")
	rows := m.generated(t).by["id"][indexKey(c, id)]
	if len(rows) == 0 {
		return nil, false
	}
	return rows[0], true
}

func (set *generatedSet) build(m *Model, t *Table) {
	switch t.Name {
	case "EFFECTIVE_MEMBER":
		set.rows = m.effectiveAll()
	case "BIRTHDAY":
		set.rows = m.birthdayRows()
	case "SEARCH":
		set.rows, _ = m.index.generatedRows()
	default:
		panic("db: no builder for " + t.Name)
	}
	set.by = map[string]map[string][]store.Row{}
	for _, c := range t.Columns {
		if c.Kind != ID && c.Kind != Ref && c.Kind != Enum {
			continue
		}
		index := map[string][]store.Row{}
		for _, row := range set.rows {
			key := indexKey(c, row[c.Name])
			index[key] = append(index[key], row)
		}
		set.by[c.Name] = index
	}
}

func (m *Model) effectiveAll() []store.Row {
	out := []store.Row{}
	for _, g := range m.Shown("GROUP").All() {
		out = append(out, m.effectiveRows(g["id"])...)
	}
	return out
}

func (m *Model) effectiveOf(group string, r reads) []store.Row {
	reasons := m.resolve(group, r)
	out := []store.Row{}
	for _, person := range slices.Sorted(maps.Keys(reasons)) {
		out = append(out, store.Row{"id": Derive(EffectiveMemberPrefix, group, person), "group": group, "person": person, "reasons": strings.Join(reasons[person], ", ")})
	}
	return out
}

func (m *Model) resolve(group string, r reads) map[string][]string {
	out := map[string][]string{}
	r[readKey{readGroup, group}] = true
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
		selected := m.selectRule(group, rule, r)
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
		r[readKey{readPerson, person}] = true
		row, ok := people.Get(person)
		if excluded[person] || !ok || strings.TrimSpace(row["deactivated"]) != "" {
			delete(out, person)
		}
	}
	return out
}

func (m *Model) selectRule(group string, rule store.Row, r reads) map[string]bool {
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
		if target != group {
			for person := range m.resolve(target, r) {
				s[person] = true
			}
		}
		if under, _ := cells.YesNo(rule["under"], false); under {
			r[readKey{readGroup, target}] = true
			for _, g := range m.groupsUnder(target) {
				r[readKey{readGroup, g}] = true
				for person := range m.resolve(g, r) {
					s[person] = true
				}
			}
		}
		narrow(s)
	}
	if person := rule["person"]; person != "" {
		narrow(map[string]bool{person: true})
	}
	// A guest matches a name, address or column only when target, person or within already
	// names them. Otherwise a text rule lists outside people its author was not allowed to see.
	textGuest := map[string]bool{}
	if strings.TrimSpace(rule["search"]) != "" || strings.TrimSpace(rule["property"]) != "" {
		if rule["target"] != "" || rule["person"] != "" {
			for person := range picked {
				textGuest[person] = true
			}
		}
		if within := rule["within"]; within != "" {
			for person := range m.resolve(within, r) {
				textGuest[person] = true
			}
		}
	}
	listed := func(row store.Row) bool {
		return row["source"] != "guest" || textGuest[row["id"]]
	}
	if search := strings.ToLower(strings.TrimSpace(rule["search"])); search != "" {
		r[readKey{kind: readPeople}] = true
		found := m.search(search)
		for person := range found {
			row, ok := m.Shown("PERSON").Get(person)
			if !ok || !listed(row) {
				delete(found, person)
			}
		}
		narrow(found)
	}
	if property := strings.TrimSpace(rule["property"]); property != "" {
		r[readKey{kind: readPeople}] = true
		want := strings.TrimSpace(rule["value"])
		s := map[string]bool{}
		for _, row := range m.Shown("PERSON").All() {
			if listed(row) && strings.EqualFold(strings.TrimSpace(row[property]), want) {
				s[row["id"]] = true
			}
		}
		narrow(s)
	}
	if picked == nil {
		return map[string]bool{}
	}
	if how := strings.ToLower(strings.TrimSpace(rule["replace_with"])); how != "" {
		r[readKey{kind: readHouseholds}] = true
		picked = m.replaceWith(picked, how)
	}
	if within := rule["within"]; within != "" {
		in := m.resolve(within, r)
		for person := range picked {
			if _, ok := in[person]; !ok {
				delete(picked, person)
			}
		}
	}
	return picked
}

var servingColumns = []string{"managed_by", "waitlist", "rsvp_yes", "rsvp_no", "unsubscribed", "chair_offers", "leads", "audience", "hidden_for"}

func (m *Model) groupsUnder(group string) []string {
	groups := m.Shown("GROUP")
	out := []string{}
	queue := []string{group}
	for len(queue) > 0 {
		g, ok := groups.Get(queue[0])
		queue = queue[1:]
		if !ok {
			continue
		}
		for _, child := range groups.Referencing("parent", g["id"]) {
			if slices.ContainsFunc(servingColumns, func(c string) bool { return g[c] == child["id"] }) {
				continue
			}
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

type relatives struct {
	student  func(person string) bool
	families func(person string) []string
	people   func(family string) []string
}

func (m *Model) replaceWith(people map[string]bool, how string) map[string]bool {
	students := m.students()
	return relatives{student: func(p string) bool { return students[p] }, families: m.familiesOf, people: m.familyPeople}.replace(people, how)
}

func (rel relatives) replace(people map[string]bool, how string) map[string]bool {
	if how == "" {
		return people
	}
	out := map[string]bool{}
	for person := range people {
		if how == "household" {
			out[person] = true
		}
		if how == "parents" && !rel.student(person) || how == "children" && rel.student(person) {
			continue
		}
		for _, f := range rel.families(person) {
			for _, p := range rel.people(f) {
				if how == "household" || how == "parents" && !rel.student(p) || how == "children" && rel.student(p) {
					out[p] = true
				}
			}
		}
	}
	return out
}
