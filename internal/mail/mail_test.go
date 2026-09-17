package mail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestMailgunPostsTheMessage(t *testing.T) {
	var path, contentType string
	var got map[string][]string
	var attachment struct{ name, kind, content string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		got = r.MultipartForm.Value
		f, header, err := r.FormFile("attachment")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(f)
		attachment.name, attachment.kind, attachment.content = header.Filename, header.Header.Get("Content-Type"), string(body)
		w.Write([]byte(`{"id":"<x@example.org>","message":"Queued. Thank you."}`))
	}))
	defer srv.Close()
	m := &Mailgun{Key: "key-test", From: "HCA-Team <team@example.org>", Endpoint: srv.URL}
	msg := Message{To: []string{"a@example.org"}, CC: []string{"b@example.org"}, ReplyTo: []string{"b@example.org"}, Subject: "Hi", HTML: "<p>Hi</p>", Text: "Hi",
		Headers:     map[string]string{"Message-Id": "<one@example.org>", "In-Reply-To": "<zero@example.org>"},
		Attachments: []Attachment{{Name: "invite.ics", ContentType: "text/calendar; method=REQUEST", Content: []byte("BEGIN:VCALENDAR")}}}
	if err := m.Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if path != "/v3/example.org/messages" || !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Fatalf("posted %s as %s", path, contentType)
	}
	want := map[string][]string{
		"from": {"HCA-Team <team@example.org>"}, "to": {"a@example.org"}, "cc": {"b@example.org"}, "subject": {"Hi"}, "html": {"<p>Hi</p>"}, "text": {"Hi"},
		"h:Reply-To": {"b@example.org"}, "h:Message-Id": {"<one@example.org>"}, "h:In-Reply-To": {"<zero@example.org>"},
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Errorf("%s: got %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("fields %v", got)
	}
	if attachment.name != "invite.ics" || attachment.kind != "text/calendar; method=REQUEST" || attachment.content != "BEGIN:VCALENDAR" {
		t.Errorf("attachment %+v", attachment)
	}
}

func TestComposeCarriesReplyTo(t *testing.T) {
	out := Compose("HCA-Team <team@example.org>", Message{To: []string{"a@example.org"}, ReplyTo: []string{"b@example.org"}, Subject: "Hi", HTML: "<p>Hi</p>"})
	if !strings.Contains(out, "Reply-To: b@example.org\r\n") || !strings.Contains(out, "multipart/alternative") {
		t.Fatalf("composed:\n%s", out)
	}
}
