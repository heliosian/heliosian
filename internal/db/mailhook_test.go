package db

import (
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
	"net/url"
	"strings"
	"testing"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/testkit/mailtest"
)

const classMail = "Received: by mxa.mailgun.org with SMTP id 1; Wed, 16 Sep 2026 07:05:00 +0000\r\n" +
	"Authentication-Results: mxa.mailgun.org; dkim=fail header.d=heliosschool.org; spf=pass smtp.mailfrom=\"parent+caf_=ask@heliosschool.org\"; dmarc=pass header.from=heliosschool.org; arc=pass\r\n" +
	"ARC-Seal: i=2; a=rsa-sha256; cv=pass; d=google.com; s=arc-20260327; b=abc\r\n" +
	"ARC-Authentication-Results: i=2; mx.google.com; dkim=pass header.i=@heliosschool.org;\r\n" +
	" spf=pass (google.com: domain of hummingbirds.parents+bncabc@heliosschool.org designates 209.85.220.69 as permitted sender) smtp.mailfrom=hummingbirds.parents+bncABC@heliosschool.org;\r\n" +
	" dmarc=pass header.from=heliosschool.org\r\n" +
	"ARC-Seal: i=1; a=rsa-sha256; cv=none; d=google.com; s=arc-20260327; b=def\r\n" +
	"ARC-Authentication-Results: i=1; mx.google.com; spf=pass smtp.mailfrom=maya.lindqvist@heliosschool.org\r\n" +
	"Received: from mail.heliosschool.org by mx.google.com; Tue, 15 Sep 2026 23:59:00 -0700\r\n" +
	"Message-ID: <class-1@heliosschool.org>\r\n" +
	"Date: Tue, 15 Sep 2026 08:30:00 -0700\r\n" +
	"From: Maya Lindqvist <maya.lindqvist@heliosschool.org>\r\n" +
	"To: hummingbirds.parents@heliosschool.org\r\n" +
	"Subject: Field trip\r\n" +
	"List-Id: <hummingbirds.parents.heliosschool.org>\r\n" +
	"Content-Type: text/plain; charset=UTF-8\r\n" +
	"\r\n" +
	"Pack a lunch.\r\n"

const personalMail = "Received: by mxa.mailgun.org with SMTP id 2; Tue, 15 Sep 2026 16:00:00 +0000\r\n" +
	"Message-ID: <personal-1@example.org>\r\n" +
	"Date: Tue, 15 Sep 2026 09:00:00 -0700\r\n" +
	"From: Sam Lee <sam@example.org>\r\n" +
	"To: Parent <parent@heliosns.org>\r\n" +
	"Subject: Playdate?\r\n" +
	"Content-Type: text/plain; charset=us-ascii\r\n" +
	"\r\n" +
	"Saturday at the park?\r\n"

