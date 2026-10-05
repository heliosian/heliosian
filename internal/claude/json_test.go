package claude

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/intercept"
)

func TestJSONReadsTheAnswer(t *testing.T) {
	intercept.Install(intercept.ClaudeHost, intercept.Claude())
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sentence": map[string]any{"type": "string"},
			"points":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"kind":     map[string]any{"type": "string", "enum": []string{"school", "class"}},
		},
		"required":             []string{"sentence", "points", "kind"},
		"additionalProperties": false,
	}
	var out struct {
		Sentence string   `json:"sentence"`
		Points   []string `json:"points"`
		Kind     string   `json:"kind"`
	}
	raw, err := JSON(context.Background(), anthropic.NewClient(option.WithAPIKey("test")), anthropic.MessageNewParams{
		Model:        "claude-sonnet-5-5",
		MaxTokens:    100,
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("Hello"))},
		OutputConfig: anthropic.OutputConfigParam{Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.Sentence != "Sample sentence." || len(out.Points) != 1 || out.Kind != "school" || raw == "" {
		t.Fatalf("read %+v from %s", out, raw)
	}
}
