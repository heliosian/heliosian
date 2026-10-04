package db

import (
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/store"
)

var designQueries = []string{
	`(from GROUP @g
  (where mail
         (= status "open")
         (or (exists EFFECTIVE_MEMBER (= group @g.managed_by) (= person @viewer))
             (exists EFFECTIVE_MEMBER (= group @g.visible_to) (= person @viewer)))))`,
	`(from GROUP @g
  (where (exists MEMBER (= group @g) (= person.grade "6"))))`,
	`(from MEMBER
  (where (= group "grpX7pQ2m9KdLr") (= person.grade "6"))
  (order person.name_sort asc)
  (include person))`,
	`(from GROUP @p
  (where (= kind "party")
         (>= start today)
         (> capacity (count MEMBER (= group @p) (= member "yes")
                                  (not (blank purchase_id))))))`,
}

func mustParse(t *testing.T, src string) *Query {
	t.Helper()
	q, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%s): %v", src, err)
	}
	return q
}

func TestDesignQueriesParse(t *testing.T) {
	for _, src := range designQueries {
		mustParse(t, src)
	}
}

func TestRenderIsCanonical(t *testing.T) {
	for _, src := range designQueries {
		q := mustParse(t, src)
		once := q.String()
		again := mustParse(t, once)
		if !q.tree.equal(again.tree) {
			t.Fatalf("rendering changed the query:\n%s\n%s", src, once)
		}
		if again.String() != once {
			t.Fatalf("rendering is not stable:\n%s\n%s", once, again.String())
		}
		for _, line := range strings.Split(once, "\n") {
			if len(line) > renderWidth && !strings.Contains(line, "(") {
				t.Fatalf("line runs past the width: %q", line)
			}
		}
	}
	if got := mustParse(t, "(from   PERSON\n (where (=  consent \"listed\")))").String(); got != `(from PERSON (where (= consent "listed")))` {
		t.Fatalf("a short query renders as %q", got)
	}
	long := mustParse(t, designQueries[0]).String()
	if !strings.HasPrefix(long, "(from GROUP @g\n  (where mail\n         (= status \"open\")") {
		t.Fatalf("a long query renders as\n%s", long)
	}
}

func TestParseRefuses(t *testing.T) {
	for src, want := range map[string]string{
		`(from PERSON (where (= grde "6")))`:                  "no column grde on PERSON; did you mean grade?",
		`(from PERSN)`:                                        "no table PERSN; did you mean PERSON?",
		`(from PERSON (where (= grade "13")))`:                `"13" is not one of`,
		`(from MEMBER (where (= group "perX7pQ2m9KdLr")))`:    "is a PERSON id",
		`(from PERSON (where (= "a" "b")))`:                   "two literals",
		`(from MEMBER (where (< group group)))`:               "orders numbers",
		`(from MEMBER (where (= group @g)))`:                  "no row named @g",
		`(from GROUP (where (= capacity "ten")))`:             "is text",
		`(from GROUP (where (= start "soon")))`:               "not a date",
		`(from PERSON (where (= consent PERSON)))`:            "is a table",
		`(from MEMBER (where (= person.name.long "x")))`:      "no column name",
		`(from MEMBER (where (= group.kind.x "x")))`:          "not a reference",
		`(from PERSON (wher (= consent "listed")))`:           "parts are",
		`(from PERSON (where (= consent "listed"))`:           "never closed",
		`(from PERSON (order name_sort up))`:                  "asc or desc",
		`(from PERSON (where consent))`:                       "not true or false",
		`(from MEMBER (where (= group (count MEMBER))))`:      "can't compare",
		`(from GROUP (where (in kind "party" "nope")))`:       `"nope" is not one of`,
		`(from PERSON (where (in id (select MEMBER.group))))`: "can't compare",
		`(from PERSON (include source))`:                      "not a reference",
		`(from PERSON (where (= hidden "Yes")))`:              "is text",
		`(from GROUP @viewer)`:                                "can't name a row",
		`(select PERSON.id)`:                                  "starts with from",
		`(from PERSON @p (where (in id (ancestors @p))))`:     "PERSON has no parent to follow",
	} {
		_, err := Parse(src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", src, err, want)
		}
	}
}

