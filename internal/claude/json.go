package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

var (
	ErrFinal    = errors.New("asking again would answer the same")
	ErrCutShort = errors.New("the answer reached its token limit")
)

func JSON(ctx context.Context, client anthropic.Client, params anthropic.MessageNewParams, out any) (string, error) {
	text, err := Text(ctx, client, params)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(text), out); err != nil {
		return "", fmt.Errorf("read claude's answer: %w: %w: %s", err, ErrFinal, text)
	}
	return text, nil
}

func Text(ctx context.Context, client anthropic.Client, params anthropic.MessageNewParams) (string, error) {
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
		return "", fmt.Errorf("claude refused: %s: %s: %w", resp.StopDetails.Category, resp.StopDetails.Explanation, ErrFinal)
	}
	if resp.StopReason == anthropic.StopReasonMaxTokens {
		return "", fmt.Errorf("claude stopped early: %s: %w: %w", resp.StopReason, ErrCutShort, ErrFinal)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return "", fmt.Errorf("claude stopped early: %s: %w", resp.StopReason, ErrFinal)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	slog.InfoContext(ctx, "claude answered", "model", params.Model, "input_tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, "cached_tokens", resp.Usage.CacheReadInputTokens, "output_tokens", resp.Usage.OutputTokens)
	return text.String(), nil
}
