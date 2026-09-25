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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

type staticFiles struct{}

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("../../web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

type sampleDirectory struct {
	model *who.Model
}

func (d sampleDirectory) Resolve(email string) string           { return d.model.Resolve(email) }
func (d sampleDirectory) Model() *who.Model                     { return d.model }
func (d sampleDirectory) Tags(owner string) map[string][]string { return d.model.Tags(owner) }
func (d sampleDirectory) Lists(owner string) []who.List         { return d.model.RoomParentLists(owner) }
func (d sampleDirectory) Shared(email string) []who.SharedTag   { return d.model.SharedTags(email) }
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
	err     error
}

func (f *fakeArchive) Put(_ context.Context, name, mimeType string, content []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if mimeType != mailType {
		return fmt.Errorf("type %s", mimeType)
	}
	f.objects[name] = content
	return nil
}

func (f *fakeArchive) Get(_ context.Context, name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	content, ok := f.objects[name]
	if !ok {
		return nil, fmt.Errorf("no object %s", name)
	}
	return content, nil
}

func (f *fakeArchive) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeArchive) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.objects)
}

type fakeDocuments struct {
	mu      sync.Mutex
	groups  []string
	dropped []string
}

func (f *fakeDocuments) Post(_ context.Context, _, group string, raw []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.groups = append(f.groups, group)
	return nil
}

func (f *fakeDocuments) Remove(_ context.Context, _, group string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropped = append(f.dropped, group)
	return nil
}

func (f *fakeDocuments) filed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.groups)
}

func (f *fakeDocuments) removed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.dropped)
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
	documents *fakeDocuments
	mailbox   Mail
	queue     store.Enqueuer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	deliveryBatch = 20 * time.Millisecond
	dir := &data.Dir{Root: "../../sampledata"}
	model, err := who.LoadModel(dir, nil, staticFiles{}, []byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	queue := store.NewQueue()
	cache, err := NewCache(dir, dir, func(string) bool { return false }, queue)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, mux: http.NewServeMux(), dir: dir, cache: cache, directory: sampleDirectory{model}, sender: &fakeSender{}, archive: &fakeArchive{objects: map[string][]byte{}}, documents: &fakeDocuments{}, queue: queue}
	h.mailbox = Mail{Sender: h.sender, SigningKey: signingKey, Key: []byte("key"), Base: "https://loop.test", Archive: h.archive, Documents: h.documents}
	Register(h.mux, cache, nil, h.directory, func() []string { return nil }, h.mailbox, nil)
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
	token := "token-" + id(fields["body-mime"]) + fields["recipient"]
	stamp, sig := mail.SignMailgun(signingKey, token, time.Now())
	fields["timestamp"] = stamp
	fields["token"] = token
	fields["signature"] = sig
	return fields
}

func id(raw string) string {
	return idOf([]byte(raw))
}

func notify(raw, recipient string) map[string]string {
	return signed(map[string]string{
		"recipient": recipient,
		"sender":    "alice@gmail.com",
		"from":      "Alice Smith <alice@gmail.com>",
		"subject":   "Re: Saturday's game",
		"body-mime": raw,
	})
}

func (h *harness) inbound(fields map[string]string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(fields)
	return h.post("/hooks/mail/mime", string(body), jsonHeader)
}