func unfiltered(t *testing.T, src string) *Query {
	t.Helper()
	tree, err := readSexp(src)
	if err != nil {
		t.Fatal(err)
	}
	q, err := newCompiler(true).query(tree)
	if err != nil {
		t.Fatalf("%s: %v", src, err)
	}
	return q
}

var testNow = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func runAs(t *testing.T, m *Model, viewer, src string) Result {
	t.Helper()
	return m.Run(unfiltered(t, src), Env{Viewer: viewer, Now: testNow})
}

func ids(rows []store.Row, column string) []string {
	out := []string{}
	for _, row := range rows {
		out = append(out, row[column])
	}
	return out
}

func TestRun(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct {
		viewer, src, column string
		want                []string
	}{
		{"", `(from PERSON (where (= source "veracross")) (order name_sort asc))`, "id", []string{"per00000000001", "per00000000002", "per00000000003"}},
		{"", `(from PERSON (where (= source "veracross")) (order name_sort desc) (limit 1))`, "id", []string{"per00000000003"}},
		{"", `(from MEMBER (where (= group "grp00000000040") (= member "yes")) (order person.name_sort asc))`, "person", []string{"per00000000002", "per00000000003", "per00000000004"}},
		{"per00000000002", `(from GROUP @g (where (exists EFFECTIVE_MEMBER (= group @g.managed_by) (= person @viewer))))`, "id", []string{"grp00000000020"}},
		{"", `(from MEMBER (where lead))`, "person", []string{"per00000000003"}},
		{"", `(from GROUP @g (where (> (count MEMBER (= group @g) (= member "yes")) 2)))`, "id", []string{"grp00000000040"}},
		{"", `(from GROUP (where (>= start today)))`, "id", []string{"grp00000000040"}},
		{"", `(from GROUP (where (< end now)))`, "id", nil},
		{"", `(from PERSON (where (in id (select MEMBER.person (= group "grp00000000020")))))`, "id", []string{"per00000000001", "per00000000002"}},
		{"", `(from COLLECTION (where (= groups "grp00000000010")))`, "token", []string{"fed00000000001"}},
		{"", `(from PERSON (where (not hidden)))`, "id", []string{"per00000000001", "per00000000002", "per00000000003", "per00000000004"}},
		{"", `(from PERSON (where (= vc_classroom.name "Hummingbirds")))`, "id", []string{"per00000000001"}},
		{"", `(from MEMBER (where (= guest_of.name_short "Rowan")))`, "person", []string{"per00000000004"}},
		{"", `(from GROUP (where (or (= kind "family") mail)))`, "id", []string{"grp00000000020", "grp00000000030"}},
		{"", `(from PERSON (where (blank vc_name)))`, "id", []string{"per00000000004"}},
		{"", `(from BIRTHDAY_YEAR (where (>= year 2026) (= charity.name "Second Harvest")))`, "id", []string{"bdy00000000001"}},
		{"", `(from GROUP (where (!= status "closed") (= kind "Event")))`, "id", []string{"grp00000000040"}},
		{"per00000000003", `(from PERSON (where (= @viewer id)))`, "id", []string{"per00000000003"}},
		{"", `(from PERSON (where (= @viewer id)))`, "id", nil},
	} {
		got := ids(runAs(t, m, c.viewer, c.src).Rows(), c.column)
		if !slices.Equal(got, c.want) && (len(got) != 0 || len(c.want) != 0) {
			t.Errorf("%s as %q = %v, want %v", c.src, c.viewer, got, c.want)
		}
	}
}

func TestCorrelatedSelectsAreNotShared(t *testing.T) {
	m := sample(t).Model()
	for _, viewer := range []string{"", "per00000000002", "per00000000003"} {
		direct := ids(runAs(t, m, viewer, `(from GROUP @g (where (exists MEMBER (= group @g))))`).Rows(), "id")
		nested := ids(runAs(t, m, viewer, `(from GROUP @g (where (in id (select MEMBER.group (in person (select MEMBER.person (= group @g)))))))`).Rows(), "id")
		if len(direct) < 2 || !slices.Equal(direct, nested) {
			t.Errorf("as %q: groups with members %v, through a nested select %v", viewer, direct, nested)
		}
		constant := ids(runAs(t, m, viewer, `(from GROUP (where (exists MEMBER (= group "grp00000000040"))))`).Rows(), "id")
		all := ids(runAs(t, m, viewer, `(from GROUP)`).Rows(), "id")
		if !slices.Equal(constant, all) {
			t.Errorf("as %q: a constant exists kept %v of %v", viewer, constant, all)
		}
	}
}

