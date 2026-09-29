package when

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/store"
	"heliosian/internal/testkit/mailtest"
)

const (
	host   = "jordan.whitfield@heliosschool.org"
	robin  = "robin.whitfield@heliosschool.org"
	sam    = "sam.whitfield@heliosschool.org"
	ella   = "ella.whitfield@heliosschool.org"
	mia    = "mia.torres@heliosschool.org"
	coach  = "coach@example.org"
	partyA = "pty0000000001"
)

const (
	carpoolID  = "dtg0000000001"
	bookClubID = "dtg0000000003"
)

var testDirectory *model.Directory

func directoryOf() *model.Directory { return testDirectory }

func noSettings() *model.Config { return &model.Config{} }

func testLists(string) []List {
	return []List{{Key: model.TagKey(carpoolID), Name: "Carpool", Kind: "tag", People: []string{mia, robin}}, {Key: "activity:e1", Name: "Book Fair", Kind: "activity", People: []string{mia, robin, ella}}}
}

var (
	testAttendees      []Attendee
	testCelebrateAdmin string
	testMoves          [][4]string
	testHooks          Hooks
)

func invitesApp(t *testing.T) (http.Handler, *Cache, *mailtest.Recorder) {
	h, c, k, _ := invitesAppWith(t)
	return h, c, k
}

func invitesAppWith(t *testing.T) (http.Handler, *Cache, *mailtest.Recorder, *sampleSources) {
	t.Helper()
	cache := sampleCache(t)
	sources := newSampleSources(t)
	testDirectory = sources.model
	kept := keptMail()
	parties := func(id string) *PartyPeople {
		if id != partyA {
			return nil
		}
		return &PartyPeople{Hosts: []string{mia}, Attendees: append([]Attendee{{Email: robin, Name: "Robin Whitfield", Status: "ticket"}, {Email: sam, Name: "Sam Whitfield", Status: "ticket"}, {Name: "A cousin", Status: "waitlist"}}, testAttendees...)}
	}
	testAttendees, testCelebrateAdmin, testMoves = nil, "", nil
	celebrate := Celebrate{
		Party:   parties,
		IsAdmin: func(email string) bool { return email != "" && email == testCelebrateAdmin },
		MoveAddress: func(_ context.Context, actor access.Actor, old, to, name string) error {
			testMoves = append(testMoves, [4]string{actor.Email, old, to, name})
			return nil
		},
	}
	linked := func(email string) []Linked {
		return []Linked{
			{Source: SourceCelebrate, ID: partyA, EventID: partyA, Title: "Fondue Night", Start: "2026-11-14 18:00", End: "2026-11-14 21:00", Path: "/parties/" + partyA, Availability: "available", Hosts: []string{mia}},
			{Source: SourceTeam, ID: "e1", EventID: "tev0000000101", Title: "Book Fair", Start: "2026-11-20 08:00", End: "2026-11-20 15:00", Path: "/activities/e1", Availability: "open", Hosts: []string{mia}},
		}
	}
	mux := http.NewServeMux()
	testDeps = Deps{
		Cache:     cache,
		Images:    memoryImages(),
		Directory: directoryOf,
		Settings:  noSettings,
		Lists:     testLists,
		Linked:    linked,
		SourceID:  linkedSourceID,
		Celebrate: celebrate,
		Sources:   sources.sources,
		Mail:      Mail{Sender: kept.Mailgun, Base: "https://when.heliosian.com", SigningKey: replySecret, ReplyTo: replyTo, Key: replyKey},
		Style:     testStyle,
		Queue:     queue,
	}
	testHooks = Register(mux, testDeps)
	return served(mux, cache, testHooks, directoryOf, linked), cache, kept, sources
}

var testDeps Deps

func fillNow(t *testing.T) {
	t.Helper()
	newApp(testDeps).fillGroups(context.Background())
}

var linkedSources = map[string]string{SourceCelebrate + "/" + partyA: partyA, SourceCelebrate + "/P001": partyA, SourceTeam + "/e1": "e1", SourceTeam + "/E001": "e1"}

func linkedSourceID(source, key string) string {
	return linkedSources[source+"/"+key]
}

func idOf(t *testing.T, cache *Cache, address string) string {
	t.Helper()
	e := cache.Model().Event(address)
	if e == nil {
		t.Fatalf("no event at %s", address)
	}
	if _, ok := id.Parse(e.ID); !ok {
		t.Fatalf("the event at %s was not minted an ID: %q", address, e.ID)
	}
	return e.ID
}

type sampleSources struct {
	model  *model.Directory
	mu     sync.Mutex
	extra  map[string][]string
	tagged map[string][]string
}

func (s *sampleSources) tag(key string, people ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tagged[key] = append(s.tagged[key], people...)
}

func (s *sampleSources) list(key string, people ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.extra = map[string][]string{key: people}
}

func (s *sampleSources) sources() model.AudienceSources {
	return model.AudienceSources{
		Directory: s.model,
		Tags: func(owner string) []model.Tag {
			s.mu.Lock()
			defer s.mu.Unlock()
			tags := s.model.Tags(owner)
			for i, tag := range tags {
				tags[i].People = append(slices.Clone(tag.People), s.tagged[tag.ID]...)
			}
			return tags
		},
		MagicTags: func(string) []model.MagicTag {
			s.mu.Lock()
			defer s.mu.Unlock()
			out := []model.MagicTag{}
			for key, people := range s.extra {
				out = append(out, model.MagicTag{Key: key, Name: key, People: people})
			}
			return out
		},
	}
}

func newSampleSources(t *testing.T) *sampleSources {
	t.Helper()
	return &sampleSources{model: sampleDirectory(t, "sampledata"), tagged: map[string][]string{}}
}

