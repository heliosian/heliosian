package db

import (
	"bytes"
	"fmt"
	"mime"
	netmail "net/mail"
	"strings"

	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

func (m *Model) mailRoot(raw []byte) (map[string]any, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("not a mail message: %w", err)
	}
	sent, err := msg.Header.Date()
	if err != nil {
		return nil, fmt.Errorf("its date: %w", err)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil {
		return nil, fmt.Errorf("its subject: %w", err)
	}
	row := map[string]any{
		"kind":      mailKind(msg.Header.Get("List-Id")),
		"name":      strings.TrimSpace(subject),
		"published": sent.In(School).Format(cells.StampFormat),
	}
	if author := m.PersonOf(mail.AddressOf(msg.Header.Get("From"))); author != "" {
		row["author"] = author
	}
	return row, nil
}

func mailKind(listID string) string {
	id := listID
	if i := strings.LastIndex(id, "<"); i >= 0 {
		id = id[i+1:]
	}
	id = strings.ToLower(strings.Trim(strings.TrimSpace(id), "<>"))
	switch {
	case strings.HasSuffix(id, ".heliosschool.org"), strings.HasSuffix(id, ".heliosns.org"):
		return "list"
	case strings.HasSuffix(id, ".loop.heliosian.com"):
		return "post"
	}
	return "newsletter"
}
