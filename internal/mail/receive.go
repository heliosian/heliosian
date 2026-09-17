package mail

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Receiving: Resend takes in mail for a domain whose MX record points at
// it, and calls a webhook for each message with no more than the message's
// id and headers; the message itself is fetched back from the API, raw,
// as it came over the wire. The webhook is signed the Svix way, and the
// signature is checked before anything is fetched.

// Received is one message Resend took in: who sent it, what it was about,
// and the whole of it as it came over the wire, for a parser to walk.
type Received struct {
	ID, From, Subject string
	Raw               []byte
}

// Inbox fetches received messages back from Resend's API by their id, the
// key as a bearer token; Endpoint stands in for the API in tests.
type Inbox struct {
	Key      string
	Endpoint string
}

// NewInbox is the inbox the key opens, nil without one - and a nil *Inbox
// must stay a nil interface where it is handed on.
func NewInbox(resendKey string) *Inbox {
	if resendKey == "" {
		return nil
	}
	return &Inbox{Key: resendKey}
}

func (in *Inbox) api() string {
	if in.Endpoint != "" {
		return in.Endpoint
	}
	return "https://api.resend.com"
}

// Received fetches one message: its record, then the raw message the
// record's signed link serves.
func (in *Inbox) Received(ctx context.Context, id string) (Received, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var record struct {
		ID      string `json:"id"`
		From    string `json:"from"`
		Subject string `json:"subject"`
		Raw     *struct {
			DownloadURL string `json:"download_url"`
		} `json:"raw"`
	}
	if err := in.get(ctx, in.api()+"/emails/receiving/"+id, true, func(body io.Reader) error {
		return json.NewDecoder(body).Decode(&record)
	}); err != nil {
		return Received{}, err
	}
	out := Received{ID: record.ID, From: record.From, Subject: record.Subject}
	if record.Raw == nil || record.Raw.DownloadURL == "" {
		return out, fmt.Errorf("mail: received %s has no raw message", id)
	}
	err := in.get(ctx, record.Raw.DownloadURL, false, func(body io.Reader) error {
		raw, err := io.ReadAll(io.LimitReader(body, 40<<20))
		out.Raw = raw
		return err
	})
	return out, err
}

func (in *Inbox) get(ctx context.Context, url string, auth bool, read func(io.Reader) error) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+in.Key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("mail: resend %d: %s", resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	return read(resp.Body)
}

// VerifyWebhook checks a webhook call the Svix way: the secret is
// "whsec_" and base64; the signed content is the message id, the timestamp
// and the raw body joined by dots; the signature header carries one or
// more "v1,<base64 HMAC-SHA256>" separated by spaces, any one of which may
// match; and the timestamp must be within five minutes of now.
func VerifyWebhook(secret string, header http.Header, body []byte, now time.Time) error {
	id, stamp, signatures := header.Get("svix-id"), header.Get("svix-timestamp"), header.Get("svix-signature")
	if id == "" || stamp == "" || signatures == "" {
		return fmt.Errorf("mail: webhook headers missing")
	}
	at, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return fmt.Errorf("mail: webhook timestamp: %w", err)
	}
	if d := now.Unix() - at; d > 300 || d < -300 {
		return fmt.Errorf("mail: webhook timestamp is %d seconds off", d)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return fmt.Errorf("mail: webhook secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%s.", id, stamp)
	mac.Write(body)
	want := mac.Sum(nil)
	for _, s := range strings.Fields(signatures) {
		version, sig, ok := strings.Cut(s, ",")
		if !ok || version != "v1" {
			continue
		}
		got, err := base64.StdEncoding.DecodeString(sig)
		if err == nil && subtle.ConstantTimeCompare(got, want) == 1 {
			return nil
		}
	}
	return fmt.Errorf("mail: webhook signature does not match")
}

// SignWebhook makes the headers VerifyWebhook accepts for a body, for tests
// and for a hand-made call.
func SignWebhook(secret, id string, at time.Time, body []byte) http.Header {
	key, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%s.", id, stamp)
	mac.Write(body)
	h := http.Header{}
	h.Set("svix-id", id)
	h.Set("svix-timestamp", stamp)
	h.Set("svix-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	return h
}
