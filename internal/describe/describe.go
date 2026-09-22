// Package describe asks Claude for what the charity list needs about a
// charity - where to donate and the one sentence the birthday newsletter
// carries - read from the charity's own site so a small local organization is
// described from what it says rather than guessed at.
package describe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/claude"
)

const model = "claude-opus-5"

const maxPrompt = 4 << 10

var ErrTooLong = errors.New("that's more than Claude needs to go on; shorten it")

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
	limit  *claude.Limiter
}

// New returns nil without a key.
func New(key string, limit *claude.Limiter) *Describer {
	if key == "" {
		return nil
	}
	return &Describer{client: anthropic.NewClient(option.WithAPIKey(key)), limit: limit}
}

// Info is what Claude found: where to donate, and the sentence.
type Info struct {
	DonationLink string `json:"donationLink"`
	Sentence     string `json:"sentence"`
}

// Charity returns what Claude found about the charity, starting from link
// when there is one; an empty sentence means it could not find out.
func (d *Describer) Charity(ctx context.Context, actor, name, link string) (Info, error) {
	if d == nil {
		return Info{}, fmt.Errorf("describing charities is not set up: no Anthropic key")
	}
	if !d.limit.Allow(actor, time.Now()) {
		return Info{}, claude.ErrTooMany
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	prompt := "Charity: " + strings.TrimSpace(name)
	if link = strings.TrimSpace(link); link != "" {
		prompt += "\nDonation link given: " + link
	}
	if len(prompt) > maxPrompt {
		return Info{}, ErrTooLong
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

// GroupFacts is what Loop knows about a group for its description: the
// title, the rules read out in words, and who the rules pick out now - the
// count, and how they fall by role, grade and classroom - never names.
type GroupFacts struct {
	Title      string
	Rules      []string
	Members    int
	Roles      map[string]int
	Grades     map[string]int
	Classrooms map[string]int
}

const groupSystem = `You write the one- or two-sentence description of an email group for a school community's group directory, from the group's title, the rules that choose its members, and a tally of who the rules pick out today.

Say plainly who the group reaches and what it is for, as a parent glancing at the list would want to know, in the register of these:

- Parents of every student in Grade 5 through Grade 8.
- The kindergarten class and their families.
- The players on the coastside soccer team and their parents.
- Everyone who volunteered for International Night, with the parents of the students among them.

Draw on the rules first, and on the tally to say who that comes to - a grade, a classroom, a role. Name nobody. No superlatives, no marketing language, no mention of email or of the rules themselves. One sentence, two at most; return the description alone.`

var groupSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"description": map[string]any{"type": "string", "description": "the description, one or two sentences"},
	},
	"required":             []string{"description"},
	"additionalProperties": false,
}

// Group returns Claude's description of a group from its facts.
func (d *Describer) Group(ctx context.Context, actor string, facts GroupFacts) (string, error) {
	if d == nil {
		return "", fmt.Errorf("describing groups is not set up: no Anthropic key")
	}
	if !d.limit.Allow(actor, time.Now()) {
		return "", claude.ErrTooMany
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	prompt := &strings.Builder{}
	fmt.Fprintf(prompt, "Title: %s\n", strings.TrimSpace(facts.Title))
	fmt.Fprintf(prompt, "Rules:\n")
	for _, r := range facts.Rules {
		fmt.Fprintf(prompt, "- %s\n", r)
	}
	fmt.Fprintf(prompt, "Members today: %d\n", facts.Members)
	tally := func(label string, counts map[string]int) {
		if len(counts) == 0 {
			return
		}
		fmt.Fprintf(prompt, "%s: %s\n", label, tallyWords(counts))
	}
	tally("By role", facts.Roles)
	tally("Students' grades", facts.Grades)
	tally("Students' classrooms", facts.Classrooms)
	if prompt.Len() > maxPrompt {
		return "", ErrTooLong
	}
	stream := d.client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: groupSystem, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.String()))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: groupSchema}},
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
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return "", fmt.Errorf("read claude's answer: %w", err)
	}
	slog.InfoContext(ctx, "described group", "title", facts.Title, "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "output_tokens", resp.Usage.OutputTokens)
	return strings.TrimSpace(out.Description), nil
}

// tallyWords is a count map as words, largest first: "Parent 12, Staff 2".
func tallyWords(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

// Fake stands in for Claude in sample mode: a sentence in the right shape
// and a made-up donate page, after a moment, so the form's flow can be tried
// without a key.
type Fake struct{}

func (Fake) Charity(ctx context.Context, actor, name, link string) (Info, error) {
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

// Group in sample mode: the rules read back as a sentence, after a moment.
func (Fake) Group(ctx context.Context, actor string, facts GroupFacts) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(1200 * time.Millisecond):
	}
	if len(facts.Rules) == 0 {
		return "A group with nobody on it yet, as the sample server describes it.", nil
	}
	return fmt.Sprintf("%s, %d of them today, as the sample server describes it.", strings.Join(facts.Rules, "; "), facts.Members), nil
}
