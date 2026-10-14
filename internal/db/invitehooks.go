package db

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const extShell = "web/public/when/ext.html"

type extCopy struct {
	recipient, group store.Row
	person           string
}

func (m *Model) invitedGroup(token string) (store.Row, store.Row, bool) {
	rcp, ok := m.recipientByToken(token)
	if !ok {
		return nil, nil, false
	}
	msg, _ := m.Table("MESSAGE").Get(rcp["message"])
	g, ok := m.Table("GROUP").Get(msg["group"])
	if !ok || !slices.Contains(guestListKinds, g["kind"]) || g["status"] == "closed" {
		return nil, nil, false
	}
	return rcp, g, true
}

func (iv *Invitations) extCopy(r *http.Request) (extCopy, error) {
	rcp, g, ok := iv.s.Model().invitedGroup(r.PathValue("token"))
	if !ok {
		return extCopy{}, access.Missing("that invitation is not here")
	}
	return extCopy{recipient: rcp, group: g, person: rcp["person"]}, nil
}

type extPerson struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Answer string `json:"answer,omitempty"`
}

type extView struct {
	Title       string      `json:"title"`
	Day         string      `json:"day"`
	Hours       string      `json:"hours,omitempty"`
	Location    string      `json:"location,omitempty"`
	Description string      `json:"description,omitempty"`
	Hosts       []string    `json:"hosts"`
	Name        string      `json:"name"`
	Answer      string      `json:"answer,omitempty"`
	Guests      bool        `json:"guests"`
	Brought     []extPerson `json:"brought"`
	Family      []extPerson `json:"family"`
	Past        bool        `json:"past,omitempty"`
	Cancelled   bool        `json:"cancelled,omitempty"`
	Flyer       string      `json:"flyer,omitempty"`
}

func (iv *Invitations) extFlyer(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil || c.group["flyer"] == "" {
		http.NotFound(w, r)
		return
	}
	raw, kind, err := iv.pics.bucket.Get(r.Context(), c.group["flyer"])
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", kind)
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Write(raw)
}

func (m *Model) broughtGuests(g store.Row, person string) []string {
	out := []string{}
	for _, row := range m.Table("MEMBER").Referencing("group", g["id"]) {
		if row["guest_of"] == person && memberAs(row) == "yes" {
			out = append(out, row["person"])
		}
	}
	return out
}

func (iv *Invitations) extView(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	m := iv.s.Model()
	g := c.group
	day, hours := dayAndHours(g)
	view := extView{
		Title: g["name"], Day: day, Hours: hours, Location: g["location"], Description: g["description"], Hosts: m.namesOf(m.hostsOf(g)),
		Name: m.nameOf(c.person), Answer: m.answerOf(g, c.person), Guests: g["join"] == "others", Brought: []extPerson{}, Family: []extPerson{},
		Past: m.ended(g, time.Now()), Cancelled: g["status"] == "cancelled",
	}
	if g["flyer"] != "" {
		view.Flyer = "/open/ext/" + r.PathValue("token") + "/flyer"
	}
	for _, p := range m.householdOnList(g, c.person)[1:] {
		view.Family = append(view.Family, extPerson{Key: p, Name: m.nameOf(p), Answer: m.answerOf(g, p)})
	}
	for _, p := range m.broughtGuests(g, c.person) {
		view.Brought = append(view.Brought, extPerson{Key: p, Name: m.nameOf(p), Answer: m.answerOf(g, p)})
	}
	iv.noteOpened(r.Context(), m, g, c.person)
	serve.Write(w, r, http.StatusOK, view)
}

