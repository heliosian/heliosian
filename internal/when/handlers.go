package when

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/id"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const shell = "web/when/index.html"

var pages = []string{"/{$}", "/c/{token}", "/day/{date}", "/mine", "/mine/{list}", "/admin"}

var eventPages = []string{"/e/{id...}", "/events/{id...}"}

type app struct {
	cache     *Cache
	images    blob.Images
	directory func() *who.Model
	settings  func() *config.Settings
	lists     func(email string) []List
	linked    func(email string) []Linked
	sourceID  func(source, key string) string
	parties   func(id string) *PartyPeople
	celebrate Celebrate
	sources   func() filter.Sources
	clock     *matchClock
	search    imagesearch.Search
	mail      Mail
	style     *sharecard.Style
}

type Deps struct {
	Cache     *Cache
	Images    blob.Images
	Directory func() *who.Model
	Settings  func() *config.Settings
	Lists     func(email string) []List
	Linked    func(email string) []Linked
	SourceID  func(source, key string) string
	Celebrate Celebrate
	Sources   func() filter.Sources
	Search    imagesearch.Search
	Mail      Mail
	Style     *sharecard.Style
}

func Register(mux *http.ServeMux, d Deps) Hooks {
	d.Search.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	a := app{cache: d.Cache, images: d.Images, directory: d.Directory, settings: d.Settings, lists: d.Lists, linked: d.Linked, sourceID: d.SourceID, parties: d.Celebrate.Party, celebrate: d.Celebrate, sources: d.Sources, clock: &matchClock{}, search: d.Search, mail: d.Mail, style: d.Style}
	go a.sweepLoop()
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	for _, page := range eventPages {
		mux.HandleFunc("GET "+page, a.eventPage)
	}
	mux.HandleFunc("GET /api/when/model", serve.JSON(a.model))
	mux.HandleFunc("GET /api/apps/rsvp", serve.JSON(a.rsvps))
	mux.HandleFunc("POST /api/when/feeds", serve.JSON(a.addFeed))
	mux.HandleFunc("PUT /api/when/feeds", serve.JSON(a.editFeed))
	mux.HandleFunc("POST /api/when/feeds/my-heliosian", serve.JSON(a.myHeliosianToken))
	mux.HandleFunc("PUT /api/when/feeds/order", serve.JSON(a.orderFeeds))
	mux.HandleFunc("POST /api/when/default", serve.JSON(a.setDefault))
	mux.HandleFunc("DELETE /api/when/feeds", serve.JSON(a.removeFeed))
	mux.HandleFunc("POST /api/when/rsvp", serve.JSON(a.rsvp))
	mux.HandleFunc("GET /api/when/invites", serve.JSON(a.invitesView))
	mux.HandleFunc("GET /api/when/invites/people", serve.JSON(a.invitePeople))
	mux.HandleFunc("POST /api/when/invites/people", serve.JSON(a.addInvites))
	mux.HandleFunc("DELETE /api/when/invites/people", serve.JSON(a.removeInvite))
	mux.HandleFunc("PUT /api/when/invites/settings", serve.JSON(a.inviteSettings))
	mux.HandleFunc("POST /api/when/invites/step-down", serve.JSON(a.stepDown))
	mux.HandleFunc("POST /api/when/invites/guest", serve.JSON(a.addGuest))
	mux.HandleFunc("POST /api/when/invites/answer", serve.JSON(a.answerFor))
	mux.HandleFunc("POST /api/when/invites/send", serve.JSON(a.sendInvites))
	mux.HandleFunc("POST /api/when/invites/skip", serve.JSON(a.skipInvites))
	mux.HandleFunc("POST /api/when/invites/email", serve.JSON(a.changeInviteEmail))
	mux.HandleFunc("POST /api/when/invites/delete", serve.JSON(a.deleteInvitation))
	mux.HandleFunc("POST /api/when/events/cancel", serve.JSON(a.cancelEvent))
	mux.HandleFunc("POST /api/when/invites/message", serve.JSON(a.messageInvites))
	mux.HandleFunc("GET /api/when/invites/options", serve.JSON(a.groupOptions))
	mux.HandleFunc("POST /api/when/invites/preview", serve.JSON(a.groupPreview))
	mux.HandleFunc("POST /api/when/invites/group", serve.JSON(a.addGroup))
	mux.HandleFunc("PUT /api/when/invites/group", serve.JSON(a.setGroup))
	mux.HandleFunc("DELETE /api/when/invites/group", serve.JSON(a.removeGroup))
	mux.HandleFunc("POST /api/when/invites/start", serve.JSON(a.startParty))
	mux.HandleFunc("GET /ext/{token}", a.extPage)
	mux.HandleFunc("GET /open/ext/{token}", serve.JSON(a.extView))
	mux.HandleFunc("POST /open/ext/{token}", serve.JSON(a.extAnswer))
	mux.HandleFunc("POST /open/ext/{token}/guest", serve.JSON(a.extGuest))
	mux.HandleFunc("DELETE /open/ext/{token}/guest", serve.JSON(a.extRemoveGuest))
	mux.HandleFunc("POST /api/when/settings", serve.JSON(a.saveSetting))
	mux.HandleFunc("POST /api/when/keywords", serve.JSON(a.setKeywords))
	mux.HandleFunc("PUT /api/when/overrides", serve.JSON(a.setOverride))
	mux.HandleFunc("PUT /api/when/overrides/image", serve.JSON(a.setOverrideImage))
	mux.HandleFunc("POST /api/when/events", serve.JSON(a.addEvents))
	mux.HandleFunc("PUT /api/when/events", serve.JSON(a.editEvent))
	mux.HandleFunc("GET /api/when/event", serve.JSON(a.oneEvent))
	mux.HandleFunc("POST /api/when/events/approve", serve.JSON(a.approveEvent))
	mux.HandleFunc("POST /api/when/events/decline", serve.JSON(a.declineEvent))
	mux.HandleFunc("POST /api/when/events/when", serve.JSON(a.moveEvent))
	mux.HandleFunc("DELETE /api/when/settings", serve.JSON(a.forgetSetting))
	mux.HandleFunc("POST /api/when/tags", serve.JSON(a.setTags))
	a.search.Register(mux, "/api/when", a.images.Folder(), imagesearch.Members)
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
	return Hooks{Answer: answer, MakeDefault: a.makeDefault, RSVPs: serve.JSON(a.rsvps), MoveAddress: a.moveAddress}
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) eventPage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("id")
	canonical := a.canonical(key)
	if canonical == key {
		serve.File(w, r, shell)
		return
	}
	e := a.anyEvent("", canonical)
	if e == nil {
		serve.File(w, r, shell)
		return
	}
	to := EventPath(e)
	if r.URL.RawQuery != "" {
		to += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, to, http.StatusMovedPermanently)
}

