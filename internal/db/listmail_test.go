package db

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const (
	loopSigningKey = "mailgun-signing-key"
	parentsList    = "grp00000000030"
	hummingbirdsTo = "hummingbirds-parents@loop.heliosian.com"
)

type listMailHarness struct {
	t      *testing.T
	s      *Store
	queue  *store.Queue
	mux    *http.ServeMux
	sender *mailtest.Recorder
	mailer *ListMailer
}

func newListMailHarness(t *testing.T) *listMailHarness {
	t.Helper()
	s, queue := sampleWithQueue(t)
	h := &listMailHarness{t: t, s: s, queue: queue, mux: http.NewServeMux(), sender: mailtest.NewRecorder(mailtest.From)}
	h.mailer = RegisterListMail(h.mux, s, queue, newPictures(s, queue), ListMailConfig{Sender: h.sender.Mailgun, SigningKey: loopSigningKey, Key: []byte("key"), Base: "https://loop.test"})
	return h
}

func loopPost(from, id, subject, references string) string {
	_, domain, _ := strings.Cut(mail.AddressOf(from), "@")
	raw := "Authentication-Results: mxa.mailgun.org; dmarc=pass header.from=" + domain + "\r\n" +
		"From: " + from + "\r\nTo: " + hummingbirdsTo + "\r\nSubject: " + subject + "\r\nMessage-ID: <" + id + ">\r\n"
	if references != "" {
		raw += "In-Reply-To: " + references + "\r\nReferences: " + references + "\r\n"
	}
	return raw + "\r\nSee you at 9.\r\n"
}

func loopNotice(raw, recipient string) map[string]string {
	token := fmt.Sprintf("token-%d-%s", len(raw), recipient)
	stamp, sig := mail.SignMailgun(loopSigningKey, token, time.Now())
	return map[string]string{"recipient": recipient, "sender": "rowan.ashdown@example.org", "from": "Rowan Ashdown <rowan.ashdown@example.org>", "subject": "Field trip", "body-mime": raw, "timestamp": stamp, "token": token, "signature": sig}
}

func (h *listMailHarness) serve(r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, r)
	return rec
}

func (h *listMailHarness) inbound(fields map[string]string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(fields)
	r := httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	return h.serve(r)
}

func (h *listMailHarness) posts(group, direction string) []store.Row {
	out := []store.Row{}
	for _, m := range h.s.Model().Table("MESSAGE").Referencing("group", group) {
		if m["kind"] == "post" && m["direction"] == direction {
			out = append(out, m)
		}
	}
	return out
}

func (h *listMailHarness) waitFor(what string, ok func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("waited in vain for %s", what)
}

func (h *listMailHarness) settled(group string) store.Row {
	h.t.Helper()
	var post store.Row
	h.waitFor("the post to settle", func() bool {
		in := h.posts(group, "in")
		if len(in) == 0 {
			return false
		}
		post = in[len(in)-1]
		return post["state"] != postReceived
	})
	return post
}

func (h *listMailHarness) addMember(group, person, member string) {
	h.t.Helper()
	if _, err := Write(context.Background(), h.s, h.queue, h.mailer.pics, access.System(importReader), Env{System: importReader, Now: testNow}, Batch{Batch: []Edit{{Insert: "MEMBER", Row: map[string]any{"group": group, "person": person, "member": member}}}}); err != nil {
		h.t.Fatal(err)
	}
}

var unsubscribeHeader = regexp.MustCompile(`List-Unsubscribe: <mailto:unsubscribe@loop\.heliosian\.com\?subject=([^>]+)>, <https://loop\.test/open/unsubscribe/([^>]+)>`)

