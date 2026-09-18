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
	partialLink = regexp.MustCompile(`^\]\(?(L\d*(\s[^)\n]{0,120})?)?$`)
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
