package mail

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Mailgun struct {
	Key, Domain string
	Endpoint    string
}

func NewMailgun(key, domain string) *Mailgun {
	return &Mailgun{Key: key, Domain: domain}
}

func (m *Mailgun) api() string {
	if m.Endpoint != "" {
		return m.Endpoint
	}
	return "https://api.mailgun.net"
}

func (m *Mailgun) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req.SetBasicAuth("api", m.Key)
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		resp.Body.Close()
		return nil, fmt.Errorf("mail: mailgun %d: %s", resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	return resp, nil
}

func (m *Mailgun) SendRaw(ctx context.Context, _ string, to []string, raw []byte) error {
	if len(to) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, rcpt := range to {
		if err := form.WriteField("to", rcpt); err != nil {
			return err
		}
	}
	part, err := form.CreateFormFile("message", "message.eml")
	if err != nil {
		return err
	}
	if _, err := part.Write(raw); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodPost, m.api()+"/v3/"+m.Domain+"/messages.mime", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := m.do(ctx, req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (m *Mailgun) Stored(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "message/rfc2822")
	resp, err := m.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var stored struct {
		Raw string `json:"body-mime"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&stored); err != nil {
		return nil, fmt.Errorf("mail: read the stored message: %w", err)
	}
	if stored.Raw == "" {
		return nil, fmt.Errorf("mail: the stored message at %s has no body-mime", url)
	}
	return []byte(stored.Raw), nil
}

func mailgunSignature(key, timestamp, token string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(timestamp + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyMailgun(key, timestamp, token, signature string, now time.Time) error {
	if key == "" {
		return fmt.Errorf("mail: no mailgun signing key")
	}
	if timestamp == "" || token == "" || signature == "" {
		return fmt.Errorf("mail: mailgun signature fields missing")
	}
	at, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("mail: mailgun timestamp: %w", err)
	}
	if d := now.Unix() - at; d > 900 || d < -900 {
		return fmt.Errorf("mail: mailgun timestamp is %d seconds off", d)
	}
	if subtle.ConstantTimeCompare([]byte(mailgunSignature(key, timestamp, token)), []byte(strings.ToLower(signature))) != 1 {
		return fmt.Errorf("mail: mailgun signature does not match")
	}
	return nil
}

func SignMailgun(key, token string, at time.Time) (timestamp, signature string) {
	timestamp = strconv.FormatInt(at.Unix(), 10)
	return timestamp, mailgunSignature(key, timestamp, token)
}
