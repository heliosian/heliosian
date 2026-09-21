package loop

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/mail"
)

const (
	eventSent      = "sent"
	eventDelivered = "delivered"
	eventBounced   = "bounced"
	eventDelayed   = "delivery_delayed"
	eventComplaint = "complained"

	copyDelivered = "delivered"
	copyFailed    = "failed"
	copyPending   = "pending"
)

type Attempt struct {
	When   string `json:"when"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

type Copy struct {
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	State    string    `json:"state"`
	When     string    `json:"when,omitempty"`
	Attempts []Attempt `json:"attempts"`
}

type SentMessage struct {
	Received   string `json:"received"`
	From       Person `json:"from"`
	Subject    string `json:"subject"`
	Recipients int    `json:"recipients"`
	Delivered  int    `json:"delivered"`
	Failed     int    `json:"failed"`
	Pending    int    `json:"pending"`
	Copies     []Copy `json:"copies"`
}

func messageKey(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func (a app) sender(from string) Person {
	email := strings.ToLower(mail.AddressOf(from))
	if p, ok := a.directory.Person(a.directory.Resolve(email)); ok {
		return p
	}
	return Person{Email: email, Name: senderName(from)}
}

func eventTime(when string) time.Time {
	t, _ := time.Parse(time.RFC3339, when)
	return t
}

func copyOf(email, name string, attempts []Attempt) Copy {
	slices.SortStableFunc(attempts, func(x, y Attempt) int { return eventTime(x.When).Compare(eventTime(y.When)) })
	c := Copy{Email: email, Name: name, State: copyPending, Attempts: attempts}
	for _, at := range attempts {
		switch at.Event {
		case eventDelivered:
			c.State, c.When = copyDelivered, at.When
			return c
		case eventBounced:
			c.State, c.When = copyFailed, at.When
		}
	}
	return c
}

var copyOrder = map[string]int{copyFailed: 0, copyPending: 1, copyDelivered: 2}

func (a app) history(name string) []SentMessage {
	tables := a.cache.Tables()
	attempts := map[string]map[string][]Attempt{}
	for _, row := range tables.Deliveries {
		key := messageKey(row["Message"])
		if key == "" || !strings.EqualFold(strings.TrimSpace(row["Group"]), name) {
			continue
		}
		email := cleanEmail(row["Email"])
		if attempts[key] == nil {
			attempts[key] = map[string][]Attempt{}
		}
		attempts[key][email] = append(attempts[key][email], Attempt{When: row["Timestamp"], Event: row["Event"], Detail: row["Detail"]})
	}
	out := []SentMessage{}
	for i := len(tables.Messages) - 1; i >= 0; i-- {
		row := tables.Messages[i]
		if row["State"] != stateSent || !strings.EqualFold(strings.TrimSpace(row["Group"]), name) {
			continue
		}
		recipients, _ := strconv.Atoi(row["Recipients"])
		sent := SentMessage{Received: row["Received"], From: a.sender(row["From"]), Subject: row["Subject"], Recipients: recipients, Copies: []Copy{}}
		for email, list := range attempts[messageKey(row["Message ID"])] {
			c := copyOf(email, a.person(email).Name, slices.Clone(list))
			switch c.State {
			case copyDelivered:
				sent.Delivered++
			case copyFailed:
				sent.Failed++
			default:
				sent.Pending++
			}
			sent.Copies = append(sent.Copies, c)
		}
		slices.SortFunc(sent.Copies, func(x, y Copy) int {
			return cmp.Or(cmp.Compare(copyOrder[x.State], copyOrder[y.State]), cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name)), cmp.Compare(x.Email, y.Email))
		})
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
