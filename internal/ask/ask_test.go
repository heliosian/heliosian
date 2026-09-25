package ask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const jordan = "jordan.whitfield@heliosschool.org"

type anyImages struct{}

func (anyImages) Has(string) (bool, error) { return true, nil }

func (anyImages) Prefetch([]string) error { return nil }

type sampleDirectory struct{ model *who.Model }

func (d sampleDirectory) Resolve(email string) string { return d.model.Resolve(email) }

func (d sampleDirectory) Person(email string) (calendar.Person, bool) {
	p := d.model.Person(email)
	if p == nil {
		return calendar.Person{}, false
	}
	return calendar.Person{Email: p.Email, Name: p.FullName, IsStudent: p.IsStudent, IsParent: p.IsParent, IsStaff: p.IsStaff, Grade: p.Grade, Classroom: p.Classroom}, true
}

func (d sampleDirectory) Children(email string) []calendar.Person {
	out := []calendar.Person{}
	for _, key := range d.model.FamilyKeysOf(email) {
		for _, kid := range d.model.Families[key].KidEmails {
			if p, ok := d.Person(kid); ok {
				out = append(out, p)
			}
		}
	}
	return out
}

func (sampleDirectory) Alerts(string) (int, bool)          { return 0, false }
func (sampleDirectory) ClassroomColors() map[string]string { return map[string]string{} }
func (sampleDirectory) GradeColors() map[string]string     { return map[string]string{} }
func (sampleDirectory) People() []calendar.Person          { return nil }
func (sampleDirectory) Lists(string) []calendar.List       { return nil }

