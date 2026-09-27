package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"mime"
	"slices"
	"strings"
	"time"
)

type Message struct {
	To          []string
	CC          []string
	ReplyTo     []string
	FromName    string
	Subject     string
	HTML        string
	Text        string
	Attachments []Attachment
	Headers     map[string]string
}

func FromLine(from string, m Message) string {
	if m.FromName == "" {
		return from
	}
	return fmt.Sprintf("%s <%s>", strings.ReplaceAll(m.FromName, "<", ""), AddressOf(from))
}

type Attachment struct {
	Name        string
	ContentType string
	Content     []byte
}

type Sender interface {
	Send(ctx context.Context, m Message) error
	From() string
}

func Compose(from string, m Message) string {
	boundary := fmt.Sprintf("hca-%d", time.Now().UnixNano())
	outer := "mixed-" + boundary
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", FromLine(from, m))
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	if len(m.CC) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(m.CC, ", "))
	}
	if len(m.ReplyTo) > 0 {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", strings.Join(m.ReplyTo, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	for _, k := range slices.Sorted(maps.Keys(m.Headers)) {
		fmt.Fprintf(&b, "%s: %s\r\n", k, m.Headers[k])
	}
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	if len(m.Attachments) > 0 {
		fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n\r\n", outer)
		fmt.Fprintf(&b, "--%s\r\n", outer)
	}
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	text := m.Text
	if text == "" {
		text = m.Subject
	}
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, crlf(text))
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, crlf(m.HTML))
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	if len(m.Attachments) > 0 {
		for _, a := range m.Attachments {
			fmt.Fprintf(&b, "--%s\r\nContent-Type: %s; name=%q\r\nContent-Disposition: attachment; filename=%q\r\nContent-Transfer-Encoding: base64\r\n\r\n", outer, a.ContentType, a.Name, a.Name)
			enc := base64.StdEncoding.EncodeToString(a.Content)
			for len(enc) > 76 {
				b.WriteString(enc[:76] + "\r\n")
				enc = enc[76:]
			}
			b.WriteString(enc + "\r\n")
		}
		fmt.Fprintf(&b, "--%s--\r\n", outer)
	}
	return b.String()
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
