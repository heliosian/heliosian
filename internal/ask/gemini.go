package ask

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"google.golang.org/api/option"
	transport "google.golang.org/api/transport/http"
)

// Gemini answers through Gemini on Vertex AI, as the Google credentials at
// hand - the same identity the embeddings already go out as, so it needs no
// key of its own. The model lives in the global location rather than a region
// of its own, unlike the embedding model.
//
// Its request and reply are still the Anthropic shapes the rest of the package
// speaks, translated at this boundary: the conversation, the tool definitions
// and the tool results go out as Gemini's and come back as Anthropic's, so
// nothing else in the package has to know which model answered.
const (
	geminiModel    = "gemini-3.8-flash"
	geminiProject  = "heliosian"
	geminiLocation = "global"
	// geminiThinking is what the model is allowed to spend before answering.
	// Ask is on the request path and this model thinks by default - a hundred
	// tokens to say "ok" - so it is held low.
	geminiThinking = "LOW"
)

type Gemini struct {
	client *http.Client
}

func NewGemini() (*Gemini, error) {
	client, _, err := transport.NewClient(context.Background(), option.WithScopes("https://www.googleapis.com/auth/cloud-platform"))
	if err != nil {
		return nil, fmt.Errorf("gemini client: %w", err)
	}
	return &Gemini{client: client}, nil
}

type geminiPart struct {
	Text             string        `json:"text,omitempty"`
	FunctionCall     *geminiCall   `json:"functionCall,omitempty"`
	FunctionResponse *geminiResult `json:"functionResponse,omitempty"`
	ThoughtSignature string        `json:"thoughtSignature,omitempty"`
	Thought          bool          `json:"thought,omitempty"`
}

type geminiCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

type geminiResult struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiDeclaration struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent  `json:"contents"`
	SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
	Tools             []map[string]any `json:"tools,omitempty"`
	ToolConfig        map[string]any   `json:"toolConfig,omitempty"`
	GenerationConfig  map[string]any   `json:"generationConfig,omitempty"`
}

