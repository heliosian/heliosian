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

const AccessLength = 30 * 24 * time.Hour

var (
	ErrToken     = errors.New("not a token this server issued")
	ErrExpired   = errors.New("expired")
	ErrSignedOut = errors.New("signed out, or no longer in the directory")
)

type accessToken struct {
	Email  string `json:"e"`
	Issued int64  `json:"i"`
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

type Tokens struct {
	Key      []byte
	Sessions interface {
		SignedOut(email string) (time.Time, bool)
	}
	SignedIn func(email string) bool
	Now      func() time.Time
}

func (t Tokens) Issue(email string) string {
	return Seal(t.Key, "access", accessToken{Email: email, Issued: t.Now().Unix()})
}

func (t Tokens) Verify(token string) (string, time.Time, error) {
	var a accessToken
	if len(t.Key) == 0 || !Unseal(t.Key, "access", token, &a) {
		return "", time.Time{}, ErrToken
	}
	expires := time.Unix(a.Issued, 0).Add(AccessLength)
	if t.Now().After(expires) {
		return "", time.Time{}, ErrExpired
	}
	if out, ok := t.Sessions.SignedOut(a.Email); ok && a.Issued <= out.Unix() {
		return "", time.Time{}, ErrSignedOut
	}
	if !t.SignedIn(a.Email) {
		return "", time.Time{}, fmt.Errorf("%w: %s", ErrSignedOut, a.Email)
	}
	return a.Email, expires, nil
}
