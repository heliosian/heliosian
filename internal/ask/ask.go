// Package ask serves Helios Ask: a chat with Claude that knows the school, the signed-in person and their family, and reads the other apps' data through tools.
package ask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const (
	shell            = "web/ask/index.html"
	maxTurns         = 40
	messagesPerHour  = 30
	idle             = time.Hour
	maxMessageLength = 4000
	turnTimeout      = 3 * time.Minute
	recentDays       = 14
	recentLimit      = 20
)

type Sources struct {
	Directory         func() *who.Model
	Tags              func(owner string) map[string][]string
	Lists             func(email string) []who.List
	Calendar          func() *calendar.Model
	CalendarDirectory calendar.Directory
	Linked            func(email string) []calendar.Linked
	Team              func() *team.Model
	Celebrate         func() *celebrate.Model
	Loop              func() *loop.Model
	LoopSources       func() loop.Sources
	Links             func() []home.Category
	Alerts            func(email string) (stale int, privacy bool)
	Artifacts         func() *artifacts.Model
	Embedder          artifacts.Embedder
}

type app struct {
	sources       Sources
	responder     Responder
	conversations *store
	recent        *limiter
}

func Register(mux *http.ServeMux, sources Sources, responder Responder) {
	a := app{sources: sources, responder: responder, conversations: newStore(), recent: newLimiter()}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /api/ask/model", a.model)
	mux.HandleFunc("POST /api/ask/chat", a.chat)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) who(r *http.Request) string {
	return a.sources.Directory().Resolve(strings.ToLower(auth.Email(r)))
}

type user struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
}

type alerts struct {
	Stale   int  `json:"stale"`
	Privacy bool `json:"privacy"`
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	email := a.who(r)
	v := a.viewer(email)
	u := user{Email: email, Name: v.name(email), Initial: strings.ToUpper(email[:1])}
	if u.Name != "" {
		u.Initial = strings.ToUpper(u.Name[:1])
	}
	u.PhotoURL = v.directory.HeroPhoto(email)
	view := struct {
		User     user     `json:"user"`
		Alerts   alerts   `json:"alerts"`
		Starters []string `json:"starters"`
		MaxTurns int      `json:"maxTurns"`
	}{User: u, Starters: v.starters(), MaxTurns: maxTurns}
	view.Alerts.Stale, view.Alerts.Privacy = a.sources.Alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode ask model", "error", err)
	}
}