func TestAPostIsSentOnToEveryMemberOnceAndFiled(t *testing.T) {
	h := newListMailHarness(t)
	h.addMember(parentsList, staff, "yes")
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "abc@example.org", "Re: Field trip", "")
	if rec := h.inbound(loopNotice(raw, hummingbirdsTo)); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	post := h.settled(parentsList)
	if post["state"] != postSent || post["detail"] != "" || post["from_person"] != parent || post["header_id"] != "abc@example.org" || post["subject"] != "Re: Field trip" {
		t.Fatalf("the received post reads %v", post)
	}
	out := h.posts(parentsList, "out")
	if len(out) != 1 || out[0]["parent"] != post["id"] || out[0]["header_id"] != "abc@example.org" {
		t.Fatalf("the sent post reads %v", out)
	}
	copies := h.s.Model().Table("RECIPIENT").Referencing("message", out[0]["id"])
	sends := h.sender.Raws()
	if len(copies) != 2 || len(sends) != 2 {
		t.Fatalf("%d copies recorded and %d sent, want 2", len(copies), len(sends))
	}
	to := map[string]bool{}
	for _, s := range sends {
		to[s.To[0]] = true
		msg := string(s.Message)
		m := unsubscribeHeader.FindStringSubmatch(msg)
		if m == nil || m[1] != m[2] {
			t.Fatalf("no matching unsubscribe forms in:\n%s", msg)
		}
		if name, email, ok := parseToken([]byte("key"), m[2]); !ok || name != "hummingbirds-parents" || email != s.To[0] {
			t.Fatalf("the token reads %q %q %v", name, email, ok)
		}
		for _, want := range []string{"Subject: Re: [Hummingbirds Parents] Field trip\r\n", "From: \"Rowan Ashdown via Hummingbirds Parents\" <hummingbirds-parents@loop.heliosian.com>\r\n", "X-Helios-Loop: hummingbirds-parents\r\n"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("no %q in:\n%s", want, msg)
			}
		}
		if !strings.HasSuffix(msg, "\r\n\r\nSee you at 9.\r\n") {
			t.Fatalf("the body was changed:\n%s", msg)
		}
	}
	if !to["rowan.ashdown@example.org"] || !to["maya.lindqvist@example.org"] {
		t.Fatalf("sent to %v", to)
	}
	docs := h.s.Model().Table("DOCUMENT").Referencing("message", post["id"])
	if len(docs) != 1 {
		t.Fatalf("the sent post was filed %d times", len(docs))
	}
	if rec := h.inbound(loopNotice(raw, hummingbirdsTo)); rec.Code != http.StatusOK {
		t.Fatalf("a replay answered %d", rec.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if len(h.posts(parentsList, "in")) != 1 || len(h.sender.Raws()) != 2 {
		t.Fatal("a replayed notification recorded or sent the post again")
	}
}

func TestAnAliasAndAFormReachTheList(t *testing.T) {
	h := newListMailHarness(t)
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "alias@example.org", "Hello", "")
	form := url.Values{}
	for k, v := range loopNotice(raw, "K7M2Q9X4V1BNC@loop.heliosian.com") {
		form.Set(k, v)
	}
	r := httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if rec := h.serve(r); rec.Code != http.StatusOK {
		t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
	}
	if post := h.settled(parentsList); post["state"] != postSent {
		t.Fatalf("the post reads %v", post)
	}
}

func TestPostsTheListDoesNotTakeAreDropped(t *testing.T) {
	for name, c := range map[string]struct {
		raw    string
		detail string
		bounce bool
	}{
		"auto-submitted":   {"Auto-Submitted: auto-replied\r\n" + loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "a@example.org", "Accepted: Picnic", ""), "auto-submitted mail", false},
		"looped":           {"X-Helios-Loop: hummingbirds-parents\r\n" + loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "b@example.org", "Again", ""), "already sent through Helios Loop", false},
		"unauthenticated":  {strings.Replace(loopPost("Stranger <stranger@example.net>", "c@example.org", "Hi", ""), "dmarc=pass", "dmarc=fail", 1), "", false},
		"not a member":     {loopPost("Stranger <stranger@example.net>", "d@example.org", "Buy now", ""), "only the email list's members may post", true},
		"a staff outsider": {loopPost("Maya Lindqvist <maya.lindqvist@example.org>", "e@example.org", "Hi all", ""), "only the email list's members may post", true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newListMailHarness(t)
			if rec := h.inbound(loopNotice(c.raw, hummingbirdsTo)); rec.Code != http.StatusOK {
				t.Fatalf("inbound answered %d: %s", rec.Code, rec.Body)
			}
			post := h.settled(parentsList)
			if post["state"] != postDropped || (c.detail != "" && post["detail"] != c.detail) || post["detail"] == "" {
				t.Fatalf("the post reads %v, want dropped with %q", post, c.detail)
			}
			if n := len(h.s.Model().Table("DOCUMENT").Referencing("message", post["id"])); n != 0 {
				t.Fatalf("a dropped post was filed %d times", n)
			}
			bounces := h.sender.Raws()
			if c.bounce != (len(bounces) == 1) || len(bounces) > 1 {
				t.Fatalf("%d notices sent, want a bounce %v", len(bounces), c.bounce)
			}
			if c.bounce && !strings.Contains(string(bounces[0].Message), "only the people on it and its managers can post") {
				t.Fatalf("the notice reads:\n%s", bounces[0].Message)
			}
		})
	}
}

