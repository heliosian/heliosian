package ask

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLinksShortenAndExpand(t *testing.T) {
	l := newLinks()
	got := l.shorten(`{"link":"https://who.heliosian.com/people/sam","more":"see https://x.org/a?b=1&c=2. and [form](https://docs.google.com/d/abc) or https://who.heliosian.com/people/sam"}`)
	want := `{"link":"L1","more":"see L2. and [form](L3) or L1"}`
	if got != want {
		t.Fatalf("shorten gave %s", got)
	}
	if got := l.expand("[Sam](L1) and [the form](L3), [x](L9)"); got != "[Sam](https://who.heliosian.com/people/sam) and [the form](https://docs.google.com/d/abc), [x](L9)" {
		t.Fatalf("expand gave %s", got)
	}
	if got := l.expand(`[Sam](L1 "") and [the form](L3 'Sign up'), [the reminder](L1 — see below)`); got != "[Sam](https://who.heliosian.com/people/sam) and [the form](https://docs.google.com/d/abc), [the reminder](https://who.heliosian.com/people/sam)" {
		t.Fatalf("expand of links with words after the key gave %s", got)
	}
	if got := string(l.expandInput([]byte(`{"path":"L2","id":"L22"}`))); got != `{"path":"https://x.org/a?b=1`+"\\"+`u0026c=2","id":"L22"}` {
		t.Fatalf("expandInput gave %s", got)
	}
}

func TestExpanderHoldsATitledKey(t *testing.T) {
	l := newLinks()
	l.shorten("https://who.heliosian.com/families/donhowe")
	out := &strings.Builder{}
	e := &expander{links: l, emit: func(kind string, data any) {
		if kind == "text" {
			out.WriteString(data.(string))
		}
	}, cards: func(string) (linkCard, bool) { return linkCard{}, false }, sent: map[string]bool{}}
	for _, piece := range []string{"The [Donhowe Family](L", "1 ", `"`, `"`, ") and [the ", "same](L1", " — see", " below) live in Mountain View"} {
		e.send("text", piece)
		if strings.Contains(out.String(), "L1") {
			t.Fatalf("the key went out raw: %q", out.String())
		}
	}
	e.flush()
	if out.String() != "The [Donhowe Family](https://who.heliosian.com/families/donhowe) and [the same](https://who.heliosian.com/families/donhowe) live in Mountain View" {
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
	question := c.messages[0].Content[0].OfText.Text
	answer := c.messages[1].Content[0].OfText.Text
	if question != "Is L2 on?" || answer != "Yes: [the picnic](L2), with [Sam](L3)." {
		t.Fatalf("restored %q and %q", question, answer)
	}
	if got := c.links.expand(answer); got != "Yes: [the picnic](https://when.heliosian.com/e/abc), with [Sam](https://who.heliosian.com/people/sam)." {
		t.Fatalf("expanded %q", got)
	}
}

func TestExpanderHoldsSplitKeys(t *testing.T) {
	l := newLinks()
	l.shorten("https://who.heliosian.com/people/sam")
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
	for _, piece := range []string{"Ask [Sam", " Lee]", "(L", "1", ") today [or] not", "]"} {
		e.send("text", piece)
	}
	e.send("tool", "Searching")
	e.send("text", "done")
	e.flush()
	if out.String() != "Ask [Sam Lee](https://who.heliosian.com/people/sam) today [or] not]done" {
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
