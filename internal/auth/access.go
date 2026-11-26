package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	AccessLength   = 30 * 24 * time.Hour
	accessPurpose  = "read and write access"
	durablePurpose = "read and write durable access"
)

var (
	ErrToken     = errors.New("not a token this server issued")
	ErrExpired   = errors.New("expired")
	ErrSignedOut = errors.New("signed out, or no longer in the directory")
)

type accessToken struct {
	Email  string `json:"e"`
	Issued int64  `json:"i"`
	Person string `json:"p,omitempty"`
	Since  int64  `json:"s,omitempty"`
}

func sealMAC(key []byte, purpose, body string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(purpose + "\n" + body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func Seal(key []byte, purpose string, v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	body := base64.RawURLEncoding.EncodeToString(payload)
	return body + "." + sealMAC(key, purpose, body)
}

func Unseal(key []byte, purpose, sealed string, v any) bool {
	body, sig, ok := strings.Cut(sealed, ".")
	if !ok || !hmac.Equal([]byte(sealMAC(key, purpose, body)), []byte(sig)) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func AccessKey(sessionKey []byte) []byte {
	h := hmac.New(sha256.New, sessionKey)
	h.Write([]byte("mcp tokens"))
	return h.Sum(nil)
}

type Tokens struct {
	Key      []byte
	Sessions interface {
		SignedOut(email string) (time.Time, bool)
	}
	Person func(email string) string
	Now    func() time.Time
}

func (t Tokens) Issue(email string, issued time.Time) string {
	return Seal(t.Key, accessPurpose, accessToken{Email: email, Issued: issued.Unix()})
}

func (t Tokens) IssueFor(email, person string, issued, since time.Time) string {
	return Seal(t.Key, accessPurpose, accessToken{Email: email, Issued: issued.Unix(), Person: person, Since: since.Unix()})
}

func (t Tokens) Durable(email, person string, issued time.Time) string {
	return Seal(t.Key, durablePurpose, accessToken{Email: email, Issued: issued.Unix(), Person: person})
}

func (t Tokens) Verify(token string) (string, time.Time, error) {
	var a accessToken
	var expires time.Time
	switch {
	case len(t.Key) == 0:
		return "", time.Time{}, ErrToken
	case Unseal(t.Key, accessPurpose, token, &a):
		expires = time.Unix(a.Issued, 0).Add(AccessLength)
		if t.Now().After(expires) {
			return "", time.Time{}, ErrExpired
		}
	case !Unseal(t.Key, durablePurpose, token, &a):
		return "", time.Time{}, ErrToken
	case a.Person == "":
		return "", time.Time{}, ErrToken
	}
	since := a.Issued
	if a.Since != 0 {
		since = a.Since
	}
	if out, ok := t.Sessions.SignedOut(a.Email); ok && since <= out.Unix() {
		return "", time.Time{}, ErrSignedOut
	}
	person := t.Person(a.Email)
	if person == "" || a.Person != "" && a.Person != person {
		return "", time.Time{}, fmt.Errorf("%w: %s", ErrSignedOut, a.Email)
	}
	return a.Email, expires, nil
}
