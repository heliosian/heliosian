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
	"maps"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/textproto"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Mailgun struct {
	Key, From string
	Endpoint  string
}

func NewMailgun(key, from string) *Mailgun {
	return &Mailgun{Key: key, From: from}
}

func (m *Mailgun) api() string {
	if m.Endpoint != "" {
		return m.Endpoint
	}
	return "https://api.mailgun.net"
}

func domainOf(from string) (string, error) {
	address := from
	if parsed, err := mail.ParseAddress(from); err == nil {
		address = parsed.Address
	}
	_, domain, ok := strings.Cut(address, "@")
	if !ok || domain == "" {
		return "", fmt.Errorf("mail: no domain in %q", from)
	}
	return domain, nil
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

func (m *Mailgun) post(ctx context.Context, domain, endpoint string, body *bytes.Buffer, contentType string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequest(http.MethodPost, m.api()+"/v3/"+domain+"/"+endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := m.do(ctx, req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func filePart(form *multipart.Writer, field, name, contentType string, content []byte) error {
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, name))
	header.Set("Content-Type", contentType)
	part, err := form.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = part.Write(content)
	return err
}

func (m *Mailgun) Send(ctx context.Context, msg Message) error {
	if len(msg.To) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	domain, err := domainOf(m.From)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := [][2]string{{"from", m.From}, {"subject", msg.Subject}, {"html", msg.HTML}}
	if msg.Text != "" {
		fields = append(fields, [2]string{"text", msg.Text})
	}
	for _, to := range msg.To {
		fields = append(fields, [2]string{"to", to})
	}
	for _, cc := range msg.CC {
		fields = append(fields, [2]string{"cc", cc})
	}
	if len(msg.ReplyTo) > 0 {
		fields = append(fields, [2]string{"h:Reply-To", strings.Join(msg.ReplyTo, ", ")})
	}
	for _, k := range slices.Sorted(maps.Keys(msg.Headers)) {
		fields = append(fields, [2]string{"h:" + k, msg.Headers[k]})
	}
	for _, f := range fields {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return err
		}
	}
	for _, a := range msg.Attachments {
		if err := filePart(form, "attachment", a.Name, a.ContentType, a.Content); err != nil {
			return err
		}
	}
	if err := form.Close(); err != nil {
		return err
	}
	return m.post(ctx, domain, "messages", &body, form.FormDataContentType())
}

func (m *Mailgun) SendRaw(ctx context.Context, from string, to []string, raw []byte) error {
	if len(to) == 0 {
		return fmt.Errorf("mail: no recipient")
	}
	domain, err := domainOf(from)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for _, rcpt := range to {
		if err := form.WriteField("to", rcpt); err != nil {
			return err
		}
	}
	if err := filePart(form, "message", "message.eml", "message/rfc822", raw); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	return m.post(ctx, domain, "messages.mime", &body, form.FormDataContentType())
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

const MaxNotification = 40 << 20

func Notification(w http.ResponseWriter, r *http.Request) (map[string]string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxNotification)
	fields := map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			return nil, err
		}
		for k, v := range raw {
			if s, ok := v.(string); ok {
				fields[strings.ToLower(k)] = s
			}
		}
		return fields, nil
	}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(MaxNotification); err != nil {
			return nil, err
		}
	} else if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for k, v := range r.Form {
		if len(v) > 0 {
			fields[strings.ToLower(k)] = v[0]
		}
	}
	return fields, nil
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

func VerifyNotification(key string, fields map[string]string, now time.Time) error {
	return VerifyMailgun(key, fields["timestamp"], fields["token"], fields["signature"], now)
}

func SignMailgun(key, token string, at time.Time) (timestamp, signature string) {
	timestamp = strconv.FormatInt(at.Unix(), 10)
	return timestamp, mailgunSignature(key, timestamp, token)
}
