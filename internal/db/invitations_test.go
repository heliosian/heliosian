package db

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
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
	mux    *http.ServeMux
	sender *mailtest.Recorder
	event  string
}

func newInviteHarness(t *testing.T) *inviteHarness {
	t.Helper()
	s := sample(t)
	h := &inviteHarness{t: t, s: s, mux: http.NewServeMux(), sender: mailtest.NewRecorder(mailtest.From)}
	iv := RegisterInvitations(h.mux, h.mux, s, newPictures(s), testTokens, InvitationConfig{
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
	ids, err := Write(context.Background(), h.s, access.System(setupEnv.System), setupEnv, Batch{Batch: edits})
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
	stamp, sig := mailtest.SignMailgun(loopSigningKey, token, time.Now())
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

func TestAnInvitationEncodesTheHostsEditableName(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Insert: "MEMBER", Row: map[string]any{"group": h.group()["managed_by"], "person": parent, "member": "yes"}}})
	name := "when@reply.heliosian.com\r\nReply-To: attacker@example.net\r\nX-Injected: yes"
	body, err := json.Marshal(Batch{Batch: []Edit{{Set: parent, Cells: map[string]any{"name_long_override": name}}}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := send(t, h.s, http.MethodPost, "application/json", rowan, string(body)); rec.Code != http.StatusOK {
		t.Fatalf("a member sets their own name: %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{"group": h.event, "people": []map[string]string{{"person": staff}}}); rec.Code != http.StatusOK {
		t.Fatalf("the host adds a guest: %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(rowan, http.MethodPost, "/api/do/invite", map[string]any{"group": h.event}); rec.Code != http.StatusOK {
		t.Fatalf("the host sends: %d %s", rec.Code, rec.Body)
	}
	h.waitFor("the copy marked sent", func() bool { return h.copyOf(staff)["sent"] != "" })
	for _, raw := range h.sender.Raws() {
		if raw.Recipient != h.copyOf(staff)["id"] {
			continue
		}
		msg, err := netmail.ReadMessage(strings.NewReader(string(raw.Message)))
		if err != nil {
			t.Fatal(err)
		}
		if len(msg.Header["Reply-To"]) != 1 || msg.Header.Get("X-Injected") != "" || strings.Contains(msg.Header.Get("Reply-To"), "attacker@example.net") {
			t.Fatalf("the host's name injected invitation headers: %v", msg.Header)
		}
		from, err := msg.Header.AddressList("From")
		if err != nil || len(from) != 1 || from[0].Address != mail.AddressOf(mailtest.From) || !strings.Contains(from[0].Name, name) {
			t.Fatalf("the invitation's sender = %v, %v", from, err)
		}
		return
	}
	t.Fatal("no invitation was recorded")
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

func TestPartyInvitationsRequireTickets(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "eligible": ""}}})
	rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": tacoNight, "people": []map[string]string{{"person": avery}},
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("adding a paid party attendee without a payment answered %d: %s", rec.Code, rec.Body)
	}
	if _, ok := h.s.Model().Table("MEMBER").Find(tacoNight, avery); ok {
		t.Fatal("the refused invitation left an unpaid ticket")
	}
}

func TestOutsideInvitationsCannotChangePaidTickets(t *testing.T) {
	h := newInviteHarness(t)
	h.event = tacoNight
	h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "eligible": ""}}})
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": tacoNight, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	}); rec.Code != http.StatusOK {
		t.Fatalf("the host cannot give a complimentary ticket: %d %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	h.waitFor("the party invitation", func() bool { return h.copyOf(coach)["token"] != "" })
	token := h.copyOf(coach)["token"]
	for _, email := range []string{"", "friend@example.net"} {
		before := h.s.Model().Table("PERSON").Len()
		rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Friend", "email": email})
		if rec.Code != http.StatusForbidden {
			t.Fatalf("an outside guest added an unpaid ticket: %d %s", rec.Code, rec.Body)
		}
		if h.s.Model().Table("PERSON").Len() != before {
			t.Fatal("the refused ticket left a guest behind")
		}
	}
	h.setup([]Edit{{Insert: "MEMBER", Row: map[string]any{"group": tacoNight, "person": avery, "member": "yes", "guest_of": coach}}})
	rec := h.do("", http.MethodDelete, "/open/ext/"+token+"/guest", map[string]string{"key": avery})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("an outside guest removed a paid party ticket: %d %s", rec.Code, rec.Body)
	}
	if _, ok := h.s.Model().Table("MEMBER").Find(tacoNight, avery); !ok {
		t.Fatal("the refused removal deleted the ticket")
	}
}

