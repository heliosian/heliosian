package db

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	LoopDomain       = "loop.heliosian.com"
	mailerSystem     = "mailer"
	unsubscribeLocal = "unsubscribe"
	bounceFrom       = "HCA-Team <team@" + LoopDomain + ">"
	mailType         = "message/rfc822"
	postReceived     = "received"
	postSent         = "sent"
	postDropped      = "dropped"
	postFailed       = "failed"
	maxEventBody     = 1 << 20
)

type ListMailConfig struct {
	Sender     *mail.Mailgun
	SigningKey string
	Key        []byte
	Base       string
}

type ListMailer struct {
	s         *Store
	queue     *store.Queue
	pics      *Pictures
	mail      ListMailConfig
	receiving sync.Mutex
	mu        sync.Mutex
	queued    map[string]bool
	work      chan string
	pending   []Edit
	flushing  bool
}

func RegisterListMail(mux *http.ServeMux, s *Store, queue *store.Queue, pics *Pictures, lm ListMailConfig) *ListMailer {
	l := &ListMailer{s: s, queue: queue, pics: pics, mail: lm, queued: map[string]bool{}, work: make(chan string, 256)}
	mux.HandleFunc("POST /hooks/mail/mime", l.inbound)
	mux.HandleFunc("POST /hooks/events", l.events)
	mux.HandleFunc("GET /open/unsubscribe/{token}", l.unsubscribePage)
	mux.HandleFunc("POST /open/unsubscribe/{token}", l.unsubscribe)
	go l.run()
	return l
}

func (l *ListMailer) Start() {
	resumed := 0
	for _, post := range l.s.Model().Table("MESSAGE").All() {
		if receivedPost(post) && post["state"] == postReceived {
			l.enqueue(post["id"])
			resumed++
		}
	}
	slog.Info("list mail:resumed posts not yet sent on", "posts", resumed)
}

func (l *ListMailer) ready() bool {
	return l.mail.SigningKey != ""
}

func (l *ListMailer) write(ctx context.Context, edits ...Edit) ([]string, error) {
	return Write(ctx, l.s, l.queue, l.pics, access.System(mailerSystem), Env{System: mailerSystem, Now: time.Now()}, Batch{Batch: edits})
}

func (l *ListMailer) enqueue(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.queued[id] {
		return
	}
	l.queued[id] = true
	l.work <- id
}

func (l *ListMailer) run() {
	for id := range l.work {
		l.forward(context.Background(), id)
		l.mu.Lock()
		delete(l.queued, id)
		l.mu.Unlock()
	}
}

func stamp(t time.Time) string {
	return t.In(School).Format(cells.StampFormat)
}

func isMailList(g store.Row) bool {
	mail, _ := cells.YesNo(g["mail"], false)
	return g["kind"] == "group" && mail && g["parent"] == "" && g["url"] == "" && g["status"] != "closed"
}

func (m *Model) mailList(local string) store.Row {
	local = strings.ToLower(strings.TrimSpace(local))
	groups := m.Table("GROUP")
	for _, g := range groups.All() {
		if isMailList(g) && strings.EqualFold(g["slug"], local) {
			return g
		}
	}
	for _, a := range m.Table("ALIAS").All() {
		if !strings.EqualFold(a["alias"], local) {
			continue
		}
		if g, ok := groups.Get(a["target"]); ok && isMailList(g) {
			return g
		}
	}
	return nil
}

func (m *Model) personAt(address string) string {
	address = strings.ToLower(strings.TrimSpace(address))
	emails := m.Shown("PERSON_EMAIL")
	for _, guest := range []string{"No", "Yes"} {
		if row, ok := emails.Find(address, guest); ok {
			return row["person"]
		}
	}
	return ""
}

func (m *Model) addressOf(person string) string {
	emails := m.Shown("PERSON_EMAIL").Referencing("person", person)
	for _, e := range emails {
		if primary, _ := cells.YesNo(e["primary"], false); primary {
			return e["address"]
		}
	}
	if len(emails) == 0 {
		return ""
	}
	return emails[0]["address"]
}

func (m *Model) effectivelyIn(group, person string) bool {
	return slices.ContainsFunc(m.effectiveRows(group), func(row store.Row) bool { return row["person"] == person })
}

func (m *Model) runsGroup(group, person string) bool {
	for seen := map[string]bool{}; group != "" && !seen[group]; {
		seen[group] = true
		g, ok := m.Table("GROUP").Get(group)
		if !ok {
			return false
		}
		if g["managed_by"] != "" && m.effectivelyIn(g["managed_by"], person) {
			return true
		}
		group = g["parent"]
	}
	return false
}

