package db

import (
	"maps"
	"slices"
	"strings"

	"heliosian/internal/store"
)

func (cx *compiler) groupOperand(s *sexp, sc *scope, op string) (operand, error) {
	if !s.isList && s.kind == atomString {
		return coerce(s, typ{class: classRow, kind: ID, table: "GROUP"})
	}
	g, err := cx.operand(s, sc)
	if err != nil {
		return operand{}, err
	}
	if g.lit != nil || g.t.class != classRow || g.t.table != "GROUP" {
		return operand{}, s.errorf("%s takes a group", op)
	}
	return g, nil
}

func (cx *compiler) membersSet(s *sexp, sc *scope) (func(f *frame) *valueSet, typ, bool, error) {
	if len(s.list) != 2 && len(s.list) != 3 {
		return nil, typ{}, false, s.errorf("members takes a group and, after it, whom to replace them with")
	}
	groups, outer, err := cx.groupsArg(s.list[1], sc)
	if err != nil {
		return nil, typ{}, false, err
	}
	how := ""
	if len(s.list) == 3 {
		item := s.list[2]
		rule, _ := Lookup("RULE")
		replace, _ := rule.Column("replace_with")
		names := []string{}
		for _, v := range replace.Values {
			names = append(names, v.Name)
		}
		if item.isList || item.kind != atomString || !slices.Contains(names, item.text) {
			return nil, typ{}, false, item.errorf("members replaces with one of %s", strings.Join(names, ", "))
		}
		how = item.text
	}
	return func(f *frame) *valueSet {
		ids := groups(f)
		key := "members\x00" + how + "\x00" + strings.Join(ids, ",")
		if out, ok := f.run.memo[key]; ok {
			return out.(*valueSet)
		}
		people := map[string]bool{}
		for _, g := range ids {
			maps.Copy(people, f.run.members(g))
		}
		out := newValueSet(false)
		for person := range f.run.relatives().replace(people, how) {
			out.add(value{kind: ID, s: person})
		}
		f.run.memo[key] = out
		return out
	}, typ{class: classRow, kind: ID, table: "PERSON"}, outer, nil
}

func (cx *compiler) groupsArg(s *sexp, sc *scope) (func(f *frame) []string, bool, error) {
	if s.isList && (s.head() == "select" || s.head() == "slug") {
		set, t, outer, err := cx.set(s, sc, typ{class: classRow, kind: ID, table: "GROUP"})
		if err != nil {
			return nil, false, err
		}
		if t.class != classRow || t.table != "GROUP" {
			return nil, false, s.errorf("members takes a select of groups, not %s", t)
		}
		return func(f *frame) []string { return slices.Sorted(maps.Keys(set(f).keys)) }, outer, nil
	}
	g, err := cx.groupOperand(s, sc, "members")
	if err != nil {
		return nil, false, err
	}
	return func(f *frame) []string {
		if v := g.eval(f); !v.blank {
			return []string{v.s}
		}
		return nil
	}, !g.local, nil
}

func (cx *compiler) covers(s *sexp, sc *scope) (cond, error) {
	if len(s.list) != 3 || !s.list[2].isList {
		return cond{}, s.errorf("covers takes a group and a set of people")
	}
	g, err := cx.groupOperand(s.list[1], sc, "covers")
	if err != nil {
		return cond{}, err
	}
	set, t, _, err := cx.set(s.list[2], sc, typ{class: classRow, kind: ID, table: "PERSON"})
	if err != nil {
		return cond{}, err
	}
	if t.class != classRow || t.table != "PERSON" {
		return cond{}, s.list[2].errorf("covers takes a set of people, not %s", t)
	}
	return cond{eval: func(f *frame) bool {
		group := g.eval(f)
		return !group.blank && f.run.covers(group.s, set(f))
	}}, nil
}

func (r *run) members(group string) map[string]bool {
	em, _ := Lookup("EFFECTIVE_MEMBER")
	whole := r.membersReadable(group)
	out := map[string]bool{}
	for _, row := range r.m.effectiveRows(group) {
		if whole || r.readable(em, row) {
			out[row["person"]] = true
		}
	}
	return out
}

// membersReadable is the EFFECTIVE_MEMBER read check on a row of the group naming nobody, so it holds
// only through clauses that read the group alone, and then every row of the group is readable.
func (r *run) membersReadable(group string) bool {
	if r.system != "" {
		return true
	}
	key := "members readable\x00" + group
	if held, ok := r.memo[key]; ok {
		return held.(bool)
	}
	em, _ := Lookup("EFFECTIVE_MEMBER")
	held := r.check(em.Name, policies.read[em.Name], &frame{table: em, row: store.Row{"group": group}, name: "row", run: r})
	r.memo[key] = held
	return held
}

func (r *run) covers(group string, people *valueSet) bool {
	if len(people.keys) == 0 {
		return false
	}
	rows := r.m.effectiveRows(group)
	found := []store.Row{}
	for person := range people.keys {
		i, ok := slices.BinarySearchFunc(rows, person, func(row store.Row, p string) int { return strings.Compare(row["person"], p) })
		if !ok {
			return false
		}
		found = append(found, rows[i])
	}
	if r.membersReadable(group) {
		return true
	}
	em, _ := Lookup("EFFECTIVE_MEMBER")
	for _, row := range found {
		if !r.readable(em, row) {
			return false
		}
	}
	return true
}

func (r *run) relatives() relatives {
	member, _ := Lookup("MEMBER")
	members, groups := r.table("MEMBER"), r.table("GROUP")
	in := func(row store.Row) bool { return memberAs(row) == "yes" && r.readable(member, row) }
	groupsOf := func(person string, is func(g store.Row) bool) []string {
		out := []string{}
		for _, row := range members.Referencing("person", person) {
			if g, ok := groups.Get(row["group"]); ok && is(g) && in(row) {
				out = append(out, row["group"])
			}
		}
		return out
	}
	return relatives{
		student: func(person string) bool {
			return len(groupsOf(person, func(g store.Row) bool { return g["kind"] == "group" && g["slug"] == "students" })) > 0
		},
		families: func(person string) []string {
			return groupsOf(person, func(g store.Row) bool { return g["kind"] == "family" })
		},
		people: func(family string) []string {
			out := []string{}
			for _, row := range members.Referencing("group", family) {
				if in(row) {
					out = append(out, row["person"])
				}
			}
			return out
		},
	}
}