func (iv *Invitations) noteOpened(ctx context.Context, m *Model, g store.Row, person string) {
	at := stamp(time.Now())
	edit := Edit{Insert: "MEMBER", Row: map[string]any{"group": g["id"], "person": person, "opened": at}}
	if row, ok := m.Table("MEMBER").Find(g["id"], person); ok {
		if row["opened"] != "" {
			return
		}
		edit = Edit{Set: row["id"], Cells: map[string]any{"opened": at}}
	}
	if _, err := iv.write(ctx, access.Actor{Email: m.addressOf(person)}, []Edit{edit}); err != nil {
		slog.ErrorContext(ctx, "invitations: opened not noted", "group", g["id"], "person", person, "error", err)
	}
}

func (iv *Invitations) extAnswer(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	var body struct {
		Answer string `json:"answer"`
		Key    string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&body); err != nil {
		serve.Error(w, r, access.Invalid("send {\"answer\": \"yes\"}: %v", err))
		return
	}
	if body.Answer != "" && body.Answer != "yes" && body.Answer != "no" {
		serve.Error(w, r, access.Invalid("an answer is yes, no or blank, not %q", body.Answer))
		return
	}
	m := iv.s.Model()
	subject := c.person
	if body.Key != "" && body.Key != c.person {
		if !slices.Contains(m.householdOnList(c.group, c.person), body.Key) && !slices.Contains(m.broughtGuests(c.group, c.person), body.Key) {
			serve.Error(w, r, access.Forbidden("you answer for yourself, your family and your guests"))
			return
		}
		subject = body.Key
	}
	edits := append(answerGroupEdits(c.group), m.answerEdits(c.group, subject, body.Answer, c.person, time.Now())...)
	if _, err := iv.write(r.Context(), access.Actor{Email: m.addressOf(c.person)}, edits); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitations: answered from outside", "person", c.person, "for", subject, "group", c.group["id"], "answer", body.Answer)
	w.WriteHeader(http.StatusNoContent)
}

func (iv *Invitations) extGuest(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	if c.group["join"] != "others" {
		serve.Error(w, r, access.Forbidden("the hosts add the guests to this one"))
		return
	}
	var body struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&body); err != nil {
		serve.Error(w, r, access.Invalid("send {\"name\": \"…\", \"email\": \"…\"}: %v", err))
		return
	}
	m := iv.s.Model()
	name := strings.Join(strings.Fields(body.Name), " ")
	address := strings.ToLower(strings.TrimSpace(body.Email))
	if name == "" {
		serve.Error(w, r, access.Invalid("a guest needs a name"))
		return
	}
	if address != "" && (mail.AddressOf(address) != address || !strings.Contains(address, "@")) {
		serve.Error(w, r, access.Invalid("%q is not an address", body.Email))
		return
	}
	at := time.Now()
	edits := []Edit{}
	guest := m.personAt(address)
	if address == "" || guest == "" {
		edits = append(edits, Edit{Insert: "PERSON", As: "guest", Row: map[string]any{"source": "guest", "name_long_override": name}})
		if address != "" {
			edits = append(edits, Edit{Insert: "PERSON_EMAIL", Row: map[string]any{"person": "@guest", "address": address, "primary": true, "source": "guest"}})
		}
		guest = "@guest"
	} else if m.onList(c.group, guest) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	edits = append(edits, Edit{Insert: "MEMBER", Row: map[string]any{"group": c.group["id"], "person": guest, "member": "yes", "guest_of": c.person, "added_by": c.person, "added": stamp(at)}})
	if address != "" {
		edits = append(edits, invitationEdits(c.group["id"], c.person, "invitation", []string{guest}, at)...)
	}
	if _, err := iv.write(r.Context(), access.Actor{Email: m.addressOf(c.person)}, edits); err != nil {
		serve.Error(w, r, err)
		return
	}
	slog.InfoContext(r.Context(), "invitations: guest brought from outside", "person", c.person, "group", c.group["id"])
	w.WriteHeader(http.StatusNoContent)
}