func sampleSources(t *testing.T) Sources {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	directory, err := who.LoadModel(dir, anyImages{}, anyImages{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	roster := calendar.Roster{}
	for _, c := range directory.Classrooms {
		room := calendar.Classroom{Name: c.Name, Grades: []string{}}
		for _, g := range directory.Grades {
			if slices.ContainsFunc(directory.People, func(p who.Person) bool { return p.IsStudent && p.Classroom == c.Name && p.Grade == g.Name }) {
				room.Grades = append(room.Grades, g.Name)
				room.Band = g.Band
			}
		}
		for _, crew := range directory.Crews {
			if crew.Classroom == c.Name && crew.Name != "" {
				room.Crews = append(room.Crews, crew.Name)
			}
		}
		roster.Classrooms = append(roster.Classrooms, room)
	}
	calendarCache, err := calendar.NewCache(dir, dir, func() calendar.Roster { return roster }, nil, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	calendarModel := calendarCache.Model()
	teamCache, err := team.NewCache(dir, dir, anyImages{}, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	teamModel := teamCache.Model()
	celebrateCache, err := celebrate.NewCache(dir, dir, anyImages{}, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	celebrateModel := celebrateCache.Model()
	loopCache, err := loop.NewCache(dir, dir, func(string) bool { return false }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	loopModel := loopCache.Model()
	homeCache, err := home.NewCache(dir, dir, anyImages{}, func() []string { return nil }, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	homeModel := homeCache.Model()
	media := blob.NewMemory()
	queue := store.NewQueue()
	artifactsCache, err := artifacts.NewCache(dir, dir, media, artifacts.Fake{}, queue)
	if err != nil {
		t.Fatal(err)
	}
	filer := artifacts.Register(http.NewServeMux(), artifactsCache, artifacts.Fake{}, queue, artifacts.Inbox{Bucket: media})
	saved, err := filepath.Glob("../../sampledata/artifacts/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range saved {
		if err := filer.FileSaved(context.Background(), "test", path); err != nil {
			t.Fatal(err)
		}
	}
	documents := artifactsCache.Model()
	tags := directory.Tags
	lists := directory.RoomParentLists
	return Sources{
		Directory:         func() *who.Model { return directory },
		Tags:              tags,
		Lists:             lists,
		Calendar:          func() *calendar.Model { return calendarModel },
		CalendarDirectory: sampleDirectory{directory},
		Linked:            func(string) []calendar.Linked { return nil },
		Team:              func() *team.Model { return teamModel },
		Celebrate:         func() *celebrate.Model { return celebrateModel },
		Loop:              func() *loop.Model { return loopModel },
		LoopSources: func() loop.Sources {
			return loop.Sources{Directory: directory, Tags: tags, Lists: lists, Shared: directory.SharedTags}
		},
		Links:     func() []home.Category { return homeModel.Categories },
		Alerts:    func(string) (int, bool) { return 0, false },
		Artifacts: func() *artifacts.Model { return documents },
		Embedder:  artifacts.Fake{},
	}
}

func sampleViewer(t *testing.T, email string) *viewer {
	t.Helper()
	return app{sources: sampleSources(t)}.viewer(email)
}

func call(t *testing.T, v *viewer, name, input string) map[string]any {
	t.Helper()
	out, err := v.run(context.Background(), name, json.RawMessage(input))
	if err != nil {
		t.Fatalf("%s %s: %v", name, input, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("%s: %v in %s", name, err, out)
	}
	return result
}

func TestViewerBlockNamesTheFamily(t *testing.T) {
	block := viewerBlock(sampleViewer(t, jordan))
	for _, want := range []string{"Jordan Whitfield", "a parent", "Sam Whitfield, Grade 3, in Jays", "Ella Whitfield, Grade 6", "Robin Whitfield", "they/them", "Room parent for: 3rd / 4th"} {
		if !strings.Contains(block, want) {
			t.Errorf("viewer block lacks %q:\n%s", want, block)
		}
	}
}

func TestLingoReadsTheModels(t *testing.T) {
	words := lingo(sampleViewer(t, jordan))
	for _, want := range []string{"Grade 3: Jayvens", "- Hegrets (Grade 7, Grade 8; Egrets, Herons)", "- Jays (https://who.heliosian.com/classrooms/jays; Jayvens; Grade 3", "Early Dismissal: dropoff 08:15-08:30", "Community:", "Schedule:"} {
		if !strings.Contains(words, want) {
			t.Errorf("lingo lacks %q:\n%s", want, words)
		}
	}
}

func TestRecentBlockListsTheNewestDocuments(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 17, 9, 0, 0, 0, calendar.Location)
	block := recentBlock(v, recentDocuments(v))
	for _, want := range []string{"## Recent documents", "- Friday, September 11, 2026, past (6 days ago): Helios Weekly Newsletter 2026 Sep 11 (key ", "Helios Weekly Newsletter 2026 September 4"} {
		if !strings.Contains(block, want) {
			t.Errorf("recent block lacks %q:\n%s", want, block)
		}
	}
	for _, unwanted := range []string{"Family Camping", "2026, past (18 days ago)"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("recent block reaches back to %q:\n%s", unwanted, block)
		}
	}
	v.now = time.Date(2027, 1, 4, 9, 0, 0, 0, calendar.Location)
	if block := recentBlock(v, recentDocuments(v)); !strings.Contains(block, "No documents have come in") {
		t.Fatalf("a quiet fortnight: %s", block)
	}
}

type recording struct {
	Fake
	requests *[]Request
}

func (r recording) Respond(ctx context.Context, req Request, emit Emitter) (Reply, error) {
	*r.requests = append(*r.requests, req)
	return r.Fake.Respond(ctx, req, emit)
}

func systemMessages(messages []anthropic.BetaMessageParam) []string {
	out := []string{}
	for _, m := range messages {
		if m.Role == anthropic.BetaMessageParamRoleSystem {
			out = append(out, *m.Content[0].GetText())
		}
	}
	return out
}

type transcript struct {
	context []any
	known   []string
}

func (c *transcript) body(t *testing.T, message string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"conversation": "t1", "message": message, "context": c.context, "known": c.known})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func (c *transcript) keep(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	d := done(t, rec)
	c.context = append(c.context, d["messages"].([]any)...)
	c.known = anyStrings(d["known"])
	return d
}

func done(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	for _, frame := range strings.Split(rec.Body.String(), "\n\n") {
		if data, ok := strings.CutPrefix(frame, "event: done\ndata: "); ok {
			out := map[string]any{}
			if err := json.Unmarshal([]byte(data), &out); err != nil {
				t.Fatal(err)
			}
			return out
		}
	}
	t.Fatalf("no done event: %d %s", rec.Code, rec.Body.String())
	return nil
}

func TestChatTellsOfANewDocumentOnce(t *testing.T) {
	sources := sampleSources(t)
	documents := sources.Artifacts()
	current := documents
	sources.Artifacts = func() *artifacts.Model { return current }
	requests := []Request{}
	mux := http.NewServeMux()
	Register(mux, sources, recording{requests: &requests}, claude.NewLimiter())
	handler := auth.Fixed(jordan, mux)
	chat := &transcript{}
	first := chat.keep(t, post(t, handler, chat.body(t, "Anything new?")))
	if known := anyStrings(first["known"]); len(known) == 0 || slices.Contains(known, "late-reminder") {
		t.Fatalf("the first turn knew %v", known)
	}
	arrived := &artifacts.Document{Key: "late-reminder", Title: "Picture Day moves to Friday", Date: time.Now().In(calendar.Location).Format(calendar.DateFormat), Kind: artifacts.KindList}
	current = &artifacts.Model{Documents: append([]*artifacts.Document{arrived}, documents.Documents...)}
	second := chat.keep(t, post(t, handler, chat.body(t, "And now?")))
	if !slices.Contains(anyStrings(second["known"]), "late-reminder") {
		t.Fatalf("the arrival was not kept as known: %v", second["known"])
	}
	chat.keep(t, post(t, handler, chat.body(t, "Still?")))
	if len(requests) != 3 {
		t.Fatalf("%d requests", len(requests))
	}
	if told := systemMessages(requests[0].Messages); len(told) != 0 {
		t.Fatalf("the first turn was told of %v", told)
	}
	messages := requests[1].Messages
	if last := messages[len(messages)-1]; last.Role != anthropic.BetaMessageParamRoleSystem || messages[len(messages)-2].Role != anthropic.BetaMessageParamRoleUser {
		t.Fatalf("the arrival does not follow the question: %v", messages)
	}
	told := systemMessages(messages)
	if len(told) != 1 || !strings.Contains(told[0], "Picture Day moves to Friday (key late-reminder)") || strings.Contains(told[0], "Sep 11") {
		t.Fatalf("second turn told: %v", told)
	}
	if strings.Contains(requests[1].System[1].Text, "late-reminder") || !strings.Contains(requests[2].System[1].Text, "late-reminder") {
		t.Fatal("the prompt lists the arrival only once it has been told")
	}
	third := requests[2].Messages
	if len(systemMessages(third)) != 1 || third[len(third)-1].Role != anthropic.BetaMessageParamRoleUser {
		t.Fatalf("the arrival was told again: %v", systemMessages(third))
	}
}

func TestFindPeopleReadsAParentThroughTheirChildren(t *testing.T) {
	v := sampleViewer(t, jordan)
	result := call(t, v, "find_people", `{"queries":["whitfield","Sam Whit"]}`)
	results := result["results"].([]any)
	if len(results) != 2 || results[0].(map[string]any)["matched"].(float64) != 4 || results[1].(map[string]any)["matched"].(float64) != 1 {
		t.Fatalf("whitfields: %v", result)
	}
	result = call(t, v, "find_people", `{"role":"parent","classroom":"Jays"}`)
	names := []string{}
	for _, p := range result["results"].([]any)[0].(map[string]any)["people"].([]any) {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	if !strings.Contains(strings.Join(names, ","), "Jordan Whitfield") {
		t.Fatalf("parents of Jays: %v", names)
	}
}

func TestGetPersonCarriesTheFamilyAndTeachers(t *testing.T) {
	result := call(t, sampleViewer(t, jordan), "get_person", `{"name":"Sam Whitfield"}`)
	person := result["person"].(map[string]any)
	if person["classroom"] != "Jays" || person["link"] != "https://who.heliosian.com/people/sam.whitfield" {
		t.Fatalf("sam: %v", person)
	}
	if parents := person["parents"].([]any); len(parents) != 2 {
		t.Fatalf("parents: %v", parents)
	}
	if families := result["families"].([]any); len(families) != 1 {
		t.Fatalf("families: %v", families)
	}
}

func TestDayPlanReadsTheViewersClassrooms(t *testing.T) {
	result := call(t, sampleViewer(t, jordan), "day_plan", `{"date":"2026-09-07"}`)
	plans := result["classrooms"].([]any)
	if len(plans) != 2 {
		t.Fatalf("classrooms: %v", plans)
	}
	for _, p := range plans {
		if p.(map[string]any)["dayType"] != "No School" {
			t.Errorf("labor day: %v", p)
		}
	}
}

func TestCalendarEventsSearchesTheYear(t *testing.T) {
	result := call(t, sampleViewer(t, jordan), "calendar_events", `{"query":"thanksgiving"}`)
	events := result["events"].([]any)
	if len(events) != 1 || events[0].(map[string]any)["dayType"] != "No School" {
		t.Fatalf("thanksgiving: %v", events)
	}
}

func TestVolunteerOpportunitiesNameTheHouseholdsSignUps(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 1, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "get_activity", `{"path":"/v/international-night"}`)
	thing := result["thing"].(map[string]any)
	if !strings.Contains(strings.Join(anyStrings(thing["household"]), ","), "you: Co-Chair") {
		t.Fatalf("international night: %v", thing)
	}
}

func TestRolesTakeTheirEventsDay(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 10, 1, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "get_activity", `{"path":"/v/international-night"}`)
	thing := result["thing"].(map[string]any)
	if thing["past"] != true {
		t.Fatalf("international night on October 1: %v", thing)
	}
	under := result["under"].([]any)
	if len(under) == 0 {
		t.Fatal("international night has nothing under it")
	}
	inherited := 0
	for _, u := range under {
		role := u.(map[string]any)
		if role["past"] != true {
			t.Errorf("a role under a past event is not past: %v", role)
		}
		if role["datesFrom"] == "International Night" {
			inherited++
		}
	}
	if inherited == 0 {
		t.Fatalf("no role took the event's day: %v", under)
	}
	listed := call(t, v, "volunteer_opportunities", `{"query":"international"}`)
	if listed["matched"].(float64) != 0 {
		t.Fatalf("things under a past event were listed: %v", listed)
	}
}

func TestPartiesSayWhereTheyStandAgainstToday(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 18, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "parties", `{}`)
	for _, p := range result["parties"].([]any) {
		party := p.(map[string]any)
		if party["past"] == true {
			t.Errorf("a past party was listed: %v", party["title"])
		}
		if party["title"] == "Fondue & Fort Night" && party["whenAgainstToday"] != "tomorrow" {
			t.Errorf("fondue: %v", party["whenAgainstToday"])
		}
	}
	all := call(t, v, "parties", `{"include_past":true}`)
	if len(all["parties"].([]any)) <= len(result["parties"].([]any)) || result["pastPartiesLeftOut"].(float64) == 0 {
		t.Fatalf("past parties: %v left out, %d with them", result["pastPartiesLeftOut"], len(all["parties"].([]any)))
	}
	if got := v.timing("2026-09-10", ""); got != "past (8 days ago)" {
		t.Errorf("timing: %q", got)
	}
}

func TestGroupsShowMembersToManagersAlone(t *testing.T) {
	result := call(t, sampleViewer(t, jordan), "my_groups", `{}`)
	seen := map[string]bool{}
	for _, g := range result["groups"].([]any) {
		group := g.(map[string]any)
		seen[group["address"].(string)] = true
		if group["youManage"] == true && len(anyStrings(group["people"])) == 0 {
			t.Errorf("managed group without members: %v", group)
		}
		if group["youManage"] != true && len(anyStrings(group["people"])) > 0 {
			t.Errorf("unmanaged group lists members: %v", group)
		}
	}
	if !seen["soccer-team@loop.heliosian.com"] || !seen["middle-school-parents@loop.heliosian.com"] {
		t.Fatalf("groups: %v", seen)
	}
}

func TestMyListsAreTheViewersOwn(t *testing.T) {
	result := call(t, sampleViewer(t, jordan), "my_lists", `{}`)
	names := []string{}
	for _, l := range result["magicTags"].([]any) {
		names = append(names, l.(map[string]any)["name"].(string))
	}
	if !strings.Contains(strings.Join(names, ","), "3rd / 4th Parents") {
		t.Fatalf("magic tags: %v", names)
	}
}

func TestSearchDocumentsFindsTheIssueAndReadsIt(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 17, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "search_documents", `{"query":"international night booths"}`)
	if result["documents"].(float64) != 4 || result["newest"] != "2026-09-11" || result["oldest"] != "2026-08-30" {
		t.Fatalf("documents: %v", result)
	}
	passages := result["passages"].([]any)
	if len(passages) == 0 {
		t.Fatal("no passages")
	}
	first := passages[0].(map[string]any)
	if first["section"] != "HCA NEWSLETTER" || !strings.Contains(first["text"].(string), "booth") {
		t.Fatalf("first passage: %v", first)
	}
	if first["published"] != "past (6 days ago)" {
		t.Fatalf("first passage's date: %v", first)
	}
	for _, field := range []string{"url", "channel", "kind", "author"} {
		if _, ok := first[field]; ok {
			t.Fatalf("a mailed document's passage carries %s: %v", field, first)
		}
	}
	issue := call(t, v, "read_document", `{"key":"`+first["key"].(string)+`"}`)
	if !strings.Contains(issue["markdown"].(string), "# A NOTE FROM BEN") || issue["title"] != "Helios Weekly Newsletter 2026 Sep 11" {
		t.Fatalf("issue: %v", issue)
	}
	if _, err := v.run(context.Background(), "read_document", json.RawMessage(`{"key":"nope"}`)); err == nil {
		t.Fatal("an unknown key was read")
	}
}

func TestSearchDocumentsLinksAPage(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 17, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "search_documents", `{"query":"what to bring for family camping tents"}`)
	first := result["passages"].([]any)[0].(map[string]any)
	const url = "https://www.heliosschool.org/student-life/family-camping"
	if first["title"] != "Family Camping" || first["url"] != url {
		t.Fatalf("first passage: %v", first)
	}
	page := call(t, v, "read_document", `{"key":"`+first["key"].(string)+`"}`)
	if page["url"] != url || !strings.Contains(page["markdown"].(string), "## What to Bring") {
		t.Fatalf("page: %v", page)
	}
	for _, field := range []string{"channel", "kind", "author"} {
		if _, ok := page[field]; ok {
			t.Fatalf("the document carries %s: %v", field, page)
		}
	}
}

func TestSearchDocumentsNarrows(t *testing.T) {
	v := sampleViewer(t, jordan)
	v.now = time.Date(2026, 9, 17, 9, 0, 0, 0, calendar.Location)
	result := call(t, v, "search_documents", `{"query":"labor day","until":"2026-09-04"}`)
	if result["searched"].(float64) != 3 {
		t.Fatalf("until: %v", result["searched"])
	}
	if _, err := v.run(context.Background(), "search_documents", json.RawMessage(`{"query":"x","since":"2027-01-01"}`)); err == nil {
		t.Fatal("a range with nothing in it was searched")
	}
	if _, err := v.run(context.Background(), "search_documents", json.RawMessage(`{"query":"x","since":"last summer"}`)); err == nil {
		t.Fatal("a date that is not a date was taken")
	}
}

func TestToolsRunTogether(t *testing.T) {
	v := sampleViewer(t, jordan)
	l := newLinks()
	wg := sync.WaitGroup{}
	for _, c := range []struct{ name, input string }{
		{"day_plan", `{}`}, {"get_classroom", `{}`}, {"my_lists", `{}`}, {"community_links", `{}`},
		{"get_person", `{"name":"Sam Whitfield"}`}, {"search_documents", `{"query":"camping"}`},
	} {
		wg.Go(func() {
			out, err := v.run(context.Background(), c.name, json.RawMessage(c.input))
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
				return
			}
			l.shorten(out)
		})
	}
	wg.Wait()
}

