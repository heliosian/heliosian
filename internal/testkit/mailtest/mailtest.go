package mailtest

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"heliosian/internal/intercept"
	"heliosian/internal/mail"
)

const From = "Helios Sample <sample@example.org>"

type Raw struct {
	Domain  string
	To      []string
	Message []byte
}

type Recorder struct {
	Mailgun  *mail.Mailgun
	mu       sync.Mutex
	messages []mail.Message
	raws     []Raw
	next     int
}

var (
	installed sync.Once
	mu        sync.Mutex
	recorders = map[string]*Recorder{}
)

func install() {
	installed.Do(func() {
		intercept.Install(mail.Host, http.HandlerFunc(serve))
	})
}

func Discard() *mail.Mailgun {
	install()
	return mail.NewMailgun("discard", From)
}

func NewRecorder(from string) *Recorder {
	install()
	mu.Lock()
	defer mu.Unlock()
	key := fmt.Sprintf("recorder-%d", len(recorders))
	r := &Recorder{Mailgun: mail.NewMailgun(key, from)}
	recorders[key] = r
	return r
}

func serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, key, _ := r.BasicAuth()
	mu.Lock()
	rec := recorders[key]
	mu.Unlock()
	if rec != nil {
		parts := strings.Split(r.URL.Path, "/")
		domain, endpoint := parts[2], parts[3]
		var err error
		switch endpoint {
		case "messages":
			err = rec.keep(r)
		case "messages.mime":
			err = rec.keepRaw(r, domain)
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	w.Write([]byte(`{"id":"<intercepted@example.org>","message":"Queued. Thank you."}`))
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (rec *Recorder) keep(r *http.Request) error {
	form := r.MultipartForm.Value
	m := mail.Message{To: form["to"], CC: form["cc"], Subject: first(form["subject"]), HTML: first(form["html"]), Text: first(form["text"])}
	if line := first(form["from"]); line != rec.Mailgun.From() {
		m.FromName = line[:strings.LastIndex(line, " <")]
	}
	if replyTo := first(form["h:Reply-To"]); replyTo != "" {
		m.ReplyTo = strings.Split(replyTo, ", ")
	}
	for k, v := range form {
		if name, ok := strings.CutPrefix(k, "h:"); ok && name != "Reply-To" {
			if m.Headers == nil {
				m.Headers = map[string]string{}
			}
			m.Headers[name] = v[0]
		}
	}
	for _, header := range r.MultipartForm.File["attachment"] {
		f, err := header.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return err
		}
		m.Attachments = append(m.Attachments, mail.Attachment{Name: header.Filename, ContentType: header.Header.Get("Content-Type"), Content: content})
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.messages = append(rec.messages, m)
	return nil
}

func (rec *Recorder) keepRaw(r *http.Request, domain string) error {
	f, _, err := r.FormFile("message")
	if err != nil {
		return err
	}
	defer f.Close()
	content, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.raws = append(rec.raws, Raw{Domain: domain, To: r.MultipartForm.Value["to"], Message: content})
	return nil
}

func (rec *Recorder) Messages() []mail.Message {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.messages)
}

func (rec *Recorder) Raws() []Raw {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return slices.Clone(rec.raws)
}

func (rec *Recorder) Wait(t *testing.T, n int) []mail.Message {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if messages := rec.Messages(); len(messages) >= n {
			return messages
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("waited for %d messages", n)
	return nil
}

func (rec *Recorder) Next(t *testing.T) mail.Message {
	t.Helper()
	m := rec.Wait(t, rec.next+1)[rec.next]
	rec.next++
	return m
}