func (m *Model) mayPost(g store.Row, sender string, reply bool) bool {
	audience := g["posting"]
	if reply {
		audience = g["replying"]
	}
	switch audience {
	case "members":
		return sender != "" && (m.effectivelyIn(g["id"], sender) || m.runsGroup(g["id"], sender))
	case "managers":
		return sender != "" && m.runsGroup(g["id"], sender)
	}
	return true
}

func (m *Model) repliesTo(group string, lines []mail.HeaderLine) bool {
	sent := map[string]bool{}
	for _, post := range m.Table("MESSAGE").Referencing("group", group) {
		if receivedPost(post) && post["state"] == postSent && post["header_id"] != "" {
			sent[messageKey(post["header_id"])] = true
		}
	}
	return slices.ContainsFunc(referenced(lines), func(id string) bool { return sent[id] })
}

func (m *Model) postOf(group, content string) bool {
	return slices.ContainsFunc(m.Table("MESSAGE").Referencing("content", content), func(post store.Row) bool {
		return receivedPost(post) && post["group"] == group
	})
}

func (l *ListMailer) inbound(w http.ResponseWriter, r *http.Request) {
	if !l.ready() {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	fields, err := mail.Notification(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := mail.VerifyNotification(l.mail.SigningKey, fields, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "list mail:inbound call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	m := l.s.Model()
	lists := []string{}
	for _, recipient := range strings.Split(fields["recipient"], ",") {
		local, domain, _ := strings.Cut(strings.ToLower(mail.AddressOf(recipient)), "@")
		if domain != LoopDomain {
			continue
		}
		if local == unsubscribeLocal {
			l.unsubscribeByMail(r.Context(), fields["subject"], fields["sender"])
			continue
		}
		g := m.mailList(local)
		if g == nil {
			slog.WarnContext(r.Context(), "list mail:mail for no list", "local", local)
			continue
		}
		if !slices.Contains(lists, g["id"]) {
			lists = append(lists, g["id"])
		}
	}
	raw := []byte(fields["body-mime"])
	if len(raw) == 0 {
		slog.WarnContext(r.Context(), "list mail:inbound call carries no body-mime", "recipient", fields["recipient"])
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := l.received(r.Context(), raw, lists); err != nil {
		slog.ErrorContext(r.Context(), "list mail:post not recorded", "recipient", fields["recipient"], "error", err)
		http.Error(w, "not recorded", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (l *ListMailer) received(ctx context.Context, raw []byte, lists []string) error {
	l.receiving.Lock()
	defer l.receiving.Unlock()
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	m := l.s.Model()
	content, found := m.Table("CONTENT").Find(hash, mailType)
	fresh := []string{}
	for _, g := range lists {
		if !found || !m.postOf(g, content["id"]) {
			fresh = append(fresh, g)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	edits := []Edit{}
	ref := content["id"]
	if !found {
		name := "content/" + hash
		if err := l.pics.bucket.Put(ctx, name, mailType, raw); err != nil {
			return fmt.Errorf("store the post: %w", err)
		}
		edits = append(edits, Edit{Insert: "CONTENT", As: "content", Row: map[string]any{"hash": hash, "blob": name, "mime": mailType, "size": strconv.Itoa(len(raw))}})
		ref = "@content"
	}
	lines, _ := mail.SplitMessage(raw)
	from := strings.ToLower(mail.AddressOf(mail.Header(lines, "from")))
	at := stamp(time.Now())
	for _, g := range fresh {
		row := map[string]any{"direction": "in", "kind": "post", "group": g, "content": ref, "created": at, "state": postReceived, "subject": decodeHeader(mail.Header(lines, "subject"))}
		if from != "" {
			row["from_address"] = from
		}
		if person := m.personAt(from); person != "" {
			row["from_person"] = person
		}
		if id := messageID(lines); id != "" {
			row["header_id"] = id
		}
		edits = append(edits, Edit{Insert: "MESSAGE", Row: row})
	}
	ids, err := l.write(ctx, edits...)
	if err != nil {
		if !found {
			dropUnheld(l.s, l.pics.bucket, []string{"content/" + hash})
		}
		return fmt.Errorf("record the post: %w", err)
	}
	for _, id := range ids[len(ids)-len(fresh):] {
		slog.InfoContext(ctx, "list mail:post received", "message", id, "hash", hash)
		l.enqueue(id)
	}
	return nil
}

func (l *ListMailer) forward(ctx context.Context, id string) {
	m := l.s.Model()
	post, ok := m.Table("MESSAGE").Get(id)
	if !ok || !receivedPost(post) || post["state"] != postReceived {
		return
	}
	log := slog.With("message", id, "group", post["group"])
	finish := func(state, detail string, edits ...Edit) {
		edits = append(edits, Edit{Set: id, Cells: map[string]any{"state": state, "detail": detail}})
		if _, err := l.write(ctx, edits...); err != nil {
			log.Error("list mail:post not recorded", "state", state, "error", err)
		}
	}
	fail := func(step string, err error) {
		log.Error("list mail:forward failed", "step", step, "error", err)
		finish(postFailed, step+": "+err.Error())
	}
	g, ok := m.Table("GROUP").Get(post["group"])
	if !ok || !isMailList(g) {
		fail("list", errors.New("the list is gone"))
		return
	}
	if !l.ready() {
		fail("mail", errors.New("mail is not set up"))
		return
	}
	content, ok := m.Table("CONTENT").Get(post["content"])
	if !ok {
		fail("content", errors.New("the post holds no mail"))
		return
	}
	raw, _, err := l.pics.bucket.Get(ctx, content["blob"])
	if err != nil {
		fail("content", err)
		return
	}
	lines, body := mail.SplitMessage(raw)
	if reason := held(lines); reason != "" {
		log.Info("list mail:post held", "reason", reason)
		finish(postDropped, reason)
		return
	}
	if reason := mail.Authenticated(lines); reason != "" {
		log.Info("list mail:post not authenticated", "reason", reason, "from", mail.Header(lines, "from"))
		finish(postDropped, reason)
		return
	}
	sender := m.personAt(mail.AddressOf(mail.Header(lines, "from")))
	reply := m.repliesTo(g["id"], lines)
	if !m.mayPost(g, sender, reply) {
		audience, verb := g["posting"], "post"
		if reply {
			audience, verb = g["replying"], "reply"
		}
		log.Info("list mail:post refused", "from", sender, "reply", reply, "audience", audience)
		if err := l.bounce(ctx, g, lines, audience, verb); err != nil {
			log.Error("list mail:bounce failed", "error", err)
		}
		finish(postDropped, "only the email list's "+audience+" may "+verb)
		return
	}
	head, err := rewrite(lines, g)
	if err != nil {
		fail("rewrite", err)
		return
	}
	out := map[string]any{"direction": "out", "kind": "post", "group": g["id"], "parent": id, "created": stamp(time.Now())}
	for _, column := range []string{"subject", "header_id", "from_person"} {
		if post[column] != "" {
			out[column] = post[column]
		}
	}
	ids, err := l.write(ctx, Edit{Insert: "MESSAGE", Row: out})
	if err != nil {
		fail("record", err)
		return
	}
	copies, failures := []Edit{}, []string{}
	members := m.effectiveRows(g["id"])
	for _, member := range members {
		address := m.addressOf(member["person"])
		if address == "" {
			failures = append(failures, member["person"]+": no address")
			continue
		}
		tok := unsubscribeToken(l.mail.Key, g["slug"], address)
		unsubscribe := "List-Unsubscribe: <mailto:" + unsubscribeLocal + "@" + LoopDomain + "?subject=" + tok + ">, <" + l.mail.Base + "/open/unsubscribe/" + tok + ">"
		msg := render(head, []string{unsubscribe, "List-Unsubscribe-Post: List-Unsubscribe=One-Click"}, body)
		if err := l.mail.Sender.SendRaw(ctx, listAddress(g), []string{address}, msg); err != nil {
			log.Error("list mail:send failed", "to", address, "error", err)
			failures = append(failures, address+": "+err.Error())
			continue
		}
		at := stamp(time.Now())
		copies = append(copies, Edit{Insert: "RECIPIENT", Row: map[string]any{"message": ids[0], "person": member["person"], "created": at, "sent": at}})
	}
	log.Info("list mail:forwarded", "members", len(members), "sent", len(copies), "failed", len(failures))
	state := postSent
	if len(copies) == 0 && len(members) > 0 {
		state = postFailed
	}
	finish(state, strings.Join(failures, "; "), copies...)
}

var posters = map[string]string{"members": "the people on it and its managers", "managers": "its managers"}

func (l *ListMailer) bounce(ctx context.Context, g store.Row, lines []mail.HeaderLine, audience, verb string) error {
	to := mail.AddressOf(mail.Header(lines, "from"))
	subject := decodeHeader(mail.Header(lines, "subject"))
	text := fmt.Sprintf("Your message to %s, “%s”, was not sent to the email list: only %s can %s to it.", listAddress(g), subject, posters[audience], verb)
	headers := map[string]string{"Auto-Submitted": "auto-replied", loopHeader: g["slug"]}
	if id := mail.Header(lines, "message-id"); id != "" {
		headers["In-Reply-To"] = id
		headers["References"] = id
	}
	raw := mail.Compose(bounceFrom, mail.Message{To: []string{to}, Subject: "Not delivered: " + subject, Text: text, HTML: "<p>" + html.EscapeString(text) + "</p>", Headers: headers})
	return l.mail.Sender.SendRaw(ctx, bounceFrom, []string{to}, []byte(raw))
}

func (l *ListMailer) events(w http.ResponseWriter, r *http.Request) {
	if !l.ready() {
		http.Error(w, "mail is not set up", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxEventBody))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var event struct {
		Signature struct {
			Timestamp string `json:"timestamp"`
			Token     string `json:"token"`
			Signature string `json:"signature"`
		} `json:"signature"`
		Data struct {
			Event     string  `json:"event"`
			Timestamp float64 `json:"timestamp"`
			Severity  string  `json:"severity"`
			Recipient string  `json:"recipient"`
			Reason    string  `json:"reason"`
			Message   struct {
				Headers struct {
					MessageID string `json:"message-id"`
					From      string `json:"from"`
				} `json:"headers"`
			} `json:"message"`
			Status struct {
				Message     string `json:"message"`
				Description string `json:"description"`
			} `json:"delivery-status"`
		} `json:"event-data"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if err := mail.VerifyMailgun(l.mail.SigningKey, event.Signature.Timestamp, event.Signature.Token, event.Signature.Signature, time.Now()); err != nil {
		slog.WarnContext(r.Context(), "list mail:event call refused", "error", err)
		http.Error(w, "signature", http.StatusNotAcceptable)
		return
	}
	d := event.Data
	detail := d.Status.Description
	if detail == "" {
		detail = d.Status.Message
	}
	if detail == "" {
		detail = d.Reason
	}
	when := stamp(time.UnixMilli(int64(d.Timestamp * 1000)))
	switch {
	case d.Event == "delivered":
		l.delivery(d.Message.Headers.From, d.Message.Headers.MessageID, d.Recipient, map[string]any{"delivered": when})
	case d.Event == "failed" && d.Severity == "permanent":
		l.delivery(d.Message.Headers.From, d.Message.Headers.MessageID, d.Recipient, map[string]any{"failed": when, "detail": detail})
	case d.Event == "failed", d.Event == "complained":
		slog.WarnContext(r.Context(), "list mail:delivery trouble", "event", d.Event, "from", d.Message.Headers.From, "to", d.Recipient, "detail", detail)
	}
	w.WriteHeader(http.StatusOK)
}

func (l *ListMailer) delivery(from, messageID, recipient string, set map[string]any) {
	m := l.s.Model()
	local, domain, _ := strings.Cut(strings.ToLower(mail.AddressOf(from)), "@")
	key := messageKey(messageID)
	if domain != LoopDomain || key == "" {
		return
	}
	g := m.mailList(local)
	if g == nil {
		return
	}
	person := m.personAt(mail.AddressOf(recipient))
	for _, post := range m.Table("MESSAGE").Referencing("group", g["id"]) {
		if post["direction"] != "out" || post["kind"] != "post" || messageKey(post["header_id"]) != key {
			continue
		}
		for _, c := range m.Table("RECIPIENT").Referencing("message", post["id"]) {
			if c["person"] == person {
				l.record(Edit{Set: c["id"], Cells: set})
				return
			}
		}
	}
}

func (l *ListMailer) record(e Edit) {
	l.mu.Lock()
	l.pending = append(l.pending, e)
	start := !l.flushing
	l.flushing = true
	l.mu.Unlock()
	if start {
		go l.flush()
	}
}

func (l *ListMailer) flush() {
	for {
		l.mu.Lock()
		edits := l.pending
		l.pending = nil
		if len(edits) == 0 {
			l.flushing = false
			l.mu.Unlock()
			return
		}
		l.mu.Unlock()
		if _, err := l.write(context.Background(), edits...); err != nil {
			slog.Error("list mail:deliveries not recorded", "edits", len(edits), "error", err)
		}
	}
}

func listAddress(g store.Row) string {
	return g["slug"] + "@" + LoopDomain
}

func unsubscribeToken(key []byte, name, email string) string {
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

func writePage(w http.ResponseWriter, title, heading, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, unsubscribePage, html.EscapeString(title), html.EscapeString(heading), body)
}

func (m *Model) listNamed(slug string) store.Row {
	for _, g := range m.Table("GROUP").All() {
		if isMailList(g) && g["slug"] == slug {
			return g
		}
	}
	return nil
}

func (l *ListMailer) tokenList(w http.ResponseWriter, r *http.Request) (store.Row, string, bool) {
	name, email, ok := parseToken(l.mail.Key, r.PathValue("token"))
	if !ok {
		http.Error(w, "this link is not one Helios Loop made", http.StatusNotFound)
		return nil, "", false
	}
	g := l.s.Model().listNamed(name)
	if g == nil {
		http.Error(w, "this email list is gone", http.StatusNotFound)
		return nil, "", false
	}
	return g, email, true
}

func (m *Model) unsubscribed(group, person string) bool {
	row, ok := m.Table("MEMBER").Find(group, person)
	return ok && memberAs(row) == "excluded"
}

func (l *ListMailer) unsubscribePage(w http.ResponseWriter, r *http.Request) {
	g, email, ok := l.tokenList(w, r)
	if !ok {
		return
	}
	m := l.s.Model()
	if person := m.personAt(email); person != "" && m.unsubscribed(g["id"], person) {
		writePage(w, g["name"], "Already unsubscribed", fmt.Sprintf(`<p>%s gets no mail from %s.</p><p class="address">A manager of the email list can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g["name"])))
		return
	}
	body := fmt.Sprintf(`<p>Stop getting mail from <strong>%s</strong> at %s?</p><p class="address">%s</p><form method="post"><button type="submit">Unsubscribe</button></form>`,
		html.EscapeString(g["name"]), html.EscapeString(email), html.EscapeString(listAddress(g)))
	writePage(w, g["name"], "Unsubscribe", body)
}

func (l *ListMailer) unsubscribeAddress(ctx context.Context, g store.Row, email, how string) error {
	m := l.s.Model()
	person := m.personAt(email)
	if person == "" {
		return access.Missing("no one has the address %s", email)
	}
	note := "Unsubscribed by " + how
	row, found := m.Table("MEMBER").Find(g["id"], person)
	var edit Edit
	switch {
	case found && memberAs(row) == "excluded":
		return nil
	case found:
		edit = Edit{Set: row["id"], Cells: map[string]any{"member": "excluded", "note": note}}
	default:
		edit = Edit{Insert: "MEMBER", Row: map[string]any{"group": g["id"], "person": person, "member": "excluded", "note": note, "added": stamp(time.Now())}}
	}
	if _, err := l.write(ctx, edit); err != nil {
		return err
	}
	slog.InfoContext(ctx, "list mail:unsubscribed", "group", g["slug"], "person", person, "how", how)
	return nil
}

func (l *ListMailer) unsubscribe(w http.ResponseWriter, r *http.Request) {
	g, email, ok := l.tokenList(w, r)
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
	if err := l.unsubscribeAddress(r.Context(), g, email, how); err != nil {
		serve.Error(w, r, err)
		return
	}
	if oneClick {
		w.WriteHeader(http.StatusOK)
		return
	}
	writePage(w, g["name"], "Unsubscribed", fmt.Sprintf(`<p>%s gets no more mail from %s.</p><p class="address">A manager of the email list can put you back on it.</p>`, html.EscapeString(email), html.EscapeString(g["name"])))
}

func (l *ListMailer) unsubscribeByMail(ctx context.Context, subject, sender string) {
	tok := strings.TrimSpace(subject)
	for _, prefix := range []string{"re:", "fwd:", "fw:"} {
		if strings.HasPrefix(strings.ToLower(tok), prefix) {
			tok = strings.TrimSpace(tok[len(prefix):])
		}
	}
	name, email, ok := parseToken(l.mail.Key, tok)
	if !ok {
		slog.WarnContext(ctx, "list mail:unsubscribe mail with no token", "sender", sender, "subject", subject)
		return
	}
	g := l.s.Model().listNamed(name)
	if g == nil {
		slog.WarnContext(ctx, "list mail:unsubscribe mail for no list", "group", name, "email", email)
		return
	}
	if err := l.unsubscribeAddress(ctx, g, email, "mail from "+strings.ToLower(mail.AddressOf(sender))); err != nil {
		slog.ErrorContext(ctx, "list mail:unsubscribe by mail", "group", name, "email", email, "error", err)
	}
}