func TestTooMuchIsRefused(t *testing.T) {
	v := sampleViewer(t, jordan)
	if _, err := v.run(context.Background(), "get_classroom", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("classrooms in brief: %v", err)
	}
	if _, err := v.run(context.Background(), "no_such_tool", json.RawMessage(`{}`)); err == nil {
		t.Fatal("an unknown tool ran")
	}
}

func anyStrings(list any) []string {
	out := []string{}
	if list == nil {
		return out
	}
	for _, item := range list.([]any) {
		out = append(out, item.(string))
	}
	return out
}

func serveApp(t *testing.T, responder Responder) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, sampleSources(t), responder, claude.NewLimiter())
	return auth.Fixed(jordan, mux)
}

func post(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestChatStreamsAndKeepsTheConversation(t *testing.T) {
	requests := []Request{}
	mux := http.NewServeMux()
	Register(mux, sampleSources(t), recording{requests: &requests}, claude.NewLimiter())
	handler := auth.Fixed(jordan, mux)
	chat := &transcript{}
	rec := post(t, handler, chat.body(t, "What kind of day is today?"))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("chat: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"event: tool", "event: text", "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("stream lacks %s:\n%s", want, body)
		}
	}
	first := chat.keep(t, rec)
	if first["turns"] != 1.0 || first["text"] == "" || !slices.Equal(anyStrings(first["tools"]), []string{"Checking the day plan"}) {
		t.Fatalf("done lacks the answer for the browser to keep: %v", first)
	}
	if len(chat.context) != 2 || len(chat.known) == 0 {
		t.Fatalf("the browser was handed %d messages and %d known documents", len(chat.context), len(chat.known))
	}
	second := chat.keep(t, post(t, handler, chat.body(t, "And tomorrow?")))
	if second["turns"] != 2.0 || len(requests) != 2 {
		t.Fatalf("second turn: %v after %d requests", second, len(requests))
	}
	sent := requests[1].Messages
	if len(sent) != 3 || sent[0].Role != anthropic.BetaMessageParamRoleUser || sent[1].Role != anthropic.BetaMessageParamRoleAssistant || sent[2].Content[0].OfText.Text != "And tomorrow?" {
		t.Fatalf("the second turn was sent %v", sent)
	}
	if got := sent[1].Content[0].OfText.Text; !strings.HasPrefix(got, fakeAnswer) {
		t.Fatalf("the model's own answer came back changed: %q", got)
	}
}

