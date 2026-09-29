package model

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
	"heliosian/internal/id"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const shell = "web/when/index.html"

var pages = []string{"/{$}", "/c/{token}", "/day/{date}", "/mine", "/mine/{list}", "/admin"}

var eventPages = []string{"/e/{id...}", "/events/{id...}"}

type calendarApp struct {
	cache        *CalendarCache
	pinned       *Calendar
	images       blob.Images
	directory    func() *Directory
	settings     func() *Config
	partiesCache *PartiesCache
	parties      func() *Parties
	activities   func() *Activities
	clock        func() time.Time
	lists        func(email string) []PickerList
	sources      func() AudienceSources
	search       imagesearch.Search
	queue        *store.Queue
	mail         CalendarMail
	style        *sharecard.Style
}

type CalendarDeps struct {
	Cache      *CalendarCache
	Images     blob.Images
	Directory  func() *Directory
	Settings   func() *Config
	Parties    *PartiesCache
	Activities *ActivitiesCache
	Lists      func(email string) []PickerList
	Sources    func() AudienceSources
	Search     imagesearch.Search
	Mail       CalendarMail
	Style      *sharecard.Style
	Queue      *store.Queue
}

func newCalendarApp(d CalendarDeps) calendarApp {
	d.Search.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	return calendarApp{
		cache: d.Cache, images: d.Images, directory: d.Directory, settings: d.Settings,
		partiesCache: d.Parties, parties: d.Parties.Model, activities: d.Activities.Model, clock: now,
		lists: d.Lists, sources: d.Sources, search: d.Search, queue: d.Queue, mail: d.Mail, style: d.Style,
	}
}

func RegisterCalendar(mux *http.ServeMux, d CalendarDeps) CalendarHooks {
	a := newCalendarApp(d)
	kick := make(chan struct{}, 1)
	d.Queue.OnSwap(func() {
		select {
		case kick <- struct{}{}:
		default:
		}
	})
	go a.deliverLoop(kick)
	for _, page := range pages {
		mux.HandleFunc("GET "+page, a.page)
	}
	for _, page := range eventPages {
		mux.HandleFunc("GET "+page, a.eventPage)
	}
	mux.HandleFunc("GET /api/apps/rsvp", serve.JSON(a.rsvps))
	mux.HandleFunc("GET /api/when/invites/options", serve.JSON(a.groupOptions))
	mux.HandleFunc("POST /api/when/invites/preview", serve.JSON(a.groupPreview))
	mux.HandleFunc("GET /ext/{token}", a.extPage)
	mux.HandleFunc("GET /open/ext/{token}", serve.JSON(a.extView))
	mux.HandleFunc("POST /open/ext/{token}", serve.JSON(a.extAnswer))
	mux.HandleFunc("POST /open/ext/{token}/guest", serve.JSON(a.extGuest))
	mux.HandleFunc("DELETE /open/ext/{token}/guest", serve.JSON(a.extRemoveGuest))
	a.search.Register(mux, "/api/when", a.images.Folder(), imagesearch.Members)
	mux.HandleFunc("GET /open/feed/{file}", a.feed)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id...}", a.shareCard)
	mux.HandleFunc("GET /open/flyer/{id...}", a.flyer)
	mux.HandleFunc("GET /open/banner/{id...}", a.banner)
	mux.HandleFunc("POST /hooks/replies/mime", a.replies)
	mux.HandleFunc("POST /hooks/events", a.deliveryEvents)
	answer := func(ctx context.Context, email, id, answer string) error {
		actor := a.as(mail.Normalize(email))
		return a.recordBy(ctx, actor, actor.Email, id, answer, ViaPage, true, false)
	}
	return CalendarHooks{Answer: answer, MakeDefault: a.makeDefault, RSVPs: serve.JSON(a.rsvps), cache: d.Cache, app: a}
}

func (a calendarApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a calendarApp) eventPage(w http.ResponseWriter, r *http.Request) {
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

func (a calendarApp) model() *Calendar {
	if a.pinned != nil {
		return a.pinned
	}
	return a.cache.Model()
}

func (a calendarApp) as(email string) access.Actor {
	return a.directory().ActorOf(email, a.cache.Held(email))
}

func (a calendarApp) actor(r *http.Request) access.Actor {
	return a.directory().Actor(r, a.cache.Held)
}

type feedBody struct {
	Token      string   `json:"token"`
	Name       string   `json:"name"`
	Emoji      string   `json:"emoji"`
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
}

type CalendarHooks struct {
	Answer      Answerer
	MakeDefault func(ctx context.Context, email, token string) error
	RSVPs       http.HandlerFunc
	cache       *CalendarCache
	app         calendarApp
}

func (a calendarApp) makeDefault(ctx context.Context, email, token string) error {
	actor := a.as(mail.Normalize(email))
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

func (a calendarApp) saveOrder(ctx context.Context, actor access.Actor, tokens []string, ops []store.Op) error {
	if err := a.cache.Commit(ctx, actor, ops...); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: feeds ordered", "actor", actor.Email, "order", strings.Join(tokens, ","))
	return nil
}

type tokensBody struct {
	Tokens []string `json:"tokens"`
}

func feedEmoji(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 12 {
		s = string(r[:12])
	}
	return s
}

type keywordsBody struct {
	ID       string   `json:"id"`
	Keywords []string `json:"keywords"`
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

func (a calendarApp) tellAdmins(ctx context.Context, by string, e *Event) error {
	admins := a.cache.Admins()
	if len(admins) == 0 {
		return nil
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
		return err
	}
	slog.InfoContext(ctx, "calendar: admins told", "event", e.ID, "to", len(admins))
	return nil
}

type moveBody struct {
	ID    string `json:"id"`
	Start string `json:"start"`
	End   string `json:"end"`
}

type viewBody struct {
	Classrooms []string `json:"classrooms"`
	Tags       []string `json:"tags"`
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

func (a calendarApp) feed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	model := a.model()
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

func (a calendarApp) linked(email string) []Linked {
	return LinkedEvents(a.directory(), a.parties(), a.activities(), email, a.clock())
}

func (a calendarApp) sourceID(source, key string) string {
	switch source {
	case SourceCelebrate:
		if p := a.parties().Party(key); p != nil {
			return p.ID
		}
	case SourceTeam:
		if act := a.activities().Activity(key); act != nil {
			return act.ID
		}
	}
	return ""
}

func (a calendarApp) moveEverywhere(ctx context.Context, actor access.Actor, old, to, name string) error {
	as := a.directory().ActorOf(actor.Email, a.partiesCache.Held(actor.Email))
	if err := a.partiesCache.moveAddress(ctx, as, old, to, name); err != nil {
		return err
	}
	a.moveAddress(ctx, actor, old, to, name)
	return nil
}
