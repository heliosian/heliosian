package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

func JSON(ctx context.Context, client anthropic.Client, params anthropic.MessageNewParams, out any) (string, error) {
	stream := client.Messages.NewStreaming(ctx, params)
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
		return "", fmt.Errorf("claude refused: %s: %s", resp.StopDetails.Category, resp.StopDetails.Explanation)
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
	slog.InfoContext(ctx, "claude answered", "model", params.Model, "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "cached_tokens", resp.Usage.CacheReadInputTokens, "output_tokens", resp.Usage.OutputTokens)
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return "", fmt.Errorf("read claude's answer: %w: %s", err, text.String())
	}
	return text.String(), nil
}