func (a app) chat(w http.ResponseWriter, r *http.Request) {
	email := a.who(r)
	var body struct {
		Conversation string `json:"conversation"`
		Message      string `json:"message"`
		Turns        []turn `json:"turns"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	message := strings.TrimSpace(body.Message)
	if message == "" || len([]rune(message)) > maxMessageLength {
		http.Error(w, "say something, in fewer than four thousand characters", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if !a.recent.allow(email, now) {
		http.Error(w, "that's a lot of questions for one hour; try again a little later", http.StatusTooManyRequests)
		return
	}
	v := a.viewer(email)
	recent := recentDocuments(v)
	conv := a.conversations.get(body.Conversation, email, now)
	if conv == nil {
		conv = a.conversations.start(email, systemBlocks(v, recent), recent, now)
		conv.restore(body.Turns)
	}
	if !a.conversations.claim(conv) {
		http.Error(w, "that conversation is still answering", http.StatusConflict)
		return
	}
	defer a.conversations.release(conv)
	if conv.asked >= maxTurns {
		http.Error(w, "this chat has run long; start a new one", http.StatusBadRequest)
		return
	}
	// The request logger wraps the writer; the controller reaches through
	// it to the one that flushes.
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	emit := func(kind string, data any) {
		encoded, err := json.Marshal(data)
		if err != nil {
			slog.ErrorContext(r.Context(), "encode ask event", "kind", kind, "error", err)
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded)
		if err := controller.Flush(); err != nil {
			slog.ErrorContext(r.Context(), "[ERROR] ask: flush", "error", err)
		}
	}
	emit("start", map[string]string{"conversation": conv.id})
	ctx, cancel := context.WithTimeout(r.Context(), turnTimeout)
	defer cancel()
	messages := append(conv.messages[:len(conv.messages):len(conv.messages)], anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(message)))
	fresh := conv.unseen(recent)
	if len(fresh) > 0 {
		messages = append(messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleSystem, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(arrivals(v, fresh))}})
	}
	req := Request{
		System:   conv.system,
		Messages: messages,
		Tools:    definitions(),
		Run:      v.run,
		Label:    label,
	}
	started := time.Now()
	reply, err := a.responder.Respond(ctx, req, emit)
	if err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] ask: answer failed", "conversation", conv.id, "error", err)
		emit("error", map[string]string{"message": "Something went wrong answering that; try again in a moment."})
		return
	}
	conv.messages = reply.Messages
	conv.see(fresh)
	conv.asked++
	conv.touched = time.Now()
	slog.InfoContext(r.Context(), "ask: answered", "conversation", conv.id, "turn", conv.asked, "rounds", reply.Usage.Rounds, "tools", len(reply.Tools), "new_documents", len(fresh),
		"input_tokens", reply.Usage.Input, "cached_tokens", reply.Usage.Cached, "output_tokens", reply.Usage.Output, "took", time.Since(started).Round(time.Millisecond))
	emit("done", map[string]any{"conversation": conv.id, "turns": conv.asked, "text": reply.Text, "tools": reply.Tools, "usage": reply.Usage})
}

type turn struct {
	Role  string   `json:"role"`
	Text  string   `json:"text"`
	Tools []string `json:"tools,omitempty"`
}

type conversation struct {
	id       string
	email    string
	system   []anthropic.BetaTextBlockParam
	messages []anthropic.BetaMessageParam
	known    map[string]bool
	asked    int
	touched  time.Time
	busy     bool
}

func (c *conversation) unseen(recent []*artifacts.Document) []*artifacts.Document {
	out := []*artifacts.Document{}
	for _, d := range recent {
		if !c.known[d.Key] {
			out = append(out, d)
		}
	}
	return out
}

func (c *conversation) see(docs []*artifacts.Document) {
	for _, d := range docs {
		c.known[d.Key] = true
	}
}

func (c *conversation) restore(turns []turn) {
	for i := 0; i+1 < len(turns); i += 2 {
		question, answer := strings.TrimSpace(turns[i].Text), strings.TrimSpace(turns[i+1].Text)
		if turns[i].Role != "user" || turns[i+1].Role != "assistant" || question == "" || answer == "" {
			continue
		}
		c.messages = append(c.messages,
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(question)),
			anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(answer)}})
		c.asked++
	}
}

type store struct {
	mu   sync.Mutex
	byID map[string]*conversation
}

func newStore() *store {
	return &store{byID: map[string]*conversation{}}
}

func (s *store) get(id, email string, now time.Time) *conversation {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, c := range s.byID {
		if !c.busy && now.Sub(c.touched) > idle {
			delete(s.byID, key)
		}
	}
	c := s.byID[id]
	if c == nil || c.email != email {
		return nil
	}
	return c
}

func (s *store) start(email string, system []anthropic.BetaTextBlockParam, recent []*artifacts.Document, now time.Time) *conversation {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	c := &conversation{id: hex.EncodeToString(raw), email: email, system: system, messages: []anthropic.BetaMessageParam{}, known: map[string]bool{}, touched: now}
	c.see(recent)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[c.id] = c
	return c
}

func (s *store) claim(c *conversation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.busy {
		return false
	}
	c.busy = true
	return true
}

func (s *store) release(c *conversation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.busy = false
}

type limiter struct {
	mu     sync.Mutex
	recent map[string][]time.Time
}

func newLimiter() *limiter {
	return &limiter{recent: map[string][]time.Time{}}
}

func (l *limiter) allow(email string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	kept := []time.Time{}
	for _, t := range l.recent[email] {
		if now.Sub(t) < time.Hour {
			kept = append(kept, t)
		}
	}
	if len(kept) >= messagesPerHour {
		l.recent[email] = kept
		return false
	}
	l.recent[email] = append(kept, now)
	return true
}
