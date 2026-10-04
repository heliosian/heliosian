package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/store"
)

type row = map[string]string

type table struct {
	order []string
	rows  map[string]row
}

func (t *table) add(id string, r row) {
	t.order = append(t.order, id)
	t.rows[id] = r
}

type state struct {
	now     string
	people  table
	emails  table
	photos  table
	groups  table
	members table
	rules   table
}

type write struct {
	Insert string `json:"insert,omitempty"`
	As     string `json:"as,omitempty"`
	Row    row    `json:"row,omitempty"`
	Set    string `json:"set,omitempty"`
	Cells  row    `json:"cells,omitempty"`
	Delete string `json:"delete,omitempty"`
}

type identity struct {
	name    string
	entries []int
	roles   map[role]bool
	emails  []string
	person  string
	isNew   bool
	cells   row
}

type household struct {
	adults  []*identity
	kids    []*identity
	address string
	phone   string
	family  string
}

var grades = []struct{ code, title, band string }{
	{"K", "Kindergarten", "Hummingbirds"},
	{"1", "Grade 1", "Halcons"},
	{"2", "Grade 2", "Halcons"},
	{"3", "Grade 3", "Jayvens"},
	{"4", "Grade 4", "Jayvens"},
	{"5", "Grade 5", "Cospreys"},
	{"6", "Grade 6", "Cospreys"},
	{"7", "Grade 7", "Hegrets"},
	{"8", "Grade 8", "Hegrets"},
}

var roleGroups = []struct {
	slug, title string
	holds       func(*identity) bool
}{
	{"students", "Students", func(id *identity) bool { return id.roles[student] }},
	{"parents", "Parents", func(id *identity) bool { return id.roles[parent] }},
	{"staff", "Staff", func(id *identity) bool { return id.roles[staff] }},
	{"adults", "Adults", func(id *identity) bool { return id.roles[parent] || id.roles[staff] }},
}

var personColumns = []string{
	"vc_name", "vc_legal_name", "vc_name_long", "vc_name_short", "vc_name_sort", "vc_grade", "vc_classroom", "vc_crew",
	"vc_department", "vc_job_title", "vc_phone", "vc_phone_visibility", "vc_address_visibility", "vc_bio", "deactivated",
}

var familyColumns = []string{"vc_address", "vc_phone"}

type planner struct {
	x       *export
	st      *state
	ids     []*identity
	of      []int
	claimed map[string]*identity
	groups  table
	counts  map[string]int
	named   int
	batch   []write
	web     map[int]websiteRow

	classroomBands map[string]string
	everyone       string

	groupWrites, personWrites, emailWrites, familyWrites []write
	memberDeletes, memberInserts, memberSets             []write
}

func plan(x *export, st *state) (*planner, error) {
	p := &planner{x: x, st: st, claimed: map[string]*identity{}, counts: map[string]int{}, groups: table{rows: map[string]row{}}, web: map[int]websiteRow{}, classroomBands: map[string]string{}}
	for _, id := range st.groups.order {
		p.groups.add(id, st.groups.rows[id])
	}
	if err := p.identify(); err != nil {
		return nil, err
	}
	if err := p.match(); err != nil {
		return nil, err
	}
	if err := p.website(); err != nil {
		return nil, err
	}
	p.ensureEveryone()
	p.ensureGrades()
	households, err := p.households()
	if err != nil {
		return nil, err
	}
	if err := p.people(households); err != nil {
		return nil, err
	}
	p.bandRules()
	for _, id := range p.ids {
		if err := p.emails(id); err != nil {
			return nil, err
		}
	}
	p.families(households)
	p.roles()
	p.batch = slices.Concat(p.groupWrites, p.personWrites, p.emailWrites, p.familyWrites, p.memberDeletes, p.memberInserts, p.memberSets)
	return p, nil
}

type portrait struct {
	person  string
	name    string
	content []byte
}

