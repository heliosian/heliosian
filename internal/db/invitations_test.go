package db

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const (
	rowan     = "rowan.ashdown@example.org"
	maya      = "maya.lindqvist@example.org"
	whenReply = "Helios When <when@reply.heliosian.com>"
)

type inviteHarness struct {
	t      *testing.T
	s      *Store
	queue  *store.Queue
	mux    *http.ServeMux
	sender *mailtest.Recorder
	event  string
}

func newInviteHarness(t *testing.T) *inviteHarness {
	t.Helper()
	s, queue := sampleWithQueue(t)
	h := &inviteHarness{t: t, s: s, queue: queue, mux: http.NewServeMux(), sender: mailtest.NewRecorder(mailtest.From)}
	iv := RegisterInvitations(h.mux, h.mux, s, queue, newPictures(s, queue), testTokens, func() time.Time { return testNow }, InvitationConfig{
		Sender: h.sender.Mailgun, ReplyTo: whenReply, SigningKey: loopSigningKey, Origin: func(app string) string { return "https://" + app + ".test" },
	})
	iv.Start()
	start := time.Now().In(School).AddDate(0, 0, 30).Format("2006-01-02") + " 18:00"
	ids := h.setup([]Edit{
		{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Fondue Night Managers", "status": "open", "posting": "none", "replying": "none"}},
		{Set: "@hosts", Cells: map[string]any{"managed_by": "@hosts", "visible_to": "@hosts"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": staff, "member": "yes"}},
		{Insert: "GROUP", As: "event", Row: map[string]any{"kind": "event", "name": "Fondue Night", "status": "open", "start": start, "location": "8 Kelp Lane", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004", "managed_by": "@hosts", "join": "others", "posting": "none", "replying": "none"}},
		{Set: "@event", Cells: map[string]any{"eligible": "@event"}},
	})
	h.event = ids[3]
	return h
}

func (h *inviteHarness) setup(edits []Edit) []string {
	h.t.Helper()
	ids, err := Write(context.Background(), h.s, h.queue, access.System(setupEnv.System), setupEnv, Batch{Batch: edits})
	if err != nil {
		h.t.Fatal(err)
	}
	return ids
}

func (h *inviteHarness) do(as, method, path string, body any) *httptest.ResponseRecorder {
	h.t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if as == "" {
		h.mux.ServeHTTP(rec, r)
		return rec
	}
	serveAs(h.mux, rec, r, as)
	return rec
}

func (h *inviteHarness) group() store.Row {
	g, _ := h.s.Model().Table("GROUP").Get(h.event)
	return g
}

func (h *inviteHarness) waitFor(what string, ok func() bool) {
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

func (h *inviteHarness) mailTo(address, subject string) mail.Message {
	h.t.Helper()
	var found mail.Message
	h.waitFor("mail to "+address+" about "+subject, func() bool {
		for _, m := range h.sender.Messages() {
			if slices.Contains(m.To, address) && strings.Contains(m.Subject, subject) {
				found = m
				return true
			}
		}
		return false
	})
	return found
}

func (h *inviteHarness) copyOf(person string) store.Row {
	return h.s.Model().copiesOf(h.event)[person]
}

func calendarReply(from, uid, standing string) string {
	ics := strings.Join([]string{"BEGIN:VCALENDAR", "METHOD:REPLY", "BEGIN:VEVENT", "UID:" + uid,
		"ATTENDEE;PARTSTAT=" + standing + ":mailto:" + from, "END:VEVENT", "END:VCALENDAR"}, "\r\n")
	return strings.Join([]string{
		"Authentication-Results: mxa.mailgun.org; dmarc=pass header.from=example.org", "From: " + from, "Subject: " + standing,
		"MIME-Version: 1.0", `Content-Type: text/calendar; method=REPLY`, "Content-Transfer-Encoding: base64", "", base64.StdEncoding.EncodeToString([]byte(ics)), "",
	}, "\r\n")
}

func (h *inviteHarness) reply(to, from, uid, standing string) int {
	h.t.Helper()
	raw := calendarReply(from, uid, standing)
	token := fmt.Sprintf("token-%d-%s", time.Now().UnixNano(), standing)
	stamp, sig := mail.SignMailgun(loopSigningKey, token, time.Now())
	return h.do("", http.MethodPost, "/hooks/replies/mime", map[string]string{"recipient": to, "from": from, "body-mime": raw, "timestamp": stamp, "token": token, "signature": sig}).Code
}

func TestAnInvitationGoesOutWhenTheHostSendsIt(t *testing.T) {
	h := newInviteHarness(t)
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": parent}}}); rec.Code != http.StatusOK {
		t.Fatalf("the host adds a guest: %d %s", rec.Code, rec.Body)
	}
	if pending := h.s.Model().pendingOf(h.event); !slices.Contains(pending, parent) {
		t.Fatalf("pending after adding = %v", pending)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(h.sender.Messages()); n != 0 {
		t.Fatalf("adding someone mailed %d messages before the host sent anything", n)
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event}); rec.Code != http.StatusForbidden {
		t.Errorf("a guest sending everyone's invitation: %d", rec.Code)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event}); rec.Code != http.StatusOK {
		t.Fatalf("the host sends: %d %s", rec.Code, rec.Body)
	}
	m := h.mailTo(rowan, "You're invited")
	copied := h.copyOf(parent)
	organizer := "when+" + hex.EncodeToString([]byte(copied["token"])) + "@reply.heliosian.com"
	if !slices.ContainsFunc(m.ReplyTo, func(a string) bool { return strings.Contains(a, organizer) }) || len(m.Attachments) != 1 || !strings.Contains(string(m.Attachments[0].Content), "METHOD:REQUEST") {
		t.Errorf("the invitation: reply-to %v, attachments %d", m.ReplyTo, len(m.Attachments))
	}
	if m.Headers["Message-Id"] != mail.MessageID(copied["id"], mailtest.From) || !strings.Contains(m.HTML, "https://when.test/g/"+h.event) {
		t.Errorf("the invitation's id %q and link", m.Headers["Message-Id"])
	}
	h.waitFor("the copy marked sent", func() bool { return h.copyOf(parent)["sent"] != "" })
	if pending := h.s.Model().pendingOf(h.event); len(pending) != 0 {
		t.Errorf("pending after the send = %v", pending)
	}
	if code := h.reply(organizer, rowan, h.event+whenUID, "ACCEPTED"); code != http.StatusOK || h.s.Model().answerOf(h.group(), parent) != "yes" {
		t.Errorf("an accept by reply: %d, answer %q", code, h.s.Model().answerOf(h.group(), parent))
	}
	h.setup([]Edit{{Insert: "ALIAS", Row: map[string]any{"alias": "0123456789abcdef0123456789abcdef", "target": copied["id"], "hidden": true}}})
	if code := h.reply("when+0123456789abcdef0123456789abcdef@reply.heliosian.com", rowan, "old"+whenUID, "DECLINED"); code != http.StatusOK || h.s.Model().answerOf(h.group(), parent) != "no" {
		t.Errorf("a decline to an old reply address: %d, answer %q", code, h.s.Model().answerOf(h.group(), parent))
	}
	if code := h.reply(organizer, maya, h.event+whenUID, "ACCEPTED"); code != http.StatusOK || h.s.Model().answerOf(h.group(), staff) != "" {
		t.Errorf("someone else's reply to rowan's address was taken")
	}
}

func TestNewcomersAreInvitedOnlyWithAutoInvite(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds someone from outside: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	time.Sleep(50 * time.Millisecond)
	if coach == "" || len(h.sender.Messages()) != 0 {
		t.Fatalf("coach %q, mail %d", coach, len(h.sender.Messages()))
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"auto_invite": true}}})
	m := h.mailTo("coach@example.net", "You're invited")
	token := h.copyOf(coach)["token"]
	if !strings.Contains(m.HTML, "https://when.test/ext/"+token) {
		t.Errorf("someone from outside is sent their own page")
	}
	rec := h.do("", http.MethodGet, "/open/ext/"+token, nil)
	var view extView
	json.Unmarshal(rec.Body.Bytes(), &view)
	if rec.Code != http.StatusOK || view.Title != "Fondue Night" || view.Name != "Coach Pat" || !view.Guests {
		t.Fatalf("the outside page: %d %+v", rec.Code, view)
	}
	h.waitFor("the page's opening noted", func() bool {
		row, ok := h.s.Model().Table("MEMBER").Find(h.event, coach)
		return ok && row["opened"] != ""
	})
	if rec := h.do("", http.MethodPost, "/open/ext/"+token, map[string]string{"answer": "yes"}); rec.Code != http.StatusNoContent || h.s.Model().answerOf(h.group(), coach) != "yes" {
		t.Errorf("an answer from the outside page: %d", rec.Code)
	}
	if rec := h.do("", http.MethodPost, "/open/ext/"+token, map[string]string{"answer": "yes", "key": parent}); rec.Code != http.StatusForbidden {
		t.Errorf("answering for a stranger from the outside page: %d", rec.Code)
	}
	if rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Assistant Kim", "email": "kim@example.net"}); rec.Code != http.StatusNoContent {
		t.Fatalf("bringing a guest from the outside page: %d %s", rec.Code, rec.Body)
	}
	h.mailTo("kim@example.net", "You're invited")
	if rec := h.do("", http.MethodGet, "/open/ext/nosuchtoken", nil); rec.Code != http.StatusNotFound {
		t.Errorf("a token nobody holds: %d", rec.Code)
	}
}

