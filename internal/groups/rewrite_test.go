package groups

import (
	"strings"
	"testing"
)

const post = "Received: from mx.example.org by inbound.resend.com\r\n" +
	"DKIM-Signature: v=1; a=rsa-sha256; d=gmail.com; s=20230601;\r\n\tb=abc\r\n" +
	"Return-Path: <alice@gmail.com>\r\n" +
	"From: =?utf-8?q?Alice_Smith?= <alice@gmail.com>\r\n" +
	"To: soccer-team@loop.heliosian.com\r\n" +
	"Subject: Re: Saturday's game\r\n" +
	"Message-ID: <abc@gmail.com>\r\n" +
	"In-Reply-To: <xyz@gmail.com>\r\n" +
	"References: <xyz@gmail.com>\r\n" +
	"List-Unsubscribe: <mailto:other@example.org>\r\n" +
	"Content-Type: text/plain\r\n" +
	"\r\n" +
	"See you at 9.\r\n"

var team = Group{Name: "soccer-team", Title: "Soccer Team Families", Prefix: true}

func TestRewriteKeepsTheThreadAndTheBody(t *testing.T) {
	lines, body := splitMessage([]byte(post))
	if string(body) != "See you at 9.\r\n" {
		t.Fatalf("body %q", body)
	}
	head, err := rewrite(lines, team)
	if err != nil {
		t.Fatal(err)
	}
	out := string(render(head, []string{"List-Unsubscribe: <https://loop.heliosian.com/unsubscribe/x>"}, body))
	for _, want := range []string{
		"From: \"Alice Smith via Soccer Team Families\" <soccer-team@loop.heliosian.com>\r\n",
		"Subject: Re: [Soccer Team Families] Saturday's game\r\n",
		"Reply-To: =?utf-8?q?Alice_Smith?= <alice@gmail.com>\r\n",
		"X-Original-From: =?utf-8?q?Alice_Smith?= <alice@gmail.com>\r\n",
		"Message-ID: <abc@gmail.com>\r\n",
		"In-Reply-To: <xyz@gmail.com>\r\n",
		"References: <xyz@gmail.com>\r\n",
		"To: soccer-team@loop.heliosian.com\r\n",
		"List-Id: Soccer Team Families <soccer-team.loop.heliosian.com>\r\n",
		"List-Post: <mailto:soccer-team@loop.heliosian.com>\r\n",
		"Precedence: list\r\n",
		"X-Helios-Loop: soccer-team\r\n",
		"List-Unsubscribe: <https://loop.heliosian.com/unsubscribe/x>\r\n",
		"Received: from mx.example.org by inbound.resend.com\r\n",
		"\r\n\r\nSee you at 9.\r\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, gone := range []string{"DKIM-Signature", "Return-Path", "mailto:other@example.org"} {
		if strings.Contains(out, gone) {
			t.Errorf("still carries %q in:\n%s", gone, out)
		}
	}
	if strings.Count(out, "Subject:") != 1 || strings.Count(out, "From:") != 2 {
		t.Errorf("headers doubled:\n%s", out)
	}
}

func TestRewriteLeavesTheSubjectWhenPrefixIsOff(t *testing.T) {
	lines, _ := splitMessage([]byte(post))
	plain := team
	plain.Prefix = false
	head, err := rewrite(lines, plain)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(head), "Subject: Re: Saturday's game\r\n") {
		t.Errorf("subject changed:\n%s", head)
	}
}

func TestPrefixedNeverDoubles(t *testing.T) {
	cases := map[string]string{
		"Saturday's game":                         "[Soccer Team Families] Saturday's game",
		"Re: Saturday's game":                     "Re: [Soccer Team Families] Saturday's game",
		"Re: [Soccer Team Families] Saturday's":   "Re: [Soccer Team Families] Saturday's",
		"RE: re: [soccer team families] Saturday": "Re: [Soccer Team Families] Saturday",
		"[Soccer Team Families] Re: Saturday":     "Re: [Soccer Team Families] Saturday",
		"Fwd: Saturday":                           "[Soccer Team Families] Fwd: Saturday",
		"":                                        "[Soccer Team Families]",
	}
	for in, want := range cases {
		if got := prefixed(in, "Soccer Team Families"); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestRewriteEncodesWhatIsNotASCII(t *testing.T) {
	raw := "From: =?utf-8?q?Jos=C3=A9?= <jose@example.org>\r\nSubject: =?utf-8?q?Caf=C3=A9?=\r\n\r\nhi\r\n"
	lines, _ := splitMessage([]byte(raw))
	head, err := rewrite(lines, team)
	if err != nil {
		t.Fatal(err)
	}
	out := string(head)
	if !strings.Contains(out, "Subject: =?utf-8?q?[Soccer_Team_Families]_Caf=C3=A9?=\r\n") {
		t.Errorf("subject:\n%s", out)
	}
	if !strings.Contains(out, "From: =?utf-8?q?Jos=C3=A9_via_Soccer_Team_Families?= <soccer-team@loop.heliosian.com>\r\n") {
		t.Errorf("from:\n%s", out)
	}
}

func TestHeldStopsLoopsAndAutomata(t *testing.T) {
	cases := map[string]string{
		"From: a@x.org\r\nX-Helios-Loop: other\r\n\r\n":            "already sent through Helios Loop",
		"From: a@x.org\r\nAuto-Submitted: auto-replied\r\n\r\n":    "auto-submitted mail",
		"From: a@x.org\r\nAuto-Submitted: no\r\n\r\n":              "",
		"From: Mail Delivery <MAILER-DAEMON@mx.example.org>\r\n\r\n": "a mail system's notice",
		"From: a@x.org\r\n\r\n": "",
	}
	for raw, want := range cases {
		lines, _ := splitMessage([]byte(raw))
		if got := held(lines); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}

func TestRewriteNeedsAFrom(t *testing.T) {
	lines, _ := splitMessage([]byte("Subject: hi\r\n\r\nbody\r\n"))
	if _, err := rewrite(lines, team); err == nil {
		t.Fatal("a message with no From was rewritten")
	}
}

func TestGroupsInReadsTheDomainsAddresses(t *testing.T) {
	got := groupsIn([]string{"Soccer <Soccer-Team@loop.heliosian.com>", "alice@gmail.com", "soccer-team@loop.heliosian.com", "pta@loop.heliosian.com"})
	if len(got) != 2 || got[0] != "soccer-team" || got[1] != "pta" {
		t.Fatalf("got %v", got)
	}
}
