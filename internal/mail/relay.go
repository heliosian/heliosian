package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"os"
	"path/filepath"
	"time"
)

type RawSender interface {
	SendRaw(ctx context.Context, from string, to []string, raw []byte) error
}

type Relay struct {
	Host, Port, User, Pass string
}

func ResendRelay(key string) *Relay {
	return &Relay{Host: "smtp.resend.com", Port: "465", User: "resend", Pass: key}
}

func (r *Relay) SendRaw(ctx context.Context, from string, to []string, raw []byte) error {
	if len(to) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 20 * time.Second}, Config: &tls.Config{ServerName: r.Host}}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(r.Host, r.Port))
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, r.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Auth(smtp.PlainAuth("", r.User, r.Pass, r.Host)); err != nil {
		return err
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (f *Files) SendRaw(ctx context.Context, from string, to []string, raw []byte) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(f.Dir, fmt.Sprintf("%s-%s.eml", time.Now().Format("20060102-150405.000000"), slug(to[0])))
	if err := os.WriteFile(name, raw, 0o644); err != nil {
		return err
	}
	slog.InfoContext(ctx, "mail: wrote raw message", "file", name, "from", from, "to", to, "bytes", len(raw))
	return nil
}