func TestChatReadsTheBrowsersContextWithKeys(t *testing.T) {
	requests := []Request{}
	mux := http.NewServeMux()
	Register(mux, sampleSources(t), recording{requests: &requests}, claude.NewLimiter())
	handler := auth.Fixed(jordan, mux)
	chat := &transcript{}
	if err := json.Unmarshal([]byte(`[{"role":"user","content":[{"type":"text","text":"Is `+samURL+` here?"}]},{"role":"assistant","content":[{"type":"text","text":"Yes, [Sam](`+samURL+`)."}]},{"role":"user","content":[{"type":"text","text":"Two"}]},{"role":"assistant","content":[{"type":"text","text":"B"}]}]`), &chat.context); err != nil {
		t.Fatal(err)
	}
	d := chat.keep(t, post(t, handler, chat.body(t, "Three")))
	if d["turns"] != 3.0 {
		t.Fatalf("turns %v", d["turns"])
	}
	sent := requests[0].Messages
	if got := sent[1].Content[0].OfText.Text; got != "Yes, [Sam]("+keyOf(samURL)+")." {
		t.Fatalf("the model read %q", got)
	}
	if len(systemMessages(sent)) != 1 {
		t.Fatalf("a chat with no known documents was not told of the recent ones: %v", systemMessages(sent))
	}
}

