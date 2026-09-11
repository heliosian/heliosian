// Package mail sends the portal's email: one Sender interface, an SMTP
// implementation for real mail, and a Files one for development that writes
// each message to disk where it can be opened in a browser.
package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Message is one email: who gets it, what it says, in HTML with a plain-text
// twin for clients that want one.
type Message struct {
	To      []string
	CC      []string
	ReplyTo []string
	Subject string
	HTML    string
	Text    string
}

type Sender interface {
	Send(ctx context.Context, m Message) error
}

// New picks the sender the environment describes: Resend when it has a key,
// SMTP when a host is set, else files in dir when one is given, else nil - no
// mail, logged once - which callers treat as "not set up".
func New(resendKey, host, port, user, pass, from, dir string) Sender {
	switch {
	case resendKey != "":
		slog.Info("mail: sending through Resend", "from", from)
		return &Resend{Key: resendKey, From: from}
	case host != "":
		if port == "" {
			port = "587"
		}
		slog.Info("mail: sending over SMTP", "host", host, "from", from)
		return &SMTP{Host: host, Port: port, User: user, Pass: pass, From: from}
	case dir != "":
		slog.Info("mail: writing messages to files", "dir", dir)
		return &Files{Dir: dir, From: from}
	}
	slog.Warn("mail: not configured; messages are dropped")
	return nil
}

// SMTP sends through a submission server with STARTTLS - Google Workspace's
// smtp.gmail.com with an app password, or any provider's SMTP endpoint.
type SMTP struct {
	Host, Port, User, Pass, From string
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	if len(m.To) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("mail: bad From %q: %w", s.From, err)
	}
	dialer := net.Dialer{Timeout: 20 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.Host, s.Port))
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.Host}); err != nil {
			return err
		}
	}
	if s.User != "" {
		if err := c.Auth(smtp.PlainAuth("", s.User, s.Pass, s.Host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	for _, rcpt := range append(append([]string{}, m.To...), m.CC...) {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(Compose(s.From, m))); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// Resend sends through resend.com's HTTP API: one POST per message, the key
// as a bearer token. The sending domain is verified in Resend's dashboard.
type Resend struct {
	Key, From string
	// Endpoint stands in for the API in tests; blank means the real one.
	Endpoint string
}

func (r *Resend) Send(ctx context.Context, m Message) error {
	if len(m.To) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	payload := map[string]any{"from": r.From, "to": m.To, "subject": m.Subject, "html": m.HTML}
	if m.Text != "" {
		payload["text"] = m.Text
	}
	if len(m.CC) > 0 {
		payload["cc"] = m.CC
	}
	if len(m.ReplyTo) > 0 {
		payload["reply_to"] = m.ReplyTo
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = "https://api.resend.com/emails"
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("mail: resend %d: %s", resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	return nil
}

// Compose is the message as bytes on the wire: headers, then a
// multipart/alternative body carrying the text and the HTML.
func Compose(from string, m Message) string {
	boundary := fmt.Sprintf("hca-%d", time.Now().UnixNano())
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	if len(m.CC) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(m.CC, ", "))
	}
	if len(m.ReplyTo) > 0 {
		fmt.Fprintf(&b, "Reply-To: %s\r\n", strings.Join(m.ReplyTo, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", m.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	text := m.Text
	if text == "" {
		text = m.Subject
	}
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, crlf(text))
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s\r\n", boundary, crlf(m.HTML))
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}

func crlf(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// Files writes each message as an .html file (the HTML part, with the
// envelope in a comment at the top) into Dir - the development sender.
type Files struct {
	Dir, From string
}

func (f *Files) Send(ctx context.Context, m Message) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(f.Dir, fmt.Sprintf("%s-%s.html", time.Now().Format("20060102-150405.000"), slug(m.Subject)))
	head := fmt.Sprintf("<!-- From: %s\nTo: %s\nCc: %s\nReply-To: %s\nSubject: %s -->\n", f.From, strings.Join(m.To, ", "), strings.Join(m.CC, ", "), strings.Join(m.ReplyTo, ", "), m.Subject)
	if err := os.WriteFile(name, []byte(head+m.HTML), 0o644); err != nil {
		return err
	}
	slog.InfoContext(ctx, "mail: wrote message", "file", name, "to", m.To, "cc", m.CC, "subject", m.Subject)
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
