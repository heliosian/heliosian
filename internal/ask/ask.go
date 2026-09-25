package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/artifacts"
	"heliosian/internal/auth"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const (
	shell                 = "web/ask/index.html"
	maxTurns              = 40
	maxMessageLength      = 4000
	maxConversationLength = 200000
	turnTimeout           = 3 * time.Minute
	recentDays            = 14
	recentLimit           = 20
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
	sources   Sources
	responder Responder
	recent    *ratelimit.Limiter
}

func Register(mux *http.ServeMux, sources Sources, responder Responder, recent *ratelimit.Limiter) {
	a := app{sources: sources, responder: responder, recent: recent}
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
		Conversation string          `json:"conversation"`
		Message      string          `json:"message"`
		Context      json.RawMessage `json:"context"`
		Known        []string        `json:"known"`
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
	links := newLinks()
	history, err := readContext(body.Context, links)
	if err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if asked(history) >= maxTurns || utf8.RuneCount(body.Context) >= maxConversationLength {
		http.Error(w, "this chat has run long; start a new one", http.StatusBadRequest)
		return
	}
	if !a.recent.Allow(email, time.Now()) {
		http.Error(w, "that's a lot of questions for one hour; try again a little later", http.StatusTooManyRequests)
		return
	}
	v := a.viewer(email)
	recent := recentDocuments(v)
	known := map[string]bool{}
	for _, key := range body.Known {
		known[key] = true
	}
	if len(history) == 0 {
		for _, d := range recent {
			known[d.Key] = true
		}
	}
	fresh := []*artifacts.Document{}
	for _, d := range recent {
		if !known[d.Key] {
			fresh = append(fresh, d)
			known[d.Key] = true
		}
	}
	listed := []*artifacts.Document{}
	for _, d := range v.documents().Documents {
		if known[d.Key] && !slices.Contains(fresh, d) {
			listed = append(listed, d)
		}
	}
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
	ctx, cancel := context.WithTimeout(r.Context(), turnTimeout)
	defer cancel()
	messages := append(history, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(links.shorten(message))))
	if len(fresh) > 0 {
		messages = append(messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleSystem, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(links.shorten(arrivals(v, fresh)))}})
	}
	req := Request{
		System:   systemBlocks(v, listed, links),
		Messages: messages,
		Tools:    definitions(),
		Run: func(ctx context.Context, name string, input json.RawMessage) (string, error) {
			out, err := v.run(ctx, name, links.expandInput(input))
			if err != nil {
				return "", errors.New(links.shorten(err.Error()))
			}
			return links.shorten(out), nil
		},
		Label: label,
	}
	started := time.Now()
	out := &expander{links: links, emit: emit, cards: v.linkCard, sent: map[string]bool{}}
	reply, err := a.responder.Respond(ctx, req, out.send)
	out.flush()
	if err != nil && r.Context().Err() != nil {
		slog.InfoContext(r.Context(), "ask: stopped", "conversation", body.Conversation, "turn", asked(history)+1, "took", time.Since(started).Round(time.Millisecond))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] ask: answer failed", "conversation", body.Conversation, "error", err)
		emit("error", map[string]string{"message": "Something went wrong answering that; try again in a moment."})
		return
	}
	turns := asked(history) + 1
	keys := []string{}
	for _, d := range v.documents().Documents {
		if known[d.Key] {
			keys = append(keys, d.Key)
		}
	}
	slog.InfoContext(r.Context(), "ask: answered", "conversation", body.Conversation, "turn", turns, "rounds", reply.Usage.Rounds, "tools", len(reply.Tools), "new_documents", len(fresh),
		"input_tokens", reply.Usage.Input, "cached_tokens", reply.Usage.Cached, "output_tokens", reply.Usage.Output, "took", time.Since(started).Round(time.Millisecond))
	emit("done", map[string]any{"messages": writeContext(reply.Messages[len(history):], links), "known": keys, "turns": turns, "text": links.expand(reply.Text), "tools": reply.Tools, "usage": reply.Usage})
}

func readContext(raw json.RawMessage, links *links) ([]anthropic.BetaMessageParam, error) {
	history := []anthropic.BetaMessageParam{}
	if len(raw) == 0 {
		return history, nil
	}
	var generic any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	shortened, err := json.Marshal(mapStrings(generic, links.shorten))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(shortened, &history); err != nil {
		return nil, err
	}
	return history, nil
}

func writeContext(messages []anthropic.BetaMessageParam, links *links) json.RawMessage {
	encoded, err := json.Marshal(messages)
	if err != nil {
		panic(err)
	}
	var generic any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		panic(err)
	}
	expanded, err := json.Marshal(mapStrings(generic, links.expandAll))
	if err != nil {
		panic(err)
	}
	return expanded
}

func asked(history []anthropic.BetaMessageParam) int {
	n := 0
	for _, m := range history {
		if m.Role == anthropic.BetaMessageParamRoleUser && len(m.Content) > 0 && m.Content[0].OfText != nil {
			n++
		}
	}
	return n
}
