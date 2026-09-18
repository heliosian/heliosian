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
)

const classMail = "Received: by mxa.mailgun.org with SMTP id 1; Wed, 16 Sep 2026 07:05:00 +0000\r\n" +
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
		to         []string
		channel    string
		kind       string
	}{
		{"", "Helios School <m@mail1.veracross.com>", []string{"parent@heliosns.org"}, "newsletter", KindNewsletter},
		{"<3064358178.560896@benchmarkemail.com>", "Helios School <news@heliosschool.org>", nil, "newsletter", KindNewsletter},
		{"Falcons Parents <falcons.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", nil, "falcons.parents", KindList},
		{"<chat.heliosns.org>", "Parent <p@gmail.com>", nil, "chat", KindList},
		{"", "Office <office@heliosschool.org>", []string{"Parent <parent@heliosns.org>", "hawksandfalcons@heliosschool.org"}, "hawksandfalcons", KindAnnouncement},
		{"<michelle-level3math.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", nil, "", ""},
		{"<boardoftrustees.heliosschool.org>", "Chair <c@heliosschool.org>", []string{"parentsandstaff@heliosschool.org"}, "", ""},
		{"<github.com>", "GitHub <noreply@github.com>", nil, "", ""},
		{"", "Sam Lee <sam@example.org>", []string{"parent@heliosns.org"}, "", ""},
	} {
		channel, kind, ok := Channel(c.list, c.from, c.to)
		if ok != (c.kind != "") || (ok && (channel != c.channel || kind != c.kind)) {
			t.Errorf("%q from %q to %q: %q %q %v", c.list, c.from, c.to, channel, kind, ok)
		}
	}
}

type storedMail map[string]string

func (s storedMail) Stored(_ context.Context, url string) ([]byte, error) {
	return []byte(s[url]), nil
}

type bucket map[string][]byte

func (b bucket) Put(folder, name, _ string, content []byte) error {
	b[folder+"/"+name] = content
	return nil
}

type inline struct{}

func (inline) Add(fn func()) {
	fn()
}

func testInbox(t *testing.T) (*inbox, bucket, *data.Dir) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, appName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, appName, documentsTab+".csv"), []byte(strings.Join(DocumentColumns, ",")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache, err := NewCache(func(*Model) (*Model, error) { return &Model{Documents: []*Document{}}, nil }, inline{})
	if err != nil {
		t.Fatal(err)
	}
	objects := bucket{}
	sheet := &data.Dir{Root: root}
	in := &inbox{
		Inbox:    Inbox{Store: storedMail{"class": classMail, "personal": personalMail}, SigningKey: "key", Bucket: objects},
		cache:    cache,
		embedder: Fake{},
		writer:   sheet,
		queue:    inline{},
	}
	return in, objects, sheet
}

func TestInboxImportsOnlyTheCommunitysMailOnce(t *testing.T) {
	in, objects, sheet := testInbox(t)
	for _, source := range []string{"personal", "class", "class"} {
		if err := in.take(context.Background(), source); err != nil {
			t.Fatalf("%s: %v", source, err)
		}
	}
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

func TestCacheRefreshKeepsAnEditMadeWhileReading(t *testing.T) {
	var cache *Cache
	loads := 0
	cache, err := NewCache(func(*Model) (*Model, error) {
		loads++
		if loads == 2 {
			cache.add(&Document{Key: "added"})
		}
		return &Model{Documents: []*Document{}}, nil
	}, inline{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.refresh(); err != nil {
		t.Fatal(err)
	}
	if model := cache.Model(); len(model.Documents) != 1 || model.Documents[0].Key != "added" {
		t.Fatalf("the refresh put back the older sheet: %+v", model.Documents)
	}
}

func TestInboxRefusesAnUnsignedCall(t *testing.T) {
	in, _, _ := testInbox(t)
	timestamp, signature := mail.SignMailgun("another key", "token", time.Now())
	form := url.Values{"timestamp": {timestamp}, "token": {"token"}, "signature": {signature}, "message-url": {"class"}}
	req := httptest.NewRequest(http.MethodPost, "/hooks/mail", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	in.hook(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Fatalf("status %d", rec.Code)
	}
}