func TestPartyInvitationsRespectCapacityAndEligibility(t *testing.T) {
	for _, test := range []struct {
		name  string
		cells map[string]any
	}{
		{"capacity", map[string]any{"capacity": "1"}},
		{"eligibility", map[string]any{"eligible": "grp00000000001"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{
				{Set: tacoNight, Cells: map[string]any{"join": "others", "price": "", "eligible": ""}},
				{Set: tacoNight, Cells: test.cells},
			})
			if test.name == "eligibility" {
				h.setup([]Edit{
					{Insert: "GROUP", As: "eligible", Row: map[string]any{"kind": "group", "name": "Eligible buyer", "status": "open"}},
					{Insert: "MEMBER", Row: map[string]any{"group": "@eligible", "person": parent, "member": "yes"}},
					{Set: tacoNight, Cells: map[string]any{"eligible": "@eligible", "price": "40"}},
					{Insert: "PAYMENT", Row: map[string]any{"group": tacoNight, "person": parent, "for": avery, "amount": "40", "added": stamp(time.Now())}},
				})
			}
			rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
				"group": tacoNight, "people": []map[string]string{{"person": avery}},
			})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("a party invitation bypassed %s: %d %s", test.name, rec.Code, rec.Body)
			}
		})
	}
}

func TestGuestAddsKeepInvitationMailBehavior(t *testing.T) {
	for _, kind := range []string{"event", "activity", "party"} {
		t.Run(kind, func(t *testing.T) {
			h := newInviteHarness(t)
			h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": kind}}})
			NewMembershipMailer(h.s, MembershipMailConfig{
				Sender: h.sender.Mailgun, Celebrate: h.sender.Mailgun,
				Origin: func(app string) string { return "https://" + app + ".test" },
			})
			rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
				"group": h.event, "send": true, "answer": "yes",
				"people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("adding an invited guest answered %d: %s", rec.Code, rec.Body)
			}
			coach := h.s.Model().personAt("coach@example.net")
			if !h.s.Model().onList(h.group(), coach) || h.s.Model().answerOf(h.group(), coach) != "yes" {
				t.Fatal("the guest's membership and answer were not recorded")
			}
			h.waitFor("the invitation's send recorded", func() bool { return h.copyOf(coach)["sent"] != "" })
			time.Sleep(100 * time.Millisecond)
			messages := h.sender.Messages()
			if len(messages) != 1 {
				t.Fatalf("guest addition sent %v, want only the invitation", subjects(messages))
			}
			msg := messages[0]
			if !strings.Contains(msg.Subject, "You're invited") || !slices.Equal(msg.To, []string{"coach@example.net"}) || len(msg.Attachments) != 1 {
				t.Fatalf("the invitation: subject %q, to %v, attachments %d", msg.Subject, msg.To, len(msg.Attachments))
			}
			calendar := string(msg.Attachments[0].Content)
			if !strings.Contains(calendar, "METHOD:REQUEST") || !strings.Contains(calendar, "UID:"+h.event+whenUID) {
				t.Fatalf("the invitation's calendar does not name the event: %s", calendar)
			}
		})
	}
}

func TestPartyInvitationsAcceptPaidExistingPeople(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{
		{Set: tacoNight, Cells: map[string]any{"join": "others", "eligible": "", "capacity": "20"}},
		{Insert: "PAYMENT", Row: map[string]any{"group": tacoNight, "person": parent, "for": avery, "amount": "40", "added": stamp(time.Now())}},
	})
	rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": tacoNight, "people": []map[string]string{{"person": avery}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("adding an eligible paid attendee answered %d: %s", rec.Code, rec.Body)
	}
	if _, ok := h.s.Model().Table("MEMBER").Find(tacoNight, avery); !ok {
		t.Fatal("the paid attendee has no ticket")
	}
}

