package calendar

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/filter"
	"heliosian/internal/mail"
	"heliosian/internal/who"
)

// A guest list's people: Jordan hosts, the Whitfields - Robin, Sam and
// Ella - are a household, Mia is another parent, and a coach from outside
// the directory comes as a guest.
const (
	host   = "jordan.whitfield@heliosschool.org"
	robin  = "robin.whitfield@heliosschool.org"
	sam    = "sam.whitfield@heliosschool.org"
	ella   = "ella.whitfield@heliosschool.org"
	mia    = "mia.torres@heliosschool.org"
	coach  = "coach@example.org"
	partyA = "celebrate/p1"
)

func invitesApp(t *testing.T) (http.Handler, *Cache, *keptMail, fakeStore) {
	h, c, k, s, _ := invitesAppWith(t)
	return h, c, k, s
}

func invitesAppWith(t *testing.T) (http.Handler, *Cache, *keptMail, fakeStore, *sampleSources) {
	t.Helper()
	t.Chdir("../..")
	dir := &data.Dir{Root: "sampledata"}
	households := map[string][]string{robin: {sam, ella}, sam: {robin, ella}, ella: {robin, sam}}
	parents := map[string][]string{sam: {robin}, ella: {robin}}
	cache, err := NewCache(dir, func() Roster { return Roster{Classrooms: roster.Classrooms, Households: households, Parents: parents} }, nil, func(string) bool { return false }, directQueue{})
	if err != nil {
		t.Fatal(err)
	}
	samP := Person{Email: sam, Name: "Sam Whitfield", IsStudent: true, Grade: "Grade 3", Classroom: "Jays"}
	ellaP := Person{Email: ella, Name: "Ella Whitfield", IsStudent: true, Grade: "Grade 6", Classroom: "Ospreys"}
	d := fakeDirectory{
		people: map[string]Person{
			host:  {Email: host, Name: "Jordan Whitfield", IsParent: true},
			robin: {Email: robin, Name: "Robin Whitfield", IsParent: true},
			sam:   samP,
			ella:  ellaP,
			mia:   {Email: mia, Name: "Mia Torres", IsParent: true},
		},
		kids:  map[string][]Person{robin: {samP, ellaP}},
		lists: []List{{Key: "tag:Carpool", Name: "Carpool", Kind: "tag", People: []string{mia, robin}}, {Key: "activity:e1", Name: "Book Fair", Kind: "activity", People: []string{mia, robin, ella}}},
	}
	kept := &keptMail{}
	store := fakeStore{}
	parties := func(id string) *PartyPeople {
		if id != "p1" {
			return nil
		}
		return &PartyPeople{Hosts: []string{mia}, Attendees: []Attendee{{Email: robin, Name: "Robin Whitfield", Status: "ticket"}, {Email: sam, Name: "Sam Whitfield", Status: "ticket"}, {Name: "A cousin", Status: "waitlist"}}}
	}
	linked := func(email string) []Linked {
		return []Linked{
			{Source: SourceCelebrate, ID: "p1", Title: "Fondue Night", Start: "2026-11-14 18:00", End: "2026-11-14 21:00", Path: "/parties/p1", Availability: "available"},
			{Source: SourceTeam, ID: "e1", Title: "Book Fair", Start: "2026-11-20 08:00", End: "2026-11-20 15:00", Path: "/activities/e1", Availability: "open", Hosts: []string{mia}},
		}
	}
	mux := http.NewServeMux()
	sources := newSampleSources(t)
	Register(mux, cache, dir, directQueue{}, nil, d, func() []string { return nil }, linked, parties, sources.sources, ImageSearch{}, Mail{Sender: kept, From: "Helios When <when@example.org>", Store: store, SigningKey: replySecret, ReplyTo: replyTo, Key: replyKey})
	return mux, cache, kept, store, sources
}

// testSources is the sample directory as a rule reads it - the people, and
// for tags the host's own from the sample Tags tab.
func testSources(t *testing.T) func() filter.Sources {
	t.Helper()
	return newSampleSources(t).sources
}

// sampleSources holds the sample directory's tables, whose Tags rows a
// test may add to between calls - the way a tag grows in Who?.
type sampleSources struct {
	tables *who.Tables
	model  *who.Model
	// extra is the other apps' lists as Magic Tags - a party's, say - by
	// key, for every owner.
	extra map[string][]string
}

func (s *sampleSources) sources() filter.Sources {
	return filter.Sources{Directory: s.model, Tags: func(owner string) map[string][]string {
		tags := who.TagsOf(s.tables.Tags, s.model, owner)
		for key, people := range s.extra {
			tags[key] = people
		}
		return tags
	}}
}

func newSampleSources(t *testing.T) *sampleSources {
	t.Helper()
	dir := &data.Dir{Root: "sampledata"}
	tables, err := who.ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	model, err := who.BuildModel(tables, nil, noFiles{})
	if err != nil {
		t.Fatal(err)
	}
	return &sampleSources{tables: tables, model: model}
}

type noFiles struct{}

func (noFiles) Has(string) (bool, error) { return false, nil }
func (noFiles) Prefetch([]string) error  { return nil }

func waitFor(kept *keptMail, n int) []mail.Message {
	for i := 0; i < 100 && len(kept.all()) < n; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	return kept.all()
}

