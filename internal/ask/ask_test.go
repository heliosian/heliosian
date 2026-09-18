package ask

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/data"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const jordan = "jordan.whitfield@heliosschool.org"

// anyImages says every picture a sheet names is there, so the models load
// without the bundled files the app serves.
type anyImages struct{}

func (anyImages) Has(string) (bool, error) { return true, nil }

func (anyImages) Prefetch([]string) error { return nil }

// sampleDirectory is the calendar's reading of the directory, over the
// sample model.
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

// sampleSources loads every sample sheet the way the server does, with the
// calendar's roster the directory's classrooms and crews.
func sampleSources(t *testing.T) Sources {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	tables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	directory, err := who.BuildModel(tables, anyImages{}, anyImages{})
	if err != nil {
		t.Fatal(err)
	}
	roster := calendar.Roster{}
	for _, c := range directory.Classrooms {
		room := calendar.Classroom{Name: c.Name, Grades: []string{}}
		for _, crew := range directory.Crews {
			if crew.Classroom == c.Name && crew.Name != "" {
				room.Crews = append(room.Crews, crew.Name)
			}
		}
		roster.Classrooms = append(roster.Classrooms, room)
	}
	calendarTables, err := calendar.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	calendarModel, err := calendar.BuildModel(calendarTables, roster)
	if err != nil {
		t.Fatal(err)
	}
	teamTables, err := team.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	teamModel, err := team.BuildModel(teamTables, anyImages{})
	if err != nil {
		t.Fatal(err)
	}
	celebrateTables, err := celebrate.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	celebrateModel, err := celebrate.BuildModel(celebrateTables, anyImages{})
	if err != nil {
		t.Fatal(err)
	}
	loopTables, err := loop.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	loopModel, err := loop.BuildModel(loopTables)
	if err != nil {
		t.Fatal(err)
	}
	homeTables, err := home.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	homeModel, err := home.BuildModel(homeTables, anyImages{})
	if err != nil {
		t.Fatal(err)
	}
	documents, err := artifacts.LoadDir("../../sampledata/artifacts", artifacts.Fake{})
	if err != nil {
		t.Fatal(err)
	}
	tags := func(owner string) map[string][]string { return who.TagsOf(tables.Tags, directory, owner) }
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
			return loop.Sources{Directory: directory, Tags: tags, Lists: lists, Shared: func(email string) []who.SharedTag {
				return who.SharedTagsOf(tables.Tags, tables.Managers, directory, email)
			}}
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
	for _, want := range []string{"Grade 3: Jayvens", "- Jays (", "Early Dismissal: dropoff 08:15-08:30", "Community:", "Schedule:"} {
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

func TestChatTellsOfANewDocumentOnce(t *testing.T) {
	sources := sampleSources(t)
	documents := sources.Artifacts()
	current := documents
	sources.Artifacts = func() *artifacts.Model { return current }
	requests := []Request{}
	mux := http.NewServeMux()
	Register(mux, sources, recording{requests: &requests})
	handler := auth.Fixed(jordan, mux)
	rec := post(t, handler, `{"message":"Anything new?"}`)
	id := strings.SplitN(strings.SplitN(rec.Body.String(), `"conversation":"`, 2)[1], `"`, 2)[0]
	arrived := &artifacts.Document{Key: "late-reminder", Title: "Picture Day moves to Friday", Date: time.Now().In(calendar.Location).Format(calendar.DateFormat), Kind: artifacts.KindList}
	current = &artifacts.Model{Documents: append([]*artifacts.Document{arrived}, documents.Documents...)}
	post(t, handler, `{"conversation":"`+id+`","message":"And now?"}`)
	post(t, handler, `{"conversation":"`+id+`","message":"Still?"}`)
	if len(requests) != 3 {
		t.Fatalf("%d requests", len(requests))
	}
	if told := systemMessages(requests[0].Messages); len(told) != 0 {
		t.Fatalf("the first turn was told of %v", told)
	}
	second := requests[1].Messages
	if last := second[len(second)-1]; last.Role != anthropic.BetaMessageParamRoleSystem || second[len(second)-2].Role != anthropic.BetaMessageParamRoleUser {
		t.Fatalf("the arrival does not follow the question: %v", second)
	}
	told := systemMessages(second)
	if len(told) != 1 || !strings.Contains(told[0], "Picture Day moves to Friday (key late-reminder)") || strings.Contains(told[0], "Sep 11") {
		t.Fatalf("second turn told: %v", told)
	}
	third := requests[2].Messages
	if len(systemMessages(third)) != 1 || third[len(third)-1].Role != anthropic.BetaMessageParamRoleUser {
		t.Fatalf("the arrival was told again: %v", systemMessages(third))
	}
}

func TestFindPeopleReadsAParentThroughTheirChildren(t *testing.T) {
	v := sampleViewer(t, jordan)
	result := call(t, v, "find_people", `{"query":"whitfield"}`)
	if result["matched"].(float64) != 4 {
		t.Fatalf("whitfields: %v", result)
	}
	result = call(t, v, "find_people", `{"role":"parent","classroom":"Jays"}`)
	names := []string{}
	for _, p := range result["people"].([]any) {
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

// A role under an event has the event's day: once the event has passed,
// so has the role, and the list this year leaves it out.
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
	Register(mux, sampleSources(t), responder)
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
	handler := serveApp(t, Fake{})
	rec := post(t, handler, `{"message":"What kind of day is today?"}`)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("chat: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"event: start", "event: tool", "event: text", "event: done"} {
		if !strings.Contains(body, want) {
			t.Errorf("stream lacks %s:\n%s", want, body)
		}
	}
	id := strings.TrimSpace(strings.SplitN(strings.SplitN(body, `"conversation":"`, 2)[1], `"`, 2)[0])
	rec = post(t, handler, `{"conversation":"`+id+`","message":"And tomorrow?"}`)
	if !strings.Contains(rec.Body.String(), `"turns":2`) {
		t.Fatalf("second turn: %s", rec.Body.String())
	}
	if !strings.Contains(body, `"text":"`) || !strings.Contains(body, `"tools":["Checking the day plan"]`) {
		t.Fatalf("done lacks the answer for the browser to keep: %s", body)
	}
}

// After a restart the server knows no ids; the browser's transcript
// rebuilds the conversation, with the questions already asked counted.
func TestChatRestoresFromTheBrowsersTranscript(t *testing.T) {
	handler := serveApp(t, Fake{})
	turns := `[{"role":"user","text":"One"},{"role":"assistant","text":"A"},{"role":"user","text":"Two"},{"role":"assistant","text":"B"},{"role":"user","text":"unanswered"}]`
	rec := post(t, handler, `{"conversation":"gone","message":"Three","turns":`+turns+`}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"turns":3`) {
		t.Fatalf("restored: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"conversation":"gone"`) {
		t.Fatal("the unknown id was kept")
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

func TestChatKeepsAStoppedAnswer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	handler := serveApp(t, stopping{cancel: cancel, calls: &calls})
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(`{"message":"What kind of day is today?"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, "event: error") || strings.Contains(body, "event: done") {
		t.Fatalf("a stopped answer was answered: %s", body)
	}
	id := strings.TrimSpace(strings.SplitN(strings.SplitN(body, `"conversation":"`, 2)[1], `"`, 2)[0])
	rec = post(t, handler, `{"conversation":"`+id+`","message":"And tomorrow?"}`)
	if !strings.Contains(rec.Body.String(), `"turns":2`) {
		t.Fatalf("the stopped turn was not kept: %s", rec.Body.String())
	}
}

// wrapped hides the recorder's Flush behind an Unwrap, the way the request
// logger's writer does, so the stream has to reach through it.
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

func TestChatRefusesAnotherPersonsConversation(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, sampleSources(t), Fake{})
	rec := post(t, auth.Fixed(jordan, mux), `{"message":"Hello"}`)
	id := strings.SplitN(strings.SplitN(rec.Body.String(), `"conversation":"`, 2)[1], `"`, 2)[0]
	rec = post(t, auth.Fixed("robin.whitfield@heliosschool.org", mux), `{"conversation":"`+id+`","message":"Hello"}`)
	if strings.Contains(rec.Body.String(), `"conversation":"`+id+`"`) || !strings.Contains(rec.Body.String(), `"turns":1`) {
		t.Fatalf("robin continued jordan's conversation: %s", rec.Body.String())
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

func TestLimiterCountsAnHour(t *testing.T) {
	l := newLimiter()
	now := time.Now()
	for i := 0; i < messagesPerHour; i++ {
		if !l.allow(jordan, now) {
			t.Fatalf("message %d refused", i)
		}
	}
	if l.allow(jordan, now) {
		t.Fatal("the message past the limit was allowed")
	}
	if !l.allow(jordan, now.Add(time.Hour+time.Minute)) {
		t.Fatal("an hour later was refused")
	}
}