func (h *harness) inboundForm(fields map[string]string) *httptest.ResponseRecorder {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}
	return h.post("/hooks/mail/mime", form.Encode(), formHeader)
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
	h := newHarness(t)
	if rec := h.inbound(notify(post, "soccer-team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	if row := h.messageRow(id(post), "soccer-team"); row == nil || row["Object"] == "" || row["Message ID"] != "abc@gmail.com" || row["State"] == "" {
		t.Fatalf("the row was not written before the ack: %+v", row)
	}
	if h.archive.count() != 1 {
		t.Fatalf("the archive holds %d objects after the ack", h.archive.count())
	}
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	h.waitFor("the filing for ask", func() bool { return len(h.documents.filed()) == 1 })
	if filed := h.documents.filed(); filed[0] != "soccer-team" {
		t.Fatalf("filed under %v", filed)
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
		if !strings.HasPrefix(name, "loop/soccer-team/") || !strings.HasSuffix(name, "-"+id(post)+".eml") || string(content) != post {
			t.Fatalf("archived %s: %q", name, content)
		}
	}
	row := h.messageRow(id(post), "soccer-team")
	if row["Recipients"] != fmt.Sprint(len(want)) || row["From"] != "Alice Smith <alice@gmail.com>" || row["Subject"] != "Re: Saturday's game" || row["Object"] == "" || row["Detail"] != "" || row["Received"] == "" {
		t.Fatalf("row %+v", row)
	}
	if rec := h.inbound(notify(post, "soccer-team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("replay answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != len(want) || h.archive.count() != 1 {
		t.Fatal("a replayed notification sent or archived the post again")
	}
}

func TestAFormNotificationIsTakenToo(t *testing.T) {
	h := newHarness(t)
	if rec := h.inboundForm(notify(post, "Soccer-Team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
}

func TestOneClickUnsubscribeTakesThemOffTheGroup(t *testing.T) {
	h := newHarness(t)
	h.inbound(notify(post, "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
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
	h := newHarness(t)
	h.inbound(notify(post, "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	first := h.sender.all()[0]
	tok := unsubscribeLink.FindStringSubmatch(string(first.raw))[1]
	unsubscribe := "From: Someone <" + first.to[0] + ">\r\nTo: unsubscribe@loop.heliosian.com\r\nSubject: Re: " + tok + "\r\n\r\n\r\n"
	fields := signed(map[string]string{
		"recipient": "unsubscribe@loop.heliosian.com",
		"sender":    first.to[0],
		"from":      "Someone <" + first.to[0] + ">",
		"subject":   "Re: " + tok,
		"body-mime": unsubscribe,
	})
	if rec := h.inbound(fields); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	g := h.cache.Model().Group("soccer-team")
	if !g.HasExcluded(first.to[0]) || g.Excluded[0].Note != "Unsubscribed by mail from "+first.to[0] {
		t.Fatalf("the mail did not exclude them: %+v", g.Excluded)
	}
	time.Sleep(100 * time.Millisecond)
	if h.messageRow(id(unsubscribe), "soccer-team") != nil || h.messageRow(id(unsubscribe), "unsubscribe") != nil || h.archive.count() != 1 {
		t.Fatal("the unsubscribe mail was taken as a post")
	}
}

func TestAnAliasReachesItsGroup(t *testing.T) {
	h := newHarness(t)
	if rec := h.inbound(notify(post, "Hummingbirds-Families@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "hummingbird-families") == stateSent })
	if h.messageRow(id(post), "hummingbirds-families") != nil {
		t.Fatal("the alias got a row of its own")
	}
	if len(h.sender.all()) != len(h.members("hummingbird-families")) {
		t.Fatalf("%d sends", len(h.sender.all()))
	}
}

func TestALoopedPostIsDropped(t *testing.T) {
	h := newHarness(t)
	looped := "From: a@x.org\r\nTo: soccer-team@loop.heliosian.com\r\nX-Helios-Loop: pta\r\n\r\nhi\r\n"
	h.inbound(notify(looped, "soccer-team@loop.heliosian.com"))
	h.waitFor("the drop", func() bool { return h.messageState(id(looped), "soccer-team") == stateDropped })
	if len(h.sender.all()) != 0 || len(h.documents.filed()) != 0 {
		t.Fatal("a looped post went out")
	}
	if h.archive.count() != 1 {
		t.Fatalf("the archive holds %d objects; a dropped post stays archived as it came", h.archive.count())
	}
}

func TestAPostFromSomeoneTheGroupDoesNotLetPostIsDropped(t *testing.T) {
	h := newHarness(t)
	h.inbound(notify(post, "middle-school-parents@loop.heliosian.com"))
	h.waitFor("the drop", func() bool { return h.messageState(id(post), "middle-school-parents") == stateDropped })
	if detail := h.messageRow(id(post), "middle-school-parents")["Detail"]; detail != "only the group's managers may post" {
		t.Fatalf("detail %q", detail)
	}
	if len(h.documents.filed()) != 0 {
		t.Fatal("a refused post went out")
	}
	sends := h.sender.all()
	if len(sends) != 1 || sends[0].from != bounceFrom || len(sends[0].to) != 1 || sends[0].to[0] != "alice@gmail.com" {
		t.Fatalf("the bounce: %+v", sends)
	}
	for _, want := range []string{"To: alice@gmail.com\r\n", "Subject: Not delivered: Re: Saturday's game\r\n", "Auto-Submitted: auto-replied\r\n", "X-Helios-Loop: middle-school-parents\r\n", "In-Reply-To: <abc@gmail.com>\r\n", "only its managers can post to it"} {
		if !strings.Contains(string(sends[0].raw), want) {
			t.Errorf("the bounce lacks %q:\n%s", want, sends[0].raw)
		}
	}
	managers := strings.NewReplacer("alice@gmail.com", "Dana.Hawkins@heliosschool.org", "gmail.com", "heliosschool.org").Replace(post)
	h.inbound(notify(managers, "middle-school-parents@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(managers), "middle-school-parents") == stateSent })
	if len(h.sender.all()) != 1+len(h.members("middle-school-parents")) {
		t.Fatalf("%d sends", len(h.sender.all()))
	}
}

func TestAnUnauthenticatedPostIsDroppedWithoutABounce(t *testing.T) {
	forged := strings.Replace(post, "dkim=pass header.d=gmail.com", "dkim=fail header.d=gmail.com", 1)
	forged = strings.Replace(forged, "spf=pass", "spf=softfail", 1)
	forged = strings.Replace(forged, "dmarc=pass", "dmarc=fail", 1)
	h := newHarness(t)
	h.inbound(notify(forged, "middle-school-parents@loop.heliosian.com"))
	h.waitFor("the drop", func() bool { return h.messageState(id(forged), "middle-school-parents") == stateDropped })
	if detail := h.messageRow(id(forged), "middle-school-parents")["Detail"]; detail != "the sender's address passed neither SPF nor DKIM" {
		t.Fatalf("detail %q", detail)
	}
	if len(h.sender.all()) != 0 || len(h.documents.filed()) != 0 {
		t.Fatal("an unauthenticated post went out or was answered")
	}
}

func testMessage(from, id, references string) string {
	_, domain, _ := strings.Cut(from, "@")
	raw := "Authentication-Results: mxa.mailgun.org; dmarc=pass header.from=" + domain + "\r\n" +
		"From: " + from + "\r\nTo: middle-school-parents@loop.heliosian.com\r\nSubject: Picture day\r\nMessage-ID: <" + id + ">\r\n"
	if references != "" {
		raw += "In-Reply-To: " + references + "\r\nReferences: " + references + "\r\n"
	}
	return raw + "\r\nbody\r\n"
}

func TestRepliesToTheGroupsOwnMailAnswerToWhoCanReply(t *testing.T) {
	h := newHarness(t)
	g := h.cache.Model().Group("middle-school-parents")
	member := ""
	for _, email := range h.members(g.Name) {
		if !g.Manages(email) {
			member = email
			break
		}
	}
	if member == "" {
		t.Fatal("no member who is not a manager")
	}
	deliver := func(raw, want, detail string) {
		t.Helper()
		h.inbound(notify(raw, g.Address()))
		h.waitFor(id(raw), func() bool { return h.messageState(id(raw), g.Name) == want })
		if got := h.messageRow(id(raw), g.Name)["Detail"]; want == stateDropped && got != detail {
			t.Fatalf("%s: detail %q", id(raw), got)
		}
	}
	deliver(testMessage("dana.hawkins@heliosschool.org", "p1@heliosschool.org", ""), stateSent, "")
	deliver(testMessage(member, "p2@heliosschool.org", "<p1@heliosschool.org>"), stateSent, "")
	deliver(testMessage(member, "p3@heliosschool.org", "<p2@heliosschool.org>"), stateSent, "")
	deliver(testMessage(member, "p4@heliosschool.org", ""), stateDropped, "only the group's managers may post")
	deliver(testMessage(member, "p5@heliosschool.org", "<CAF1x7qNw3pR@mail.gmail.com>"), stateDropped, "only the group's managers may post")
	deliver(testMessage("alice@gmail.com", "p6@gmail.com", "<other@x.org> <p1@heliosschool.org>"), stateDropped, "only the group's members may reply")
	sends := h.sender.all()
	last := sends[len(sends)-1]
	if last.to[0] != "alice@gmail.com" || !strings.Contains(string(last.raw), "only the people on it and its managers can reply to it") {
		t.Fatalf("the reply's bounce: %s", last.raw)
	}
}

func TestAnArchiveFailureIsAnsweredWithAnErrorAndTheRetryLands(t *testing.T) {
	h := newHarness(t)
	h.archive.fail(fmt.Errorf("the bucket is down"))
	if rec := h.inbound(notify(post, "soccer-team@loop.heliosian.com")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("inbound answered %d with the archive down", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if h.messageRow(id(post), "soccer-team") != nil || len(h.sender.all()) != 0 {
		t.Fatal("a message the archive refused was recorded or sent")
	}
	h.archive.fail(nil)
	if rec := h.inbound(notify(post, "soccer-team@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("the retry answered %d", rec.Code)
	}
	h.waitFor("the retry", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	if len(h.sender.all()) == 0 {
		t.Fatal("the retry sent nothing")
	}
}

func TestARestartResumesAMessageFromItsArchivedCopy(t *testing.T) {
	h := newHarness(t)
	object := "loop/soccer-team/20260921T120000Z-" + id(post) + ".eml"
	if err := h.archive.Put(context.Background(), object, mailType, []byte(post)); err != nil {
		t.Fatal(err)
	}
	cells := store.Row{"Received": time.Now().Format(time.RFC3339), "From": "Alice Smith <alice@gmail.com>", "Subject": "Re: Saturday's game", "State": stateReceived, "Object": object, "Message ID": "abc@gmail.com"}
	if err := h.cache.CommitAndWait(context.Background(), "test", store.Set(messagesTab, store.Row{"ID": id(post), "Group": "soccer-team"}, cells)); err != nil {
		t.Fatal(err)
	}
	newMailer(h.cache, h.directory, h.mailbox).recover()
	h.waitFor("the resumed forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	if len(h.sender.all()) != len(h.members("soccer-team")) || h.archive.count() != 1 {
		t.Fatalf("%d sends, %d objects", len(h.sender.all()), h.archive.count())
	}
}

func TestMailForNoGroupIsIgnored(t *testing.T) {
	h := newHarness(t)
	before := len(h.rows(messagesTab))
	if rec := h.inbound(notify(post, "nobody@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d", rec.Code)
	}
	time.Sleep(100 * time.Millisecond)
	if len(h.sender.all()) != 0 || len(h.rows(messagesTab)) != before || h.archive.count() != 0 {
		t.Fatal("mail for no group was taken")
	}
}

const eventStamp = 4102444800.25

func event(kind, severity, from, messageID, recipient, detail string) string {
	stamp, sig := mail.SignMailgun(signingKey, "token-"+kind+recipient, time.Now())
	body, _ := json.Marshal(map[string]any{
		"signature": map[string]string{"timestamp": stamp, "token": "token-" + kind + recipient, "signature": sig},
		"event-data": map[string]any{
			"event": kind, "timestamp": eventStamp, "severity": severity, "recipient": recipient, "reason": "bounce",
			"message":         map[string]any{"headers": map[string]string{"message-id": messageID, "from": from}},
			"delivery-status": map[string]any{"message": "OK", "description": detail, "code": 550},
		},
	})
	return string(body)
}

func (h *harness) deliveryRows(messageID, event string) []map[string]string {
	out := []map[string]string{}
	for _, row := range h.rows(deliveriesTab) {
		if row["Message"] == messageID && row["Event"] == event {
			out = append(out, row)
		}
	}
	return out
}

func TestDeliveryEventsAreRecordedAgainstTheGroup(t *testing.T) {
	h := newHarness(t)
	h.inbound(notify(post, "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	members := h.members("soccer-team")
	h.waitFor("a sent row per copy", func() bool { return len(h.deliveryRows("abc@gmail.com", eventSent)) == len(members) })
	for _, row := range h.deliveryRows("abc@gmail.com", eventSent) {
		if row["Group"] != "soccer-team" || !slices.Contains(members, row["Email"]) || row["Timestamp"] == "" {
			t.Fatalf("sent row %+v", row)
		}
	}
	from := "Alice via Soccer Team Families <soccer-team@loop.heliosian.com>"
	if rec := h.post("/hooks/events", event("failed", "permanent", from, "abc@gmail.com", "Gone@example.org", "550 no such user"), jsonHeader); rec.Code != http.StatusOK {
		t.Fatalf("events answered %d: %s", rec.Code, rec.Body)
	}
	h.post("/hooks/events", event("failed", "temporary", from, "abc@gmail.com", "slow@example.org", "greylisted"), jsonHeader)
	h.post("/hooks/events", event("complained", "", from, "abc@gmail.com", "cross@example.org", ""), jsonHeader)
	h.post("/hooks/events", event("delivered", "", from, "abc@gmail.com", "fine@example.org", ""), jsonHeader)
	h.post("/hooks/events", event("failed", "permanent", "HCA-Team <team@heliosian.com>", "abc@gmail.com", "other@example.org", "550"), jsonHeader)
	h.post("/hooks/events", event("delivered", "", "HCA-Team <team@loop.heliosian.com>", "portal@loop.heliosian.com", "else@example.org", ""), jsonHeader)
	h.waitFor("the event rows", func() bool {
		n := 0
		for _, row := range h.rows(deliveriesTab) {
			if row["Message"] == "abc@gmail.com" && row["Event"] != eventSent {
				n++
			}
		}
		return n == 4
	})
	rows := map[string]map[string]string{}
	for _, row := range h.rows(deliveriesTab) {
		if row["Message"] == "abc@gmail.com" && row["Event"] != eventSent {
			rows[row["Email"]] = row
		}
	}
	stamp := time.UnixMilli(eventStamp * 1000).Format(time.RFC3339)
	if r := rows["gone@example.org"]; r["Event"] != eventBounced || r["Detail"] != "550 no such user" || r["Group"] != "soccer-team" || r["Timestamp"] != stamp {
		t.Fatalf("bounce row %+v", r)
	}
	if rows["slow@example.org"]["Event"] != eventDelayed || rows["slow@example.org"]["Detail"] != "greylisted" || rows["cross@example.org"]["Event"] != eventComplaint {
		t.Fatalf("rows %v", rows)
	}
	if r := rows["fine@example.org"]; r["Event"] != eventDelivered || r["Detail"] != "" || r["Timestamp"] != stamp {
		t.Fatalf("delivered row %+v", r)
	}
	time.Sleep(100 * time.Millisecond)
	for _, row := range h.rows(deliveriesTab) {
		if row["Email"] == "other@example.org" || row["Email"] == "else@example.org" {
			t.Fatalf("mail Loop did not send was recorded: %+v", row)
		}
	}
}

func TestHistoryListsSentMessagesWithEachCopy(t *testing.T) {
	h := newHarness(t)
	h.inbound(notify(post, "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	if messageID := h.messageRow(id(post), "soccer-team")["Message ID"]; messageID != "abc@gmail.com" {
		t.Fatalf("message id %q", messageID)
	}
	members := h.members("soccer-team")
	h.waitFor("a sent row per copy", func() bool { return len(h.deliveryRows("abc@gmail.com", eventSent)) == len(members) })
	from := "Alice via Soccer Team Families <soccer-team@loop.heliosian.com>"
	h.post("/hooks/events", event("failed", "permanent", from, "abc@gmail.com", members[0], "550 no such user"), jsonHeader)
	h.post("/hooks/events", event("failed", "temporary", from, "abc@gmail.com", members[1], "4.3.0 Temporary System Problem"), jsonHeader)
	h.post("/hooks/events", event("failed", "temporary", from, "abc@gmail.com", members[2], "greylisted"), jsonHeader)
	h.post("/hooks/events", event("delivered", "", from, "abc@gmail.com", members[2], ""), jsonHeader)
	h.waitFor("the event rows", func() bool {
		return len(h.deliveryRows("abc@gmail.com", eventBounced))+len(h.deliveryRows("abc@gmail.com", eventDelayed))+len(h.deliveryRows("abc@gmail.com", eventDelivered)) == 4
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
	if m.Subject != "Re: Saturday's game" || m.From.Email != "alice@gmail.com" || m.Recipients != len(members) || len(m.Copies) != len(members) || m.Delivered != 1 || m.Failed != 1 || m.Pending != len(members)-2 {
		t.Fatalf("newest message %+v", m)
	}
	copies := map[string]Copy{}
	for _, c := range m.Copies {
		copies[c.Email] = c
	}
	if c := copies[members[0]]; c.State != copyFailed || c.When == "" || m.Copies[0].Email != members[0] {
		t.Fatalf("the bounced copy %+v, first %+v", c, m.Copies[0])
	}
	if c := copies[members[1]]; c.State != copyPending || c.When != "" || len(c.Attempts) != 2 || c.Attempts[1].Detail != "4.3.0 Temporary System Problem" {
		t.Fatalf("the delayed copy %+v", c)
	}
	if c := copies[members[2]]; c.State != copyDelivered || c.When == "" || len(c.Attempts) != 3 {
		t.Fatalf("the copy delivered after a delay %+v", c)
	}
	if c := copies[members[3]]; c.State != copyPending || len(c.Attempts) != 1 || c.Attempts[0].Event != eventSent {
		t.Fatalf("a copy not yet heard of %+v", c)
	}
	if sample := body.Messages[1]; sample.From.Name != "Jordan Whitfield" || sample.Delivered != 3 || sample.Failed != 1 || sample.Pending != 1 || len(sample.Copies) != 5 {
		t.Fatalf("sample message %+v", sample)
	}
	if (app{cache: h.cache}).sentCount("soccer-team") != 2 {
		t.Fatal("the sent count is not the history's length")
	}
	if rec := h.as("ruth.amari@heliosschool.org", http.MethodGet, "/api/loop/messages?name=soccer-team", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("a non-manager got %d", rec.Code)
	}
}

func (h *harness) groupRows(tab, name string) int {
	n := 0
	for _, row := range h.rows(tab) {
		if row["Group"] == name {
			n++
		}
	}
	return n
}

func TestDeletingAGroupTakesItsMailRecordWithIt(t *testing.T) {
	h := newHarness(t)
	h.inbound(notify(post, "soccer-team@loop.heliosian.com"))
	h.waitFor("the forward", func() bool { return h.messageState(id(post), "soccer-team") == stateSent })
	h.waitFor("a sent row per copy", func() bool { return len(h.deliveryRows("abc@gmail.com", eventSent)) == len(h.members("soccer-team")) })
	if h.archive.count() != 1 || h.groupRows(messagesTab, "soccer-team") != 2 || h.groupRows(deliveriesTab, "soccer-team") < 2 {
		t.Fatalf("before: %d archived, %d messages, %d deliveries", h.archive.count(), h.groupRows(messagesTab, "soccer-team"), h.groupRows(deliveriesTab, "soccer-team"))
	}
	if rec := h.as("jordan.whitfield@heliosschool.org", http.MethodDelete, "/api/loop/group", `{"name":"soccer-team"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("delete answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the record to go", func() bool {
		return h.groupRows(messagesTab, "soccer-team") == 0 && h.groupRows(deliveriesTab, "soccer-team") == 0 && len(h.documents.removed()) == 1
	})
	if removed := h.documents.removed(); removed[0] != "soccer-team" {
		t.Fatalf("documents removed for %v", removed)
	}
	if h.archive.count() != 1 {
		t.Fatalf("the archive holds %d objects; it is written, never emptied", h.archive.count())
	}
	if model := h.cache.Model(); len(model.Messages) != 0 || len(model.Deliveries) != 0 {
		t.Fatalf("memory keeps %d messages and %d deliveries", len(model.Messages), len(model.Deliveries))
	}
	deleted := map[string]int{}
	h.waitFor("the delete's log", func() bool {
		deleted = map[string]int{}
		for _, row := range h.rows(store.ChangeLogTab) {
			if row["Action"] == "delete" && strings.Contains(row["Key"], "Group=soccer-team") {
				deleted[row["Tab"]]++
			}
		}
		return deleted[messagesTab] > 0
	})
	if deleted[messagesTab] == 0 || deleted[managersTab] == 0 || deleted[deliveriesTab] != 0 {
		t.Fatalf("the delete logged %v; the deliveries are append-only and unlogged", deleted)
	}
	rec := h.as("ruth.amari@heliosschool.org", http.MethodPost, "/api/loop/group", `{"original":"","name":"soccer-team","title":"Soccer again","managers":["ruth.amari@heliosschool.org"],"rules":[{"kind":"include","roles":["Staff"]}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("taking the name answered %d: %s", rec.Code, rec.Body)
	}
	rec = h.as("ruth.amari@heliosschool.org", http.MethodGet, "/api/loop/messages?name=soccer-team", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("messages answered %d: %s", rec.Code, rec.Body)
	}
	var body struct {
		Messages []SentMessage `json:"messages"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 0 {
		t.Fatalf("the new group reads the old one's mail: %+v", body.Messages)
	}
}

func TestRoutesRefuseTheUnsignedAndTheUnconfigured(t *testing.T) {
	h := newHarness(t)
	fields := notify(post, "soccer-team@loop.heliosian.com")
	fields["signature"] = "0000"
	if rec := h.inbound(fields); rec.Code != http.StatusNotAcceptable {
		t.Fatalf("a badly signed notification answered %d", rec.Code)
	}
	body := event("failed", "permanent", "x <soccer-team@loop.heliosian.com>", "abc@gmail.com", "a@example.org", "x")
	if rec := h.post("/hooks/events", strings.Replace(body, `"signature":"`, `"signature":"ff`, 1), jsonHeader); rec.Code != http.StatusNotAcceptable {
		t.Fatalf("a badly signed event answered %d", rec.Code)
	}
	bare := app{cache: h.cache, mail: Mail{}}
	rec := httptest.NewRecorder()
	bare.inbound(rec, httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the unconfigured inbound route answered %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	bare.events(rec, httptest.NewRequest(http.MethodPost, "/hooks/events", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("the unconfigured events route answered %d", rec.Code)
	}
}
