package id

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"strings"
)

const Alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

const (
	Length      = 13
	tokenLength = 24
)

func New(taken func(string) bool) string {
	for {
		s := random(Length)
		if !taken(s) {
			return s
		}
	}
}

func Token() string {
	return random(tokenLength)
}

func Of(key []byte, kind, value string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(kind + "\x00" + value))
	return encode(mac.Sum(nil), Length)
}

func Parse(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) != Length {
		return "", false
	}
	for _, r := range s {
		if !strings.ContainsRune(Alphabet, r) {
			return "", false
		}
	}
	return s, true
}

func random(n int) string {
	raw := make([]byte, (n*5+7)/8)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return encode(raw, n)
}

func encode(raw []byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		bit := i * 5
		v := int(raw[bit/8])<<8 | int(at(raw, bit/8+1))
		out[i] = Alphabet[(v>>(11-bit%8))&31]
	}
	return string(out)
}

func at(raw []byte, i int) byte {
	if i >= len(raw) {
		return 0
	}
	return raw[i]
}
