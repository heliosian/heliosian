package testkit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode"
)

func ClaudeReplying(reply func(request string) string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, _ := io.ReadAll(r.Body)
		ClaudeStream(w, reply(string(request)), "end_turn")
	}
}

func ClaudeStream(w http.ResponseWriter, text, stop string) {
	w.Header().Set("Content-Type", "text/event-stream")
	answer, _ := json.Marshal(text)
	for _, event := range []string{
		`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(answer) + `}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"` + stop + `","stop_sequence":null},"usage":{"output_tokens":0}}`,
		`{"type":"message_stop"}`,
	} {
		var kind struct {
			Type string `json:"type"`
		}
		json.Unmarshal([]byte(event), &kind)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind.Type, event)
	}
}

func UserText(request string) string {
	var asked struct {
		Messages []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal([]byte(request), &asked)
	if len(asked.Messages) == 0 || len(asked.Messages[0].Content) == 0 {
		return ""
	}
	return asked.Messages[0].Content[0].Text
}

func SearchClaude() http.Handler {
	return ClaudeReplying(func(request string) string {
		keywords := []string{}
		for _, w := range strings.FieldsFunc(strings.ToLower(UserText(request)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len(w) > 1 && !slices.Contains(keywords, w) {
				keywords = append(keywords, w)
			}
		}
		answer, _ := json.Marshal(map[string]any{"summary": "a thing in the sample", "keywords": keywords})
		return string(answer)
	})
}