func (p *planner) portraits() ([]portrait, error) {
	held := map[string]bool{}
	for _, pid := range p.st.photos.order {
		r := p.st.photos.rows[pid]
		name := path.Base(r["original"])
		held[r["person"]+"/"+strings.TrimSuffix(name, path.Ext(name))] = true
	}
	out := []portrait{}
	for _, id := range p.ids {
		files := []string{}
		for _, i := range id.entries {
			for _, f := range slices.Concat(p.x.entries[i].photos, p.web[i].photos) {
				if !slices.Contains(files, f) {
					files = append(files, f)
				}
			}
		}
		for _, f := range slices.Backward(files) {
			content, err := os.ReadFile(filepath.Join(p.x.photoDir, f))
			if err != nil {
				return nil, fmt.Errorf("%s's photo: %w", id.name, err)
			}
			sum := sha256.Sum256(content)
			hash := hex.EncodeToString(sum[:])
			if held[id.person+"/"+hash] {
				continue
			}
			held[id.person+"/"+hash] = true
			out = append(out, portrait{person: id.person, name: f, content: content})
		}
	}
	return out, nil
}

func (p *planner) identify() error {
	up := make([]int, len(p.x.entries))
	for i := range up {
		up[i] = i
	}
	find := func(i int) int {
		for up[i] != i {
			up[i] = up[up[i]]
			i = up[i]
		}
		return i
	}
	union := func(a, b int) {
		a, b = find(a), find(b)
		if a > b {
			a, b = b, a
		}
		up[b] = a
	}
	byEmail := map[string]int{}
	byName := map[string][]int{}
	for i, e := range p.x.entries {
		for _, a := range e.emails {
			if j, ok := byEmail[a]; ok {
				union(i, j)
				continue
			}
			byEmail[a] = i
		}
		key := string(e.role) + "\x00" + resolved(e.name)
		byName[key] = append(byName[key], i)
	}
	for _, group := range byName {
		anchor := group[0]
		if i := slices.IndexFunc(group, func(i int) bool { return len(p.x.entries[i].emails) > 0 }); i >= 0 {
			anchor = group[i]
		}
		for _, i := range group {
			if len(p.x.entries[i].emails) == 0 {
				union(anchor, i)
			}
		}
	}
	p.of = make([]int, len(p.x.entries))
	byRoot := map[int]int{}
	for i, e := range p.x.entries {
		root := find(i)
		k, ok := byRoot[root]
		if !ok {
			k = len(p.ids)
			byRoot[root] = k
			p.ids = append(p.ids, &identity{name: e.name, roles: map[role]bool{}})
		}
		id := p.ids[k]
		if resolved(id.name) != resolved(e.name) {
			return fmt.Errorf("the export names one person both %q and %q, sharing an email", id.name, e.name)
		}
		id.entries = append(id.entries, i)
		id.roles[e.role] = true
		for _, a := range e.emails {
			if !slices.Contains(id.emails, a) {
				id.emails = append(id.emails, a)
			}
		}
		p.of[i] = k
	}
	return nil
}

func (p *planner) identityOf(entry int) *identity {
	return p.ids[p.of[entry]]
}

func rolesOf(id *identity) string {
	out := []string{}
	for r := range id.roles {
		out = append(out, string(r))
	}
	slices.Sort(out)
	return strings.Join(out, "+")
}

func (p *planner) claim(id *identity, person, how string) error {
	if other, ok := p.claimed[person]; ok && other != id {
		return fmt.Errorf("%s and %s both match %s (%s) %s", other.name, id.name, person, p.st.people.rows[person]["vc_name"], how)
	}
	p.claimed[person] = id
	id.person = person
	return nil
}

