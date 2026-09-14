package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A webhook call is taken when its signature matches the body and its
// timestamp is fresh, and refused when either is off.
func TestVerifyWebhook(t *testing.T) {
	secret := "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
	body := []byte(`{"type":"email.received"}`)
	at := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	h := SignWebhook(secret, "msg_1", at, body)
	if err := VerifyWebhook(secret, h, body, at.Add(time.Minute)); err != nil {
		t.Errorf("good call refused: %v", err)
	}
	if err := VerifyWebhook(secret, h, []byte(`{"type":"email.sent"}`), at); err == nil {
		t.Errorf("a changed body was taken")
	}
	if err := VerifyWebhook(secret, h, body, at.Add(time.Hour)); err == nil {
		t.Errorf("a stale call was taken")
	}
	if err := VerifyWebhook("whsec_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", h, body, at); err == nil {
		t.Errorf("another secret's call was taken")
	}
	if err := VerifyWebhook(secret, http.Header{}, body, at); err == nil {
		t.Errorf("an unsigned call was taken")
	}
}

// The inbox fetches the record, then the raw message its link serves.
func TestInboxReceived(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/emails/receiving/abc":
			auth = r.Header.Get("Authorization")
			w.Write([]byte(`{"id":"abc","from":"Sam <sam@example.org>","subject":"Accepted: Picnic","raw":{"download_url":"http://` + r.Host + `/raw/abc"}}`))
		case "/raw/abc":
			w.Write([]byte("From: sam@example.org\r\nSubject: Accepted: Picnic\r\n\r\nhello\r\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	in := &Inbox{Key: "re_test", Endpoint: srv.URL}
	got, err := in.Received(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_test" || got.From != "Sam <sam@example.org>" || got.Subject != "Accepted: Picnic" || string(got.Raw) != "From: sam@example.org\r\nSubject: Accepted: Picnic\r\n\r\nhello\r\n" {
		t.Errorf("got %+v with %q", got, auth)
	}
}
