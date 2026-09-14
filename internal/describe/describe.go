// Package describe asks Claude for what the charity list needs about a
// charity - where to donate and the one sentence the birthday newsletter
// carries - read from the charity's own site so a small local organization is
// described from what it says rather than guessed at.
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
const system = `You look up charities for a school community newsletter that announces staff birthday donations, and return two things: where to donate, and a one-sentence description.

Find the charity's own website (search the web by its name; when a donation link is given, start there) and fetch it. Return the URL of the page where a donation is made - the site's donate page, or the giving-platform page the site itself links to, or the homepage when neither can be found - as a URL you actually saw, never one you made up. Then write exactly one sentence that starts with the charity's name and says plainly what it does and for whom, in the same register as these:

- Second Harvest of Silicon Valley provides free nutritious groceries and fresh meals to families, seniors, and individuals experiencing food insecurity in Santa Clara and San Mateo counties.
- The Reeve Foundation helps individuals and families impacted by paralysis by funding innovative spinal cord injury research and improving their quality of life through grants, information, and advocacy.
- Camp Natoma provides immersive outdoor summer camp experiences that help youth connect with nature, build resilience, and develop life skills.

Say only what the site supports. No superlatives, no marketing language, no mention of donating or of the newsletter. If you cannot find the organization, return "Unknown" as the sentence and an empty donation link rather than guessing.`

var schema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"sentence":     map[string]any{"type": "string", "description": "the one sentence, or Unknown"},
		"donationLink": map[string]any{"type": "string", "description": "the URL where a donation is made, or empty"},
	},
	"required":             []string{"sentence", "donationLink"},
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

// Info is what Claude found: where to donate, and the sentence.
type Info struct {
	DonationLink string `json:"donationLink"`
	Sentence     string `json:"sentence"`
}

// Charity returns what Claude found about the charity, starting from link
// when there is one; an empty sentence means it could not find out.
func (d *Describer) Charity(ctx context.Context, name, link string) (Info, error) {
	if d == nil {
		return Info{}, fmt.Errorf("describing charities is not set up: no Anthropic key")
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	prompt := "Charity: " + strings.TrimSpace(name)
	if link = strings.TrimSpace(link); link != "" {
		prompt += "\nDonation link given: " + link
	}
	// The donation link is often a giving platform's page rather than the
	// charity's own site, so the fetch is left free to follow a search there.
	fetch := anthropic.WebFetchTool20260209Param{MaxUses: anthropic.Int(4)}
	stream := d.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		Tools: []anthropic.ToolUnionParam{
			{OfWebFetchTool20260209: &fetch},
			{OfWebSearchTool20260209: &anthropic.WebSearchTool20260209Param{MaxUses: anthropic.Int(3)}},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	})
	resp := anthropic.Message{}
	for stream.Next() {
		if err := resp.Accumulate(stream.Current()); err != nil {
			return Info{}, err
		}
	}
	if err := stream.Err(); err != nil {
		return Info{}, err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return Info{}, fmt.Errorf("claude declined: %s", resp.StopDetails.Explanation)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return Info{}, fmt.Errorf("claude stopped early: %s", resp.StopReason)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out Info
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return Info{}, fmt.Errorf("read claude's answer: %w", err)
	}
	slog.InfoContext(ctx, "described charity", "name", name, "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "output_tokens", resp.Usage.OutputTokens)
	out.Sentence, out.DonationLink = strings.TrimSpace(out.Sentence), strings.TrimSpace(out.DonationLink)
	if strings.EqualFold(out.Sentence, "unknown") {
		out.Sentence = ""
	}
	if !strings.HasPrefix(out.DonationLink, "http://") && !strings.HasPrefix(out.DonationLink, "https://") {
		out.DonationLink = ""
	}
	if out.DonationLink == "" {
		out.DonationLink = link
	}
	return out, nil
}

// Fake stands in for Claude in sample mode: a sentence in the right shape
// and a made-up donate page, after a moment, so the form's flow can be tried
// without a key.
type Fake struct{}

func (Fake) Charity(ctx context.Context, name, link string) (Info, error) {
	select {
	case <-ctx.Done():
		return Info{}, ctx.Err()
	case <-time.After(1500 * time.Millisecond):
	}
	name = strings.TrimSpace(name)
	if link == "" {
		link = "https://www." + strings.ToLower(strings.Join(strings.Fields(name), "")) + ".org/donate"
	}
	return Info{DonationLink: link, Sentence: name + " provides sample support to the sample community by doing sample things, as the sample server says."}, nil
}
