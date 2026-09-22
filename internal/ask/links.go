package ask

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"
)

var (
	address     = regexp.MustCompile(`https?://[^\s"\\<>()\[\]{}|^` + "`" + `]+`)
	linkTarget  = regexp.MustCompile(`\]\(L([0-9a-f]{8})(?:\s[^)\n]*)?\)`)
	quotedKey   = regexp.MustCompile(`"L([0-9a-f]{8})"`)
	bareKey     = regexp.MustCompile(`\bL([0-9a-f]{8})([.,;:!?'*_]*(?:[\s"\\<>()\[\]{}|^` + "`" + `]|$))`)
	partialLink = regexp.MustCompile(`^\[[^\]\n]{0,400}(\](\([^)\n]{0,200})?)?$|^https?://[^\s]{0,300}$`)
	fullLink    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

type links struct {
	mu   sync.Mutex
	urls map[string]string
}

func newLinks() *links {
	return &links{urls: map[string]string{}}
}

func keyOf(url string) string {
	sum := sha256.Sum256([]byte(url))
	return "L" + hex.EncodeToString(sum[:4])
}

func (l *links) shorten(text string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return address.ReplaceAllStringFunc(text, func(match string) string {
		url := strings.TrimRight(match, ".,;:!?'*_")
		key := keyOf(url)
		l.urls[key] = url
		return key + match[len(url):]
	})
}

func (l *links) url(key string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	url, ok := l.urls["L"+key]
	if !ok {
		slog.Error("[ERROR] ask: unknown link key", "key", "L"+key)
	}
	return url, ok
}

func (l *links) known(url string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.urls[keyOf(url)]
	return ok
}

func (l *links) expand(text string) string {
	text = linkTarget.ReplaceAllStringFunc(text, func(match string) string {
		url, ok := l.url(linkTarget.FindStringSubmatch(match)[1])
		if !ok {
			return match
		}
		return "](" + url + ")"
	})
	text = fullLink.ReplaceAllStringFunc(text, func(match string) string {
		m := fullLink.FindStringSubmatch(match)
		if l.known(m[2]) {
			return match
		}
		slog.Error("[ERROR] ask: dropped a link the model made up", "target", m[2])
		return m[1]
	})
	return address.ReplaceAllStringFunc(text, func(match string) string {
		url := strings.TrimRight(match, ".,;:!?'*_")
		if l.known(url) {
			return match
		}
		slog.Error("[ERROR] ask: dropped an address the model made up", "target", url)
		return match[len(url):]
	})
}

// A key is expanded only where what follows would be trimmed off or end
// an address, so shortening the result gives the same text back.
func (l *links) expandAll(text string) string {
	return bareKey.ReplaceAllStringFunc(text, func(match string) string {
		m := bareKey.FindStringSubmatch(match)
		l.mu.Lock()
		url, ok := l.urls["L"+m[1]]
		l.mu.Unlock()
		if !ok {
			return match
		}
		return url + m[2]
	})
}

func (l *links) expandInput(input []byte) []byte {
	return quotedKey.ReplaceAllFunc(input, func(match []byte) []byte {
		url, ok := l.url(string(quotedKey.FindSubmatch(match)[1]))
		if !ok {
			return match
		}
		quoted, err := json.Marshal(url)
		if err != nil {
			return match
		}
		return quoted
	})
}

// mapStrings applies f to every string in decoded JSON but those inside a
// thinking block, which goes back to the model exactly as it came.
func mapStrings(v any, f func(string) string) any {
	switch v := v.(type) {
	case string:
		return f(v)
	case []any:
		for i := range v {
			v[i] = mapStrings(v[i], f)
		}
		return v
	case map[string]any:
		if kind, _ := v["type"].(string); kind == "thinking" || kind == "redacted_thinking" {
			return v
		}
		for k := range v {
			v[k] = mapStrings(v[k], f)
		}
		return v
	}
	return v
}

var expandedTarget = regexp.MustCompile(`\]\((https?://[^)\s]+)\)`)

type expander struct {
	links *links
	emit  Emitter
	cards func(address string) (linkCard, bool)
	sent  map[string]bool
	held  string
}

func (e *expander) out(text string) {
	text = e.links.expand(text)
	for _, m := range expandedTarget.FindAllStringSubmatch(text, -1) {
		if e.sent[m[1]] {
			continue
		}
		e.sent[m[1]] = true
		if card, ok := e.cards(m[1]); ok {
			e.emit("card", card)
		}
	}
	e.emit("text", text)
}

func (e *expander) send(kind string, data any) {
	text, ok := data.(string)
	if kind != "text" || !ok {
		e.flush()
		e.emit(kind, data)
		return
	}
	text = e.held + text
	cut := len(text)
	for _, start := range []string{"[", "http"} {
		if i := strings.LastIndex(text, start); i >= 0 && i < cut && partialLink.MatchString(text[i:]) {
			cut = i
		}
	}
	e.held = text[cut:]
	if cut > 0 {
		e.out(text[:cut])
	}
}

func (e *expander) flush() {
	if e.held == "" {
		return
	}
	e.out(e.held)
	e.held = ""
}
