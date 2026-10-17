package model

import (
	"fmt"
	"slices"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

const (
	KindMessage   = "message"
	KindCancelled = "cancelled"
)

var messageKinds = []string{KindMessage, inviteReminder, inviteUpdate, KindCancelled}

type EventMessage struct {
	ID         string
	EventID    string
	Kind       string
	Subject    string
	Text       string
	Recipients []string
	Attach     bool
	SentBy     string
	Created    string
	SentTo     []string
}

func (b *builder) messages(rows []store.Row) error {
	for _, row := range rows {
		key := strings.TrimSpace(row["Message ID"])
		if key == "" {
			continue
		}
		kind := strings.TrimSpace(row["Kind"])
		if !slices.Contains(messageKinds, kind) {
			return fmt.Errorf("message %s: kind %q is not %s", key, kind, strings.Join(messageKinds, ", "))
		}
		attach, err := cells.YesNo(row["Attach"], false)
		if err != nil {
			return fmt.Errorf("message %s: attach %w", key, err)
		}
		b.model.Messages = append(b.model.Messages, EventMessage{
			ID: key, EventID: strings.TrimSpace(row["Event ID"]), Kind: kind, Subject: strings.TrimSpace(row["Subject"]), Text: strings.TrimSpace(row["Text"]),
			Recipients: splitEmails(row["Recipients"]), Attach: attach, SentBy: mail.Normalize(row["Sent By"]), Created: strings.TrimSpace(row["Created"]),
			SentTo: splitEmails(row["Sent To"]),
		})
		b.model.eventIDs[key] = true
	}
	return nil
}