func TestOnlyTheCommunitysBroadcastsAreFiled(t *testing.T) {
	for _, c := range []struct {
		list, from string
		channel    string
		newsletter bool
		ok         bool
	}{
		{"", "Helios School <m@mail1.veracross.com>", "newsletter", true, true},
		{"<3064358178.560896@benchmarkemail.com>", "Helios School <news@heliosschool.org>", "", false, false},
		{"Falcons Parents <falcons.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", "falcons.parents", false, true},
		{"<chat.heliosschool.org>", "Parent <p@gmail.com>", "chat", false, true},
		{"<chat.heliosns.org>", "Parent <p@gmail.com>", "", false, false},
		{"", "Office <hawksandfalcons@heliosschool.org>", "", false, false},
		{"", "\"veracross.com\" <x@elsewhere.example>", "", false, false},
		{"", "x@veracross.com.elsewhere.example", "", false, false},
		{"", "Juliet Arnswald via CampSite <services@mail.campsite-mail.com>", "newsletter", true, true},
		{"", "x@campsite-mail.com.elsewhere.example", "", false, false},
		{"<michelle-level3math.parents.heliosschool.org>", "Teacher <t@heliosschool.org>", "", false, false},
		{"<boardoftrustees.heliosschool.org>", "Chair <c@heliosschool.org>", "", false, false},
		{"<github.com>", "GitHub <noreply@github.com>", "", false, false},
		{"", "Sam Lee <sam@example.org>", "", false, false},
	} {
		channel, newsletter, ok := broadcastChannel(c.list, c.from)
		if ok != c.ok || (ok && (channel != c.channel || newsletter != c.newsletter)) {
			t.Errorf("%q from %q: %q %v %v", c.list, c.from, channel, newsletter, ok)
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
	const campsite = "From: Juliet Arnswald via CampSite <services@mail.campsite-mail.com>\r\n\r\n"
	const campsiteSealed = "dkim=pass header.i=@campsite-mail.com header.s=smtpapi; spf=pass smtp.mailfrom=\"bounces+1035658-cd6e-ian.gulliver=heliosschool.org@mail.campsite-mail.com\"; dmarc=pass header.from=campsite-mail.com"
	const list = "List-Id: <parentsandstaff.heliosschool.org>\r\nFrom: Sunny <sunny@heliosschool.org>\r\n\r\n"
	const groups = "dkim=pass header.i=@heliosschool.org; spf=pass (google.com: domain of parentsandstaff+bnc@heliosschool.org) smtp.mailfrom=parentsandstaff+bncX@heliosschool.org; dmarc=pass header.from=heliosschool.org"
	cases := map[string]bool{
		mailgun + "pass\r\n" + sealed(veracross) + newsletter: true,
		mailgun + "fail\r\n" + sealed(veracross) + newsletter: false,
		mailgun + "pass\r\n" + sealed("dkim=pass header.i=@elsewhere.example; spf=pass smtp.mailfrom=x@elsewhere.example; dmarc=fail header.from=mail1.veracross.com") + newsletter: false,
		mailgun + "pass\r\n" + sealed(campsiteSealed) + campsite: true,
		mailgun + "pass\r\n" + sealed("dkim=pass header.i=@elsewhere.example; spf=pass smtp.mailfrom=x@elsewhere.example; dmarc=fail header.from=campsite-mail.com") + campsite: false,
		mailgun + "pass\r\n" + sealed(groups) + list: true,
		mailgun + "fail\r\n" + sealed(groups) + list: false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=sunny@heliosschool.org; dmarc=pass header.from=heliosschool.org") + list:                            false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=chat+bncX@heliosschool.org; dmarc=pass header.from=heliosschool.org") + list:                        false,
		mailgun + "pass\r\n" + sealed("spf=pass smtp.mailfrom=parentsandstaff+bncX@forger.example; dmarc=pass header.from=forger.example") + list:                 false,
		mailgun + "pass\r\n" + "ARC-Seal: i=1; cv=none; d=forger.example; b=abc\r\nARC-Authentication-Results: i=1; mx.forger.example; " + groups + "\r\n" + list: false,
	}
	for raw, want := range cases {
		lines, _ := mail.SplitMessage([]byte(raw))
		msg, err := netmail.ReadMessage(strings.NewReader(raw + "Words.\r\n"))
		if err != nil {
			t.Fatal(err)
		}
		channel, newsletter, _ := broadcastChannel(msg.Header.Get("List-Id"), msg.Header.Get("From"))
		if got := vouch(lines, channel, newsletter); (got == "") != want {
			t.Errorf("%q: %q", raw, got)
		}
	}
}

func hookMail(mux *http.ServeMux, key, raw string) int {
	timestamp, signature := mailtest.SignMailgun(key, "token", time.Now())
	form := url.Values{"timestamp": {timestamp}, "token": {"token"}, "signature": {signature}, "body-mime": {raw}}
	req := httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestTheMailHookFilesOnlyTheCommunitysVouchedMail(t *testing.T) {
	s := sample(t)
	pics := newPictures(s)
	mux := http.NewServeMux()
	RegisterMailHook(mux, s, pics, "key")
	before := s.Model().Table("DOCUMENT").Len()
	direct := strings.NewReplacer("smtp.mailfrom=hummingbirds.parents+bncABC@", "smtp.mailfrom=maya.lindqvist@", "<class-1@", "<class-2@").Replace(classMail)
	for _, raw := range []string{personalMail, direct, classMail, classMail} {
		if code := hookMail(mux, "key", raw); code != http.StatusOK {
			t.Fatalf("the hook answered %d", code)
		}
	}
	if code := hookMail(mux, "another key", classMail); code != http.StatusNotAcceptable {
		t.Fatalf("an unsigned call: %d", code)
	}
	filed := []string{}
	for _, row := range s.Model().Table("DOCUMENT").All() {
		if row["kind"] == "mail" && row["parent"] == "" && row["name"] == "Field trip" {
			filed = append(filed, row["id"])
		}
	}
	if len(filed) != 1 || s.Model().Table("DOCUMENT").Len() != before+1 {
		t.Fatalf("filed %v, %d documents more", filed, s.Model().Table("DOCUMENT").Len()-before)
	}
	if got := sentToSorted(s, filed[0]); len(got) != 1 || got[0] != "grp00000000030" {
		t.Errorf("the class mail is sent to %v", got)
	}
}
