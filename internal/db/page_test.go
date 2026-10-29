package db

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"heliosian/internal/trace"
)

func TestTheRailAndDirectoryQueriesAreTheShippedOnes(t *testing.T) {
	qs := pageQueries(t)
	m := sample(t).Model()
	for name, src := range qs {
		q, err := Parse(src)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, src)
		}
		switch name {
		case "navManaged", "groups":
			if q.scan.probeSet == nil {
				t.Errorf("%s does not probe a set", name)
			}
		case "roomGroups":
			if len(q.scan.probes) == 0 && q.scan.probeSet == nil {
				t.Errorf("%s does not probe", name)
			}
		case "effective":
			if q.scan.union == nil {
				t.Errorf("%s does not probe each branch", name)
			}
		}
		for _, viewer := range []string{parent, staff} {
			if res := m.Run(t.Context(), q, Env{Viewer: viewer, Now: testNow}); res.Err != nil {
				t.Errorf("%s as %s: %v", name, viewer, res.Err)
			}
		}
	}
	wide := scaleStore(t).Model()
	const walked = `(from GROUP @g (where (manages @g) (sidebar_group @g)) (order name asc) (include parent parent.parent parent.parent.parent))`
	before := groupReadChecks(t, wide, scaleViewer, walked)
	after := groupReadChecks(t, wide, scaleViewer, qs["navManaged"])
	t.Logf("GROUP read checks: walk %d, shipped navManaged %d", before, after)
	if before < 500 || after*10 > before {
		t.Errorf("shipped navManaged read-checked GROUP %d times, the walk %d", after, before)
	}
	tags := mustParse(t, qs["navTags"])
	if tags.scan.probeCol != "managed_by" || tags.scan.probeSet == nil {
		t.Errorf("navTags probes %q, want managed_by", tags.scan.probeCol)
	}
	kindOnly := strings.Replace(qs["navTags"], "(in managed_by (select EFFECTIVE_MEMBER.group (= person @viewer))) ", "", 1)
	if kindOnly == qs["navTags"] {
		t.Fatal("navTags has no managed_by probe")
	}
	kindChecks := groupReadChecks(t, wide, scaleViewer, kindOnly)
	probed := groupReadChecks(t, wide, scaleViewer, qs["navTags"])
	t.Logf("GROUP read checks: kind probe %d, shipped navTags %d", kindChecks, probed)
	if kindChecks < 100 || probed*10 > kindChecks {
		t.Errorf("shipped navTags read-checked GROUP %d times, the kind probe %d", probed, kindChecks)
	}
	cal := calendarQueries(t)
	if !strings.Contains(cal["places"], "(not (blank capacity))") {
		t.Errorf("places loads members of parties and activities with no capacity: %s", cal["places"])
	}
	if _, err := Parse(cal["places"]); err != nil {
		t.Errorf("places: %v", err)
	}
	for _, name := range []string{"photos", "sources", "dayRules"} {
		q, err := Parse(cal[name])
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if q.scan.probeCol != "group" || q.scan.probeSet == nil {
			t.Errorf("%s does not probe group", name)
		}
	}
	hostedQ, err := Parse(cal["hosted"])
	if err != nil {
		t.Fatalf("hosted: %v", err)
	}
	if hostedQ.scan.probeCol != "id" || hostedQ.scan.probeSet == nil {
		t.Errorf("hosted probes %q, want id", hostedQ.scan.probeCol)
	}
	const walkHosted = `(from GROUP @g (where (in kind "event" "party" "activity") (manages @g)) (columns kind))`
	walkHostedChecks := groupReadChecks(t, wide, scaleViewer, walkHosted)
	shippedHostedChecks := groupReadChecks(t, wide, scaleViewer, cal["hosted"])
	t.Logf("calendar hosted GROUP checks: walk %d, shipped %d", walkHostedChecks, shippedHostedChecks)
	if walkHostedChecks < 500 || shippedHostedChecks*10 > walkHostedChecks {
		t.Errorf("shipped hosted read-checked GROUP %d times, the walk %d", shippedHostedChecks, walkHostedChecks)
	}
	invitesQ, err := Parse(cal["invites"])
	if err != nil {
		t.Fatalf("invites: %v", err)
	}
	if invitesQ.scan.probeCol != "id" || invitesQ.scan.probeSet == nil {
		t.Errorf("invites probes %q, want id", invitesQ.scan.probeCol)
	}
	rail := readSource(t, "../../web/common/rail.js")
	railBind := map[string]string{}
	bindConsts(t, railBind, rail)
	loopLists := fmt.Sprintf(`(from GROUP @g (where (in id (select GROUP.id @g %s))) (order name asc))`, railBind["viewerListCondition"])
	loopQ, err := Parse(loopLists)
	if err != nil {
		t.Fatalf("loop lists: %v", err)
	}
	if loopQ.scan.probeCol != "id" || loopQ.scan.probeSet == nil {
		t.Errorf("loop lists does not probe id set, got %q", loopQ.scan.probeCol)
	}
	const walkLoopLists = `(from GROUP @g (where (in id (select GROUP.id @g (or (manages @g) (in id (select EFFECTIVE_MEMBER.group (= person @viewer))) (in rsvp_yes (select EFFECTIVE_MEMBER.group (= person @viewer)))) (or (!= kind "event") (not (blank rsvp_yes))) (sidebar_group @g)))) (order name asc))`
	walkListChecks := groupReadChecks(t, wide, scaleViewer, walkLoopLists)
	shippedListChecks := groupReadChecks(t, wide, scaleViewer, loopLists)
	t.Logf("loop lists GROUP checks: walk %d, shipped %d", walkListChecks, shippedListChecks)
	if walkListChecks < 500 || shippedListChecks*10 > walkListChecks {
		t.Errorf("shipped loop lists read-checked GROUP %d times, the walk %d", shippedListChecks, walkListChecks)
	}
}