func (p *planner) match() error {
	owner := map[string]string{}
	for _, eid := range p.st.emails.order {
		e := p.st.emails.rows[eid]
		owner[e["address"]] = e["person"]
	}
	for _, id := range p.ids {
		owners := []string{}
		for _, a := range id.emails {
			if o := owner[a]; o != "" && !slices.Contains(owners, o) {
				owners = append(owners, o)
			}
		}
		if len(owners) > 1 {
			return fmt.Errorf("%s's emails %v belong to %d people: %v", id.name, id.emails, len(owners), owners)
		}
		if len(owners) == 1 {
			if err := p.claim(id, owners[0], "by email"); err != nil {
				return err
			}
		}
	}
	byEmail := maps.Clone(p.claimed)
	byName := map[string][]string{}
	for _, pid := range p.st.people.order {
		if _, taken := byEmail[pid]; taken {
			continue
		}
		if n := resolved(p.st.people.rows[pid]["vc_name"]); n != "" {
			byName[n] = append(byName[n], pid)
		}
	}
	unmatched := map[string][]*identity{}
	for _, id := range p.ids {
		if id.person != "" {
			continue
		}
		candidates := byName[resolved(id.name)]
		switch len(candidates) {
		case 0:
			unmatched[resolved(id.name)] = append(unmatched[resolved(id.name)], id)
		case 1:
			if err := p.claim(id, candidates[0], "by name"); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s matches %d people by name (%v) and none by email: add an email to the right one", id.name, len(candidates), candidates)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(unmatched)) {
		group := unmatched[name]
		for i, a := range group {
			for _, b := range group[i+1:] {
				adults := !a.roles[student] && !b.roles[student]
				if adults && rolesOf(a) != rolesOf(b) && (len(a.emails) == 0 || len(b.emails) == 0) {
					return fmt.Errorf("%s is on export rows as %s and as %s, and one has no email: give it the other's email if they are one person", a.name, rolesOf(a), rolesOf(b))
				}
			}
		}
	}
	for _, id := range p.ids {
		if id.person == "" {
			id.person = p.name("p")
			id.isNew = true
			p.claimed[id.person] = id
		}
	}
	return nil
}

func (p *planner) staffEntry(id *identity) int {
	for _, i := range id.entries {
		if p.x.entries[i].role == staff {
			return i
		}
	}
	return -1
}

func (p *planner) website() error {
	byEmail := map[string]int{}
	byName := map[string][]int{}
	for _, id := range p.ids {
		i := p.staffEntry(id)
		if i < 0 {
			continue
		}
		for _, e := range id.emails {
			byEmail[e] = i
		}
		byName[resolved(id.name)] = append(byName[resolved(id.name)], i)
	}
	for _, eid := range p.st.emails.order {
		e := p.st.emails.rows[eid]
		if id, ok := p.claimed[e["person"]]; ok && p.staffEntry(id) >= 0 {
			byEmail[e["address"]] = p.staffEntry(id)
		}
	}
	seen := map[int]bool{}
	unmatched := []string{}
	for _, w := range p.x.website {
		i, ok := -1, false
		for _, e := range w.emails {
			if i, ok = byEmail[e]; ok {
				break
			}
		}
		if !ok {
			switch candidates := byName[resolved(w.name)]; len(candidates) {
			case 0:
				unmatched = append(unmatched, w.name)
				continue
			case 1:
				i = candidates[0]
			default:
				return fmt.Errorf("the staff page's entry for %s has no email on file, and %d staff share the name", w.name, len(candidates))
			}
		}
		if seen[i] {
			return fmt.Errorf("the staff page has two entries for %s", p.x.entries[i].name)
		}
		seen[i] = true
		p.web[i] = w
	}
	slog.Info("website staff page", "entries", len(p.x.website), "matched", len(seen), "unmatched", unmatched)
	return nil
}

func (p *planner) name(kind string) string {
	p.named++
	return fmt.Sprintf("@%s%d", kind, p.named)
}

func (p *planner) findGroup(match func(row) bool) string {
	for _, id := range p.groups.order {
		if match(p.groups.rows[id]) {
			return id
		}
	}
	return ""
}

func (p *planner) ensureEveryone() {
	p.everyone = p.findGroup(func(r row) bool { return r["kind"] == "group" && strings.EqualFold(r["slug"], "everyone") })
	if p.everyone != "" {
		return
	}
	id := p.name("g")
	cells := row{"kind": "group", "slug": "everyone", "name": "Everyone", "listed": "Yes", "status": "open"}
	p.groups.add(id, cells)
	p.groupWrites = append(p.groupWrites,
		write{Insert: "GROUP", As: strings.TrimPrefix(id, "@"), Row: cells},
		write{Set: id, Cells: row{"visible_to": id, "members_visible_to": id}})
	orders := store.Order(make([]string, 2))
	for i, source := range []string{"veracross", "manual"} {
		p.groupWrites = append(p.groupWrites, write{Insert: "RULE", Row: row{"group": id, "order": orders[i], "property": "source", "value": source}})
	}
	p.counts["group groups added"]++
	p.everyone = id
}

func (p *planner) newGroup(cells row) string {
	id := p.name("g")
	cells["listed"] = "Yes"
	cells["status"] = "open"
	cells["visible_to"] = p.everyone
	cells["members_visible_to"] = p.everyone
	p.groups.add(id, cells)
	p.groupWrites = append(p.groupWrites, write{Insert: "GROUP", As: strings.TrimPrefix(id, "@"), Row: cells})
	p.counts[cells["kind"]+" groups added"]++
	return id
}

func (p *planner) ensureGrades() {
	for _, g := range grades {
		slug := "grade-" + strings.ToLower(g.code)
		if p.findGroup(func(r row) bool { return r["kind"] == "grade" && strings.EqualFold(r["slug"], slug) }) != "" {
			continue
		}
		band := p.findGroup(func(r row) bool { return r["kind"] == "band" && strings.EqualFold(r["name"], g.band) })
		if band == "" {
			band = p.newGroup(row{"kind": "band", "name": g.band})
		}
		p.newGroup(row{"kind": "grade", "slug": slug, "name": g.title, "parent": band})
	}
}

func plainTarget(r row) bool {
	for _, c := range []string{"exclude", "person", "search", "property", "value", "replace_with", "within"} {
		if strings.TrimSpace(r[c]) != "" && !strings.EqualFold(strings.TrimSpace(r[c]), "no") {
			return false
		}
	}
	return r["target"] != ""
}

func (p *planner) bandRules() {
	for _, band := range p.groups.order {
		if p.groups.rows[band]["kind"] != "band" {
			continue
		}
		want := map[string]bool{}
		for _, g := range p.groups.order {
			r := p.groups.rows[g]
			if r["parent"] == band && (r["kind"] == "grade" || r["kind"] == "classroom") {
				want[g] = true
			}
		}
		orders := []string{}
		held := map[string]bool{}
		for _, rid := range p.st.rules.order {
			r := p.st.rules.rows[rid]
			if r["group"] != band {
				continue
			}
			if plainTarget(r) && want[r["target"]] && !held[r["target"]] {
				held[r["target"]] = true
				orders = append(orders, r["order"])
				continue
			}
			p.groupWrites = append(p.groupWrites, write{Delete: rid})
			p.counts["band rules removed"]++
		}
		missing := []string{}
		for _, g := range p.groups.order {
			if want[g] && !held[g] {
				missing = append(missing, g)
			}
		}
		slices.Sort(orders)
		filled := store.Order(append(orders, make([]string, len(missing))...))
		for i, g := range missing {
			p.groupWrites = append(p.groupWrites, write{Insert: "RULE", Row: row{"group": band, "order": filled[len(orders)+i], "target": g}})
			p.counts["band rules added"]++
		}
	}
}

func (p *planner) bandOf(grade string) (string, error) {
	slug := "grade-" + strings.ToLower(grade)
	g := p.findGroup(func(r row) bool { return r["kind"] == "grade" && strings.EqualFold(r["slug"], slug) })
	if g == "" {
		return "", fmt.Errorf("no grade group has the slug %s", slug)
	}
	return p.groups.rows[g]["parent"], nil
}

func (p *planner) classroom(name, grade string) (string, error) {
	band, err := p.bandOf(grade)
	if err != nil {
		return "", err
	}
	id := p.findGroup(func(r row) bool { return r["kind"] == "classroom" && strings.EqualFold(r["name"], name) })
	if id == "" {
		id = p.newGroup(row{"kind": "classroom", "name": name, "parent": band})
	}
	if seen, ok := p.classroomBands[id]; ok && seen != band {
		return "", fmt.Errorf("classroom %s has students in the %s and %s bands", name, p.groups.rows[seen]["name"], p.groups.rows[band]["name"])
	}
	p.classroomBands[id] = band
	if current := p.groups.rows[id]; current["parent"] != band {
		current["parent"] = band
		p.groupWrites = append(p.groupWrites, write{Set: id, Cells: row{"parent": band}})
		p.counts["classroom bands set"]++
	}
	return id, nil
}

func (p *planner) department(name string) string {
	if id := p.findGroup(func(r row) bool { return r["kind"] == "department" && strings.EqualFold(r["name"], name) }); id != "" {
		return id
	}
	return p.newGroup(row{"kind": "department", "name": name})
}

func (p *planner) crew(classroom, name string) string {
	if id := p.findGroup(func(r row) bool {
		return r["kind"] == "crew" && r["parent"] == classroom && strings.EqualFold(r["name"], name)
	}); id != "" {
		return id
	}
	return p.newGroup(row{"kind": "crew", "name": name, "parent": classroom})
}

func (p *planner) households() ([]*household, error) {
	out := []*household{}
	byKey := map[string]*household{}
	adultOf := map[*identity]*household{}
	for _, hr := range p.x.households {
		adults := []*identity{}
		keys := []string{}
		for _, i := range hr.adults {
			if a := p.identityOf(i); !slices.Contains(adults, a) {
				adults = append(adults, a)
				keys = append(keys, a.person)
			}
		}
		slices.Sort(keys)
		key := strings.Join(keys, ",")
		h, ok := byKey[key]
		if !ok {
			h = &household{adults: adults, address: hr.address, phone: hr.phone}
			for _, a := range adults {
				if adultOf[a] != nil {
					return nil, fmt.Errorf("%s is an adult in two households", a.name)
				}
				adultOf[a] = h
			}
			byKey[key] = h
			out = append(out, h)
		} else if h.address != hr.address || h.phone != hr.phone {
			return nil, fmt.Errorf("the household of %s has a different address or phone on different students' rows", adults[0].name)
		}
		if kid := p.identityOf(hr.kid); !slices.Contains(h.kids, kid) {
			h.kids = append(h.kids, kid)
		}
	}
	return out, nil
}

var leadingDigit = regexp.MustCompile(`^\s*\d`)

func addressVisibility(address string) string {
	switch {
	case address == "":
		return "hidden"
	case leadingDigit.MatchString(address):
		return "full"
	}
	return "partial"
}

func phoneVisibility(h *household) string {
	visible := 0
	for _, a := range h.adults {
		if a.cells["vc_phone"] != "" {
			visible++
		}
	}
	switch {
	case visible == len(h.adults):
		return "visible"
	case visible > 0:
		return "mixed"
	}
	return "hidden"
}

func (p *planner) desired(id *identity) (row, error) {
	n := parseName(id.name)
	cells := row{}
	for _, c := range personColumns {
		cells[c] = ""
	}
	cells["vc_name"] = clean(id.name)
	cells["vc_legal_name"] = n.legal
	cells["vc_name_long"] = n.long
	cells["vc_name_short"] = n.short
	cells["vc_name_sort"] = n.sort
	staffPhone := ""
	students := 0
	for _, i := range id.entries {
		e := p.x.entries[i]
		switch e.role {
		case student:
			students++
			if students > 1 {
				return nil, fmt.Errorf("%s is on two students' rows", id.name)
			}
			cells["vc_grade"] = e.grade
			classroom, err := p.classroom(e.classroom, e.grade)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id.name, err)
			}
			cells["vc_classroom"] = classroom
			if e.crew != "" {
				cells["vc_crew"] = p.crew(classroom, e.crew)
			}
		case parent:
			if cells["vc_phone"] == "" {
				cells["vc_phone"] = e.phone
			}
		case staff:
			w := p.web[i]
			if e.department != "" {
				cells["vc_department"] = p.department(e.department)
			}
			cells["vc_job_title"] = e.jobTitle
			if e.jobTitle == "" {
				cells["vc_job_title"] = w.title
			}
			cells["vc_bio"] = w.bio
			staffPhone = e.phone
		}
	}
	if cells["vc_phone"] == "" {
		cells["vc_phone"] = staffPhone
	}
	return cells, nil
}

