package describe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/claude"
	"heliosian/internal/ratelimit"
)

const model = "claude-opus-5"

const maxPrompt = 4 << 10

var ErrTooLong = errors.New("that's more than Claude needs to go on; shorten it")

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

type Describer struct {
	client anthropic.Client
	limit  *ratelimit.Limiter
}

func New(key string, limit *ratelimit.Limiter) *Describer {
	return &Describer{client: anthropic.NewClient(option.WithAPIKey(key)), limit: limit}
}

type Info struct {
	DonationLink string `json:"donationLink"`
	Sentence     string `json:"sentence"`
}

func (d *Describer) Charity(ctx context.Context, actor, name, link string) (Info, error) {
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
	fetch := anthropic.WebFetchTool20260209Param{MaxUses: anthropic.Int(4)}
	var out Info
	if _, err := claude.JSON(ctx, d.client, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 16000,
		System:    []anthropic.TextBlockParam{{Text: system, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		Tools: []anthropic.ToolUnionParam{
			{OfWebFetchTool20260209: &fetch},
			{OfWebSearchTool20260209: &anthropic.WebSearchTool20260209Param{MaxUses: anthropic.Int(3)}},
		},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, &out); err != nil {
		return Info{}, err
	}
	slog.InfoContext(ctx, "described charity", "name", name)
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

type GroupFacts struct {
	Title      string
	Rules      []string
	Members    int
	Roles      map[string]int
	Grades     map[string]int
	Classrooms map[string]int
}

const groupSystem = `You write the one- or two-sentence description of an email list for a school community's directory of email lists, from the email list's title, the rules that choose its members, and a tally of who the rules pick out today.

Say plainly who the email list reaches and what it is for, as a parent glancing at the list would want to know, in the register of these:

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

func (d *Describer) Group(ctx context.Context, actor string, facts GroupFacts) (string, error) {
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
	var out struct {
		Description string `json:"description"`
	}
	if _, err := claude.JSON(ctx, d.client, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: groupSystem, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.String()))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: groupSchema}},
	}, &out); err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "described group", "title", facts.Title)
	return strings.TrimSpace(out.Description), nil
}

type ActivityFacts struct {
	Title    string
	Parent   string
	Category string
	When     string
	Notes    string
}

const activitySystem = `You write the description of a volunteer activity or school event on a school parent community's volunteer portal. It sits on the activity's page, under "What people should know before they sign up", where parents decide whether to take part. You are given the activity's title, the event it is part of, its category, when it happens, and the organizer's current description, which is often rough notes.

Rewrite the notes into a clear, warm description of a few short sentences, as a parent volunteer would write it for other parents: what this is, and what the people who sign up would do. Keep every fact, request and caveat in the notes - a call for someone to lead it, a thing to bring, a cost, a deadline - and fix their spelling and capitalization. Add nothing the notes and details don't support: no invented times, places, numbers, tasks or history. Don't restate the title or the date unless the notes say more about them. When the notes are empty, write one or two sentences from the title and details alone.

Plain text in paragraphs; no headings, no markdown, no bullet points, no superlatives or marketing language. Return the description alone.`

var activitySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"description": map[string]any{"type": "string", "description": "the description"},
	},
	"required":             []string{"description"},
	"additionalProperties": false,
}

func (d *Describer) Activity(ctx context.Context, actor string, facts ActivityFacts) (string, error) {
	if !d.limit.Allow(actor, time.Now()) {
		return "", claude.ErrTooMany
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	prompt := &strings.Builder{}
	line := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(prompt, "%s: %s\n", label, value)
		}
	}
	line("Title", facts.Title)
	line("Part of", facts.Parent)
	line("Category", facts.Category)
	line("When", facts.When)
	fmt.Fprintf(prompt, "Current description:\n%s\n", strings.TrimSpace(facts.Notes))
	if prompt.Len() > maxPrompt {
		return "", ErrTooLong
	}
	var out struct {
		Description string `json:"description"`
	}
	if _, err := claude.JSON(ctx, d.client, anthropic.MessageNewParams{
		Model:        model,
		MaxTokens:    4000,
		System:       []anthropic.TextBlockParam{{Text: activitySystem, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt.String()))},
		OutputConfig: anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffortLow, Format: anthropic.JSONOutputFormatParam{Schema: activitySchema}},
	}, &out); err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "described activity", "title", facts.Title)
	return strings.TrimSpace(out.Description), nil
}

func tallyWords(counts map[string]int) string {
	keys := []string{}
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := []string{}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}