func inviteView(t *testing.T, h http.Handler, id string) InviteView {
	t.Helper()
	rec := call(t, h, "GET", "/api/calendar/invites?id="+id, "")
	if rec.Code != 200 {
		t.Fatalf("guest list: %d %s", rec.Code, rec.Body)
	}
	var v InviteView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func rowOf(v InviteView, key string) *GuestRow {
	for i := range v.List {
		if v.List[i].Key == key {
			return &v.List[i]
		}
	}
	return nil
}

// mailTo is every message sent to one address so far, in order.
func mailTo(kept *keptMail, to string) []mail.Message {
	out := []mail.Message{}
	for _, m := range kept.all() {
		if m.To[0] == to {
			out = append(out, m)
		}
	}
	return out
}

// A host shares an event as RSVP invites, builds its list from the
// directory and from outside, sends it, and reads the answers; the
// invited answer for their household, bring a guest, and are reminded.
func TestInvitationLifecycle(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	robinH := as(robin, mux)
	samH := as(sam, mux)
	miaH := as(mia, mux)
	rec := call(t, jordan, "POST", "/api/calendar/events", `{"title":"Class meetup","start":"2026-10-10 15:00","end":"2026-10-10 17:00","location":"The park","tags":["Jays"],"inviteOnly":true,"id":"meetup"}`)
	if rec.Code != 200 {
		t.Fatalf("share: %d %s", rec.Code, rec.Body)
	}
	e := cache.Model().Event("meetup")
	if e == nil || !e.InviteOnly || e.Status != StatusPrivate {
		t.Fatalf("shared event = %+v", e)
	}
	waitFor(kept, 1)
	if sent := kept.all(); len(sent) != 1 || !strings.HasPrefix(sent[0].Subject, "Private event added") {
		t.Errorf("admins' mail = %+v", sent)
	}
	// Only a host builds the list.
	if rec := call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`); rec.Code != 403 {
		t.Errorf("someone else adding: %d", rec.Code)
	}
	if rec := call(t, miaH, "GET", "/api/calendar/invites/people?id=meetup", ""); rec.Code != 403 {
		t.Errorf("someone else's picker: %d", rec.Code)
	}
	rec = call(t, jordan, "GET", "/api/calendar/invites/people?id=meetup", "")
	var picker PickerView
	json.Unmarshal(rec.Body.Bytes(), &picker)
	if rec.Code != 200 || len(picker.People) != 5 || len(picker.Lists) != 2 || len(picker.Classrooms) != 9 || len(picker.OnList) != 0 {
		t.Errorf("picker: %d people %d lists %d classrooms %d on list %d", rec.Code, len(picker.People), len(picker.Lists), len(picker.Classrooms), len(picker.OnList))
	}
	if p := picker.People[slices.IndexFunc(picker.People, func(p PickerPerson) bool { return p.Email == robin })]; strings.Join(p.Household, ",") != sam+","+ella || p.Line != "Parent to Sam (Grade 3), Ella (Grade 6)" {
		t.Errorf("robin in the picker = %+v", p)
	}
	rec = call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`","via":"family"},{"email":"`+sam+`","via":"family"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"},{"email":"not-an-address"}]}`)
	if rec.Code != 400 {
		t.Errorf("a bad address among them: %d", rec.Code)
	}
	rec = call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`","via":"family"},{"email":"`+sam+`","via":"family"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	if rec.Code != 200 || rec.Body.String() != "{\"added\":3,\"sent\":0}\n" {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations["meetup"]; inv == nil || inv.CreatedBy != host || inv.Audience != AudienceBoth || inv.GuestList != GuestListPublic || inv.Sent != "" {
		t.Errorf("invitation = %+v", inv)
	}
	if rows := cache.Model().Invites["meetup"]; len(rows) != 3 || rows[0].Email != robin || rows[0].Name != "Robin Whitfield" || rows[0].Via != "family" || rows[2].Name != "Coach Lee" || rows[2].Sent != "" {
		t.Errorf("invites = %+v", rows)
	}
	// A draft list is nobody's calendar yet.
	var view View
	rec = call(t, robinH, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	if slices.ContainsFunc(view.Events, func(e *Event) bool { return e.ID == "meetup" }) {
		t.Errorf("an unsent invite put the event on Robin's calendar")
	}
	if rec := call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","audience":"students","message":"Bring a snack to share!","hosts":["`+mia+`"]}`); rec.Code != 204 {
		t.Fatalf("settings: %d %s", rec.Code, rec.Body)
	}
	if inv := cache.Model().Invitations["meetup"]; inv.Audience != AudienceStudents || inv.Message != "Bring a snack to share!" || strings.Join(inv.Hosts, ",") != mia {
		t.Errorf("invitation after settings = %+v", inv)
	}
	// Mia hears she is a co-host; saving the same hosts again tells nobody.
	waitFor(kept, 2)
	if m := mailTo(kept, mia); len(m) != 1 || m[0].Subject != "[Class meetup] You're a co-host" || !strings.Contains(m[0].Text, "Jordan Whitfield made you a co-host") || m[0].ReplyTo[0] != host {
		t.Errorf("co-host note = %+v", m)
	}
	call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","hosts":["`+mia+`"]}`)
	time.Sleep(30 * time.Millisecond)
	if m := mailTo(kept, mia); len(m) != 1 {
		t.Errorf("co-host told twice: %d", len(m))
	}
	// Mia is a co-host now, and may add people.
	if rec := call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+ella+`","via":"search"}]}`); rec.Code != 200 {
		t.Errorf("a co-host adding: %d %s", rec.Code, rec.Body)
	}
	// Sending: one message per person - a student's to them and their
	// parents - with everyone invited in the household named; the outside
	// guest gets their own.
	rec = call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"new"}`)
	if rec.Code != 200 || rec.Body.String() != "{\"invites\":4,\"messages\":4}\n" {
		t.Fatalf("send: %d %s", rec.Code, rec.Body)
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
	if m, ok := byTo[robin]; !ok || strings.Join(m.ReplyTo, ",") != host+","+mia+","+replyAddress("meetup", robin) || strings.Join(byTo[sam].ReplyTo, ",") == strings.Join(m.ReplyTo, ",") || m.Subject != "[Class meetup] You're invited!" || !strings.Contains(m.Text, "Hosted by Jordan Whitfield and Mia Torres") || !strings.Contains(m.HTML, "/open/share/meetup.png") || !strings.Contains(m.Text, "Invited: Robin, Sam, Ella") || !strings.Contains(m.HTML, "Bring a snack to share!") || !strings.Contains(m.HTML, "https://when.heliosian.com/e/meetup") || len(m.Attachments) != 1 || !strings.Contains(string(m.Attachments[0].Content), "ATTENDEE;CN="+robin) || !strings.Contains(string(m.Attachments[0].Content), "UID:meetup@calendar.heliosian.com") || !strings.Contains(strings.ReplaceAll(string(m.Attachments[0].Content), "\r\n ", ""), "ORGANIZER;CN=Helios When:mailto:"+mailAddress(replyAddress("meetup", robin))+"\r\n") {
		t.Errorf("robin's mail = %+v\n%s", m, m.Text)
	}
	if m, ok := byTo[coach]; !ok || !strings.Contains(m.Text, "Invited: Coach") {
		t.Errorf("coach's mail = %+v", m)
	}
	if inv := cache.Model().Invitations["meetup"]; inv.Sent == "" {
		t.Errorf("invitation not marked sent")
	}
	for _, row := range cache.Model().Invites["meetup"] {
		if row.Sent == "" {
			t.Errorf("%s not marked sent", row.Email)
		}
	}
	// Nothing new to send now.
	if rec := call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"new"}`); rec.Code != 400 {
		t.Errorf("sending again: %d", rec.Code)
	}
	// The event is on the household's calendar now, and the page knows it
	// carries a guest list.
	rec = call(t, robinH, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	i := slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == "meetup" })
	if i < 0 || !view.Events[i].Invitation || !view.Events[i].Invited || slices.Contains(view.Events[i].Tags, TagGoing) {
		t.Errorf("robin's calendar after sending: %d %+v", i, view.Events)
	}
	// Upcoming lists it for Robin whatever her classrooms: the event is
	// the Jays', so it is read for a Robin whose only child is in Hawks.
	hawk := Person{Email: "hawk@x.org", Name: "Hawk", IsStudent: true, Classroom: "Hawks"}
	elsewhere := fakeDirectory{people: map[string]Person{robin: {Email: robin, Name: "Robin", IsParent: true}, "hawk@x.org": hawk}, kids: map[string][]Person{robin: {hawk}}}
	if up := cache.Model().UpcomingUnder(elsewhere, robin, nil, now(), 0, ""); !slices.ContainsFunc(up, func(u Card) bool { return u.ID == "meetup" }) {
		t.Errorf("an invitation is not in Robin's Upcoming")
	}
	if up := cache.Model().UpcomingUnder(elsewhere, mia, nil, now(), 0, ""); slices.ContainsFunc(up, func(u Card) bool { return u.ID == "meetup" }) {
		t.Errorf("the event is in Mia's Upcoming, uninvited")
	}
	// Robin's own and her children's invitations are hers to answer; Sam
	// is asked about his alone - not his mother's, nor his sister's.
	v := inviteView(t, robinH, "meetup")
	if v.Host || len(v.Mine) != 3 || v.Mine[0].Email != robin || !v.Mine[0].Mine || !v.Mine[1].Mine || !v.Mine[2].Mine || v.List != nil || v.Settings != nil || !v.Guests || len(v.Hosts) != 2 || v.Hosts[0].Name != "Jordan Whitfield" || v.Counts.Invited != 4 || v.Counts.Waiting != 4 {
		t.Errorf("robin's view = %+v", v)
	}
	v = inviteView(t, samH, "meetup")
	if len(v.Mine) != 1 || v.Mine[0].Email != sam || !v.Mine[0].Mine {
		t.Errorf("sam's view = %+v", v.Mine)
	}
	// Robin answers for the household: a yes for Sam, a maybe for Ella,
	// a no for herself; Sam cannot answer for Robin, Mia not for anyone
	// in it - Mia being a co-host may, though.
	if rec := call(t, robinH, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+sam+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("robin for sam: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, robinH, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+ella+`","answer":"maybe"}`); rec.Code != 204 {
		t.Errorf("robin for ella: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, robinH, "POST", "/api/calendar/rsvp", `{"id":"meetup","answer":"no"}`); rec.Code != 204 {
		t.Errorf("robin's own: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, samH, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+robin+`","answer":"yes"}`); rec.Code != 403 {
		t.Errorf("sam for robin: %d", rec.Code)
	}
	if rec := call(t, robinH, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+sam+`","answer":"hidden"}`); rec.Code != 400 {
		t.Errorf("hiding for someone else: %d", rec.Code)
	}
	if got := cache.Model().Answered[sam]["meetup"]; got.Answer != AnswerYes || got.By != robin || got.Via != ViaPage || got.At == "" {
		t.Errorf("sam's answer = %+v", got)
	}
	// A yes given by an invitee sends no second invite: the invitation
	// carried it.
	time.Sleep(30 * time.Millisecond)
	if len(kept.all()) != 6 {
		t.Errorf("mail after answering: %d", len(kept.all()))
	}
	// Robin brings a guest by name; Sam, a student, may not; nor anyone
	// once guests are off.
	rec = call(t, robinH, "POST", "/api/calendar/invites/guest", `{"id":"meetup","name":"Grandma June"}`)
	if rec.Code != 200 {
		t.Fatalf("guest: %d %s", rec.Code, rec.Body)
	}
	var made struct{ Email string }
	json.Unmarshal(rec.Body.Bytes(), &made)
	if !strings.HasPrefix(made.Email, guestPrefix) || cache.Model().AnswerOf(made.Email, "meetup") != AnswerYes {
		t.Errorf("guest = %q, answer %q", made.Email, cache.Model().AnswerOf(made.Email, "meetup"))
	}
	if g := cache.Model().InviteOf("meetup", made.Email); g == nil || g.GuestOf != robin || g.Via != ViaGuest || g.Name != "Grandma June" {
		t.Errorf("guest row = %+v", g)
	}
	if rec := call(t, samH, "POST", "/api/calendar/invites/guest", `{"id":"meetup","name":"A friend"}`); rec.Code != 200 {
		t.Errorf("a student's guest for themselves: %d %s", rec.Code, rec.Body)
	}
	// A guest with an address, brought after the invites went out, gets
	// theirs at once.
	rec = call(t, robinH, "POST", "/api/calendar/invites/guest", `{"id":"meetup","name":"Uncle Bo","email":"bo@example.org"}`)
	if rec.Code != 200 {
		t.Fatalf("guest with address: %d %s", rec.Code, rec.Body)
	}
	sent = waitFor(kept, 7)
	if bo := mailTo(kept, "bo@example.org"); len(sent) != 7 || len(bo) != 1 || !strings.Contains(bo[0].Text, "Invited: Uncle") {
		t.Errorf("guest's invite = %+v", sent)
	}
	// Mia, not invited, answers by the link: on the host's list as such,
	// and sent the invite a yes brings, since none came to her.
	if rec := call(t, miaH, "POST", "/api/calendar/rsvp", `{"id":"meetup","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("mia by link: %d", rec.Code)
	}
	waitFor(kept, 8)
	if m := mailTo(kept, mia); len(m) != 2 || !strings.HasPrefix(m[1].Subject, "Invitation: Class meetup") {
		t.Errorf("mia's own invite = %+v", m)
	}
	// The host's list: everyone - the seven on it, with the host and Mia
	// who answered by the link - with answers, who gave them, and the
	// guests under whoever brought them; the counts sum it up.
	v = inviteView(t, jordan, "meetup")
	if !v.Host || v.Settings == nil || v.Settings.Message != "Bring a snack to share!" || len(v.List) != 9 || v.Counts.Invited != 7 || v.Counts.Yes != 6 || v.Counts.Maybe != 1 || v.Counts.No != 1 || v.Counts.Waiting != 1 || v.Counts.Guests != 3 {
		t.Errorf("host's view: host %v settings %+v list %d counts %+v", v.Host, v.Settings, len(v.List), v.Counts)
	}
	if r := rowOf(v, sam); r == nil || r.Answer != AnswerYes || r.AnsweredBy != "Robin Whitfield" || !r.Invited || r.Sent == "" || r.Grade != "Grade 3" || r.Household != ella {
		t.Errorf("sam's row = %+v", r)
	}
	if r := rowOf(v, robin); r == nil || r.Answer != AnswerNo || r.AnsweredBy != "" || !r.Mine {
		t.Errorf("robin's row = %+v", r)
	}
	if r := rowOf(v, made.Email); r == nil || r.Email != "" || r.Name != "Grandma June" || r.GuestOfName != "Robin Whitfield" || r.Line != "Guest of Robin Whitfield" || !r.Outside || r.Answer != AnswerYes || r.Household != ella {
		t.Errorf("grandma's row = %+v", r)
	}
	if r := rowOf(v, coach); r == nil || r.Line != "Outside Helios" || !r.Outside || r.Answer != "" {
		t.Errorf("coach's row = %+v", r)
	}
	if r := rowOf(v, mia); r == nil || r.Invited || r.Answer != AnswerYes {
		t.Errorf("mia's row = %+v", r)
	}
	// Coming is everyone but the nos: the six yeses, the maybe, and the
	// coach still to answer - Robin's no kept from everyone but the hosts.
	if len(v.Coming) != 8 || slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Answer == AnswerNo }) {
		t.Errorf("coming = %d", len(v.Coming))
	}
	// Who is coming is everyone's to read while the list is public, and
	// the hosts' alone once it is private.
	if v := inviteView(t, robinH, "meetup"); len(v.Coming) != 8 || len(v.Mine) != 6 || slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Answer == AnswerNo }) {
		t.Errorf("robin reads who is coming: %d, mine %d", len(v.Coming), len(v.Mine))
	}
	call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","guestList":"private"}`)
	if v := inviteView(t, robinH, "meetup"); v.Coming != nil || v.GuestList != GuestListPrivate {
		t.Errorf("a private list read by an invitee: %+v", v.Coming)
	}
	// The host corrects Robin's answer, and takes the coach off; Robin
	// takes back a guest of her own but not Mia's place.
	if rec := call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+robin+`","answer":"yes"}`); rec.Code != 204 {
		t.Errorf("host for robin: %d", rec.Code)
	}
	if got := cache.Model().Answered[robin]["meetup"]; got.Answer != AnswerYes || got.By != host {
		t.Errorf("robin's corrected answer = %+v", got)
	}
	if rec := call(t, robinH, "DELETE", "/api/calendar/invites/people", `{"id":"meetup","email":"`+coach+`"}`); rec.Code != 403 {
		t.Errorf("robin removing the coach: %d", rec.Code)
	}
	if rec := call(t, robinH, "DELETE", "/api/calendar/invites/people", `{"id":"meetup","email":"`+made.Email+`"}`); rec.Code != 204 {
		t.Errorf("robin removing her guest: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, jordan, "DELETE", "/api/calendar/invites/people", `{"id":"meetup","email":"`+coach+`"}`); rec.Code != 204 {
		t.Errorf("host removing the coach: %d %s", rec.Code, rec.Body)
	}
	if rows := cache.Model().Invites["meetup"]; len(rows) != 5 || cache.Model().InviteOf("meetup", coach) != nil || cache.Model().AnswerOf(made.Email, "meetup") != "" {
		t.Errorf("after removals: %+v", rows)
	}
	// A reminder goes to whoever has not answered - nobody on the list
	// now, so the friend Sam brought aside, it is refused.
	if rec := call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"unanswered"}`); rec.Code != 400 {
		t.Errorf("reminder with nobody waiting: %d %s", rec.Code, rec.Body)
	}
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+mia+`","via":"search"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+mia+`","answer":""}`)
	rec = call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"unanswered"}`)
	if rec.Code != 200 || rec.Body.String() != "{\"invites\":1,\"messages\":1}\n" {
		t.Errorf("reminder: %d %s", rec.Code, rec.Body)
	}
	waitFor(kept, 9)
	if m := mailTo(kept, mia); len(m) != 3 || m[2].Subject != "[Class meetup] You're invited!" {
		t.Errorf("mia's first invite = %+v", m)
	}
	rec = call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"unanswered"}`)
	waitFor(kept, 10)
	if m := mailTo(kept, mia); rec.Code != 200 || len(m) != 4 || m[3].Subject != "[Class meetup] Reminder: you're invited!" || !strings.Contains(m[3].Text, "is still hoping to hear from Mia about") {
		t.Errorf("mia's reminder = %d %+v", rec.Code, m)
	}
	// A guest may be left to answer, and left uninvited for now; a host
	// then sends that one person their invite by name.
	rec = call(t, robinH, "POST", "/api/calendar/invites/guest", `{"id":"meetup","name":"Cousin Vi","email":"vi@example.org","answer":"","invite":false}`)
	if rec.Code != 200 || cache.Model().AnswerOf("vi@example.org", "meetup") != "" || cache.Model().InviteOf("meetup", "vi@example.org").Sent != "" {
		t.Errorf("a guest left to answer: %d %s, answer %q, row %+v", rec.Code, rec.Body, cache.Model().AnswerOf("vi@example.org", "meetup"), cache.Model().InviteOf("meetup", "vi@example.org"))
	}
	if rec := call(t, robinH, "POST", "/api/calendar/invites/guest", `{"id":"meetup","name":"X","answer":"no"}`); rec.Code != 400 {
		t.Errorf("a guest put down as no: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","emails":["vi@example.org"]}`); rec.Code != 200 || rec.Body.String() != "{\"invites\":1,\"messages\":1}\n" || cache.Model().InviteOf("meetup", "vi@example.org").Sent == "" {
		t.Errorf("send to one: %d %s", rec.Code, rec.Body)
	}
}

