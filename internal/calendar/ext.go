package calendar

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"

	"heliosian/internal/store"
)

const extShell = "web/public/calendar/ext.html"

func extPath(token string) string {
	return "/ext/" + token
}

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
	Family      []ExtGuest `json:"family"`
	Past        bool       `json:"past,omitempty"`
	Flyer       string     `json:"flyer,omitempty"`
	Banner      string     `json:"banner"`
}

type ExtGuest struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Answer string `json:"answer,omitempty"`
}

func (a app) extPage(w http.ResponseWriter, r *http.Request) {
	page, err := os.ReadFile(extShell)
	if err != nil {
		http.Error(w, "the page is missing", http.StatusInternalServerError)
		return
	}
	head := ""
	if inv, ok := a.cache.Model().InviteByToken(r.PathValue("token")); ok {
		if e := a.cache.Model().invitedEvent(a.eventFor(inv.Email, false, inv.EventID)); e != nil {
			origin := "https://" + r.Host
			parts := []string{when(e)}
			if e.Location != "" {
				parts = append(parts, e.Location)
			}
			head = previewTags(e.Title, strings.Join(parts, " — "), origin+extPath(inv.Token), origin+"/open/share/"+e.ID+".png")
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(bytes.Replace(page, []byte("<!--preview-->"), []byte(head), 1))
}

func (a app) extInvite(w http.ResponseWriter, r *http.Request) (Invite, *Event, bool) {
	model := a.cache.Model()
	inv, ok := model.InviteByToken(r.PathValue("token"))
	if !ok {
		http.Error(w, "that invitation is not here", http.StatusNotFound)
		return Invite{}, nil, false
	}
	e := a.eventFor(inv.Email, false, inv.EventID)
	if e == nil {
		http.Error(w, "that event is not on the calendar", http.StatusNotFound)
		return Invite{}, nil, false
	}
	return inv, model.invitedEvent(e), true
}

func (a app) extView(w http.ResponseWriter, r *http.Request) {
	inv, e, ok := a.extInvite(w, r)
	if !ok {
		return
	}
	a.noteOpened(r.Context(), e, inv.Email)
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
		slog.ErrorContext(r.Context(), "[ERROR] encode outside invitation", "error", err)
	}
}

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
	if err := a.recordBy(r.Context(), inv.Email, subject, e.ID, answer, ViaPage, false, false); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	slog.InfoContext(r.Context(), "calendar: answered from outside", "actor", inv.Email, "for", subject, "event", e.ID, "answer", answer)
	w.WriteHeader(http.StatusNoContent)
}

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
	if !a.commit(w, r, inv.Email, store.Delete(InvitesTab, store.Row{"Event ID": e.ID, "Email": key})) {
		return
	}
	slog.InfoContext(r.Context(), "calendar: guest removed from outside", "actor", inv.Email, "event", e.ID, "guest", key)
	w.WriteHeader(http.StatusNoContent)
}
