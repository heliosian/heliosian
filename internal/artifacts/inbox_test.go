package artifacts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const classMail = "Received: by mxa.mailgun.org with SMTP id 1; Wed, 16 Sep 2026 07:05:00 +0000\r\n" +
	"Authentication-Results: mxa.mailgun.org; dkim=fail header.d=heliosschool.org; spf=pass smtp.mailfrom=\"parent+caf_=ask@heliosschool.org\"; dmarc=pass header.from=heliosschool.org; arc=pass\r\n" +
	"ARC-Seal: i=2; a=rsa-sha256; cv=pass; d=google.com; s=arc-20260327; b=abc\r\n" +
	"ARC-Authentication-Results: i=2; mx.google.com; dkim=pass header.i=@heliosschool.org;\r\n" +
	" spf=pass (google.com: domain of falcons.parents+bncabc@heliosschool.org designates 209.85.220.69 as permitted sender) smtp.mailfrom=falcons.parents+bncABC@heliosschool.org;\r\n" +
	" dmarc=pass header.from=heliosschool.org\r\n" +
	"ARC-Seal: i=1; a=rsa-sha256; cv=none; d=google.com; s=arc-20260327; b=def\r\n" +
	"ARC-Authentication-Results: i=1; mx.google.com; spf=pass smtp.mailfrom=renee.park@heliosschool.org\r\n" +
	"Received: from mail.heliosschool.org by mx.google.com; Tue, 15 Sep 2026 23:59:00 -0700\r\n" +
	"Message-ID: <class-1@heliosschool.org>\r\n" +
	"Date: Tue, 15 Sep 2026 08:30:00 -0700\r\n" +
	"From: =?UTF-8?Q?Ren=C3=A9e_Park?= <renee.park@heliosschool.org>\r\n" +
	"To: falcons.parents@heliosschool.org\r\n" +
	"Subject: =?UTF-8?Q?Falcons_field_trip_=E2=80=93_Thursday?=\r\n" +
	"List-Id: <falcons.parents.heliosschool.org>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=outer\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"Content-Transfer-Encoding: base64\r\n" +
	"\r\n" +
	"UGFjayBhIGx1bmNoLg==\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=\"UTF-8\"\r\n" +
	"Content-Transfer-Encoding: quoted-printable\r\n" +
	"\r\n" +
	"<p>We walk to the library on Thursday. Pack a lunch =E2=80=93 and a hat.</p>=\r\n" +
	"\r\n" +
	"--inner--\r\n" +
	"--outer\r\n" +
	"Content-Type: text/plain; name=\"notes.txt\"\r\n" +
	"Content-Disposition: attachment; filename=\"notes.txt\"\r\n" +
	"\r\n" +
	"not the body\r\n" +
	"--outer--\r\n"

const personalMail = "Received: by mxa.mailgun.org with SMTP id 2; Tue, 15 Sep 2026 16:00:00 +0000\r\n" +
	"Message-ID: <personal-1@example.org>\r\n" +
	"Date: Tue, 15 Sep 2026 09:00:00 -0700\r\n" +
	"From: Sam Lee <sam@example.org>\r\n" +
	"To: Parent <parent@heliosns.org>\r\n" +
	"Subject: Playdate?\r\n" +
	"Content-Type: text/plain; charset=us-ascii\r\n" +
	"\r\n" +
	"Saturday at the park?\r\n"

func TestParseMailReadsHeadersAndBody(t *testing.T) {
	m, err := ParseMail([]byte(classMail))
	if err != nil {
		t.Fatal(err)
	}
	if m.MessageID != "class-1@heliosschool.org" || m.Date != "2026-09-16T07:05:00Z" || m.Subject != "Falcons field trip – Thursday" {
		t.Fatalf("headers: %+v", m)
	}
	if m.From != "Renée Park <renee.park@heliosschool.org>" || len(m.To) != 1 || m.To[0] != "falcons.parents@heliosschool.org" || m.ListID != "<falcons.parents.heliosschool.org>" {
		t.Fatalf("addresses: %+v", m)
	}
	if m.Text != "Pack a lunch." || m.HTML != "<p>We walk to the library on Thursday. Pack a lunch – and a hat.</p>" {
		t.Fatalf("body: text %q, html %q", m.Text, m.HTML)
	}
	doc, err := Build(m, NewResolver(), Fake{}.Model())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Kind != KindList || doc.Channel != "falcons.parents" || doc.Key != Key("class-1@heliosschool.org") || doc.Date != "2026-09-16" {
		t.Fatalf("document: %+v", doc)
	}
}

