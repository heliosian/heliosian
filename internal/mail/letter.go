package mail

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/config"
)

type Brand struct {
	Name    string
	Color   string
	Tagline string
}

type Letter struct {
	Brand    Brand
	Base     string
	Title    string
	Subtitle string
	When     string
	Where    string
	Path     string
	Picture  string
	Heading  string
	Intro    string
	Note     string
	Rows     [][2]string
	Button   string
	Calendar string
	Footnote string
}

func Base(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

var letterTemplate = template.Must(template.New("letter").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Heading}}</title></head>
<body style="margin:0;padding:0;background:#f3f6f6;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1f2a2b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f6f6;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 2px 10px rgba(0,0,0,0.06);">
<tr><td style="background:{{.Brand.Color}};padding:14px 24px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td style="vertical-align:middle;"><img src="{{.Base}}/brand/logo-mark.png" width="28" height="28" alt="" style="display:block;border:0;"></td>
<td style="vertical-align:middle;padding-left:10px;color:#ffffff;font-weight:700;font-size:16px;letter-spacing:0.02em;">{{.Brand.Name}}</td>
</tr></table>
</td></tr>
{{if .Picture}}<tr><td style="padding:0;"><a href="{{.Path}}" style="display:block;"><img src="{{.Picture}}" width="600" alt="{{.Title}}" style="display:block;width:100%;height:auto;border:0;"></a></td></tr>{{end}}
<tr><td style="padding:28px 28px 8px;">
<h1 style="margin:0 0 10px;font-size:24px;line-height:1.25;color:{{.Brand.Color}};">{{.Heading}}</h1>
<p style="margin:0 0 18px;font-size:16px;line-height:1.5;">{{.Intro}}</p>
{{if .Note}}<blockquote style="margin:0 0 18px;padding:10px 16px;border-left:3px solid {{.Brand.Color}};font-size:15px;line-height:1.55;white-space:pre-wrap;">{{.Note}}</blockquote>{{end}}
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f8f8;border-radius:12px;">
<tr><td style="padding:16px 18px;">
<div style="font-size:19px;font-weight:700;color:{{.Brand.Color}};">{{.Title}}</div>
{{if .Subtitle}}<div style="font-size:13.5px;color:#5c6b6c;margin-top:2px;">{{.Subtitle}}</div>{{end}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin-top:12px;font-size:15px;line-height:1.45;">
{{if .When}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">When</td><td style="padding:3px 0;">{{.When}}</td></tr>{{end}}
{{if .Where}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">Where</td><td style="padding:3px 0;">{{.Where}}</td></tr>{{end}}
{{range .Rows}}<tr><td style="color:#5c6b6c;padding:3px 16px 3px 0;white-space:nowrap;vertical-align:top;">{{index . 0}}</td><td style="padding:3px 0;white-space:pre-wrap;">{{index . 1}}</td></tr>{{end}}
</table>
</td></tr>
</table>
</td></tr>
<tr><td style="padding:18px 28px 28px;">
{{if .Button}}<a href="{{.Path}}" style="display:inline-block;background:{{.Brand.Color}};color:#ffffff;text-decoration:none;font-weight:700;font-size:15px;padding:12px 20px;border-radius:10px;">{{.Button}}</a>{{end}}
{{if .Calendar}}<a href="{{.Calendar}}" style="display:inline-block;margin-left:10px;background:#ffffff;color:{{.Brand.Color}};border:2px solid {{.Brand.Color}};text-decoration:none;font-weight:700;font-size:15px;padding:10px 18px;border-radius:10px;">Add to Calendar</a>{{end}}
{{if .Footnote}}<p style="margin:18px 0 0;font-size:13.5px;line-height:1.5;color:#5c6b6c;">{{.Footnote}}</p>{{end}}
</td></tr>
<tr><td style="padding:14px 28px;background:#f4f8f8;font-size:12.5px;color:#7a8788;">
Sent by {{.Brand.Name}}, {{.Brand.Tagline}} · <a href="{{.Base}}" style="color:{{.Brand.Color}};">{{.Base}}</a>
</td></tr>
</table>
</td></tr>
</table>
</body></html>`))

func (l Letter) Render() (string, string) {
	var html bytes.Buffer
	if err := letterTemplate.Execute(&html, l); err != nil {
		slog.Error("mail: render", "error", err)
	}
	var text strings.Builder
	fmt.Fprintf(&text, "%s\n\n%s\n\n", l.Heading, l.Intro)
	if l.Note != "" {
		fmt.Fprintf(&text, "%s\n\n", l.Note)
	}
	fmt.Fprintf(&text, "%s\n", l.Title)
	if l.Subtitle != "" {
		fmt.Fprintf(&text, "%s\n", l.Subtitle)
	}
	if l.When != "" {
		fmt.Fprintf(&text, "When: %s\n", l.When)
	}
	if l.Where != "" {
		fmt.Fprintf(&text, "Where: %s\n", l.Where)
	}
	for _, row := range l.Rows {
		fmt.Fprintf(&text, "%s: %s\n", row[0], row[1])
	}
	fmt.Fprintf(&text, "\n%s\n", l.Path)
	if l.Calendar != "" {
		fmt.Fprintf(&text, "Add to calendar: %s\n", l.Calendar)
	}
	if l.Footnote != "" {
		fmt.Fprintf(&text, "\n%s\n", l.Footnote)
	}
	return html.String(), text.String()
}

func (l Letter) Message(subject string, to, cc, replyTo []string) Message {
	m := Message{To: config.NormalizeEmails(to), ReplyTo: config.NormalizeEmails(replyTo), Subject: subject}
	m.CC = slices.DeleteFunc(config.NormalizeEmails(cc), func(e string) bool { return slices.Contains(m.To, e) })
	m.HTML, m.Text = l.Render()
	return m
}

func Post(ctx context.Context, s Sender, m Message) {
	if s == nil || len(m.To) == 0 {
		return
	}
	go func() {
		if err := s.Send(context.WithoutCancel(ctx), m); err != nil {
			slog.Error("mail: send", "error", err, "subject", m.Subject, "to", m.To)
		}
	}()
}