func eventually(t *testing.T, what string, done func() bool) {
	t.Helper()
	for range 500 {
		if done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("never happened: %s", what)
}

func waitFor(kept *mailtest.Recorder, n int) []mail.Message {
	for i := 0; i < 100 && len(kept.Messages()) < n; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return kept.Messages()
}

func rowOf(v InviteView, key string) *GuestRow {
	for i := range v.List {
		if v.List[i].Key == key {
			return &v.List[i]
		}
	}
	return nil
}

func guestKeyOf(t *testing.T, cache *Cache, eventID, of, name string) string {
	t.Helper()
	for _, inv := range cache.Model().Invites[eventID] {
		if inv.GuestOf == of && inv.Name == name {
			return inv.Email
		}
	}
	t.Fatalf("no guest %s of %s on %s: %+v", name, of, eventID, cache.Model().Invites[eventID])
	return ""
}

func viaGroup(cache *Cache, eventID, group string) int {
	n := 0
	for _, inv := range cache.Model().Invites[eventID] {
		if inv.Via == ViaGroup+group {
			n++
		}
	}
	return n
}

func mailTo(kept *mailtest.Recorder, to string) []mail.Message {
	out := []mail.Message{}
	for _, m := range kept.Messages() {
		if m.To[0] == to {
			out = append(out, m)
		}
	}
	return out
}

func TestInvitationLifecycle(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	robinH := as(robin, mux)
	samH := as(sam, mux)
	miaH := as(mia, mux)
	rec := call(t, jordan, "POST", "/api/events", `{"title":"Class meetup","start":"2026-10-10 15:00","end":"2026-10-10 17:00","location":"The park","tags":[`+quoted(t, "Jays")+`],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	if rec.Code != 200 || created(t, rec) != meetup {
		t.Fatalf("share: %d %s", rec.Code, rec.Body)
	}
	e := cache.Model().Event("meetup")
	if e == nil || e.Sharing != SharingLink || e.Status != "" {
		t.Fatalf("shared event = %+v", e)
	}
	waitFor(kept, 1)
	if sent := kept.Messages(); len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Link event added") {
		t.Errorf("admins' mail = %+v", sent)
	}
	if rec := act(t, miaH, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`); rec.Code != 403 {
		t.Errorf("someone else adding: %d", rec.Code)
	}
	if p := pickerOf(t, miaH, "meetup"); len(p.Attendees) != 0 || len(p.OnList) != 0 {
		t.Errorf("someone else's picker: %+v", p)
	}
	picker, settings := pickerOf(t, jordan, "meetup"), settingsOf(t, jordan)
	if len(settings.Lists) != 2 || len(settings.Classrooms) != 9 || len(picker.OnList) != 0 {
		t.Errorf("picker: lists %d classrooms %d on list %d", len(settings.Lists), len(settings.Classrooms), len(picker.OnList))
	}
	rec = act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`","via":"family"},{"email":"`+sam+`","via":"family"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"},{"email":"not-an-address"}]}`)
	if rec.Code != 400 {
		t.Errorf("a bad address among them: %d", rec.Code)
	}
	rec = act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`","via":"family"},{"email":"`+sam+`","via":"family"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	if rec.Code != 204 || len(cache.Model().Invites[meetup]) != 3 || slices.ContainsFunc(cache.Model().Invites[meetup], func(inv Invite) bool { return inv.Requested != "" }) {
		t.Fatalf("add: %d %s, invites %+v", rec.Code, rec.Body, cache.Model().Invites[meetup])
	}
	if inv := cache.Model().Invitations[meetup]; inv == nil || inv.CreatedBy != host || inv.Audience != AudienceBoth || inv.Sent != "" {
		t.Errorf("invitation = %+v", inv)
	}
	if rows := cache.Model().Invites[meetup]; len(rows) != 3 || rows[0].Email != robin || rows[0].Name != "Robin Whitfield" || rows[0].Via != "family" || rows[2].Name != "Coach Lee" || rows[2].Sent != "" {
		t.Errorf("invites = %+v", rows)
	}
	view := calendarOf(t, robinH)
	if slices.ContainsFunc(view.Events, func(e *Event) bool { return e.ID == meetup }) {
		t.Errorf("an unsent invite put the event on Robin's calendar")
	}
	if rec := act(t, jordan, "meetup", "settings", `{"audience":"students","message":"Bring a snack to share!","hosts":["`+mia+`"]}`); rec.Code != 204 {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations[meetup]; inv.Audience != AudienceStudents || inv.Message != "Bring a snack to share!" || strings.Join(inv.Hosts, ",") != mia {
		t.Errorf("invitation after settings = %+v", inv)
	}
	if got := cache.Model().Answered[mia][meetup]; got.Answer != AnswerYes || got.By != host {
		t.Errorf("the co-host's answer = %+v", got)
	}
	waitFor(kept, 2)
	if m := mailTo(kept, mia); len(m) != 1 || m[0].Subject != "[Class meetup] You're a co-host" || !strings.Contains(m[0].Text, "Jordan Whitfield made you a co-host") || m[0].ReplyTo[0] != host {
		t.Errorf("co-host note = %+v", m)
	}
	act(t, jordan, "meetup", "settings", `{"hosts":["`+mia+`"]}`)
	time.Sleep(30 * time.Millisecond)
	if m := mailTo(kept, mia); len(m) != 1 {
		t.Errorf("co-host told twice: %d", len(m))
	}
	if rec := act(t, miaH, "meetup", "invite", `{"people":[{"email":"`+ella+`","via":"search"}]}`); rec.Code != 204 {
		t.Errorf("a co-host adding: %d %s", rec.Code, rec.Body)
	}
	rec = act(t, jordan, "meetup", "send", `{"to":"new"}`)
	if requested := slices.DeleteFunc(slices.Clone(cache.Model().Invites[meetup]), func(inv Invite) bool { return inv.Requested == "" }); rec.Code != 204 || len(requested) != 4 {
		t.Fatalf("send: %d %s, requested %+v", rec.Code, rec.Body, requested)
	}
	sent := waitFor(kept, 6)
	if len(sent) != 6 {
		t.Fatalf("mail after sending: %d", len(sent))
	}
	byTo := map[string]mail.Message{}
	for _, m := range sent[2:] {
		byTo[m.To[0]] = m
	}
	if m, ok := byTo[sam]; !ok || !strings.Contains(m.Text, "Invited: Sam, Robin, Ella") {
		t.Errorf("sam's own mail = %+v", m)
	}
	if m, ok := byTo[robin]; !ok || strings.Join(m.ReplyTo, ",") != host+","+mia+","+replyAddress(meetup, robin) || strings.Join(byTo[sam].ReplyTo, ",") == strings.Join(m.ReplyTo, ",") || m.Subject != "[Class meetup] You're invited!" || !strings.Contains(m.Text, "Hosted by Jordan Whitfield and Mia Torres") || !strings.Contains(m.HTML, "/open/share/"+meetup+".png") || !strings.Contains(m.Text, "Invited: Robin, Sam, Ella") || !strings.Contains(m.HTML, "Bring a snack to share!") || !strings.Contains(m.HTML, "https://when.heliosian.com/e/meetup") || len(m.Attachments) != 1 || !strings.Contains(string(m.Attachments[0].Content), "ATTENDEE;CN="+robin) || !strings.Contains(string(m.Attachments[0].Content), "UID:"+meetup+"@when.heliosian.com") || !strings.Contains(strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", ""), "ORGANIZER;CN=Helios When:mailto:"+mail.AddressOf(replyAddress(meetup, robin))+"\r\n") {
		t.Errorf("robin's mail = %+v\n%s", m, m.Text)
	}
	if m, ok := byTo[coach]; !ok || !strings.Contains(m.Text, "Invited: Coach") {
		t.Errorf("coach's mail = %+v", m)
	}
	if inv := cache.Model().Invitations[meetup]; inv.Sent == "" {
		t.Errorf("invitation not marked sent")
	}
	for _, row := range cache.Model().Invites[meetup] {
		if row.Sent == "" {
			t.Errorf("%s not marked sent", row.Email)
		}
	}
	if rec := act(t, jordan, "meetup", "send", `{"to":"new"}`); rec.Code != 400 {
		t.Errorf("sending again: %d", rec.Code)
	}
	view = calendarOf(t, robinH)
	i := slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == meetup })
	if i < 0 || !view.Events[i].Invitation || !view.Events[i].Invited || slices.Contains(view.Events[i].Tags, TagGoing) {
		t.Errorf("robin's calendar after sending: %d %+v", i, view.Events)
	}
	if rec := call(t, robinH, "POST", settingsPath("save-view"), `{"classrooms":["Hawks"],"tags":[`+quoted(t, "Community")+`]}`); rec.Code != 204 {
		t.Fatalf("robin's saved view: %d %s", rec.Code, rec.Body)
	}
	if up := cache.Model().UpcomingUnder(testDirectory, robin, nil, now(), 0, ""); !slices.ContainsFunc(up, func(u Card) bool { return u.ID == meetup }) {
		t.Errorf("an invitation is not in Robin's Upcoming")
	}
	if rec := call(t, robinH, "POST", settingsPath("forget-view"), ""); rec.Code != 204 {
		t.Fatalf("robin forgets the view: %d %s", rec.Code, rec.Body)
	}
	if up := cache.Model().UpcomingUnder(testDirectory, mia, nil, now(), 0, ""); !slices.ContainsFunc(up, func(u Card) bool { return u.ID == meetup && u.Answer == AnswerYes }) {
		t.Errorf("the event is not in the co-host's Upcoming as a yes")
	}
	v := inviteView(t, robinH, meetup)
	if v.Host || len(v.Mine) != 3 || v.Mine[0].Email != robin || !v.Mine[0].Mine || !v.Mine[1].Mine || !v.Mine[2].Mine || v.List != nil || v.Settings != nil || !v.Guests || len(v.Hosts) != 2 || v.Hosts[0].Name != "Jordan Whitfield" || v.Counts.Invited != 6 || v.Counts.Waiting != 4 {
		t.Errorf("robin's view = %+v", v)
	}
	v = inviteView(t, samH, meetup)
	if len(v.Mine) != 1 || v.Mine[0].Email != sam || !v.Mine[0].Mine {
		t.Errorf("sam's view = %+v", v.Mine)
	}
	if rec := act(t, robinH, "meetup", "answer-for", `{"email":"`+sam+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("robin for sam: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, robinH, "meetup", "answer-for", `{"email":"`+ella+`","answer":"maybe"}`); rec.Code != 204 {
		t.Errorf("robin for ella: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, robinH, "meetup", "answer", `{"answer":"no"}`); rec.Code != 204 {
		t.Errorf("robin's own: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, samH, "meetup", "answer-for", `{"email":"`+robin+`","answer":"yes"}`); rec.Code != 403 {
		t.Errorf("sam for robin: %d", rec.Code)
	}
	if rec := act(t, robinH, "meetup", "answer-for", `{"email":"`+sam+`","answer":"hidden"}`); rec.Code != 400 {
		t.Errorf("hiding for someone else: %d", rec.Code)
	}
	if got := cache.Model().Answered[sam][meetup]; got.Answer != AnswerYes || got.By != robin || got.Via != ViaPage || got.At == "" {
		t.Errorf("sam's answer = %+v", got)
	}
	time.Sleep(30 * time.Millisecond)
	if len(kept.Messages()) != 6 {
		t.Errorf("mail after answering: %d", len(kept.Messages()))
	}
	rec = act(t, robinH, "meetup", "bring-guest", `{"name":"Grandma June"}`)
	if rec.Code != 204 {
		t.Fatalf("guest: %d %s", rec.Code, rec.Body)
	}
	grandma := guestKeyOf(t, cache, meetup, robin, "Grandma June")
	if !strings.HasPrefix(grandma, guestPrefix) || cache.Model().AnswerOf(grandma, meetup) != AnswerYes {
		t.Errorf("guest = %q, answer %q", grandma, cache.Model().AnswerOf(grandma, meetup))
	}
	if g := cache.Model().InviteOf(meetup, grandma); g == nil || g.GuestOf != robin || g.Via != ViaGuest || g.Name != "Grandma June" {
		t.Errorf("guest row = %+v", g)
	}
	if rec := act(t, samH, "meetup", "bring-guest", `{"name":"A friend"}`); rec.Code != 204 {
		t.Errorf("a student's guest for themselves: %d %s", rec.Code, rec.Body)
	}
	rec = act(t, robinH, "meetup", "bring-guest", `{"name":"Uncle Bo","email":"bo@example.org"}`)
	if rec.Code != 204 {
		t.Fatalf("guest with address: %d %s", rec.Code, rec.Body)
	}
	sent = waitFor(kept, 7)
	if bo := mailTo(kept, "bo@example.org"); len(sent) != 7 || len(bo) != 1 || !strings.Contains(bo[0].Text, "Invited: Uncle") {
		t.Errorf("guest's invite = %+v", sent)
	}
	if rec := act(t, miaH, "meetup", "answer", `{"answer":"yes"}`); rec.Code != 204 {
		t.Errorf("mia by link: %d", rec.Code)
	}
	waitFor(kept, 8)
	if m := mailTo(kept, mia); len(m) != 2 || !strings.HasPrefix(m[1].Subject, "Invitation: Class meetup") {
		t.Errorf("mia's own invite = %+v", m)
	}
	v = inviteView(t, jordan, meetup)
	if !v.Host || v.Settings == nil || v.Settings.Message != "Bring a snack to share!" || len(v.List) != 9 || v.Counts.Invited != 9 || v.Counts.Yes != 6 || v.Counts.Maybe != 1 || v.Counts.No != 1 || v.Counts.Waiting != 1 || v.Counts.Guests != 3 {
		t.Errorf("host's view: host %v settings %+v list %d counts %+v", v.Host, v.Settings, len(v.List), v.Counts)
	}
	if r := rowOf(v, sam); r == nil || r.Answer != AnswerYes || r.AnsweredBy != "Robin Whitfield" || !r.Invited || r.Sent == "" || r.Grade != "Grade 3" || r.Household != ella {
		t.Errorf("sam's row = %+v", r)
	}
	if r := rowOf(v, robin); r == nil || r.Answer != AnswerNo || r.AnsweredBy != "" || !r.Mine {
		t.Errorf("robin's row = %+v", r)
	}
	if r := rowOf(v, grandma); r == nil || r.Email != "" || r.Name != "Grandma June" || r.GuestOfName != "Robin Whitfield" || r.Line != "Guest of Robin Whitfield" || !r.Outside || r.Answer != AnswerYes || r.Household != ella {
		t.Errorf("grandma's row = %+v", r)
	}
	if r := rowOf(v, coach); r == nil || r.Line != "Outside Helios" || !r.Outside || r.Answer != "" {
		t.Errorf("coach's row = %+v", r)
	}
	if r := rowOf(v, mia); r == nil || r.Invited || r.Answer != AnswerYes {
		t.Errorf("mia's row = %+v", r)
	}
	if len(v.Coming) != 8 || slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Answer == AnswerNo }) {
		t.Errorf("coming = %d", len(v.Coming))
	}
	if v := inviteView(t, robinH, meetup); len(v.Coming) != 8 || len(v.Mine) != 6 || v.List != nil || slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Answer == AnswerNo || g.Link != "" || g.Sent != "" }) {
		t.Errorf("robin reads who is coming: %d, mine %d, list %d", len(v.Coming), len(v.Mine), len(v.List))
	}
	if rec := act(t, jordan, "meetup", "answer-for", `{"email":"`+robin+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("host for robin: %d", rec.Code)
	}
	if got := cache.Model().Answered[robin][meetup]; got.Answer != AnswerYes || got.By != host {
		t.Errorf("robin's corrected answer = %+v", got)
	}
	if rec := act(t, robinH, "meetup", "uninvite", `{"email":"`+coach+`"}`); rec.Code != 403 {
		t.Errorf("robin removing the coach: %d", rec.Code)
	}
	if rec := act(t, robinH, "meetup", "uninvite", `{"email":"`+grandma+`"}`); rec.Code != 204 {
		t.Errorf("robin removing her guest: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, jordan, "meetup", "uninvite", `{"email":"`+coach+`"}`); rec.Code != 204 {
		t.Errorf("host removing the coach: %d %s", rec.Code, rec.Body)
	}
	if rows := cache.Model().Invites[meetup]; len(rows) != 5 || cache.Model().InviteOf(meetup, coach) != nil || cache.Model().AnswerOf(grandma, meetup) != "" {
		t.Errorf("after removals: %+v", rows)
	}
	if rec := act(t, jordan, "meetup", "send", `{"to":"unanswered"}`); rec.Code != 400 {
		t.Errorf("reminder with nobody waiting: %d %s", rec.Code, rec.Body)
	}
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+mia+`","via":"search"}]}`)
	act(t, jordan, "meetup", "answer-for", `{"email":"`+mia+`","answer":""}`)
	rec = act(t, jordan, "meetup", "send", `{"to":"unanswered"}`)
	if rec.Code != 204 || cache.Model().InviteOf(meetup, mia).Requested == "" {
		t.Errorf("reminder: %d %s", rec.Code, rec.Body)
	}
	if sent := waitFor(kept, 9); len(sent) != 9 {
		t.Errorf("mail after the reminder: %d", len(sent))
	}
	if m := mailTo(kept, mia); len(m) != 3 || m[2].Subject != "[Class meetup] You're invited!" {
		t.Errorf("mia's first invite = %+v", m)
	}
	rec = act(t, jordan, "meetup", "send", `{"to":"unanswered"}`)
	waitFor(kept, 10)
	if m := mailTo(kept, mia); rec.Code != 204 || len(m) != 4 || m[3].Subject != "[Class meetup] Reminder: you're invited!" || !strings.Contains(m[3].Text, "is still hoping to hear from Mia about") {
		t.Errorf("mia's reminder = %d %+v", rec.Code, m)
	}
	rec = act(t, robinH, "meetup", "bring-guest", `{"name":"Cousin Vi","email":"vi@example.org","answer":"","invite":false}`)
	if rec.Code != 204 || cache.Model().AnswerOf("vi@example.org", meetup) != "" || cache.Model().InviteOf(meetup, "vi@example.org").Sent != "" {
		t.Errorf("a guest left to answer: %d %s, answer %q, row %+v", rec.Code, rec.Body, cache.Model().AnswerOf("vi@example.org", meetup), cache.Model().InviteOf(meetup, "vi@example.org"))
	}
	if rec := act(t, robinH, "meetup", "bring-guest", `{"name":"X","answer":"no"}`); rec.Code != 400 {
		t.Errorf("a guest put down as no: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "send", `{"emails":["vi@example.org"]}`); rec.Code != 204 || cache.Model().InviteOf(meetup, "vi@example.org").Requested == "" {
		t.Errorf("send to one: %d %s", rec.Code, rec.Body)
	}
	eventually(t, "vi's invitation goes and is stamped sent", func() bool { return cache.Model().InviteOf(meetup, "vi@example.org").Sent != "" })
}

func TestPartyInvitation(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	miaH := as(mia, mux)
	robinH := as(robin, mux)
	if rec := call(t, robinH, "POST", "/api/invite-groups", `{"id":"`+partyA+`","rule":{"tags":["`+model.TagKey(carpoolID)+`"]}}`); rec.Code != 403 {
		t.Errorf("a ticket holder adding a group: %d", rec.Code)
	}
	if picker := pickerOf(t, miaH, partyA); len(picker.Attendees) != 3 || picker.Attendees[0].Email != robin || picker.Attendees[2].Status != "waitlist" {
		t.Errorf("party picker: %+v", picker.Attendees)
	}
	if rec := act(t, miaH, partyA, "invite", `{"people":[{"email":"`+robin+`","via":"tickets"},{"email":"`+sam+`","via":"tickets"},{"email":"`+ella+`","via":"family"}]}`); rec.Code != 204 {
		t.Fatalf("party list: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, miaH, partyA, "send", `{}`); rec.Code != 204 {
		t.Fatalf("party send: %d %s", rec.Code, rec.Body)
	}
	sent := waitFor(kept, 3)
	if r := mailTo(kept, robin); len(sent) != 3 || len(r) != 1 || !strings.Contains(r[0].Subject, "[Fondue Night] You're invited!") || !strings.Contains(string(r[0].Attachments[0].Content), "UID:"+partyA+"@when.heliosian.com") || len(mailTo(kept, sam)) != 1 {
		t.Errorf("party invite = %+v", sent)
	}
	act(t, robinH, partyA, "answer-for", `{"email":"`+sam+`","answer":"yes"}`)
	v := inviteView(t, miaH, partyA)
	if !v.Host || !v.Party || v.Counts.Tickets != 2 || v.Counts.TicketsWaiting != 1 || v.Counts.Invited != 3 {
		t.Errorf("party view: host %v party %v counts %+v", v.Host, v.Party, v.Counts)
	}
	if r := rowOf(v, robin); r == nil || r.Ticket != "ticket" || r.Answer != "" {
		t.Errorf("robin on the party = %+v", r)
	}
	for _, g := range inviteView(t, robinH, partyA).Coming {
		if g.Ticket != "" || g.Sent != "" || g.Via != "" || g.Warning != "" || g.AnsweredBy != "" {
			t.Errorf("a guest's coming row carries the hosts' fields: %+v", g)
		}
	}
	if r := rowOf(v, ella); r == nil || r.Ticket != "" {
		t.Errorf("ella on the party = %+v", r)
	}
	if cache.Model().AnswerOf(sam, partyA) != AnswerYes {
		t.Errorf("sam's party answer = %q", cache.Model().AnswerOf(sam, partyA))
	}
}

func TestRepliesFromGuests(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[`+quoted(t, "Jays")+`],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	replies := map[string]string{
		"coach": replyMail(coach, coach, meetup, "ACCEPTED", "dkim=pass header.d=example.org"),
		"robin": replyMail(robin, robin, meetup, "TENTATIVE", "spf=pass smtp.mailfrom=robin.whitfield@heliosschool.org"),
	}
	post := func(id, from string) int {
		return postReply(mux, replyAddress(meetup, from), from, replies[id], true)
	}
	if code := post("coach", coach); code != 200 || cache.Model().AnswerOf(coach, meetup) != "" {
		t.Errorf("a stranger's reply before the list: %d %q", code, cache.Model().AnswerOf(coach, meetup))
	}
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+coach+`","name":"Coach Lee"},{"email":"`+robin+`"}]}`)
	if code := post("coach", coach); code != 200 || cache.Model().AnswerOf(coach, meetup) != AnswerYes {
		t.Errorf("a guest's reply: %d %q", code, cache.Model().AnswerOf(coach, meetup))
	}
	if code := post("robin", robin); code != 200 || cache.Model().AnswerOf(robin, meetup) != AnswerMaybe || cache.Model().Answered[robin][meetup].Via != ViaCalendar {
		t.Errorf("a tentative reply: %d %+v", code, cache.Model().Answered[robin][meetup])
	}
}

