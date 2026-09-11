package mail

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResendPostsTheMessage(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.Write([]byte(`{"id":"1"}`))
	}))
	defer srv.Close()
	r := &Resend{Key: "re_test", From: "HCA-Team <team@example.org>", Endpoint: srv.URL}
	m := Message{To: []string{"a@example.org"}, CC: []string{"b@example.org"}, ReplyTo: []string{"b@example.org"}, Subject: "Hi", HTML: "<p>Hi</p>", Text: "Hi"}
	if err := r.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_test" || got["from"] != r.From || got["subject"] != "Hi" || got["html"] != "<p>Hi</p>" {
		t.Fatalf("posted %v with %q", got, auth)
	}
	if cc, _ := got["cc"].([]any); len(cc) != 1 || cc[0] != "b@example.org" {
		t.Fatalf("cc %v", got["cc"])
	}
	if rt, _ := got["reply_to"].([]any); len(rt) != 1 {
		t.Fatalf("reply_to %v", got["reply_to"])
	}
}

func TestResendReportsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"domain not verified"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	r := &Resend{Key: "re_test", From: "x@example.org", Endpoint: srv.URL}
	err := r.Send(context.Background(), Message{To: []string{"a@example.org"}, Subject: "Hi", HTML: "<p>Hi</p>"})
	if err == nil || !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "domain not verified") {
		t.Fatalf("err = %v", err)
	}
}

func TestComposeCarriesReplyTo(t *testing.T) {
	out := Compose("HCA-Team <team@example.org>", Message{To: []string{"a@example.org"}, ReplyTo: []string{"b@example.org"}, Subject: "Hi", HTML: "<p>Hi</p>"})
	if !strings.Contains(out, "Reply-To: b@example.org\r\n") || !strings.Contains(out, "multipart/alternative") {
		t.Fatalf("composed:\n%s", out)
	}
}