func TestRepliesFollowTheListsReplying(t *testing.T) {
	h := newListMailHarness(t)
	if _, err := Write(context.Background(), h.s, h.queue, h.mailer.pics, access.System(importReader), Env{System: importReader, Now: testNow}, Batch{Batch: []Edit{{Set: parentsList, Cells: map[string]any{"posting": "managers", "replying": "members"}}}}); err != nil {
		t.Fatal(err)
	}
	first := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "first@example.org", "Plan", "")
	h.inbound(loopNotice(first, hummingbirdsTo))
	if post := h.settled(parentsList); post["state"] != postDropped {
		t.Fatalf("a member's new post to a managers-only list reads %v", post)
	}
	if _, err := Write(context.Background(), h.s, h.queue, h.mailer.pics, access.System(importReader), Env{System: importReader, Now: testNow}, Batch{Batch: []Edit{{Set: h.posts(parentsList, "in")[0]["id"], Cells: map[string]any{"state": postSent}}}}); err != nil {
		t.Fatal(err)
	}
	reply := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "reply@example.org", "Re: Plan", "<first@example.org>")
	h.inbound(loopNotice(reply, hummingbirdsTo))
	h.waitFor("the reply", func() bool { return len(h.posts(parentsList, "in")) == 2 })
	if post := h.settled(parentsList); post["header_id"] != "reply@example.org" || post["state"] != postSent {
		t.Fatalf("a member's reply reads %v", post)
	}
}

func TestUnsubscribingByLinkAndByMailKeepsThemOff(t *testing.T) {
	h := newListMailHarness(t)
	h.addMember(parentsList, staff, "yes")
	tok := unsubscribeToken([]byte("key"), "hummingbirds-parents", "rowan.ashdown@example.org")
	page := h.serve(httptest.NewRequest(http.MethodGet, "/open/unsubscribe/"+tok, nil))
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Stop getting mail from <strong>Hummingbirds Parents</strong>") {
		t.Fatalf("the page answered %d: %s", page.Code, page.Body)
	}
	r := httptest.NewRequest(http.MethodPost, "/open/unsubscribe/"+tok, strings.NewReader("List-Unsubscribe=One-Click"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if rec := h.serve(r); rec.Code != http.StatusOK {
		t.Fatalf("one-click answered %d: %s", rec.Code, rec.Body)
	}
	row, ok := h.s.Model().Table("MEMBER").Find(parentsList, parent)
	if !ok || row["member"] != "excluded" || row["note"] != "Unsubscribed by one-click" {
		t.Fatalf("the unsubscribe reads %v", row)
	}
	if again := h.serve(httptest.NewRequest(http.MethodGet, "/open/unsubscribe/"+tok, nil)); !strings.Contains(again.Body.String(), "Already unsubscribed") {
		t.Fatalf("the page after reads %s", again.Body)
	}

	mayaTok := unsubscribeToken([]byte("key"), "hummingbirds-parents", "maya.lindqvist@example.org")
	if rec := h.inbound(loopNotice("Subject: x\r\n\r\nbody\r\n", "unsubscribe@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("an unsubscribe mail with no token answered %d", rec.Code)
	}
	notice := loopNotice("Subject: Re: "+mayaTok+"\r\n\r\nbody\r\n", "unsubscribe@loop.heliosian.com")
	notice["subject"] = "Re: " + mayaTok
	if rec := h.inbound(notice); rec.Code != http.StatusOK {
		t.Fatalf("the unsubscribe mail answered %d", rec.Code)
	}
	if row, ok := h.s.Model().Table("MEMBER").Find(parentsList, staff); !ok || row["member"] != "excluded" || row["note"] != "Unsubscribed by mail from rowan.ashdown@example.org" {
		t.Fatalf("the hand addition's unsubscribe reads %v", row)
	}
	if len(h.posts(parentsList, "in")) != 0 {
		t.Fatal("an unsubscribe mail was taken as a post")
	}
	if rec := h.serve(httptest.NewRequest(http.MethodGet, "/open/unsubscribe/"+tok+"x", nil)); rec.Code != http.StatusNotFound {
		t.Fatalf("a forged token answered %d", rec.Code)
	}
}

func TestARestartResumesAPostNotYetSentOn(t *testing.T) {
	h := newListMailHarness(t)
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "resume@example.org", "Resume", "")
	content := "content/resume"
	if err := h.mailer.pics.bucket.Put(context.Background(), content, mailType, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(context.Background(), h.s, h.queue, h.mailer.pics, access.System(importReader), Env{System: mailerSystem, Now: testNow}, Batch{Batch: []Edit{
		{Insert: "CONTENT", As: "content", Row: map[string]any{"hash": "resume", "blob": content, "mime": mailType, "size": "10"}},
		{Insert: "MESSAGE", Row: map[string]any{"direction": "in", "kind": "post", "group": parentsList, "content": "@content", "created": "2026-10-09 09:00", "state": postReceived}},
	}}); err != nil {
		t.Fatal(err)
	}
	h.mailer.Start()
	if post := h.settled(parentsList); post["state"] != postSent {
		t.Fatalf("the resumed post reads %v", post)
	}
}

func TestMailForNoListAndUnsignedCallsAreRefused(t *testing.T) {
	h := newListMailHarness(t)
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "none@example.org", "Hi", "")
	if rec := h.inbound(loopNotice(raw, "nobody@loop.heliosian.com")); rec.Code != http.StatusOK {
		t.Fatalf("mail for no list answered %d", rec.Code)
	}
	fields := loopNotice(raw, hummingbirdsTo)
	fields["signature"] = "forged"
	if rec := h.inbound(fields); rec.Code != http.StatusNotAcceptable {
		t.Fatalf("an unsigned call answered %d", rec.Code)
	}
	if len(h.s.Model().Table("MESSAGE").Referencing("group", parentsList)) != 0 {
		t.Fatal("a post was recorded")
	}
	bare := http.NewServeMux()
	RegisterListMail(bare, h.s, h.queue, h.mailer.pics, ListMailConfig{})
	rec := httptest.NewRecorder()
	bare.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/hooks/mail/mime", strings.NewReader("{}")))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unconfigured hook answered %d", rec.Code)
	}
}

