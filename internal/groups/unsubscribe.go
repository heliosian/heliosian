package groups

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

func token(key []byte, name, email string) string {
	payload := name + "|" + email
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signature(key, payload)
}

func signature(key []byte, payload string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseToken(key []byte, t string) (name, email string, ok bool) {
	encoded, sig, found := strings.Cut(t, ".")
	if !found {
		return "", "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", false
	}
	payload := string(raw)
	if !hmac.Equal([]byte(signature(key, payload)), []byte(sig)) {
		return "", "", false
	}
	name, email, found = strings.Cut(payload, "|")
	if !found || name == "" || email == "" {
		return "", "", false
	}
	return name, email, true
}

const unsubscribePage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s · Helios Loop</title>
<link rel="icon" href="/brand/icon-192.png">
<link rel="stylesheet" href="/fonts/fonts.css">
<style>
body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center; background: #0f4e54; color: #0d0d0d; font-family: Roboto, -apple-system, BlinkMacSystemFont, system-ui, sans-serif; font-size: 16px; line-height: 1.4; }
.card { background: #fff; border-radius: 14px; padding: 28px 32px; max-width: 440px; margin: 24px; }
h1 { font-family: Montserrat, Roboto, sans-serif; font-size: 24px; font-weight: 400; margin: 0 0 12px; color: #0f4e54; }
p { margin: 0 0 16px; }
.address { color: #647071; font-size: 14px; }
button { font: inherit; font-weight: 600; padding: 11px 20px; border-radius: 8px; border: 1px solid #a4c21e; background: #a4c21e; color: #082b2e; cursor: pointer; }
button:hover { background: #88a700; border-color: #88a700; }
</style>
</head>
<body>
<div class="card">
<h1>%s</h1>
%s
</div>
</body>
</html>
`

func (a app) unsubscribeGroup(w http.ResponseWriter, r *http.Request) (*Group, string, bool) {
	name, email, ok := parseToken(a.mail.Key, r.PathValue("token"))
	if !ok {
		http.Error(w, "this link is not one Helios Loop made", http.StatusNotFound)
		return nil, "", false
	}
	g := a.cache.Model().Group(name)
	if g == nil {
		http.Error(w, "this group is gone", http.StatusNotFound)
		return nil, "", false
	}
	return g, email, true
}

func writePage(w http.ResponseWriter, title, heading, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, unsubscribePage, html.EscapeString(title), html.EscapeString(heading), body)
}

func (a app) unsubscribePage(w http.ResponseWriter, r *http.Request) {
	g, email, ok := a.unsubscribeGroup(w, r)
	if !ok {
		return
	}
	if g.HasExcluded(email) {
		writePage(w, g.Title, "Already unsubscribed", fmt.Sprintf(`<p>%s gets no mail from %s.</p><p class="address">A manager of the group can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g.Title)))
		return
	}
	body := fmt.Sprintf(`<p>Stop getting mail from <strong>%s</strong> at %s?</p><p class="address">%s</p><form method="post"><button type="submit">Unsubscribe</button></form>`,
		html.EscapeString(g.Title), html.EscapeString(email), html.EscapeString(g.Address()))
	writePage(w, g.Title, "Unsubscribe", body)
}

func (a app) unsubscribeAddress(ctx context.Context, g *Group, email, how string) error {
	if g.HasExcluded(email) {
		return nil
	}
	when := time.Now().Format(time.RFC3339)
	note := "Unsubscribed by " + how
	next := *g
	next.Excluded = append(slices.Clone(g.Excluded), Excluded{Email: email, Note: note, When: when})
	tables := a.cache.Tables().withGroup(next)
	model, err := BuildModel(tables)
	if err != nil {
		return err
	}
	applied := make(chan struct{})
	a.queue.Add(func() {
		a.cache.set(tables, model)
		close(applied)
		if err := a.writer.Append(appName, excludedTab, []string{g.Name, email, note, when}); err != nil {
			slog.ErrorContext(ctx, "groups: unsubscribe write", "error", err)
			return
		}
		if err := a.logChange(email, "unsubscribe", g.Name, email+" by "+how); err != nil {
			slog.ErrorContext(ctx, "groups: unsubscribe log", "error", err)
		}
	})
	<-applied
	slog.InfoContext(ctx, "groups: unsubscribed", "group", g.Name, "email", email, "how", how)
	return nil
}

func (a app) unsubscribe(w http.ResponseWriter, r *http.Request) {
	g, email, ok := a.unsubscribeGroup(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	oneClick := r.PostForm.Get("List-Unsubscribe") == "One-Click"
	how := "the page"
	if oneClick {
		how = "one-click"
	}
	if err := a.unsubscribeAddress(r.Context(), g, email, how); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if oneClick {
		w.WriteHeader(http.StatusOK)
		return
	}
	writePage(w, g.Title, "Unsubscribed", fmt.Sprintf(`<p>%s gets no more mail from %s.</p><p class="address">A manager of the group can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g.Title)))
}

func (a app) unsubscribeByMail(ctx context.Context, subject, sender string) {
	tok := strings.TrimSpace(subject)
	for _, prefix := range []string{"re:", "fwd:", "fw:"} {
		if strings.HasPrefix(strings.ToLower(tok), prefix) {
			tok = strings.TrimSpace(tok[len(prefix):])
		}
	}
	name, email, ok := parseToken(a.mail.Key, tok)
	if !ok {
		slog.WarnContext(ctx, "groups: unsubscribe mail with no token", "sender", sender, "subject", subject)
		return
	}
	g := a.cache.Model().Group(name)
	if g == nil {
		slog.WarnContext(ctx, "groups: unsubscribe mail for no group", "group", name, "email", email)
		return
	}
	if err := a.unsubscribeAddress(ctx, g, email, "mail from "+strings.ToLower(addressOf(sender))); err != nil {
		slog.ErrorContext(ctx, "groups: unsubscribe by mail", "group", name, "email", email, "error", err)
	}
}
