package ask

import (
	"slices"
	"strings"
	"testing"
	"time"
)

// keyFor is the key a conversation minted for one address. Keys are random, so
// a test asks for the one that was issued rather than assuming its shape.
func keyFor(t *testing.T, l *links, url string) string {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	key, ok := l.keys[url]
	if !ok {
		t.Fatalf("no key was minted for %s", url)
	}
	return key
}

func TestLinksShortenAndExpand(t *testing.T) {
	l := newLinks()
	const (
		sam  = "https://who.heliosian.com/people/sam"
		x    = "https://x.org/a?b=1&c=2"
		form = "https://docs.google.com/d/abc"
	)
	got := l.shorten(`{"link":"` + sam + `","more":"see ` + x + `. and [form](` + form + `) or ` + sam + `"}`)
	samKey, xKey, formKey := keyFor(t, l, sam), keyFor(t, l, x), keyFor(t, l, form)
	want := `{"link":"` + samKey + `","more":"see ` + xKey + `. and [form](` + formKey + `) or ` + samKey + `"}`
	if got != want {
		t.Fatalf("shorten gave %s, want %s", got, want)
	}
	// One url always takes the same key, and no two take the same one.
	if samKey == xKey || samKey == formKey || xKey == formKey {
		t.Fatalf("keys collided: %s %s %s", samKey, xKey, formKey)
	}
	if got := l.expand("[Sam](" + samKey + ") and [the form](" + formKey + "), [x](Lzzzz)"); got != "[Sam]("+sam+") and [the form]("+form+"), [x](Lzzzz)" {
		t.Fatalf("expand gave %s", got)
	}
	if got := l.expand(`[Sam](` + samKey + ` "") and [the form](` + formKey + ` 'Sign up'), [the reminder](` + samKey + ` — see below)`); got != "[Sam]("+sam+") and [the form]("+form+"), [the reminder]("+sam+")" {
		t.Fatalf("expand of links with words after the key gave %s", got)
	}
	if got := string(l.expandInput([]byte(`{"path":"` + xKey + `","id":"Lzzzz"}`))); got != `{"path":"https://x.org/a?b=1`+"\\"+`u0026c=2","id":"Lzzzz"}` {
		t.Fatalf("expandInput gave %s", got)
	}
}

// A key drawn at random cannot be guessed from the ones already issued, which
// is what the model used to do when it wanted to link something it had no key
// for.
func TestLinkKeysAreNotSequential(t *testing.T) {
	l := newLinks()
	seen := map[string]bool{}
	for _, url := range []string{"https://a.example/1", "https://a.example/2", "https://a.example/3", "https://a.example/4"} {
		key := strings.TrimSpace(l.shorten(url))
		if seen[key] {
			t.Fatalf("key %s was issued twice", key)
		}
		seen[key] = true
		if len(key) != keyLength+1 || !strings.HasPrefix(key, "L") {
			t.Fatalf("key %q is not L plus %d characters", key, keyLength)
		}
		for _, c := range key[1:] {
			if !strings.ContainsRune(keyChars, c) {
				t.Fatalf("key %q uses %q, which is not in the alphabet", key, c)
			}
		}
	}
}

func TestExpanderHoldsATitledKey(t *testing.T) {
	l := newLinks()
	const family = "https://who.heliosian.com/families/donhowe"
	l.shorten(family)
	key := keyFor(t, l, family)
	out := &strings.Builder{}
	e := &expander{links: l, emit: func(kind string, data any) {
		if kind == "text" {
			out.WriteString(data.(string))
		}
	}, cards: func(string) (linkCard, bool) { return linkCard{}, false }, sent: map[string]bool{}}
	// The key arrives split across chunks, with a title after it.
	for _, piece := range []string{"The [Donhowe Family](" + key[:2], key[2:] + " ", `"`, `"`, ") and [the ", "same](" + key, " — see", " below) live in Mountain View"} {
		e.send("text", piece)
		if strings.Contains(out.String(), key) {
			t.Fatalf("the key went out raw: %q", out.String())
		}
	}
	e.flush()
	if out.String() != "The [Donhowe Family]("+family+") and [the same]("+family+") live in Mountain View" {
		t.Fatalf("streamed %q", out.String())
	}
}

func TestRestoreShortensAddresses(t *testing.T) {
	c := newStore().start("parent@example.com", nil, time.Now())
	c.links.shorten("https://who.heliosian.com")
	c.restore([]turn{
		{Role: "user", Text: "Is https://when.heliosian.com/e/abc on?"},
		{Role: "assistant", Text: "Yes: [the picnic](https://when.heliosian.com/e/abc), with [Sam](https://who.heliosian.com/people/sam)."},
	})
	picnic := keyFor(t, c.links, "https://when.heliosian.com/e/abc")
	sam := keyFor(t, c.links, "https://who.heliosian.com/people/sam")
	question := c.messages[0].Content[0].OfText.Text
	answer := c.messages[1].Content[0].OfText.Text
	if question != "Is "+picnic+" on?" || answer != "Yes: [the picnic]("+picnic+"), with [Sam]("+sam+")." {
		t.Fatalf("restored %q and %q", question, answer)
	}
	if got := c.links.expand(answer); got != "Yes: [the picnic](https://when.heliosian.com/e/abc), with [Sam](https://who.heliosian.com/people/sam)." {
		t.Fatalf("expanded %q", got)
	}
}

func TestExpanderHoldsSplitKeys(t *testing.T) {
	l := newLinks()
	const sam = "https://who.heliosian.com/people/sam"
	l.shorten(sam)
	key := keyFor(t, l, sam)
	out := &strings.Builder{}
	events := []string{}
	cards := []string{}
	e := &expander{links: l, emit: func(kind string, data any) {
		events = append(events, kind)
		switch kind {
		case "text":
			out.WriteString(data.(string))
		case "card":
			cards = append(cards, data.(linkCard).Name)
		}
	}, cards: func(address string) (linkCard, bool) {
		return linkCard{URL: address, Kind: "person", Name: "Sam Lee"}, strings.Contains(address, "/people/")
	}, sent: map[string]bool{}}
	for _, piece := range []string{"Ask [Sam", " Lee]", "(" + key[:1], key[1:], ") today [or] not", "]"} {
		e.send("text", piece)
	}
	e.send("tool", "Searching")
	e.send("text", "done")
	e.flush()
	if out.String() != "Ask [Sam Lee]("+sam+") today [or] not]done" {
		t.Fatalf("streamed %q", out.String())
	}
	if events[len(events)-3] != "text" || events[len(events)-2] != "tool" {
		t.Fatalf("held text did not go out before the tool: %v", events)
	}
	at := slices.Index(events, "card")
	if len(cards) != 1 || at < 0 || events[at+1] != "text" {
		t.Fatalf("the card did not go out once, before its link: %v %v", cards, events)
	}
}