func loopEvent(kind, severity, from, messageID, recipient, detail string) string {
	stamp, sig := mail.SignMailgun(loopSigningKey, "event-"+kind+recipient, time.Now())
	body, _ := json.Marshal(map[string]any{
		"signature": map[string]string{"timestamp": stamp, "token": "event-" + kind + recipient, "signature": sig},
		"event-data": map[string]any{
			"event": kind, "severity": severity, "timestamp": float64(time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC).Unix()), "recipient": recipient,
			"message":         map[string]any{"headers": map[string]string{"message-id": messageID, "from": from}},
			"delivery-status": map[string]string{"description": detail},
		},
	})
	return string(body)
}

func TestDeliveryEventsSettleEachCopy(t *testing.T) {
	h := newListMailHarness(t)
	h.addMember(parentsList, staff, "yes")
	raw := loopPost("Rowan Ashdown <rowan.ashdown@example.org>", "events@example.org", "Events", "")
	h.inbound(loopNotice(raw, hummingbirdsTo))
	h.settled(parentsList)
	from := "\"Rowan Ashdown via Hummingbirds Parents\" <hummingbirds-parents@loop.heliosian.com>"
	for _, body := range []string{
		loopEvent("delivered", "", from, "<events@example.org>", "rowan.ashdown@example.org", ""),
		loopEvent("failed", "temporary", from, "<events@example.org>", "maya.lindqvist@example.org", "try later"),
		loopEvent("failed", "permanent", from, "<events@example.org>", "maya.lindqvist@example.org", "no such mailbox"),
		loopEvent("delivered", "", "team@loop.heliosian.com", "<other@example.org>", "rowan.ashdown@example.org", ""),
	} {
		r := httptest.NewRequest(http.MethodPost, "/hooks/events", strings.NewReader(body))
		if rec := h.serve(r); rec.Code != http.StatusOK {
			t.Fatalf("the event answered %d: %s", rec.Code, rec.Body)
		}
	}
	out := h.posts(parentsList, "out")[0]
	h.waitFor("the deliveries", func() bool {
		settled := 0
		for _, c := range h.s.Model().Table("RECIPIENT").Referencing("message", out["id"]) {
			if c["delivered"] != "" || c["failed"] != "" {
				settled++
			}
		}
		return settled == 2
	})
	for _, c := range h.s.Model().Table("RECIPIENT").Referencing("message", out["id"]) {
		switch c["person"] {
		case parent:
			if c["delivered"] != "2026-10-09 09:00:00" || c["failed"] != "" {
				t.Errorf("the delivered copy reads %v", c)
			}
		case staff:
			if c["failed"] != "2026-10-09 09:00:00" || c["detail"] != "no such mailbox" || c["delivered"] != "" {
				t.Errorf("the bounced copy reads %v", c)
			}
		}
	}
}
