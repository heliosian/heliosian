package calendar

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/filter"
	"heliosian/internal/imagesearch"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const shell = "web/calendar/index.html"

var pages = []string{"/{$}", "/c/{token}", "/day/{date}", "/e/{id...}", "/events/{id...}", "/feeds", "/mine", "/mine/{list}", "/admin"}

type app struct {
	cache       *Cache
	store       *blob.Store
	directory   Directory
	superAdmins func() []string
	linked      func(email string) []Linked
	parties     func(id string) *PartyPeople
	sources     func() filter.Sources
	clock       *matchClock
	search      ImageSearch
	mail        Mail
}

type ImageSearch = imagesearch.Search

const (
	imageFolder  = "category-images"
	maxImageSize = 8 << 20
)

func Register(mux *http.ServeMux, cache *Cache, store *blob.Store, directory Directory, superAdmins func() []string, linked func(email string) []Linked, parties func(id string) *PartyPeople, sources func() filter.Sources, search ImageSearch, mailbox Mail) Hooks {
	if search.UserAgent == "" {
		search.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	}
	a := app{cache: cache, store: store, directory: directory, superAdmins: superAdmins, linked: linked, parties: parties, sources: sources, clock: &matchClock{}, search: search, mail: mailbox}
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
	mux.HandleFunc("POST /api/calendar/image", a.uploadImage)
	mux.HandleFunc("GET /api/calendar/images/search", a.search.ServeSearch)
	mux.HandleFunc("GET /api/calendar/images/thumb", a.search.ServeThumb)
	mux.HandleFunc("POST /api/calendar/images/import", a.importImage)
	mux.HandleFunc("GET /open/feed/{file}", a.feed)
	mux.HandleFunc("GET /open/share/upcoming.png", a.shareUpcoming)
	mux.HandleFunc("GET /open/share/{id...}", a.shareCard)
	mux.HandleFunc("GET /open/flyer/{id...}", a.flyer)
	mux.HandleFunc("GET /open/banner/{id...}", a.banner)
	mux.HandleFunc("POST /hooks/replies/mime", a.replies)
	mux.HandleFunc("POST /hooks/events", a.deliveryEvents)
	return Hooks{Answer: a.answer, MakeDefault: a.makeDefault, RSVPs: a.rsvps}
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
	view := Render(a.cache.Model(), a.directory, email, admin, now(), a.linked(email))
	for i, e := range view.Events {
		hosted := (e.Source == SourceSheet || e.linked() || e.imported()) && a.isHost(email, false, e)
		if !hosted && len(e.Hosts) == 0 {
			continue
		}
		c := *e
		c.Hosted = hosted
		for _, h := range e.Hosts {
			if p, known := a.directory.Person(a.directory.Resolve(normalizeEmail(h))); known && p.Name != "" {
				c.HostNames = append(c.HostNames, p.Name)
			}
		}
		view.Events[i] = &c
	}
	view.ImageSearch = a.search.On()
	view.User.IsSuperAdmin = a.cache.IsSuperAdmin(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] encode calendar model", "error", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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

func (a app) addFeed(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Name       string   `json:"name"`
		Emoji      string   `json:"emoji"`
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	token := NewToken()
	cells := store.Row{
		"Token": token, "Email": actor, "Name": a.cache.Model().unusedFeedName(actor, strings.TrimSpace(body.Name)), "Emoji": feedEmoji(body.Emoji),
		"Classrooms": JoinList(SplitList(JoinList(body.Classrooms))), "Tags": JoinList(SplitList(JoinList(body.Tags))), "Created": now().Format(DateTimeFormat),
	}
	if !a.commit(w, r, actor, store.Insert(FeedsTab, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed added", "actor", actor, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": token, "url": feedURL(r, token)})
}

func (a app) editFeed(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		Token      string   `json:"token"`
		Name       string   `json:"name"`
		Emoji      string   `json:"emoji"`
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		http.Error(w, "a feed needs a name", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Token) == MyHeliosianToken {
		if err := a.cache.Commit(r.Context(), actor, homeOp(actor, store.Row{"Home Name": strings.TrimSpace(body.Name), "Home Emoji": feedEmoji(body.Emoji)})); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	f := a.cache.Model().Feed(strings.TrimSpace(body.Token))
	if f == nil {
		http.Error(w, "no such feed", http.StatusNotFound)
		return
	}
	if f.Email != actor && !admin {
		http.Error(w, "only the person who made a feed, or an admin, can change it", http.StatusForbidden)
		return
	}
	cells := store.Row{
		"Name": strings.TrimSpace(body.Name), "Emoji": feedEmoji(body.Emoji), "Classrooms": JoinList(SplitList(JoinList(body.Classrooms))), "Tags": JoinList(SplitList(JoinList(body.Tags))),
	}
	if !a.commit(w, r, actor, store.Update(FeedsTab, store.Row{"Token": f.Token}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed changed", "actor", actor, "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
	w.WriteHeader(http.StatusNoContent)
}

type Hooks struct {
	Answer      Answerer
	MakeDefault func(ctx context.Context, email, token string) error
	// RSVPs answers GET /api/apps/rsvp, which every app's host serves for
	// the shared toolbar - the calendar's own registers it itself: the
	// invitations waiting for the viewer's reply.
	RSVPs http.HandlerFunc
}

func homeOp(email string, cells store.Row) store.Op {
	return store.Set(SettingsTab, store.Row{"Email": email}, cells)
}

func (a app) makeDefault(ctx context.Context, email, token string) error {
	email = normalizeEmail(email)
	mine := a.cache.Model().MyCalendars(email)
	tokens := []string{token}
	found := false
	for _, f := range mine {
		if f.Token == token {
			found = true
			continue
		}
		tokens = append(tokens, f.Token)
	}
	if !found {
		return fmt.Errorf("that is not one of your calendars")
	}
	if err := a.reorder(ctx, email, tokens); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: default calendar set", "actor", email, "token", token)
	return nil
}

func (a app) setDefault(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.makeDefault(r.Context(), actor, strings.TrimSpace(body.Token)); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorder(ctx context.Context, email string, tokens []string) error {
	model := a.cache.Model()
	ops := []store.Op{}
	if at := slices.Index(tokens, MyHeliosianToken); at >= 0 {
		ops = append(ops, homeOp(email, store.Row{"Home Position": strconv.Itoa(at)}))
		tokens = slices.Delete(slices.Clone(tokens), at, at+1)
	}
	mine := map[string]string{}
	for _, f := range model.Feeds {
		if normalizeEmail(f.Email) == email {
			mine[f.Token] = f.order
		}
	}
	if len(tokens) != len(mine) {
		return fmt.Errorf("the order must name each of your calendars once")
	}
	current := []string{}
	for _, t := range tokens {
		order, ok := mine[t]
		if !ok || slices.Contains(tokens[:len(current)], t) {
			return fmt.Errorf("the order must name each of your calendars once")
		}
		current = append(current, order)
	}
	keys := store.Order(current)
	for i, t := range tokens {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(FeedsTab, store.Row{"Token": t}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	if err := a.cache.Commit(ctx, email, ops...); err != nil {
		return err
	}
	slog.InfoContext(ctx, "calendar: feeds ordered", "actor", email, "order", strings.Join(tokens, ","))
	return nil
}

func (a app) orderFeeds(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := a.reorder(r.Context(), actor, body.Tokens); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	actor, admin := a.who(r)
	var body struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &body) {
		return
	}
	f := a.cache.Model().Feed(strings.TrimSpace(body.Token))
	if f == nil {
		http.Error(w, "no such feed", http.StatusNotFound)
		return
	}
	if f.Email != actor && !admin {
		http.Error(w, "only the person who made a feed, or an admin, can remove it", http.StatusForbidden)
		return
	}
	name := f.Name
	if !a.commit(w, r, actor, store.Delete(FeedsTab, store.Row{"Token": f.Token})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: feed removed", "actor", actor, "name", name)
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

func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, imageFolder, maxImageSize)
}

func (a app) setKeywords(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID       string   `json:"id"`
		Keywords []string `json:"keywords"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(body.ID)
	if e == nil {
		http.Error(w, "that event is not in the sheet", http.StatusNotFound)
		return
	}
	words := SplitList(JoinList(body.Keywords))
	cell := Clear
	if len(words) > 0 {
		cell = JoinList(words)
	}
	if !a.commit(w, r, actor, store.Set(OverridesTab, store.Row{"Event ID": e.ID}, store.Row{"Keywords": cell})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: keywords set", "actor", actor, "event", e.ID, "keywords", cell)
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

func (a app) addEvents(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
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
		ID          string   `json:"id"`
		RepeatWeeks int      `json:"repeatWeeks"`
		RepeatTimes int      `json:"repeatTimes"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains(sharingWords, body.Sharing) {
		http.Error(w, "sharing is "+strings.Join(sharingWords, ", "), http.StatusBadRequest)
		return
	}
	if body.RepeatTimes < 0 || body.RepeatTimes > 52 || body.RepeatWeeks < 1 && body.RepeatTimes > 0 {
		http.Error(w, "repeat up to 52 more times, some whole number of weeks apart", http.StatusBadRequest)
		return
	}
	chosen := strings.ToLower(strings.TrimSpace(body.ID))
	if chosen != "" {
		if !eventIDForm.MatchString(chosen) {
			http.Error(w, "a web address is 3 to 40 letters, digits and dashes", http.StatusBadRequest)
			return
		}
		if a.cache.Model().Event(chosen) != nil || a.cache.Count(EventsTab, store.Row{"Event ID": chosen}) > 0 {
			http.Error(w, "that web address is taken", http.StatusBadRequest)
			return
		}
	}
	if !admin {
		body.RepeatTimes, body.DayType = 0, ""
	}
	pending := body.Sharing == SharingPublic
	status := ""
	if pending {
		status = StatusPending
	}
	stamp := now().Format(DateFormat)
	ids := []string{}
	ops := []store.Op{}
	for i := 0; i <= body.RepeatTimes; i++ {
		id := newEventID()
		if i == 0 && chosen != "" {
			id = chosen
		}
		ids = append(ids, id)
		ops = append(ops, store.Insert(EventsTab, store.Row{
			"Event ID": id, "Start": shiftWhen(strings.TrimSpace(body.Start), i*body.RepeatWeeks), "End": shiftWhen(strings.TrimSpace(body.End), i*body.RepeatWeeks),
			"Title": strings.TrimSpace(body.Title), "Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
			"Tags": JoinList(SplitList(JoinList(body.Tags))), "Day Type": strings.TrimSpace(body.DayType), "Keywords": JoinList(SplitList(JoinList(body.Keywords))),
			"Added By": actor, "Added": stamp, "Source": strings.TrimSpace(body.Source), "Sharing": body.Sharing, "Status": status, "Image": strings.Trim(strings.TrimSpace(body.Image), "/"),
		}))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: events added", "actor", actor, "title", strings.TrimSpace(body.Title), "count", len(ids), "pending", pending)
	for _, id := range ids {
		if err := a.record(r.Context(), actor, id, AnswerYes, false); err != nil {
			slog.WarnContext(r.Context(), "calendar: host's yes", "event", id, "error", err)
		}
	}
	if a.mail.Sender != nil {
		if e := a.cache.Model().Event(ids[0]); e != nil {
			go a.tellAdmins(context.WithoutCancel(r.Context()), r.Host, actor, e)
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

func (a app) tellAdmins(ctx context.Context, host, by string, e *Event) {
	admins := a.cache.Admins(a.superAdmins())
	if len(admins) == 0 {
		return
	}
	who := by
	if p, ok := a.directory.Person(by); ok && p.Name != "" {
		who = p.Name + " (" + by + ")"
	}
	link := "https://" + host + EventPath(e)
	day, hours := whenLines(e)
	when := day
	if hours != "" {
		when += " · " + hours
	}
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
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n%s\n", lead, e.Title, when)
	if e.Location != "" {
		fmt.Fprintf(&text, "%s\n", e.Location)
	}
	if e.Description != "" {
		fmt.Fprintf(&text, "\n%s\n", e.Description)
	}
	fmt.Fprintf(&text, "\nIts page: %s\n%s\n", link, closing)
	var htm strings.Builder
	fmt.Fprintf(&htm, "<p style=\"font:16px/1.5 -apple-system,Segoe UI,Roboto,sans-serif\">%s</p>", html.EscapeString(lead))
	fmt.Fprintf(&htm, "<p style=\"font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#0e4d54\"><strong>%s</strong><br>%s", html.EscapeString(e.Title), html.EscapeString(when))
	if e.Location != "" {
		fmt.Fprintf(&htm, "<br>%s", html.EscapeString(e.Location))
	}
	htm.WriteString("</p>")
	if e.Description != "" {
		fmt.Fprintf(&htm, "<p style=\"font:14px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#333\">%s</p>", html.EscapeString(e.Description))
	}
	fmt.Fprintf(&htm, "<p style=\"margin:20px 0\"><a href=\"%s\" style=\"display:inline-block;padding:10px 18px;border-radius:8px;background:#0e4d54;color:#fff;font:700 15px -apple-system,Segoe UI,Roboto,sans-serif;text-decoration:none\">%s</a></p>", html.EscapeString(link), map[bool]string{true: "Review the event", false: "See the event"}[e.Sharing == SharingPublic])
	fmt.Fprintf(&htm, "<p style=\"font:13px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;color:#647071\">%s</p>", html.EscapeString(closing))
	err := a.mail.Sender.Send(ctx, mail.Message{
		To: admins, Subject: subject + e.Title + " · " + day,
		Text: text.String(), HTML: htm.String(),
	})
	if err != nil {
		slog.ErrorContext(ctx, "[ERROR] calendar: tell admins", "event", e.ID, "error", err)
		return
	}
	slog.InfoContext(ctx, "calendar: admins told", "event", e.ID, "to", len(admins))
}

func (a app) editEvent(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	var body struct {
		ID          string   `json:"id"`
		Title       string   `json:"title"`
		Start       string   `json:"start"`
		End         string   `json:"end"`
		Location    string   `json:"location"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Keywords    []string `json:"keywords"`
		Source      string   `json:"source"`
		Image       string   `json:"image"`
		Sharing     string   `json:"sharing"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains(sharingWords, body.Sharing) {
		http.Error(w, "sharing is "+strings.Join(sharingWords, ", "), http.StatusBadRequest)
		return
	}
	e := a.cache.Model().Event(strings.TrimSpace(body.ID))
	if e == nil || e.Source != SourceSheet {
		http.Error(w, "that event is not one added by hand", http.StatusNotFound)
		return
	}
	if (normalizeEmail(e.AddedBy) != actor || e.PosterLeft) && !admin {
		http.Error(w, "only the person who added an event, while they host it, or an admin, can change it", http.StatusForbidden)
		return
	}
	cells := store.Row{
		"Title": strings.TrimSpace(body.Title), "Start": strings.TrimSpace(body.Start), "End": strings.TrimSpace(body.End),
		"Location": strings.TrimSpace(body.Location), "Description": strings.TrimSpace(body.Description),
		"Tags": JoinList(SplitList(JoinList(body.Tags))), "Keywords": JoinList(SplitList(JoinList(body.Keywords))),
		"Source": strings.TrimSpace(body.Source), "Image": strings.Trim(strings.TrimSpace(body.Image), "/"),
	}
	if body.Sharing != e.Sharing {
		cells["Sharing"] = body.Sharing
		cells["Status"] = ""
		if body.Sharing == SharingPublic {
			cells["Status"] = StatusPending
		}
	}
	if !a.commit(w, r, actor, store.Update(EventsTab, store.Row{"Event ID": e.ID}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event changed", "actor", actor, "event", e.ID, "title", cells["Title"])
	if cells["Status"] == StatusPending && a.mail.Sender != nil {
		if changed := a.cache.Model().Event(e.ID); changed != nil {
			go a.tellAdmins(context.WithoutCancel(r.Context()), r.Host, actor, changed)
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
	actor, _ := a.who(r)
	var body struct {
		ID string `json:"id"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(strings.TrimSpace(body.ID))
	if e == nil || e.Source != SourceSheet {
		http.Error(w, "that event is not one added by hand", http.StatusNotFound)
		return
	}
	if status == StatusDeclined && e.Sharing != SharingPublic {
		http.Error(w, "only a public event is declined", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Status": status})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event "+did, "actor", actor, "event", e.ID, "title", e.Title, "by", e.AddedBy)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) moveEvent(w http.ResponseWriter, r *http.Request) {
	actor, _ := a.who(r)
	var body struct {
		ID    string `json:"id"`
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if !decode(w, r, &body) {
		return
	}
	e := a.cache.Model().Event(body.ID)
	if e == nil || e.Source != SourceSheet {
		http.Error(w, "only an event of the Events tab moves from here", http.StatusBadRequest)
		return
	}
	start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
	if end == "" {
		end = start
	}
	if !a.commit(w, r, actor, store.Update(EventsTab, store.Row{"Event ID": e.ID}, store.Row{"Start": start, "End": end})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: event moved", "actor", actor, "event", e.ID, "start", start, "end", end)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveSetting(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	var body struct {
		Classrooms []string `json:"classrooms"`
		Tags       []string `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	cells := store.Row{
		"Classrooms": JoinList(SplitList(JoinList(body.Classrooms))), "Categories": JoinList(SplitList(JoinList(body.Tags))), "Saved": now().Format(DateTimeFormat),
	}
	if !a.commit(w, r, email, store.Set(SettingsTab, store.Row{"Email": email}, cells)) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view saved", "actor", email, "classrooms", cells["Classrooms"], "tags", cells["Categories"])
	w.WriteHeader(http.StatusNoContent)
}

func (a app) forgetSetting(w http.ResponseWriter, r *http.Request) {
	email, _ := a.who(r)
	if _, ok := a.cache.Model().Settings[email]; !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, email, store.Update(SettingsTab, store.Row{"Email": email}, store.Row{"Classrooms": "", "Categories": "", "Saved": ""})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: view forgotten", "actor", email)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] calendar: store image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name, "url": "/" + imageFolder + "/" + name})
}

func (a app) setTags(w http.ResponseWriter, r *http.Request) {
	actor, admin := a.who(r)
	if !admin {
		http.Error(w, "only a calendar admin can change the categories", http.StatusForbidden)
		return
	}
	var body struct {
		Tags []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Group       string `json:"group"`
			Default     bool   `json:"default"`
			Image       string `json:"image"`
		} `json:"tags"`
	}
	if !decode(w, r, &body) {
		return
	}
	current := map[string]Tag{}
	for _, t := range a.cache.Model().Tags {
		current[t.Name] = t
	}
	names, orders, rows := []string{}, []string{}, []store.Row{}
	for _, t := range body.Tags {
		name, description, group, image := strings.TrimSpace(t.Name), strings.TrimSpace(t.Description), strings.TrimSpace(t.Group), strings.TrimSpace(t.Image)
		if name == "" || slices.Contains(names, name) {
			http.Error(w, "every category needs a name of its own", http.StatusBadRequest)
			return
		}
		if description == "" {
			http.Error(w, fmt.Sprintf("%q needs a description", name), http.StatusBadRequest)
			return
		}
		if _, ok := current[name]; !ok && slices.ContainsFunc(builtinTags, func(b Tag) bool { return b.Name == name }) {
			http.Error(w, fmt.Sprintf("%q is built in and has no row of its own", name), http.StatusBadRequest)
			return
		}
		names = append(names, name)
		orders = append(orders, current[name].order)
		rows = append(rows, store.Row{"Description": description, "Group": group, "Default": yesNoWord(t.Default), "Image": image})
	}
	for name := range current {
		if !slices.Contains(names, name) {
			http.Error(w, fmt.Sprintf("%q is missing - a category cannot be removed from here", name), http.StatusBadRequest)
			return
		}
	}
	keys := store.Order(orders)
	ops := []store.Op{}
	added := 0
	for i, name := range names {
		cells := rows[i]
		cells[store.OrderColumn] = keys[i]
		was, ok := current[name]
		if !ok {
			cells["Tag"] = name
			ops = append(ops, store.Insert(TagsTab, cells))
			added++
			continue
		}
		if tagDefault(cells["Default"]) == was.Default {
			delete(cells, "Default")
		}
		ops = append(ops, store.Update(TagsTab, store.Row{"Tag": name}, cells))
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: categories saved", "actor", actor, "added", added)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) feed(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSuffix(r.PathValue("file"), ".ics")
	model := a.cache.Model()
	f := model.Feed(token)
	if f == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", "helios-calendar.ics"))
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(ICS(model, f, a.linked(f.Email), "https://"+r.Host, now()))
}
