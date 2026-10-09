package ask

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

type prompt struct {
	System []string          `json:"system"`
	Links  map[string]string `json:"links"`
	Day    string            `json:"day"`
	Seal   string            `json:"seal,omitempty"`
}

func (a app) sealOf(email string, p prompt) string {
	p.Seal = ""
	raw, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	mac := hmac.New(sha256.New, a.chatKey)
	mac.Write([]byte("ask prompt\x00" + email + "\x00"))
	mac.Write(raw)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (a app) sealed(email string, p prompt) prompt {
	p.Seal = a.sealOf(email, p)
	return p
}

func (a app) intact(email string, p prompt) bool {
	return hmac.Equal([]byte(p.Seal), []byte(a.sealOf(email, p)))
}

func statedDay(p prompt) string {
	for _, text := range p.System {
		if _, rest, ok := strings.Cut(text, "Today is "); ok {
			day, _, _ := strings.Cut(rest, ". ")
			return day
		}
	}
	return ""
}
