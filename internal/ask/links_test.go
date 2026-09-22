package ask

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

const (
	samURL  = "https://who.heliosian.com/people/sam"
	formURL = "https://docs.google.com/d/abc"
	xURL    = "https://x.org/a?b=1&c=2"
)

func TestLinksShortenAndExpand(t *testing.T) {
	l := newLinks()
	sam, form, x := keyOf(samURL), keyOf(formURL), keyOf(xURL)
	if len(sam) != 9 || sam == form || keyOf(samURL) != sam {
		t.Fatalf("keys %s %s", sam, form)
	}
	got := l.shorten(`{"link":"` + samURL + `","more":"see ` + xURL + `. and [form](` + formURL + `) or ` + samURL + `"}`)
	want := `{"link":"` + sam + `","more":"see ` + x + `. and [form](` + form + `) or ` + sam + `"}`
	if got != want {
		t.Fatalf("shorten gave %s", got)
	}
	if got := l.expand("[Sam](" + sam + ") and [the form](" + form + "), [x](L00000000)"); got != "[Sam]("+samURL+") and [the form]("+formURL+"), x" {
		t.Fatalf("expand gave %s", got)
	}
	if got := l.expand("[open](javascript:alert%281%29) or [send](https://elsewhere.example/?d=secret) or https://elsewhere.example/x. but " + xURL + "."); got != "open or send or . but "+xURL+"." {
		t.Fatalf("expand of made-up targets gave %s", got)
	}
	if got := l.expand(`[Sam](` + sam + ` "") and [the form](` + form + ` 'Sign up'), [the reminder](` + sam + ` — see below)`); got != "[Sam]("+samURL+") and [the form]("+formURL+"), [the reminder]("+samURL+")" {
		t.Fatalf("expand of links with words after the key gave %s", got)
	}
	if got := string(l.expandInput([]byte(`{"path":"` + x + `","id":"L00000000"}`))); got != `{"path":"https://x.org/a?b=1`+"\\"+`u0026c=2","id":"L00000000"}` {
		t.Fatalf("expandInput gave %s", got)
	}
}

func TestLinksExpandAllRoundTrips(t *testing.T) {
	l := newLinks()
	sam, x := keyOf(samURL), keyOf(xURL)
	l.shorten(samURL + " " + xURL)
	for _, text := range []string{
		"[Sam](" + sam + ") and " + x + ". " + sam + `"`,
		"the key " + sam + " at the end " + x,
		sam + "', " + x + "?!) " + sam + "..",
		"L00000000 is unknown",
	} {
		expanded := l.expandAll(text)
		if strings.Contains(expanded, sam) || strings.Contains(expanded, x) {
			t.Errorf("a key stayed in %q", expanded)
		}
		if back := l.shorten(expanded); back != text {
			t.Errorf("round trip of %q gave %q", text, back)
		}
	}
	for _, text := range []string{sam + "/people " + sam + ".foo", "x" + sam + " " + sam + "9"} {
		if got := l.expandAll(text); got != text {
			t.Errorf("a key followed by more of an address was expanded: %q", got)
		}
		if back := l.shorten(text); back != text {
			t.Errorf("shortening %q gave %q", text, back)
		}
	}
}

func TestMapStringsLeavesThinkingAlone(t *testing.T) {
	var generic any
	if err := json.Unmarshal([]byte(`[{"role":"assistant","content":[{"type":"thinking","thinking":"see `+samURL+`","signature":"sig"},{"type":"text","text":"see `+samURL+`"}]},{"n":1}]`), &generic); err != nil {
		t.Fatal(err)
	}
	l := newLinks()
	out, err := json.Marshal(mapStrings(generic, l.shorten))
	if err != nil {
		t.Fatal(err)
	}
	if want := `[{"content":[{"signature":"sig","thinking":"see ` + samURL + `","type":"thinking"},{"text":"see ` + keyOf(samURL) + `","type":"text"}],"role":"assistant"},{"n":1}]`; string(out) != want {
		t.Fatalf("mapped to %s", out)
	}
}