func TestInviteOnlyIsForTheInvited(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	robinH := as(robin, mux)
	miaH := as(mia, mux)
	rec := call(t, jordan, "POST", "/api/events", `{"title":"Sam’s party","start":"2026-10-10 15:00","tags":[],"sharing":"Invite Only","address":"party"}`)
	if rec.Code != 200 {
		t.Fatalf("share: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("party"); e == nil || e.Sharing != SharingInvited || e.Status != "" || e.Pending {
		t.Fatalf("event = %+v", e)
	}
	shut := func(h http.Handler, who string) {
		t.Helper()
		if rec := eventOf(t, h, "party"); rec.Code != 404 {
			t.Errorf("%s opens the page: %d", who, rec.Code)
		}
		if rec := call(t, h, "GET", "/api/events/party?include=guest-list", ""); rec.Code != 404 {
			t.Errorf("%s reads the list: %d %s", who, rec.Code, rec.Body)
		}
		if rec := act(t, h, "party", "answer", `{"answer":"yes"}`); rec.Code != 404 {
			t.Errorf("%s answers: %d", who, rec.Code)
		}
		if rec := act(t, h, "party", "bring-guest", `{"name":"Plus one"}`); rec.Code != 404 {
			t.Errorf("%s brings a guest: %d", who, rec.Code)
		}
	}
	shut(robinH, "robin, not yet invited")
	shut(miaH, "mia")
	if v := inviteView(t, jordan, "party"); !v.Host || v.List == nil {
		t.Errorf("host's view = %+v", v)
	}
	act(t, jordan, "party", "invite", `{"people":[{"email":"`+sam+`"}]}`)
	if rec := eventOf(t, as(sam, mux), "party"); rec.Code != 200 {
		t.Errorf("someone on the list, unsent: %d", rec.Code)
	}
	if rec := eventOf(t, robinH, "party"); rec.Code != 200 {
		t.Errorf("a parent of someone on the list, unsent: %d", rec.Code)
	}
	shut(miaH, "mia, with sam on the list")
	if rec := act(t, jordan, "party", "send", `{"to":"new"}`); rec.Code != 204 {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, robinH, "party", "answer", `{"answer":"yes"}`); rec.Code != 204 {
		t.Errorf("robin's own yes: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, robinH, "party", "bring-guest", `{"name":"Grandma June","of":"`+sam+`"}`); rec.Code != 204 {
		t.Errorf("robin brings a guest for sam: %d %s", rec.Code, rec.Body)
	}
	if v := inviteView(t, robinH, "party"); v.Host || v.List != nil || len(v.Coming) != 4 || slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Link != "" || g.Sent != "" }) {
		t.Errorf("robin's view: host %v, list %d, coming %d", v.Host, len(v.List), len(v.Coming))
	}
	shut(miaH, "mia, with the invites out")
}

func TestOutsideInvitation(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","location":"The park","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	if e := cache.Model().Event("meetup"); e == nil || len(e.Tags) != 0 {
		t.Fatalf("an invite-only event with no tags = %+v", e)
	}
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "settings", `{"message":"Bring cleats."}`)
	if rec := call(t, mux, "GET", "/open/banner/meetup", ""); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/") {
		t.Errorf("banner: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	inv := cache.Model().InviteOf(meetup, coach)
	if inv == nil || len(inv.Token) != 24 || cache.Model().InviteOf(meetup, robin).Token != "" {
		t.Fatalf("tokens: coach %+v, robin %+v", inv, cache.Model().InviteOf(meetup, robin))
	}
	if r := rowOf(inviteView(t, jordan, meetup), coach); r == nil || r.Link != "/ext/"+inv.Token {
		t.Errorf("coach's row = %+v", r)
	}
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 3)
	if m := mailTo(kept, coach); len(m) != 1 || !strings.Contains(m[0].HTML, "https://when.heliosian.com/ext/"+inv.Token) || strings.Contains(m[0].Text, "/e/meetup") || !strings.Contains(string(m[0].Attachments[0].Content), "URL:https://when.heliosian.com/ext/"+inv.Token) || !strings.Contains(m[0].HTML, "no account needed") {
		t.Errorf("coach's mail = %+v", m)
	}
	if m := mailTo(kept, robin); len(m) != 1 || !strings.Contains(m[0].HTML, "https://when.heliosian.com/e/meetup") || strings.Contains(m[0].HTML, "/ext/") {
		t.Errorf("robin's mail = %+v", m)
	}
	if rec := call(t, mux, "GET", "/ext/"+inv.Token, ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<html") {
		t.Errorf("outside page: %d", rec.Code)
	}
	if rec := call(t, mux, "GET", "/open/ext/nosuchtoken", ""); rec.Code != 404 {
		t.Errorf("a wrong token: %d", rec.Code)
	}
	rec := call(t, mux, "GET", "/open/ext/"+inv.Token, "")
	var v ExtView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != 200 || v.Title != "Meetup" || v.Name != "Coach Lee" || v.Location != "The park" || v.Message != "Bring cleats." || strings.Join(v.Hosts, ",") != "Jordan Whitfield" || v.Answer != "" || !v.Guests || len(v.Brought) != 0 {
		t.Errorf("outside view = %+v", v)
	}
	if body := rec.Body.String(); strings.Contains(body, robin) || strings.Contains(body, "Robin") {
		t.Errorf("the outside view names someone else: %s", body)
	}
	if rec := call(t, mux, "POST", "/open/ext/"+inv.Token, `{"answer":"maybe"}`); rec.Code != 204 || cache.Model().AnswerOf(coach, meetup) != AnswerMaybe {
		t.Errorf("answer from outside: %d %q", rec.Code, cache.Model().AnswerOf(coach, meetup))
	}
	if rec := call(t, mux, "POST", "/open/ext/"+inv.Token, `{"answer":"hidden"}`); rec.Code != 400 {
		t.Errorf("hiding from outside: %d", rec.Code)
	}
	rec = call(t, mux, "POST", "/open/ext/"+inv.Token+"/guest", `{"name":"Assistant Coach","email":"assistant@example.org"}`)
	if rec.Code != 200 {
		t.Fatalf("guest from outside: %d %s", rec.Code, rec.Body)
	}
	guest := cache.Model().InviteOf(meetup, "assistant@example.org")
	if guest == nil || guest.GuestOf != coach || guest.Token == "" || cache.Model().AnswerOf("assistant@example.org", meetup) != AnswerYes {
		t.Fatalf("guest row = %+v", guest)
	}
	waitFor(kept, 4)
	if m := mailTo(kept, "assistant@example.org"); len(m) != 1 || !strings.Contains(m[0].HTML, "/ext/"+guest.Token) {
		t.Errorf("guest's mail = %+v", m)
	}
	rec = call(t, mux, "GET", "/open/ext/"+inv.Token, "")
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Answer != AnswerMaybe || len(v.Brought) != 1 || v.Brought[0].Name != "Assistant Coach" || v.Brought[0].Key != "assistant@example.org" {
		t.Errorf("outside view after answering = %+v", v)
	}
	if rec := call(t, mux, "DELETE", "/open/ext/"+inv.Token+"/guest", `{"key":"`+robin+`"}`); rec.Code != 404 {
		t.Errorf("taking someone else off from outside: %d", rec.Code)
	}
	if rec := call(t, mux, "DELETE", "/open/ext/"+inv.Token+"/guest", `{"key":"assistant@example.org"}`); rec.Code != 204 || cache.Model().InviteOf(meetup, "assistant@example.org") != nil {
		t.Errorf("taking a guest back from outside: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "settings", `{"flyer":"sample/community.jpg"}`); rec.Code != 204 {
		t.Errorf("flyer: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, mux, "GET", "/open/ext/"+inv.Token, "")
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Flyer != "/open/flyer/"+meetup {
		t.Errorf("outside view's flyer = %q", v.Flyer)
	}
	if rec := call(t, mux, "GET", "/ext/"+inv.Token, ""); !strings.Contains(rec.Body.String(), `property="og:title" content="Meetup"`) || !strings.Contains(rec.Body.String(), "/open/share/"+meetup+".png") {
		t.Errorf("outside page's head lacks the preview: %s", rec.Body.String()[:400])
	}
}

func TestInviteGroups(t *testing.T) {
	mux, cache, kept, sources := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	rec := call(t, jordan, "GET", "/api/when/invites/options", "")
	var options model.AudienceOptions
	json.Unmarshal(rec.Body.Bytes(), &options)
	carpool := model.TagKey(carpoolID)
	if rec.Code != 200 || !slices.Contains(options.Classrooms, "Jays") || !slices.ContainsFunc(options.Tags, func(t model.TagOption) bool { return t.Key == carpool && t.Name == "Carpool" }) {
		t.Fatalf("options: %d %+v", rec.Code, options)
	}
	rec = call(t, jordan, "POST", "/api/when/invites/preview", `{"id":"meetup","rule":{"tags":["`+carpool+`"]}}`)
	var preview struct {
		Count int
		Names []string
	}
	json.Unmarshal(rec.Body.Bytes(), &preview)
	if rec.Code != 200 || preview.Count != 2 || len(preview.Names) != 2 {
		t.Errorf("preview: %d %+v", rec.Code, preview)
	}
	theirs := call(t, jordan, "POST", "/api/when/invites/preview", `{"id":"meetup","rule":{"tags":["`+model.TagKey(bookClubID)+`"]}}`)
	gone := call(t, jordan, "POST", "/api/when/invites/preview", `{"id":"meetup","rule":{"tags":["`+model.TagKey("dtg0000000099")+`"]}}`)
	if theirs.Code != 400 || gone.Code != 400 || theirs.Body.String() != gone.Body.String() {
		t.Errorf("someone else's tag: %d %s; a tag that does not exist: %d %s", theirs.Code, theirs.Body, gone.Code, gone.Body)
	}
	if rec := call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{}}`); rec.Code != 400 {
		t.Errorf("an empty rule: %d", rec.Code)
	}
	if rec := call(t, as(mia, mux), "POST", "/api/invite-groups", `{"id":"meetup","rule":{"tags":["`+carpool+`"]}}`); rec.Code != 403 {
		t.Errorf("someone else's group: %d", rec.Code)
	}
	rec = call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{"tags":["`+carpool+`"]}}`)
	made := created(t, rec)
	if rec.Code != 200 || made == "" || viaGroup(cache, meetup, made) != 2 {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	g := cache.Model().GroupOf(meetup, made)
	if g == nil || !g.Auto || g.Rule.Kind != "include" || strings.Join(g.Rule.Tags, ",") != carpool {
		t.Fatalf("group row = %+v", g)
	}
	abena := cache.Model().InviteOf(meetup, "abena.osei@heliosschool.org")
	if abena == nil || abena.Via != ViaGroup+g.ID || abena.AddedBy != host {
		t.Errorf("a member's row = %+v", abena)
	}
	v := inviteView(t, jordan, meetup)
	if len(v.Groups) != 1 || v.Groups[0].Count != 2 || v.Counts.Invited != 3 {
		t.Errorf("view groups = %+v counts %+v", v.Groups, v.Counts)
	}
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 3)
	sources.tag(carpoolID, mia)
	queue.Refresh()
	eventually(t, "a newcomer to an auto group comes on and is sent", func() bool {
		r := cache.Model().InviteOf(meetup, mia)
		return r != nil && r.Via == ViaGroup+g.ID && r.Sent != ""
	})
	if v := inviteView(t, jordan, meetup); v.Groups[0].Count != 3 {
		t.Errorf("groups after mia = %+v", v.Groups)
	}
	waitFor(kept, 4)
	if m := mailTo(kept, mia); len(m) != 1 || !strings.Contains(m[0].Subject, "You're invited!") {
		t.Errorf("mia's auto invite = %+v", m)
	}
	if g := cache.Model().GroupOf(meetup, g.ID); g.Sent == "" {
		t.Errorf("the group is not marked sent after its invites went: %+v", g)
	}
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	before := len(kept.Messages())
	if rec := act(t, jordan, "meetup", "skip", `{"emails":["`+robin+`"]}`); rec.Code != 204 {
		t.Errorf("skip: %d %s", rec.Code, rec.Body)
	}
	if r := rowOf(inviteView(t, jordan, meetup), robin); r == nil || r.Sent == "" || len(kept.Messages()) != before {
		t.Errorf("skipped row = %+v, mail %d -> %d", r, before, len(kept.Messages()))
	}
	if rec := act(t, jordan, "meetup", "skip", `{"emails":["`+robin+`"]}`); rec.Code != 400 {
		t.Errorf("skipping someone sent: %d", rec.Code)
	}
	act(t, jordan, "meetup", "uninvite", `{"email":"`+robin+`"}`)
	rec = call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{"roles":["Student"],"classrooms":["Ospreys"]}}`)
	second := created(t, rec)
	added := viaGroup(cache, meetup, second)
	if rec.Code != 200 || added == 0 {
		t.Fatalf("second group: %d %s", rec.Code, rec.Body)
	}
	if r := rowOf(inviteView(t, jordan, meetup), ella); r == nil || r.Sent != "" {
		t.Errorf("ella came on sent: %+v", r)
	}
	if g2 := cache.Model().GroupOf(meetup, second); g2 == nil || g2.Sent != "" || len(mailTo(kept, ella)) != 0 {
		t.Errorf("a new group's people were sent before the host did: %+v, mail %d", g2, len(mailTo(kept, ella)))
	}
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 4+added)
	if g2 := cache.Model().GroupOf(meetup, second); g2.Sent == "" || len(mailTo(kept, ella)) != 1 {
		t.Errorf("after sending the second group: %+v, mail %d", g2, len(mailTo(kept, ella)))
	}
	if rec := call(t, jordan, "POST", "/api/invite-groups/"+g.ID+"/edit", `{"auto":false}`); rec.Code != 204 {
		t.Errorf("auto off: %d %s", rec.Code, rec.Body)
	}
	sources.tag(carpoolID, robin)
	queue.Refresh()
	eventually(t, "a newcomer to a group with auto off comes on unsent", func() bool {
		r := cache.Model().InviteOf(meetup, robin)
		return r != nil && r.Via == ViaGroup+g.ID && r.Sent == "" && r.Requested == ""
	})
	if m := mailTo(kept, robin); slices.ContainsFunc(m, func(m mail.Message) bool { return strings.Contains(m.HTML, "Invited: Robin") }) {
		t.Errorf("a newcomer was sent with auto off: %+v", m)
	}
	listed := len(cache.Model().Invites[meetup])
	if rec := call(t, jordan, "DELETE", "/api/invite-groups/"+g.ID, ""); rec.Code != 204 || listed-len(cache.Model().Invites[meetup]) != 1 {
		t.Errorf("remove group: %d %s, invites %d -> %d", rec.Code, rec.Body, listed, len(cache.Model().Invites[meetup]))
	}
	if cache.Model().GroupOf(meetup, g.ID) != nil || cache.Model().InviteOf(meetup, mia) == nil || cache.Model().InviteOf(meetup, robin) != nil {
		t.Errorf("after removing: groups %d, mia %v", len(cache.Model().Groups[meetup]), cache.Model().InviteOf(meetup, mia))
	}
	call(t, jordan, "POST", "/api/events", `{"title":"Other","start":"2026-10-11 15:00","tags":[],"sharing":"Link","address":"other"}`)
	other := idOf(t, cache, "other")
	rec = call(t, jordan, "POST", "/api/invite-groups", `{"id":"other","rule":{"roles":["Student"],"classrooms":["Jays"]},"auto":false}`)
	classroom := created(t, rec)
	added = viaGroup(cache, other, classroom)
	if rec.Code != 200 || added == 0 || len(cache.Model().Invites[other]) != added {
		t.Fatalf("classroom group: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, jordan, "DELETE", "/api/invite-groups/"+classroom, ""); rec.Code != 204 || len(cache.Model().Invites[other]) != 0 {
		t.Errorf("remove unsent group: %d %s, left %d", rec.Code, rec.Body, len(cache.Model().Invites[other]))
	}
}

func TestHostMessage(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	const mina = "mina.park@heliosschool.org"
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+sam+`"},{"email":"`+mina+`"},{"email":"`+coach+`","name":"Coach Lee"}]}`)
	act(t, jordan, "meetup", "answer-for", `{"email":"`+robin+`","answer":"yes"}`)
	act(t, jordan, "meetup", "answer-for", `{"email":"`+mina+`","answer":"no"}`)
	waitFor(kept, 1)
	before := len(kept.Messages())
	if rec := act(t, jordan, "meetup", "message", `{"subject":"Chairs","message":"","to":["yes"]}`); rec.Code != 400 {
		t.Errorf("no words: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "message", `{"subject":"  ","message":"Hi","to":["yes"]}`); rec.Code != 400 {
		t.Errorf("no subject: %d", rec.Code)
	}
	if rec := act(t, as(mina, mux), "meetup", "message", `{"subject":"Hi","message":"Hi","to":["yes"]}`); rec.Code != 403 {
		t.Errorf("someone else's message: %d", rec.Code)
	}
	rec := act(t, jordan, "meetup", "message", `{"subject":"Chairs, please","message":"Bring a chair!","to":["yes","maybe","none"]}`)
	if rec.Code != 204 {
		t.Fatalf("message: %d %s", rec.Code, rec.Body)
	}
	sent := waitFor(kept, before+3)
	if len(sent) != before+3 {
		t.Fatalf("mail: %d", len(sent)-before)
	}
	if m := mailTo(kept, sam); len(m) != 1 || !strings.Contains(m[0].Text, "You: No response yet") {
		t.Errorf("sam's message = %+v", m)
	}
	r := mailTo(kept, robin)
	if len(r) != 1 || r[0].Subject != "[Meetup] Chairs, please" || strings.Join(r[0].ReplyTo, ",") != host || !strings.Contains(r[0].Text, "Bring a chair!") || !strings.Contains(r[0].Text, "You: Yes") || !strings.Contains(r[0].Text, "Sam Whitfield: No response yet") || !strings.Contains(r[0].Text, "has not answered yet") || !strings.Contains(r[0].Text, "/e/meetup") {
		t.Errorf("robin's message = %+v", r)
	}
	c := mailTo(kept, coach)
	if len(c) != 1 || !strings.Contains(c[0].Text, "You: No response yet") || !strings.Contains(c[0].Text, "/ext/") {
		t.Errorf("coach's message = %+v", c)
	}
	if m := mailTo(kept, mina); len(m) != 0 {
		t.Errorf("a no was written to: %+v", m)
	}
	act(t, jordan, "meetup", "message", `{"subject":"Next time","message":"Sorry you can't make it.","to":["no"],"attach":true}`)
	waitFor(kept, before+4)
	if m := mailTo(kept, mina); len(m) != 1 || !strings.Contains(m[0].Text, "You: No") || strings.Contains(m[0].Text, "not answered") || len(m[0].Attachments) != 1 || !strings.Contains(string(m[0].Attachments[0].Content), "UID:"+meetup+"@") {
		t.Errorf("mina's message = %+v", m)
	}
	if r := mailTo(kept, robin); len(r[0].Attachments) != 0 {
		t.Errorf("a plain message carried an invite")
	}
	rec = act(t, jordan, "meetup", "message", `{"subject":"Just you","message":"A word for you.","to":[],"emails":["`+coach+`"]}`)
	if sent := waitFor(kept, before+5); rec.Code != 204 || len(sent) != before+5 || len(mailTo(kept, coach)) != 2 {
		t.Errorf("message to named people: %d %s, mail %d", rec.Code, rec.Body, len(sent)-before)
	}
}

