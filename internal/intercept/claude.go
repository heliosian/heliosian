package intercept

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const ClaudeHost = "api.anthropic.com"

const sampleAnswer = "This is the sample server, so nothing is asking Claude. A real answer streams in here, drawn from the directory, the calendar, HCA-Team, Celebrate, Loop and Heliosian's links."

type claudeRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Content []claudeBlock `json:"content"`
	} `json:"messages"`
	Tools []struct {
		Name string `json:"name"`
	} `json:"tools"`
	OutputConfig struct {
		Format struct {
			Schema map[string]any `json:"schema"`
		} `json:"format"`
	} `json:"output_config"`
}

type claudeBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Content json.RawMessage `json:"content"`
}

func (r claudeRequest) toolResult() (string, bool) {
	if len(r.Messages) == 0 {
		return "", false
	}
	for _, b := range r.Messages[len(r.Messages)-1].Content {
		if b.Type != "tool_result" {
			continue
		}
		var text string
		if json.Unmarshal(b.Content, &text) == nil {
			return text, true
		}
		blocks := []claudeBlock{}
		if err := json.Unmarshal(b.Content, &blocks); err != nil {
			return string(b.Content), true
		}
		parts := []string{}
		for _, part := range blocks {
			parts = append(parts, part.Text)
		}
		return strings.Join(parts, ""), true
	}
	return "", false
}

type claudeStream struct {
	w     http.ResponseWriter
	index int
	open  bool
	stop  string
}

func (s *claudeStream) event(kind string, data map[string]any) {
	data["type"] = kind
	encoded, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", kind, encoded)
	if err := http.NewResponseController(s.w).Flush(); err != nil {
		slog.Error("intercept: flush claude stream", "error", err)
	}
}

func (s *claudeStream) close() {
	if !s.open {
		return
	}
	s.event("content_block_stop", map[string]any{"index": s.index})
	s.index++
	s.open = false
}

func (s *claudeStream) text(text string) {
	if !s.open {
		s.event("content_block_start", map[string]any{"index": s.index, "content_block": map[string]any{"type": "text", "text": ""}})
		s.open = true
	}
	s.event("content_block_delta", map[string]any{"index": s.index, "delta": map[string]any{"type": "text_delta", "text": text}})
}

func (s *claudeStream) tool(name, input string) {
	s.close()
	s.event("content_block_start", map[string]any{"index": s.index, "content_block": map[string]any{"type": "tool_use", "id": fmt.Sprintf("toolu_%d", s.index), "name": name, "input": map[string]any{}}})
	s.event("content_block_delta", map[string]any{"index": s.index, "delta": map[string]any{"type": "input_json_delta", "partial_json": input}})
	s.open = true
	s.close()
	s.stop = "tool_use"
}

func Claude() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			http.Error(w, "intercept answers only POST /v1/messages", http.StatusNotFound)
			return
		}
		var req claudeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		s := &claudeStream{w: w, stop: "end_turn"}
		s.event("message_start", map[string]any{"message": map[string]any{
			"id": "msg_intercept", "type": "message", "role": "assistant", "model": req.Model, "content": []any{},
			"stop_reason": nil, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0},
		}})
		if !answer(r.Context(), s, req) {
			return
		}
		s.close()
		s.event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": s.stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 0}})
		s.event("message_stop", map[string]any{})
	})
}

func SearchAnswer(text string) map[string]any {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return map[string]any{"summary": "Sample search entry for " + first}
}

func searchSchema(schema map[string]any) bool {
	properties, _ := schema["properties"].(map[string]any)
	_, summary := properties["summary"]
	return len(properties) == 1 && summary
}

func (r claudeRequest) userText() string {
	if len(r.Messages) == 0 || len(r.Messages[0].Content) == 0 {
		return ""
	}
	return r.Messages[0].Content[0].Text
}

func answer(ctx context.Context, s *claudeStream, req claudeRequest) bool {
	if schema := req.OutputConfig.Format.Schema; schema != nil {
		if !pause(ctx, time.Second) {
			return false
		}
		reply := sampleOf(schema, "answer")
		if searchSchema(schema) {
			reply = SearchAnswer(req.userText())
		}
		encoded, err := json.Marshal(reply)
		if err != nil {
			panic(err)
		}
		s.text(string(encoded))
		return true
	}
	result, answered := req.toolResult()
	if len(req.Tools) > 0 && !answered {
		s.tool(req.Tools[0].Name, "{}")
		return true
	}
	for i, word := range strings.Fields(sampleAnswer) {
		if !pause(ctx, 30*time.Millisecond) {
			return false
		}
		if i > 0 {
			word = " " + word
		}
		s.text(word)
	}
	if answered {
		tail := "\n\nThe tool returned: " + result
		if len(tail) > 600 {
			tail = tail[:600] + "…"
		}
		s.text(tail)
	}
	return true
}

func pause(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func sampleOf(schema map[string]any, name string) any {
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		return enum[0]
	}
	switch schema["type"] {
	case "object":
		out := map[string]any{}
		properties, _ := schema["properties"].(map[string]any)
		for key, property := range properties {
			out[key] = sampleOf(property.(map[string]any), key)
		}
		return out
	case "array":
		items, _ := schema["items"].(map[string]any)
		return []any{sampleOf(items, name)}
	case "integer", "number":
		return 1
	case "boolean":
		return true
	}
	return "Sample " + name + "."
}