func TestWhoAdminQueriesProbe(t *testing.T) {
	src := readSource(t, "../../web/who/pages/admin.js")
	bind := map[string]string{}
	qs := jsObject(t, bind, src, "queries({", "")
	for name, targetCol := range map[string]string{
		"groups":      "kind",
		"roomParents": "parent",
		"roomRows":    "group",
		"photos":      "group",
	} {
		q, err := Parse(qs[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if q.scan.probeCol != targetCol || (q.scan.probeSet == nil && len(q.scan.probes) == 0) {
			t.Errorf("%s does not probe %s", name, targetCol)
		}
	}
	wide := scaleStore(t).Model()
	walkGroup := `(from GROUP (where (or (in kind "classroom" "grade" "gradeband" "crew" "department") (and (= kind "group") (= parent.kind "gradeband")))) (order order asc name asc))`
	walkChecks := groupReadChecks(t, wide, scaleViewer, walkGroup)
	probedChecks := groupReadChecks(t, wide, scaleViewer, qs["groups"])
	t.Logf("admin groups GROUP checks: walk %d, probed %d", walkChecks, probedChecks)
	walkPhoto := `(from PHOTO (where (in group.kind "event" "party" "activity" "category")) (order order asc) (columns group ready))`
	probedPhoto := qs["photos"]
	walkP, walkG := photoReadChecks(t, wide, scaleViewer, walkPhoto)
	probP, probG := photoReadChecks(t, wide, scaleViewer, probedPhoto)
	t.Logf("photos walk checks: PHOTO %d, GROUP %d; probed: PHOTO %d, GROUP %d", walkP, walkG, probP, probG)
}

func TestHomeAndCalendarQueriesProbe(t *testing.T) {
	homeSrc := readSource(t, "../../web/home/state.js")
	bind := map[string]string{"since": "2026-09-01"}
	qs := jsObject(t, bind, homeSrc, "const homeQueries = since => ({", "")
	for name, col := range map[string]string{
		"appsShown":    "visible_to",
		"widgetsShown": "visible_to",
		"linksShown":   "parent",
	} {
		q, err := Parse(qs[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if q.scan.probeCol != col || q.scan.probeSet == nil {
			t.Errorf("%s does not probe %s", name, col)
		}
	}
	teamSrc := readSource(t, "../../web/home/team.js")
	teamQs := jsObject(t, bind, teamSrc, "export const teamQueries = {", "")
	q, err := Parse(teamQs["headings"])
	if err != nil {
		t.Fatalf("team headings: %v", err)
	}
	if q.scan.probeCol != "parent" || q.scan.probeSet == nil {
		t.Errorf("team headings does not probe parent")
	}
}

func TestAlertsQueriesProbe(t *testing.T) {
	src := readSource(t, "../../web/common/alerts.js")
	bind := map[string]string{}
	qs := jsObject(t, bind, src, "queries({", "")
	q, err := Parse(qs["waiting"])
	if err != nil {
		t.Fatalf("waiting: %v", err)
	}
	if q.scan.probeCol != "id" || q.scan.probeSet == nil {
		t.Errorf("waiting does not probe id set, got col %q probeSet=%v", q.scan.probeCol, q.scan.probeSet != nil)
	}
	wide := scaleStore(t).Model()
	walkWaiting := `(from GROUP @e (where (= kind "event") (not (in status "closed" "cancelled")) (or (>= start today) (>= end today)) (not (manages @e)) (exists MESSAGE @m (= kind "invitation") (= group @e) (exists RECIPIENT (= message @m) (or (= person @viewer) (in person (household @viewer))))) (not (exists MEMBER (= person @viewer) (or (= group @e.rsvp_yes) (= group @e.rsvp_no) (= group @e.hidden_for))))) (order start asc) (columns name start all_day when_href))`
	walkChecks := groupReadChecks(t, wide, scaleViewer, walkWaiting)
	probedChecks := groupReadChecks(t, wide, scaleViewer, qs["waiting"])
	t.Logf("alerts waiting GROUP checks: walk %d, probed %d", walkChecks, probedChecks)
	if walkChecks < 500 || probedChecks*10 > walkChecks {
		t.Errorf("shipped waiting read-checked GROUP %d times, the walk %d", probedChecks, walkChecks)
	}
}

func TestWikiAndDirectoryQueriesBatchAndProbe(t *testing.T) {
	wikiSrc := readSource(t, "../../web/wiki/state.js")
	bind := map[string]string{}
	wikiQs := jsObject(t, bind, wikiSrc, "queries: {", "")
	for _, name := range []string{"pages", "person", "photo", "admin", "cards", "redirects"} {
		if wikiQs[name] == "" {
			t.Fatalf("wiki missing query %s", name)
		}
		if _, err := Parse(wikiQs[name]); err != nil {
			t.Fatalf("wiki query %s: %v", name, err)
		}
	}
	dirSrc := readSource(t, "../../web/common/directory.js")
	dirQs := jsObject(t, bind, dirSrc, "queries({", "")
	for _, name := range []string{"people", "emails", "photos", "members", "roles", "groups"} {
		if dirQs[name] == "" {
			t.Fatalf("directory missing query %s", name)
		}
		if _, err := Parse(dirQs[name]); err != nil {
			t.Fatalf("directory query %s: %v", name, err)
		}
	}
}

func photoReadChecks(t *testing.T, m *Model, viewer, src string) (int, int) {
	t.Helper()
	root := trace.New("request")
	res := m.Run(trace.With(t.Context(), root), mustParse(t, src), Env{Viewer: viewer, Now: testNow})
	if res.Err != nil {
		t.Fatalf("%s: %v", src, res.Err)
	}
	return spanCount(root, "PHOTO"), spanCount(root, "GROUP")
}

func calendarQueries(t *testing.T) map[string]string {
	t.Helper()
	src := readSource(t, "../../web/common/calendar.js")
	bind := map[string]string{}
	bindConsts(t, bind, src)
	out := jsObject(t, bind, src, "export const calendarQueries = {", "")
	if out["places"] == "" {
		t.Fatal("calendar queries have no places")
	}
	return out
}

func spanCountFor(t *testing.T, m *Model, viewer, src, table string) int {
	t.Helper()
	root := trace.New("request")
	res := m.Run(trace.With(t.Context(), root), mustParse(t, src), Env{Viewer: viewer, Now: testNow})
	if res.Err != nil {
		t.Fatalf("%s: %v", src, res.Err)
	}
	return spanCount(root, table)
}

func groupReadChecks(t *testing.T, m *Model, viewer, src string) int {
	return spanCountFor(t, m, viewer, src, "GROUP")
}

func spanCount(s *trace.Span, name string) int {
	if s == nil {
		return 0
	}
	n := 0
	if s.Name == name {
		n += s.Count
	}
	for _, c := range s.Children {
		n += spanCount(c, name)
	}
	return n
}

func pageQueries(tb testing.TB) map[string]string {
	tb.Helper()
	rail := readSource(tb, "../../web/common/rail.js")
	who := readSource(tb, "../../web/who/state.js")
	bind := map[string]string{}
	bindConsts(tb, bind, rail)
	bindConsts(tb, bind, who)
	out := map[string]string{}
	for name, src := range jsObject(tb, bind, rail, "export function navQueries()", "return {") {
		out[name] = src
	}
	for name, src := range jsObject(tb, bind, who, "queries({", "") {
		out[name] = src
	}
	for _, name := range []string{"navManaged", "navJoined", "groups", "roomGroups", "effective"} {
		if out[name] == "" {
			tb.Fatalf("the page queries have no %s", name)
		}
	}
	return out
}

func readSource(tb testing.TB, path string) string {
	tb.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return string(body)
}

var constDecl = regexp.MustCompile(`(?m)^(?:export )?const ([A-Za-z0-9_]+) = `)

func bindConsts(tb testing.TB, bind map[string]string, src string) {
	tb.Helper()
	for _, match := range constDecl.FindAllStringSubmatchIndex(src, -1) {
		value, _, err := parseStringExpr(src, match[1], bind)
		if err == errNotString {
			continue
		}
		if err != nil {
			tb.Fatalf("%s: %v", src[match[2]:match[3]], err)
		}
		bind[src[match[2]:match[3]]] = value
	}
}

func jsObject(tb testing.TB, bind map[string]string, src, marker, brace string) map[string]string {
	tb.Helper()
	at := strings.Index(src, marker)
	if at < 0 {
		tb.Fatalf("no %s", marker)
	}
	i := at
	if brace == "" {
		i = at + len(marker) - 1
	} else {
		rel := strings.Index(src[at:], brace)
		if rel < 0 {
			tb.Fatalf("%s has no object", marker)
		}
		i = at + rel + len(brace) - 1
	}
	if i >= len(src) || src[i] != '{' {
		tb.Fatalf("%s object starts at %q", marker, src[i:])
	}
	obj, _, err := parseObject(src, i, bind)
	if err != nil {
		tb.Fatal(err)
	}
	return obj
}

func parseObject(src string, i int, bind map[string]string) (map[string]string, int, error) {
	if src[i] != '{' {
		return nil, i, fmt.Errorf("page query at %d is not an object", i)
	}
	out := map[string]string{}
	i = skipJS(src, i+1)
	for i < len(src) && src[i] != '}' {
		if strings.HasPrefix(src[i:], "...") {
			i = skipJS(src, i+3)
			if !strings.HasPrefix(src[i:], "navQueries()") {
				return nil, i, fmt.Errorf("page query at %d spreads something other than navQueries()", i)
			}
			i = skipJS(src, i+len("navQueries()"))
			if i < len(src) && src[i] == ',' {
				i = skipJS(src, i+1)
			}
			continue
		}
		name, next, ok := jsIdent(src, i)
		if !ok {
			return nil, i, fmt.Errorf("page query at %d has no name", i)
		}
		next = skipJS(src, next)
		if next >= len(src) || src[next] != ':' {
			return nil, next, fmt.Errorf("page query %s has no value", name)
		}
		value, after, err := parseStringExpr(src, next+1, bind)
		if err != nil {
			return nil, after, fmt.Errorf("page query %s: %w", name, err)
		}
		out[name] = value
		i = skipJS(src, after)
		if i < len(src) && src[i] == ',' {
			i = skipJS(src, i+1)
		}
	}
	if i >= len(src) || src[i] != '}' {
		return nil, i, fmt.Errorf("page query object never closes")
	}
	return out, i + 1, nil
}

func jsIdent(src string, i int) (string, int, bool) {
	if i >= len(src) || !isIdent(src[i]) {
		return "", i, false
	}
	j := i + 1
	for j < len(src) && (isIdent(src[j]) || src[j] >= '0' && src[j] <= '9') {
		j++
	}
	return src[i:j], j, true
}

func isIdent(c byte) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
}

var errNotString = fmt.Errorf("not a string")

func parseStringExpr(src string, i int, bind map[string]string) (string, int, error) {
	i = skipJS(src, i)
	var b strings.Builder
	saw := false
	for i < len(src) {
		if src[i] != '\'' && src[i] != '"' && src[i] != '`' {
			if !saw {
				return "", i, errNotString
			}
			return b.String(), i, nil
		}
		part, next, err := parseJSString(src, i, bind)
		if err != nil {
			return "", next, err
		}
		b.WriteString(part)
		saw = true
		j := skipJS(src, next)
		if j >= len(src) || src[j] != '+' {
			return b.String(), j, nil
		}
		i = skipJS(src, j+1)
	}
	if !saw {
		return "", i, errNotString
	}
	return b.String(), i, nil
}

func parseJSString(src string, i int, bind map[string]string) (string, int, error) {
	quote := src[i]
	var b strings.Builder
	for i = i + 1; i < len(src); i++ {
		switch src[i] {
		case '\\':
			if i+1 >= len(src) {
				return "", i, fmt.Errorf("page query string at %d never closes", i)
			}
			b.WriteByte(src[i+1])
			i++
		case quote:
			return b.String(), i + 1, nil
		case '$':
			if quote == '`' && i+1 < len(src) && src[i+1] == '{' {
				name, next, ok := jsIdent(src, i+2)
				if !ok || next >= len(src) || src[next] != '}' {
					return "", i, fmt.Errorf("page query template at %d is not a name", i)
				}
				value, ok := bind[name]
				if !ok {
					return "", i, fmt.Errorf("page query template names %s before it is defined", name)
				}
				b.WriteString(value)
				i = next
				continue
			}
			b.WriteByte(src[i])
		case '\n':
			if quote != '`' {
				return "", i, fmt.Errorf("page query string at %d breaks the line", i)
			}
			b.WriteByte(src[i])
		default:
			b.WriteByte(src[i])
		}
	}
	return "", i, fmt.Errorf("page query string never closes")
}

func skipJS(src string, i int) int {
	for i < len(src) {
		switch src[i] {
		case ' ', '\t', '\n', '\r':
			i++
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					i++
				}
				if i+1 < len(src) {
					i += 2
				}
				continue
			}
			return i
		default:
			return i
		}
	}
	return i
}