func TestPartyStart(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	miaH := as(mia, mux)
	if rec := act(t, as(robin, mux), partyA, "start", ""); rec.Code != 403 {
		t.Errorf("a ticket holder starting: %d", rec.Code)
	}
	rec := act(t, miaH, partyA, "start", "")
	if rec.Code != 204 || len(cache.Model().Groups[partyA]) != 1 {
		t.Fatalf("start: %d %s, groups %+v", rec.Code, rec.Body, cache.Model().Groups[partyA])
	}
	made := cache.Model().Groups[partyA][0].ID
	g := cache.Model().GroupOf(partyA, made)
	if g == nil || !g.Auto || strings.Join(g.Rule.Tags, ",") != "party:"+partyA || cache.Model().Invitations[partyA] == nil {
		t.Errorf("party group = %+v", g)
	}
	rec = act(t, miaH, partyA, "start", "")
	if rec.Code != 204 || len(cache.Model().Groups[partyA]) != 1 || cache.Model().Groups[partyA][0].ID != made {
		t.Errorf("a second start: %d %s, groups %+v", rec.Code, rec.Body, cache.Model().Groups[partyA])
	}
	if sent, _, ok := cache.LinkedRSVPs(nil, SourceCelebrate, partyA); !ok || sent {
		t.Errorf("rsvps before sending: ok %v sent %v", ok, sent)
	}
	act(t, miaH, partyA, "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+sam+`"}]}`)
	act(t, miaH, partyA, "send", `{}`)
	act(t, miaH, partyA, "answer-for", `{"email":"`+robin+`","answer":"maybe"}`)
	sent, answers, ok := cache.LinkedRSVPs(nil, SourceCelebrate, partyA)
	if !ok || !sent || answers[robin] != AnswerMaybe || answers[sam] != "none" || answers[mia] != "" {
		t.Errorf("rsvps = ok %v sent %v %v", ok, sent, answers)
	}
	if _, _, ok := cache.LinkedRSVPs(nil, SourceCelebrate, "nope"); ok {
		t.Errorf("a party with no list has rsvps")
	}
}

func TestLinkedEventsBySourcesOldIDs(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	robinH, miaH := as(robin, mux), as(mia, mux)
	for key, want := range map[string]string{partyA: partyA, "tev0000000101": "tev0000000101"} {
		rec := eventOf(t, robinH, key)
		var got struct{ Result string }
		json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != 200 || got.Result != want {
			t.Errorf("%s: %d %q, want %s", key, rec.Code, got.Result, want)
		}
	}
	for path, want := range map[string]string{"/e/celebrate/P001": "/e/" + partyA, "/e/team/E001": "/e/tev0000000101"} {
		if rec := call(t, mux, "GET", path, ""); rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want a 301 to %s", path, rec.Code, rec.Header().Get("Location"), want)
		}
	}
	if rec := call(t, mux, "GET", "/e/celebrate/P999", ""); rec.Code != 200 || rec.Header().Get("Location") != "" {
		t.Errorf("an unknown party's page: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := call(t, mux, "GET", "/e/"+partyA, ""); rec.Code != 200 {
		t.Errorf("the party's own page: %d", rec.Code)
	}
	rec := act(t, miaH, partyA, "start", "")
	if rec.Code != 204 || len(cache.Model().Groups[partyA]) != 1 || cache.Model().Invitations[partyA] == nil {
		t.Fatalf("a list started on the party: %d %s", rec.Code, rec.Body)
	}
	made := cache.Model().Groups[partyA][0].ID
	if _, ok := id.Parse(made); !ok {
		t.Errorf("a minted group ID does not parse: %q", made)
	}
}

func TestGuestListIDsAreMinted(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	rec := call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{"roles":["Student"],"classrooms":["Jays"]},"auto":false}`)
	group := created(t, rec)
	if _, ok := id.Parse(group); rec.Code != 200 || !ok || cache.Model().GroupOf(meetup, group) == nil {
		t.Errorf("group: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, jordan, "meetup", "bring-guest", `{"name":"Grandma June"}`); rec.Code != 204 {
		t.Fatalf("guest: %d %s", rec.Code, rec.Body)
	}
	keys := []string{guestKeyOf(t, cache, meetup, host, "Grandma June")}
	act(t, jordan, "meetup", "invite", `{"people":[{"name":"Kit Lee","via":"outside","household":"`+coach+`"},{"name":"Kim Lee","via":"outside","household":"`+coach+`"}]}`)
	for _, inv := range cache.Model().Invites[meetup] {
		if inv.Household == coach {
			keys = append(keys, inv.Email)
		}
	}
	if len(keys) != 3 || keys[1] == keys[2] {
		t.Fatalf("guest keys = %v", keys)
	}
	for _, key := range keys {
		minted, ok := strings.CutPrefix(key, guestPrefix)
		if _, parses := id.Parse(minted); !ok || !parses || !isGuestKey(key) {
			t.Errorf("a guest key is not guest- and a minted ID: %q", key)
		}
	}
}

func TestStudentInviteCcsParents(t *testing.T) {
	mux, _, kept := invitesApp(t)
	miaH := as(mia, mux)
	act(t, miaH, partyA, "invite", `{"people":[{"email":"`+sam+`"},{"email":"`+ella+`"}]}`)
	act(t, miaH, partyA, "send", `{}`)
	sent := waitFor(kept, 2)
	if len(sent) != 2 || len(mailTo(kept, robin)) != 0 {
		t.Fatalf("mail after sending: %+v", sent)
	}
	m := mailTo(kept, sam)
	if len(m) != 1 || strings.Join(m[0].CC, ",") != host+","+robin || len(m[0].Attachments) != 0 || strings.Contains(m[0].Text, "invite attached") || strings.Contains(m[0].HTML, "invite attached") {
		t.Fatalf("sam's mail = %+v", m)
	}
	if !strings.Contains(m[0].HTML, "Mia Torres sent Sam an invitation for") || !strings.Contains(m[0].HTML, `<a href="https://when.heliosian.com/e/`+partyA+`" style="color:#1b2a2c;font-weight:700">RSVP for Sam and Ella here</a>`) || !strings.Contains(m[0].Text, "Mia Torres sent Sam an invitation for") || !strings.Contains(m[0].Text, "RSVP for Sam and Ella here") {
		t.Errorf("sam's words:\n%s", m[0].Text)
	}
	if e := mailTo(kept, ella); len(e) != 1 || strings.Join(e[0].CC, ",") != host+","+robin || !strings.Contains(e[0].HTML, "sent Ella an invitation for") {
		t.Errorf("ella's mail = %+v", e)
	}
	if got := joinNames([]string{"Sam", "Ella", "Robin", "Mia"}); got != "Sam, Ella, Robin and Mia" {
		t.Errorf("joinNames = %q", got)
	}
}

func TestStudentMessagesCcParentsWithoutCalendarFiles(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[`+quoted(t, "Jays")+`],"sharing":"Link","address":"meetup"}`)
	idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+sam+`"},{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	before := len(waitFor(kept, 2))
	if rec := act(t, jordan, "meetup", "message", `{"subject":"Chairs","message":"Bring a chair!","to":["none"],"attach":true}`); rec.Code != 204 {
		t.Fatalf("message: %d %s", rec.Code, rec.Body)
	}
	if sent := waitFor(kept, before+2); len(sent) != before+2 {
		t.Fatalf("message mail: %d", len(sent)-before)
	}
	if rec := act(t, jordan, "meetup", "cancel", `{"notify":true,"note":"Rain."}`); rec.Code != 204 {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if sent := waitFor(kept, before+4); len(sent) != before+4 {
		t.Fatalf("cancel mail: %d", len(sent)-before-2)
	}
	s, r := mailTo(kept, sam), mailTo(kept, robin)
	if len(s) != 3 || len(r) != 3 {
		t.Fatalf("sam's mail %d, robin's %d", len(s), len(r))
	}
	for i, m := range s {
		if strings.Join(m.CC, ",") != host+","+robin || len(m.Attachments) != 0 || strings.Contains(m.Text, "attached") {
			t.Errorf("sam's mail %d = %+v", i, m)
		}
	}
	for i, name := range []string{"invite.ics", "invite.ics", "cancel.ics"} {
		if len(r[i].CC) != 0 || len(r[i].Attachments) != 1 || r[i].Attachments[0].Name != name {
			t.Errorf("robin's mail %d = %+v", i, r[i])
		}
	}
	if !strings.Contains(r[0].Text, "invite attached") || !strings.Contains(r[2].Text, "cancellation attached") {
		t.Errorf("robin's words: %q, %q", r[0].Text, r[2].Text)
	}
}