// A party's guest list is its Celebrate hosts' to build - from the
// ticket holders, who stand apart from those who answered - and an
// answer on it is an answer to the party.
func TestPartyInvitation(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	miaH := as(mia, mux)
	robinH := as(robin, mux)
	// A ticket holder may invite people to the party one at a time - as
	// anyone may to any event on their calendar - never a group.
	if rec := call(t, robinH, "POST", "/api/calendar/invites/group", `{"id":"`+partyA+`","rule":{"tags":["Carpool"]}}`); rec.Code != 403 {
		t.Errorf("a ticket holder adding a group: %d", rec.Code)
	}
	rec := call(t, miaH, "GET", "/api/calendar/invites/people?id="+partyA, "")
	var picker PickerView
	json.Unmarshal(rec.Body.Bytes(), &picker)
	if rec.Code != 200 || len(picker.Attendees) != 3 || picker.Attendees[0].Email != robin || picker.Attendees[2].Status != "waitlist" {
		t.Errorf("party picker: %d %+v", rec.Code, picker.Attendees)
	}
	if rec := call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"`+partyA+`","people":[{"email":"`+robin+`","via":"tickets"},{"email":"`+sam+`","via":"tickets"},{"email":"`+ella+`","via":"family"}]}`); rec.Code != 200 {
		t.Fatalf("party list: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, miaH, "POST", "/api/calendar/invites/send", `{"id":"`+partyA+`"}`); rec.Code != 200 {
		t.Fatalf("party send: %d %s", rec.Code, rec.Body)
	}
	sent := waitFor(kept, 3)
	if r := mailTo(kept, robin); len(sent) != 3 || len(r) != 1 || !strings.Contains(r[0].Subject, "[Fondue Night] You're invited!") || !strings.Contains(string(r[0].Attachments[0].Content), "UID:celebrate/p1@calendar.heliosian.com") || len(mailTo(kept, sam)) != 1 {
		t.Errorf("party invite = %+v", sent)
	}
	call(t, robinH, "POST", "/api/calendar/invites/answer", `{"id":"`+partyA+`","email":"`+sam+`","answer":"yes"}`)
	v := inviteView(t, miaH, partyA)
	if !v.Host || !v.Party || v.Counts.Tickets != 2 || v.Counts.TicketsWaiting != 1 || v.Counts.Invited != 3 {
		t.Errorf("party view: host %v party %v counts %+v", v.Host, v.Party, v.Counts)
	}
	if r := rowOf(v, robin); r == nil || r.Ticket != "ticket" || r.Answer != "" {
		t.Errorf("robin on the party = %+v", r)
	}
	// A guest reads who is coming without the hosts' side of it.
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

// A calendar app's reply counts for a guest from outside once they are on
// the list, and a tentative one is a maybe.
func TestRepliesFromGuests(t *testing.T) {
	mux, cache, _, store := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":["Jays"],"inviteOnly":true,"id":"meetup"}`)
	store[stored("coach")] = []byte(replyMail(coach, coach, "meetup", "ACCEPTED", "dkim=pass header.d=example.org"))
	store[stored("robin")] = []byte(replyMail(robin, robin, "meetup", "TENTATIVE", "spf=pass smtp.mailfrom=robin.whitfield@heliosschool.org"))
	post := func(id, from string) int {
		stamp, sig := mail.SignMailgun(replySecret, "token-"+id, now())
		fields := map[string]string{"recipient": replyAddress("meetup", from), "from": from, "subject": "Accepted: Meetup", "message-url": stored(id), "timestamp": stamp, "token": "token-" + id, "signature": sig}
		body, _ := json.Marshal(fields)
		req := httptest.NewRequest("POST", "https://when.local.heliosian.com:8080/hooks/replies", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post("coach", coach); code != 200 || cache.Model().AnswerOf(coach, "meetup") != "" {
		t.Errorf("a stranger's reply before the list: %d %q", code, cache.Model().AnswerOf(coach, "meetup"))
	}
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+coach+`","name":"Coach Lee"},{"email":"`+robin+`"}]}`)
	if code := post("coach", coach); code != 200 || cache.Model().AnswerOf(coach, "meetup") != AnswerYes {
		t.Errorf("a guest's reply: %d %q", code, cache.Model().AnswerOf(coach, "meetup"))
	}
	if code := post("robin", robin); code != 200 || cache.Model().AnswerOf(robin, "meetup") != AnswerMaybe || cache.Model().Answered[robin]["meetup"].Via != ViaCalendar {
		t.Errorf("a tentative reply: %d %+v", code, cache.Model().Answered[robin]["meetup"])
	}
}

