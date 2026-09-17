package mail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMailgunSendsRawToEachRecipient(t *testing.T) {
	var path, auth, contentType string
	var to []string
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		to = r.MultipartForm.Value["to"]
		f, _, err := r.FormFile("message")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(f)
		raw = string(body)
		w.Write([]byte(`{"id":"<x@y>","message":"Queued. Thank you."}`))
	}))
	defer srv.Close()
	m := &Mailgun{Key: "key-test", Domain: "loop.example.org", Endpoint: srv.URL}
	if err := m.SendRaw(context.Background(), "team@loop.example.org", []string{"a@example.org"}, []byte("From: x\r\n\r\nhi\r\n")); err != nil {
		t.Fatal(err)
	}
	if path != "/v3/loop.example.org/messages.mime" || !strings.HasPrefix(contentType, "multipart/form-data") || len(to) != 1 || to[0] != "a@example.org" || raw != "From: x\r\n\r\nhi\r\n" {
		t.Fatalf("posted %s %s to %v: %q", path, contentType, to, raw)
	}
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("api", "key-test")
	if auth != req.Header.Get("Authorization") {
		t.Fatalf("auth %q", auth)
	}
}

func TestMailgunReportsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Domain not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	m := &Mailgun{Key: "key-test", Domain: "loop.example.org", Endpoint: srv.URL}
	err := m.SendRaw(context.Background(), "x", []string{"a@example.org"}, []byte("hi"))
	if err == nil || !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "Domain not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestMailgunFetchesTheStoredMessage(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		w.Write([]byte(`{"recipient":"team@loop.example.org","Body-mime":"From: a@x.org\r\nSubject: hi\r\n\r\nhello\r\n"}`))
	}))
	defer srv.Close()
	m := &Mailgun{Key: "key-test", Domain: "loop.example.org"}
	raw, err := m.Stored(context.Background(), srv.URL+"/v3/domains/loop.example.org/messages/abc")
	if err != nil {
		t.Fatal(err)
	}
	if accept != "message/rfc2822" || string(raw) != "From: a@x.org\r\nSubject: hi\r\n\r\nhello\r\n" {
		t.Fatalf("accept %q raw %q", accept, raw)
	}
}

func TestVerifyMailgun(t *testing.T) {
	at := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	stamp, sig := SignMailgun("signing-key", "token-1", at)
	if err := VerifyMailgun("signing-key", stamp, "token-1", sig, at.Add(time.Minute)); err != nil {
		t.Errorf("good call refused: %v", err)
	}
	if err := VerifyMailgun("signing-key", stamp, "token-1", strings.ToUpper(sig), at); err != nil {
		t.Errorf("uppercase hex refused: %v", err)
	}
	if err := VerifyMailgun("signing-key", stamp, "token-2", sig, at); err == nil {
		t.Errorf("a changed token was taken")
	}
	if err := VerifyMailgun("other-key", stamp, "token-1", sig, at); err == nil {
		t.Errorf("another key's call was taken")
	}
	if err := VerifyMailgun("signing-key", stamp, "token-1", sig, at.Add(time.Hour)); err == nil {
		t.Errorf("a stale call was taken")
	}
	if err := VerifyMailgun("", stamp, "token-1", sig, at); err == nil {
		t.Errorf("a call was taken with no key")
	}
}