func TestExpanderHoldsATitledKey(t *testing.T) {
	l := newLinks()
	donhowe := "https://who.heliosian.com/families/donhowe"
	l.shorten(donhowe)
	key := keyOf(donhowe)
	out := &strings.Builder{}
	e := &expander{links: l, emit: func(kind string, data any) {
		if kind == "text" {
			out.WriteString(data.(string))
		}
	}, cards: func(string) (linkCard, bool) { return linkCard{}, false }, sent: map[string]bool{}}
	for _, piece := range []string{"The [Donhowe Family](" + key[:2], key[2:] + " ", `"`, `"`, ") and [the ", "same](" + key, " — see", " below) live in Mountain View"} {
		e.send("text", piece)
		if strings.Contains(out.String(), key) {
			t.Fatalf("the key went out raw: %q", out.String())
		}
	}
	e.flush()
	if out.String() != "The [Donhowe Family]("+donhowe+") and [the same]("+donhowe+") live in Mountain View" {
		t.Fatalf("streamed %q", out.String())
	}
}

func TestExpanderDropsSplitMadeUpLinks(t *testing.T) {
	l := newLinks()
	l.shorten(samURL)
	key := keyOf(samURL)
	out := &strings.Builder{}
	e := &expander{links: l, emit: func(kind string, data any) {
		if kind == "text" {
			out.WriteString(data.(string))
		}
	}, cards: func(string) (linkCard, bool) { return linkCard{}, false }, sent: map[string]bool{}}
	for _, piece := range []string{"Use [the ", "form](https://else", "where.example/?d=", "secret) or https://el", "sewhere.example/x", " or [Sam](" + key[:3], key[3:] + ") [sic] done"} {
		e.send("text", piece)
		if strings.Contains(out.String(), "elsewhere") || strings.Contains(out.String(), key) {
			t.Fatalf("a made-up target went out: %q", out.String())
		}
	}
	e.flush()
	if out.String() != "Use the form or  or [Sam]("+samURL+") [sic] done" {
		t.Fatalf("streamed %q", out.String())
	}
}

func TestContextReadsWithKeysAndWritesWithAddresses(t *testing.T) {
	l := newLinks()
	picnic := "https://when.heliosian.com/e/abc"
	raw := json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"Is ` + picnic + ` on?"}]},{"role":"assistant","content":[{"type":"text","text":"Yes: [the picnic](` + picnic + `), with [Sam](` + samURL + `)."}]}]`)
	history, err := readContext(raw, l)
	if err != nil {
		t.Fatal(err)
	}
	question := history[0].Content[0].OfText.Text
	answer := history[1].Content[0].OfText.Text
	if question != "Is "+keyOf(picnic)+" on?" || answer != "Yes: [the picnic]("+keyOf(picnic)+"), with [Sam]("+keyOf(samURL)+")." {
		t.Fatalf("read %q and %q", question, answer)
	}
	if asked(history) != 1 {
		t.Fatalf("%d asked", asked(history))
	}
	out := writeContext(history, l)
	var back any
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	var sent any
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(sent)
	got, _ := json.Marshal(back)
	if string(got) != string(want) {
		t.Fatalf("wrote %s", out)
	}
	if _, err := readContext(json.RawMessage(`{"not":"a list"}`), l); err == nil {
		t.Fatal("a context that is not a list was read")
	}
}

func TestExpanderHoldsSplitKeys(t *testing.T) {
	l := newLinks()
	l.shorten(samURL)
	key := keyOf(samURL)
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
	for _, piece := range []string{"Ask [Sam", " Lee]", "(" + key[:1], key[1:5], key[5:] + ") today [or] not", "]"} {
		e.send("text", piece)
	}
	e.send("tool", "Searching")
	e.send("text", "done")
	e.flush()
	if out.String() != "Ask [Sam Lee]("+samURL+") today [or] not]done" {
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
