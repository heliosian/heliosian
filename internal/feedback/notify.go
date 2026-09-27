package feedback

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"heliosian/internal/mail"
)

// notifyTimeout bounds one announcement; it runs off the request already, so
// the only thing waiting on it is the next report in the queue.
const notifyTimeout = 30 * time.Second

// Notifier tells the super admins a report came in, so the queue is something
// they hear about rather than somewhere they remember to look. base is the
// address of Heliosian's admin page, which the mail links to.
type Notifier struct {
	Sender      mail.Sender
	From        string
	Base        string
	SuperAdmins func() []string
}

// Notify is the queue's announcement hook. A report nobody can be told about -
// no sender, or nobody to tell - is simply not announced; it is already saved.
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
	link := strings.TrimSuffix(n.Base, "/") + "/admin?panel=feedback&report=" + r.ID
	details := r.Details
	if details == "" {
		details = "No details given."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "<p>%s reported from %s by %s.</p>", html.EscapeString(what), html.EscapeString(r.AppName), html.EscapeString(r.Email))
	fmt.Fprintf(&b, "<p><strong>%s</strong></p>", html.EscapeString(r.Summary))
	fmt.Fprintf(&b, "<p>%s</p>", strings.ReplaceAll(html.EscapeString(details), "\n", "<br>"))
	fmt.Fprintf(&b, "<p>They were on %s at %s.</p>", html.EscapeString(r.Page), html.EscapeString(r.At.In(school).Format("2006-01-02 15:04 MST")))
	fmt.Fprintf(&b, "<p><a href=%q>Read it, edit it and file it</a></p>", link)
	text := fmt.Sprintf("%s reported from %s by %s.\n\n%s\n\n%s\n\nThey were on %s at %s.\n\nRead it, edit it and file it: %s\n",
		what, r.AppName, r.Email, r.Summary, details, r.Page, r.At.In(school).Format("2006-01-02 15:04 MST"), link)
	return mail.Message{To: to, Subject: subject, HTML: b.String(), Text: text}
}
