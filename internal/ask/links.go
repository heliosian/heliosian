package ask

import (
	"encoding/json"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var (
	address     = regexp.MustCompile(`https?://[^\s"\\<>()\[\]{}|^` + "`" + `]+`)
	linkTarget  = regexp.MustCompile(`\]\(L(\d+)(?:\s[^)\n]*)?\)`)
	quotedKey   = regexp.MustCompile(`"L(\d+)"`)
	partialLink = regexp.MustCompile(`^\[[^\]\n]{0,400}(\](\([^)\n]{0,200})?)?$|^https?://[^\s]{0,300}$`)
	fullLink    = regexp.MustCompile(`\[([^\]\n]+)\]\(([^)\s]+)\)`)
)

type links struct {
	mu   sync.Mutex
	keys map[string]string
	urls []string
}

func newLinks() *links {
	return &links{keys: map[string]string{}, urls: []string{}}
}

func (l *links) shorten(text string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return address.ReplaceAllStringFunc(text, func(match string) string {
		url := strings.TrimRight(match, ".,;:!?'*_")
		key, ok := l.keys[url]
		if !ok {
			l.urls = append(l.urls, url)
			key = "L" + strconv.Itoa(len(l.urls))
			l.keys[url] = key
		}
		return key + match[len(url):]
	})
}

func (l *links) url(digits string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := strconv.Atoi(digits)
	if err != nil || n < 1 || n > len(l.urls) {
		slog.Error("[ERROR] ask: unknown link key", "key", "L"+digits)
		return "", false
	}
	return l.urls[n-1], true
}

func (l *links) known(url string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, ok := l.keys[url]
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