// Someone from outside the community gets a page of their own, found by
// the secret their invitation carries, served without sign-in: the event,
// the hosts and their own answer - never the guest list - where they
// answer and bring a guest.
func TestOutsideInvitation(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","location":"The park","tags":[],"inviteOnly":true,"id":"meetup"}`)
	if e := cache.Model().Event("meetup"); e == nil || len(e.Tags) != 0 {
		t.Fatalf("an invite-only event with no tags = %+v", e)
	}
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","message":"Bring cleats."}`)
	// The outside page wears the event's banner, served public by id.
	if rec := call(t, mux, "GET", "/open/banner/meetup", ""); rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/") {
		t.Errorf("banner: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	inv := cache.Model().InviteOf("meetup", coach)
	if inv == nil || len(inv.Token) != 24 || cache.Model().InviteOf("meetup", robin).Token != "" {
		t.Fatalf("tokens: coach %+v, robin %+v", inv, cache.Model().InviteOf("meetup", robin))
	}
	// The host's list carries the coach's link, for passing on by hand.
	if r := rowOf(inviteView(t, jordan, "meetup"), coach); r == nil || r.Link != "/ext/"+inv.Token {
		t.Errorf("coach's row = %+v", r)
	}
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 3)
	if m := mailTo(kept, coach); len(m) != 1 || !strings.Contains(m[0].HTML, "https://when.heliosian.com/ext/"+inv.Token) || strings.Contains(m[0].Text, "/e/meetup") || !strings.Contains(string(m[0].Attachments[0].Content), "URL:https://when.heliosian.com/ext/"+inv.Token) || !strings.Contains(m[0].HTML, "no account needed") {
		t.Errorf("coach's mail = %+v", m)
	}
	if m := mailTo(kept, robin); len(m) != 1 || !strings.Contains(m[0].HTML, "https://when.heliosian.com/e/meetup") || strings.Contains(m[0].HTML, "/ext/") {
		t.Errorf("robin's mail = %+v", m)
	}
	// The page and its routes need no session.
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
	if rec := call(t, mux, "POST", "/open/ext/"+inv.Token, `{"answer":"maybe"}`); rec.Code != 204 || cache.Model().AnswerOf(coach, "meetup") != AnswerMaybe {
		t.Errorf("answer from outside: %d %q", rec.Code, cache.Model().AnswerOf(coach, "meetup"))
	}
	if rec := call(t, mux, "POST", "/open/ext/"+inv.Token, `{"answer":"hidden"}`); rec.Code != 400 {
		t.Errorf("hiding from outside: %d", rec.Code)
	}
	rec = call(t, mux, "POST", "/open/ext/"+inv.Token+"/guest", `{"name":"Assistant Coach","email":"assistant@example.org"}`)
	if rec.Code != 200 {
		t.Fatalf("guest from outside: %d %s", rec.Code, rec.Body)
	}
	// The guest, from outside too, gets a page of their own and their
	// invite at once.
	guest := cache.Model().InviteOf("meetup", "assistant@example.org")
	if guest == nil || guest.GuestOf != coach || guest.Token == "" || cache.Model().AnswerOf("assistant@example.org", "meetup") != AnswerYes {
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
	if rec := call(t, mux, "DELETE", "/open/ext/"+inv.Token+"/guest", `{"key":"assistant@example.org"}`); rec.Code != 204 || cache.Model().InviteOf("meetup", "assistant@example.org") != nil {
		t.Errorf("taking a guest back from outside: %d", rec.Code)
	}
	// A flyer on the invitation shows on the outside page and is fetched
	// public; the page's head carries the event's preview for a chat app.
	if rec := call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","flyer":"sample/flyer.jpg"}`); rec.Code != 204 {
		t.Errorf("flyer: %d %s", rec.Code, rec.Body)
	}
	rec = call(t, mux, "GET", "/open/ext/"+inv.Token, "")
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Flyer != "/open/flyer/meetup" {
		t.Errorf("outside view's flyer = %q", v.Flyer)
	}
	if rec := call(t, mux, "GET", "/ext/"+inv.Token, ""); !strings.Contains(rec.Body.String(), `property="og:title" content="Meetup"`) || !strings.Contains(rec.Body.String(), "/open/share/meetup.png") {
		t.Errorf("outside page's head lacks the preview: %s", rec.Body.String()[:400])
	}
}

