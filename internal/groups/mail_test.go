package groups

import (
	"context"
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
func (d sampleDirectory) Model() *who.Model          { return d.model }
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

type fakeInbox struct {
	raw []byte
	err error
}

func (f *fakeInbox) Received(_ context.Context, id string) (mail.Received, error) {
	return mail.Received{ID: id, From: "alice@gmail.com", Subject: "Re: Saturday's game", Raw: f.raw}, f.err
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

const secret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"

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

func newHarness(t *testing.T, inbox Inbox) *harness {
	t.Helper()
	sendInterval = 0
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
	h.mailbox = Mail{Sender: h.sender, Inbox: inbox, Secret: secret, Key: []byte("key"), Base: "https://loop.test", Archive: h.archive}
	Register(h.mux, cache, dir, queue, nil, h.directory, func() []string { return nil }, h.mailbox)
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

func (h *harness) webhook(body string) *httptest.ResponseRecorder {
	return h.post("/api/loop/mail", body, mail.SignWebhook(secret, "msg_1", time.Now(), []byte(body)))
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

func (h *harness) messageState(id, group string) string {
	for _, row := range h.rows(messagesTab) {
		if row["ID"] == id && row["Group"] == group {
			return row["State"]
		}
	}
	return ""
}

func (h *harness) members(name string) []string {
	return Members(*h.cache.Model().Group(name), SourcesOf(h.directory))
}

const receivedEvent = `{"type":"email.received","created_at":"2026-09-16T10:00:00Z","data":{"email_id":"%s","from":"alice@gmail.com","to":["Soccer Team Families <soccer-team@loop.heliosian.com>"],"subject":"Re: Saturday's game","message_id":"<abc@gmail.com>"}}`

var unsubscribeLink = regexp.MustCompile(`List-Unsubscribe: <https://loop.test/unsubscribe/([^>]+)>`)

func TestAPostIsForwardedToEveryMemberOnce(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte(post)})
	if rec := h.webhook(fmt.Sprintf(receivedEvent, "m1")); rec.Code != http.StatusNoContent {
		t.Fatalf("webhook answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState("m1", "soccer-team") == stateSent })
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
		name, email, ok := parseToken(h.mailbox.Key, m[1])
		if !ok || name != "soccer-team" || email != s.to[0] {
			t.Fatalf("token for %s reads %q %q %v", s.to[0], name, email, ok)
		}
		if !strings.Contains(out, "List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n") || !strings.Contains(out, "Subject: Re: [Soccer Team Families] Saturday's game\r\n") || !strings.HasSuffix(out, "\r\n\r\nSee you at 9.\r\n") {
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
	var row map[string]string
	for _, r := range h.rows(messagesTab) {
		if r["ID"] == "m1" {
			row = r
		}
	}
	if row["Recipients"] != fmt.Sprint(len(want)) || row["From"] != "alice@gmail.com" || row["Subject"] != "Re: Saturday's game" || row["Object"] == "" || row["Detail"] != "" || row["Received"] == "" {
		t.Fatalf("row %+v", row)
	}
	if rec := h.webhook(fmt.Sprintf(receivedEvent, "m1")); rec.Code != http.StatusNoContent {
		t.Fatalf("replay answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != len(want) {
		t.Fatal("a replayed webhook sent the post again")
	}
}

func TestOneClickUnsubscribeTakesThemOffTheGroup(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte(post)})
	h.webhook(fmt.Sprintf(receivedEvent, "m2"))
	h.waitFor("the forward", func() bool { return h.messageState("m2", "soccer-team") == stateSent })
	first := h.sender.all()[0]
	tok := unsubscribeLink.FindStringSubmatch(string(first.raw))[1]
	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/"+tok, nil)
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Soccer Team Families") || !strings.Contains(rec.Body.String(), first.to[0]) {
		t.Fatalf("page %d: %s", rec.Code, rec.Body)
	}
	form := url.Values{"List-Unsubscribe": {"One-Click"}}.Encode()
	formHeader := http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}
	rec = h.post("/unsubscribe/"+tok, form, formHeader)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("one-click answered %d: %s", rec.Code, rec.Body)
	}
	g := h.cache.Model().Group("soccer-team")
	if !g.HasUnsubscribed(first.to[0]) {
		t.Fatalf("not unsubscribed: %+v", g.Unsubscribed)
	}
	for _, m := range h.members("soccer-team") {
		if m == first.to[0] {
			t.Fatal("still a member")
		}
	}
	rowsFor := func() int {
		n := 0
		for _, row := range h.rows(unsubscribedTab) {
			if row["Group"] == "soccer-team" && row["Email"] == first.to[0] && row["Timestamp"] != "" {
				n++
			}
		}
		return n
	}
	h.waitFor("the row", func() bool { return rowsFor() == 1 })
	if rec = h.post("/unsubscribe/"+tok, form, formHeader); rec.Code != http.StatusOK {
		t.Fatalf("second click answered %d", rec.Code)
	}
	if rowsFor() != 1 {
		t.Fatal("the second click added a second row")
	}
	if rec = h.post("/unsubscribe/"+tok[:len(tok)-3]+"xyz", form, formHeader); rec.Code != http.StatusNotFound {
		t.Fatalf("a forged token answered %d", rec.Code)
	}
	rec = h.post("/unsubscribe/"+tok, "", formHeader)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "no more mail") {
		t.Fatalf("the form answered %d: %s", rec.Code, rec.Body)
	}
}