func TestPartyListIsTheHolders(t *testing.T) {
	mux, cache, kept, sources := invitesAppWith(t)
	sources.list("party:"+partyA, mia, robin, sam, host)
	miaH := as(mia, mux)
	rec := act(t, miaH, partyA, "start", "")
	if rec.Code != 204 || len(cache.Model().Groups[partyA]) != 1 || viaGroup(cache, partyA, cache.Model().Groups[partyA][0].ID) != 3 {
		t.Fatalf("start: %d %s, invites %+v", rec.Code, rec.Body, cache.Model().Invites[partyA])
	}
	for _, email := range []string{mia, robin, sam} {
		if cache.Model().InviteOf(partyA, email) == nil {
			t.Errorf("%s is not on the list", email)
		}
	}
	if cache.Model().InviteOf(partyA, host) != nil {
		t.Errorf("the buyer of a ticket is on the list")
	}
	act(t, miaH, partyA, "send", `{}`)
	to := []string{}
	for _, m := range waitFor(kept, 3) {
		to = append(to, m.To...)
	}
	slices.Sort(to)
	if strings.Join(to, ",") != strings.Join([]string{mia, robin, sam}, ",") {
		t.Errorf("sent to %v", to)
	}
}

func TestInvitationDetails(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	miaH := as(mia, mux)
	robinH := as(robin, mux)
	act(t, miaH, partyA, "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee"}]}`)
	if rec := act(t, miaH, partyA, "settings", `{"start":"not a time"}`); rec.Code != 400 {
		t.Errorf("a bad start: %d", rec.Code)
	}
	if rec := act(t, miaH, partyA, "settings", `{"title":"Fondue: the early sitting","start":"2026-11-14 17:30","end":"2026-11-14 19:00","location":"The Torres kitchen","description":"Come early - the kids eat first."}`); rec.Code != 204 {
		t.Fatalf("details: %d %s", rec.Code, rec.Body)
	}
	inv := cache.Model().Invitations[partyA]
	if inv.Title != "Fondue: the early sitting" || inv.Start != "2026-11-14 17:30" || inv.End != "2026-11-14 19:00" || inv.Location != "The Torres kitchen" {
		t.Errorf("invitation = %+v", inv)
	}
	act(t, miaH, partyA, "send", `{}`)
	waitFor(kept, 2)
	view := calendarOf(t, robinH)
	i := slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue: the early sitting" || view.Events[i].Start != "2026-11-14 17:30" || view.Events[i].Location != "The Torres kitchen" {
		t.Errorf("robin's party = %+v", view.Events[i])
	}
	calendarOf(t, as(sam, mux))
	view = calendarOf(t, as("dana.hawkins@heliosschool.org", mux))
	i = slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue Night" || view.Events[i].Start != "2026-11-14 18:00" {
		t.Errorf("an uninvited view of the party = %+v", view.Events[i])
	}
	m := mailTo(kept, robin)
	if len(m) != 1 || !strings.Contains(m[0].Subject, "[Fondue: the early sitting] You're invited!") || !strings.Contains(m[0].Text, "5:30") || !strings.Contains(m[0].Text, "The Torres kitchen") || !strings.Contains(string(m[0].Attachments[0].Content), "SUMMARY:Fondue: the early sitting") || !strings.Contains(string(m[0].Attachments[0].Content), "DTSTART:20261115T013000Z") {
		t.Errorf("robin's invite = %+v", m)
	}
	token := cache.Model().InviteOf(partyA, coach).Token
	rec := call(t, mux, "GET", "/open/ext/"+token, "")
	var ext ExtView
	json.Unmarshal(rec.Body.Bytes(), &ext)
	if ext.Banner != "/open/banner/"+partyA {
		t.Errorf("outside banner = %q", ext.Banner)
	}
	if ext.Title != "Fondue: the early sitting" || ext.Location != "The Torres kitchen" || !strings.Contains(ext.Hours, "5:30") {
		t.Errorf("outside view = %+v", ext)
	}
	act(t, miaH, partyA, "settings", `{"title":"","start":"","location":"","description":""}`)
	view = calendarOf(t, robinH)
	i = slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue Night" {
		t.Errorf("robin's party once blanked = %+v", view.Events[i])
	}
}

func TestAddressWarnings(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	alum := "old.grad@heliosschool.org"
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+alum+`","name":"Old Grad","via":"outside"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	v := inviteView(t, jordan, meetup)
	if r := rowOf(v, alum); r == nil || r.Warning != "unknown" || r.WarningWords == "" {
		t.Errorf("a school address off the directory = %+v", r)
	}
	if r := rowOf(v, coach); r == nil || r.Warning != "" {
		t.Errorf("an outside address = %+v", r)
	}
	if r := rowOf(v, robin); r == nil || r.Warning != "" {
		t.Errorf("a directory address = %+v", r)
	}
	bounce := func(email, severity string) *httptest.ResponseRecorder {
		stamp, sig := mail.SignMailgun(replySecret, "token-"+email, time.Now())
		body, _ := json.Marshal(map[string]any{
			"signature":  map[string]string{"timestamp": stamp, "token": "token-" + email, "signature": sig},
			"event-data": map[string]any{"event": "failed", "severity": severity, "recipient": email, "reason": "bounce", "delivery-status": map[string]any{"message": "550 no such user", "description": "No such user here"}},
		})
		req := httptest.NewRequest("POST", "/hooks/events", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := bounce(coach, "temporary"); rec.Code != 200 || len(cache.Model().Bounced) != 0 {
		t.Errorf("a temporary failure: %d, bounced %v", rec.Code, cache.Model().Bounced)
	}
	if rec := bounce(coach, "permanent"); rec.Code != 200 {
		t.Fatalf("a bounce: %d %s", rec.Code, rec.Body)
	}
	if b, ok := cache.Model().Bounced[coach]; !ok || b.Reason != "No such user here" {
		t.Errorf("bounce noted = %+v (%v)", b, ok)
	}
	if r := rowOf(inviteView(t, jordan, meetup), coach); r == nil || r.Warning != "bounced" || !strings.Contains(r.WarningWords, "No such user here") {
		t.Errorf("a bounced address = %+v", r)
	}
	req := httptest.NewRequest("POST", "/hooks/events", strings.NewReader(`{"signature":{},"event-data":{"event":"failed","severity":"permanent","recipient":"x@example.org"}}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Errorf("unsigned event: %d", rec.Code)
	}
	act(t, jordan, "meetup", "send", `{"emails":["`+coach+`"]}`)
	waitFor(kept, 2)
	if len(mailTo(kept, coach)) != 1 {
		t.Errorf("sending to a bounced address anyway: %d mails", len(mailTo(kept, coach)))
	}
	act(t, jordan, "meetup", "answer-for", `{"email":"`+coach+`","answer":"yes"}`)
	token := cache.Model().InviteOf(meetup, coach).Token
	if rec := act(t, jordan, "meetup", "change-email", `{"email":"`+coach+`","to":"coach.lee@example.org"}`); rec.Code != 204 {
		t.Fatalf("change address: %d %s", rec.Code, rec.Body)
	}
	moved := cache.Model().InviteOf(meetup, "coach.lee@example.org")
	if moved == nil || moved.Token != token || moved.Sent == "" || cache.Model().InviteOf(meetup, coach) != nil || cache.Model().AnswerOf("coach.lee@example.org", meetup) != AnswerYes {
		t.Errorf("after the change: %+v, old %v, answer %q", moved, cache.Model().InviteOf(meetup, coach), cache.Model().AnswerOf("coach.lee@example.org", meetup))
	}
	if r := rowOf(inviteView(t, jordan, meetup), "coach.lee@example.org"); r == nil || r.Warning != "" {
		t.Errorf("the new address = %+v", r)
	}
	if rec := act(t, jordan, "meetup", "change-email", `{"email":"`+robin+`","to":"robin@example.org"}`); rec.Code != 400 {
		t.Errorf("changing a directory address: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "change-email", `{"email":"`+alum+`","to":"coach.lee@example.org"}`); rec.Code != 400 {
		t.Errorf("changing to an address on the list: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "change-email", `{"email":"`+alum+`","to":"not an address"}`); rec.Code != 400 {
		t.Errorf("changing to nonsense: %d", rec.Code)
	}
}

func TestDeleteAndCancel(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	if rec := act(t, as(mia, mux), "meetup", "delete-invitation", ""); rec.Code != 403 {
		t.Errorf("someone else deleting: %d", rec.Code)
	}
	token := cache.Model().InviteOf(meetup, coach).Token
	if rec := act(t, jordan, "meetup", "delete-invitation", ""); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	if m.Event("meetup") != nil || m.Invitations[meetup] != nil || len(m.Invites[meetup]) != 0 {
		t.Errorf("after delete: event %v, invitation %v, invites %d", m.Event("meetup"), m.Invitations[meetup], len(m.Invites[meetup]))
	}
	if rec := call(t, mux, "GET", "/ext/"+token, ""); rec.Code != 200 {
		t.Errorf("outside page for a deleted event: %d", rec.Code)
	}
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup = idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 3)
	before := len(kept.Messages())
	if rec := act(t, jordan, "meetup", "delete-invitation", ""); rec.Code != 403 {
		t.Errorf("deleting once sent: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "cancel", `{"notify":true,"note":"Rain, sadly."}`); rec.Code != 204 {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if sent := waitFor(kept, before+2); len(sent) != before+2 || len(mailTo(kept, robin)) != 2 {
		t.Errorf("told of the cancellation: %d", len(sent)-before)
	}
	if e := cache.Model().Event("meetup"); e == nil || !e.Cancelled || e.Status != StatusCancelled {
		t.Errorf("after cancel: %+v", e)
	}
	if events := cache.Model().eventsFor(testDirectory, robin, nil); slices.ContainsFunc(events, func(e *Event) bool { return e.ID == meetup }) {
		t.Errorf("a cancelled event still on a guest's calendar")
	}
	waitFor(kept, before+2)
	msgs := mailTo(kept, coach)
	if len(msgs) != 2 || msgs[1].Subject != "[Meetup] Cancelled" || !strings.Contains(msgs[1].HTML, "Rain, sadly.") || !strings.Contains(msgs[1].HTML, "Jordan Whitfield has cancelled Meetup") {
		t.Fatalf("the coach's cancellation: %+v", msgs)
	}
	ics := string(msgs[1].Attachments[0].Content)
	if msgs[1].Attachments[0].Name != "cancel.ics" || !strings.Contains(ics, "METHOD:CANCEL") || !strings.Contains(ics, "STATUS:CANCELLED") || !strings.Contains(ics, "UID:"+uidOf(meetup)) {
		t.Errorf("cancellation file:\n%s", ics)
	}
	if !slices.Contains(msgs[1].ReplyTo, host) {
		t.Errorf("reply-to: %v", msgs[1].ReplyTo)
	}
	if rec := act(t, jordan, "meetup", "cancel", `{"notify":true}`); rec.Code != 403 {
		t.Errorf("cancelling twice: %d", rec.Code)
	}
}

func TestUpdatedInvitation(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 2)
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+mia+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	act(t, jordan, "meetup", "send", `{"emails":["`+coach+`"]}`)
	waitFor(kept, 3)
	act(t, jordan, "meetup", "answer-for", `{"email":"`+coach+`","answer":"no"}`)
	if rec := act(t, jordan, "meetup", "edit", `{"title":"Meetup","start":"2026-10-10 16:00","end":"2026-10-10 16:00","location":"The park","tags":[],"sharing":"Link"}`); rec.Code != 204 {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body)
	}
	before := len(kept.Messages())
	if rec := act(t, jordan, "meetup", "send", `{"to":"sent","update":true}`); rec.Code != 204 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if sent := waitFor(kept, before+1); len(sent) != before+1 {
		t.Errorf("update mail: %d", len(sent)-before)
	}
	msgs := mailTo(kept, robin)
	last := msgs[len(msgs)-1]
	if len(msgs) != 2 || last.Subject != "[Meetup] Updated: the details have changed" || !strings.Contains(last.HTML, "has updated the details of") || !strings.Contains(last.HTML, "4:00 PM") || !strings.Contains(last.HTML, "The park") || !strings.Contains(string(last.Attachments[0].Content), "DTSTART:20261010T230000Z") {
		t.Errorf("robin's update: %+v", last)
	}
	if len(mailTo(kept, mia)) != 0 {
		t.Errorf("someone pending was sent the update")
	}
	if len(mailTo(kept, coach)) != 1 {
		t.Errorf("someone who said no was sent the update: %d mails", len(mailTo(kept, coach)))
	}
}

func TestOutsideFamily(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	rec := act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside","household":"`+coach+`"},{"email":"pat@example.org","name":"Pat Lee","via":"outside","household":"`+coach+`"},{"name":"Kit Lee","via":"outside","household":"`+coach+`"}]}`)
	if rows := cache.Model().Invites[meetup]; rec.Code != 204 || len(rows) != 3 || slices.ContainsFunc(rows, func(inv Invite) bool { return inv.Requested != "" }) {
		t.Fatalf("family: %d %s, rows %+v", rec.Code, rec.Body, rows)
	}
	m := cache.Model()
	var kit string
	for _, inv := range m.Invites[meetup] {
		if inv.Name == "Kit Lee" {
			kit = inv.Email
		}
	}
	if kit == "" || !isGuestKey(kit) || m.InviteOf(meetup, kit).Token != "" || m.InviteOf(meetup, kit).Household != coach || m.InviteOf(meetup, "pat@example.org").Token == "" {
		t.Fatalf("family rows = %+v", m.Invites[meetup])
	}
	v := inviteView(t, jordan, meetup)
	if rowOf(v, coach).Household != rowOf(v, "pat@example.org").Household || rowOf(v, kit).Household != rowOf(v, coach).Household {
		t.Errorf("not one household: %q %q %q", rowOf(v, coach).Household, rowOf(v, "pat@example.org").Household, rowOf(v, kit).Household)
	}
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 3)
	if msgs := mailTo(kept, coach); len(msgs) != 1 || !strings.Contains(msgs[0].HTML, "This email is for Coach, Pat, Kit.") || !strings.Contains(msgs[0].HTML, "Jordan Whitfield sent Coach an invitation for") || msgs[0].FromName != "Jordan Whitfield" {
		t.Errorf("the coach's invite: %+v", msgs)
	}
	invites := 0
	for _, msg := range kept.Messages() {
		if strings.Contains(msg.Subject, "You're invited!") {
			invites++
		}
	}
	if len(mailTo(kept, "pat@example.org")) != 1 || invites != 2 {
		t.Errorf("the family's invites: %d", invites)
	}
	token := m.InviteOf(meetup, "pat@example.org").Token
	rec = call(t, mux, "GET", "/open/ext/"+token, "")
	if opened := cache.Model().InviteOf(meetup, "pat@example.org").Opened; opened == "" {
		t.Errorf("pat's page opened, not noted")
	}
	if r := rowOf(inviteView(t, jordan, meetup), "pat@example.org"); r == nil || r.Opened == "" {
		t.Errorf("the host does not see pat opened: %+v", r)
	}
	if r := rowOf(inviteView(t, jordan, meetup), coach); r == nil || r.Opened != "" {
		t.Errorf("the coach, who has not opened: %+v", r)
	}
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	if rec := opened(t, as(robin, mux), meetup); rec.Code != 204 {
		t.Errorf("robin opens the page: %d %s", rec.Code, rec.Body)
	}
	if v := inviteView(t, as(robin, mux), meetup); slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Opened != "" }) {
		t.Errorf("a guest reads who opened")
	}
	if r := rowOf(inviteView(t, jordan, meetup), robin); r == nil || r.Opened == "" {
		t.Errorf("robin opened the page, not noted: %+v", r)
	}
	var ext ExtView
	json.Unmarshal(rec.Body.Bytes(), &ext)
	if len(ext.Family) != 2 || ext.Family[0].Key != coach || ext.Family[1].Key != kit {
		t.Errorf("pat's family = %+v", ext.Family)
	}
	if rec := call(t, mux, "POST", "/open/ext/"+token, `{"answer":"yes","key":"`+kit+`"}`); rec.Code != 204 {
		t.Errorf("answering for kit: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, mux, "POST", "/open/ext/"+token, `{"answer":"yes","key":"`+robin+`"}`); rec.Code != 403 {
		t.Errorf("answering for a stranger: %d", rec.Code)
	}
	if cache.Model().AnswerOf(kit, meetup) != AnswerYes {
		t.Errorf("kit's answer = %q", cache.Model().AnswerOf(kit, meetup))
	}
}

