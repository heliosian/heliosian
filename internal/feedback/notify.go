package feedback

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"heliosian/internal/mail"
	"heliosian/internal/when"
)

const notifyTimeout = 30 * time.Second

type Notifier struct {
	Sender      mail.Sender
	From        string
	Base        string
	SuperAdmins func() []string
}

func (n Notifier) Notify(r Report) {
	if n.Sender == nil {
		return
	}
	to := []string{}
	for _, email := range n.SuperAdmins() {
		if email = strings.ToLower(strings.TrimSpace(email)); email != "" {
			to = append(to, email)
		}
	}
	if len(to) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if err := n.Sender.Send(ctx, n.message(to, r)); err != nil {
		slog.Error("feedback: announce", "error", err, "id", r.ID)
	}
}

func (n Notifier) message(to []string, r Report) mail.Message {
	what := "An idea"
	if r.Kind == "bug" {
		what = "A problem"
	}
	subject := fmt.Sprintf("[%s] %s: %s", r.AppName, strings.ToLower(what), r.Summary)
	link := strings.TrimSuffix(n.Base, "/") + "/admin?tab=feedback&report=" + r.ID
	details := r.Details
	if details == "" {
		details = "No details given."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<p>%s reported from %s by %s.</p>", html.EscapeString(what), html.EscapeString(r.AppName), html.EscapeString(r.Email))
	fmt.Fprintf(&b, "<p><strong>%s</strong></p>", html.EscapeString(r.Summary))
	fmt.Fprintf(&b, "<p>%s</p>", strings.ReplaceAll(html.EscapeString(details), "\n", "<br>"))
	fmt.Fprintf(&b, "<p>They were on %s at %s.</p>", html.EscapeString(r.Page), html.EscapeString(r.At.In(when.Location).Format("2006-01-02 15:04 MST")))
	fmt.Fprintf(&b, "<p><a href=%q>Read it, edit it and file it</a></p>", link)
	text := fmt.Sprintf("%s reported from %s by %s.\n\n%s\n\n%s\n\nThey were on %s at %s.\n\nRead it, edit it and file it: %s\n",
		what, r.AppName, r.Email, r.Summary, details, r.Page, r.At.In(when.Location).Format("2006-01-02 15:04 MST"), link)
	return mail.Message{To: to, Subject: subject, HTML: b.String(), Text: text}
}