func TestAnOutsideGuestBringsOnlySoManyGuests(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"auto_invite": true}}})
	h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}})
	coach := h.s.Model().personAt("coach@example.net")
	h.mailTo("coach@example.net", "You're invited")
	token := h.copyOf(coach)["token"]
	guest := func(i int) int {
		return h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Guest", "email": fmt.Sprintf("guest%d@example.net", i)}).Code
	}
	for i := 0; i < maxGuestInvitations; i++ {
		if code := guest(i); code != http.StatusNoContent {
			t.Fatalf("guest %d: %d", i, code)
		}
	}
	if code := guest(maxGuestInvitations); code != http.StatusForbidden {
		t.Errorf("one guest too many: %d", code)
	}
	one := h.s.Model().personAt("guest0@example.net")
	h.do("", http.MethodDelete, "/open/ext/"+token+"/guest", map[string]string{"key": one})
	if code := guest(maxGuestInvitations + 1); code != http.StatusForbidden {
		t.Errorf("a guest taken back and another brought: %d", code)
	}
	h.waitFor("the last allowed invitation", func() bool { return h.copyOf(h.s.Model().personAt("guest9@example.net"))["sent"] != "" })
	time.Sleep(50 * time.Millisecond)
	for _, m := range h.sender.Messages() {
		if slices.Contains(m.To, "guest10@example.net") || slices.Contains(m.To, "guest11@example.net") {
			t.Errorf("mail went to %v past the limit", m.To)
		}
	}
}