func TestChatRefusesAConversationPastItsLength(t *testing.T) {
	handler := serveApp(t, Fake{})
	chat := &transcript{context: []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "One"}}}, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text", "text": strings.Repeat("a", maxConversationLength)}}}}}
	rec := post(t, handler, chat.body(t, "Two"))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "new one") {
		t.Fatalf("oversized context: %d %s", rec.Code, rec.Body.String())
	}
	if rec := post(t, handler, `{"message":"Hello","context":{"not":"a list"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("a context that is not a list: %d", rec.Code)
	}
}

type stopping struct {
	cancel context.CancelFunc
	calls  *int
}

func (s stopping) Respond(ctx context.Context, req Request, emit Emitter) (Reply, error) {
	*s.calls++
	if *s.calls > 1 {
		return Fake{}.Respond(ctx, req, emit)
	}
	emit("text", "The first words")
	s.cancel()
	<-ctx.Done()
	return Reply{Text: "The first words"}, ctx.Err()
}

func TestChatLeavesAStoppedAnswerToTheBrowser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	handler := serveApp(t, stopping{cancel: cancel, calls: &calls})
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(`{"message":"What kind of day is today?"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "The first words") || strings.Contains(body, "event: error") || strings.Contains(body, "event: done") {
		t.Fatalf("a stopped answer was answered: %s", body)
	}
}

type wrapped struct {
	inner *httptest.ResponseRecorder
}

func (w wrapped) Header() http.Header         { return w.inner.Header() }
func (w wrapped) Write(b []byte) (int, error) { return w.inner.Write(b) }
func (w wrapped) WriteHeader(status int)      { w.inner.WriteHeader(status) }
func (w wrapped) Unwrap() http.ResponseWriter { return w.inner }

func TestChatStreamsThroughAWrappedWriter(t *testing.T) {
	handler := serveApp(t, Fake{})
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(`{"message":"Hello"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(wrapped{rec}, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: done") || !rec.Flushed {
		t.Fatalf("wrapped chat: %d flushed=%v %s", rec.Code, rec.Flushed, rec.Body.String())
	}
}

func TestChatRefusesEmptyAndOverlongMessages(t *testing.T) {
	handler := serveApp(t, Fake{})
	if rec := post(t, handler, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("blank: %d", rec.Code)
	}
	if rec := post(t, handler, `{"message":"`+strings.Repeat("a", 5000)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("overlong: %d", rec.Code)
	}
}
