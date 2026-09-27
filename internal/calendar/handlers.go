package calendar

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const shell = "web/calendar/index.html"

var pages = []string{"/{$}", "/c/{token}", "/day/{date}", "/e/{id...}", "/events/{id...}", "/mine", "/mine/{list}", "/admin"}

type app struct {
	cache       *Cache
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	linked      func(email string) []Linked
	parties     func(id string) *PartyPeople
	celebrate   Celebrate
	sources     func() filter.Sources
	clock       *matchClock
	search      ImageSearch
	mail        Mail
	style       *sharecard.Style
}

type ImageSearch = imagesearch.Search

const imageFolder = "category-images"

func Register(mux *http.ServeMux, cache *Cache, store *blob.Store, directory Directory, superAdmins func() []string, linked func(email string) []Linked, celebrate Celebrate, sources func() filter.Sources, search ImageSearch, mailbox Mail, style *sharecard.Style) Hooks {
	if search.UserAgent == "" {
		search.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	}
	a := app{cache: cache, store: store, directory: directory, superAdmins: superAdmins, linked: linked, parties: celebrate.Party, celebrate: celebrate, sources: sources, clock: &matchClock{}, search: search, mail: mailbox, style: style}
	if sources != nil {
		go a.sweepLoop()
	}
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	mux.HandleFunc("GET /api/calendar/model", a.model)
	mux.HandleFunc("GET /api/apps/rsvp", a.rsvps)
	mux.HandleFunc("POST /api/calendar/feeds", a.addFeed)
	mux.HandleFunc("PUT /api/calendar/feeds", a.editFeed)
	mux.HandleFunc("POST /api/calendar/feeds/my-heliosian", a.myHeliosianToken)
	mux.HandleFunc("PUT /api/calendar/feeds/order", a.orderFeeds)
	mux.HandleFunc("POST /api/calendar/default", a.setDefault)
	mux.HandleFunc("DELETE /api/calendar/feeds", a.removeFeed)
	mux.HandleFunc("POST /api/calendar/rsvp", a.rsvp)
	mux.HandleFunc("GET /api/calendar/invites", a.invitesView)
	mux.HandleFunc("GET /api/calendar/invites/people", a.invitePeople)
	mux.HandleFunc("POST /api/calendar/invites/people", a.addInvites)
	mux.HandleFunc("DELETE /api/calendar/invites/people", a.removeInvite)
	mux.HandleFunc("PUT /api/calendar/invites/settings", a.inviteSettings)
	mux.HandleFunc("POST /api/calendar/invites/step-down", a.stepDown)
	mux.HandleFunc("POST /api/calendar/invites/guest", a.addGuest)
	mux.HandleFunc("POST /api/calendar/invites/answer", a.answerFor)
	mux.HandleFunc("POST /api/calendar/invites/send", a.sendInvites)
	mux.HandleFunc("POST /api/calendar/invites/skip", a.skipInvites)
	mux.HandleFunc("POST /api/calendar/invites/email", a.changeInviteEmail)
	mux.HandleFunc("POST /api/calendar/invites/delete", a.deleteInvitation)
	mux.HandleFunc("POST /api/calendar/events/cancel", a.cancelEvent)
	mux.HandleFunc("POST /api/calendar/invites/message", a.messageInvites)
	mux.HandleFunc("GET /api/calendar/invites/options", a.groupOptions)
	mux.HandleFunc("POST /api/calendar/invites/preview", a.groupPreview)
	mux.HandleFunc("POST /api/calendar/invites/group", a.addGroup)
	mux.HandleFunc("PUT /api/calendar/invites/group", a.setGroup)
	mux.HandleFunc("DELETE /api/calendar/invites/group", a.removeGroup)
	mux.HandleFunc("POST /api/calendar/invites/start", a.startParty)
	mux.HandleFunc("GET /ext/{token}", a.extPage)
	mux.HandleFunc("GET /open/ext/{token}", a.extView)
	mux.HandleFunc("POST /open/ext/{token}", a.extAnswer)
	mux.HandleFunc("POST /open/ext/{token}/guest", a.extGuest)
	mux.HandleFunc("DELETE /open/ext/{token}/guest", a.extRemoveGuest)
	mux.HandleFunc("POST /api/calendar/settings", a.saveSetting)
	mux.HandleFunc("POST /api/calendar/keywords", a.admin(a.setKeywords))
	mux.HandleFunc("PUT /api/calendar/overrides", a.admin(a.setOverride))
	mux.HandleFunc("PUT /api/calendar/overrides/image", a.admin(a.setOverrideImage))
	mux.HandleFunc("POST /api/calendar/events", a.addEvents)
	mux.HandleFunc("PUT /api/calendar/events", a.editEvent)
	mux.HandleFunc("GET /api/calendar/event", a.oneEvent)
	mux.HandleFunc("POST /api/calendar/events/approve", a.admin(a.approveEvent))
	mux.HandleFunc("POST /api/calendar/events/decline", a.admin(a.declineEvent))
	mux.HandleFunc("POST /api/calendar/events/when", a.admin(a.moveEvent))
	mux.HandleFunc("DELETE /api/calendar/settings", a.forgetSetting)
	mux.HandleFunc("POST /api/calendar/tags", a.setTags)
	a.search.Register(mux, "/api/calendar", imageFolder, imagesearch.Members)
	mux.HandleFunc("GET /open/feed/{file}", a.feed)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id...}", a.shareCard)
	mux.HandleFunc("GET /open/flyer/{id...}", a.flyer)
	mux.HandleFunc("GET /open/banner/{id...}", a.banner)
	mux.HandleFunc("POST /hooks/replies/mime", a.replies)
	mux.HandleFunc("POST /hooks/events", a.deliveryEvents)
	answer := func(ctx context.Context, email, id, answer string) error {
		actor := a.as(config.NormalizeEmail(email))
		return a.recordBy(ctx, actor, actor.Email, id, answer, ViaPage, true, false)
	}
	return Hooks{Answer: answer, MakeDefault: a.makeDefault, RSVPs: a.rsvps, MoveAddress: a.moveAddress}
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) who(r *http.Request) (string, bool) {
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return email, a.cache.IsAdmin(email)
}

