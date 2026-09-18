package ask

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

const (
	model     = "claude-opus-5"
	maxTokens = 16000
	maxRounds = 8
	refused   = "I can't help with that one here."
)

type Emitter func(kind string, data any)

type Request struct {
	System   []anthropic.BetaTextBlockParam
	Messages []anthropic.BetaMessageParam
	Tools    []anthropic.BetaToolUnionParam
	Run      func(ctx context.Context, name string, input json.RawMessage) (string, error)
	Label    func(name string) string
}

type Reply struct {
	Messages []anthropic.BetaMessageParam
	Text     string
	Tools    []string
	Usage    Usage
}

type Usage struct {
	Input  int64 `json:"input"`
	Cached int64 `json:"cached"`
	Output int64 `json:"output"`
	Rounds int   `json:"rounds"`
}

func (u *Usage) add(usage anthropic.BetaUsage) {
	u.Input += usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	u.Cached += usage.CacheReadInputTokens
	u.Output += usage.OutputTokens
	u.Rounds++
}

type toolCall struct {
	use    anthropic.BetaToolUseBlock
	result anthropic.BetaContentBlockParamUnion
}

type Responder interface {
	Respond(ctx context.Context, req Request, emit Emitter) (Reply, error)
}

type Claude struct {
	client anthropic.Client
}

func NewClaude(key string) *Claude {
	return &Claude{client: anthropic.NewClient(option.WithAPIKey(key))}
}

func (c *Claude) Respond(ctx context.Context, req Request, emit Emitter) (Reply, error) {
	reply := Reply{Messages: req.Messages, Tools: []string{}}
	text := &strings.Builder{}
	for round := 0; round < maxRounds; round++ {
		params := anthropic.BetaMessageNewParams{
			Model:        model,
			MaxTokens:    maxTokens,
			Betas:        []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
			Fallbacks:    anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
			System:       req.System,
			Messages:     reply.Messages,
			Tools:        req.Tools,
			OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffortMedium},
			CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}
		// The last round allowed has to be the answer, so the tools are
		// kept on offer for the history's sake but closed to use.
		if round == maxRounds-1 {
			params.ToolChoice = anthropic.BetaToolChoiceUnionParam{OfNone: &anthropic.BetaToolChoiceNoneParam{}}
		}
		stream := c.client.Beta.Messages.NewStreaming(ctx, params)
		msg := anthropic.BetaMessage{}
		// Words written before a tool call and words written after it are
		// kept as separate paragraphs in the text the browser stores.
		first := true
		for stream.Next() {
			event := stream.Current()
			if err := msg.Accumulate(event); err != nil {
				reply.Text = text.String()
				return reply, err
			}
			if delta, ok := event.AsAny().(anthropic.BetaRawContentBlockDeltaEvent); ok {
				if t, ok := delta.Delta.AsAny().(anthropic.BetaTextDelta); ok && t.Text != "" {
					if first && text.Len() > 0 {
						text.WriteString("\n\n")
					}
					first = false
					text.WriteString(t.Text)
					emit("text", t.Text)
				}
			}
		}
		if err := stream.Err(); err != nil {
			reply.Text = text.String()
			return reply, err
		}
		reply.Usage.add(msg.Usage)
		if msg.StopReason == anthropic.BetaStopReasonRefusal {
			text.WriteString(refused)
			emit("text", refused)
			reply.Messages = append(reply.Messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(refused)}})
			break
		}
		reply.Messages = append(reply.Messages, msg.ToParam())
		if msg.StopReason != anthropic.BetaStopReasonToolUse {
			break
		}
		calls := []*toolCall{}
		for _, block := range msg.Content {
			use, ok := block.AsAny().(anthropic.BetaToolUseBlock)
			if !ok {
				continue
			}
			words := req.Label(use.Name)
			if !slices.Contains(reply.Tools, words) {
				reply.Tools = append(reply.Tools, words)
				emit("tool", words)
			}
			calls = append(calls, &toolCall{use: use})
		}
		wg := sync.WaitGroup{}
		for _, c := range calls {
			wg.Go(func() {
				out, err := req.Run(ctx, c.use.Name, json.RawMessage(c.use.JSON.Input.Raw()))
				if err != nil {
					c.result = anthropic.NewBetaToolResultBlock(c.use.ID, err.Error(), true)
					return
				}
				c.result = anthropic.NewBetaToolResultBlock(c.use.ID, out, false)
			})
		}
		wg.Wait()
		results := []anthropic.BetaContentBlockParamUnion{}
		for _, c := range calls {
			results = append(results, c.result)
		}
		reply.Messages = append(reply.Messages, anthropic.NewBetaUserMessage(results...))
	}
	reply.Text = text.String()
	return reply, nil
}

type Fake struct{}

const fakeAnswer = "This is the sample server, so nothing is asking Claude. With an Anthropic key in creds/anthropic.key the real answer streams in here, drawn from the directory, the calendar, HCA-Team, Celebrate, Loop and Heliosian's links."

func (Fake) Respond(ctx context.Context, req Request, emit Emitter) (Reply, error) {
	words := req.Label("day_plan")
	emit("tool", words)
	out, err := req.Run(ctx, "day_plan", json.RawMessage(`{}`))
	if err != nil {
		out = err.Error()
	}
	text := &strings.Builder{}
	for i, word := range strings.Fields(fakeAnswer) {
		select {
		case <-ctx.Done():
			return Reply{Text: text.String()}, ctx.Err()
		case <-time.After(30 * time.Millisecond):
		}
		if i > 0 {
			word = " " + word
		}
		text.WriteString(word)
		emit("text", word)
	}
	tail := "\n\nToday's plan, as the tool returned it: " + out
	if len(tail) > 600 {
		tail = tail[:600] + "…"
	}
	text.WriteString(tail)
	emit("text", tail)
	messages := append(req.Messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(text.String())}})
	return Reply{Messages: messages, Text: text.String(), Tools: []string{words}, Usage: Usage{Rounds: 1}}, nil
}
