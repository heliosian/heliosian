package calendar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/mail"
)

const (
	replySecret = "mailgun-signing-key"
	replyTo     = "Helios When <rsvp@reply.heliosian.com>"
)

var replyKey = []byte("session-key")

func replyAddress(id, email string) string {
	return app{mail: Mail{ReplyTo: replyTo, Key: replyKey}}.organizer(id, email)
}

// replyMail is a reply the way Google Calendar sends one: alternatives of
// text, HTML and a base64 calendar, and the same calendar again as a .ics
// attachment, under the results Mailgun found for the sender's domain.
func replyMail(from, attendee, uid, standing, results string) string {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "VERSION:2.0", "METHOD:REPLY",
		"BEGIN:VEVENT", "DTSTART:20260924T230000Z", "DTEND:20260925T010000Z", "DTSTAMP:20260914T170000Z",
		"ORGANIZER;CN=Helios When:mailto:rsvp@reply.heliosian.com", "UID:" + uid,
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=" + standing + ";CN=" + attendee + ";X-NUM-GUESTS=0:mailto:" + attendee,
		"SUMMARY:Accepted: International Night", "END:VEVENT", "END:VCALENDAR",
	}, "\r\n")
	b64 := base64.StdEncoding.EncodeToString([]byte(ics))
	return strings.Join([]string{
		"Authentication-Results: mxa.mailgun.org; " + results,
		"From: " + from,
		"To: rsvp@reply.heliosian.com",
		"Subject: Accepted: International Night",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="outer"`,
		"",
		"--outer",
		`Content-Type: multipart/alternative; boundary="inner"`,
		"",
		"--inner",
		`Content-Type: text/plain; charset="UTF-8"`,
		"",
		"Sam has accepted this invitation.",
		"--inner",
		`Content-Type: text/calendar; charset="UTF-8"; method=REPLY`,
		"Content-Transfer-Encoding: base64",
		"",
		b64,
		"--inner--",
		"--outer",
		`Content-Type: application/ics; name="invite.ics"`,
		`Content-Disposition: attachment; filename="invite.ics"`,
		"Content-Transfer-Encoding: base64",
		"",
		b64,
		"--outer--",
		"",
	}, "\r\n")
}

func TestParseReply(t *testing.T) {
	got, err := ParseReply([]byte(replyMail("jordan.whitfield@heliosschool.org", "jordan.whitfield@heliosschool.org", "a7@sample", "ACCEPTED", "dmarc=pass header.from=heliosschool.org")))
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != "a7@sample" || got.Email != "jordan.whitfield@heliosschool.org" || got.Standing != "ACCEPTED" {
		t.Errorf("reply = %+v", got)
	}
	// A sheet event's UID gives its id back; a folded attendee line reads
	// the same as a whole one.
	folded := "BEGIN:VCALENDAR\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:7QK2M4XN@calendar.heliosian.com\r\nATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=DECLINED;CN=jordan.whitfield@h\r\n eliosschool.org:mailto:jordan.whitfield@h\r\n eliosschool.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if got, err := readReply([]byte(folded)); err != nil || got.Standing != "DECLINED" || got.Email != "jordan.whitfield@heliosschool.org" || idOfUID(got.UID) != "7QK2M4XN" {
		t.Errorf("declined reply = %+v, %v", got, err)
	}
	if _, err := ParseReply([]byte("From: a@example.org\r\nSubject: hi\r\n\r\nJust a note.\r\n")); err == nil {
		t.Errorf("a plain note read as a reply")
	}
}

type fakeStore map[string][]byte

func (f fakeStore) Stored(ctx context.Context, url string) ([]byte, error) {
	raw, ok := f[url]
	if !ok {
		return nil, fmt.Errorf("no message at %s", url)
	}
	return raw, nil
}

func stored(id string) string {
	return "https://sw.api.mailgun.net/v3/domains/reply.heliosian.com/messages/" + id
}

// A signed notification of a reply records the attendee's answer with no
// invite sent; an unsigned one is refused; a reply for someone the
// directory does not know, from another sender, from a sender Mailgun did
// not authenticate, or to any address but the attendee's own for the event
// changes nothing.
func TestRepliesRecordAnswers(t *testing.T) {
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	cache, err := NewCache(dir, func() Roster { return roster }, nil, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	me := "jordan.whitfield@heliosschool.org"
	d := fakeDirectory{people: map[string]Person{me: {Email: me, Name: "Jordan", IsParent: true}}, kids: map[string][]Person{}}
	const school, elsewhere = "dmarc=pass header.from=heliosschool.org", "dmarc=pass header.from=example.org"
	store := fakeStore{
		stored("yes"):      []byte(replyMail("Jordan <"+me+">", me, "a7@sample", "ACCEPTED", school)),
		stored("no"):       []byte(replyMail(me, me, "a7@sample", "DECLINED", school)),
		stored("stranger"): []byte(replyMail("x@example.org", "x@example.org", "a7@sample", "ACCEPTED", elsewhere)),
		stored("forged"):   []byte(replyMail("x@example.org", me, "a7@sample", "DECLINED", elsewhere)),
		stored("spoofed"):  []byte(replyMail(me, me, "a7@sample", "DECLINED", "dkim=pass header.d=example.org; spf=pass smtp.mailfrom=x@example.org; dmarc=fail header.from=heliosschool.org")),
	}
	m := Mail{Store: store, SigningKey: replySecret, ReplyTo: replyTo, Key: replyKey}
	mux := http.NewServeMux()
	Register(mux, cache, dir, directQueue{}, nil, d, func() []string { return nil }, func(string) []Linked { return nil }, nil, nil, ImageSearch{}, m)
	own := replyAddress("a7@sample", me)
	if own != "Helios When <rsvp+"+(app{mail: m}).replyToken("a7@sample", me)+"@reply.heliosian.com>" {
		t.Fatalf("organizer = %q", own)
	}
	to := strings.ToUpper(own)
	post := func(id string, signed bool) int {
		fields := map[string]string{"recipient": to, "from": me, "subject": "Accepted: International Night", "message-url": stored(id)}
		if signed {
			stamp, sig := mail.SignMailgun(replySecret, "token-"+id, now())
			fields["timestamp"], fields["token"], fields["signature"] = stamp, "token-"+id, sig
		}
		body, _ := json.Marshal(fields)
		req := httptest.NewRequest("POST", "https://when.heliosiandev.com:8080/hooks/replies", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("yes", false); code != 406 {
		t.Errorf("unsigned call: %d", code)
	}
	if code := post("yes", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerYes {
		t.Errorf("accepted: %d, answer %q", code, cache.Model().AnswerOf(me, "a7@sample"))
	}
	if code := post("no", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerNo {
		t.Errorf("declined: %d, answer %q", code, cache.Model().AnswerOf(me, "a7@sample"))
	}
	if code := post("stranger", true); code != 200 || cache.Model().AnswerOf("x@example.org", "a7@sample") != "" {
		t.Errorf("a stranger's reply was taken")
	}
	post("yes", true)
	if code := post("forged", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerYes {
		t.Errorf("a reply from another sender was taken")
	}
	if code := post("spoofed", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerYes {
		t.Errorf("a reply Mailgun did not authenticate was taken")
	}
	for _, other := range []string{"rsvp@reply.heliosian.com", "rsvp+@reply.heliosian.com", replyAddress("a8@sample", me), replyAddress("a7@sample", "x@example.org")} {
		to = other
		if code := post("no", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerYes {
			t.Errorf("a reply to %s was taken", other)
		}
	}
	// A notification for another address under the domain is left alone,
	// unfetched.
	to = "someone-else@reply.heliosian.com"
	if code := post("no", true); code != 200 || cache.Model().AnswerOf(me, "a7@sample") != AnswerYes {
		t.Errorf("mail for another address was taken as a reply")
	}
}
