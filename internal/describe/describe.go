// Package describe asks Claude for the one sentence about a charity that the
// birthday newsletter carries, read from the charity's own site so a small
// local organization is described from what it says rather than guessed at.
package describe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const model = "claude-opus-5"

// The style the list already uses: the charity's name, what it does, for whom.
const system = `You write one-sentence descriptions of charities for a school community newsletter that announces staff birthday donations.

Read the charity's website at the donation link you are given (fetch it; search the web if the page says too little) and write exactly one sentence that starts with the charity's name and says plainly what it does and for whom, in the same register as these:

- Second Harvest of Silicon Valley provides free nutritious groceries and fresh meals to families, seniors, and individuals experiencing food insecurity in Santa Clara and San Mateo counties.
- The Reeve Foundation helps individuals and families impacted by paralysis by funding innovative spinal cord injury research and improving their quality of life through grants, information, and advocacy.
- Camp Natoma provides immersive outdoor summer camp experiences that help youth connect with nature, build resilience, and develop life skills.

Say only what the site supports. No superlatives, no marketing language, no mention of donating or of the newsletter. If you cannot find out what the organization does, say so in the sentence field as "Unknown" rather than guessing.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"sentence": map[string]any{"type": "string", "description": "the one sentence, or Unknown"},
	},
	"required":             []string{"sentence"},
	"additionalProperties": false,
}

// Describer holds the client; a nil Describer describes nothing, so the app
// runs without a key and the button says so.
type Describer struct {
	client anthropic.Client
}

// New returns nil without a key.
func New(key string) *Describer {
	if key == "" {
		return nil
	}
	return &Describer{client: anthropic.NewClient(option.WithAPIKey(key))}
}

// Charity returns the sentence for the charity at link, or "" when Claude
// could not find out what it does.
func (d *Describer) Charity(ctx context.Context, name, link string) (string, error) {
	if d == nil {
		return "", fmt.Errorf("describing charities is not set up: no Anthropic key")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	prompt := fmt.Sprintf("Charity: %s\nDonation link: %s", strings.TrimSpace(name), strings.TrimSpace(link))
	// The donation link is often a giving platform's page rather than the
	// charity's own site, so the fetch is left free to follow a search there.
	fetch := anthropic.WebFetchTool20260209Param{MaxUses: anthropic.Int(3)}
	stream := d.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		Tools: []anthropic.ToolUnionParam{
			{OfWebFetchTool20260209: &fetch},
			{OfWebSearchTool20260209: &anthropic.WebSearchTool20260209Param{MaxUses: anthropic.Int(2)}},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	})
	resp := anthropic.Message{}
	for stream.Next() {
		if err := resp.Accumulate(stream.Current()); err != nil {
			return "", err
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("claude declined: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return "", fmt.Errorf("claude stopped early: %s", resp.StopReason)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out struct {
		Sentence string `json:"sentence"`
	}
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return "", fmt.Errorf("read claude's answer: %w", err)
	}
	slog.InfoContext(ctx, "described charity", "name", name, "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "output_tokens", resp.Usage.OutputTokens)
	sentence := strings.TrimSpace(out.Sentence)
	if strings.EqualFold(sentence, "unknown") {
		return "", nil
	}
	return sentence, nil
}
