package ask

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

	"heliosian/internal/api"
	"heliosian/internal/model"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
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
	Registry *api.Registry[*model.Model]
	Now      func() time.Time
}

type app struct {
	sources Sources
	claude  *Claude
	recent  *ratelimit.Limiter
	chatKey []byte
}

func Register(mux *http.ServeMux, sources Sources, claude *Claude, recent *ratelimit.Limiter, chatKey []byte, about *sharecard.About) {
	a := app{sources: sources, claude: claude, recent: recent, chatKey: chatKey}
	mux.HandleFunc("GET /{$}", a.page)
	mux.Handle("GET /open/share/about.png", about)
	mux.HandleFunc("GET /api/ask/model", serve.JSON(a.model))
	mux.HandleFunc("GET /api/ask/key", a.key)
	mux.HandleFunc("POST /api/ask/chat", a.chat)
}

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, shell)
}

func (a app) turn(r *http.Request) *turn {
	return &turn{reg: a.sources.Registry, r: r, clock: a.sources.Now, found: newFound()}
}

func (a app) who(r *http.Request) string {
	return a.sources.Registry.Actor(r).Email
}

type user struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
}

type modelView struct {
	User     user     `json:"user"`
	Starters []string `json:"starters"`
	MaxTurns int      `json:"maxTurns"`
}

func (a app) model(r *http.Request, _ serve.None) (modelView, error) {
	t := a.turn(r)
	email := a.who(r)
	p, err := t.viewer()
	if err != nil {
		return modelView{}, err
	}
	u := user{Email: email, Name: s(p, "fullName"), Initial: strings.ToUpper(email[:1]), PhotoURL: s(p, "heroPhotoUrl")}
	if u.Name == "" {
		u.Name, _, _ = strings.Cut(email, "@")
	}
	u.Initial = strings.ToUpper(u.Name[:1])
	starters, err := t.starters()
	if err != nil {
		return modelView{}, err
	}
	return modelView{User: u, Starters: starters, MaxTurns: maxTurns}, nil
}

func (a app) key(w http.ResponseWriter, r *http.Request) {
	mac := hmac.New(sha256.New, a.chatKey)
	mac.Write([]byte(a.who(r)))
	w.Header().Set("Cache-Control", "no-store")
	serve.Write(w, r, http.StatusOK, map[string]string{"key": base64.StdEncoding.EncodeToString(mac.Sum(nil))})
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
	t := a.turn(r)
	recent, err := t.recentDocuments()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	known := map[string]bool{}
	for _, key := range body.Known {
		known[key] = true
	}
	if len(history) == 0 {
		for _, d := range recent {
			known[s(d, "id")] = true
		}
	}
	fresh := []document{}
	for _, d := range recent {
		if !known[s(d, "id")] {
			fresh = append(fresh, d)
		}
	}
	told := []string{}
	for key := range known {
		if !slices.ContainsFunc(fresh, func(d document) bool { return s(d, "id") == key }) {
			told = append(told, key)
		}
	}
	listed, err := t.documentsKnown(told)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slices.SortStableFunc(listed, func(x, y document) int { return strings.Compare(s(y, "date"), s(x, "date")) })
	system, err := systemBlocks(t, listed, links)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
			slog.ErrorContext(r.Context(), "ask: flush", "error", err)
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), turnTimeout)
	defer cancel()
	messages := append(history, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(links.shorten(message))))
	if len(fresh) > 0 {
		messages = append(messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleSystem, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(links.shorten(arrivals(t, fresh)))}})
	}
	req := Request{
		System:   system,
		Messages: messages,
		Tools:    definitions(),
		Run: func(ctx context.Context, name string, input json.RawMessage) (string, error) {
			out, err := t.run(ctx, name, links.expandInput(input))
			if err != nil {
				return "", errors.New(links.shorten(err.Error()))
			}
			return links.shorten(out), nil
		},
		Label: label,
	}
	started := time.Now()
	out := &expander{links: links, emit: emit, cards: t.linkCard, sent: map[string]bool{}}
	reply, err := a.claude.Respond(ctx, req, out.send)
	out.flush()
	if err != nil && r.Context().Err() != nil {
		slog.InfoContext(r.Context(), "ask: stopped", "conversation", body.Conversation, "turn", asked(history)+1, "took", time.Since(started).Round(time.Millisecond))
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "ask: answer failed", "conversation", body.Conversation, "error", err)
		emit("error", map[string]string{"message": "Something went wrong answering that; try again in a moment."})
		return
	}
	turns := asked(history) + 1
	keys := []string{}
	for _, d := range slices.Concat(listed, fresh) {
		keys = append(keys, s(d, "id"))
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