func TestChannelKeepsOnlyBroadcasts(t *testing.T) {
	for _, c := range []struct {
		list, from string
		channel    string
		kind       string
	}{
		{"", "Helios School <m@mail1.veracross.com>", "newsletter", KindNewsletter},
		{"<3064358178.560896@benchmarkemail.com>", "Helios School <news@heliosschool.org>", "", ""},
		{"Falcons Parents <falcons.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", "falcons.parents", KindList},
		{"<chat.heliosns.org>", "Parent <p@gmail.com>", "chat", KindList},
		{"", "Office <hawksandfalcons@heliosschool.org>", "", ""},
		{"", "\"veracross.com\" <x@elsewhere.example>", "", ""},
		{"", "x@veracross.com.elsewhere.example", "", ""},
		{"<michelle-level3math.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", "", ""},
		{"<boardoftrustees.heliosschool.org>", "Chair <c@heliosschool.org>", "", ""},
		{"<github.com>", "GitHub <noreply@github.com>", "", ""},
		{"", "Sam Lee <sam@example.org>", "", ""},
	} {
		channel, kind, ok := Channel(c.list, c.from)
		if ok != (c.kind != "") || (ok && (channel != c.channel || kind != c.kind)) {
			t.Errorf("%q from %q: %q %q %v", c.list, c.from, channel, kind, ok)
		}
	}
}

func TestVouchTakesTheForwardingMailboxsSealedResults(t *testing.T) {
	const mailgun = "Authentication-Results: mxa.mailgun.org; dkim=fail; spf=pass smtp.mailfrom=\"parent+caf_=ask@heliosschool.org\"; arc="
	sealed := func(results string) string {
		return "ARC-Seal: i=1; a=rsa-sha256; cv=none; d=google.com; s=arc-20260327; b=abc\r\n" +
			"ARC-Authentication-Results: i=1; mx.google.com; " + results + "\r\n"
	}
	const newsletter = "From: Helios School <m@mail1.veracross.com>\r\n\r\n"
	const veracross = "dkim=pass header.i=@mail1.veracross.com; spf=pass smtp.mailfrom=m@mail1.veracross.com; dmarc=pass header.from=mail1.veracross.com"
	const list = "List-Id: <parentsandstaff.heliosschool.org>\r\nFrom: Sunny <sunny@heliosschool.org>\r\n\r\n"
	const groups = "dkim=pass header.i=@heliosschool.org; spf=pass (google.com: domain of parentsandstaff+bnc@heliosschool.org) smtp.mailfrom=parentsandstaff+bncX@heliosschool.org; dmarc=pass header.from=heliosschool.org"
	cases := map[string]bool{
		mailgun + "pass\r\n" + sealed(veracross) + newsletter: true,
		mailgun + "fail\r\n" + sealed(veracross) + newsletter: false,
		mailgun + "pass\r\n" + sealed("dkim=pass header.i=@elsewhere.example; spf=pass smtp.mailfrom=x@elsewhere.example; dmarc=fail header.from=mail1.veracross.com") + newsletter: false,
		mailgun + "pass\r\n" + sealed(groups) + list: true,
		mailgun + "fail\r\n" + sealed(groups) + list: false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=sunny@heliosschool.org; dmarc=pass header.from=heliosschool.org") + list:                            false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=chat+bncX@heliosschool.org; dmarc=pass header.from=heliosschool.org") + list:                        false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=parentsandstaff+bncX@forger.example; dmarc=pass header.from=forger.example") + list:                 false,
		mailgun + "pass\r\n" + "ARC-Seal: i=1; cv=none; d=forger.example; b=abc\r\nARC-Authentication-Results: i=1; mx.forger.example; " + groups + "\r\n" + list: false,
	}
	for raw, want := range cases {
		lines, _ := mail.SplitMessage([]byte(raw))
		m, err := ParseMail([]byte("Received: by mxa.mailgun.org; Wed, 16 Sep 2026 07:05:00 +0000\r\nMessage-ID: <v@x>\r\n" + raw + "Words.\r\n"))
		if err != nil {
			t.Fatal(err)
		}
		if got := vouch(lines, m); (got == "") != want {
			t.Errorf("%q: %q", raw, got)
		}
	}
}