// A group is a rule on the list: its matches go on when it is added, and
// again as new ones come while Auto is on - sent their invites when the
// invitation has gone out. Taking the group off takes off whoever it
// added who has not been sent one.
func TestInviteGroups(t *testing.T) {
	mux, cache, kept, _, sources := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	rec := call(t, jordan, "GET", "/api/calendar/invites/options", "")
	var options filter.Options
	json.Unmarshal(rec.Body.Bytes(), &options)
	if rec.Code != 200 || !slices.Contains(options.Classrooms, "Jays") || !slices.Contains(options.Tags, "Carpool") {
		t.Fatalf("options: %d %+v", rec.Code, options)
	}
	rec = call(t, jordan, "POST", "/api/calendar/invites/preview", `{"rule":{"tags":["Carpool"]}}`)
	var preview struct {
		Count int
		Names []string
	}
	json.Unmarshal(rec.Body.Bytes(), &preview)
	if rec.Code != 200 || preview.Count != 2 || len(preview.Names) != 2 {
		t.Errorf("preview: %d %+v", rec.Code, preview)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/group", `{"id":"meetup","rule":{}}`); rec.Code != 400 {
		t.Errorf("an empty rule: %d", rec.Code)
	}
	if rec := call(t, as(mia, mux), "POST", "/api/calendar/invites/group", `{"id":"meetup","rule":{"tags":["Carpool"]}}`); rec.Code != 403 {
		t.Errorf("someone else's group: %d", rec.Code)
	}
	rec = call(t, jordan, "POST", "/api/calendar/invites/group", `{"id":"meetup","rule":{"tags":["Carpool"]}}`)
	var made struct {
		Group string
		Added int
	}
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Added != 2 || made.Group == "" {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	g := cache.Model().GroupOf("meetup", made.Group)
	if g == nil || !g.Auto || g.Rule.Kind != "include" || g.Rule.Owner != host || strings.Join(g.Rule.Tags, ",") != "Carpool" {
		t.Fatalf("group row = %+v", g)
	}
	abena := cache.Model().InviteOf("meetup", "abena.osei@heliosschool.org")
	if abena == nil || abena.Via != ViaGroup+g.ID || abena.AddedBy != host {
		t.Errorf("a member's row = %+v", abena)
	}
	v := inviteView(t, jordan, "meetup")
	if len(v.Groups) != 1 || v.Groups[0].Count != 2 || v.Counts.Invited != 2 {
		t.Errorf("view groups = %+v counts %+v", v.Groups, v.Counts)
	}
	// Someone new is tagged in Carpool: the next look at the list notes
	// them but waits the grace - a tag given by mistake can be taken back
	// - and only a look after it brings them on and, the invites having
	// gone out, sends them theirs.
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 3)
	sources.tables.Tags = append(sources.tables.Tags, map[string]string{"Owner Email": host, "Tag": "Carpool", "Person Email": mia})
	if v := inviteView(t, jordan, "meetup"); rowOf(v, mia) != nil {
		t.Errorf("a newcomer came on inside the grace")
	}
	start := now()
	now = func() time.Time { return start.Add(grace - time.Minute) }
	if v := inviteView(t, jordan, "meetup"); rowOf(v, mia) != nil {
		t.Errorf("a newcomer came on a minute short of the grace")
	}
	// Taken back and given again inside the grace, the clock restarts.
	sources.tables.Tags = sources.tables.Tags[:len(sources.tables.Tags)-1]
	inviteView(t, jordan, "meetup")
	sources.tables.Tags = append(sources.tables.Tags, map[string]string{"Owner Email": host, "Tag": "Carpool", "Person Email": mia})
	now = func() time.Time { return start.Add(grace + time.Minute) }
	if v := inviteView(t, jordan, "meetup"); rowOf(v, mia) != nil {
		t.Errorf("a newcomer came on with the clock restarted")
	}
	now = func() time.Time { return start.Add(2*grace + 2*time.Minute) }
	v = inviteView(t, jordan, "meetup")
	if r := rowOf(v, mia); r == nil || r.Via != ViaGroup+g.ID || r.Sent == "" || v.Groups[0].Count != 3 {
		t.Errorf("mia after the grace = %+v, groups %+v", r, v.Groups)
	}
	now = func() time.Time { return time.Now().In(Location) }
	waitFor(kept, 4)
	if m := mailTo(kept, mia); len(m) != 1 || !strings.Contains(m[0].Subject, "You're invited!") {
		t.Errorf("mia's auto invite = %+v", m)
	}
	if g := cache.Model().GroupOf("meetup", g.ID); g.Sent == "" {
		t.Errorf("the group is not marked sent after its invites went: %+v", g)
	}
	// Skipping someone pending marks them sent with no email, and a group
	// left with nobody unsent goes live the same way.
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`)
	before := len(kept.all())
	if rec := call(t, jordan, "POST", "/api/calendar/invites/skip", `{"id":"meetup","emails":["`+robin+`"]}`); rec.Code != 200 || rec.Body.String() != "{\"skipped\":1}\n" {
		t.Errorf("skip: %d %s", rec.Code, rec.Body)
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), robin); r == nil || r.Sent == "" || len(kept.all()) != before {
		t.Errorf("skipped row = %+v, mail %d -> %d", r, before, len(kept.all()))
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/skip", `{"id":"meetup","emails":["`+robin+`"]}`); rec.Code != 400 {
		t.Errorf("skipping someone sent: %d", rec.Code)
	}
	call(t, jordan, "DELETE", "/api/calendar/invites/people", `{"id":"meetup","email":"`+robin+`"}`)
	// A second group, added once the invitation is out, still waits for
	// the host: its people go on unsent, and nobody hears until Send.
	rec = call(t, jordan, "POST", "/api/calendar/invites/group", `{"id":"meetup","rule":{"roles":["Student"],"classrooms":["Ospreys"]}}`)
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Added == 0 {
		t.Fatalf("second group: %d %s", rec.Code, rec.Body)
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), ella); r == nil || r.Sent != "" {
		t.Errorf("ella came on sent: %+v", r)
	}
	if g2 := cache.Model().GroupOf("meetup", made.Group); g2 == nil || g2.Sent != "" || len(mailTo(kept, ella)) != 0 {
		t.Errorf("a new group's people were sent before the host did: %+v, mail %d", g2, len(mailTo(kept, ella)))
	}
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 4+made.Added)
	if g2 := cache.Model().GroupOf("meetup", made.Group); g2.Sent == "" || len(mailTo(kept, ella)) != 1 {
		t.Errorf("after sending the second group: %+v, mail %d", g2, len(mailTo(kept, ella)))
	}
	// Auto off: the next newcomer still comes on after the grace, but
	// lands pending for the host rather than being sent.
	if rec := call(t, jordan, "PUT", "/api/calendar/invites/group", `{"id":"meetup","group":"`+g.ID+`","auto":false}`); rec.Code != 204 {
		t.Errorf("auto off: %d %s", rec.Code, rec.Body)
	}
	sources.tables.Tags = append(sources.tables.Tags, map[string]string{"Owner Email": host, "Tag": "Carpool", "Person Email": robin})
	start = now()
	if v := inviteView(t, jordan, "meetup"); rowOf(v, robin) != nil {
		t.Errorf("a newcomer came on inside the grace with auto off")
	}
	now = func() time.Time { return start.Add(grace + time.Minute) }
	if r := rowOf(inviteView(t, jordan, "meetup"), robin); r == nil || r.Sent != "" || r.Via != ViaGroup+g.ID {
		t.Errorf("a newcomer with auto off = %+v", r)
	}
	now = func() time.Time { return time.Now().In(Location) }
	// Robin has Ella's invite as her parent; none of her own.
	if m := mailTo(kept, robin); slices.ContainsFunc(m, func(m mail.Message) bool { return strings.Contains(m.HTML, "Invited: Robin") }) {
		t.Errorf("a newcomer was sent with auto off: %+v", m)
	}
	// Taking the group off: Mia's and Abena's invites went out, so they
	// stay; Robin, unsent, goes with it.
	if rec := call(t, jordan, "DELETE", "/api/calendar/invites/group", `{"id":"meetup","group":"`+g.ID+`"}`); rec.Code != 200 || rec.Body.String() != "{\"dropped\":1}\n" {
		t.Errorf("remove group: %d %s", rec.Code, rec.Body)
	}
	if cache.Model().GroupOf("meetup", g.ID) != nil || cache.Model().InviteOf("meetup", mia) == nil || cache.Model().InviteOf("meetup", robin) != nil {
		t.Errorf("after removing: groups %d, mia %v", len(cache.Model().Groups["meetup"]), cache.Model().InviteOf("meetup", mia))
	}
	// A group on a fresh, unsent list goes with its people.
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Other","start":"2026-10-11 15:00","tags":[],"inviteOnly":true,"id":"other"}`)
	rec = call(t, jordan, "POST", "/api/calendar/invites/group", `{"id":"other","rule":{"roles":["Student"],"classrooms":["Jays"]},"auto":false}`)
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Added == 0 {
		t.Fatalf("classroom group: %d %s", rec.Code, rec.Body)
	}
	if rec := call(t, jordan, "DELETE", "/api/calendar/invites/group", `{"id":"other","group":"`+made.Group+`"}`); rec.Code != 200 || !strings.Contains(rec.Body.String(), fmt.Sprint(made.Added)) || len(cache.Model().Invites["other"]) != 0 {
		t.Errorf("remove unsent group: %d %s, left %d", rec.Code, rec.Body, len(cache.Model().Invites["other"]))
	}
}

