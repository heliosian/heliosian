package db

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/store"
	"heliosian/internal/trace"
)

func TestImplementationOnlyGroupsServeOneGroup(t *testing.T) {
	s := sample(t)
	group := func(as, name string, cells map[string]any) Edit {
		row := map[string]any{"kind": "group", "name": name, "status": "open", "posting": "members", "replying": "members"}
		for k, v := range cells {
			row[k] = v
		}
		return Edit{Insert: "GROUP", As: as, Row: row}
	}
	made := apply(t, s, []Edit{
		group("tech", "Tech Team", map[string]any{"slug": "tech"}),
		group("techManagers", "Tech Team Managers", map[string]any{"visible_to": "@tech"}),
		{Set: "@tech", Cells: map[string]any{"visible_to": "@tech", "managed_by": "@techManagers"}},
		{Set: "@techManagers", Cells: map[string]any{"managed_by": "@techManagers"}},
		group("viewers", "Hummingbirds Chat Viewers", nil),
		group("", "Hummingbirds Chat", map[string]any{"visible_to": "@viewers", "managed_by": "@techManagers"}),
		group("sciOly", "Sci Oly Managers", nil),
		{Set: "@sciOly", Cells: map[string]any{"managed_by": "@sciOly"}},
		group("", "Sci Oly Parents", map[string]any{"managed_by": "@sciOly"}),
		group("", "Sci Oly Students", map[string]any{"managed_by": "@sciOly"}),
	})
	tech, techManagers, viewers, sciOly := made[0], made[1], made[4], made[6]
	got := ids(as(t, s, staff, `(from GROUP @g (where (implementation_only @g)))`), "id")
	slices.Sort(got)
	want := []string{"grp00000000041", "grp00000000042", "grp00000000043", "grp00000000044", "grp00000000513", "grp00000000514", "grp00000000522", "grp00000000704", "grp00000020073", "grp00000020075", "grp00000020077", viewers}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("implementation only: %v, want %v", got, want)
	}
	sidebar := ids(as(t, s, staff, `(from GROUP @g (where (sidebar_group @g)))`), "id")
	nested := ids(as(t, s, staff, `(from GROUP @g (where (in id (select GROUP.id @s (sidebar_group @s)))))`), "id")
	if !slices.Equal(nested, sidebar) {
		t.Errorf("a select of the sidebar groups found %v, the sidebar %v", nested, sidebar)
	}
	for id, want := range map[string]bool{tech: true, techManagers: true, sciOly: true, viewers: false, "grp00000000041": false, "grp00000000060": false, "grp00000000030": true} {
		if slices.Contains(sidebar, id) != want {
			t.Errorf("%s in the sidebar: %v, want %v", id, !want, want)
		}
	}
}

func TestABatchScansEachTableOnceForItsSelects(t *testing.T) {
	m := sample(t).Model()
	root := trace.New("test")
	gradebands := `(select GROUP.id (or (in slug "students" "parents" "staff") (and (= kind "group") (= parent.kind "gradeband"))))`
	queries := []*Query{}
	for _, src := range []string{
		`(from EFFECTIVE_MEMBER (where (in group ` + gradebands + `)) (columns group person))`,
		`(from GROUP @g (where (in id ` + gradebands + `) (sidebar_group @g)))`,
		`(from GROUP @g (where (sidebar_group @g) (= kind "event")))`,
		`(from GROUP @g (where (implementation_only @g)))`,
	} {
		queries = append(queries, mustParse(t, src))
	}
	results := m.RunAll(trace.With(t.Context(), root), queries, Env{Viewer: staff, Now: testNow})
	if len(results[3].IDs) != 11 {
		t.Fatalf("the fourth query found %v", results[3].IDs)
	}
	passes := map[string]int{}
	for _, run := range root.Children {
		for _, c := range run.Children {
			if c.Name != "fuse" {
				continue
			}
			for table, n := range c.Counts {
				passes[table] += n
			}
		}
	}
	if passes["GROUP"] != 1 {
		t.Errorf("GROUP was scanned %d times for the gradebands select, want once: %v", passes["GROUP"], passes)
	}
	for _, table := range []string{"RULE", "APP", "WIDGET", "PERSON"} {
		if passes[table] != 0 {
			t.Errorf("%s was scanned %d times for implementation_only's selects, which probe: %v", table, passes[table], passes)
		}
	}
}

var designQueries = []string{
	`(from GROUP @g
  (where listed
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
         (> capacity (count MEMBER (= group @p) (= member "yes")))))`,
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
	if !strings.HasPrefix(long, "(from GROUP @g\n  (where listed\n         (= status \"open\")") {
		t.Fatalf("a long query renders as\n%s", long)
	}
}