var now = func() time.Time {
	return time.Now().In(Location)
}

func (a app) model(r *http.Request, _ serve.None) (View, error) {
	actor := a.actor(r)
	email := actor.Email
	directory := a.directory()
	model := a.cache.Model()
	view := Render(model, directory, a.settings(), actor, now(), a.linked(email))
	for i, e := range view.Events {
		hosted := model.hostedBy(directory, email, e)
		if !hosted && len(e.Hosts) == 0 {
			continue
		}
		c := *e
		c.Hosted = hosted
		for _, h := range e.Hosts {
			if p := directory.Person(directory.Resolve(config.NormalizeEmail(h))); p != nil && p.FullName != "" {
				c.HostNames = append(c.HostNames, p.FullName)
			}
		}
		view.Events[i] = &c
	}
	view.ImageSearch = a.search.On()
	return view, nil
}

func (a app) as(email string) access.Actor {
	return a.directory().ActorOf(email, a.cache.Held(email))
}

func (a app) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
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

func (a app) addFeed(r *http.Request, body feedBody) (map[string]string, error) {
	actor := a.actor(r)
	ops, cells := a.newFeed(actor, body)
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "calendar: feed added", "actor", actor.Email, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	return map[string]string{"token": cells["Token"], "url": feedURL(r, cells["Token"])}, nil
}

