package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"maps"
	"mime"
	"os"
	"path/filepath"
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
}

func New(mailgunKey, from, dir string) Sender {
	switch {
	case mailgunKey != "":
		slog.Info("mail: sending through Mailgun", "from", from)
		return NewMailgun(mailgunKey, from)
	case dir != "":
		slog.Info("mail: writing messages to files", "dir", dir)
		return &Files{Dir: dir, From: from}
	}
	slog.Warn("mail: not configured; messages are dropped")
	return nil
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

type Files struct {
	Dir, From string
}

func (f *Files) Send(ctx context.Context, m Message) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	to := ""
	if len(m.To) > 0 {
		to = "-" + slug(strings.SplitN(m.To[0], "@", 2)[0])
	}
	name := filepath.Join(f.Dir, fmt.Sprintf("%s-%s%s.html", time.Now().Format("20060102-150405.000"), slug(m.Subject), to))
	extra := ""
	for _, k := range slices.Sorted(maps.Keys(m.Headers)) {
		extra += fmt.Sprintf("\n%s: %s", k, m.Headers[k])
	}
	head := fmt.Sprintf("<!-- From: %s\nTo: %s\nCc: %s\nReply-To: %s\nSubject: %s%s -->\n", FromLine(f.From, m), strings.Join(m.To, ", "), strings.Join(m.CC, ", "), strings.Join(m.ReplyTo, ", "), m.Subject, extra)
	if err := os.WriteFile(name, []byte(head+m.HTML), 0o644); err != nil {
		return err
	}
	for _, a := range m.Attachments {
		if err := os.WriteFile(strings.TrimSuffix(name, ".html")+"-"+a.Name, a.Content, 0o644); err != nil {
			return err
		}
	}
	slog.InfoContext(ctx, "mail: wrote message", "file", name, "to", m.To, "cc", m.CC, "subject", m.Subject, "attachments", len(m.Attachments))
	return nil
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