func TestValueSetMatchesEquality(t *testing.T) {
	vals := map[Kind][]string{
		Text:  {"Ash", "ash", "ASH ", "oak", ""},
		Order: {"a", "A", "b", ""},
		Int:   {"1", "1.0", "2", "x"},
		Money: {"7.25", "7.250", "7.2"},
		Date:  {"2026-09-24", "2026-09-25"},
		Bool:  {"Yes", "No", "true"},
		Ref:   {"per00000000001", "per00000000002", ""},
		Refs:  {"per00000000001, per00000000002", "per00000000002", "per00000000003"},
	}
	moments := []string{"2026-09-24 00:00", "2026-09-24 16:00"}
	values := func(k Kind) []value {
		out := []value{}
		for _, s := range vals[k] {
			out = append(out, cellValue(Column{Kind: k}, s))
		}
		if k == Date {
			for _, s := range moments {
				out = append(out, cellValue(Column{Kind: Moment}, s))
			}
		}
		return out
	}
	pairs := [][2]Kind{{Text, Text}, {Order, Order}, {Order, Text}, {Text, Order}, {Int, Int}, {Int, Money}, {Date, Date}, {Bool, Bool}, {Ref, Ref}, {Ref, Refs}, {Refs, Ref}, {Refs, Refs}}
	for _, p := range pairs {
		items := values(p[1])
		set := newValueSet(p[0] == Order && p[1] == Order)
		for _, item := range items {
			set.add(item)
		}
		for _, x := range values(p[0]) {
			want := slices.ContainsFunc(items, func(item value) bool { return equalValues(x, item) })
			if got := set.has(x); got != want {
				t.Errorf("%v in %v: %q has %v, equality says %v", p[0], p[1], x.s, got, want)
			}
		}
	}
}

func TestInclude(t *testing.T) {
	m := sample(t).Model()
	r := runAs(t, m, "", `(from MEMBER (where (= group "grp00000000040")) (include person group.added_by))`)
	if len(r.Resources["MEMBER"]) != 3 || len(r.Resources["PERSON"]) != 3 || len(r.Resources["GROUP"]) != 1 {
		t.Fatalf("resources %v", r.Resources)
	}
	r = runAs(t, m, "", `(from COLLECTION (include groups))`)
	if len(r.Resources["GROUP"]) != 2 {
		t.Fatalf("resources %v", r.Resources)
	}
}

func TestSum(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": "per00000000002"}, store.Row{"price": "12.50"}),
		store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": "per00000000004"}, store.Row{"price": "7.25"}),
	); err != nil {
		t.Fatal(err)
	}
	got := ids(runAs(t, s.Model(), "", `(from GROUP @g (where (= (sum price MEMBER (= group @g)) 19.75)))`).Rows(), "id")
	if !slices.Equal(got, []string{"grp00000000040"}) {
		t.Fatalf("sum matched %v", got)
	}
}

func effective(t *testing.T, s *Store, group string) []string {
	t.Helper()
	return ids(runAs(t, s.Model(), "", `(from EFFECTIVE_MEMBER (where (= group "`+group+`")))`).Rows(), "person")
}

