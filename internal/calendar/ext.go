package calendar

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
)

// Someone from outside the community - a coach, a grandparent, a friend -
// invited to an event has no Helios sign-in, so their invitation names a
// page of their own, /ext/{token}, found by the secret their Invites row
// carries and served without a session (auth.Public). It shows the event
// and nothing of anyone else: no guest list, no counts, no names but the
// hosts' and their own guests'. The page answers, brings a guest and takes
// one back through /open/ext/{token}, public the same way.

const extShell = "web/public/calendar/ext.html"

// extPath is an outside person's page, as a path.
func extPath(token string) string {
	return "/ext/" + token
}

// ExtView is the event as an outside person's page shows it: the event,
// who invites them, the hosts' message, their own name and answer, and
// the guests they have brought.
type ExtView struct {
	Title       string     `json:"title"`
	Day         string     `json:"day"`
	Hours       string     `json:"hours,omitempty"`
	Location    string     `json:"location,omitempty"`
	Description string     `json:"description,omitempty"`
	Hosts       []string   `json:"hosts"`
	Message     string     `json:"message,omitempty"`
	Name        string     `json:"name"`
	Answer      string     `json:"answer,omitempty"`
	Guests      bool       `json:"guests"`
	Brought     []ExtGuest `json:"brought"`
	// Family is the rest of their household on the list - the family they
	// were added with - each with an answer of their own to give here.
	Family []ExtGuest `json:"family"`
	Past   bool       `json:"past,omitempty"`
	// Flyer is the invitation's flyer to show, when there is one; Banner
	// the picture the event's page wears, across the top.
	Flyer  string `json:"flyer,omitempty"`
	Banner string `json:"banner"`
}

// ExtGuest is one guest an outside person brought.
type ExtGuest struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Answer string `json:"answer,omitempty"`
}