var now = func() time.Time {
	return time.Now().In(Location)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	view := Render(a.cache.Model(), a.directory, access.Actor{Email: email, Admin: admin}, now(), a.linked(email))
	for i, e := range view.Events {
		hosted := (e.Source == SourceSheet || e.linked() || e.imported()) && a.isHost(access.Actor{Email: email}, e)
		if !hosted && len(e.Hosts) == 0 {
			continue
		}
		c := *e
		c.Hosted = hosted
		for _, h := range e.Hosts {
			if p, known := a.directory.Person(a.directory.Resolve(config.NormalizeEmail(h))); known && p.Name != "" {
				c.HostNames = append(c.HostNames, p.Name)
			}
		}
		view.Events[i] = &c
	}
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode calendar model", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (a app) as(email string) access.Actor {
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email), Household: a.directory.Family(email)}
}

func (a app) actor(r *http.Request) access.Actor {
	email, _ := a.who(r)
	return a.as(email)
}

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		refuse(w, err)
		return false
	}
	return true
}

func NewToken() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func feedURL(r *http.Request, token string) string {
	return "https://" + r.Host + "/open/feed/" + token + ".ics"
}

type feedBody struct {
	Token      string   `json:"token"`
	Name       string   `json:"name"`
	Emoji      string   `json:"emoji"`
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
}

func (a app) addFeed(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body feedBody
	if !decode(w, r, &body) {
		return
	}
	ops, cells := a.newFeed(actor, body)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed added", "actor", actor.Email, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": cells["Token"], "url": feedURL(r, cells["Token"])})
}