type bucket map[string][]byte

func (b bucket) Put(folder, name, _ string, content []byte) error {
	b[folder+"/"+name] = content
	return nil
}

func (b bucket) Get(name string) ([]byte, error) {
	content, ok := b[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return content, nil
}

func testInbox(t *testing.T) (*Filer, bucket, *data.Dir, *store.Queue) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, appName), 0o755); err != nil {
		t.Fatal(err)
	}
	for tab, columns := range map[string][]string{documentsTab: DocumentColumns, store.ChangeLogTab: store.ChangeLogColumns} {
		if err := os.WriteFile(filepath.Join(root, appName, tab+".csv"), []byte(strings.Join(columns, ",")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	objects := bucket{}
	sheet := &data.Dir{Root: root}
	queue := store.NewQueue()
	cache, err := NewCache(sheet, sheet, objects, Fake{}, queue)
	if err != nil {
		t.Fatal(err)
	}
	return &Filer{Inbox: Inbox{SigningKey: "key", Bucket: objects}, cache: cache, embedder: Fake{}, holder: queue}, objects, sheet, queue
}

func sampleModel(t *testing.T) *Model {
	t.Helper()
	in, _, _, _ := testInbox(t)
	files, err := filepath.Glob(filepath.Join(samples, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := in.FileSaved(context.Background(), "test", file); err != nil {
			t.Fatal(err)
		}
	}
	return in.cache.Model()
}

func TestAFreshStoreReadsWhatWasFiled(t *testing.T) {
	in, objects, sheet, queue := testInbox(t)
	if err := in.FileSaved(context.Background(), "test", samples+"/2026-09-11-newsletter-sep-11.json"); err != nil {
		t.Fatal(err)
	}
	queue.Flush()
	again, err := NewCache(sheet, sheet, objects, Fake{}, queue)
	if err != nil {
		t.Fatal(err)
	}
	if model := again.Model(); len(model.Documents) != 1 || model.Fetched != 1 || model.Documents[0].Title != "Helios Weekly Newsletter 2026 Sep 11" {
		t.Fatalf("a fresh load read %+v", model)
	}
	_, log, err := sheet.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(log) != 1 || log[0]["Action"] != "insert" || log[0]["Actor"] != "test" {
		t.Fatalf("change log %v", log)
	}
}

func hook(in *Filer, key, raw string) int {
	timestamp, signature := mail.SignMailgun(key, "token", time.Now())
	form := url.Values{"timestamp": {timestamp}, "token": {"token"}, "signature": {signature}, "body-mime": {raw}}
	req := httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	in.hook(rec, req)
	return rec.Code
}

func TestInboxImportsOnlyTheCommunitysMailOnce(t *testing.T) {
	in, objects, sheet, queue := testInbox(t)
	direct := strings.NewReplacer("smtp.mailfrom=falcons.parents+bncABC@", "smtp.mailfrom=renee.park@", "<class-1@", "<class-2@").Replace(classMail)
	for _, raw := range []string{personalMail, direct, classMail, classMail} {
		if code := hook(in, "key", raw); code != http.StatusOK {
			t.Fatalf("the hook answered %d", code)
		}
	}
	queue.Flush()
	_, rows, err := sheet.Table(appName, documentsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["Key"] != Key("class-1@heliosschool.org") || rows[0]["Channel"] != "falcons.parents" || rows[0]["Kind"] != KindList {
		t.Fatalf("rows: %+v", rows)
	}
	if _, ok := objects[rows[0]["Object"]]; !ok || len(objects) != 1 {
		t.Fatalf("objects: %d, none named %s", len(objects), rows[0]["Object"])
	}
	model := in.cache.Model()
	if len(model.Documents) != 1 || model.Documents[0].Key != rows[0]["Key"] {
		t.Fatalf("the model holds %d documents, not the one imported", len(model.Documents))
	}
	query, err := Fake{}.Embed(context.Background(), []string{"field trip to the library"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if hits := model.Search(query[0], "field trip to the library", 1); len(hits) != 1 || hits[0].Document.Key != rows[0]["Key"] {
		t.Fatalf("search finds %+v", hits)
	}
}

func TestGroupMailIsFiledUnderEachGroupOnce(t *testing.T) {
	in, _, sheet, queue := testInbox(t)
	for _, group := range []string{"soccer-team", "soccer-team", "chess-club"} {
		if err := in.Post(context.Background(), "loop mailer", group, []byte(personalMail)); err != nil {
			t.Fatalf("%s: %v", group, err)
		}
	}
	queue.Flush()
	_, rows, err := sheet.Table(appName, documentsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: %+v", rows)
	}
	for i, group := range []string{"soccer-team", "chess-club"} {
		if rows[i]["Key"] != Key(group+"/personal-1@example.org") || rows[i]["Channel"] != group || rows[i]["Kind"] != KindGroup {
			t.Errorf("row %d: %+v", i, rows[i])
		}
	}
	if len(in.cache.Model().Documents) != 2 {
		t.Fatalf("the model holds %d documents, not the two filed", len(in.cache.Model().Documents))
	}
}

func TestRemovingAGroupsMailTakesItsRowsAndDocumentsAndLeavesTheObjects(t *testing.T) {
	in, objects, sheet, queue := testInbox(t)
	for _, group := range []string{"soccer-team", "chess-club"} {
		if err := in.Post(context.Background(), "loop mailer", group, []byte(personalMail)); err != nil {
			t.Fatalf("%s: %v", group, err)
		}
	}
	if err := in.Remove(context.Background(), "owner@example.org", "soccer-team"); err != nil {
		t.Fatal(err)
	}
	queue.Flush()
	_, rows, err := sheet.Table(appName, documentsTab)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["Channel"] != "chess-club" {
		t.Fatalf("rows: %+v", rows)
	}
	if _, ok := objects[rows[0]["Object"]]; !ok || len(objects) != 2 {
		t.Fatalf("objects: %d, none named %s", len(objects), rows[0]["Object"])
	}
	if model := in.cache.Model(); len(model.Documents) != 1 || model.Documents[0].Channel != "chess-club" {
		t.Fatalf("the model holds %+v", model.Documents)
	}
	if err := in.Remove(context.Background(), "owner@example.org", "soccer-team"); err != nil {
		t.Fatalf("removing a group with no mail: %v", err)
	}
	queue.Flush()
	_, log, err := sheet.Table(appName, store.ChangeLogTab)
	if err != nil {
		t.Fatal(err)
	}
	removed := false
	for _, row := range log {
		if row["Action"] == "delete" && row["Actor"] == "owner@example.org" && row["Column"] == "Channel" && row["Previous"] == "soccer-team" {
			removed = true
		}
	}
	if !removed {
		t.Errorf("the removal is not in the change log: %v", log)
	}
	queue.Refresh()
	queue.Flush()
	in.cache.documents.mu.Lock()
	defer in.cache.documents.mu.Unlock()
	if len(in.cache.documents.held) != 1 || in.cache.documents.held[rows[0]["Object"]] == nil {
		t.Errorf("the document cache after a refresh holds %d, not just the one the sheet names", len(in.cache.documents.held))
	}
}

func TestInboxRefusesAnUnsignedCall(t *testing.T) {
	in, objects, _, _ := testInbox(t)
	if code := hook(in, "another key", classMail); code != http.StatusNotAcceptable {
		t.Fatalf("status %d", code)
	}
	if len(objects) != 0 {
		t.Fatal("an unsigned call was filed")
	}
}