func TestRemovedStayRemoved(t *testing.T) {
	mux, cache, _, _ := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Moms","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"moms"}`)
	moms := idOf(t, cache, "moms")
	rec := call(t, jordan, "POST", "/api/invite-groups", `{"id":"moms","rule":{"tags":["`+model.TagKey(carpoolID)+`"]},"auto":false}`)
	made := created(t, rec)
	if rec.Code != 200 || viaGroup(cache, moms, made) != 2 {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	dropped := "daniel.park@heliosschool.org"
	if rec := act(t, jordan, "moms", "uninvite", `{"email":"`+dropped+`"}`); rec.Code != 204 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if g := cache.Model().GroupOf(moms, made); !slices.Contains(g.Removed, dropped) {
		t.Errorf("the group does not remember the removal: %+v", g)
	}
	fillNow(t)
	fillNow(t)
	if cache.Model().InviteOf(moms, dropped) != nil {
		t.Errorf("the group put %s back", dropped)
	}
	if rec := call(t, jordan, "POST", "/api/invite-groups/"+made+"/edit", `{"auto":true}`); rec.Code != 204 {
		t.Errorf("auto on: %d %s", rec.Code, rec.Body)
	}
	fillNow(t)
	if cache.Model().InviteOf(moms, dropped) != nil {
		t.Errorf("auto-invite put %s back", dropped)
	}
	act(t, jordan, "moms", "invite", `{"people":[{"email":"`+dropped+`"}]}`)
	if cache.Model().InviteOf(moms, dropped) == nil {
		t.Errorf("adding %s by hand did not take", dropped)
	}
}

func TestInvitationIsPersonal(t *testing.T) {
	mux, cache, _, _ := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Moms","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"moms"}`)
	moms := idOf(t, cache, "moms")
	act(t, jordan, "moms", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "moms", "send", `{}`)
	m := cache.Model()
	on := func(email string) bool {
		return slices.ContainsFunc(m.eventsFor(testDirectory, email, nil), func(e *Event) bool { return e.ID == moms })
	}
	if !on(robin) || on(sam) || on(ella) {
		t.Errorf("robin's invitation on the calendars: robin %v, sam %v, ella %v", on(robin), on(sam), on(ella))
	}
	if v := inviteView(t, as(sam, mux), "moms"); len(v.Mine) != 0 {
		t.Errorf("sam asked about his mother's invitation: %+v", v.Mine)
	}
	call(t, jordan, "POST", "/api/events", `{"title":"Kids","start":"2026-10-11 15:00","tags":[],"sharing":"Link","address":"kids"}`)
	kidsID := idOf(t, cache, "kids")
	act(t, jordan, "kids", "invite", `{"people":[{"email":"`+sam+`"}]}`)
	act(t, jordan, "kids", "send", `{}`)
	m = cache.Model()
	kids := func(email string) bool {
		return slices.ContainsFunc(m.eventsFor(testDirectory, email, nil), func(e *Event) bool { return e.ID == kidsID })
	}
	if !kids(sam) || !kids(robin) || kids(ella) {
		t.Errorf("sam's invitation on the calendars: sam %v, robin %v, ella %v", kids(sam), kids(robin), kids(ella))
	}
	if v := inviteView(t, as(robin, mux), "kids"); len(v.Mine) != 1 || v.Mine[0].Email != sam || !v.Mine[0].Mine {
		t.Errorf("robin's ask for sam's invitation: %+v", v.Mine)
	}
	for _, key := range testDirectory.FamilyKeysOf(sam) {
		family := testDirectory.Families[key]
		family.AdultEmails = slices.DeleteFunc(slices.Clone(family.AdultEmails), func(e string) bool { return e == robin })
		testDirectory.Families[key] = family
	}
	if kids(robin) || m.Invited(testDirectory, robin, kidsID) {
		t.Errorf("a parent the directory no longer lists still sees sam's invitation")
	}
}

func TestGuestsInvite(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	if rec := act(t, as(mia, mux), "meetup", "invite", `{"people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`); rec.Code != 403 {
		t.Errorf("an outsider inviting: %d", rec.Code)
	}
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 2)
	listed := len(cache.Model().Invites[meetup])
	rec := act(t, as(robin, mux), "meetup", "invite", `{"people":[{"email":"`+mia+`"}]}`)
	if rec.Code != 204 || len(cache.Model().Invites[meetup]) != listed+1 {
		t.Fatalf("robin inviting mia: %d %s", rec.Code, rec.Body)
	}
	inv := cache.Model().InviteOf(meetup, mia)
	if inv == nil || inv.Via != ViaInvited || inv.AddedBy != robin || inv.Requested == "" || inv.RequestedBy != robin {
		t.Errorf("mia's row = %+v", inv)
	}
	waitFor(kept, 3)
	if msgs := mailTo(kept, mia); len(msgs) != 1 || !strings.Contains(msgs[0].HTML, "Robin Whitfield sent Mia an invitation for") {
		t.Errorf("mia's invite: %+v", msgs)
	}
	if r := rowOf(inviteView(t, jordan, meetup), mia); r == nil || r.InvitedBy != "Robin Whitfield" {
		t.Errorf("the host's row for mia: %+v", r)
	}
	if rec := call(t, as(robin, mux), "POST", "/api/invite-groups", `{"id":"meetup","rule":{"tags":["`+model.TagKey(carpoolID)+`"]}}`); rec.Code != 403 {
		t.Errorf("a guest adding a group: %d", rec.Code)
	}
}

