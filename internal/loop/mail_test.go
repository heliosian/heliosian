package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/who"
)

type staticFiles struct{}

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("../../web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

type sampleDirectory struct {
	model  *who.Model
	tables *who.Tables
}

func (d sampleDirectory) Resolve(email string) string { return d.model.Resolve(email) }
func (d sampleDirectory) Model() *who.Model           { return d.model }
func (d sampleDirectory) Tags(owner string) map[string][]string {
	return who.TagsOf(d.tables.Tags, d.model, owner)
}
func (d sampleDirectory) Lists(owner string) []who.List { return d.model.RoomParentLists(owner) }
func (d sampleDirectory) Shared(email string) []who.SharedTag {
	return who.SharedTagsOf(d.tables.Tags, d.tables.Managers, d.model, email)
}
func (d sampleDirectory) Person(email string) (Person, bool) {
	p := d.model.Person(email)
	if p == nil {
		return Person{}, false
	}
	return Person{Email: p.Email, Name: p.FullName}, true
}
func (d sampleDirectory) People() []Person          { return nil }
func (d sampleDirectory) Alerts(string) (int, bool) { return 0, false }

func (d sampleDirectory) GradeColors() map[string]string { return nil }

type fakeStore struct {
	raw  []byte
	err  error
	mu   sync.Mutex
	urls []string
}

func (f *fakeStore) Stored(_ context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	f.urls = append(f.urls, url)
	f.mu.Unlock()
	return f.raw, f.err
}

type sent struct {
	from string
	to   []string
	raw  []byte
}

type fakeSender struct {
	mu    sync.Mutex
	sends []sent
}

func (f *fakeSender) SendRaw(_ context.Context, from string, to []string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends = append(f.sends, sent{from, to, raw})
	return nil
}

func (f *fakeSender) all() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent{}, f.sends...)
}

type fakeArchive struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (f *fakeArchive) Put(_ context.Context, name, mimeType string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if mimeType != mailType {
		return fmt.Errorf("type %s", mimeType)
	}
	f.objects[name] = content
	return nil
}

func (f *fakeArchive) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

const signingKey = "mailgun-signing-key"

type harness struct {
	t         *testing.T
	mux       *http.ServeMux
	dir       *data.Dir
	cache     *Cache
	directory sampleDirectory
	sender    *fakeSender
	archive   *fakeArchive
	mailbox   Mail
}

func newHarness(t *testing.T, store Fetcher) *harness {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	whoTables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(whoTables, nil, staticFiles{})
	if err != nil {
		t.Fatal(err)
	}
	queue := who.NewQueue()
	cache, err := NewCache(dir, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, mux: http.NewServeMux(), dir: dir, cache: cache, directory: sampleDirectory{model, whoTables}, sender: &fakeSender{}, archive: &fakeArchive{objects: map[string][]byte{}}}
	h.mailbox = Mail{Sender: h.sender, Store: store, SigningKey: signingKey, Key: []byte("key"), Base: "https://loop.test", Archive: h.archive}
	Register(h.mux, cache, dir, queue, nil, h.directory, func() []string { return nil }, h.mailbox, nil)
	return h
}

func (h *harness) post(path, body string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	return rec
}

var formHeader = http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}

var jsonHeader = http.Header{"Content-Type": {"application/json"}}

func signed(fields map[string]string) map[string]string {
	stamp, sig := mail.SignMailgun(signingKey, "token-"+fields["message-url"]+fields["recipient"], time.Now())
	fields["timestamp"] = stamp
	fields["token"] = "token-" + fields["message-url"] + fields["recipient"]
	fields["signature"] = sig
	return fields
}

func notify(id, recipient string) map[string]string {
	return signed(map[string]string{
		"recipient":   recipient,
		"sender":      "alice@gmail.com",
		"from":        "Alice Smith <alice@gmail.com>",
		"subject":     "Re: Saturday's game",
		"message-url": "https://sw.api.mailgun.net/v3/domains/loop.heliosian.com/messages/" + id,
	})
}

func (h *harness) inbound(fields map[string]string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(fields)
	return h.post("/hooks/mail", string(body), jsonHeader)
}

func (h *harness) inboundForm(fields map[string]string) *httptest.ResponseRecorder {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	return h.post("/hooks/mail", form.Encode(), formHeader)
}

func (h *harness) rows(tab string) []map[string]string {
	_, rows, err := h.dir.Table(appName, tab)
	if err != nil {
		h.t.Fatal(err)
	}
	return rows
}

func (h *harness) waitFor(what string, ok func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("waited in vain for %s", what)
}

func (h *harness) messageRow(id, group string) map[string]string {
	for _, row := range h.rows(messagesTab) {
		if row["ID"] == id && row["Group"] == group {
			return row
		}
	}
	return nil
}