func (iv *Invitations) extRemoveGuest(w http.ResponseWriter, r *http.Request) {
	c, err := iv.extCopy(r)
	if err != nil {
		serve.Error(w, r, err)
		return
	}
	var body struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&body); err != nil {
		serve.Error(w, r, access.Invalid("send {\"key\": \"…\"}: %v", err))
		return
	}
	m := iv.s.Model()
	row, ok := m.Table("MEMBER").Find(c.group["id"], body.Key)
	if !ok || row["guest_of"] != c.person {
		serve.Error(w, r, access.Missing("no guest of yours %s", body.Key))
		return
	}
	edits := append([]Edit{{Delete: row["id"]}}, m.answerEdits(c.group, body.Key, "", "", time.Now())...)
	if _, err := iv.write(r.Context(), access.Actor{Email: m.addressOf(c.person)}, edits); err != nil {
		serve.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (iv *Invitations) replyTag(recipients string) (string, bool) {
	want := mail.Normalize(mail.AddressOf(iv.config.ReplyTo))
	for _, rcpt := range strings.Split(recipients, ",") {
		local, domain, _ := strings.Cut(mail.Normalize(mail.AddressOf(rcpt)), "@")
		base, tag, _ := strings.Cut(local, "+")
		if base+"@"+domain == want {
			return tag, true
		}
	}
	return "", false
}

func (m *Model) replyRecipient(tag string) (store.Row, bool) {
	if raw, err := hex.DecodeString(tag); err == nil {
		if row, ok := m.Table("RECIPIENT").Find(string(raw)); ok {
			return row, true
		}
	}
	return m.recipientByToken(tag)
}

func (iv *Invitations) replies(w http.ResponseWriter, r *http.Request) {
	if iv.config.SigningKey == "" {
		http.Error(w, "replies are not set up", http.StatusNotFound)
		return
	}
	fields, err := mail.Notification(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyNotification(iv.config.SigningKey, fields, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "invitations: reply notification refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	tag, ok := iv.replyTag(fields["recipient"])
	if !ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	raw := []byte(fields["body-mime"])
	reply, err := mail.ParseReply(raw)
	if err != nil {
		slog.InfoContext(r.Context(), "invitations: mail received is not a reply", "from", fields["from"], "subject", fields["subject"], "error", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	lines, _ := mail.SplitMessage(raw)
	from := mail.Normalize(mail.AddressOf(mail.Header(lines, "from")))
	log := slog.With("from", from, "uid", reply.UID, "attendee", reply.Email, "standing", reply.Standing)
	if reason := mail.Authenticated(lines); reason != "" {
		log.WarnContext(r.Context(), "invitations: reply not authenticated", "reason", reason)
		w.WriteHeader(http.StatusOK)
		return
	}
	m := iv.s.Model()
	rcp, ok := m.replyRecipient(tag)
	if !ok {
		log.WarnContext(r.Context(), "invitations: reply to an address no copy carries")
		w.WriteHeader(http.StatusOK)
		return
	}
	person := rcp["person"]
	if m.personAt(from) != person || mail.Normalize(reply.Email) != from {
		log.WarnContext(r.Context(), "invitations: reply from someone else", "person", person)
		w.WriteHeader(http.StatusOK)
		return
	}
	answer := map[string]string{"ACCEPTED": "yes", "DECLINED": "no"}[reply.Standing]
	if answer == "" {
		log.InfoContext(r.Context(), "invitations: reply changes nothing")
		w.WriteHeader(http.StatusOK)
		return
	}
	msg, _ := m.Table("MESSAGE").Get(rcp["message"])
	g, ok := m.Table("GROUP").Get(msg["group"])
	if !ok {
		w.WriteHeader(http.StatusOK)
		return
	}
	edits := append(answerGroupEdits(g), m.answerEdits(g, person, answer, person, time.Now())...)
	if _, err := iv.write(r.Context(), access.Actor{Email: from}, edits); err != nil {
		log.ErrorContext(r.Context(), "invitations: reply not recorded", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.InfoContext(r.Context(), "invitations: answered by reply", "person", person, "group", g["id"], "answer", answer)
	w.WriteHeader(http.StatusOK)
}