func TestSimultaneousGuestsStayWithinTheLimit(t *testing.T) {
	h := newInviteHarness(t)
	h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}})
	token := h.copyOf(h.s.Model().personAt("coach@example.net"))["token"]
	codes := make(chan int, 3*maxGuestInvitations)
	for i := 0; i < 3*maxGuestInvitations; i++ {
		go func() {
			codes <- h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Guest", "email": fmt.Sprintf("guest%d@example.net", i)}).Code
		}()
	}
	accepted := 0
	for i := 0; i < 3*maxGuestInvitations; i++ {
		if <-codes == http.StatusNoContent {
			accepted++
		}
	}
	if accepted != maxGuestInvitations {
		t.Errorf("%d guests brought at once, want %d", accepted, maxGuestInvitations)
	}
}

func TestAnOutsideFamilyAnswersForOneAnother(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "family": true, "send": true, "people": []map[string]string{{"name": "Elaine Tse", "email": "elaine@example.net"}, {"name": "Sam Tse"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds a family: %d %s", rec.Code, rec.Body)
	}
	m := h.s.Model()
	elaine := m.personAt("elaine@example.net")
	families := m.familiesOf(elaine)
	if len(families) != 1 || len(m.familyPeople(families[0])) != 2 {
		t.Fatalf("elaine's families %v", families)
	}
	h.mailTo("elaine@example.net", "You're invited")
	h.waitFor("elaine's copy", func() bool { return h.copyOf(elaine)["token"] != "" })
	rec := h.do("", http.MethodGet, "/open/ext/"+h.copyOf(elaine)["token"], nil)
	var view extView
	json.Unmarshal(rec.Body.Bytes(), &view)
	if len(view.Family) != 1 || view.Family[0].Name != "Sam Tse" {
		t.Fatalf("elaine's family on her page: %+v", view.Family)
	}
	sam := view.Family[0].Key
	if rec := h.do("", http.MethodPost, "/open/ext/"+h.copyOf(elaine)["token"], map[string]string{"answer": "no", "key": sam}); rec.Code != http.StatusNoContent || h.s.Model().answerOf(h.group(), sam) != "no" {
		t.Errorf("elaine answering for sam: %d", rec.Code)
	}
}

func TestHostsMailTheListByWhereEachStands(t *testing.T) {
	h := newInviteHarness(t)
	h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "send": true, "people": []map[string]string{{"person": parent}, {"name": "Coach Pat", "email": "coach@example.net"}}})
	h.mailTo(rowan, "You're invited")
	h.mailTo("coach@example.net", "You're invited")
	coach := h.s.Model().personAt("coach@example.net")
	h.do("", http.MethodPost, "/open/ext/"+h.copyOf(coach)["token"], map[string]string{"answer": "yes"})
	ask := map[string]any{"group": h.event, "kind": "reminder", "to": []string{"none"}, "subject": "Still hoping", "text": "Can you make it?"}
	if rec := h.do(maya, http.MethodPost, "/api/do/guest-mail", ask); rec.Code != http.StatusOK {
		t.Fatalf("a reminder: %d %s", rec.Code, rec.Body)
	}
	m := h.mailTo(rowan, "Still hoping")
	if !strings.Contains(m.Text, "Can you make it?") || len(m.Attachments) != 1 {
		t.Errorf("the reminder: %q, %d attachments", m.Text, len(m.Attachments))
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/guest-mail", ask); rec.Code != http.StatusForbidden {
		t.Errorf("a guest mailing the list: %d", rec.Code)
	}
	if rec := h.do(maya, http.MethodPost, "/api/do/guest-mail", map[string]any{"group": h.event, "kind": "cancel", "notify": true, "text": "Snowed in."}); rec.Code != http.StatusOK {
		t.Fatalf("a cancellation: %d %s", rec.Code, rec.Body)
	}
	cancelled := h.mailTo("coach@example.net", "Cancelled")
	if h.group()["status"] != "cancelled" || !strings.Contains(cancelled.Text, "Snowed in.") || len(cancelled.Attachments) != 1 || !strings.Contains(string(cancelled.Attachments[0].Content), "METHOD:CANCEL") {
		t.Errorf("the cancellation: status %q, %q", h.group()["status"], cancelled.Text)
	}
	for _, m := range h.sender.Messages() {
		if slices.Contains(m.To, "coach@example.net") && strings.Contains(m.Subject, "Still hoping") {
			t.Errorf("the reminder to those with no answer reached someone who said yes")
		}
	}
}
