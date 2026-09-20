package ask

import (
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"heliosian/internal/logging"
)

// keyChars is the alphabet a key's tail is drawn from, less the letters and
// digits that read alike in a proportional font.
const keyChars = "abcdefghjkmnpqrstuvwxyz23456789"

// keyLength is how many of those a key carries. Keys are random rather than
// counted up, so there is no next one to guess: a model that wants to link
// something it was given no key for cannot continue the sequence, which is
// what it used to do - inventing L46 through L49 for four people whose names
// it had and whose links it did not.
const keyLength = 4

var (
	address     = regexp.MustCompile(`https?://[^\s"\\<>()\[\]{}|^` + "`" + `]+`)
	linkTarget  = regexp.MustCompile(`\]\(L([a-z0-9]+)(?:\s[^)\n]*)?\)`)
	quotedKey   = regexp.MustCompile(`"L([a-z0-9]+)"`)
	partialLink = regexp.MustCompile(`^\]\(?(L[a-z0-9]*(\s[^)\n]{0,120})?)?$`)
)

type links struct {
	mu   sync.Mutex
	keys map[string]string
	urls map[string]string
}

func newLinks() *links {
	return &links{keys: map[string]string{}, urls: map[string]string{}}
}

// mint draws a key no other url in this conversation holds.
func (l *links) mint() string {
	for {
		b := make([]byte, keyLength)
		if _, err := rand.Read(b); err != nil {
			logging.Fatal("ask: read random bytes", "error", err)
		}
		key := "L"
		for _, c := range b {
			key += string(keyChars[int(c)%len(keyChars)])
		}
		if _, taken := l.urls[key]; !taken {
			return key
		}
	}
}

func (l *links) shorten(text string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return address.ReplaceAllStringFunc(text, func(match string) string {
		url := strings.TrimRight(match, ".,;:!?'*_")
		key, ok := l.keys[url]
		if !ok {
			key = l.mint()
			l.keys[url] = key
			l.urls[key] = url
		}
		return key + match[len(url):]
	})
}

func (l *links) url(tail string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	url, ok := l.urls["L"+tail]
	if !ok {
		slog.Error("[ERROR] ask: unknown link key", "key", "L"+tail)
		return "", false
	}
	return url, true
}

func (l *links) expand(text string) string {
	return linkTarget.ReplaceAllStringFunc(text, func(match string) string {
		url, ok := l.url(linkTarget.FindStringSubmatch(match)[1])
		if !ok {
			return match
		}
		return "](" + url + ")"
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
	if i := strings.LastIndex(text, "]"); i >= 0 && partialLink.MatchString(text[i:]) {
		cut = i
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
