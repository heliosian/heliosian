package model

import (
	"bytes"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/serve"
)

const extShell = "web/public/when/ext.html"

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

func (a calendarApp) extPage(w http.ResponseWriter, r *http.Request) {
	page, err := os.ReadFile(extShell)
	if err != nil {
		http.Error(w, "the page is missing", http.StatusInternalServerError)
		return
	}
	head := ""
	if inv, ok := a.model().InviteByToken(r.PathValue("token")); ok {
		if e := a.model().invitedEvent(a.eventFor(access.Actor{Email: inv.Email}, inv.EventID)); e != nil {
			origin := "https://" + r.Host
			parts := []string{when(e)}
			if e.Location != "" {
				parts = append(parts, e.Location)
			}
			head = a.style.PreviewTags(e.Title, strings.Join(parts, " — "), origin+extPath(inv.Token), origin+"/open/share/"+e.ID+".png")
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(bytes.Replace(page, []byte("<!--preview-->"), []byte(head), 1))
}

func (a calendarApp) extInvite(r *http.Request) (Invite, *Event, error) {
	model := a.model()
	inv, ok := model.InviteByToken(r.PathValue("token"))
	if !ok {
		return Invite{}, nil, access.Missing("that invitation is not here")
	}
	e, err := a.findEvent(access.Actor{Email: inv.Email}, inv.EventID)
	if err != nil {
		return Invite{}, nil, err
	}
	return inv, model.invitedEvent(e), nil
}

func (a calendarApp) extView(r *http.Request, _ serve.None) (ExtView, error) {
	inv, e, err := a.extInvite(r)
	if err != nil {
		return ExtView{}, err
	}
	a.noteOpened(r.Context(), access.Actor{Email: inv.Email}, e)
	model := a.model()
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
		if p := a.directory().Person(h); p != nil && p.FullName != "" {
			view.Hosts = append(view.Hosts, p.FullName)
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
	return view, nil
}

type extAnswerBody struct {
	Answer string `json:"answer"`
	Key    string `json:"key"`
}

func (a calendarApp) extAnswer(r *http.Request, body extAnswerBody) (serve.None, error) {
	inv, e, err := a.extInvite(r)
	if err != nil {
		return serve.None{}, err
	}
	actor := access.Actor{Email: inv.Email}
	subject, err := a.extSubject(actor, e, body.Key, body.Answer)
	if err != nil {
		return serve.None{}, err
	}
	answer := strings.ToLower(strings.TrimSpace(body.Answer))
	if err := a.recordBy(r.Context(), actor, subject, e.ID, answer, ViaPage, false, false); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: answered from outside", "actor", actor.Email, "for", subject, "event", e.ID, "answer", answer)
	return serve.None{}, nil
}

type extGuestBody struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (a calendarApp) extGuest(r *http.Request, body extGuestBody) (guestKey, error) {
	inv, e, err := a.extInvite(r)
	if err != nil {
		return guestKey{}, err
	}
	actor := access.Actor{Email: inv.Email}
	ops, g, err := a.extGuestOps(actor, e, body.Name, body.Email)
	if err != nil {
		return guestKey{}, err
	}
	if err := a.bringGuest(r.Context(), actor, ops, g); err != nil {
		return guestKey{}, err
	}
	return guestKey{Email: g.key}, nil
}

type keyBody struct {
	Key string `json:"key"`
}

func (a calendarApp) extRemoveGuest(r *http.Request, body keyBody) (serve.None, error) {
	inv, e, err := a.extInvite(r)
	if err != nil {
		return serve.None{}, err
	}
	actor := access.Actor{Email: inv.Email}
	ops, key, err := a.extRemoveGuestOps(actor, e, body.Key)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "calendar: guest removed from outside", "actor", actor.Email, "event", e.ID, "guest", key)
	return serve.None{}, nil
}