func (a app) editFeed(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body feedBody
	if !decode(w, r, &body) {
		return
	}
	ops, cells, err := a.changeFeed(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	if cells != nil {
		slog.InfoContext(r.Context(), "calendar: feed changed", "actor", actor.Email, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	}
	w.WriteHeader(http.StatusNoContent)
}

type Hooks struct {
	Answer      Answerer
	MakeDefault func(ctx context.Context, email, token string) error
	RSVPs       http.HandlerFunc
	MoveAddress func(ctx context.Context, actor access.Actor, old, to, name string)
}

func (a app) makeDefault(ctx context.Context, email, token string) error {
	actor := a.as(config.NormalizeEmail(email))
	ops, tokens, err := a.defaultOps(actor, token)
	if err != nil {
		return err
	}
	if err := a.saveOrder(ctx, actor, tokens, ops); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: default calendar set", "actor", actor.Email, "token", token)
	return nil
}

func (a app) setDefault(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.makeDefault(r.Context(), actor.Email, strings.TrimSpace(body.Token)); err != nil {
		refuse(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveOrder(ctx context.Context, actor access.Actor, tokens []string, ops []store.Op) error {
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: feeds ordered", "actor", actor.Email, "order", strings.Join(tokens, ","))
	return nil
}

func (a app) orderFeeds(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, err := a.orderOps(actor, body.Tokens)
	if err != nil {
		refuse(w, err)
		return
	}
	if err := a.saveOrder(r.Context(), actor, body.Tokens, ops); err != nil {
		refuse(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func feedEmoji(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 12 {
		s = string(r[:12])
	}
	return s
}

func (a app) removeFeed(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, name, err := a.dropFeed(actor, body.Token)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed removed", "actor", actor.Email, "name", name)
	w.WriteHeader(http.StatusNoContent)
}

func yesNoWord(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func (a app) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, admin := a.who(r); !admin {
			http.Error(w, "only a calendar admin can do that", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (a app) setKeywords(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		ID       string   `json:"id"`
		Keywords []string `json:"keywords"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, e, cell, err := a.keywordOps(actor, body.ID, body.Keywords)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: keywords set", "actor", actor.Email, "event", e.ID, "keywords", cell)
	w.WriteHeader(http.StatusNoContent)
}

var eventIDForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

func newEventID() string {
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ0123456789"
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(out)
}

func shiftWhen(when string, weeks int) string {
	if when == "" {
		return ""
	}
	layout := DateFormat
	if len(when) > len(DateFormat) {
		layout = DateTimeFormat
	}
	t, err := time.ParseInLocation(layout, when, Location)
	if err != nil {
		return when
	}
	return t.AddDate(0, 0, 7*weeks).Format(layout)
}

type eventBody struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Location    string   `json:"location"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	DayType     string   `json:"dayType"`
	Keywords    []string `json:"keywords"`
	Source      string   `json:"source"`
	Image       string   `json:"image"`
	Sharing     string   `json:"sharing"`
	RepeatWeeks int      `json:"repeatWeeks"`
	RepeatTimes int      `json:"repeatTimes"`
}

func (a app) addEvents(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body eventBody
	if !decode(w, r, &body) {
		return
	}
	ops, ids, pending, err := a.newEvents(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: events added", "actor", actor.Email, "title", strings.TrimSpace(body.Title), "count", len(ids), "pending", pending)
	for _, id := range ids {
		if err := a.recordBy(r.Context(), actor, actor.Email, id, AnswerYes, ViaPage, false, false); err != nil {
			slog.WarnContext(r.Context(), "calendar: host's yes", "event", id, "error", err)
		}
	}
	if a.mail.Sender != nil {
		if e := a.cache.Model().Event(ids[0]); e != nil {
			go a.tellAdmins(context.WithoutCancel(r.Context()), actor.Email, e)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ids": ids, "pending": pending})
}

func (a app) oneEvent(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	e := a.eventFor(actor, admin, strings.TrimSpace(r.URL.Query().Get("id")))
	if e == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(struct {
		*Event
		AdminOnly bool `json:"adminOnly,omitempty"`
	}{e, admin && a.eventFor(actor, false, e.ID) == nil})
}

func (a app) tellAdmins(ctx context.Context, by string, e *Event) {
	admins := a.cache.Admins(a.superAdmins())
	if len(admins) == 0 {
		return
	}
	who := by
	if p, ok := a.directory.Person(by); ok && p.Name != "" {
		who = p.Name + " (" + by + ")"
	}
	day, _ := whenLines(e)
	lead := who + " shared an event on Helios When that is waiting for approval."
	closing := "Approve and Decline are at the top of its page. Until then it is shared by link: anyone with its link can open it."
	subject := "Event to approve: "
	if e.Sharing == SharingLink {
		lead = who + " added an event shared by link on Helios When. It needs no approval: it is not on the calendar, only on the calendars of the people they invite and of those they send the link to who answer it."
		closing = "Nothing is needed from you; this is so the admins know what is being shared."
		subject = "Link event added: "
	}
	if e.Sharing == SharingInvited {
		lead = who + " added an invite-only event on Helios When. It needs no approval: it is not on the calendar, only on the calendars of the people they invite."
		closing = "Nothing is needed from you; this is so the admins know what is being shared."
		subject = "Invite-only event added: "
	}
	l := a.letterFor(e, EventPath(e))
	l.Heading = strings.TrimSuffix(subject, ": ")
	l.Intro = lead
	if e.Description != "" {
		l.Rows = [][2]string{{"Details", e.Description}}
	}
	l.Button = "See the event"
	if e.Sharing == SharingPublic {
		l.Button = "Review the event"
	}
	l.Footnote = closing
	if err := a.mail.Sender.Send(ctx, l.Message(subject+e.Title+" · "+day, admins, nil, nil)); err != nil {
		slog.ErrorContext(ctx, "calendar: tell admins", "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: admins told", "event", e.ID, "to", len(admins))
}

func (a app) editEvent(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body eventBody
	if !decode(w, r, &body) {
		return
	}
	ops, e, cells, err := a.changeEvent(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event changed", "actor", actor.Email, "event", e.ID, "title", cells["Title"])
	if cells["Status"] == StatusPending && a.mail.Sender != nil {
		if changed := a.cache.Model().Event(e.ID); changed != nil {
			go a.tellAdmins(context.WithoutCancel(r.Context()), actor.Email, changed)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) approveEvent(w http.ResponseWriter, r *http.Request) {
	a.setStatus(w, r, StatusApproved, "approved")
}

func (a app) declineEvent(w http.ResponseWriter, r *http.Request) {
	a.setStatus(w, r, StatusDeclined, "declined")
}

func (a app) setStatus(w http.ResponseWriter, r *http.Request, status, did string) {
	actor := a.actor(r)
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, e, err := a.statusOps(actor, body.ID, status)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event "+did, "actor", actor.Email, "event", e.ID, "title", e.Title, "by", e.AddedBy)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) moveEvent(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		ID    string `json:"id"`
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if !decode(w, r, &body) {
		return
	}
	start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	if end == "" {
		end = start
	}
	ops, e, err := a.moveOps(actor, body.ID, start, end)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event moved", "actor", actor.Email, "event", e.ID, "start", start, "end", end)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSetting(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, cells := saveViewOps(actor, body.Classrooms, body.Tags)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view saved", "actor", actor.Email, "classrooms", cells["Classrooms"], "tags", cells["Categories"])
	w.WriteHeader(http.StatusNoContent)
}

func (a app) forgetSetting(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	ops := a.forgetViewOps(actor)
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view forgotten", "actor", actor.Email)
	w.WriteHeader(http.StatusNoContent)
}

type tagBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
	Image       string `json:"image"`
}

func (a app) setTags(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	var body struct {
		Tags []tagBody `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, added, err := a.tagOps(actor, body.Tags)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: categories saved", "actor", actor.Email, "added", added)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) feed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	model := a.cache.Model()
	f := model.Feed(token)
	if email := model.myHeliosianFeed(token); f == nil && email != "" {
		home := model.MyHeliosian(email)
		home.Token = token
		home.Classrooms, home.Tags = model.myHeliosianView(a.directory, email)
		f = &home
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "helios-calendar.ics"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(ICS(model, a.directory, f, a.linked(f.Email), "https://"+r.Host, now()))
}

func (a app) myHeliosianToken(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	ops, token := a.feedTokenOps(actor)
	if len(ops) > 0 {
		if !a.commit(w, r, actor, ops...) {
			return
		}
		slog.InfoContext(r.Context(), "calendar: my heliosian feed made", "actor", actor.Email)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token})
}