// extPage serves the page itself, its head carrying the event's preview
// tags - the title, a line, and the share card - so a link to it previews
// in a chat app the way the event's page does; the token is checked
// again when the page asks for its event, so a stale link shows the
// page's own words for that.
func (a app) extPage(w http.ResponseWriter, r *http.Request) {
	page, err := os.ReadFile(extShell)
	if err != nil {
		http.Error(w, "the page is missing", http.StatusInternalServerError)
		return
	}
	head := ""
	if inv, ok := a.cache.Model().InviteByToken(r.PathValue("token")); ok {
		if e := a.cache.Model().invitedEvent(a.eventFor(inv.Email, inv.EventID)); e != nil {
			origin := "https://" + r.Host
			parts := []string{when(e)}
			if e.Location != "" {
				parts = append(parts, e.Location)
			}
			head = previewTags(e.Title, strings.Join(parts, " \u2014 "), origin+extPath(inv.Token), origin+"/open/share/"+e.ID+".png")
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(bytes.Replace(page, []byte("<!--preview-->"), []byte(head), 1))
}

// extInvite is the row a token names and the event it is for, or a 404.
func (a app) extInvite(w http.ResponseWriter, r *http.Request) (Invite, *Event, bool) {
	model := a.cache.Model()
	inv, ok := model.InviteByToken(r.PathValue("token"))
	if !ok {
		http.Error(w, "that invitation is not here", http.StatusNotFound)
		return Invite{}, nil, false
	}
	e := a.eventFor(inv.Email, inv.EventID)
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return Invite{}, nil, false
	}
	return inv, model.invitedEvent(e), true
}

// extView answers GET /open/ext/{token}.
func (a app) extView(w http.ResponseWriter, r *http.Request) {
	inv, e, ok := a.extInvite(w, r)
	if !ok {
		return
	}
	model := a.cache.Model()
	day, hours := whenLines(e)
	view := ExtView{Title: e.Title, Day: day, Hours: hours, Location: e.Location, Description: e.Description, Hosts: []string{}, Name: inv.Name, Answer: model.AnswerOf(inv.Email, e.ID), Guests: true, Brought: []ExtGuest{}, Family: []ExtGuest{}, Past: e.end.Before(now()), Banner: "/open/banner/" + e.ID}
	for _, member := range a.householdOn(e, inv.Email)[1:] {
		if row := model.InviteOf(e.ID, member); row != nil {
			answer := model.AnswerOf(member, e.ID)
			if answer == AnswerHidden {
				answer = ""
			}
			view.Family = append(view.Family, ExtGuest{Key: member, Name: row.Name, Answer: answer})
		}
	}
	if view.Answer == AnswerHidden {
		view.Answer = ""
	}
	for _, h := range a.hostsOf(e) {
		if p, known := a.directory.Person(h); known && p.Name != "" {
			view.Hosts = append(view.Hosts, p.Name)
		}
	}
	if settings := model.Invitations[e.ID]; settings != nil {
		view.Message, view.Guests = settings.Message, settings.Guests
		if settings.Flyer != "" {
			view.Flyer = flyerPath(e.ID)
		}
	}
	for _, g := range model.Invites[e.ID] {
		if g.GuestOf == inv.Email {
			view.Brought = append(view.Brought, ExtGuest{Key: g.Email, Name: g.Name, Answer: model.AnswerOf(g.Email, e.ID)})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode outside invitation", "error", err)
	}
}

// extAnswer is POST /open/ext/{token}: the outside person's own word, or
// - with a key - one for someone in their family on the list.
func (a app) extAnswer(w http.ResponseWriter, r *http.Request) {
	inv, e, ok := a.extInvite(w, r)
	if !ok {
		return
	}
	var body struct {
		Answer string `json:"answer"`
		Key    string `json:"key"`
	}
	if !decode(w, r, &body) {
		return
	}
	answer := strings.ToLower(strings.TrimSpace(body.Answer))
	if answer == AnswerHidden {
		http.Error(w, "an answer is yes, no, maybe, or blank", http.StatusBadRequest)
		return
	}
	subject := inv.Email
	if key := normalizeEmail(body.Key); key != "" && key != inv.Email {
		if !slices.Contains(a.householdOn(e, inv.Email), key) {
			http.Error(w, "that is not someone in your family", http.StatusForbidden)
			return
		}
		subject = key
	}
	// The invitation carried their calendar invite already: none is sent
	// back for a yes.
	if err := a.recordBy(r.Context(), inv.Email, subject, e.ID, answer, ViaPage, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "calendar: answered from outside", "actor", inv.Email, "for", subject, "event", e.ID, "answer", answer)
	w.WriteHeader(http.StatusNoContent)
}

// extGuest is POST /open/ext/{token}/guest: the outside person bringing a
// guest of their own, when the hosts allow guests.
func (a app) extGuest(w http.ResponseWriter, r *http.Request) {
	inv, e, ok := a.extInvite(w, r)
	if !ok {
		return
	}
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if !decode(w, r, &body) {
		return
	}
	if settings := a.cache.Model().Invitations[e.ID]; settings == nil || !settings.Guests {
		http.Error(w, "this event is not taking guests", http.StatusForbidden)
		return
	}
	key, err := a.bringGuest(r.Context(), inv.Email, e, inv.Email, body.Name, body.Email, AnswerYes, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"email": key})
}

// extRemoveGuest is DELETE /open/ext/{token}/guest: taking back a guest
// of their own.
func (a app) extRemoveGuest(w http.ResponseWriter, r *http.Request) {
	inv, e, ok := a.extInvite(w, r)
	if !ok {
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := normalizeEmail(body.Key)
	guest := a.cache.Model().InviteOf(e.ID, key)
	if guest == nil || guest.GuestOf != inv.Email {
		http.Error(w, "that is not a guest of yours", http.StatusNotFound)
		return
	}
	if !a.commit(r.Context(), w, a.cache.Tables().WithoutInvite(e.ID, key), func() error {
		if err := a.writer.Delete(appName, InvitesTab, map[string]string{"Event ID": e.ID, "Email": key}); err != nil {
			return err
		}
		return a.writer.Delete(appName, RSVPsTab, map[string]string{"Event ID": e.ID, "Email": key})
	}) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: guest removed from outside", "actor", inv.Email, "event", e.ID, "guest", key)
	w.WriteHeader(http.StatusNoContent)
}