func (a app) editFeed(r *http.Request, body feedBody) (serve.None, error) {
	actor := a.actor(r)
	ops, cells, err := a.changeFeed(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	if cells != nil {
		slog.InfoContext(r.Context(), "calendar: feed changed", "actor", actor.Email, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	}
	return serve.None{}, nil
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

type tokenBody struct {
	Token string `json:"token"`
}

func (a app) setDefault(r *http.Request, body tokenBody) (serve.None, error) {
	actor := a.actor(r)
	return serve.None{}, a.makeDefault(r.Context(), actor.Email, strings.TrimSpace(body.Token))
}

func (a app) saveOrder(ctx context.Context, actor access.Actor, tokens []string, ops []store.Op) error {
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: feeds ordered", "actor", actor.Email, "order", strings.Join(tokens, ","))
	return nil
}

type tokensBody struct {
	Tokens []string `json:"tokens"`
}

func (a app) orderFeeds(r *http.Request, body tokensBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.orderOps(actor, body.Tokens)
	if err != nil {
		return serve.None{}, err
	}
	return serve.None{}, a.saveOrder(r.Context(), actor, body.Tokens, ops)
}

func feedEmoji(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 12 {
		s = string(r[:12])
	}
	return s
}

func (a app) removeFeed(r *http.Request, body tokenBody) (serve.None, error) {
	actor := a.actor(r)
	ops, name, err := a.dropFeed(actor, body.Token)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: feed removed", "actor", actor.Email, "name", name)
	return serve.None{}, nil
}

type keywordsBody struct {
	ID       string   `json:"id"`
	Keywords []string `json:"keywords"`
}

func (a app) setKeywords(r *http.Request, body keywordsBody) (serve.None, error) {
	actor := a.actor(r)
	if err := adminOnly(actor); err != nil {
		return serve.None{}, err
	}
	ops, e, cell, err := a.keywordOps(actor, body.ID, body.Keywords)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: keywords set", "actor", actor.Email, "event", e.ID, "keywords", cell)
	return serve.None{}, nil
}

var addressForm = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$`)

func validAddress(address string) bool {
	_, isID := id.Parse(address)
	return addressForm.MatchString(address) && !isID
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
	Address     string   `json:"address"`
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

func (a app) addEvents(r *http.Request, body eventBody) (map[string]any, error) {
	actor := a.actor(r)
	ops, ids, pending, err := a.newEvents(actor, body)
	if err != nil {
		return nil, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return nil, err
	}
	slog.InfoContext(r.Context(), "calendar: events added", "actor", actor.Email, "title", strings.TrimSpace(body.Title), "count", len(ids), "pending", pending)
	for _, id := range ids {
		if err := a.recordBy(r.Context(), actor, actor.Email, id, AnswerYes, ViaPage, false, false); err != nil {
			slog.WarnContext(r.Context(), "calendar: host's yes", "event", id, "error", err)
		}
	}
	if e := a.cache.Model().Event(ids[0]); e != nil {
		go a.tellAdmins(context.WithoutCancel(r.Context()), actor.Email, e)
	}
	return map[string]any{"ids": ids, "pending": pending}, nil
}

type oneEventView struct {
	*Event
	AdminOnly bool `json:"adminOnly,omitempty"`
}

func (a app) oneEvent(r *http.Request, _ serve.None) (oneEventView, error) {
	actor := a.actor(r)
	e := a.eventFor(actor, strings.TrimSpace(r.URL.Query().Get("id")))
	if e == nil {
		return oneEventView{}, access.Missing("404 page not found")
	}
	return oneEventView{e, actor.May(SeeAll) && a.eventFor(access.Actor{Email: actor.Email}, e.ID) == nil}, nil
}

func (a app) tellAdmins(ctx context.Context, by string, e *Event) {
	admins := a.cache.Admins()
	if len(admins) == 0 {
		return
	}
	who := by
	if p := a.directory().Person(by); p != nil && p.FullName != "" {
		who = p.FullName + " (" + by + ")"
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

func (a app) editEvent(r *http.Request, body eventBody) (serve.None, error) {
	actor := a.actor(r)
	ops, e, cells, err := a.changeEvent(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: event changed", "actor", actor.Email, "event", e.ID, "title", cells["Title"])
	if cells["Status"] == StatusPending {
		if changed := a.cache.Model().Event(e.ID); changed != nil {
			go a.tellAdmins(context.WithoutCancel(r.Context()), actor.Email, changed)
		}
	}
	return serve.None{}, nil
}

type idBody struct {
	ID string `json:"id"`
}

func (a app) approveEvent(r *http.Request, body idBody) (serve.None, error) {
	return a.setStatus(r, body, StatusApproved, "approved")
}

func (a app) declineEvent(r *http.Request, body idBody) (serve.None, error) {
	return a.setStatus(r, body, StatusDeclined, "declined")
}

func (a app) setStatus(r *http.Request, body idBody, status, did string) (serve.None, error) {
	actor := a.actor(r)
	if err := adminOnly(actor); err != nil {
		return serve.None{}, err
	}
	ops, e, err := a.statusOps(actor, body.ID, status)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: event "+did, "actor", actor.Email, "event", e.ID, "title", e.Title, "by", e.AddedBy)
	return serve.None{}, nil
}

type moveBody struct {
	ID    string `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
}

func (a app) moveEvent(r *http.Request, body moveBody) (serve.None, error) {
	actor := a.actor(r)
	if err := adminOnly(actor); err != nil {
		return serve.None{}, err
	}
	start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	if end == "" {
		end = start
	}
	ops, e, err := a.moveOps(actor, body.ID, start, end)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: event moved", "actor", actor.Email, "event", e.ID, "start", start, "end", end)
	return serve.None{}, nil
}

type viewBody struct {
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
}

func (a app) saveSetting(r *http.Request, body viewBody) (serve.None, error) {
	actor := a.actor(r)
	ops, cells := saveViewOps(actor, body.Classrooms, body.Tags)
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: view saved", "actor", actor.Email, "classrooms", cells["Classrooms"], "tags", cells["Categories"])
	return serve.None{}, nil
}

func (a app) forgetSetting(r *http.Request, _ serve.None) (serve.None, error) {
	actor := a.actor(r)
	ops := a.forgetViewOps(actor)
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: view forgotten", "actor", actor.Email)
	return serve.None{}, nil
}

type tagBody struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group"`
	Default     bool   `json:"default"`
	Image       string `json:"image"`
}

type tagsBody struct {
	Tags []tagBody `json:"tags"`
}

func (a app) setTags(r *http.Request, body tagsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, added, err := a.tagOps(actor, body.Tags)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: categories saved", "actor", actor.Email, "added", added)
	return serve.None{}, nil
}

func (a app) feed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	model := a.cache.Model()
	f := model.Feed(token)
	if email := model.myHeliosianFeed(token); f == nil && email != "" {
		home := model.MyHeliosian(email)
		home.Token = token
		home.Classrooms, home.Tags = model.myHeliosianView(a.directory(), email)
		f = &home
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "helios-calendar.ics"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(ICS(model, a.directory(), f, a.linked(f.Email), "https://"+r.Host, now()))
}

func (a app) myHeliosianToken(r *http.Request, _ serve.None) (tokenBody, error) {
	actor := a.actor(r)
	ops, token := a.feedTokenOps(actor)
	if len(ops) > 0 {
		if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
			return tokenBody{}, err
		}
		slog.InfoContext(r.Context(), "calendar: my heliosian feed made", "actor", actor.Email)
	}
	return tokenBody{Token: token}, nil
}
