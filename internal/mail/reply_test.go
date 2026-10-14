package mail

import (
	"encoding/base64"
	"strings"
	"testing"
)

func replyMail(from, attendee, uid, standing string) string {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR", "PRODID:-//Google Inc//Google Calendar 70.9054//EN", "VERSION:2.0", "METHOD:REPLY",
		"BEGIN:VEVENT", "DTSTART:20260924T230000Z", "DTEND:20260925T010000Z", "DTSTAMP:20260914T170000Z",
		"ORGANIZER;CN=Helios When:mailto:rsvp@reply.heliosian.com", "UID:" + uid,
		"ATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=" + standing + ";CN=" + attendee + ";X-NUM-GUESTS=0:mailto:" + attendee,
		"SUMMARY:Accepted: International Night", "END:VEVENT", "END:VCALENDAR",
	}, "\r\n")
	b64 := base64.StdEncoding.EncodeToString([]byte(ics))
	return strings.Join([]string{
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
	got, err := ParseReply([]byte(replyMail("juni.ashdown@heliosschool.org", "juni.ashdown@heliosschool.org", "a7@sample", "ACCEPTED")))
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != "a7@sample" || got.Email != "juni.ashdown@heliosschool.org" || got.Standing != "ACCEPTED" {
		t.Errorf("reply = %+v", got)
	}
	folded := "BEGIN:VCALENDAR\r\nMETHOD:REPLY\r\nBEGIN:VEVENT\r\nUID:evt0000000001@when.heliosian.com\r\nATTENDEE;CUTYPE=INDIVIDUAL;ROLE=REQ-PARTICIPANT;PARTSTAT=DECLINED;CN=juni.ashdown@h\r\n eliosschool.org:mailto:juni.ashdown@h\r\n eliosschool.org\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if got, err := readReply([]byte(folded)); err != nil || got.Standing != "DECLINED" || got.Email != "juni.ashdown@heliosschool.org" || got.UID != "evt0000000001@when.heliosian.com" {
		t.Errorf("declined reply = %+v, %v", got, err)
	}
	if _, err := ParseReply([]byte("From: a@example.org\r\nSubject: hi\r\n\r\nJust a note.\r\n")); err == nil {
		t.Errorf("a plain note read as a reply")
	}
}
