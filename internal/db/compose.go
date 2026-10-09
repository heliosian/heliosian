package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/access"
	"heliosian/internal/claude"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
)

const composeTimeout = 3 * time.Minute

const composeIntro = `You write queries for Helios Admin's Query page from a plain description. The page runs the query as the person asking, so they see only the rows the read policies let them see.

` + Language + `

Read the description, then say back in a sentence or two what you understood it to ask for, in terms of the data: which table, which rows, in what order. Write the query that answers it. When the description can't be answered from these tables, or is too unclear to pick one reading, say why in understanding and leave query empty. Use only the tables, columns and enum values below. Prefer the plainest query that answers the description; add include for references the reader will want to see named. Don't add a limit unless the description asks for one.`

const Language = `A query is one fully bracketed prefix expression, the operator first in every bracket:

(from TABLE [@name] (where cond…) (order path asc|desc …) (limit N) (include path…))

- A condition is (and …), (or …), (not cond), a comparison (= a b), (!= a b), (< a b), (<= a b), (> a b), (>= a b), (in x "a" "b"), (in x (select TABLE.column cond…)), (in x (ancestors @row)), (blank path), (contains path "text"), holding when the text at the path has the quoted text in it, ignoring case, (exists TABLE [@name] cond…), or a yes/no path on its own. Several conditions in one bracket are anded.
- A value is a path, a quoted string, a number, true, false, today, now, (count TABLE [@name] cond…), (sum path TABLE [@name] cond…) or (length path), the number of characters in a text path, 0 when blank.
- Upper-case names are tables. A path is columns joined by dots, each but the last a reference followed to the row it names (group.kind, person.name_show). A path starts at the innermost row being matched, or at a named one (@g.visible_to), or at @viewer, the person running the query.
- (ancestors @row) is the row and every row above it through parent, for a table whose parent names its own table.
- Text and enums compare ignoring case; references and IDs exactly. A date compares against a moment as that day's midnight. A blank cell equals nothing, and != holds when exactly one side is blank.
- include brings the rows a reference path names (include parent, include person group) so their titles show beside the answer.
- The policy definitions are part of the language: (visible @g), (manages @g), (super_admin), (household @p) and the rest can be called inside a query that names its row, (from GROUP @g (where (visible @g))).`

const composeExamples = `Examples:

Description: linked documents still waiting to be fetched
Query: (from DOCUMENT (where (= relation "linked") (not (blank url)) (blank content) (blank fetch)) (include parent))

Description: groups I manage
Query: (from GROUP @g (where (manages @g)) (order name asc))

Description: people in grade 3 with no primary email
Query: (from PERSON @p (where (= grade "3") (not (exists PERSON_EMAIL (= person @p) primary))) (order name_sort asc))`

var composeSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"understanding": map[string]any{"type": "string", "description": "what you understood the description to ask for, in a sentence or two, or why it can't be answered"},
		"query":         map[string]any{"type": "string", "description": "the query in the language, or empty when there is none"},
	},
	"required":             []string{"understanding", "query"},
	"additionalProperties": false,
}

var composeSystem = composeIntro + "\n\n" + describeTables() + "\nPolicy definitions and clauses:\n" + PolicySource + "\n" + composeExamples

func describeTables() string {
	out := &strings.Builder{}
	out.WriteString("Tables:\n")
	for _, t := range Tables {
		out.WriteString("\n" + DescribeTable(t))
	}
	return out.String()
}

func DescribeTable(t Table) string {
	out := &strings.Builder{}
	fmt.Fprintf(out, "%s: %s", t.Name, t.Description)
	if t.Generated {
		out.WriteString(" Worked out from other tables.")
	}
	out.WriteString("\n")
	for _, c := range t.Columns {
		if c.Private {
			continue
		}
		kind := kindNames[c.Kind]
		if c.Kind == Ref && c.Target != "" {
			kind += " → " + c.Target
		}
		fmt.Fprintf(out, "- %s (%s) %s", c.Name, kind, c.Description)
		if len(c.Values) > 0 {
			values := []string{}
			for _, value := range c.Values {
				values = append(values, fmt.Sprintf("%q %s", value.Name, value.Description))
			}
			fmt.Fprintf(out, " Values: %s.", strings.Join(values, "; "))
		}
		out.WriteString("\n")
	}
	return out.String()
}

type Composer struct {
	client anthropic.Client
	limit  *ratelimit.Limiter
}

func NewComposer(key string, limit *ratelimit.Limiter) *Composer {
	return &Composer{client: anthropic.NewClient(option.WithAPIKey(key)), limit: limit}
}

type Composed struct {
	Understanding string `json:"understanding"`
	Query         string `json:"query"`
}

func (c *Composer) Compose(ctx context.Context, words string, today time.Time) (Composed, error) {
	ctx, cancel := context.WithTimeout(ctx, composeTimeout)
	defer cancel()
	out := Composed{}
	prompt := "Today is " + today.Format("Monday "+time.DateOnly) + ".\n\nDescription: " + words
	_, err := claude.JSON(ctx, c.client, anthropic.MessageNewParams{
		Model:        claude.ComposeModel,
		MaxTokens:    32000,
		System:       []anthropic.TextBlockParam{{Text: composeSystem, CacheControl: anthropic.NewCacheControlEphemeralParam()}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(prompt))},
		OutputConfig: anthropic.OutputConfigParam{Effort: claude.ComposeEffort, Format: anthropic.JSONOutputFormatParam{Schema: composeSchema}},
	}, &out)
	return out, err
}

func RegisterCompose(mux *http.ServeMux, s *Store, c *Composer, importKey []byte, now func() time.Time) {
	mux.HandleFunc("POST /api/do/compose", func(w http.ResponseWriter, r *http.Request) {
		at := now()
		env, _, ok := caller(w, r, s.Model(), importKey, at)
		if !ok {
			return
		}
		var asked struct {
			Words string `json:"words"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, queryLimit)).Decode(&asked); err != nil {
			serve.Error(w, r, access.Invalid("send {\"words\": \"…\"}: %v", err))
			return
		}
		words := strings.TrimSpace(asked.Words)
		if words == "" {
			serve.Error(w, r, access.Invalid("words is required"))
			return
		}
		if !c.limit.Allow(env.Viewer+env.System, at) {
			serve.Error(w, r, access.Refuse(http.StatusTooManyRequests, "%v", claude.ErrTooMany))
			return
		}
		start := time.Now()
		out, err := c.Compose(r.Context(), words, at)
		if errors.Is(err, claude.ErrFinal) {
			serve.Error(w, r, access.Invalid("%v", err))
			return
		}
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "compose", "viewer", env.Viewer, "system", env.System, "words", words, "query", out.Query, "took", time.Since(start).Round(time.Millisecond))
		serve.Write(w, r, http.StatusOK, out)
	})
}
