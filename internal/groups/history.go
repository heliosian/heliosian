package groups

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

type Trouble struct {
	When   string `json:"when"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

type SentMessage struct {
	Received   string    `json:"received"`
	From       Person    `json:"from"`
	Subject    string    `json:"subject"`
	Recipients int       `json:"recipients"`
	Trouble    []Trouble `json:"trouble"`
}

func messageKey(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func (a app) sender(from string) Person {
	email := strings.ToLower(addressOf(from))
	if p, ok := a.directory.Person(a.directory.Resolve(email)); ok {
		return p
	}
	return Person{Email: email, Name: senderName(from)}
}

func (a app) history(name string) []SentMessage {
	tables := a.cache.Tables()
	trouble := map[string][]Trouble{}
	for _, row := range tables.Deliveries {
		key := messageKey(row["Message"])
		if key == "" || !strings.EqualFold(strings.TrimSpace(row["Group"]), name) {
			continue
		}
		email := cleanEmail(row["Email"])
		trouble[key] = append(trouble[key], Trouble{When: row["Timestamp"], Email: email, Name: a.person(email).Name, Event: row["Event"], Detail: row["Detail"]})
	}
	out := []SentMessage{}
	for i := len(tables.Messages) - 1; i >= 0; i-- {
		row := tables.Messages[i]
		if row["State"] != stateSent || !strings.EqualFold(strings.TrimSpace(row["Group"]), name) {
			continue
		}
		recipients, _ := strconv.Atoi(row["Recipients"])
		sent := SentMessage{Received: row["Received"], From: a.sender(row["From"]), Subject: row["Subject"], Recipients: recipients, Trouble: []Trouble{}}
		if key := messageKey(row["Message ID"]); key != "" {
			sent.Trouble = append(sent.Trouble, trouble[key]...)
		}
		out = append(out, sent)
	}
	return out
}

func (a app) sentCount(name string) int {
	n := 0
	for _, row := range a.cache.Tables().Messages {
		if row["State"] == stateSent && strings.EqualFold(strings.TrimSpace(row["Group"]), name) {
			n++
		}
	}
	return n
}

func (a app) messages(w http.ResponseWriter, r *http.Request) {
	email, admin := a.who(r)
	g := a.cache.Model().Group(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("name"))))
	if g == nil {
		http.Error(w, "no such group", http.StatusNotFound)
		return
	}
	if !admin && !g.Manages(email) {
		http.Error(w, "you do not manage this group", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"messages": a.history(g.Name)}); err != nil {
		slog.ErrorContext(r.Context(), "encode groups messages", "error", err)
	}
}