func TestALoopedPostIsDropped(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte("From: a@x.org\r\nTo: soccer-team@loop.heliosian.com\r\nX-Helios-Loop: pta\r\n\r\nhi\r\n")})
	h.webhook(fmt.Sprintf(receivedEvent, "m3"))
	h.waitFor("the drop", func() bool { return h.messageState("m3", "soccer-team") == stateDropped })
	if len(h.sender.all()) != 0 || h.archive.count() != 0 {
		t.Fatal("a looped post went out")
	}
}

func TestAFetchFailureIsRecordedAndRetriedOnReplay(t *testing.T) {
	inbox := &fakeInbox{raw: []byte(post), err: fmt.Errorf("resend is down")}
	h := newHarness(t, inbox)
	h.webhook(fmt.Sprintf(receivedEvent, "m4"))
	h.waitFor("the failure", func() bool { return h.messageState("m4", "soccer-team") == stateFailed })
	inbox.err = nil
	h.webhook(fmt.Sprintf(receivedEvent, "m4"))
	h.waitFor("the retry", func() bool { return h.messageState("m4", "soccer-team") == stateSent })
	if len(h.sender.all()) == 0 {
		t.Fatal("the retry sent nothing")
	}
}

func TestMailForNoGroupIsIgnored(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte(post)})
	body := strings.Replace(fmt.Sprintf(receivedEvent, "m5"), "soccer-team@", "nobody@", 1)
	if rec := h.webhook(body); rec.Code != http.StatusNoContent {
		t.Fatalf("webhook answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != 0 || len(h.rows(messagesTab)) != 0 {
		t.Fatal("mail for no group was taken")
	}
}

func TestBouncesAreRecordedAgainstTheGroup(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte(post)})
	before := len(h.rows(deliveriesTab))
	body := `{"type":"email.bounced","created_at":"2026-09-16T10:00:00Z","data":{"email_id":"s1","from":"Alice via Soccer Team Families <soccer-team@loop.heliosian.com>","to":["Gone@example.org"],"subject":"x","bounce":{"message":"550 no such user","type":"Permanent"}}}`
	if rec := h.webhook(body); rec.Code != http.StatusNoContent {
		t.Fatalf("webhook answered %d", rec.Code)
	}
	h.waitFor("the delivery row", func() bool {
		for _, row := range h.rows(deliveriesTab) {
			if row["Group"] == "soccer-team" && row["Email"] == "gone@example.org" && row["Event"] == "bounced" && row["Message"] == "s1" && row["Detail"] == "550 no such user" {
				return true
			}
		}
		return false
	})
	other := strings.Replace(body, "soccer-team@loop.heliosian.com", "team@heliosian.com", 1)
	h.webhook(other)
	time.Sleep(100 * time.Millisecond)
	if len(h.rows(deliveriesTab)) != before+1 {
		t.Fatalf("another app's bounce was recorded: %+v", h.rows(deliveriesTab))
	}
}

func TestWebhookRefusesTheUnsignedAndTheUnconfigured(t *testing.T) {
	h := newHarness(t, &fakeInbox{raw: []byte(post)})
	body := fmt.Sprintf(receivedEvent, "m6")
	if rec := h.post("/api/loop/mail", body, http.Header{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned call answered %d", rec.Code)
	}
	bare := app{cache: h.cache, mail: Mail{}}
	rec := httptest.NewRecorder()
	bare.webhook(rec, httptest.NewRequest(http.MethodPost, "/api/loop/mail", strings.NewReader(body)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unconfigured route answered %d", rec.Code)
	}
}