func TestEffectiveMembers(t *testing.T) {
	s := sample(t)
	if got := effective(t, s, "grp00000000030"); !slices.Equal(got, []string{"per00000000002"}) {
		t.Fatalf("the Hummingbirds parents list holds %v", got)
	}
	if got := effective(t, s, "grp00000000006"); !slices.Equal(got, []string{"per00000000003"}) {
		t.Fatalf("Who? admins are %v", got)
	}
	if got := effective(t, s, "grp00000000012"); !slices.Equal(got, []string{"per00000000001"}) {
		t.Fatalf("the Jayvens band, through its grades and classrooms, holds %v", got)
	}
	if got := effective(t, s, "grp00000000040"); !slices.Equal(got, []string{"per00000000002", "per00000000003", "per00000000004"}) {
		t.Fatalf("the picnic holds %v", got)
	}
	rows := runAs(t, s.Model(), "per00000000002", `(from GROUP @g (where (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer))) (order id asc))`).Rows()
	if got := ids(rows, "id"); !slices.Equal(got, []string{"grp00000000002", "grp00000000004", "grp00000000020", "grp00000000030", "grp00000000040", "grp00000000042"}) {
		t.Fatalf("Rowan is effectively in %v", got)
	}
	if err := commit(s, GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000099", "group": "grp00000000030", "person": "per00000000002", "member": "excluded"})); err != nil {
		t.Fatal(err)
	}
	if got := effective(t, s, "grp00000000030"); len(got) != 0 {
		t.Fatalf("an excluded parent stays on the list: %v", got)
	}
}

func TestEffectiveCycleResolves(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Insert("RULE", store.Row{"id": "rul00000000099", "group": "grp00000000005", "order": "i", "target": "grp00000000006"})); err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"grp00000000005", "grp00000000006"} {
		if got := effective(t, s, g); !slices.Equal(got, []string{"per00000000003"}) {
			t.Fatalf("%s holds %v", g, got)
		}
	}
}

func TestEffectiveDropsDeactivated(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000003"}, store.Row{"deactivated": "2026-09-30 08:00"})); err != nil {
		t.Fatal(err)
	}
	if got := effective(t, s, "grp00000000006"); len(got) != 0 {
		t.Fatalf("a deactivated admin stays an admin: %v", got)
	}
}

func TestParentsAndChildrenFollowStudents(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"id": "grp00000000051", "kind": "group", "name": "Juni's parents"}),
		store.Insert("RULE", store.Row{"id": "rul00000000055", "group": "grp00000000051", "order": "a", "person": student, "replace_with": "parents"}),
		store.Insert("GROUP", store.Row{"id": "grp00000000052", "kind": "group", "name": "Rowan's children"}),
		store.Insert("RULE", store.Row{"id": "rul00000000056", "group": "grp00000000052", "order": "a", "person": parent, "replace_with": "children"}),
		store.Insert("GROUP", store.Row{"id": "grp00000000053", "kind": "group", "name": "Rowan's parents"}),
		store.Insert("RULE", store.Row{"id": "rul00000000057", "group": "grp00000000053", "order": "a", "person": parent, "replace_with": "parents"}),
	); err != nil {
		t.Fatal(err)
	}
	for group, want := range map[string][]string{"grp00000000051": {parent}, "grp00000000052": {student}, "grp00000000053": nil} {
		if got := effective(t, s, group); !slices.Equal(got, want) {
			t.Errorf("%s holds %v, want %v", group, got, want)
		}
	}
}

func TestRuleSelectors(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"id": "grp00000000050", "kind": "group", "name": "Test"}),
		store.Insert("RULE", store.Row{"id": "rul00000000051", "group": "grp00000000050", "order": "a", "property": "source", "value": "veracross", "within": "grp00000000004"}),
		store.Insert("RULE", store.Row{"id": "rul00000000052", "group": "grp00000000050", "order": "b", "exclude": "Yes", "search": "lindqvist"}),
		store.Insert("RULE", store.Row{"id": "rul00000000053", "group": "grp00000000050", "order": "c", "person": "per00000000001", "replace_with": "household"}),
	); err != nil {
		t.Fatal(err)
	}
	if got := effective(t, s, "grp00000000050"); !slices.Equal(got, []string{"per00000000001", "per00000000002"}) {
		t.Fatalf("the tag holds %v", got)
	}
	if err := commit(s, GroupsSheet, store.Insert("RULE", store.Row{"id": "rul00000000054", "group": "grp00000000050", "order": "d", "property": "colour", "value": "red"})); err == nil || !strings.Contains(err.Error(), "no PERSON column") {
		t.Fatalf("a rule on no column: %v", err)
	}
}
