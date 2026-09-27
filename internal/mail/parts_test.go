package mail

import (
	"io"
	netmail "net/mail"
	"strings"
	"testing"
)

func parts(t *testing.T, raw string) ([]Part, []string, error) {
	t.Helper()
	msg, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	found, bodies := []Part{}, []string{}
	err = Parts(msg, func(p Part) error {
		body, err := io.ReadAll(p.Body)
		if err != nil {
			return err
		}
		found = append(found, p)
		bodies = append(bodies, string(body))
		return nil
	})
	return found, bodies, err
}

func TestPartsDecodesEachLeaf(t *testing.T) {
	raw := strings.Join([]string{
		"From: a@example.org",
		`Content-Type: multipart/mixed; boundary="outer"`,
		"",
		"--outer",
		`Content-Type: multipart/alternative; boundary="inner"`,
		"",
		"--inner",
		"Content-Type: text/plain; charset=iso-8859-1",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		"Caf=E9 at n=",
		"oon.",
		"--inner",
		"Content-Type: text/html; charset=utf-8",
		"Content-Transfer-Encoding: base64",
		"",
		"PHA+Q2Fmw6k8L3A+",
		"--inner--",
		"--outer",
		`Content-Type: application/octet-stream; name="invite.ics"`,
		`Content-Disposition: attachment; filename="Invite.ICS"`,
		"",
		"BEGIN:VCALENDAR",
		"--outer--",
		"",
	}, "\r\n")
	found, bodies, err := parts(t, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 3 {
		t.Fatalf("parts = %+v", found)
	}
	if found[0].MediaType != "text/plain" || bodies[0] != "Café at noon." {
		t.Errorf("plain part = %+v %q", found[0], bodies[0])
	}
	if found[1].MediaType != "text/html" || bodies[1] != "<p>Café</p>" {
		t.Errorf("html part = %+v %q", found[1], bodies[1])
	}
	if found[2].Disposition != "attachment" || found[2].Name != "Invite.ICS" || bodies[2] != "BEGIN:VCALENDAR" {
		t.Errorf("attachment = %+v %q", found[2], bodies[2])
	}
}

func TestPartsTakesPlainTextWhenUntyped(t *testing.T) {
	found, bodies, err := parts(t, "From: a@example.org\r\n\r\nWords.\r\n")
	if err != nil || len(found) != 1 || found[0].MediaType != "text/plain" || bodies[0] != "Words.\r\n" {
		t.Fatalf("parts = %+v %q, %v", found, bodies, err)
	}
}

func TestPartsRefusesABadContentType(t *testing.T) {
	if _, _, err := parts(t, "From: a@example.org\r\nContent-Type: text/\r\n\r\nWords.\r\n"); err == nil {
		t.Fatal("a bad content type was read")
	}
	if _, _, err := parts(t, "From: a@example.org\r\nContent-Type: text/plain; charset=no-such-charset\r\n\r\nWords.\r\n"); err == nil {
		t.Fatal("an unknown charset was read")
	}
}
