package model

import (
	"cmp"
	netmail "net/mail"
	"slices"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/id"
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

	kindMessage = "email-list-message"
	kindCopy    = "email-list-copy"
)

type Attempt struct {
	When   string `json:"when"`
	Event  string `json:"event"`
	Detail string `json:"detail,omitempty"`
}

type Copy struct {
	ID       string
	Group    string
	Email    string
	State    string
	When     string
	Attempts []Attempt
}

type Sent struct {
	ID         string
	Message    ListMessage
	Recipients int
	Delivered  int
	Failed     int
	Pending    int
	Copies     []*Copy
}

func messageKey(id string) string {
	return strings.Trim(strings.TrimSpace(id), "<>")
}

func senderName(from string) string {
	a, err := netmail.ParseAddress(from)
	if err != nil {
		local, _, _ := strings.Cut(mail.AddressOf(from), "@")
		return local
	}
	if a.Name != "" {
		return a.Name
	}
	local, _, _ := strings.Cut(a.Address, "@")
	return local
}

func eventTime(when string) time.Time {
	t, _ := time.Parse(time.RFC3339, when)
	return t
}

func copyOf(email string, attempts []Attempt) Copy {
	slices.SortStableFunc(attempts, func(x, y Attempt) int { return eventTime(x.When).Compare(eventTime(y.When)) })
	c := Copy{Email: email, State: copyPending, Attempts: attempts}
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

func (m *EmailLists) indexHistory() {
	m.sent, m.sentByID, m.copyByID = map[string][]*Sent{}, map[string]*Sent{}, map[string]*Copy{}
	attempts := map[string]map[string]map[string][]Attempt{}
	for _, d := range m.Deliveries {
		key := messageKey(d.Message)
		if key == "" {
			continue
		}
		if attempts[d.Group] == nil {
			attempts[d.Group] = map[string]map[string][]Attempt{}
		}
		if attempts[d.Group][key] == nil {
			attempts[d.Group][key] = map[string][]Attempt{}
		}
		attempts[d.Group][key][d.Email] = append(attempts[d.Group][key][d.Email], Attempt{When: d.Timestamp, Event: d.Event, Detail: d.Detail})
	}
	for _, msg := range m.Messages {
		if msg.State != "sent" || m.Group(msg.Group) == nil {
			continue
		}
		s := &Sent{ID: id.Of(m.idKey, kindMessage, msg.Group+"\x00"+msg.ID), Message: msg, Copies: []*Copy{}}
		s.Recipients, _ = strconv.Atoi(msg.Recipients)
		for email, list := range attempts[msg.Group][messageKey(msg.MessageID)] {
			c := copyOf(email, slices.Clone(list))
			c.ID, c.Group = id.Of(m.idKey, kindCopy, s.ID+"\x00"+email), msg.Group
			switch c.State {
			case copyDelivered:
				s.Delivered++
			case copyFailed:
				s.Failed++
			default:
				s.Pending++
			}
			s.Copies = append(s.Copies, &c)
			m.copyByID[c.ID] = &c
		}
		slices.SortFunc(s.Copies, func(x, y *Copy) int {
			return cmp.Or(cmp.Compare(copyOrder[x.State], copyOrder[y.State]), cmp.Compare(x.Email, y.Email))
		})
		m.sent[msg.Group] = append(m.sent[msg.Group], s)
		m.sentByID[s.ID] = s
	}
	for _, list := range m.sent {
		slices.SortStableFunc(list, func(x, y *Sent) int { return eventTime(y.Message.Received).Compare(eventTime(x.Message.Received)) })
	}
}