func TestParseRefuses(t *testing.T) {
	for src, want := range map[string]string{
		`(from PERSON (where (= grde "6")))`:                                       "no column grde on PERSON; did you mean grade?",
		`(from PERSN)`:                                                             "no table PERSN; did you mean PERSON?",
		`(from PERSON (where (= grade "13")))`:                                     `"13" is not one of`,
		`(from MEMBER (where (= group "perX7pQ2m9KdLr")))`:                         "is a PERSON id",
		`(from PERSON (where (= "a" "b")))`:                                        "two literals",
		`(from MEMBER (where (< group group)))`:                                    "orders numbers",
		`(from MEMBER (where (= group @g)))`:                                       "no row named @g",
		`(from GROUP (where (= capacity "ten")))`:                                  "is text",
		`(from GROUP (where (= start "soon")))`:                                    "not a date",
		`(from PERSON (where (= consent PERSON)))`:                                 "is a table",
		`(from MEMBER (where (= person.name.long "x")))`:                           "no column name",
		`(from MEMBER (where (= group.kind.x "x")))`:                               "not a reference",
		`(from PERSON (wher (= consent "listed")))`:                                "parts are",
		`(from PERSON (where (= consent "listed"))`:                                "never closed",
		`(from PERSON (order name_sort up))`:                                       "asc or desc",
		`(from PERSON (where consent))`:                                            "not true or false",
		`(from MEMBER (where (= group (count MEMBER))))`:                           "can't compare",
		`(from GROUP (where (in kind "party" "nope")))`:                            `"nope" is not one of`,
		`(from PERSON (where (in id (select MEMBER.group))))`:                      "can't compare",
		`(from PERSON (include source))`:                                           "not a reference",
		`(from PERSON (where (= hidden "Yes")))`:                                   "is text",
		`(from GROUP @viewer)`:                                                     "can't name a row",
		`(select PERSON.id)`:                                                       "starts with from",
		`(from PERSON @p (where (in id (ancestors @p))))`:                          "PERSON has no parent to follow",
		`(from GROUP (where (> (length capacity) 1)))`:                             "counts the characters of text",
		`(from GROUP (where (> (length name kind) 1)))`:                            "length takes one path",
		`(from GROUP (where (contains capacity "1")))`:                             "looks in text",
		`(from GROUP (where (contains name kind)))`:                                "contains takes a path and a quoted string",
		`(from GROUP @e (where (covers @e (members "grp00000000010" "cousins"))))`: "members replaces with one of",
		`(from GROUP @e (where (covers @e (select GROUP.id))))`:                    "covers takes a set of people",
		`(from PERSON (where (in id (members (select PERSON.id)))))`:               "members takes a select of groups",
		`(from PERSON @p (where (covers @p (members "grp00000000010"))))`:          "covers takes a group",
		`(from GROUP @e (where (covers @e (members "per00000000001"))))`:           "is a PERSON id",
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
	return m.Run(t.Context(), unfiltered(t, src), Env{Viewer: viewer, Now: testNow})
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
		{"", `(from PERSON (where (= source "veracross")) (order name_sort asc))`, "id", []string{
			"per00000020053", "per00000020055", "per00000020054", "per00000020001", "per00000020082", "per00000020083",
			"per00000020018", "per00000000001", "per00000000002", "per00000020019", "per00000020097", "per00000020095",
			"per00000020096", "per00000020016", "per00000020036", "per00000020035", "per00000020014", "per00000020009",
			"per00000020037", "per00000020087", "per00000020084", "per00000020086", "per00000020085", "per00000020002",
			"per00000020080", "per00000020081", "per00000020056", "per00000020011", "per00000020015", "per00000020022",
			"per00000020024", "per00000020025", "per00000020006", "per00000020062", "per00000020063", "per00000020065",
			"per00000020064", "per00000020029", "per00000020028", "per00000020030", "per00000020017", "per00000020094",
			"per00000020093", "per00000020004", "per00000020067", "per00000020068", "per00000020066", "per00000020005",
			"per00000020021", "per00000020020", "per00000000003", "per00000020073", "per00000020071", "per00000020072",
			"per00000020008", "per00000020060", "per00000020061", "per00000020059", "per00000020088", "per00000020090",
			"per00000020089", "per00000020047", "per00000020049", "per00000020048", "per00000020042", "per00000020043",
			"per00000020041", "per00000020040", "per00000020027", "per00000020026", "per00000020007", "per00000020032",
			"per00000020031", "per00000020033", "per00000020034", "per00000020046", "per00000020044", "per00000020045",
			"per00000020010", "per00000020003", "per00000020023", "per00000020069", "per00000020070", "per00000020058",
			"per00000020057", "per00000020051", "per00000020050", "per00000020052", "per00000020013", "per00000020012",
			"per00000020078", "per00000020079", "per00000020039", "per00000020038", "per00000020091", "per00000020092",
		}},
		{"", `(from PERSON (where (= source "veracross")) (order name_sort desc) (limit 1))`, "id", []string{"per00000020092"}},
		{"", `(from MEMBER (where (= group "grp00000000040") (= member "yes")) (order person.name_sort asc))`, "person", []string{"per00000000002", "per00000000003", "per00000000004"}},
		{"per00000000002", `(from GROUP @g (where (exists EFFECTIVE_MEMBER (= group @g.managed_by) (= person @viewer))))`, "id", []string{
			"grp00000000020", "grp00000000512", "grp00000000513", "grp00000020073", "grp00000020074", "grp00000020075",
			"grp00000020076",
		}},
		{"", `(from MEMBER (where (in group (select GROUP.leads))))`, "person", []string{"per00000000003", "per00000000002"}},
		{"", `(from GROUP @g (where (> (count MEMBER (= group @g) (= member "yes")) 2)))`, "id", []string{
			"grp00000000001", "grp00000000002", "grp00000000003", "grp00000000010", "grp00000000011", "grp00000000020",
			"grp00000000040", "grp00000000501", "grp00000000503", "grp00000020002", "grp00000020003", "grp00000020007",
			"grp00000020008", "grp00000020009", "grp00000020012", "grp00000020015", "grp00000020017", "grp00000020021",
			"grp00000020022", "grp00000020023", "grp00000020026", "grp00000020030", "grp00000020031", "grp00000020032",
			"grp00000020035", "grp00000020038", "grp00000020040", "grp00000020041", "grp00000020043", "grp00000020045",
			"grp00000020046", "grp00000020048", "grp00000020049", "grp00000020050", "grp00000020051", "grp00000020052",
			"grp00000020053", "grp00000020054", "grp00000020055", "grp00000020056", "grp00000020057", "grp00000020059",
			"grp00000020062", "grp00000020064", "grp00000020065", "grp00000020068", "grp00000020074", "grp00000020076",
			"grp00000020078",
		}},
		{"", `(from GROUP (where (>= start today) (!= kind "group")))`, "id", []string{"grp00000000040", "grp00000000512", "grp00000000521"}},
		{"", `(from GROUP (where (< end now)))`, "id", nil},
		{"", `(from PERSON (where (in id (select MEMBER.person (= group "grp00000000020")))))`, "id", []string{"per00000000001", "per00000000002", "per00000020018", "per00000020019"}},
		{"", `(from SAVED_SEARCH (where (contains search "community")))`, "token", []string{"fed00000000002"}},
		{"", `(from PERSON (where (not hidden)))`, "id", []string{
			"per00000000001", "per00000000002", "per00000000003", "per00000000004", "per00000020001", "per00000020002",
			"per00000020003", "per00000020004", "per00000020005", "per00000020006", "per00000020007", "per00000020008",
			"per00000020009", "per00000020010", "per00000020011", "per00000020012", "per00000020013", "per00000020014",
			"per00000020015", "per00000020016", "per00000020017", "per00000020018", "per00000020019", "per00000020020",
			"per00000020021", "per00000020022", "per00000020023", "per00000020024", "per00000020025", "per00000020026",
			"per00000020027", "per00000020028", "per00000020029", "per00000020030", "per00000020031", "per00000020032",
			"per00000020033", "per00000020034", "per00000020035", "per00000020036", "per00000020037", "per00000020038",
			"per00000020039", "per00000020040", "per00000020041", "per00000020042", "per00000020043", "per00000020044",
			"per00000020045", "per00000020046", "per00000020047", "per00000020048", "per00000020049", "per00000020050",
			"per00000020051", "per00000020052", "per00000020053", "per00000020054", "per00000020055", "per00000020056",
			"per00000020057", "per00000020058", "per00000020059", "per00000020060", "per00000020061", "per00000020062",
			"per00000020063", "per00000020064", "per00000020065", "per00000020066", "per00000020067", "per00000020068",
			"per00000020069", "per00000020070", "per00000020071", "per00000020072", "per00000020073", "per00000020078",
			"per00000020079", "per00000020080", "per00000020081", "per00000020082", "per00000020083", "per00000020084",
			"per00000020085", "per00000020086", "per00000020087", "per00000020088", "per00000020089", "per00000020090",
			"per00000020091", "per00000020092", "per00000020093", "per00000020094", "per00000020095", "per00000020096",
			"per00000020097",
		}},
		{"", `(from PERSON (where (= vc_classroom.name "Hummingbirds")))`, "id", []string{"per00000000001", "per00000000003", "per00000020036", "per00000020043", "per00000020081", "per00000020090"}},
		{"", `(from MEMBER (where (= guest_of.name_short "Rowan")))`, "person", []string{"per00000000004"}},
		{"", `(from GROUP (where (or (= kind "family") (= replying "everyone"))))`, "id", []string{
			"grp00000000020", "grp00000000030", "grp00000000031", "grp00000000032", "grp00000000033", "grp00000000034", "grp00000000035", "grp00000000036", "grp00000000037", "grp00000000038",
			"grp00000000503", "grp00000020043", "grp00000020044", "grp00000020045",
			"grp00000020046", "grp00000020047", "grp00000020048", "grp00000020049", "grp00000020050", "grp00000020051",
			"grp00000020052", "grp00000020053", "grp00000020054", "grp00000020055", "grp00000020056", "grp00000020057",
			"grp00000020058", "grp00000020059", "grp00000020061", "grp00000020062", "grp00000020063", "grp00000020064",
			"grp00000020065", "grp00000020066", "grp00000020067", "grp00000020068",
		}},
		{"", `(from PERSON (where (blank vc_name)))`, "id", []string{"per00000000004"}},
		{"", `(from GROUP (where (contains name "LL PICnic") (= kind "event")))`, "id", []string{"grp00000000040"}},
		{"", `(from GROUP (where (contains description "picnic")))`, "id", nil},
		{"", `(from GROUP (where (contains kind "ami")))`, "id", []string{
			"grp00000000020", "grp00000000503", "grp00000020043", "grp00000020044", "grp00000020045", "grp00000020046",
			"grp00000020047", "grp00000020048", "grp00000020049", "grp00000020050", "grp00000020051", "grp00000020052",
			"grp00000020053", "grp00000020054", "grp00000020055", "grp00000020056", "grp00000020057", "grp00000020058",
			"grp00000020059", "grp00000020061", "grp00000020062", "grp00000020063", "grp00000020064", "grp00000020065",
			"grp00000020066", "grp00000020067", "grp00000020068",
		}},
		{"", `(from PERSON (where (= (length vc_name) 0)))`, "id", []string{"per00000000004"}},
		{"", `(from MEMBER (where (= (length guest_of.name_short) 5)))`, "person", []string{"per00000000004"}},
		{"", `(from BIRTHDAY_YEAR (where (>= year 2026) (= charity.name "Second Harvest")))`, "id", []string{"bdy00000000001", "bdy00000000003"}},
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

func TestLimitedQueriesKeepTheReadablePrefix(t *testing.T) {
	m := sample(t).Model()
	for _, viewer := range []string{"", parent, student, staff, guest} {
		for _, src := range []string{
			"(from PERSON (columns name_show))",
			"(from MEMBER (where (= member \"yes\")) (include person group))",
			"(from DOCUMENT (columns name))",
			"(from PERSON (order name_sort desc) (columns name_show))",
		} {
			q := mustParse(t, src)
			env := Env{Viewer: viewer, Now: testNow}
			full := m.Run(t.Context(), q, env)
			if full.Err != nil {
				t.Fatal(full.Err)
			}
			checked := 0
			q.scan.conds = append(q.scan.conds, cond{eval: func(*frame) bool {
				checked++
				return true
			}})
			q.limit = 2
			got := m.Run(t.Context(), q, env)
			want := full.IDs[:min(len(full.IDs), q.limit)]
			if got.Err != nil || !slices.Equal(got.IDs, want) {
				t.Fatalf("%s as %q: %v, %v; want %v", src, viewer, got.IDs, got.Err, want)
			}
			wantChecked := len(want)
			if len(q.order) > 0 {
				wantChecked = len(full.IDs)
			}
			if checked != wantChecked {
				t.Errorf("%s as %q checked %d readable matches, want %d", src, viewer, checked, wantChecked)
			}
			for table, rows := range got.Resources {
				for id, row := range rows {
					if !maps.Equal(row, full.Resources[table][id]) {
						t.Errorf("%s as %q changed %s %s: %v", src, viewer, table, id, row)
					}
				}
			}
		}
	}
}

func TestInLiteralsMatchOr(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct{ in, or string }{
		{`(from GROUP (where (in kind "FAMILY" "event")))`, `(from GROUP (where (or (= kind "family") (= kind "event"))))`},
		{`(from DOCUMENT (where (in kind "wiki")))`, `(from DOCUMENT (where (or (= kind "wiki"))))`},
		{`(from PERSON (where (in id "per00000000001" "per00000000003")))`, `(from PERSON (where (or (= id "per00000000001") (= id "per00000000003"))))`},
		{`(from MEMBER (where (in group "grp00000000020" "grp00000000040")))`, `(from MEMBER (where (or (= group "grp00000000020") (= group "grp00000000040"))))`},
	} {
		in := ids(runAs(t, m, "", c.in).Rows(), "id")
		or := ids(runAs(t, m, "", c.or).Rows(), "id")
		if len(in) == 0 || !slices.Equal(in, or) {
			t.Errorf("%s = %v, %s = %v", c.in, in, c.or, or)
		}
	}
}

func TestInAConstantSelectProbes(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct {
		src    string
		probes bool
	}{
		{`(from EFFECTIVE_MEMBER (where (in group (select GROUP.id (= slug "everyone")))))`, true},
		{`(from MEMBER (where (in group (select GROUP.id (= kind "family")))))`, true},
		{`(from GROUP @g (where (in id (select MEMBER.group (= group @g)))))`, false},
		{`(from GROUP (where (in id (descendants (select GROUP.id (in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer))))))))`, true},
	} {
		q := mustParse(t, c.src)
		if probes := q.scan.probeSet != nil; probes != c.probes {
			t.Errorf("%s probes %v, want %v", c.src, probes, c.probes)
		}
	}
	for _, viewer := range []string{"", "per00000000002", "per00000000003"} {
		probed := ids(runAs(t, m, viewer, `(from MEMBER (where (in group (select GROUP.id (= kind "family")))))`).Rows(), "id")
		scanned := ids(runAs(t, m, viewer, `(from MEMBER (where (= group.kind "family")))`).Rows(), "id")
		if len(scanned) == 0 || !slices.Equal(probed, scanned) {
			t.Errorf("as %q: family memberships through a select %v, through a path %v", viewer, probed, scanned)
		}
	}
}

func TestDescendantsAreTheGroupsTheViewerManages(t *testing.T) {
	s := sample(t)
	made := apply(t, s, []Edit{
		{Insert: "GROUP", As: "mgr", Row: map[string]any{"kind": "group", "name": "Chain Managers", "status": "open", "posting": "members", "replying": "members"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@mgr", "person": staff, "member": "yes"}},
		{Insert: "GROUP", As: "root", Row: map[string]any{"kind": "group", "name": "Chain", "status": "open", "posting": "members", "replying": "members", "managed_by": "@mgr"}},
		{Insert: "GROUP", As: "child", Row: map[string]any{"kind": "event", "name": "Chain Child", "status": "open", "posting": "members", "replying": "members", "parent": "@root"}},
		{Insert: "GROUP", As: "grand", Row: map[string]any{"kind": "event", "name": "Chain Grandchild", "status": "open", "posting": "members", "replying": "members", "parent": "@child"}},
		{Insert: "GROUP", As: "other", Row: map[string]any{"kind": "group", "name": "Not Theirs", "status": "open", "posting": "members", "replying": "members"}},
	})
	root, child, grand, other := made[2], made[3], made[4], made[5]
	probe := `(from GROUP @g (where (in id (descendants (select GROUP.id (in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer))))))))`
	walk := `(from GROUP @g (where (manages @g)))`
	for _, viewer := range []string{staff, parent, student} {
		got := ids(as(t, s, viewer, probe), "id")
		want := ids(as(t, s, viewer, walk), "id")
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) || (viewer == staff && len(want) == 0) {
			t.Errorf("as %s: descendants %v, manages %v", viewer, got, want)
		}
	}
	got := ids(as(t, s, staff, probe), "id")
	for _, id := range []string{root, child, grand} {
		if !slices.Contains(got, id) {
			t.Errorf("descendants left out %s", id)
		}
	}
	if slices.Contains(got, other) {
		t.Errorf("descendants included %s", other)
	}
}

func TestDescendantsOfEachOuterRowStayApart(t *testing.T) {
	s := sample(t)
	const nested = `(from GROUP @g (where (exists GROUP (= id @g) (in id (descendants @g)))))`
	for _, viewer := range []string{staff, parent} {
		got := ids(as(t, s, viewer, nested), "id")
		want := ids(as(t, s, viewer, `(from GROUP)`), "id")
		slices.Sort(got)
		slices.Sort(want)
		if len(want) < 2 || !slices.Equal(got, want) {
			t.Errorf("as %s: descendants of each group found %d, the groups themselves %d", viewer, len(got), len(want))
		}
	}
}

func TestAnOrOfProbesProbesEachBranch(t *testing.T) {
	m := sample(t).Model()
	for _, c := range []struct {
		src    string
		probes bool
	}{
		{`(from MEMBER (where (= member "yes") (or (= person @viewer) (in person (household @viewer)))) (columns group person))`, true},
		{`(from MEMBER (where (= member "yes") (or (in group (select WIDGET.visible_to)) (in group (select APP.visible_to)) (in group (select GROUP.visible_to (in parent (select WIDGET.group)))))) (columns group person))`, true},
		{`(from DOCUMENT_GROUP (where (= relation "sent_to") (or (in document (select DOCUMENT.id (= kind "mail") (blank parent) (>= published "2000-01-01"))) (in document (select TODO.document @t (exists TODO_MARK (= todo @t) (= person @viewer)))))) (columns document group))`, true},
		{`(from MEMBER (where (or (in group (select GROUP.id (= kind "party"))) (in group (select GROUP.waitlist (= kind "party"))))))`, true},
		{`(from MEMBER (where (or (= group "grp00000000020") (in group (select GROUP.id (= kind "family"))))))`, true},
		{`(from MEMBER (where (or (= group "grp00000000020") (and (= member "yes") (in group (select GROUP.id (= kind "family")))))))`, false},
	} {
		q := mustParse(t, c.src)
		if probes := q.scan.union != nil; probes != c.probes {
			t.Errorf("%s probes each branch %v, want %v", c.src, probes, c.probes)
		}
		walked := strings.Replace(c.src, "(or ", "(or false ", 1)
		if mustParse(t, walked).scan.union != nil {
			t.Fatalf("%s still probes", walked)
		}
		answered := false
		for _, viewer := range []string{"", student, parent, staff, guest} {
			got := ids(runAs(t, m, viewer, c.src).Rows(), "id")
			want := ids(runAs(t, m, viewer, walked).Rows(), "id")
			if !slices.Equal(got, want) {
				t.Errorf("as %q: %s answers %v, walking the table %v", viewer, c.src, got, want)
			}
			answered = answered || len(want) > 0
		}
		if !answered {
			t.Errorf("%s answers nothing for any viewer", c.src)
		}
	}
}

func TestASelectNamesItsRow(t *testing.T) {
	m := sample(t).Model()
	named := mustParse(t, `(from MEMBER (where (in group (select GROUP.id @f (= @f.kind "family")))))`)
	if named.scan.probeSet == nil {
		t.Error("a select naming its row no longer probes")
	}
	for _, viewer := range []string{"", "per00000000002", "per00000000003"} {
		got := ids(runAs(t, m, viewer, `(from MEMBER (where (in group (select GROUP.id @f (= @f.kind "family")))))`).Rows(), "id")
		want := ids(runAs(t, m, viewer, `(from MEMBER (where (in group (select GROUP.id (= kind "family")))))`).Rows(), "id")
		if len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("as %q: through a named select %v, through an unnamed one %v", viewer, got, want)
		}
	}
	if _, err := Parse(`(from MEMBER (where (in group (select GROUP.id @viewer (= kind "family")))))`); err == nil {
		t.Error("a select named its row @viewer")
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

func TestACorrelatedDescendantWalkPastTheBudgetIsRefused(t *testing.T) {
	const src = `(from GROUP @g (where (in id (descendants (ancestors @g)))))`
	sampleRows := runAs(t, sample(t).Model(), parent, src)
	if sampleRows.Err != nil || len(sampleRows.IDs) < 2 {
		t.Fatalf("the same walk over the sample answered %v with %d rows", sampleRows.Err, len(sampleRows.IDs))
	}
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../sampledata")); err != nil {
		t.Fatal(err)
	}
	const n = 2000
	tree := fmt.Sprintf("grp%011d", 100000)
	row := func(id, parent, name string) map[string]string {
		return map[string]string{"id": id, "parent": parent, "kind": "group", "name": name, "status": "open", "visible_to": "grp00000000004", "posting": "members", "replying": "members"}
	}
	rows := []map[string]string{row(tree, "", "Tree")}
	for i := 1; i < n; i++ {
		rows = append(rows, row(fmt.Sprintf("grp%011d", 100000+i), tree, fmt.Sprintf("Tree %d", i)))
	}
	appendRows(t, filepath.Join(root, "datagroups", "GROUP.csv"), rows)
	dir := &data.Dir{Root: root}
	s, err := NewStore(dir, dir, NewSearchIndex(), testOrigin)
	if err != nil {
		t.Fatal(err)
	}
	if res := runAs(t, s.Model(), parent, src); res.Err == nil || !strings.Contains(res.Err.Error(), "visits more than") {
		t.Errorf("descendants of each group's ancestors answered %v", res.Err)
	}
}

func TestAQueryThatVisitsTooManyRowsIsRefused(t *testing.T) {
	m := sample(t).Model()
	src := `(from PERSON @p (where (exists MEMBER @a (exists MEMBER @b (exists MEMBER @c (and (= @c.note @b.note) (!= @c.note @a.note) (= @c.note @p.name_sort)))))))`
	if res := runAs(t, m, "per00000000002", src); res.Err == nil || !strings.Contains(res.Err.Error(), "visits more than") {
		t.Errorf("a query visiting billions of rows answered %v", res.Err)
	}
	batch := m.RunAll(t.Context(), []*Query{unfiltered(t, src), unfiltered(t, `(from GROUP (where (= id "grp00000000040")))`)}, Env{Viewer: "per00000000002", Now: testNow})
	if batch[0].Err == nil || batch[1].Err == nil || len(batch[1].IDs) != 0 || len(batch[1].Resources["GROUP"]) != 0 {
		t.Errorf("after a query spent the run's visits, the next answered %v with %v", batch[1].Err, batch[1].IDs)
	}
	system := m.Run(t.Context(), unfiltered(t, `(from MEMBER @a (where (exists MEMBER @b (and (= @b.note @a.note) (!= @b.note @a.note)))))`), Env{System: "test", Now: testNow})
	if system.Err != nil {
		t.Errorf("a server job's query was refused: %v", system.Err)
	}
}

func TestALongInIsOneLookupPerRow(t *testing.T) {
	m := sample(t).Model()
	words := []string{}
	for i := range 20000 {
		words = append(words, fmt.Sprintf(`"word-%d"`, i))
	}
	src := `(from PERSON @p (where (exists MEMBER @a (in @a.note ` + strings.Join(words, " ") + `) (= @a.note @p.name_sort))))`
	start := time.Now()
	if res := runAs(t, m, "per00000000002", src); res.Err != nil || len(res.IDs) != 0 {
		t.Errorf("a long in answered %v with %v", res.Err, res.IDs)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Errorf("a 20000-literal in over 36000 row visits took %v", took)
	}
}

func TestIndexLookupsThatFindNothingAreCountedToo(t *testing.T) {
	m := sample(t).Model()
	ids := []string{}
	for i := range 3000 {
		ids = append(ids, fmt.Sprintf(`"memZ%010d"`, i))
	}
	q, err := Parse(`(from PERSON @p (where (exists PERSON @a (exists PERSON @b (exists MEMBER @z (in id ` + strings.Join(ids, " ") + `) (= @z.note @p.name_sort) (= @z.note @a.name_sort) (= @z.note @b.name_sort))))))`)
	if err != nil {
		t.Fatal(err)
	}
	if res := m.Run(t.Context(), q, Env{Viewer: "per00000000002", Now: testNow}); res.Err == nil || !strings.Contains(res.Err.Error(), "visits more than") {
		t.Errorf("a million probes of missing ids answered %v", res.Err)
	}
}

func TestALiteralInOnAnOrderKeyIsExact(t *testing.T) {
	m := sample(t).Model()
	lower := ids(runAs(t, m, "", `(from DOCUMENT (where (in order "a")))`).Rows(), "id")
	upper := ids(runAs(t, m, "", `(from DOCUMENT (where (in order "A")))`).Rows(), "id")
	equal := ids(runAs(t, m, "", `(from DOCUMENT (where (= order "A")))`).Rows(), "id")
	if len(lower) == 0 || len(upper) != 0 || len(equal) != 0 {
		t.Errorf("order keys a, A and = A matched %v, %v and %v", lower, upper, equal)
	}
}

func TestManyFusedSelectsAreCountedToo(t *testing.T) {
	m := scaleStore(t).Model()
	conds := []string{}
	for i := range 1500 {
		conds = append(conds, fmt.Sprintf(`(in id (select DOCUMENT.id (contains name "absent-%d")))`, i))
	}
	q, err := Parse(`(from DOCUMENT (columns name) (where (or ` + strings.Join(conds, " ") + `)))`)
	if err != nil {
		t.Fatal(err)
	}
	if res := m.Run(t.Context(), q, Env{Viewer: scaleViewer, Now: testNow}); res.Err == nil || !strings.Contains(res.Err.Error(), "visits more than") {
		t.Errorf("1500 selects over 10000 documents answered %v", res.Err)
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
	pairs := [][2]Kind{{Text, Text}, {Order, Order}, {Order, Text}, {Text, Order}, {Int, Int}, {Int, Money}, {Date, Date}, {Bool, Bool}, {Ref, Ref}}
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
	r = runAs(t, m, "", `(from RULE (include target))`)
	if len(r.Resources["GROUP"]) != 24 {
		t.Fatalf("resources %v", r.Resources)
	}
}

func TestSum(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{
		{Insert: "PAYMENT", Row: map[string]any{"person": "per00000000002", "group": "grp00000000040", "amount": "12.50", "added": "2026-09-28 18:30"}},
		{Insert: "PAYMENT", Row: map[string]any{"person": "per00000000002", "for": "per00000000004", "group": "grp00000000040", "amount": "7.25", "added": "2026-09-28 18:30"}},
	})
	got := ids(runAs(t, s.Model(), "", `(from GROUP @g (where (= (sum amount PAYMENT (= group @g)) 19.75)))`).Rows(), "id")
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
	if got := effective(t, s, "grp00000000030"); !slices.Equal(got, []string{"per00000000002", "per00000020013", "per00000020018", "per00000020035", "per00000020037", "per00000020038", "per00000020040", "per00000020041", "per00000020080", "per00000020088", "per00000020089"}) {
		t.Fatalf("the Hummingbirds parents list holds %v", got)
	}
	if got := effective(t, s, "grp00000000006"); !slices.Equal(got, []string{"per00000000003"}) {
		t.Fatalf("Who? admins are %v", got)
	}
	if got := effective(t, s, "grp00000000012"); !slices.Equal(got, []string{"per00000000001", "per00000000003", "per00000020009", "per00000020024", "per00000020036", "per00000020043", "per00000020054", "per00000020068", "per00000020081", "per00000020090"}) {
		t.Fatalf("the Jayvens gradeband, through its grades and classrooms, holds %v", got)
	}
	if got := effective(t, s, "grp00000000040"); !slices.Equal(got, []string{"per00000000002", "per00000000003", "per00000000004"}) {
		t.Fatalf("the picnic holds %v", got)
	}
	rows := runAs(t, s.Model(), "per00000000002", `(from GROUP @g (where (exists EFFECTIVE_MEMBER (= group @g) (= person @viewer))) (order id asc))`).Rows()
	if got := ids(rows, "id"); !slices.Equal(got, []string{"grp00000000002", "grp00000000004", "grp00000000007", "grp00000000008", "grp00000000020", "grp00000000030", "grp00000000036", "grp00000000040", "grp00000000042", "grp00000000502", "grp00000000512", "grp00000000513", "grp00000000514", "grp00000000521", "grp00000000701", "grp00000000703", "grp00000020073", "grp00000020075", "grp00000020078"}) {
		t.Fatalf("Rowan is effectively in %v", got)
	}
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000030", "person": "per00000000002", "member": "excluded"}}})
	if got := effective(t, s, "grp00000000030"); len(got) != 10 || slices.Contains(got, "per00000000002") {
		t.Fatalf("an excluded parent stays on the list: %v", got)
	}
}

func TestConcurrentReadsAfterAWriteAgree(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{{Set: "grp00000000030", Cells: map[string]any{"subtitle": "changed"}}})
	m := s.Model()
	src := `(from GROUP @g (where (exists EFFECTIVE_MEMBER (= person @viewer) (= group @g))))`
	want := ids(runAs(t, sample(t).Model(), "per00000000002", src).Rows(), "id")
	got := make([][]string, 16)
	var wg sync.WaitGroup
	for i := range got {
		wg.Go(func() { got[i] = ids(runAs(t, m, "per00000000002", src).Rows(), "id") })
	}
	wg.Wait()
	for _, g := range got {
		if len(want) == 0 || !slices.Equal(g, want) {
			t.Fatalf("concurrent reads answered %v, want %v", g, want)
		}
	}
}

func TestGroupCyclesAreRefused(t *testing.T) {
	s := sample(t)
	for _, c := range []struct {
		name string
		edit Edit
		want string
	}{
		{"two groups taking in each other", Edit{Insert: "RULE", Row: map[string]any{"group": "grp00000000005", "order": "i", "target": "grp00000000006"}}, "take in one another's members"},
		{"a group taking in what is under its own parent", Edit{Insert: "RULE", Row: map[string]any{"group": "grp00000000531", "order": "i", "target": "grp00000000530", "under": "Yes"}}, "takes in its own members"},
		{"a group under itself", Edit{Set: "grp00000000012", Cells: map[string]any{"parent": "grp00000000010"}}, "is under itself"},
	} {
		_, err := applyErr(s, []Edit{c.edit})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestEffectiveDropsDeactivated(t *testing.T) {
	s := sample(t)
	apply(t, s, []Edit{{Set: "per00000000003", Cells: map[string]any{"deactivated": "2026-09-30 08:00"}}})
	if got := effective(t, s, "grp00000000006"); len(got) != 0 {
		t.Fatalf("a deactivated admin stays an admin: %v", got)
	}
}

func TestParentsAndChildrenFollowStudents(t *testing.T) {
	s := sample(t)
	made := apply(t, s, []Edit{
		{Insert: "GROUP", As: "juniParents", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Juni's parents"}},
		{Insert: "RULE", Row: map[string]any{"group": "@juniParents", "order": "a", "person": student, "replace_with": "parents"}},
		{Insert: "GROUP", As: "rowanChildren", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Rowan's children"}},
		{Insert: "RULE", Row: map[string]any{"group": "@rowanChildren", "order": "a", "person": parent, "replace_with": "children"}},
		{Insert: "GROUP", As: "rowanParents", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Rowan's parents"}},
		{Insert: "RULE", Row: map[string]any{"group": "@rowanParents", "order": "a", "person": parent, "replace_with": "parents"}},
	})
	for group, want := range map[string][]string{made[0]: {parent, "per00000020018"}, made[2]: {student, "per00000020019"}, made[4]: nil} {
		if got := effective(t, s, group); !slices.Equal(got, want) {
			t.Errorf("%s holds %v, want %v", group, got, want)
		}
	}
}

func TestRuleSelectors(t *testing.T) {
	s := sample(t)
	tag := apply(t, s, []Edit{
		{Insert: "GROUP", As: "tag", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Test"}},
		{Insert: "RULE", Row: map[string]any{"group": "@tag", "order": "a", "property": "source", "value": "veracross", "within": "grp00000000004"}},
		{Insert: "RULE", Row: map[string]any{"group": "@tag", "order": "b", "exclude": "Yes", "search": "lindqvist"}},
		{Insert: "RULE", Row: map[string]any{"group": "@tag", "order": "c", "person": "per00000000001", "replace_with": "household"}},
	})[0]
	got := effective(t, s, tag)
	if len(got) != 93 || !slices.Contains(got, "per00000000001") || !slices.Contains(got, "per00000000002") {
		t.Fatalf("the tag holds %v", got)
	}
	for _, lindqvist := range []string{"per00000000003", "per00000020020", "per00000020021"} {
		if slices.Contains(got, lindqvist) {
			t.Fatalf("the tag holds %s, whom the search excludes", lindqvist)
		}
	}
	if _, err := applyErr(s, []Edit{{Insert: "RULE", Row: map[string]any{"group": tag, "order": "d", "property": "colour", "value": "red"}}}); err == nil || !strings.Contains(err.Error(), "no PERSON column") {
		t.Fatalf("a rule on no column: %v", err)
	}
}
