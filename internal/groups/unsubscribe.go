package groups

import (
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
	if g.HasUnsubscribed(email) {
		writePage(w, g.Title, "Already unsubscribed", fmt.Sprintf(`<p>%s gets no mail from %s.</p><p class="address">A manager of the group can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g.Title)))
		return
	}
	body := fmt.Sprintf(`<p>Stop getting mail from <strong>%s</strong> at %s?</p><p class="address">%s</p><form method="post"><button type="submit">Unsubscribe</button></form>`,
		html.EscapeString(g.Title), html.EscapeString(email), html.EscapeString(g.Address()))
	writePage(w, g.Title, "Unsubscribe", body)
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
	if !g.HasUnsubscribed(email) {
		when := time.Now().Format(time.RFC3339)
		next := *g
		next.Unsubscribed = append(slices.Clone(g.Unsubscribed), Unsubscribed{Email: email, When: when})
		if !a.commit(r, w, a.cache.Tables().withGroup(next), func() error {
			if err := a.writer.Append(appName, unsubscribedTab, []string{g.Name, email, when}); err != nil {
				return err
			}
			return a.logChange(email, "unsubscribe", g.Name, email)
		}) {
			return
		}
		slog.InfoContext(r.Context(), "groups: unsubscribed", "group", g.Name, "email", email, "oneClick", oneClick)
	}
	if oneClick {
		w.WriteHeader(http.StatusOK)
		return
	}
	writePage(w, g.Title, "Unsubscribed", fmt.Sprintf(`<p>%s gets no more mail from %s.</p><p class="address">A manager of the group can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g.Title)))
}