func TestFreePartyInvitationsAcceptNewOutsideGuests(t *testing.T) {
	for _, email := range []string{"", "coach@example.net"} {
		t.Run(email, func(t *testing.T) {
			h := newInviteHarness(t)
			h.event = tacoNight
			h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "price": "", "eligible": "", "capacity": "20"}}})
			rec := h.do(rowan, http.MethodPost, "/api/do/add-guests", map[string]any{
				"group": tacoNight, "people": []map[string]string{{"name": "Coach Pat", "email": email}},
			})
			if rec.Code != http.StatusOK {
				t.Fatalf("adding a new free-party guest answered %d: %s", rec.Code, rec.Body)
			}
			found := false
			for _, row := range h.s.Model().Table("MEMBER").Referencing("group", tacoNight) {
				if h.s.Model().nameOf(row["person"]) == "Coach Pat" {
					found = true
				}
			}
			if !found {
				t.Fatal("the new guest has no free-party membership")
			}
		})
	}
}

func TestOutsideFreePartyGuestsCanBringAndRemoveGuests(t *testing.T) {
	h := newInviteHarness(t)
	h.event = tacoNight
	h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "price": "", "eligible": "", "capacity": "20"}}})
	rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": tacoNight, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("the host's invitation answered %d: %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	token := h.copyOf(coach)["token"]
	for _, email := range []string{"", "friend@example.net"} {
		rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Friend", "email": email})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("bringing a free-party guest answered %d: %s", rec.Code, rec.Body)
		}
		guests := h.s.Model().broughtGuests(h.group(), coach)
		if len(guests) != 1 {
			t.Fatalf("the outside guest brought %v, want one guest", guests)
		}
		rec = h.do("", http.MethodDelete, "/open/ext/"+token+"/guest", map[string]string{"key": guests[0]})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("removing their free-party guest answered %d: %s", rec.Code, rec.Body)
		}
		if _, ok := h.s.Model().Table("MEMBER").Find(tacoNight, guests[0]); ok {
			t.Fatal("the removed guest is still on the list")
		}
	}
}

func TestFreePartyGuestWritesRequireAHouseholdSponsor(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: tacoNight, Cells: map[string]any{"join": "others", "price": "", "eligible": "", "capacity": "20"}}})
	ids := h.setup([]Edit{
		{Insert: "PERSON", As: "guest", Row: map[string]any{"source": "guest", "name_long_override": "Pat Coach"}},
		{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@guest", "address": "coach@example.net", "primary": true, "source": "guest"}},
	})
	row := map[string]any{"group": tacoNight, "person": ids[0], "member": "yes", "added_by": parent}
	write := func(as string) *httptest.ResponseRecorder {
		body, err := json.Marshal(Batch{Batch: []Edit{{Insert: "MEMBER", Row: row}}})
		if err != nil {
			t.Fatal(err)
		}
		return send(t, h.s, http.MethodPost, "application/json", as, string(body))
	}
	if rec := write("coach@example.net"); rec.Code != http.StatusForbidden {
		t.Fatalf("a guest wrote through /api/q: %d %s", rec.Code, rec.Body)
	}
	if rec := write(rowan); rec.Code != http.StatusForbidden {
		t.Fatalf("an unseen guest with no sponsor was added: %d %s", rec.Code, rec.Body)
	}
	row["guest_of"] = staff
	if rec := write(rowan); rec.Code != http.StatusForbidden {
		t.Fatalf("someone else's unseen guest was added: %d %s", rec.Code, rec.Body)
	}
	row["guest_of"] = avery
	if rec := write(rowan); rec.Code != http.StatusOK {
		t.Fatalf("a household's new guest was refused: %d %s", rec.Code, rec.Body)
	}
}

func TestPartyOutsideInvitationsStayLimitedAndClose(t *testing.T) {
	h := newInviteHarness(t)
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"kind": "party", "price": ""}}})
	rec := h.do(maya, http.MethodPost, "/api/do/add-guests", map[string]any{
		"group": h.event, "send": true, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("the host's party invitation answered %d: %s", rec.Code, rec.Body)
	}
	coach := h.s.Model().personAt("coach@example.net")
	token := h.copyOf(coach)["token"]
	for i := 0; i < maxGuestInvitations; i++ {
		rec := h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "Friend", "email": fmt.Sprintf("friend%d@example.net", i)})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("party invitation %d answered %d: %s", i, rec.Code, rec.Body)
		}
	}
	rec = h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "One too many", "email": "extra@example.net"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a party invitation past the limit answered %d: %s", rec.Code, rec.Body)
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"status": "closed"}}})
	rec = h.do("", http.MethodPost, "/open/ext/"+token+"/guest", map[string]string{"name": "No email"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a closed party accepted an outside guest: %d %s", rec.Code, rec.Body)
	}
}
