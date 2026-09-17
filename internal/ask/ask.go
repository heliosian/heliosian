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
)

// Sources is what the chat reads: each app's model as it stands when asked,
// the directory's view of a person, and the lists the other apps give them.
type Sources struct {
	Directory func() *who.Model
	Tags      func(owner string) map[string][]string
	Lists     func(email string) []who.List
	Calendar  func() *calendar.Model
	// CalendarDirectory is the calendar's own view of the directory, for
	// the default view it reads a person's events under.
	CalendarDirectory calendar.Directory
	Linked            func(email string) []calendar.Linked
	Team              func() *team.Model
	Celebrate         func() *celebrate.Model
	Loop              func() *loop.Model
	LoopSources       func() loop.Sources
	Links             func() []home.Category
	Alerts            func(email string) (stale int, privacy bool)
}

type app struct {
	sources       Sources
	responder     Responder
	conversations *store
	recent        *limiter
}

// Register wires the app: the one page, the model, and the chat itself.
// Every route sits behind sign-in.
func Register(mux *http.ServeMux, sources Sources, responder Responder) {
	a := app{sources: sources, responder: responder, conversations: newStore(), recent: newLimiter()}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /api/ask/model", a.model)
	mux.HandleFunc("POST /api/ask/chat", a.chat)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

// who is the signed-in person as the directory keys them.
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

// chat takes one message on a conversation and streams the answer back as
// server-sent events: start (the conversation's id), text (a piece of the
// answer), tool (what is being looked up, in words), done (the usage) and
// error. The browser holds every transcript and sends the conversation's
// turns along, so an id the server no longer knows - after a restart, or
// an hour's quiet - is rebuilt from them under a fresh id; a blank id with
// no turns starts a new one.
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
	conv := a.conversations.get(body.Conversation, email, now)
	if conv == nil {
		conv = a.conversations.start(email, systemBlocks(v), now)
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
	req := Request{
		System:   conv.system,
		Messages: append(conv.messages[:len(conv.messages):len(conv.messages)], anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(message))),
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
	conv.asked++
	conv.touched = time.Now()
	slog.InfoContext(r.Context(), "ask: answered", "conversation", conv.id, "turn", conv.asked, "rounds", reply.Usage.Rounds, "tools", len(reply.Tools),
		"input_tokens", reply.Usage.Input, "cached_tokens", reply.Usage.Cached, "output_tokens", reply.Usage.Output, "took", time.Since(started).Round(time.Millisecond))
	emit("done", map[string]any{"conversation": conv.id, "turns": conv.asked, "text": reply.Text, "tools": reply.Tools, "usage": reply.Usage})
}

// A turn is one side of the exchange as the browser keeps it: who spoke,
// what they said, and for an answer, what was looked up along the way.
type turn struct {
	Role  string   `json:"role"`
	Text  string   `json:"text"`
	Tools []string `json:"tools,omitempty"`
}

// A conversation is one person's chat as the API sees it: the system
// blocks frozen when it began, so the cached prefix holds turn after turn,
// and every message including the tool calls and their results. The
// browser keeps the transcript; this is the working copy.
type conversation struct {
	id       string
	email    string
	system   []anthropic.BetaTextBlockParam
	messages []anthropic.BetaMessageParam
	asked    int
	touched  time.Time
	busy     bool
}

// restore rebuilds the working copy from a transcript the browser kept:
// each question and its answer as plain text, in pairs, so the model reads
// what was said without the tool results and thinking that came with it -
// it looks things up again when it needs them. A question without an
// answer at the end is dropped, since the new message follows it.
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

// get is the conversation an id names when it is this person's and has
// been touched within the hour; idle conversations are dropped as it looks.
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

func (s *store) start(email string, system []anthropic.BetaTextBlockParam, now time.Time) *conversation {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	c := &conversation{id: hex.EncodeToString(raw), email: email, system: system, messages: []anthropic.BetaMessageParam{}, touched: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[c.id] = c
	return c
}

// claim marks a conversation as answering; a second message while it is
// gets refused rather than interleaved.
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

// limiter counts each person's messages over the last hour.
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
