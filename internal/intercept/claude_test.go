package intercept

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestASearchEntryIsAnsweredWithItsName(t *testing.T) {
	request := `{"model": "m", "messages": [{"content": [{"type": "text", "text": "Email: Tide pools\nKind: mail\n\nBring boots."}]}],
		"output_config": {"format": {"schema": {"type": "object", "properties": {"summary": {"type": "string"}}}}}}`
	rec := httptest.NewRecorder()
	Claude().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(request)))
	text := ""
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		var event struct {
			Delta struct {
				Text string `json:"text"`
			} `json:"delta"`
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok && json.Unmarshal([]byte(data), &event) == nil {
			text += event.Delta.Text
		}
	}
	var got struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("%v: %s", err, rec.Body)
	}
	if got.Summary != "Sample search entry for Email: Tide pools" {
		t.Fatalf("answered %+v", got)
	}
}
