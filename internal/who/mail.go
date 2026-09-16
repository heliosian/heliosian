package who

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"heliosian/internal/mail"
)

// baseURL is the site as the request reached it, for the links and the
// logo in a message - who.lab.heliosian.com today, who.heliosian.com later.
func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}

// sharedLetter is the one message the directory sends: word to somebody
// that a tag has been shared with them, with the way to it.
type sharedLetter struct {
	Base    string
	Heading string
	Intro   string
	Tag     string
	People  string
	Path    string
	Button  string
	Note    string
}

var sharedTemplate = template.Must(template.New("shared").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>{{.Heading}}</title></head>
<body style="margin:0;padding:0;background:#f3f6f6;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#1f2a2b;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f3f6f6;padding:24px 12px;">
<tr><td align="center">
<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="max-width:600px;width:100%;background:#ffffff;border-radius:16px;overflow:hidden;box-shadow:0 2px 10px rgba(0,0,0,0.06);">
<tr><td style="background:#244d53;padding:14px 24px;">
<table role="presentation" cellpadding="0" cellspacing="0"><tr>
<td style="vertical-align:middle;"><img src="{{.Base}}/brand/apps/who.png" width="28" height="28" alt="" style="display:block;border:0;"></td>
<td style="vertical-align:middle;padding-left:10px;color:#ffffff;font-weight:700;font-size:16px;letter-spacing:0.02em;">Helios Who?</td>
</tr></table>
</td></tr>
<tr><td style="padding:28px 28px 8px;">
<h1 style="margin:0 0 10px;font-size:24px;line-height:1.25;color:#244d53;">{{.Heading}}</h1>
<p style="margin:0 0 18px;font-size:16px;line-height:1.5;">{{.Intro}}</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:#f4f8f8;border-radius:12px;">
<tr><td style="padding:16px 18px;">
<div style="font-size:19px;font-weight:700;color:#244d53;">{{.Tag}}</div>
<div style="font-size:14px;color:#5c6b6c;margin-top:4px;">{{.People}}</div>
</td></tr>
</table>
</td></tr>
<tr><td style="padding:18px 28px 28px;">
<a href="{{.Path}}" style="display:inline-block;background:#244d53;color:#ffffff;text-decoration:none;font-weight:700;font-size:15px;padding:12px 20px;border-radius:10px;">{{.Button}}</a>
<p style="margin:18px 0 0;font-size:13.5px;line-height:1.5;color:#5c6b6c;">{{.Note}}</p>
</td></tr>
<tr><td style="padding:14px 28px;background:#f4f8f8;font-size:12.5px;color:#7a8788;">
Sent by Helios Who?, the Helios visual directory · <a href="{{.Base}}" style="color:#244d53;">{{.Base}}</a>
</td></tr>
</table>
</td></tr>
</table>
</body></html>`))

func (l sharedLetter) render() (string, string) {
	var html bytes.Buffer
	if err := sharedTemplate.Execute(&html, l); err != nil {
		slog.Error("mail: render", "error", err)
	}
	text := fmt.Sprintf("%s\n\n%s\n\n%s\n%s\n\n%s\n\n%s\n", l.Heading, l.Intro, l.Tag, l.People, l.Path, l.Note)
	return html.String(), text
}

// notifyShared tells someone a tag has just been shared with them - from
// the directory, replying to the owner - off the request; the outcome is
// logged. Nothing goes out when mail isn't set up.
func (t tagger) notifyShared(r *http.Request, owner, tag, manager string) {
	if t.mailer == nil {
		return
	}
	model := t.cache.Model()
	ownerName := model.DisplayName(owner)
	people := len(t.cache.Tags(owner)[tag])
	count := fmt.Sprintf("%d people", people)
	if people == 1 {
		count = "1 person"
	}
	base := baseURL(r)
	l := sharedLetter{
		Base:    base,
		Heading: fmt.Sprintf("%s shared a tag with you", ownerName),
		Intro:   fmt.Sprintf("%s has made you a manager of their tag in Helios Who?. You can tag and untag people on it just as they can, and it shows under \"Shared Tags\" beside your own.", ownerName),
		Tag:     tag,
		People:  count,
		Path:    base + "/people?shared=" + url.QueryEscape(owner+":"+tag),
		Button:  "Open the tag",
		Note:    fmt.Sprintf("The tag stays %s's - only they can delete it. If you'd rather not manage it, open it and choose Leave under Manage.", ownerName),
	}
	m := mail.Message{To: []string{manager}, ReplyTo: []string{owner}, Subject: fmt.Sprintf("%s shared the tag \"%s\" with you", ownerName, tag)}
	m.HTML, m.Text = l.render()
	go func() {
		if err := t.mailer.Send(context.WithoutCancel(r.Context()), m); err != nil {
			slog.Error("mail: send", "error", err, "subject", m.Subject, "to", m.To)
		}
	}()
}