func TestGuestInvitesBeforeTheHosts(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Parade","start":"2026-10-30 08:15","tags":[`+quoted(t, "Jays")+`],"sharing":"Public","address":"parade"}`)
	parade := idOf(t, cache, "parade")
	if v := inviteView(t, as(robin, mux), "parade"); !v.MayInvite || v.Sent != "" {
		t.Errorf("robin's view before any send: invite %v sent %q", v.MayInvite, v.Sent)
	}
	listed := len(cache.Model().Invites[parade])
	rec := act(t, as(robin, mux), "parade", "invite", `{"people":[{"email":"`+mia+`"}]}`)
	if rec.Code != 204 || len(cache.Model().Invites[parade]) != listed+1 {
		t.Fatalf("robin inviting mia: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().InviteOf(parade, mia); inv == nil || inv.Via != ViaInvited || inv.Requested == "" {
		t.Errorf("mia's row = %+v", inv)
	}
	eventually(t, "mia's invitation goes and is stamped sent", func() bool {
		inv := cache.Model().InviteOf(parade, mia)
		return inv != nil && inv.Sent != ""
	})
	if inv := cache.Model().Invitations[parade]; inv == nil || inv.Sent == "" {
		t.Errorf("the invitation after robin's send = %+v", inv)
	}
	waitFor(kept, 2)
	if msgs := mailTo(kept, mia); len(msgs) != 1 {
		t.Errorf("mia's invite: %d messages", len(msgs))
	}
}

func TestFamilyAnswersAnOpenEvent(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	call(t, as(mia, mux), "POST", "/api/events", `{"title":"Parade","start":"2026-10-30 08:15","tags":[`+quoted(t, "Jays")+`],"sharing":"Public","address":"parade"}`)
	parade := idOf(t, cache, "parade")
	robinH := as(robin, mux)
	v := inviteView(t, robinH, "parade")
	keys := []string{}
	for _, r := range v.Mine {
		keys = append(keys, r.Key)
	}
	if len(keys) < 2 || keys[0] != robin || !slices.Contains(keys, sam) || slices.Contains(keys, ella) || slices.ContainsFunc(v.Mine, func(r GuestRow) bool { return r.Invited || !r.Mine }) {
		t.Errorf("robin's family on an open event = %v", keys)
	}
	if rec := call(t, as(mia, mux), "POST", "/api/events", `{"title":"Parent coffee","start":"2026-10-29 08:30","tags":[`+quoted(t, "Community, Parents, Jays")+`],"sharing":"Public","address":"coffee"}`); rec.Code != 200 {
		t.Fatalf("adding the coffee: %d %s", rec.Code, rec.Body)
	}
	coffee := inviteView(t, robinH, "coffee")
	if len(coffee.Mine) < 2 || coffee.Mine[0].Key != robin || !slices.ContainsFunc(coffee.Mine, func(r GuestRow) bool { return r.Key == host }) || slices.ContainsFunc(coffee.Mine, func(r GuestRow) bool { return r.Key == sam || r.Key == ella }) {
		t.Errorf("robin's family on a parents' event = %+v", coffee.Mine)
	}
	call(t, as(mia, mux), "POST", "/api/events", `{"title":"Staff meeting","start":"2026-10-28 15:30","tags":[`+quoted(t, "Staff")+`],"sharing":"Public","address":"staffmeeting"}`)
	if v := inviteView(t, robinH, "staffmeeting"); len(v.Mine) != 0 {
		t.Errorf("robin's family on a staff event = %+v", v.Mine)
	}
	if rec := act(t, robinH, "parade", "answer-for", `{"email":"`+sam+`","answer":"yes"}`); rec.Code != 204 {
		t.Fatalf("robin answering for sam: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().AnswerOf(sam, parade) != AnswerYes {
		t.Errorf("sam's answer = %q", cache.Model().AnswerOf(sam, parade))
	}
	after := inviteView(t, robinH, "parade")
	if i := slices.IndexFunc(after.Mine, func(r GuestRow) bool { return r.Key == sam }); i < 0 || after.Mine[i].Answer != AnswerYes {
		t.Errorf("robin's rows after = %+v", after.Mine)
	}
	call(t, as(mia, mux), "POST", "/api/events", `{"title":"Party","start":"2026-10-31 15:00","tags":[],"sharing":"Invite Only","address":"party"}`)
	act(t, as(mia, mux), "party", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, as(mia, mux), "party", "send", `{}`)
	if v := inviteView(t, robinH, "party"); len(v.Mine) != 1 || v.Mine[0].Key != robin {
		t.Errorf("robin's family on an invite-only event = %+v", v.Mine)
	}
}

func TestGuestsWithoutAnInvitation(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan, robinH := as(host, mux), as(robin, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, robinH, "meetup", "answer", `{"answer":"yes"}`)
	if rec := act(t, robinH, "meetup", "bring-guest", `{"name":"Pat"}`); rec.Code != 204 {
		t.Fatalf("robin bringing a guest from the link: %d %s", rec.Code, rec.Body)
	}
	guests := slices.DeleteFunc(slices.Clone(cache.Model().Invites[meetup]), func(inv Invite) bool { return inv.GuestOf != robin })
	if len(guests) != 1 || guests[0].Name != "Pat" {
		t.Errorf("robin's guests = %+v", guests)
	}
	if rec := act(t, robinH, "meetup", "bring-guest", `{"name":"Lee","of":"`+mia+`"}`); rec.Code != 403 {
		t.Errorf("robin bringing a guest for someone outside the household: %d", rec.Code)
	}
	act(t, jordan, "meetup", "settings", `{"guests":false}`)
	if rec := act(t, robinH, "meetup", "bring-guest", `{"name":"Kim"}`); rec.Code != 403 {
		t.Errorf("robin bringing a guest when the hosts said no: %d", rec.Code)
	}
	call(t, jordan, "POST", "/api/events", `{"title":"Party","start":"2026-10-11 15:00","tags":[],"sharing":"Invite Only","address":"party"}`)
	act(t, jordan, "party", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "party", "send", `{}`)
	if rec := act(t, robinH, "party", "bring-guest", `{"name":"Lee","of":"`+sam+`"}`); rec.Code != 403 || !strings.Contains(rec.Body.String(), "on the list") {
		t.Errorf("a guest for someone off an invite-only list: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, robinH, "party", "bring-guest", `{"name":"Lee"}`); rec.Code != 204 {
		t.Errorf("robin, on the list, bringing a guest: %d %s", rec.Code, rec.Body)
	}
}

func TestPermissions(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan, robinH := as(host, mux), as(robin, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 1)
	if v := inviteView(t, robinH, "meetup"); !v.MayInvite || !v.Guests || v.ListPrivate || v.Coming == nil {
		t.Errorf("robin's view to start: invite %v guests %v private %v coming %v", v.MayInvite, v.Guests, v.ListPrivate, v.Coming)
	}
	if rec := act(t, jordan, "meetup", "settings", `{"guests":false,"publicList":false}`); rec.Code != 204 {
		t.Fatalf("closing: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations[meetup]; inv.Guests || inv.PublicList == nil || *inv.PublicList {
		t.Errorf("the settings after closing = %+v", inv)
	}
	if v := inviteView(t, robinH, "meetup"); v.MayInvite || v.Guests || !v.ListPrivate || v.Coming != nil {
		t.Errorf("robin's view closed: invite %v guests %v private %v coming %v", v.MayInvite, v.Guests, v.ListPrivate, v.Coming)
	}
	if rec := act(t, robinH, "meetup", "invite", `{"people":[{"email":"`+mia+`"}]}`); rec.Code != 403 {
		t.Errorf("robin inviting when closed: %d", rec.Code)
	}
	if rec := act(t, robinH, "meetup", "bring-guest", `{"name":"Pat"}`); rec.Code != 403 {
		t.Errorf("robin bringing a guest when closed: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+mia+`"}]}`); rec.Code != 204 {
		t.Errorf("the host inviting when closed: %d %s", rec.Code, rec.Body)
	}
	if v := inviteView(t, jordan, "meetup"); !v.Host || v.List == nil {
		t.Errorf("the host's view closed: %+v", v)
	}
}

func TestTeamStart(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	teamA := "tev0000000101"
	bookFair := []Linked{{Source: SourceTeam, ID: "e1", EventID: teamA, Title: "Book Fair", Start: "2026-11-20 08:00", End: "2026-11-20 15:00", Path: "/activities/e1"}}
	miaH := as(mia, mux)
	if rec := act(t, as(robin, mux), teamA, "start", ""); rec.Code != 403 {
		t.Errorf("a volunteer starting: %d", rec.Code)
	}
	rec := act(t, miaH, teamA, "start", "")
	if rec.Code != 204 || len(cache.Model().Groups[teamA]) != 1 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	g := cache.Model().GroupOf(teamA, cache.Model().Groups[teamA][0].ID)
	if g == nil || !g.Auto || strings.Join(g.Rule.Tags, ",") != "activity:e1" || cache.Model().Invitations[teamA] == nil {
		t.Errorf("team group = %+v", g)
	}
	if cache.Model().AnswerOf(mia, teamA) != AnswerYes {
		t.Errorf("the chair's answer = %q", cache.Model().AnswerOf(mia, teamA))
	}
	if sent, _, ok := cache.LinkedRSVPs(bookFair, SourceTeam, "e1"); !ok || sent {
		t.Errorf("rsvps before sending: ok %v sent %v", ok, sent)
	}
	v := inviteView(t, miaH, teamA)
	if !v.Host || !v.Linked || v.Party || v.Original == nil || len(v.Hosts) != 1 || v.Hosts[0].Email != mia {
		t.Errorf("chair's view = host %v linked %v party %v original %v hosts %+v", v.Host, v.Linked, v.Party, v.Original, v.Hosts)
	}
}

func TestNotifyHost(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 2)
	if inviteView(t, jordan, meetup).NotifyMe {
		t.Errorf("notify on to start")
	}
	if rec := act(t, jordan, "meetup", "settings", `{"notifyMe":true}`); rec.Code != 204 {
		t.Fatalf("notify me: %d %s", rec.Code, rec.Body)
	}
	if !inviteView(t, jordan, meetup).NotifyMe || !slices.Contains(cache.Model().Invitations[meetup].Notify, host) {
		t.Errorf("notify not kept: %+v", cache.Model().Invitations[meetup].Notify)
	}
	before := len(mailTo(kept, host))
	act(t, as(robin, mux), "meetup", "answer", `{"answer":"yes"}`)
	waitFor(kept, len(kept.Messages())+1)
	notes := mailTo(kept, host)
	if len(notes) != before+1 || notes[len(notes)-1].Subject != "[Meetup] Robin Whitfield said Yes" || !strings.Contains(notes[len(notes)-1].Text, "1 yes, 0 maybe, 0 no, 0 still to answer") {
		t.Errorf("the host's note: %+v", notes)
	}
	before = len(mailTo(kept, host))
	act(t, jordan, "meetup", "answer-for", `{"email":"`+robin+`","answer":"maybe"}`)
	act(t, jordan, "meetup", "settings", `{"notifyMe":false}`)
	act(t, as(robin, mux), "meetup", "answer", `{"answer":"no"}`)
	if len(mailTo(kept, host)) != before {
		t.Errorf("notes after: %d, before %d", len(mailTo(kept, host)), before)
	}
}

func TestMailLeavesItsRecord(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	row := func(tab, key, value string) store.Row {
		for _, r := range sheetTables(t)[tab] {
			if r[key] == value {
				return r
			}
		}
		return nil
	}
	eventually(t, "the admins are told of the new event and it is recorded", func() bool {
		told := row(EventsTab, "Event ID", meetup)["Admins Told"]
		return told != "" && !strings.HasPrefix(told, owed)
	})
	act(t, jordan, "meetup", "settings", `{"hosts":["`+mia+`"],"notifyMe":true}`)
	eventually(t, "the new co-host is told and nobody is left to tell", func() bool {
		return len(mailTo(kept, mia)) == 1 && row(InvitationsTab, "Event ID", meetup)["Hosts To Tell"] == ""
	})
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "message", `{"subject":"Bring snacks","message":"Anything nut-free.","to":["none"]}`)
	eventually(t, "the message reaches everyone it was for and says so", func() bool {
		messages := cache.Model().Messages
		return len(messages) == 1 && slices.Equal(messages[0].SentTo, []string{robin}) && len(messages[0].pending()) == 0
	})
	act(t, as(robin, mux), "meetup", "answer", `{"answer":"yes"}`)
	eventually(t, "the hosts who asked are told of the answer and it is recorded", func() bool {
		ans := cache.Model().Answered[robin][meetup]
		return ans.hostsTold != "" && ans.hostsTold != owed && ans.inviteMail != owed
	})
	if notes := mailTo(kept, host); len(notes) == 0 || notes[len(notes)-1].Subject != "[Meetup] Robin Whitfield said Yes" {
		t.Errorf("the host's note: %+v", notes)
	}
}

func TestStepDown(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan, miaH := as(host, mux), as(mia, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "settings", `{"hosts":["`+mia+`"],"notifyMe":true}`)
	if rec := act(t, as(robin, mux), "meetup", "step-down", `{}`); rec.Code != 403 {
		t.Errorf("someone not hosting stepping down: %d", rec.Code)
	}
	if rec := act(t, miaH, "meetup", "step-down", `{}`); rec.Code != 204 {
		t.Fatalf("the co-host stepping down: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations[meetup]; len(inv.Hosts) != 0 || inv.SteppedDown != "" {
		t.Errorf("after the co-host = %+v", inv)
	}
	if v := inviteView(t, miaH, meetup); v.Host {
		t.Errorf("the co-host still hosts")
	}
	if rec := act(t, jordan, "meetup", "step-down", `{}`); rec.Code != 204 {
		t.Fatalf("the poster stepping down: %d %s", rec.Code, rec.Body)
	}
	inv := cache.Model().Invitations[meetup]
	if inv.SteppedDown != host || len(inv.Notify) != 0 {
		t.Errorf("after the poster = %+v", inv)
	}
	e := cache.Model().Event("meetup")
	if e == nil || !e.PosterLeft || e.AddedBy != host {
		t.Fatalf("the event after = %+v", e)
	}
	if v := inviteView(t, jordan, meetup); v.Host || len(v.Hosts) != 0 {
		t.Errorf("the poster still hosts: host %v hosts %+v", v.Host, v.Hosts)
	}
	if rec := act(t, jordan, "meetup", "edit", `{"title":"Meetup!","start":"2026-10-10 15:00","tags":[],"sharing":"Link"}`); rec.Code != 403 {
		t.Errorf("the poster editing after stepping down: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "step-down", `{}`); rec.Code != 403 {
		t.Errorf("stepping down twice: %d", rec.Code)
	}
}

func TestCascadesReachTheSheet(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	act(t, jordan, "meetup", "answer-for", `{"email":"`+coach+`","answer":"yes"}`)
	if rec := act(t, jordan, "meetup", "change-email", `{"email":"`+coach+`","to":"coach.lee@example.org"}`); rec.Code != 204 {
		t.Fatalf("change address: %d %s", rec.Code, rec.Body)
	}
	answers := func() []string {
		out := []string{}
		for _, row := range sheetTables(t)[RSVPsTab] {
			if row["Event ID"] == meetup {
				out = append(out, row["Email"]+"="+row["Answer"])
			}
		}
		slices.Sort(out)
		return out
	}
	if got := strings.Join(answers(), ","); got != "coach.lee@example.org=yes,"+host+"=yes" {
		t.Errorf("answers in the sheet after the address change: %s", got)
	}
	if !slices.Contains(changeLog(t), host+"|set|RSVPs|Event ID="+meetup+"; Email=coach.lee@example.org|Email|"+coach) {
		t.Errorf("the carried answer is not logged: %v", changeLog(t))
	}
	if rec := act(t, jordan, "meetup", "delete-invitation", ""); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	if m.Event("meetup") != nil || m.Invitations[meetup] != nil || len(m.Invites[meetup]) != 0 || m.AnswerOf(host, meetup) != "" || m.AnswerOf("coach.lee@example.org", meetup) != "" {
		t.Errorf("memory after the delete: event %v, invitation %v, invites %d", m.Event("meetup"), m.Invitations[meetup], len(m.Invites[meetup]))
	}
	for _, tab := range []string{EventsTab, OverridesTab, InvitationsTab, InvitesTab, RSVPsTab} {
		for _, row := range sheetTables(t)[tab] {
			if row["Event ID"] == meetup {
				t.Errorf("%s kept %v", tab, row)
			}
		}
	}
	deleted := map[string]bool{}
	for _, line := range changeLog(t) {
		if parts := strings.Split(line, "|"); parts[1] == "delete" && strings.Contains(parts[3], "Event ID="+meetup) {
			deleted[parts[2]+" "+parts[3]] = true
		}
	}
	key := "Event ID=" + meetup
	for _, want := range []string{"Events " + key, "Overrides " + key, "Invitations " + key, "Invites " + key + "; Email=" + robin, "Invites " + key + "; Email=coach.lee@example.org", "RSVPs " + key + "; Email=" + host, "RSVPs " + key + "; Email=coach.lee@example.org"} {
		if !deleted[want] {
			t.Errorf("the change log lacks the delete of %s: %v", want, deleted)
		}
	}
}

func TestBouncesAreAppendOnly(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	stamp, sig := mail.SignMailgun(replySecret, "token-bounce", time.Now())
	body, _ := json.Marshal(map[string]any{
		"signature":  map[string]string{"timestamp": stamp, "token": "token-bounce", "signature": sig},
		"event-data": map[string]any{"event": "failed", "severity": "permanent", "recipient": coach, "delivery-status": map[string]any{"description": "No such user here"}},
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/hooks/events", strings.NewReader(string(body))))
	if rec.Code != 200 || cache.Model().Bounced[coach].Reason != "No such user here" {
		t.Fatalf("bounce: %d %s", rec.Code, rec.Body)
	}
	if rows := sheetTables(t)[BouncesTab]; len(rows) != 1 || rows[0]["Email"] != coach {
		t.Errorf("the sheet's bounces: %v", rows)
	}
	if log := changeLog(t); len(log) != 0 {
		t.Errorf("a bounce was logged: %v", log)
	}
}

func TestCategoryOrder(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	admin := as("dana.hawkins@heliosschool.org", mux)
	listed := func() []map[string]any {
		tags := []map[string]any{}
		for _, tag := range cache.Model().Tags {
			if !tag.BuiltIn {
				tags = append(tags, map[string]any{"id": tag.ID, "name": tag.Name, "description": tag.Description, "group": tag.Group, "default": tag.Default, "image": tag.Image})
			}
		}
		return tags
	}
	tags := listed()
	tags[0], tags[1] = tags[1], tags[0]
	tags = append(tags, map[string]any{"name": "Fundraiser", "description": "Raising money.", "group": "Community", "default": false})
	raw, _ := json.Marshal(map[string]any{"tags": tags})
	if rec := call(t, admin, "POST", settingsPath("tags"), string(raw)); rec.Code != 204 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	names := []string{}
	for _, tag := range cache.Model().Tags {
		names = append(names, tag.Name)
	}
	fundraiser := cache.Model().Tags[15]
	if len(names) != 21 || names[0] != "Conference" || names[1] != "Schedule" || fundraiser.Name != "Fundraiser" || fundraiser.Default || names[16] != "Celebrate" {
		t.Errorf("categories after the save: %v", names)
	}
	if key, ok := id.Parse(fundraiser.ID); !ok || key != fundraiser.ID || strings.HasPrefix(key, "tag0") {
		t.Errorf("a new category's id: %q", fundraiser.ID)
	}
	orders := []string{}
	for _, row := range sheetTables(t)[TagsTab] {
		if !BuiltInTag(row["Tag ID"]) {
			orders = append(orders, row[store.OrderColumn])
		}
	}
	if len(orders) != 16 || slices.Contains(orders, "") || !slices.IsSorted(append([]string{orders[1], orders[0]}, orders[2:]...)) {
		t.Errorf("the sheet's order keys: %v", orders)
	}
	for _, line := range changeLog(t) {
		if strings.Contains(line, "|Default|") || strings.Contains(line, "|Description|") {
			t.Errorf("an unchanged cell was logged: %s", line)
		}
	}
	unchanged := len(changeLog(t))
	raw, _ = json.Marshal(map[string]any{"tags": listed()})
	if rec := call(t, admin, "POST", settingsPath("tags"), string(raw)); rec.Code != 204 || len(changeLog(t)) != unchanged {
		t.Errorf("saving again: %d %s, log %d then %d", rec.Code, rec.Body, unchanged, len(changeLog(t)))
	}
	going := append(listed(), map[string]any{"id": TagGoing, "name": "Going", "description": "Mine.", "group": "Other", "default": true})
	raw, _ = json.Marshal(map[string]any{"tags": going})
	if rec := call(t, admin, "POST", settingsPath("tags"), string(raw)); rec.Code != 400 {
		t.Errorf("a built-in changed from Admin Tools: %d %s", rec.Code, rec.Body)
	}
	twin := append(listed(), map[string]any{"name": "Going", "description": "Again.", "group": "Other"})
	raw, _ = json.Marshal(map[string]any{"tags": twin})
	if rec := call(t, admin, "POST", settingsPath("tags"), string(raw)); rec.Code != 400 {
		t.Errorf("a new category named as a built-in: %d %s", rec.Code, rec.Body)
	}
}

func TestHideHosts(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	if v := inviteView(t, as(robin, mux), meetup); v.HostsHidden || len(v.Hosts) != 1 {
		t.Errorf("hosts before hiding: hidden %v hosts %+v", v.HostsHidden, v.Hosts)
	}
	if rec := act(t, as(robin, mux), "meetup", "settings", `{"hideHosts":true}`); rec.Code != 403 {
		t.Errorf("a guest hiding the hosts: %d", rec.Code)
	}
	if rec := act(t, jordan, "meetup", "settings", `{"hideHosts":true}`); rec.Code != 204 {
		t.Fatalf("hiding: %d %s", rec.Code, rec.Body)
	}
	if !cache.Model().Invitations[meetup].HideHosts {
		t.Errorf("not kept")
	}
	if v := inviteView(t, as(robin, mux), meetup); !v.HostsHidden || len(v.Hosts) != 0 {
		t.Errorf("a guest's view when hidden: hidden %v hosts %+v", v.HostsHidden, v.Hosts)
	}
	if v := inviteView(t, jordan, meetup); !v.HostsHidden || len(v.Hosts) != 1 {
		t.Errorf("the host's view when hidden: hidden %v hosts %+v", v.HostsHidden, v.Hosts)
	}
	act(t, jordan, "meetup", "settings", `{"hideHosts":false}`)
	if v := inviteView(t, as(robin, mux), meetup); v.HostsHidden || len(v.Hosts) != 1 {
		t.Errorf("shown again: hidden %v hosts %+v", v.HostsHidden, v.Hosts)
	}
}

func TestAdminActsAsHost(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan, dana := as(host, mux), as("dana.hawkins@heliosschool.org", mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	v := inviteView(t, dana, meetup)
	if !v.Host || v.Poster != host || slices.ContainsFunc(v.Hosts, func(p Person) bool { return p.Email == "dana.hawkins@heliosschool.org" }) {
		t.Errorf("the admin's view: host %v poster %q hosts %v", v.Host, v.Poster, v.Hosts)
	}
	if rec := act(t, dana, "meetup", "settings", `{"hideHosts":true}`); rec.Code != 204 {
		t.Errorf("the admin hiding the hosts: %d %s", rec.Code, rec.Body)
	}
	if rec := act(t, as(robin, mux), "meetup", "step-down", `{"email":"`+host+`"}`); rec.Code != 403 {
		t.Errorf("a guest stepping the poster down: %d", rec.Code)
	}
	if rec := act(t, dana, "meetup", "step-down", `{"email":"`+host+`"}`); rec.Code != 204 {
		t.Fatalf("the admin stepping the poster down: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("meetup"); !e.PosterLeft {
		t.Errorf("the poster still hosts")
	}
	if v := inviteView(t, jordan, meetup); v.Host {
		t.Errorf("the poster after: host %v", v.Host)
	}
	if v := inviteView(t, dana, meetup); !v.Host || v.Poster != "" {
		t.Errorf("the admin after: host %v poster %q", v.Host, v.Poster)
	}
}

func TestToolbarRSVPs(t *testing.T) {
	mux, cache, kept := invitesApp(t)
	jordan, robinH := as(host, mux), as(robin, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "invite", `{"people":[{"email":"`+robin+`"}]}`)
	act(t, jordan, "meetup", "send", `{}`)
	waitFor(kept, 1)
	waiting := func(h http.Handler) []RSVP {
		var view struct {
			Waiting []RSVP `json:"waiting"`
		}
		json.NewDecoder(call(t, h, "GET", "/api/apps/rsvp", "").Body).Decode(&view)
		return view.Waiting
	}
	if w := waiting(robinH); len(w) != 1 || w[0].Title != "Meetup" || w[0].Path != "/e/meetup" {
		t.Fatalf("robin owes: %+v", w)
	}
	if w := waiting(jordan); slices.ContainsFunc(w, func(r RSVP) bool { return r.Path == "/e/meetup" }) {
		t.Errorf("the host owes: %+v", w)
	}
	act(t, robinH, "meetup", "answer", `{"answer":"yes"}`)
	if w := waiting(robinH); len(w) != 0 {
		t.Errorf("robin after answering owes: %+v", w)
	}
}

func TestTicketGuestsAndMovedAddresses(t *testing.T) {
	mux, cache, kept, sources := invitesAppWith(t)
	const (
		alum   = "ella.graduated@heliosschool.org"
		home   = "ella.w@gmail.com"
		cousin = "kit@example.org"
	)
	testAttendees = []Attendee{{Email: alum, Status: "ticket"}, {Email: cousin, Name: "Kit Whitfield", Status: "free"}, {Email: "hopeful@example.org", Name: "Hopeful", Status: "waitlist"}}
	sources.list("party:"+partyA, mia, robin, sam)
	miaH := as(mia, mux)
	if rec := act(t, miaH, partyA, "start", ""); rec.Code != 204 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	model := cache.Model()
	for _, email := range []string{alum, cousin} {
		if inv := model.InviteOf(partyA, email); inv == nil || inv.Token == "" {
			t.Errorf("ticket guest %s = %+v", email, inv)
		}
	}
	if inv := model.InviteOf(partyA, cousin); inv == nil || inv.Name != "Kit Whitfield" {
		t.Errorf("the cousin's name = %+v", inv)
	}
	if model.InviteOf(partyA, "hopeful@example.org") != nil {
		t.Errorf("someone only on the waitlist is on the list")
	}
	act(t, miaH, partyA, "send", `{}`)
	waitFor(kept, 5)
	if len(mailTo(kept, alum)) != 1 || len(mailTo(kept, cousin)) != 1 {
		t.Fatalf("ticket guests were not sent the invitation: alum %d, cousin %d", len(mailTo(kept, alum)), len(mailTo(kept, cousin)))
	}
	act(t, miaH, partyA, "answer-for", `{"email":"`+alum+`","answer":"maybe"}`)

	if v := inviteView(t, miaH, partyA); v.MoveEverywhere {
		t.Errorf("a host who is no admin of Celebrate may move addresses everywhere")
	}
	body := `{"email":"` + alum + `","to":"` + home + `","everywhere":true}`
	if rec := act(t, miaH, partyA, "change-email", body); rec.Code != 403 {
		t.Errorf("moved everywhere without being Celebrate's admin: %d", rec.Code)
	}
	testCelebrateAdmin = mia
	if v := inviteView(t, miaH, partyA); !v.MoveEverywhere {
		t.Errorf("Celebrate's admin is not offered the move")
	}
	if rec := act(t, miaH, partyA, "change-email", body); rec.Code != 204 {
		t.Fatalf("move everywhere: %d %s", rec.Code, rec.Body)
	}
	if len(testMoves) != 1 || testMoves[0] != [4]string{mia, alum, home, "Ella Graduated"} {
		t.Fatalf("Celebrate was asked %v", testMoves)
	}

	token := cache.Model().InviteOf(partyA, alum).Token
	testAttendees[0].Email = home
	testHooks.MoveAddress(context.Background(), access.Actor{Email: mia}, alum, home, "Ella Whitfield")
	model = cache.Model()
	moved := model.InviteOf(partyA, home)
	if moved == nil || model.InviteOf(partyA, alum) != nil || moved.Token != token || moved.Name != "Ella Whitfield" {
		t.Fatalf("after the move: %+v, old %+v", moved, model.InviteOf(partyA, alum))
	}
	if model.AnswerOf(home, partyA) != AnswerMaybe {
		t.Errorf("the answer did not follow: %q", model.AnswerOf(home, partyA))
	}
	waitFor(kept, 6)
	if len(mailTo(kept, home)) != 1 {
		t.Errorf("the invitation was not sent again to the new address: %d", len(mailTo(kept, home)))
	}
	before := len(cache.Model().Invites[partyA])
	fillNow(t)
	if n := len(cache.Model().Invites[partyA]) - before; n != 0 {
		t.Errorf("the fill added %d after the move", n)
	}
}

func TestCohostsRunTheEvent(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan, miaH, robinH := as(host, mux), as(mia, mux), as(robin, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	if rec := act(t, robinH, "meetup", "answer", `{"answer":"no"}`); rec.Code != 204 {
		t.Fatalf("robin says no: %d %s", rec.Code, rec.Body)
	}
	act(t, jordan, "meetup", "settings", `{"hosts":["`+mia+`","`+robin+`"]}`)
	if cache.Model().AnswerOf(mia, meetup) != AnswerYes || cache.Model().AnswerOf(robin, meetup) != AnswerNo || cache.Model().AnswerOf(host, meetup) != AnswerYes {
		t.Errorf("the hosts' answers: mia %q robin %q poster %q", cache.Model().AnswerOf(mia, meetup), cache.Model().AnswerOf(robin, meetup), cache.Model().AnswerOf(host, meetup))
	}
	if rec := act(t, miaH, "meetup", "answer", `{"answer":"no"}`); rec.Code != 204 || cache.Model().AnswerOf(mia, meetup) != AnswerNo {
		t.Errorf("a co-host saying no: %d %q", rec.Code, cache.Model().AnswerOf(mia, meetup))
	}
	if rec := act(t, as(sam, mux), "meetup", "edit", `{"title":"Meetup?","start":"2026-10-10 15:00","tags":[],"sharing":"Link"}`); rec.Code != 403 {
		t.Errorf("someone not hosting editing the event: %d", rec.Code)
	}
	if rec := act(t, miaH, "meetup", "edit", `{"title":"Meetup!","start":"2026-10-10 15:00","tags":[],"sharing":"Link"}`); rec.Code != 204 {
		t.Fatalf("a co-host editing the event: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("meetup"); e == nil || e.Title != "Meetup!" {
		t.Errorf("the event after the co-host's edit = %+v", e)
	}
	if rec := act(t, as(sam, mux), "meetup", "step-down", `{"email":"`+robin+`"}`); rec.Code != 403 {
		t.Errorf("someone not hosting stepping a co-host down: %d", rec.Code)
	}
	if rec := act(t, miaH, "meetup", "step-down", `{"email":"`+robin+`"}`); rec.Code != 204 {
		t.Fatalf("a co-host stepping another down: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations[meetup]; slices.Contains(inv.Hosts, robin) || !slices.Contains(inv.Hosts, mia) {
		t.Errorf("hosts after the co-host was stepped down = %v", inv.Hosts)
	}
	if rec := act(t, robinH, "meetup", "edit", `{"title":"Meetup?","start":"2026-10-10 15:00","tags":[],"sharing":"Link"}`); rec.Code != 403 {
		t.Errorf("a stepped-down co-host editing the event: %d", rec.Code)
	}
	if rec := act(t, miaH, "meetup", "step-down", `{"email":"`+host+`"}`); rec.Code != 204 {
		t.Fatalf("a co-host stepping the poster down: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("meetup"); e == nil || !e.PosterLeft {
		t.Errorf("the poster still hosts: %+v", e)
	}
	if rec := act(t, jordan, "meetup", "step-down", `{"email":"`+mia+`"}`); rec.Code != 403 {
		t.Errorf("a poster who stepped down stepping a co-host down: %d", rec.Code)
	}
	if rec := act(t, miaH, "meetup", "cancel", `{}`); rec.Code != 204 {
		t.Fatalf("a co-host cancelling: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("meetup"); e == nil || !e.Cancelled {
		t.Errorf("not cancelled: %+v", e)
	}
	call(t, jordan, "POST", "/api/events", `{"title":"Other","start":"2026-10-11 15:00","tags":[],"sharing":"Link","address":"other"}`)
	act(t, jordan, "other", "settings", `{"hosts":["`+mia+`"]}`)
	if rec := act(t, miaH, "other", "delete-invitation", ""); rec.Code != 204 {
		t.Fatalf("a co-host deleting: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().Event("other") != nil {
		t.Errorf("the event outlived its co-host's delete")
	}
}

func TestSweepActsOnlyForAHost(t *testing.T) {
	mux, cache, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"sharing":"Link","address":"meetup"}`)
	meetup := idOf(t, cache, "meetup")
	act(t, jordan, "meetup", "settings", `{"hosts":["`+mia+`"]}`)
	if rec := call(t, jordan, "POST", "/api/invite-groups", `{"id":"meetup","rule":{"roles":["Student"],"classrooms":["Jays"]},"auto":false}`); rec.Code != 200 {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	a := newApp(testDeps)
	gone := cache.Model().Invites[meetup][0].Email
	drop := func() {
		t.Helper()
		if err := cache.Commit(context.Background(), access.System("test"), store.Delete(InvitesTab, store.Row{"Event ID": meetup, "Email": gone})); err != nil {
			t.Fatal(err)
		}
	}
	drop()
	fillNow(t)
	if cache.Model().InviteOf(meetup, gone) == nil {
		t.Fatalf("the fill did not fill the group while its maker hosts")
	}
	if a.sweptEvent(a.as(host), meetup) == nil {
		t.Errorf("the group's maker does not count as its host")
	}
	act(t, jordan, "meetup", "step-down", `{}`)
	if a.sweptEvent(a.as(host), meetup) != nil {
		t.Errorf("the group's maker counts as a host after stepping down")
	}
	drop()
	fillNow(t)
	if cache.Model().InviteOf(meetup, gone) != nil {
		t.Errorf("the fill filled the group for someone who no longer hosts")
	}
	e := a.eventFor(access.Actor{Email: mia}, meetup)
	filled, _ := a.fillOps(access.System(sweepActor), e, cache.Model().Groups[meetup][0])
	if len(filled) == 0 {
		t.Errorf("the group no longer matches, so the fill proved nothing")
	}
}