func (p *planner) people(households []*household) error {
	for _, id := range p.ids {
		cells, err := p.desired(id)
		if err != nil {
			return err
		}
		id.cells = cells
	}
	for _, h := range households {
		for _, a := range h.adults {
			a.cells["vc_address_visibility"] = addressVisibility(h.address)
			a.cells["vc_phone_visibility"] = phoneVisibility(h)
		}
	}
	for _, id := range p.ids {
		if id.isNew {
			r := row{"source": "veracross"}
			for k, v := range id.cells {
				if v != "" {
					r[k] = v
				}
			}
			p.personWrites = append(p.personWrites, write{Insert: "PERSON", As: strings.TrimPrefix(id.person, "@"), Row: r})
			p.counts["people added"]++
			continue
		}
		current := p.st.people.rows[id.person]
		set := row{}
		for _, c := range personColumns {
			if c == "deactivated" && current["source"] != "veracross" {
				continue
			}
			if strings.TrimSpace(current[c]) != id.cells[c] {
				set[c] = id.cells[c]
			}
		}
		if len(set) == 0 {
			continue
		}
		if _, ok := set["deactivated"]; ok {
			p.counts["people reactivated"]++
		}
		p.personWrites = append(p.personWrites, write{Set: id.person, Cells: set})
		p.counts["people changed"]++
	}
	for _, pid := range p.st.people.order {
		r := p.st.people.rows[pid]
		if _, listed := p.claimed[pid]; listed || r["source"] != "veracross" || r["deactivated"] != "" {
			continue
		}
		p.personWrites = append(p.personWrites, write{Set: pid, Cells: row{"deactivated": p.st.now}})
		p.counts["people deactivated"]++
	}
	return nil
}

