package mail

import "testing"

const post = "Received: from mx.example.org by inbound.resend.com\r\n" +
	"Authentication-Results: mxa.mailgun.org;\r\n dkim=pass header.d=gmail.com header.s=20230601 header.b=abc;\r\n" +
	" spf=pass (domain gmail.com designates 1.2.3.4 as permitted sender) smtp.mailfrom=\"alice@gmail.com\";\r\n" +
	" dmarc=pass (dkim=pass domain=gmail.com; spf=pass domain=gmail.com) header.from=gmail.com\r\n" +
	"DKIM-Signature: v=1; a=rsa-sha256; d=gmail.com; s=20230601;\r\n\tb=abc\r\n" +
	"Return-Path: <alice@gmail.com>\r\n" +
	"From: =?utf-8?q?Alice_Smith?= <alice@gmail.com>\r\n" +
	"To: soccer-team@loop.heliosian.com\r\n" +
	"Subject: Re: Saturday's game\r\n" +
	"\r\n" +
	"See you at 9.\r\n"

func TestAuthenticatedTakesMailgunsAlignedResults(t *testing.T) {
	const mailgun = "Authentication-Results: mxa.mailgun.org;\r\n "
	cases := map[string]string{
		mailgun + "dkim=pass header.d=x.org; spf=fail smtp.mailfrom=a@x.org; dmarc=pass (dkim=pass domain=x.org; spf=fail domain=x.org) header.from=x.org\r\nFrom: A <a@x.org>\r\n\r\n": "",
		mailgun + "dkim=pass header.d=x.org; spf=none smtp.mailfrom=a@x.org; dmarc=none header.from=x.org\r\nFrom: a@x.org\r\n\r\n":                                                     "",
		mailgun + "dkim=none; spf=pass (x.org designates it) smtp.mailfrom=\"bounce@mail.x.org\"; dmarc=none header.from=x.org\r\nFrom: a@x.org\r\n\r\n":                                "",
		mailgun + "dkim=pass header.d=x.org; spf=pass smtp.mailfrom=a@x.org; dmarc=pass header.from=x.org\r\nFrom: A <A@X.org>\r\n\r\n":                                                 "",
		mailgun + "dkim=pass header.d=spammer.example; spf=pass smtp.mailfrom=s@spammer.example; dmarc=fail header.from=x.org\r\nFrom: a@x.org\r\n\r\n":                                 "the sender's address passed neither SPF nor DKIM",
		mailgun + "dkim=pass header.d=notx.org; spf=softfail smtp.mailfrom=a@x.org; dmarc=fail header.from=x.org\r\nFrom: a@x.org\r\n\r\n":                                              "the sender's address passed neither SPF nor DKIM",
		mailgun + "dkim=fail header.d=x.org; spf=fail smtp.mailfrom=a@x.org\r\nAuthentication-Results: mxa.mailgun.org; dmarc=pass header.from=x.org\r\nFrom: a@x.org\r\n\r\n":          "the sender's address passed neither SPF nor DKIM",
		"Authentication-Results: mx.forger.example; dmarc=pass header.from=x.org\r\nFrom: a@x.org\r\n\r\n":                                                                              "no authentication results from Mailgun",
		"From: a@x.org\r\n\r\n": "no authentication results from Mailgun",
	}
	for raw, want := range cases {
		lines, _ := SplitMessage([]byte(raw))
		if got := Authenticated(lines); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
	lines, body := SplitMessage([]byte(post))
	if got := Authenticated(lines); got != "" {
		t.Errorf("the sample post: %q", got)
	}
	if string(body) != "See you at 9.\r\n" {
		t.Errorf("body %q", body)
	}
}

func TestAuthenticatedTakesGooglesSealedResults(t *testing.T) {
	const relayed = "Authentication-Results: mxa.mailgun.org;\r\n dkim=pass header.d=heliosian.com; spf=pass smtp.mailfrom=\"team+b@heliosian.com\"; dmarc=fail (dkim=none) header.from=x.org; arc="
	seal := func(i, d string) string {
		return "ARC-Seal: i=" + i + "; a=rsa-sha256; t=1789839813; cv=pass;\r\n        d=" + d + "; s=arc-20260327;\r\n        b=f4CKeXa7\r\n         68qs==\r\n"
	}
	results := func(i, dmarc string) string {
		return "ARC-Authentication-Results: i=" + i + "; mx.google.com;\r\n       dkim=pass header.i=@x.org header.s=fm2;\r\n       dmarc=" + dmarc + " (p=NONE sp=NONE dis=NONE) header.from=x.org\r\n"
	}
	const from = "From: \"A\" <a@x.org>\r\n\r\n"
	cases := map[string]string{
		relayed + "pass\r\n" + seal("2", "google.com") + results("2", "pass") + seal("1", "google.com") + results("1", "pass") + from:     "",
		relayed + "fail\r\n" + seal("2", "google.com") + results("2", "pass") + seal("1", "google.com") + results("1", "pass") + from:     "the sender's address passed neither SPF nor DKIM",
		relayed + "pass\r\n" + seal("2", "forger.example") + results("2", "pass") + seal("1", "google.com") + results("1", "pass") + from: "the sender's address passed neither SPF nor DKIM",
		relayed + "pass\r\n" + seal("2", "google.com") + results("2", "fail") + seal("1", "google.com") + results("1", "pass") + from:     "the sender's address passed neither SPF nor DKIM",
		relayed + "pass\r\n" + seal("1", "google.com") + results("1", "pass") + "From: b@y.org\r\n\r\n":                                   "the sender's address passed neither SPF nor DKIM",
		relayed + "pass\r\n" + from: "the sender's address passed neither SPF nor DKIM",
	}
	for raw, want := range cases {
		lines, _ := SplitMessage([]byte(raw))
		if got := Authenticated(lines); got != want {
			t.Errorf("%q: got %q, want %q", raw, got, want)
		}
	}
}
