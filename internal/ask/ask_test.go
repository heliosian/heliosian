package ask

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/intercept"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

const jordan = "jordan.whitfield@heliosschool.org"

var sampleNow = time.Date(2026, 9, 12, 9, 0, 0, 0, model.Location)

func sampleDir(t *testing.T) *data.Dir {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	if err := dir.Update(model.ConfigApp, "Super Admins", map[string]string{"Email": jordan}, map[string]string{"Email": "someone.else@heliosschool.org"}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct{ sheet, tab, email string }{
		{"apps", "Admins", jordan},
		{"events", "Admins", "dana.hawkins@heliosschool.org"},
		{"celebrate", "Admins", "dana.hawkins@heliosschool.org"},
		{model.CalendarApp, "Admins", "dana.hawkins@heliosschool.org"},
	} {
		if err := dir.Delete(row.sheet, row.tab, map[string]string{"Email": row.email}); err != nil {
			t.Fatal(err)
		}
	}
	for _, sheet := range []string{"apps", "events", "celebrate", "groups", model.CalendarApp} {
		if err := dir.Insert(sheet, "Admins", []map[string]string{{"Email": sampleAdmin}}); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

type sample struct {
	sources Sources
	store   *model.Store
	filer   *model.DocumentFiler
}

func sampleFrom(t *testing.T, dir *data.Dir) sample {
	t.Helper()
	queue := store.NewQueue()
	bucket := blob.NewMemoryBucket()
	embedder, err := artifacts.NewVertex()
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("sample")
	deps := model.Deps{IDKey: key, Photos: testkit.All, Static: testkit.All, Parties: testkit.All, Activities: testkit.All, Home: testkit.All, Objects: bucket, Embedder: embedder}
	s, err := model.NewStore(dir, dir, queue, deps)
	if err != nil {
		t.Fatal(err)
	}
	images := blob.New(bucket)
	calendar := model.RegisterCalendar(http.NewServeMux(), model.CalendarDeps{Store: s, Images: blob.NewImages(images, "when", "celebrate", "team"), Mail: model.CalendarMail{Sender: mailtest.Discard()}, Queue: queue, IDKey: key})
	home := model.RegisterHome(http.NewServeMux(), model.HomeDeps{Store: s, Images: blob.NewImages(images, "home")})
	activities := model.RegisterActivities(http.NewServeMux(), model.ActivitiesDeps{Store: s, Images: blob.NewImages(images, "team"), Calendar: calendar, Mailer: mailtest.Discard()})
	parties := model.RegisterParties(http.NewServeMux(), model.PartiesDeps{Store: s, Images: blob.NewImages(images, "celebrate"), Calendar: calendar, Mailer: mailtest.Discard()})
	feedback := model.RegisterFeedbackAdmin(http.NewServeMux(), s, bucket, nil)
	filer := model.RegisterDocuments(http.NewServeMux(), s, embedder, queue, artifacts.Inbox{Bucket: bucket})
	registry := model.NewRegistry(s, queue, calendar, parties, activities, home, feedback, filer, "")
	saved, err := filepath.Glob("../../sampledata/artifacts/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range saved {
		if err := filer.FileSaved(context.Background(), access.System("test"), path); err != nil {
			t.Fatal(err)
		}
	}
	return sample{sources: Sources{Registry: registry, Now: func() time.Time { return sampleNow }}, store: s, filer: filer}
}

func sampleSources(t *testing.T) sample {
	t.Helper()
	return sampleFrom(t, sampleDir(t))
}

const sampleAdmin = "grace.kim@heliosschool.org"

func requestAs(email string) *http.Request {
	var got *http.Request
	auth.Fixed(email, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = r })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	return got
}

func (s sample) turn(email string) *turn {
	return &turn{reg: s.sources.Registry, r: requestAs(email), clock: s.sources.Now, found: newFound()}
}

func sampleTurn(t *testing.T, email string) *turn {
	t.Helper()
	return sampleSources(t).turn(email)
}

func runTool(tr *turn, name, input string) (string, error) {
	return tr.run(tr.r.Context(), name, json.RawMessage(input))
}

func call(t *testing.T, tr *turn, name, input string) map[string]any {
	t.Helper()
	out, err := runTool(tr, name, input)
	if err != nil {
		t.Fatalf("%s %s: %v", name, input, err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("%s: %v in %s", name, err, out)
	}
	return result
}

func items(v any) []map[string]any {
	out := []map[string]any{}
	list, _ := v.([]any)
	for _, item := range list {
		out = append(out, item.(map[string]any))
	}
	return out
}

func TestViewerBlockNamesTheFamily(t *testing.T) {
	block, err := sampleTurn(t, jordan).viewerBlock()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Jordan Whitfield", "a parent", "Sam Whitfield, Grade 3, in Jays", "Ella Whitfield, Grade 6", "Robin Whitfield", "they/them", "Room parent for: ", "Grade 3"} {
		if !strings.Contains(block, want) {
			t.Errorf("viewer block lacks %q:\n%s", want, block)
		}
	}
	stranger, err := sampleTurn(t, "nobody@heliosschool.org").viewerBlock()
	if err != nil || !strings.Contains(stranger, "whom the directory does not list") {
		t.Fatalf("a stranger: %v\n%s", err, stranger)
	}
}

func TestLingoReadsTheModels(t *testing.T) {
	words, err := sampleTurn(t, jordan).lingo()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Grade 3: Jayvens", "- Hegrets (Grade 7, Grade 8; Egrets, Herons)", "- Jays (https://who.heliosian.com/classrooms/jays; Jayvens; Grade 3", "Early Dismissal: dropoff 08:15-08:30", "Community:", "Schedule:"} {
		if !strings.Contains(words, want) {
			t.Errorf("lingo lacks %q:\n%s", want, words)
		}
	}
}

func TestRecentBlockListsTheNewestDocuments(t *testing.T) {
	tr := sampleTurn(t, jordan)
	tr.clock = func() time.Time { return time.Date(2026, 9, 17, 9, 0, 0, 0, model.Location) }
	recent, err := tr.recentDocuments()
	if err != nil {
		t.Fatal(err)
	}
	block := recentBlock(tr, recent)
	for _, want := range []string{"## Recent documents", "- Friday, September 11, 2026, past (6 days ago): Helios Weekly Newsletter 2026 Sep 11 (id ", "Helios Weekly Newsletter 2026 September 4"} {
		if !strings.Contains(block, want) {
			t.Errorf("recent block lacks %q:\n%s", want, block)
		}
	}
	for _, unwanted := range []string{"Family Camping", "2026, past (18 days ago)"} {
		if strings.Contains(block, unwanted) {
			t.Errorf("recent block reaches back to %q:\n%s", unwanted, block)
		}
	}
	tr.clock = func() time.Time { return time.Date(2027, 1, 4, 9, 0, 0, 0, model.Location) }
	if recent, err = tr.recentDocuments(); err != nil || !strings.Contains(recentBlock(tr, recent), "No documents have come in") {
		t.Fatalf("a quiet fortnight: %v %v", err, recent)
	}
}

type sentMessage struct {
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

type sentRequest struct {
	System []struct {
		Text string `json:"text"`
	} `json:"system"`
	Messages []sentMessage `json:"messages"`
}

var claudeTurns = struct {
	mu    sync.Mutex
	byKey map[string][]sentRequest
}{byKey: map[string][]sentRequest{}}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ask")
	if err != nil {
		panic(err)
	}
	intercept.GoogleLogin(dir)
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.GeocodeHost, intercept.Geocode())
	answer := intercept.Claude()
	intercept.Install(intercept.ClaudeHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var req sentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if last := req.Messages[len(req.Messages)-1]; last.Content[0].Type != "tool_result" {
			key := r.Header.Get("X-Api-Key")
			claudeTurns.mu.Lock()
			claudeTurns.byKey[key] = append(claudeTurns.byKey[key], req)
			claudeTurns.mu.Unlock()
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		answer.ServeHTTP(w, r)
	}))
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func sentTo(t *testing.T) []sentRequest {
	claudeTurns.mu.Lock()
	defer claudeTurns.mu.Unlock()
	return slices.Clone(claudeTurns.byKey[t.Name()])
}

func systemMessages(messages []sentMessage) []string {
	out := []string{}
	for _, m := range messages {
		if m.Role == string(anthropic.BetaMessageParamRoleSystem) {
			out = append(out, m.Content[0].Text)
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

const lateReminder = "Received: by mxa.mailgun.org with SMTP id 3; Sat, 12 Sep 2026 15:00:00 +0000\r\n" +
	"Message-ID: <late-reminder@example.org>\r\n" +
	"Date: Sat, 12 Sep 2026 08:00:00 -0700\r\n" +
	"From: Coach <coach@example.org>\r\n" +
	"To: soccer-team@loop.heliosian.com\r\n" +
	"Subject: Picture Day moves to Friday\r\n" +
	"Content-Type: text/plain; charset=us-ascii\r\n" +
	"\r\n" +
	"Picture Day is on Friday now.\r\n"

func TestChatTellsOfANewDocumentOnce(t *testing.T) {
	t.Parallel()
	s := sampleSources(t)
	mux := http.NewServeMux()
	Register(mux, s.sources, NewClaude(t.Name()), claude.NewLimiter(), []byte("test"), About(func() string { return "Ask" }, func() string { return "" }))
	handler := auth.Fixed(jordan, mux)
	chat := &transcript{}
	first := chat.keep(t, post(t, handler, chat.body(t, "Anything new?")))
	if len(anyStrings(first["known"])) == 0 {
		t.Fatalf("the first turn knew %v", first["known"])
	}
	if err := s.filer.Post(context.Background(), access.System("loop mailer"), "soccer-team", []byte(lateReminder)); err != nil {
		t.Fatal(err)
	}
	second := chat.keep(t, post(t, handler, chat.body(t, "And now?")))
	arrived := slices.DeleteFunc(anyStrings(second["known"]), func(key string) bool { return slices.Contains(anyStrings(first["known"]), key) })
	if len(arrived) != 1 {
		t.Fatalf("the arrival was not kept as known: %v after %v", second["known"], first["known"])
	}
	chat.keep(t, post(t, handler, chat.body(t, "Still?")))
	requests := sentTo(t)
	if len(requests) != 3 {
		t.Fatalf("%d requests", len(requests))
	}
	if told := systemMessages(requests[0].Messages); len(told) != 0 {
		t.Fatalf("the first turn was told of %v", told)
	}
	messages := requests[1].Messages
	if last := messages[len(messages)-1]; last.Role != string(anthropic.BetaMessageParamRoleSystem) || messages[len(messages)-2].Role != string(anthropic.BetaMessageParamRoleUser) {
		t.Fatalf("the arrival does not follow the question: %v", messages)
	}
	told := systemMessages(messages)
	if len(told) != 1 || !strings.Contains(told[0], "Picture Day moves to Friday (id "+arrived[0]) || strings.Contains(told[0], "Sep 11") {
		t.Fatalf("second turn told: %v", told)
	}
	if strings.Contains(requests[1].System[1].Text, "Picture Day moves") || !strings.Contains(requests[2].System[1].Text, "Picture Day moves") {
		t.Fatal("the prompt lists the arrival only once it has been told")
	}
	third := requests[2].Messages
	if len(systemMessages(third)) != 1 || third[len(third)-1].Role != string(anthropic.BetaMessageParamRoleUser) {
		t.Fatalf("the arrival was told again: %v", systemMessages(third))
	}
}

func TestFindPeopleReadsAParentThroughTheirChildren(t *testing.T) {
	tr := sampleTurn(t, jordan)
	results := items(call(t, tr, "find_people", `{"queries":["whitfield","Sam Whit"]}`)["results"])
	if len(results) != 2 || len(items(results[0]["people"])) != 4 || len(items(results[1]["people"])) != 1 {
		t.Fatalf("whitfields: %v", results)
	}
	sam := items(results[1]["people"])[0]
	if sam["classroom"].(map[string]any)["name"] != "Jays" || sam["grade"] != "Grade 3" || len(anyStrings(sam["parents"])) != 2 {
		t.Fatalf("sam: %v", sam)
	}
	names := []string{}
	for _, p := range items(items(call(t, tr, "find_people", `{"role":"parent","classroom":"Jays"}`)["results"])[0]["people"]) {
		names = append(names, p["fullName"].(string))
	}
	if !slices.Contains(names, "Jordan Whitfield") {
		t.Fatalf("parents of Jays: %v", names)
	}
	page := items(call(t, tr, "find_people", `{"limit":3}`)["results"])[0]
	if len(items(page["people"])) != 3 || page["more"] != true {
		t.Fatalf("a full page: %v", page)
	}
}

func TestGetPersonCarriesTheFamilyAndTeachers(t *testing.T) {
	person := call(t, sampleTurn(t, jordan), "get_person", `{"name":"Sam Whitfield"}`)["person"].(map[string]any)
	if person["classroom"].(map[string]any)["name"] != "Jays" || person["link"] != "https://who.heliosian.com/people/sam.whitfield" {
		t.Fatalf("sam: %v", person)
	}
	if len(anyStrings(person["parents"])) != 2 || len(items(person["families"])) != 1 {
		t.Fatalf("sam's parents and families: %v", person)
	}
	several := call(t, sampleTurn(t, jordan), "get_person", `{"name":"whitfield"}`)
	if len(items(several["several"])) != 4 {
		t.Fatalf("several: %v", several)
	}
	if _, err := runTool(sampleTurn(t, jordan), "get_person", `{"name":"nobody at all"}`); err == nil {
		t.Fatal("a name nobody has was found")
	}
}

func TestDayPlanReadsTheViewersClassrooms(t *testing.T) {
	result := call(t, sampleTurn(t, jordan), "day_plan", `{"date":"2026-09-07"}`)
	plans := items(result["plans"])
	if len(plans) != 2 || result["weekday"] != "Monday" {
		t.Fatalf("plans: %v", result)
	}
	for _, p := range plans {
		if p["day-type"].(map[string]any)["name"] != "No School" {
			t.Errorf("labor day: %v", p)
		}
	}
}

func TestCalendarEventsSearchesTheYear(t *testing.T) {
	tr := sampleTurn(t, jordan)
	events := items(call(t, tr, "calendar_events", `{"query":"thanksgiving"}`)["events"])
	if len(events) != 1 || events[0]["day-type"] != "No School" {
		t.Fatalf("thanksgiving: %v", events)
	}
	if tags := fmt.Sprint(events[0]["calendar-tags"], events[0]["classrooms"]); !strings.Contains(tags, "Jays") || !strings.Contains(tags, "Schedule") {
		t.Errorf("thanksgiving's tags by name: %s", tags)
	}
	tagged := items(call(t, tr, "calendar_events", `{"from":"2026-09-01","to":"2026-09-30","tag":"trip"}`)["events"])
	if len(tagged) != 1 || tagged[0]["title"] != "Jays and Ravens Camping" {
		t.Errorf("trips in September: %v", tagged)
	}
}

func TestGetActivityNamesTheViewersPosition(t *testing.T) {
	result := call(t, sampleTurn(t, jordan), "get_activity", `{"id":"international-night"}`)
	thing := result["thing"].(map[string]any)
	if thing["me"].(map[string]any)["position"] != model.PositionCoChair || len(items(result["under"])) == 0 {
		t.Fatalf("international night: %v", result)
	}
}

func TestPrivateVolunteerListAsTeamShowsIt(t *testing.T) {
	const student, stranger = "sam.whitfield@heliosschool.org", "elena.torres@heliosschool.org"
	s := sampleSources(t)
	var room *model.Activity
	for _, root := range s.store.Model().Activities.Activities {
		for _, a := range append([]*model.Activity{root}, root.Descendants()...) {
			if a.Title == "Room Parents" {
				room = a
			}
		}
	}
	if room == nil || !room.VolunteersHidden {
		t.Fatalf("no private Room Parents list in the sample: %+v", room)
	}
	room.Volunteers = append(room.Volunteers, model.Volunteer{Email: jordan, Position: model.PositionOpen}, model.Volunteer{Email: stranger, Position: model.PositionOpen})
	chairs := len(room.CoChairs())
	for _, c := range []struct {
		email  string
		others int
	}{
		{student, 0},
		{jordan, 1},
		{sampleAdmin, 2},
	} {
		thing := call(t, s.turn(c.email), "get_activity", `{"id":"`+room.ID+`"}`)["thing"].(map[string]any)
		open := 0
		for _, v := range items(thing["volunteers"]) {
			if v["position"] == model.PositionOpen {
				open++
			}
		}
		if open != c.others || thing["volunteersHidden"] != true || thing["taken"].(float64) != float64(chairs+2) {
			t.Errorf("%s: %d open volunteers, want %d: %v", c.email, open, c.others, thing)
		}
	}
}

func TestAdminSeesTeamsHiddenThings(t *testing.T) {
	s := sampleSources(t)
	hidden := 0
	for _, root := range s.store.Model().Activities.Activities {
		if root.Status != model.StatusHidden && root.Status != model.StatusPending {
			continue
		}
		hidden++
		input := `{"id":"` + root.ID + `"}`
		if _, err := runTool(s.turn(sampleAdmin), "get_activity", input); err != nil {
			t.Errorf("admin cannot read %s (%s): %v", root.Title, root.Status, err)
		}
		if _, err := runTool(s.turn("nobody@heliosschool.org"), "get_activity", input); err == nil {
			t.Errorf("a stranger read %s (%s)", root.Title, root.Status)
		}
	}
	if hidden == 0 {
		t.Fatal("no hidden or pending thing in the sample, so the test proves nothing")
	}
}

func TestPartiesListTheirTicketsAndPage(t *testing.T) {
	result := call(t, sampleTurn(t, jordan), "parties", `{"include_past":true}`)
	named := 0
	for _, party := range items(result["parties"]) {
		if party["sold"].(float64) > 0 && len(items(party["tickets"])) == 0 {
			t.Errorf("%v: tickets sold but none named", party["title"])
		}
		named += len(items(party["tickets"]))
	}
	if named == 0 {
		t.Fatal("no ticket named anywhere")
	}
	social := items(call(t, sampleTurn(t, jordan), "parties", `{"query":"children social","include_past":true}`)["parties"])
	if len(social) != 1 || social[0]["title"] != "Nerf Blaster Bash" || social[0]["category"] != "Children Social" {
		t.Errorf("parties by category: %v", social)
	}
}

func TestGroupsShowMembersAsLoopDoes(t *testing.T) {
	seen := map[string]bool{}
	for _, group := range items(call(t, sampleTurn(t, jordan), "my_groups", `{}`)["groups"]) {
		seen[group["address"].(string)] = true
		if group["memberCount"].(float64) > 0 && len(items(group["members"])) == 0 {
			t.Errorf("group without its members: %v", group)
		}
	}
	if !seen["soccer-team@loop.heliosian.com"] || !seen["middle-school-parents@loop.heliosian.com"] {
		t.Fatalf("groups: %v", seen)
	}
}

func TestMyListsAreTheViewersOwn(t *testing.T) {
	names := []string{}
	for _, l := range items(call(t, sampleTurn(t, jordan), "my_lists", `{}`)["magicTags"]) {
		names = append(names, l["name"].(string))
	}
	if !slices.Contains(names, "3rd / 4th Parents") {
		t.Fatalf("magic tags: %v", names)
	}
}

func TestSearchDocumentsFindsTheIssueAndReadsIt(t *testing.T) {
	tr := sampleTurn(t, jordan)
	passages := items(call(t, tr, "search_documents", `{"query":"international night booths"}`)["passages"])
	if len(passages) == 0 {
		t.Fatal("no passages")
	}
	first := passages[0]
	if first["section"] != "HCA NEWSLETTER" || !strings.Contains(first["text"].(string), "booth") || first["score"].(float64) <= 0 {
		t.Fatalf("first passage: %v", first)
	}
	document := first["document"].(map[string]any)
	if _, ok := document["url"]; ok {
		t.Fatalf("a mailed document's passage carries a url: %v", first)
	}
	issue := call(t, tr, "read_document", `{"id":"`+document["id"].(string)+`"}`)["document"].(map[string]any)
	if !strings.Contains(issue["markdown"].(string), "# A NOTE FROM BEN") || issue["title"] != "Helios Weekly Newsletter 2026 Sep 11" {
		t.Fatalf("issue: %v", issue)
	}
	if _, err := runTool(tr, "read_document", `{"id":"nope"}`); err == nil {
		t.Fatal("an unknown document was read")
	}
}

func TestSearchDocumentsLinksAPage(t *testing.T) {
	tr := sampleTurn(t, jordan)
	first := items(call(t, tr, "search_documents", `{"query":"what to bring for family camping tents"}`)["passages"])[0]
	document := first["document"].(map[string]any)
	const url = "https://www.heliosschool.org/student-life/family-camping"
	if document["title"] != "Family Camping" || document["url"] != url {
		t.Fatalf("first passage: %v", first)
	}
	page := call(t, tr, "read_document", `{"id":"`+document["id"].(string)+`"}`)["document"].(map[string]any)
	if page["url"] != url || !strings.Contains(page["markdown"].(string), "## What to Bring") {
		t.Fatalf("page: %v", page)
	}
}

func TestSearchDocumentsNarrows(t *testing.T) {
	tr := sampleTurn(t, jordan)
	for _, p := range items(call(t, tr, "search_documents", `{"query":"labor day","until":"2026-09-04"}`)["passages"]) {
		if date := p["document"].(map[string]any)["date"].(string); date > "2026-09-04" {
			t.Errorf("a passage from %s", date)
		}
	}
	if none := items(call(t, tr, "search_documents", `{"query":"x","since":"2027-01-01"}`)["passages"]); len(none) != 0 {
		t.Fatalf("passages from 2027: %v", none)
	}
	if _, err := runTool(tr, "search_documents", `{"query":"x","since":"last summer"}`); err == nil {
		t.Fatal("a date that is not a date was taken")
	}
}

func TestToolsRunTogether(t *testing.T) {
	tr := sampleTurn(t, jordan)
	l := newLinks()
	wg := sync.WaitGroup{}
	for _, c := range []struct{ name, input string }{
		{"day_plan", `{}`}, {"get_classroom", `{}`}, {"my_lists", `{}`}, {"community_links", `{}`},
		{"get_person", `{"name":"Sam Whitfield"}`}, {"search_documents", `{"query":"camping"}`},
	} {
		wg.Go(func() {
			out, err := runTool(tr, c.name, c.input)
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
	tr := sampleTurn(t, jordan)
	if _, err := runTool(tr, "get_classroom", `{}`); err != nil {
		t.Fatalf("classrooms in brief: %v", err)
	}
	if _, err := runTool(tr, "no_such_tool", `{}`); err == nil {
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

func serveApp(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, sampleSources(t).sources, NewClaude(t.Name()), claude.NewLimiter(), []byte("test"), About(func() string { return "Ask" }, func() string { return "" }))
	return auth.Fixed(jordan, mux)
}

func chatKey(t *testing.T, handler http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ask/key", nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("key: %d cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(body.Key)
	if err != nil || len(raw) != 32 {
		t.Fatalf("key %q is not 32 bytes: %v", body.Key, err)
	}
	return body.Key
}

func TestChatKeyIsTheSameEachLoadAndGoesWithTheServerKey(t *testing.T) {
	handler := serveApp(t)
	first := chatKey(t, handler)
	if again := chatKey(t, handler); again != first {
		t.Fatalf("a second load gave %q, not %q", again, first)
	}
	mux := http.NewServeMux()
	Register(mux, sampleSources(t).sources, NewClaude("test"), claude.NewLimiter(), []byte("other"), About(func() string { return "Ask" }, func() string { return "" }))
	if other := chatKey(t, auth.Fixed(jordan, mux)); other == first {
		t.Fatalf("a different server key gave the same chat key")
	}
}

func TestModelNamesTheViewerAndStarters(t *testing.T) {
	rec := httptest.NewRecorder()
	serveApp(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ask/model", nil))
	var body modelView
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("model: %d %s", rec.Code, rec.Body)
	}
	if body.User.Name != "Jordan Whitfield" || body.User.Initial != "J" || !slices.ContainsFunc(body.Starters, func(s string) bool { return strings.HasPrefix(s, "Who teaches ") }) {
		t.Fatalf("model: %+v", body)
	}
}

func post(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestChatStreamsAndKeepsTheConversation(t *testing.T) {
	t.Parallel()
	handler := serveApp(t)
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
	if first["turns"] != 1.0 || first["text"] == "" || !slices.Equal(anyStrings(first["tools"]), []string{findPeople.words}) {
		t.Fatalf("done lacks the answer for the browser to keep: %v", first)
	}
	if len(chat.context) != 4 || len(chat.known) == 0 {
		t.Fatalf("the browser was handed %d messages and %d known documents", len(chat.context), len(chat.known))
	}
	second := chat.keep(t, post(t, handler, chat.body(t, "And tomorrow?")))
	requests := sentTo(t)
	if second["turns"] != 2.0 || len(requests) != 2 {
		t.Fatalf("second turn: %v after %d requests", second, len(requests))
	}
	sent := requests[1].Messages
	if len(sent) != 5 || sent[0].Role != "user" || sent[1].Content[0].Type != "tool_use" || sent[2].Content[0].Type != "tool_result" || sent[3].Role != "assistant" || sent[4].Content[0].Text != "And tomorrow?" {
		t.Fatalf("the second turn was sent %+v", sent)
	}
	if got := sent[3].Content[0].Text; !strings.HasPrefix(got, "This is the sample server") {
		t.Fatalf("the model's own answer came back changed: %q", got)
	}
}

func TestChatReadsTheBrowsersContextWithKeys(t *testing.T) {
	t.Parallel()
	handler := serveApp(t)
	chat := &transcript{}
	if err := json.Unmarshal([]byte(`[{"role":"user","content":[{"type":"text","text":"Is `+samURL+` here?"}]},{"role":"assistant","content":[{"type":"text","text":"Yes, [Sam](`+samURL+`)."}]},{"role":"user","content":[{"type":"text","text":"Two"}]},{"role":"assistant","content":[{"type":"text","text":"B"}]}]`), &chat.context); err != nil {
		t.Fatal(err)
	}
	d := chat.keep(t, post(t, handler, chat.body(t, "Three")))
	if d["turns"] != 3.0 {
		t.Fatalf("turns %v", d["turns"])
	}
	sent := sentTo(t)[0].Messages
	if got := sent[1].Content[0].Text; got != "Yes, [Sam]("+keyOf(samURL)+")." {
		t.Fatalf("the model read %q", got)
	}
	if len(systemMessages(sent)) != 1 {
		t.Fatalf("a chat with no known documents was not told of the recent ones: %v", systemMessages(sent))
	}
}

func TestChatRefusesAConversationPastItsLength(t *testing.T) {
	handler := serveApp(t)
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
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (s stopping) Write(b []byte) (int, error) {
	if strings.Contains(string(b), "event: tool") {
		s.cancel()
	}
	return s.ResponseRecorder.Write(b)
}

func TestChatLeavesAStoppedAnswerToTheBrowser(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	handler := serveApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(`{"message":"What kind of day is today?"}`)).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(stopping{ResponseRecorder: rec, cancel: cancel}, req)
	body := rec.Body.String()
	if !strings.Contains(body, "event: tool") || strings.Contains(body, "event: error") || strings.Contains(body, "event: done") {
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
	t.Parallel()
	handler := serveApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/ask/chat", strings.NewReader(`{"message":"Hello"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(wrapped{rec}, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "event: done") || !rec.Flushed {
		t.Fatalf("wrapped chat: %d flushed=%v %s", rec.Code, rec.Flushed, rec.Body.String())
	}
}

func TestChatRefusesEmptyAndOverlongMessages(t *testing.T) {
	handler := serveApp(t)
	if rec := post(t, handler, `{"message":"   "}`); rec.Code != http.StatusBadRequest {
		t.Errorf("blank: %d", rec.Code)
	}
	if rec := post(t, handler, `{"message":"`+strings.Repeat("a", 5000)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("overlong: %d", rec.Code)
	}
}