type geminiChunk struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int64 `json:"promptTokenCount"`
		CandidatesTokenCount    int64 `json:"candidatesTokenCount"`
		ThoughtsTokenCount      int64 `json:"thoughtsTokenCount"`
		CachedContentTokenCount int64 `json:"cachedContentTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// geminiTools is every tool as Gemini takes them, read from the registry
// directly rather than translated back out of the Anthropic definitions.
func geminiTools() []map[string]any {
	declarations := []geminiDeclaration{}
	for _, t := range tools {
		properties := t.properties
		if properties == nil {
			properties = map[string]any{}
		}
		d := geminiDeclaration{Name: t.name, Description: t.description}
		// A function with no arguments takes no parameters block at all;
		// Gemini refuses an object schema with an empty properties map.
		if len(properties) > 0 {
			d.Parameters = map[string]any{"type": "object", "properties": properties}
			if len(t.required) > 0 {
				d.Parameters["required"] = t.required
			}
		}
		declarations = append(declarations, d)
	}
	return []map[string]any{{"functionDeclarations": declarations}}
}

// toolNames maps each tool-use id in the history to the tool it called, since
// Anthropic names the tool on the call and Gemini names it on the result too.
func toolNames(messages []anthropic.BetaMessageParam) map[string]string {
	names := map[string]string{}
	for _, m := range messages {
		for _, block := range m.Content {
			if use := block.OfToolUse; use != nil {
				names[use.ID] = use.Name
			}
		}
	}
	return names
}

// asContents is the conversation so far as Gemini's. An assistant turn is a
// model turn, a tool result is a user turn carrying a function response, and
// the mid-conversation system notes the chat inserts become plain user text,
// Gemini having no system role within a conversation.
func asContents(messages []anthropic.BetaMessageParam) []geminiContent {
	names := toolNames(messages)
	out := []geminiContent{}
	for _, m := range messages {
		role := "user"
		if m.Role == anthropic.BetaMessageParamRoleAssistant {
			role = "model"
		}
		parts := []geminiPart{}
		for _, block := range m.Content {
			switch {
			case block.OfText != nil:
				if block.OfText.Text != "" {
					parts = append(parts, geminiPart{Text: block.OfText.Text})
				}
			case block.OfToolUse != nil:
				args, err := json.Marshal(block.OfToolUse.Input)
				if err != nil {
					args = []byte("{}")
				}
				parts = append(parts, geminiPart{FunctionCall: &geminiCall{Name: block.OfToolUse.Name, Args: args}})
			case block.OfToolResult != nil:
				text := ""
				for _, c := range block.OfToolResult.Content {
					if c.OfText != nil {
						text += c.OfText.Text
					}
				}
				parts = append(parts, geminiPart{FunctionResponse: &geminiResult{
					Name:     names[block.OfToolResult.ToolUseID],
					Response: map[string]any{"result": text},
				}})
			}
		}
		if len(parts) > 0 {
			out = append(out, geminiContent{Role: role, Parts: parts})
		}
	}
	return out
}

func (g *Gemini) Respond(ctx context.Context, req Request, emit Emitter) (Reply, error) {
	reply := Reply{Messages: req.Messages, Tools: []string{}}
	text := &strings.Builder{}
	system := []string{}
	for _, block := range req.System {
		system = append(system, block.Text)
	}
	contents := asContents(req.Messages)
	for round := 0; round < maxRounds; round++ {
		body := geminiRequest{
			Contents:          contents,
			SystemInstruction: &geminiContent{Parts: []geminiPart{{Text: strings.Join(system, "\n\n")}}},
			Tools:             geminiTools(),
			GenerationConfig: map[string]any{
				"maxOutputTokens": maxTokens,
				"thinkingConfig":  map[string]any{"thinkingLevel": geminiThinking},
			},
		}
		// The last round allowed has to be the answer, so the tools stay on
		// offer for the history's sake but are closed to use.
		if round == maxRounds-1 {
			body.ToolConfig = map[string]any{"functionCallingConfig": map[string]any{"mode": "NONE"}}
		}
		answer, usage, err := g.stream(ctx, body, text, emit)
		if err != nil {
			reply.Text = text.String()
			return reply, err
		}
		reply.Usage.Input += usage.Input
		reply.Usage.Cached += usage.Cached
		reply.Usage.Output += usage.Output
		reply.Usage.Rounds++
		contents = append(contents, answer)

		calls := []geminiPart{}
		blocks := []anthropic.BetaContentBlockParamUnion{}
		for _, part := range answer.Parts {
			switch {
			case part.FunctionCall != nil:
				calls = append(calls, part)
			case part.Text != "" && !part.Thought:
				blocks = append(blocks, anthropic.NewBetaTextBlock(part.Text))
			}
		}
		for i, call := range calls {
			words := req.Label(call.FunctionCall.Name)
			if !slices.Contains(reply.Tools, words) {
				reply.Tools = append(reply.Tools, words)
				emit("tool", words)
			}
			blocks = append(blocks, anthropic.NewBetaToolUseBlock(geminiCallID(round, i), json.RawMessage(call.FunctionCall.Args), call.FunctionCall.Name))
		}
		if len(blocks) > 0 {
			reply.Messages = append(reply.Messages, anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: blocks})
		}
		if len(calls) == 0 {
			break
		}

		outputs := make([]string, len(calls))
		failed := make([]bool, len(calls))
		wg := sync.WaitGroup{}
		for i, call := range calls {
			wg.Go(func() {
				out, err := req.Run(ctx, call.FunctionCall.Name, json.RawMessage(call.FunctionCall.Args))
				if err != nil {
					outputs[i], failed[i] = err.Error(), true
					return
				}
				outputs[i] = out
			})
		}
		wg.Wait()
		parts := []geminiPart{}
		results := []anthropic.BetaContentBlockParamUnion{}
		for i, call := range calls {
			parts = append(parts, geminiPart{FunctionResponse: &geminiResult{
				Name:     call.FunctionCall.Name,
				Response: map[string]any{"result": outputs[i]},
			}})
			results = append(results, anthropic.NewBetaToolResultBlock(geminiCallID(round, i), outputs[i], failed[i]))
		}
		contents = append(contents, geminiContent{Role: "user", Parts: parts})
		reply.Messages = append(reply.Messages, anthropic.NewBetaUserMessage(results...))
	}
	reply.Text = text.String()
	return reply, nil
}

// geminiCallID names a call for the Anthropic-shaped history, Gemini giving
// its calls no id of their own.
func geminiCallID(round, i int) string {
	return fmt.Sprintf("call_%d_%d", round, i)
}

// geminiWaits paces a round the model has refused for capacity. This model
// carries no requests-per-minute quota of its own: capacity is shared across
// everyone on it, so a busy moment is answered 429 rather than queued, and
// asking again shortly after is the whole fix. The waits are short because
// somebody is watching the answer stream - a turn has three minutes in all -
// where the Sheets ones (internal/data) can afford to wait out a whole minute
// at startup.
var geminiWaits = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// open sends one round and hands back its body, waiting out a refusal for
// capacity. Only the response's own status is retried, before a single word
// has been read, so nothing is ever streamed to the page twice.
func (g *Gemini) open(ctx context.Context, request *http.Request, body []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			request = request.Clone(ctx)
			request.Body = io.NopCloser(bytes.NewReader(body))
		}
		resp, err := g.client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("ask: gemini: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		refused := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
		if !refused || attempt >= len(geminiWaits) {
			reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
			resp.Body.Close()
			return nil, fmt.Errorf("ask: gemini %d: %s", resp.StatusCode, strings.TrimSpace(string(reply)))
		}
		resp.Body.Close()
		slog.WarnContext(ctx, "ask: gemini refused for capacity; waiting to ask again", "status", resp.StatusCode, "wait", geminiWaits[attempt])
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(geminiWaits[attempt]):
		}
	}
}

// stream runs one round, writing the answer's words to text as they arrive and
// returning the model's whole turn.
func (g *Gemini) stream(ctx context.Context, body geminiRequest, text *strings.Builder, emit Emitter) (geminiContent, Usage, error) {
	usage := Usage{}
	raw, err := json.Marshal(body)
	if err != nil {
		return geminiContent{}, usage, err
	}
	address := fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
		geminiProject, geminiLocation, geminiModel)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(raw))
	if err != nil {
		return geminiContent{}, usage, err
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := g.open(ctx, request, raw)
	if err != nil {
		return geminiContent{}, usage, err
	}
	defer resp.Body.Close()
	answer := geminiContent{Role: "model"}
	// The answer's words arrive as many deltas and its calls arrive whole.
	// They are gathered into one text part and one part per call, since a
	// part carrying neither is not a part the next round may send back.
	said := &strings.Builder{}
	signature := ""
	calls := []geminiPart{}
	first := true
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var chunk geminiChunk
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &chunk); err != nil {
			return answer, usage, fmt.Errorf("ask: gemini stream: %w", err)
		}
		if chunk.Error != nil {
			return answer, usage, fmt.Errorf("ask: gemini: %s", chunk.Error.Message)
		}
		if chunk.UsageMetadata.PromptTokenCount > 0 {
			usage.Input = chunk.UsageMetadata.PromptTokenCount
			usage.Cached = chunk.UsageMetadata.CachedContentTokenCount
			usage.Output = chunk.UsageMetadata.CandidatesTokenCount + chunk.UsageMetadata.ThoughtsTokenCount
		}
		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.ThoughtSignature != "" {
					signature = part.ThoughtSignature
				}
				if part.Thought {
					continue
				}
				if part.FunctionCall != nil {
					calls = append(calls, part)
					continue
				}
				if part.Text == "" {
					continue
				}
				// Words written before a tool call and words written after
				// it are kept as separate paragraphs, as Claude's answers are.
				if first && text.Len() > 0 {
					text.WriteString("\n\n")
				}
				first = false
				said.WriteString(part.Text)
				text.WriteString(part.Text)
				emit("text", part.Text)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return answer, usage, fmt.Errorf("ask: gemini stream: %w", err)
	}
	if said.Len() > 0 {
		answer.Parts = append(answer.Parts, geminiPart{Text: said.String()})
	}
	// The signature covers the reasoning behind the calls, and the next round
	// has to hand it back for the model to pick that reasoning up again.
	for i, call := range calls {
		if i == 0 {
			call.ThoughtSignature = signature
		}
		answer.Parts = append(answer.Parts, call)
	}
	return answer, usage, nil
}