func (h *harness) messageState(id, group string) string {
	return h.messageRow(id, group)["State"]
}

func (h *harness) members(name string) []string {
	return Members(*h.cache.Model().Group(name), SourcesOf(h.directory))
}

var unsubscribeLink = regexp.MustCompile(`List-Unsubscribe: <mailto:unsubscribe@loop\.heliosian\.com\?subject=([^>]+)>, <https://loop\.test/open/unsubscribe/([^>]+)>`)

func TestAPostIsForwardedToEveryMemberOnce(t *testing.T) {
	store := &fakeStore{raw: []byte(post)}
	h := newHarness(t, store)
	if rec := h.inbound(notify("m1", "soccer-team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState("m1", "soccer-team") == stateSent })
	if len(store.urls) != 1 || store.urls[0] != "https://sw.api.mailgun.net/v3/domains/loop.heliosian.com/messages/m1" {
		t.Fatalf("fetched %v", store.urls)
	}
	sends := h.sender.all()
	want := h.members("soccer-team")
	if len(want) < 3 || len(sends) != len(want) {
		t.Fatalf("%d sends for %d members", len(sends), len(want))
	}
	seen := map[string]bool{}
	for _, s := range sends {
		if s.from != "soccer-team@loop.heliosian.com" || len(s.to) != 1 {
			t.Fatalf("send %+v", s)
		}
		seen[s.to[0]] = true
		out := string(s.raw)
		m := unsubscribeLink.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("no unsubscribe link in:\n%s", out)
		}
		if m[1] != m[2] {
			t.Fatalf("the mailto and https tokens differ: %s %s", m[1], m[2])
		}
		name, email, ok := parseToken(h.mailbox.Key, m[2])
		if !ok || name != "soccer-team" || email != s.to[0] {
			t.Fatalf("token for %s reads %q %q %v", s.to[0], name, email, ok)
		}
		if !strings.Contains(out, "List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n") || !strings.Contains(out, "Subject: Re: [Soccer Team Families] Saturday's game\r\n") || !strings.Contains(out, "To: soccer-team@loop.heliosian.com\r\n") || !strings.HasSuffix(out, "\r\n\r\nSee you at 9.\r\n") {
			t.Fatalf("headers:\n%s", out)
		}
	}
	for _, m := range want {
		if !seen[m] {
			t.Errorf("%s got nothing", m)
		}
	}
	if h.archive.count() != 1 {
		t.Fatalf("archive holds %d objects", h.archive.count())
	}
	for name, content := range h.archive.objects {
		if !strings.HasPrefix(name, "loop/soccer-team/") || !strings.HasSuffix(name, "-m1.eml") || string(content) != post {
			t.Fatalf("archived %s: %q", name, content)
		}
	}
	row := h.messageRow("m1", "soccer-team")
	if row["Recipients"] != fmt.Sprint(len(want)) || row["From"] != "Alice Smith <alice@gmail.com>" || row["Subject"] != "Re: Saturday's game" || row["Object"] == "" || row["Detail"] != "" || row["Received"] == "" || row["Source"] != store.urls[0] {
		t.Fatalf("row %+v", row)
	}
	if rec := h.inbound(notify("m1", "soccer-team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("replay answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != len(want) {
		t.Fatal("a replayed notification sent the post again")
	}
}

func TestAFormNotificationIsTakenToo(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	if rec := h.inboundForm(notify("m7", "Soccer-Team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState("m7", "soccer-team") == stateSent })
}

func TestOneClickUnsubscribeTakesThemOffTheGroup(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	h.inbound(notify("m2", "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState("m2", "soccer-team") == stateSent })
	first := h.sender.all()[0]
	tok := unsubscribeLink.FindStringSubmatch(string(first.raw))[2]
	req := httptest.NewRequest(http.MethodGet, "/open/unsubscribe/"+tok, nil)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Soccer Team Families") || !strings.Contains(rec.Body.String(), first.to[0]) {
		t.Fatalf("page %d: %s", rec.Code, rec.Body)
	}
	form := url.Values{"List-Unsubscribe": {"One-Click"}}.Encode()
	rec = h.post("/open/unsubscribe/"+tok, form, formHeader)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("one-click answered %d: %s", rec.Code, rec.Body)
	}
	g := h.cache.Model().Group("soccer-team")
	if !g.HasExcluded(first.to[0]) || g.Excluded[0].Note != "Unsubscribed by one-click" {
		t.Fatalf("not excluded: %+v", g.Excluded)
	}
	for _, m := range h.members("soccer-team") {
		if m == first.to[0] {
			t.Fatal("still a member")
		}
	}
	rowsFor := func() int {
		n := 0
		for _, row := range h.rows(excludedTab) {
			if row["Group"] == "soccer-team" && row["Email"] == first.to[0] && row["Note"] == "Unsubscribed by one-click" && row["Timestamp"] != "" {
				n++
			}
		}
		return n
	}
	h.waitFor("the row", func() bool { return rowsFor() == 1 })
	if rec = h.post("/open/unsubscribe/"+tok, form, formHeader); rec.Code != http.StatusOK {
		t.Fatalf("second click answered %d", rec.Code)
	}
	if rowsFor() != 1 {
		t.Fatal("the second click added a second row")
	}
	if rec = h.post("/open/unsubscribe/"+tok[:len(tok)-3]+"xyz", form, formHeader); rec.Code != http.StatusNotFound {
		t.Fatalf("a forged token answered %d", rec.Code)
	}
	rec = h.post("/open/unsubscribe/"+tok, "", formHeader)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no more mail") {
		t.Fatalf("the form answered %d: %s", rec.Code, rec.Body)
	}
}

func TestUnsubscribeByMailTakesThemOffTheGroup(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	h.inbound(notify("m8", "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState("m8", "soccer-team") == stateSent })
	first := h.sender.all()[0]
	tok := unsubscribeLink.FindStringSubmatch(string(first.raw))[1]
	fields := signed(map[string]string{
		"recipient":   "unsubscribe@loop.heliosian.com",
		"sender":      first.to[0],
		"from":        "Someone <" + first.to[0] + ">",
		"subject":     "Re: " + tok,
		"message-url": "https://sw.api.mailgun.net/v3/domains/loop.heliosian.com/messages/u1",
	})
	if rec := h.inbound(fields); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	g := h.cache.Model().Group("soccer-team")
	if !g.HasExcluded(first.to[0]) || g.Excluded[0].Note != "Unsubscribed by mail from "+first.to[0] {
		t.Fatalf("the mail did not exclude them: %+v", g.Excluded)
	}
	time.Sleep(100 * time.Millisecond)
	if h.messageRow("u1", "soccer-team") != nil || h.messageRow("u1", "unsubscribe") != nil {
		t.Fatal("the unsubscribe mail was taken as a post")
	}
}

func TestAnAliasReachesItsGroup(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	if rec := h.inbound(notify("m9", "Hummingbirds-Families@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState("m9", "hummingbird-families") == stateSent })
	if h.messageRow("m9", "hummingbirds-families") != nil {
		t.Fatal("the alias got a row of its own")
	}
	if len(h.sender.all()) != len(h.members("hummingbird-families")) {
		t.Fatalf("%d sends", len(h.sender.all()))
	}
}

func TestALoopedPostIsDropped(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte("From: a@x.org\r\nTo: soccer-team@loop.heliosian.com\r\nX-Helios-Loop: pta\r\n\r\nhi\r\n")})
	h.inbound(notify("m3", "soccer-team@loop.heliosian.com"))
	h.waitFor("the drop", func() bool { return h.messageState("m3", "soccer-team") == stateDropped })
	if len(h.sender.all()) != 0 || h.archive.count() != 0 {
		t.Fatal("a looped post went out")
	}
}

func TestAFetchFailureIsRecordedAndRetriedOnReplay(t *testing.T) {
	store := &fakeStore{raw: []byte(post), err: fmt.Errorf("mailgun is down")}
	h := newHarness(t, store)
	h.inbound(notify("m4", "soccer-team@loop.heliosian.com"))
	h.waitFor("the failure", func() bool { return h.messageState("m4", "soccer-team") == stateFailed })
	store.err = nil
	h.inbound(notify("m4", "soccer-team@loop.heliosian.com"))
	h.waitFor("the retry", func() bool { return h.messageState("m4", "soccer-team") == stateSent })
	if len(h.sender.all()) == 0 {
		t.Fatal("the retry sent nothing")
	}
}

func TestMailForNoGroupIsIgnored(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	before := len(h.rows(messagesTab))
	if rec := h.inbound(notify("m5", "nobody@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != 0 || len(h.rows(messagesTab)) != before {
		t.Fatal("mail for no group was taken")
	}
}

func event(kind, severity, from, recipient, detail string) string {
	stamp, sig := mail.SignMailgun(signingKey, "token-"+kind+recipient, time.Now())
	body, _ := json.Marshal(map[string]any{
		"signature": map[string]string{"timestamp": stamp, "token": "token-" + kind + recipient, "signature": sig},
		"event-data": map[string]any{
			"event": kind, "severity": severity, "recipient": recipient, "reason": "bounce",
			"message":         map[string]any{"headers": map[string]string{"message-id": "abc@gmail.com", "from": from}},
			"delivery-status": map[string]any{"message": "", "description": detail, "code": 550},
		},
	})
	return string(body)
}

func TestBouncesAreRecordedAgainstTheGroup(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	before := len(h.rows(deliveriesTab))
	from := "Alice via Soccer Team Families <soccer-team@loop.heliosian.com>"
	if rec := h.post("/hooks/events", event("failed", "permanent", from, "Gone@example.org", "550 no such user"), jsonHeader); rec.Code != http.StatusOK {
		t.Fatalf("events answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the delivery row", func() bool {
		for _, row := range h.rows(deliveriesTab) {
			if row["Group"] == "soccer-team" && row["Email"] == "gone@example.org" && row["Event"] == "bounced" && row["Message"] == "abc@gmail.com" && row["Detail"] == "550 no such user" {
				return true
			}
		}
		return false
	})
	h.post("/hooks/events", event("failed", "temporary", from, "slow@example.org", "greylisted"), jsonHeader)
	h.post("/hooks/events", event("complained", "", from, "cross@example.org", ""), jsonHeader)
	h.post("/hooks/events", event("delivered", "", from, "fine@example.org", ""), jsonHeader)
	h.post("/hooks/events", event("failed", "permanent", "HCA-Team <team@heliosian.com>", "other@example.org", "550"), jsonHeader)
	h.waitFor("the other rows", func() bool { return len(h.rows(deliveriesTab)) == before+3 })
	kinds := map[string]string{}
	for _, row := range h.rows(deliveriesTab)[before:] {
		kinds[row["Email"]] = row["Event"]
	}
	if kinds["slow@example.org"] != "delivery_delayed" || kinds["cross@example.org"] != "complained" {
		t.Fatalf("kinds %v", kinds)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.rows(deliveriesTab)) != before+3 {
		t.Fatalf("a delivery or another app's bounce was recorded: %+v", h.rows(deliveriesTab))
	}
}

func TestHistoryListsSentMessagesWithTheirBounces(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	h.inbound(notify("m10", "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState("m10", "soccer-team") == stateSent })
	if id := h.messageRow("m10", "soccer-team")["Message ID"]; id != "abc@gmail.com" {
		t.Fatalf("message id %q", id)
	}
	from := "Alice via Soccer Team Families <soccer-team@loop.heliosian.com>"
	h.post("/hooks/events", event("failed", "permanent", from, "gone@example.org", "550 no such user"), jsonHeader)
	h.waitFor("the delivery row", func() bool {
		for _, row := range h.rows(deliveriesTab) {
			if row["Email"] == "gone@example.org" {
				return true
			}
		}
		return false
	})
	rec := h.as("jordan.whitfield@heliosschool.org", http.MethodGet, "/api/loop/messages?name=soccer-team", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("messages answered %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Messages []SentMessage `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 2 {
		t.Fatalf("messages %+v", body.Messages)
	}
	m := body.Messages[0]
	if m.Subject != "Re: Saturday's game" || m.From.Email != "alice@gmail.com" || m.Recipients != len(h.members("soccer-team")) || len(m.Trouble) != 1 || m.Trouble[0].Email != "gone@example.org" || m.Trouble[0].Event != "bounced" {
		t.Fatalf("newest message %+v", m)
	}
	if sample := body.Messages[1]; sample.From.Name != "Jordan Whitfield" || len(sample.Trouble) != 1 || sample.Trouble[0].Email != "office@coastsidesoccer.example.org" {
		t.Fatalf("sample message %+v", sample)
	}
	if (app{cache: h.cache}).sentCount("soccer-team") != 2 {
		t.Fatal("the sent count is not the history's length")
	}
	if rec := h.as("ruth.amari@heliosschool.org", http.MethodGet, "/api/loop/messages?name=soccer-team", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-manager got %d", rec.Code)
	}
}

func TestRoutesRefuseTheUnsignedAndTheUnconfigured(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	fields := notify("m6", "soccer-team@loop.heliosian.com")
	fields["signature"] = "0000"
	if rec := h.inbound(fields); rec.Code != http.StatusNotAcceptable {
		t.Fatalf("a badly signed notification answered %d", rec.Code)
	}
	body := event("failed", "permanent", "x <soccer-team@loop.heliosian.com>", "a@example.org", "x")
	if rec := h.post("/hooks/events", strings.Replace(body, `"signature":"`, `"signature":"ff`, 1), jsonHeader); rec.Code != http.StatusNotAcceptable {
		t.Fatalf("a badly signed event answered %d", rec.Code)
	}
	bare := app{cache: h.cache, mail: Mail{}}
	rec := httptest.NewRecorder()
	bare.inbound(rec, httptest.NewRequest(http.MethodPost, "/hooks/mail", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the unconfigured inbound route answered %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	bare.events(rec, httptest.NewRequest(http.MethodPost, "/hooks/events", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the unconfigured events route answered %d", rec.Code)
	}
}