// A host writes to the list by where people stand: one email per adult,
// a student's to their parents, with the household's answers and a nudge
// for anyone still to answer; replies go to the host.
func TestHostMessage(t *testing.T) {
	mux, _, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"},{"email":"`+sam+`"},{"email":"`+mia+`"},{"email":"`+coach+`","name":"Coach Lee"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+robin+`","answer":"yes"}`)
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+mia+`","answer":"no"}`)
	waitFor(kept, 1)
	before := len(kept.all())
	if rec := call(t, jordan, "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"Chairs","message":"","to":["yes"]}`); rec.Code != 400 {
		t.Errorf("no words: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"  ","message":"Hi","to":["yes"]}`); rec.Code != 400 {
		t.Errorf("no subject: %d", rec.Code)
	}
	if rec := call(t, as(mia, mux), "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"Hi","message":"Hi","to":["yes"]}`); rec.Code != 403 {
		t.Errorf("someone else's message: %d", rec.Code)
	}
	// Yes, maybe and no response: Robin (yes), Sam (no response) and his
	// parent Robin again, the coach (no response) - not Mia (no).
	rec := call(t, jordan, "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"Chairs, please","message":"Bring a chair!","to":["yes","maybe","none"]}`)
	if rec.Code != 200 || rec.Body.String() != "{\"messages\":3}\n" {
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
	if m := mailTo(kept, mia); len(m) != 0 {
		t.Errorf("a no was written to: %+v", m)
	}
	// To the nos alone: Mia, with no nudge.
	call(t, jordan, "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"Next time","message":"Sorry you can't make it.","to":["no"],"attach":true}`)
	waitFor(kept, before+4)
	if m := mailTo(kept, mia); len(m) != 1 || !strings.Contains(m[0].Text, "You: No") || strings.Contains(m[0].Text, "not answered") || len(m[0].Attachments) != 1 || !strings.Contains(string(m[0].Attachments[0].Content), "UID:meetup@") {
		t.Errorf("mia's message = %+v", m)
	}
	if r := mailTo(kept, robin); len(r[0].Attachments) != 0 {
		t.Errorf("a plain message carried an invite")
	}
	// Named people, whatever they stand: the coach alone.
	rec = call(t, jordan, "POST", "/api/calendar/invites/message", `{"id":"meetup","subject":"Just you","message":"A word for you.","to":[],"emails":["`+coach+`"]}`)
	if rec.Code != 200 || rec.Body.String() != "{\"messages\":1}\n" {
		t.Errorf("message to named people: %d %s", rec.Code, rec.Body)
	}
}

// Create Invite on Helios Celebrate starts a party's list here: the
// invitation, and one group for the party's ticket holders with Auto on,
// added once; and Celebrate reads the answers back once the invites have
// gone out.
func TestPartyStart(t *testing.T) {
	mux, cache, _, _ := invitesApp(t)
	miaH := as(mia, mux)
	if rec := call(t, as(robin, mux), "POST", "/api/calendar/invites/start", `{"id":"`+partyA+`"}`); rec.Code != 403 {
		t.Errorf("a ticket holder starting: %d", rec.Code)
	}
	rec := call(t, miaH, "POST", "/api/calendar/invites/start", `{"id":"`+partyA+`"}`)
	var made struct{ Group string }
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Group == "" {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	g := cache.Model().GroupOf(partyA, made.Group)
	if g == nil || !g.Auto || strings.Join(g.Rule.Tags, ",") != "party:p1" || g.Rule.Owner != mia || cache.Model().Invitations[partyA] == nil {
		t.Errorf("party group = %+v", g)
	}
	rec = call(t, miaH, "POST", "/api/calendar/invites/start", `{"id":"`+partyA+`"}`)
	var again struct{ Group string }
	json.Unmarshal(rec.Body.Bytes(), &again)
	if rec.Code != 200 || again.Group != made.Group || len(cache.Model().Groups[partyA]) != 1 {
		t.Errorf("a second start: %d %+v, groups %d", rec.Code, again, len(cache.Model().Groups[partyA]))
	}
	if sent, _, ok := cache.PartyRSVPs("p1"); !ok || sent {
		t.Errorf("rsvps before sending: ok %v sent %v", ok, sent)
	}
	call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"`+partyA+`","people":[{"email":"`+robin+`"},{"email":"`+sam+`"}]}`)
	call(t, miaH, "POST", "/api/calendar/invites/send", `{"id":"`+partyA+`"}`)
	call(t, miaH, "POST", "/api/calendar/invites/answer", `{"id":"`+partyA+`","email":"`+robin+`","answer":"maybe"}`)
	sent, answers, ok := cache.PartyRSVPs("p1")
	if !ok || !sent || answers[robin] != AnswerMaybe || answers[sam] != "none" || answers[mia] != "" {
		t.Errorf("rsvps = ok %v sent %v %v", ok, sent, answers)
	}
	if _, _, ok := cache.PartyRSVPs("nope"); ok {
		t.Errorf("a party with no list has rsvps")
	}
}

// A student's invitation goes to them with their parents on the Cc, so
// the parents hear with the child and answer for them - one message, the
// child's calendar invite on it, its lead naming whose it is - and the
// bold line over the button says whom the RSVP is for, linked to the page.
func TestStudentInviteCcsParents(t *testing.T) {
	mux, _, kept, _ := invitesApp(t)
	miaH := as(mia, mux)
	call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"`+partyA+`","people":[{"email":"`+sam+`"},{"email":"`+ella+`"}]}`)
	call(t, miaH, "POST", "/api/calendar/invites/send", `{"id":"`+partyA+`"}`)
	sent := waitFor(kept, 2)
	if len(sent) != 2 || len(mailTo(kept, robin)) != 0 {
		t.Fatalf("mail after sending: %+v", sent)
	}
	m := mailTo(kept, sam)
	if len(m) != 1 || strings.Join(m[0].CC, ",") != robin || len(m[0].Attachments) != 1 || !strings.Contains(string(m[0].Attachments[0].Content), "ATTENDEE;CN="+sam) {
		t.Fatalf("sam's mail = %+v", m)
	}
	if !strings.Contains(m[0].HTML, "Mia Torres sent Sam an invitation for") || !strings.Contains(m[0].HTML, `<a href="https://when.heliosian.com/e/celebrate/p1" style="color:#1b2a2c;font-weight:700">RSVP for Sam and Ella here</a>`) || !strings.Contains(m[0].Text, "Mia Torres sent Sam an invitation for") || !strings.Contains(m[0].Text, "RSVP for Sam and Ella here") {
		t.Errorf("sam's words:\n%s", m[0].Text)
	}
	if e := mailTo(kept, ella); len(e) != 1 || strings.Join(e[0].CC, ",") != robin || !strings.Contains(e[0].HTML, "sent Ella an invitation for") {
		t.Errorf("ella's mail = %+v", e)
	}
	if got := andList([]string{"Sam", "Ella", "Robin", "Mia"}); got != "Sam, Ella, Robin and Mia" {
		t.Errorf("andList = %q", got)
	}
}

// A party's list on Who? carries whoever bought a ticket beside whoever
// holds one; the guest list takes the holders and the hosts alone, so a
// parent who bought their child's ticket hears with the child and answers
// for them, without a place of their own.
func TestPartyListIsTheHolders(t *testing.T) {
	mux, cache, kept, _, sources := invitesAppWith(t)
	sources.extra = map[string][]string{"party:p1": {mia, robin, sam, host}}
	miaH := as(mia, mux)
	rec := call(t, miaH, "POST", "/api/calendar/invites/start", `{"id":"`+partyA+`"}`)
	var made struct{ Added int }
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Added != 3 {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	for _, email := range []string{mia, robin, sam} {
		if cache.Model().InviteOf(partyA, email) == nil {
			t.Errorf("%s is not on the list", email)
		}
	}
	if cache.Model().InviteOf(partyA, host) != nil {
		t.Errorf("the buyer of a ticket is on the list")
	}
	call(t, miaH, "POST", "/api/calendar/invites/send", `{"id":"`+partyA+`"}`)
	to := []string{}
	for _, m := range waitFor(kept, 3) {
		to = append(to, m.To...)
	}
	slices.Sort(to)
	if strings.Join(to, ",") != strings.Join([]string{mia, robin, sam}, ",") {
		t.Errorf("sent to %v", to)
	}
}

// A party's invitation may say the event its own way - a title, when,
// where, a description - which the invitation email, the calendar invite,
// the outside person's page and everyone invited see, while the party
// itself stays as Celebrate has it for everyone else.
func TestInvitationDetails(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	miaH := as(mia, mux)
	robinH := as(robin, mux)
	call(t, miaH, "POST", "/api/calendar/invites/people", `{"id":"`+partyA+`","people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee"}]}`)
	if rec := call(t, miaH, "PUT", "/api/calendar/invites/settings", `{"id":"`+partyA+`","start":"not a time"}`); rec.Code != 400 {
		t.Errorf("a bad start: %d", rec.Code)
	}
	if rec := call(t, miaH, "PUT", "/api/calendar/invites/settings", `{"id":"`+partyA+`","title":"Fondue: the early sitting","start":"2026-11-14 17:30","end":"2026-11-14 19:00","location":"The Torres kitchen","description":"Come early - the kids eat first."}`); rec.Code != 204 {
		t.Fatalf("details: %d %s", rec.Code, rec.Body)
	}
	inv := cache.Model().Invitations[partyA]
	if inv.Title != "Fondue: the early sitting" || inv.Start != "2026-11-14 17:30" || inv.End != "2026-11-14 19:00" || inv.Location != "The Torres kitchen" {
		t.Errorf("invitation = %+v", inv)
	}
	// Uninvited, Sam sees the party as Celebrate has it; invited, Robin
	// sees the invitation's words - and so does the outside page.
	call(t, miaH, "POST", "/api/calendar/invites/send", `{"id":"`+partyA+`"}`)
	waitFor(kept, 2)
	var view View
	rec := call(t, robinH, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	i := slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue: the early sitting" || view.Events[i].Start != "2026-11-14 17:30" || view.Events[i].Location != "The Torres kitchen" {
		t.Errorf("robin's party = %+v", view.Events[i])
	}
	rec = call(t, as(sam, mux), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	// Sam is in Robin's household, so invited too; Ella's parents' other
	// child - use the admin Dana, uninvited.
	rec = call(t, as("dana.hawkins@heliosschool.org", mux), "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	i = slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue Night" || view.Events[i].Start != "2026-11-14 18:00" {
		t.Errorf("an uninvited view of the party = %+v", view.Events[i])
	}
	m := mailTo(kept, robin)
	if len(m) != 1 || !strings.Contains(m[0].Subject, "[Fondue: the early sitting] You're invited!") || !strings.Contains(m[0].Text, "5:30") || !strings.Contains(m[0].Text, "The Torres kitchen") || !strings.Contains(string(m[0].Attachments[0].Content), "SUMMARY:Fondue: the early sitting") || !strings.Contains(string(m[0].Attachments[0].Content), "DTSTART:20261115T013000Z") {
		t.Errorf("robin's invite = %+v", m)
	}
	token := cache.Model().InviteOf(partyA, coach).Token
	rec = call(t, mux, "GET", "/open/ext/"+token, "")
	var ext ExtView
	json.Unmarshal(rec.Body.Bytes(), &ext)
	if ext.Banner != "/open/banner/"+partyA {
		t.Errorf("outside banner = %q", ext.Banner)
	}
	if ext.Title != "Fondue: the early sitting" || ext.Location != "The Torres kitchen" || !strings.Contains(ext.Hours, "5:30") {
		t.Errorf("outside view = %+v", ext)
	}
	// Blanked, the party's own words come back.
	call(t, miaH, "PUT", "/api/calendar/invites/settings", `{"id":"`+partyA+`","title":"","start":"","location":"","description":""}`)
	rec = call(t, robinH, "GET", "/api/calendar/model", "")
	json.NewDecoder(rec.Body).Decode(&view)
	i = slices.IndexFunc(view.Events, func(e *Event) bool { return e.ID == partyA })
	if i < 0 || view.Events[i].Title != "Fondue Night" {
		t.Errorf("robin's party once blanked = %+v", view.Events[i])
	}
}

// An address that may not reach anyone wears a warning for the host - a
// school address the directory does not hold, or one the provider has
// bounced - who may change it, keeping everything else on the row, or
// send to it anyway.
func TestAddressWarnings(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	alum := "old.grad@heliosschool.org"
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+alum+`","name":"Old Grad","via":"outside"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"},{"email":"`+robin+`"}]}`)
	v := inviteView(t, jordan, "meetup")
	if r := rowOf(v, alum); r == nil || r.Warning != "unknown" || r.WarningWords == "" {
		t.Errorf("a school address off the directory = %+v", r)
	}
	if r := rowOf(v, coach); r == nil || r.Warning != "" {
		t.Errorf("an outside address = %+v", r)
	}
	if r := rowOf(v, robin); r == nil || r.Warning != "" {
		t.Errorf("a directory address = %+v", r)
	}
	// The provider reports the coach's address gone: a bounce, noted.
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
	if r := rowOf(inviteView(t, jordan, "meetup"), coach); r == nil || r.Warning != "bounced" || !strings.Contains(r.WarningWords, "No such user here") {
		t.Errorf("a bounced address = %+v", r)
	}
	// An unsigned call is refused.
	req := httptest.NewRequest("POST", "/hooks/events", strings.NewReader(`{"signature":{},"event-data":{"event":"failed","severity":"permanent","recipient":"x@example.org"}}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotAcceptable {
		t.Errorf("unsigned event: %d", rec.Code)
	}
	// The host may still send to a warned address.
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","emails":["`+coach+`"]}`)
	waitFor(kept, 2)
	if len(mailTo(kept, coach)) != 1 {
		t.Errorf("sending to a bounced address anyway: %d mails", len(mailTo(kept, coach)))
	}
	// Or change it: the row keeps its token and its sent mark, and an
	// answer moves with it; a directory person's address is not theirs to
	// change, nor to one already on the list.
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+coach+`","answer":"yes"}`)
	token := cache.Model().InviteOf("meetup", coach).Token
	if rec := call(t, jordan, "POST", "/api/calendar/invites/email", `{"id":"meetup","email":"`+coach+`","to":"coach.lee@example.org"}`); rec.Code != 204 {
		t.Fatalf("change address: %d %s", rec.Code, rec.Body)
	}
	moved := cache.Model().InviteOf("meetup", "coach.lee@example.org")
	if moved == nil || moved.Token != token || moved.Sent == "" || cache.Model().InviteOf("meetup", coach) != nil || cache.Model().AnswerOf("coach.lee@example.org", "meetup") != AnswerYes {
		t.Errorf("after the change: %+v, old %v, answer %q", moved, cache.Model().InviteOf("meetup", coach), cache.Model().AnswerOf("coach.lee@example.org", "meetup"))
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), "coach.lee@example.org"); r == nil || r.Warning != "" {
		t.Errorf("the new address = %+v", r)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/email", `{"id":"meetup","email":"`+robin+`","to":"robin@example.org"}`); rec.Code != 400 {
		t.Errorf("changing a directory address: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/email", `{"id":"meetup","email":"`+alum+`","to":"coach.lee@example.org"}`); rec.Code != 400 {
		t.Errorf("changing to an address on the list: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/email", `{"id":"meetup","email":"`+alum+`","to":"not an address"}`); rec.Code != 400 {
		t.Errorf("changing to nonsense: %d", rec.Code)
	}
}

// A host takes an event back before anyone hears of it, or calls it off
// once the invites are out - telling the guests or not, with a calendar
// cancellation when they do.
func TestDeleteAndCancel(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	// Nothing sent: Delete takes the event and its list away whole.
	if rec := call(t, as(mia, mux), "POST", "/api/calendar/invites/delete", `{"id":"meetup"}`); rec.Code != 403 {
		t.Errorf("someone else deleting: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/invites/delete", `{"id":"meetup"}`); rec.Code != 200 || rec.Body.String() != "{\"event\":true}\n" {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	if m.Event("meetup") != nil || m.Invitations["meetup"] != nil || len(m.Invites["meetup"]) != 0 {
		t.Errorf("after delete: event %v, invitation %v, invites %d", m.Event("meetup"), m.Invitations["meetup"], len(m.Invites["meetup"]))
	}
	// Sent: Delete is refused; Cancel marks the event and tells the guests.
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 3)
	before := len(kept.all())
	if rec := call(t, jordan, "POST", "/api/calendar/invites/delete", `{"id":"meetup"}`); rec.Code != 400 {
		t.Errorf("deleting once sent: %d", rec.Code)
	}
	if rec := call(t, jordan, "POST", "/api/calendar/events/cancel", `{"id":"meetup","notify":true,"note":"Rain, sadly."}`); rec.Code != 200 || rec.Body.String() != "{\"told\":2}\n" {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
	if e := cache.Model().Event("meetup"); e == nil || !e.Cancelled || e.Status != StatusCancelled {
		t.Errorf("after cancel: %+v", e)
	}
	// Off the guests' calendars: Robin's household no longer has it.
	if events := cache.Model().eventsFor(robin, nil); slices.ContainsFunc(events, func(e *Event) bool { return e.ID == "meetup" }) {
		t.Errorf("a cancelled event still on a guest's calendar")
	}
	waitFor(kept, before+2)
	msgs := mailTo(kept, coach)
	if len(msgs) != 2 || msgs[1].Subject != "[Meetup] Cancelled" || !strings.Contains(msgs[1].HTML, "Rain, sadly.") || !strings.Contains(msgs[1].HTML, "Jordan Whitfield has cancelled Meetup") {
		t.Fatalf("the coach's cancellation: %+v", msgs)
	}
	ics := string(msgs[1].Attachments[0].Content)
	if msgs[1].Attachments[0].Name != "cancel.ics" || !strings.Contains(ics, "METHOD:CANCEL") || !strings.Contains(ics, "STATUS:CANCELLED") || !strings.Contains(ics, "UID:"+uidOf("meetup")) {
		t.Errorf("cancellation file:\n%s", ics)
	}
	if !slices.Contains(msgs[1].ReplyTo, host) {
		t.Errorf("reply-to: %v", msgs[1].ReplyTo)
	}
	// Cancelled again, nothing more happens; a party is Celebrate's.
	if rec := call(t, jordan, "POST", "/api/calendar/events/cancel", `{"id":"meetup","notify":true}`); rec.Code != 204 {
		t.Errorf("cancelling twice: %d", rec.Code)
	}
}

// Once the details change, the invitation goes again to everyone who has
// it as an update - and to nobody still pending.
func TestUpdatedInvitation(t *testing.T) {
	mux, _, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 2)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+mia+`"},{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`)
	// The coach is sent the invitation and says no: the update skips them.
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","emails":["`+coach+`"]}`)
	waitFor(kept, 3)
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+coach+`","answer":"no"}`)
	call(t, jordan, "PUT", "/api/calendar/events", `{"id":"meetup","title":"Meetup","start":"2026-10-10 16:00","end":"2026-10-10 16:00","location":"The park","tags":[],"inviteOnly":true}`)
	before := len(kept.all())
	if rec := call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup","to":"sent","update":true}`); rec.Code != 200 || rec.Body.String() != "{\"invites\":1,\"messages\":1}\n" {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	waitFor(kept, before+1)
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

// A family from outside goes on together: the one with the address, the
// others under it, one named with no address of their own under a key;
// they sit as one household on the list, are named together in the
// invitation, and each answers for the rest on their own page.
func TestOutsideFamily(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	rec := call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside","household":"`+coach+`"},{"email":"pat@example.org","name":"Pat Lee","via":"outside","household":"`+coach+`"},{"name":"Kit Lee","via":"outside","household":"`+coach+`"}]}`)
	if rec.Code != 200 || rec.Body.String() != "{\"added\":3,\"sent\":0}\n" {
		t.Fatalf("family: %d %s", rec.Code, rec.Body)
	}
	m := cache.Model()
	var kit string
	for _, inv := range m.Invites["meetup"] {
		if inv.Name == "Kit Lee" {
			kit = inv.Email
		}
	}
	if kit == "" || !isGuestKey(kit) || m.InviteOf("meetup", kit).Token != "" || m.InviteOf("meetup", kit).Household != coach || m.InviteOf("meetup", "pat@example.org").Token == "" {
		t.Fatalf("family rows = %+v", m.Invites["meetup"])
	}
	v := inviteView(t, jordan, "meetup")
	if rowOf(v, coach).Household != rowOf(v, "pat@example.org").Household || rowOf(v, kit).Household != rowOf(v, coach).Household {
		t.Errorf("not one household: %q %q %q", rowOf(v, coach).Household, rowOf(v, "pat@example.org").Household, rowOf(v, kit).Household)
	}
	// The invitation names the family; Kit, with no address, is sent none.
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 3)
	if msgs := mailTo(kept, coach); len(msgs) != 1 || !strings.Contains(msgs[0].HTML, "This email is for Coach, Pat, Kit.") || !strings.Contains(msgs[0].HTML, "Jordan Whitfield sent Coach an invitation for") || msgs[0].FromName != "Jordan Whitfield" {
		t.Errorf("the coach's invite: %+v", msgs)
	}
	invites := 0
	for _, msg := range kept.all() {
		if strings.Contains(msg.Subject, "You're invited!") {
			invites++
		}
	}
	if len(mailTo(kept, "pat@example.org")) != 1 || invites != 2 {
		t.Errorf("the family's invites: %d", invites)
	}
	// Pat's page shows the rest of the family, and answers for Kit -
	// and opening it is opening the invitation, noted once for the host.
	token := m.InviteOf("meetup", "pat@example.org").Token
	rec = call(t, mux, "GET", "/open/ext/"+token, "")
	if opened := cache.Model().InviteOf("meetup", "pat@example.org").Opened; opened == "" {
		t.Errorf("pat's page opened, not noted")
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), "pat@example.org"); r == nil || r.Opened == "" {
		t.Errorf("the host does not see pat opened: %+v", r)
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), coach); r == nil || r.Opened != "" {
		t.Errorf("the coach, who has not opened: %+v", r)
	}
	// Someone from the directory opens theirs on the event's page here; a
	// guest reads nobody's opening.
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	if v := inviteView(t, as(robin, mux), "meetup"); slices.ContainsFunc(v.Coming, func(g GuestRow) bool { return g.Opened != "" }) {
		t.Errorf("a guest reads who opened")
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), robin); r == nil || r.Opened == "" {
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
	if cache.Model().AnswerOf(kit, "meetup") != AnswerYes {
		t.Errorf("kit's answer = %q", cache.Model().AnswerOf(kit, "meetup"))
	}
}

// Someone a group put on and the host took off stays off: the sweep that
// keeps the group up to date, on opening the list and on its own, does
// not put them back, however well they still match - until the host adds
// them by hand, when they are on the list again as anyone is.
func TestRemovedStayRemoved(t *testing.T) {
	mux, cache, _, _, _ := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Moms","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"moms"}`)
	rec := call(t, jordan, "POST", "/api/calendar/invites/group", `{"id":"moms","rule":{"tags":["Carpool"]},"auto":false}`)
	var made struct {
		Group string
		Added int
	}
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Added != 2 {
		t.Fatalf("group: %d %s", rec.Code, rec.Body)
	}
	dropped := "daniel.park@heliosschool.org"
	if rec := call(t, jordan, "DELETE", "/api/calendar/invites/people", `{"id":"moms","email":"`+dropped+`"}`); rec.Code != 204 {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if g := cache.Model().GroupOf("moms", made.Group); !slices.Contains(g.Removed, dropped) {
		t.Errorf("the group does not remember the removal: %+v", g)
	}
	// The list opened again, and the sweep well past the grace: still off.
	start := now()
	now = func() time.Time { return start.Add(2*grace + time.Minute) }
	inviteView(t, jordan, "moms")
	inviteView(t, jordan, "moms")
	now = func() time.Time { return time.Now().In(Location) }
	if cache.Model().InviteOf("moms", dropped) != nil {
		t.Errorf("the group put %s back", dropped)
	}
	// Auto-invite on takes everyone matching at once - except them.
	call(t, jordan, "PUT", "/api/calendar/invites/group", `{"id":"moms","group":"`+made.Group+`","auto":true}`)
	if cache.Model().InviteOf("moms", dropped) != nil {
		t.Errorf("auto-invite put %s back", dropped)
	}
	// Added by hand, they are on again.
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"moms","people":[{"email":"`+dropped+`"}]}`)
	if cache.Model().InviteOf("moms", dropped) == nil {
		t.Errorf("adding %s by hand did not take", dropped)
	}
}

// An invitation is the invitee's own: a partner's alone puts nothing on
// the other's calendar and asks them nothing; a child's is the parent's
// too, who answers for them.
func TestInvitationIsPersonal(t *testing.T) {
	mux, cache, _, _, _ := invitesAppWith(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Moms","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"moms"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"moms","people":[{"email":"`+robin+`"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"moms"}`)
	m := cache.Model()
	on := func(email string) bool {
		return slices.ContainsFunc(m.eventsFor(email, nil), func(e *Event) bool { return e.ID == "moms" })
	}
	if !on(robin) || on(sam) || on(ella) {
		t.Errorf("robin's invitation on the calendars: robin %v, sam %v, ella %v", on(robin), on(sam), on(ella))
	}
	if v := inviteView(t, as(sam, mux), "moms"); len(v.Mine) != 0 {
		t.Errorf("sam asked about his mother's invitation: %+v", v.Mine)
	}
	// A child's reaches the parent.
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Kids","start":"2026-10-11 15:00","tags":[],"inviteOnly":true,"id":"kids"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"kids","people":[{"email":"`+sam+`"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"kids"}`)
	m = cache.Model()
	kids := func(email string) bool {
		return slices.ContainsFunc(m.eventsFor(email, nil), func(e *Event) bool { return e.ID == "kids" })
	}
	if !kids(sam) || !kids(robin) || kids(ella) {
		t.Errorf("sam's invitation on the calendars: sam %v, robin %v, ella %v", kids(sam), kids(robin), kids(ella))
	}
	if v := inviteView(t, as(robin, mux), "kids"); len(v.Mine) != 1 || v.Mine[0].Email != sam || !v.Mine[0].Mine {
		t.Errorf("robin's ask for sam's invitation: %+v", v.Mine)
	}
}

// Anyone the event is on the calendar of invites people one at a time:
// their rows say who invited them, and once the invitation is out they
// are sent it at once. A group is a host's alone; on a private event the
// link alone lets nobody invite.
func TestGuestsInvite(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`)
	// Mia has the link but no invitation: not hers to invite to.
	if rec := call(t, as(mia, mux), "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+coach+`","name":"Coach Lee","via":"outside"}]}`); rec.Code != 403 {
		t.Errorf("an outsider inviting: %d", rec.Code)
	}
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 2)
	// Robin, invited, invites Mia: on the list as Robin's doing, sent now.
	rec := call(t, as(robin, mux), "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+mia+`"}]}`)
	if rec.Code != 200 || rec.Body.String() != "{\"added\":1,\"sent\":1}\n" {
		t.Fatalf("robin inviting mia: %d %s", rec.Code, rec.Body)
	}
	inv := cache.Model().InviteOf("meetup", mia)
	if inv == nil || inv.Via != ViaInvited || inv.AddedBy != robin || inv.Sent == "" {
		t.Errorf("mia's row = %+v", inv)
	}
	waitFor(kept, 3)
	if msgs := mailTo(kept, mia); len(msgs) != 1 || !strings.Contains(msgs[0].HTML, "Robin Whitfield sent Mia an invitation for") {
		t.Errorf("mia's invite: %+v", msgs)
	}
	if r := rowOf(inviteView(t, jordan, "meetup"), mia); r == nil || r.InvitedBy != "Robin Whitfield" {
		t.Errorf("the host's row for mia: %+v", r)
	}
	// A group is a host's alone.
	if rec := call(t, as(robin, mux), "POST", "/api/calendar/invites/group", `{"id":"meetup","rule":{"tags":["Carpool"]}}`); rec.Code != 403 {
		t.Errorf("a guest adding a group: %d", rec.Code)
	}
}

// An HCA event's chair starts its guest list from HCA-Team's Create
// Invite as a party's host does from Celebrate's: the invitation, and a
// group for the event's volunteers - its list in Who? - with Auto-invite
// on; Team reads the answers back.
func TestTeamStart(t *testing.T) {
	mux, cache, _, _ := invitesApp(t)
	teamA := SourceTeam + "/e1"
	miaH := as(mia, mux)
	if rec := call(t, as(robin, mux), "POST", "/api/calendar/invites/start", `{"id":"`+teamA+`"}`); rec.Code != 403 {
		t.Errorf("a volunteer starting: %d", rec.Code)
	}
	rec := call(t, miaH, "POST", "/api/calendar/invites/start", `{"id":"`+teamA+`"}`)
	var made struct {
		Group string
		Added int
	}
	json.Unmarshal(rec.Body.Bytes(), &made)
	if rec.Code != 200 || made.Group == "" {
		t.Fatalf("start: %d %s", rec.Code, rec.Body)
	}
	g := cache.Model().GroupOf(teamA, made.Group)
	if g == nil || !g.Auto || strings.Join(g.Rule.Tags, ",") != "activity:e1" || g.Rule.Owner != mia || cache.Model().Invitations[teamA] == nil {
		t.Errorf("team group = %+v", g)
	}
	if sent, _, ok := cache.LinkedRSVPs(nil, SourceTeam, "e1"); !ok || sent {
		t.Errorf("rsvps before sending: ok %v sent %v", ok, sent)
	}
	v := inviteView(t, miaH, teamA)
	if !v.Host || !v.Linked || v.Party || v.Original == nil || len(v.Hosts) != 1 || v.Hosts[0].Email != mia {
		t.Errorf("chair's view = host %v linked %v party %v original %v hosts %+v", v.Host, v.Linked, v.Party, v.Original, v.Hosts)
	}
}

// A host who asks hears of each answer by email - their own choice, kept
// on the invitation per host - and not of one they gave themselves.
func TestNotifyHost(t *testing.T) {
	mux, cache, kept, _ := invitesApp(t)
	jordan := as(host, mux)
	call(t, jordan, "POST", "/api/calendar/events", `{"title":"Meetup","start":"2026-10-10 15:00","tags":[],"inviteOnly":true,"id":"meetup"}`)
	call(t, jordan, "POST", "/api/calendar/invites/people", `{"id":"meetup","people":[{"email":"`+robin+`"}]}`)
	call(t, jordan, "POST", "/api/calendar/invites/send", `{"id":"meetup"}`)
	waitFor(kept, 2)
	if inviteView(t, jordan, "meetup").NotifyMe {
		t.Errorf("notify on to start")
	}
	if rec := call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","notifyMe":true}`); rec.Code != 204 {
		t.Fatalf("notify me: %d %s", rec.Code, rec.Body)
	}
	if !inviteView(t, jordan, "meetup").NotifyMe || !slices.Contains(cache.Model().Invitations["meetup"].Notify, host) {
		t.Errorf("notify not kept: %+v", cache.Model().Invitations["meetup"].Notify)
	}
	before := len(mailTo(kept, host))
	call(t, as(robin, mux), "POST", "/api/calendar/rsvp", `{"id":"meetup","answer":"yes"}`)
	waitFor(kept, len(kept.all())+1)
	notes := mailTo(kept, host)
	if len(notes) != before+1 || notes[len(notes)-1].Subject != "[Meetup] Robin Whitfield said Yes" || !strings.Contains(notes[len(notes)-1].Text, "1 yes, 0 maybe, 0 no, 0 still to answer") {
		t.Errorf("the host's note: %+v", notes)
	}
	// The host's own answer for someone brings no note; off, nothing more.
	before = len(mailTo(kept, host))
	call(t, jordan, "POST", "/api/calendar/invites/answer", `{"id":"meetup","email":"`+robin+`","answer":"maybe"}`)
	call(t, jordan, "PUT", "/api/calendar/invites/settings", `{"id":"meetup","notifyMe":false}`)
	call(t, as(robin, mux), "POST", "/api/calendar/rsvp", `{"id":"meetup","answer":"no"}`)
	if len(mailTo(kept, host)) != before {
		t.Errorf("notes after: %d, before %d", len(mailTo(kept, host)), before)
	}
}
