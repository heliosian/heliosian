package ask

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/tools"
)

const maxToolOutput = 40000

func definitions(set *tools.Set) []anthropic.BetaToolUnionParam {
	out := []anthropic.BetaToolUnionParam{}
	for _, t := range set.Tools {
		raw, err := json.Marshal(t.Schema)
		if err != nil {
			panic(err)
		}
		var schema struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			panic(err)
		}
		if schema.Properties == nil {
			schema.Properties = map[string]any{}
		}
		out = append(out, anthropic.BetaToolUnionParam{OfTool: &anthropic.BetaToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.BetaToolInputSchemaParam{Properties: schema.Properties, Required: schema.Required},
		}})
	}
	return out
}

func label(set *tools.Set) func(name string) string {
	return func(name string) string {
		if t, ok := set.Tool(name); ok {
			return t.Words
		}
		return "Looking something up"
	}
}

func (t *turn) run(ctx context.Context, name string, input json.RawMessage) (string, error) {
	answer, err := t.sources.Tools.Run(ctx, t.m, t.env.Viewer, name, input)
	if err != nil {
		return "", err
	}
	for href, id := range answer.Links {
		t.found.note(href, id)
	}
	if len(answer.Text) > maxToolOutput {
		return "", fmt.Errorf("that is too much at once (%d characters); narrow it with words, a date range, a classroom or an ID", len(answer.Text))
	}
	return answer.Text, nil
}
