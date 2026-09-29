package model

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"heliosian/internal/mail"
)

const notifyTimeout = 30 * time.Second

type FeedbackNotifier struct {
	Sender      *mail.Mailgun
	Base        string
	SuperAdmins func() []string
}

func (n FeedbackNotifier) Notify(r Report, attachments []mail.Attachment) {
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
	m := n.message(to, r)
	m.Attachments = attachments
	if err := n.Sender.Send(ctx, m); err != nil {
		slog.Error("feedback: announce", "error", err, "id", r.ID)
	}
}

func screenOf(r Report) string {
	switch {
	case r.Viewport != "" && r.Screen != "":
		return r.Viewport + " window on a " + r.Screen + " screen"
	case r.Viewport != "":
		return r.Viewport + " window"
	}
	return r.Screen
}

func (n FeedbackNotifier) message(to []string, r Report) mail.Message {
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
	facts := [][2]string{{"Browser", r.UserAgent}, {"Screen", screenOf(r)}}
	if r.Screenshot != "" {
		facts = append(facts, [2]string{"Screenshot", "attached"})
	}
	var b strings.Builder
	var t strings.Builder
	fmt.Fprintf(&b, "<p>%s reported from %s by %s.</p>", html.EscapeString(what), html.EscapeString(r.AppName), html.EscapeString(r.Email))
	fmt.Fprintf(&b, "<p><strong>%s</strong></p>", html.EscapeString(r.Summary))
	fmt.Fprintf(&b, "<p>%s</p>", strings.ReplaceAll(html.EscapeString(details), "\n", "<br>"))
	fmt.Fprintf(&b, "<p>They were on %s at %s.</p>", html.EscapeString(r.Page), html.EscapeString(r.At.In(Location).Format("2006-01-02 15:04 MST")))
	fmt.Fprintf(&t, "%s reported from %s by %s.\n\n%s\n\n%s\n\nThey were on %s at %s.\n\n",
		what, r.AppName, r.Email, r.Summary, details, r.Page, r.At.In(Location).Format("2006-01-02 15:04 MST"))
	b.WriteString("<p>")
	for _, f := range facts {
		if f[1] == "" {
			continue
		}
		fmt.Fprintf(&b, "%s: %s<br>", f[0], html.EscapeString(f[1]))
		fmt.Fprintf(&t, "%s: %s\n", f[0], f[1])
	}
	b.WriteString("</p>")
	fmt.Fprintf(&b, "<p><a href=%q>Read it, edit it and file it</a></p>", link)
	fmt.Fprintf(&t, "\nRead it, edit it and file it: %s\n", link)
	return mail.Message{To: to, Subject: subject, HTML: b.String(), Text: t.String()}
}