func primary(r row) bool {
	return strings.EqualFold(strings.TrimSpace(r["primary"]), "yes")
}

func (p *planner) emails(id *identity) error {
	existing := []row{}
	held := map[string]bool{}
	for _, eid := range p.st.emails.order {
		if e := p.st.emails.rows[eid]; e["person"] == id.person {
			existing = append(existing, e)
			held[e["address"]] = true
		}
	}
	adds := slices.DeleteFunc(slices.Clone(id.emails), func(a string) bool { return held[a] })
	removes := slices.DeleteFunc(slices.Clone(existing), func(e row) bool {
		return e["source"] != "veracross" || slices.Contains(id.emails, e["address"])
	})
	slices.SortStableFunc(removes, func(a, b row) int {
		switch {
		case primary(a) == primary(b):
			return 0
		case primary(a):
			return -1
		}
		return 1
	})
	for len(adds) > 0 && len(removes) > 0 {
		p.emailWrites = append(p.emailWrites, write{Set: removes[0]["id"], Cells: row{"address": adds[0]}})
		p.counts["emails changed"]++
		adds, removes = adds[1:], removes[1:]
	}
	if len(removes) > 0 && primary(removes[0]) && len(removes) < len(existing) {
		return fmt.Errorf("%s's primary email %s is gone from the export and they have others: mark one of those primary in the sheet", id.name, removes[0]["address"])
	}
	for i := len(removes) - 1; i >= 0; i-- {
		p.emailWrites = append(p.emailWrites, write{Delete: removes[i]["id"]})
		p.counts["emails removed"]++
	}
	for i, a := range adds {
		r := row{"address": a, "person": id.person, "source": "veracross", "primary": "No"}
		if len(existing) == 0 && i == 0 {
			r["primary"] = "Yes"
		}
		p.emailWrites = append(p.emailWrites, write{Insert: "PERSON_EMAIL", Row: r})
		p.counts["emails added"]++
	}
	return nil
}

func (p *planner) managed(person string) bool {
	if _, listed := p.claimed[person]; listed {
		return true
	}
	return p.st.people.rows[person]["source"] == "veracross"
}

func (p *planner) syncMembers(group string, want map[string]bool) {
	seen := map[string]bool{}
	for _, mid := range p.st.members.order {
		m := p.st.members.rows[mid]
		if m["group"] != group {
			continue
		}
		if want[m["person"]] && !seen[m["person"]] {
			seen[m["person"]] = true
			if !strings.EqualFold(strings.TrimSpace(m["member"]), "yes") {
				p.memberSets = append(p.memberSets, write{Set: mid, Cells: row{"member": "yes"}})
				p.counts["memberships changed"]++
			}
			continue
		}
		if !p.managed(m["person"]) {
			continue
		}
		p.memberDeletes = append(p.memberDeletes, write{Delete: mid})
		p.counts["memberships removed"]++
	}
	for _, person := range slices.Sorted(maps.Keys(want)) {
		if seen[person] {
			continue
		}
		p.memberInserts = append(p.memberInserts, write{Insert: "MEMBER", Row: row{"group": group, "person": person, "member": "yes"}})
		p.counts["memberships added"]++
	}
}

func (p *planner) families(households []*household) {
	students := map[string]bool{}
	for _, id := range p.ids {
		if id.roles[student] {
			students[id.person] = true
		}
	}
	parents := map[string]map[string]bool{}
	members := map[string]map[string]bool{}
	for _, mid := range p.st.members.order {
		m := p.st.members.rows[mid]
		if p.groups.rows[m["group"]]["kind"] != "family" {
			continue
		}
		if members[m["group"]] == nil {
			members[m["group"]] = map[string]bool{}
			parents[m["group"]] = map[string]bool{}
		}
		if !strings.EqualFold(strings.TrimSpace(m["member"]), "yes") {
			continue
		}
		members[m["group"]][m["person"]] = true
		if !students[m["person"]] {
			parents[m["group"]][m["person"]] = true
		}
	}
	families := []string{}
	for _, gid := range p.st.groups.order {
		if p.st.groups.rows[gid]["kind"] == "family" {
			families = append(families, gid)
		}
	}
	shared := func(h *household, family string) int {
		n := 0
		for _, a := range h.adults {
			if parents[family][a.person] {
				n++
			}
		}
		return n
	}
	for _, h := range households {
		inHousehold := map[string]bool{}
		for _, id := range slices.Concat(h.adults, h.kids) {
			inHousehold[id.person] = true
		}
		for _, f := range families {
			if len(parents[f]) == len(h.adults) && shared(h, f) == len(h.adults) {
				h.family = f
				break
			}
		}
		if h.family != "" {
			continue
		}
		best := 0
		for _, f := range families {
			hasMember := slices.ContainsFunc(slices.Collect(maps.Keys(members[f])), func(person string) bool { return inHousehold[person] })
			hasParent := slices.ContainsFunc(slices.Collect(maps.Keys(parents[f])), func(person string) bool { return inHousehold[person] })
			if hasMember && hasParent && shared(h, f) > best {
				h.family, best = f, shared(h, f)
			}
		}
	}
	winner := map[string]*household{}
	for _, h := range households {
		if h.family == "" {
			continue
		}
		if w, ok := winner[h.family]; !ok || shared(h, h.family) > shared(w, h.family) {
			winner[h.family] = h
		}
	}
	for _, h := range households {
		if h.family != "" && winner[h.family] != h {
			h.family = ""
		}
	}
	for _, h := range households {
		cells := row{"vc_address": h.address, "vc_phone": h.phone}
		if h.family == "" {
			insert := row{"kind": "family"}
			for k, v := range cells {
				if v != "" {
					insert[k] = v
				}
			}
			h.family = p.newGroup(insert)
			p.familyWrites = append(p.familyWrites, write{Set: h.family, Cells: row{"managed_by": h.family}})
		} else {
			current := p.groups.rows[h.family]
			set := row{}
			for _, c := range familyColumns {
				if strings.TrimSpace(current[c]) != cells[c] {
					set[c] = cells[c]
				}
			}
			if current["managed_by"] != h.family {
				set["managed_by"] = h.family
			}
			if len(set) > 0 {
				p.familyWrites = append(p.familyWrites, write{Set: h.family, Cells: set})
				p.counts["families changed"]++
			}
		}
		want := map[string]bool{}
		for _, id := range slices.Concat(h.kids, h.adults) {
			want[id.person] = true
		}
		p.syncMembers(h.family, want)
	}
}

func (p *planner) roles() {
	for _, rg := range roleGroups {
		group := p.findGroup(func(r row) bool { return r["kind"] == "group" && strings.EqualFold(r["slug"], rg.slug) })
		if group == "" {
			group = p.newGroup(row{"kind": "group", "slug": rg.slug, "name": rg.title})
		}
		want := map[string]bool{}
		for _, id := range p.ids {
			if rg.holds(id) {
				want[id.person] = true
			}
		}
		p.syncMembers(group, want)
	}
}
