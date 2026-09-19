// Command darkcheck loads pages in dark mode and reads the rendered page for
// what dark.css missed: words with too little contrast against the ground
// they sit on, light surfaces a dark page was never meant to hold, and a
// tab strip painted a different colour from what it sits on. It walks every
// visible element, compositing each one's background up through its
// ancestors, so a pale fill painted by name shows up whatever stylesheet
// put it there. One url per -url, or several separated by commas; -click
// clicks its way into a window first, as cmd/screenshot does.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// The script runs in the page and answers with the findings. Colours are
// read as computed rgb(a); a background-image (a gradient, a picture) stops
// the walk with an unknown ground, which is reported only when asked.
const script = `(() => {
  // A computed colour is rgb(a); one mixed with color-mix comes back as color(srgb r g b / a), on 0..1.
  const parse = c => { let m = (c || '').match(/rgba?\(([^)]+)\)/); if (m) { const p = m[1].split(',').map(Number); return {r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1}; } m = (c || '').match(/color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+))?\)/); if (m) return {r: m[1] * 255, g: m[2] * 255, b: m[3] * 255, a: m[4] === undefined ? 1 : Number(m[4])}; return null; };
  const lum = c => { const f = v => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); }; return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b); };
  const ratio = (a, b) => { const la = lum(a), lb = lum(b); return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05); };
  const over = (top, under) => { const a = top.a + under.a * (1 - top.a); return {r: (top.r * top.a + under.r * under.a * (1 - top.a)) / a, g: (top.g * top.a + under.g * under.a * (1 - top.a)) / a, b: (top.b * top.a + under.b * under.a * (1 - top.a)) / a, a}; };
  const hex = c => '#' + [c.r, c.g, c.b].map(v => Math.round(v).toString(16).padStart(2, '0')).join('');
  // ground composites an element's own background over its ancestors' until one is opaque.
  const ground = el => {
    let acc = null;
    for (let e = el; e; e = e.parentElement) {
      const cs = getComputedStyle(e);
      const c = parse(cs.backgroundColor);
      if (c && c.a > 0) { acc = acc ? over(acc, c) : c; if (acc.a >= 0.995) return acc; }
      if (cs.backgroundImage && cs.backgroundImage !== 'none') return acc && acc.a > 0.9 ? acc : null;
    }
    const c = parse(getComputedStyle(document.documentElement).backgroundColor);
    return acc ? over(acc, c && c.a > 0 ? c : {r: 255, g: 255, b: 255, a: 1}) : (c && c.a > 0 ? c : {r: 255, g: 255, b: 255, a: 1});
  };
  const visible = el => { const cs = getComputedStyle(el); if (cs.display === 'none' || cs.visibility === 'hidden' || Number(cs.opacity) === 0) return false; const r = el.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  const path = el => { const bits = []; for (let e = el, n = 0; e && n < 4 && e !== document.body; e = e.parentElement, n++) { let s = e.tagName.toLowerCase(); if (e.id) s += '#' + e.id; else if (e.classList.length) s += '.' + [...e.classList].slice(0, 2).join('.'); bits.unshift(s); } return bits.join(' > '); };
  const out = {contrast: [], light: [], tabs: []};
  const seen = new Set();
  for (const el of document.body.querySelectorAll('*')) {
    if (['SCRIPT', 'STYLE', 'SVG', 'PATH', 'IMG', 'CANVAS', 'VIDEO', 'BR'].includes(el.tagName)) continue;
    if (el.closest('svg')) continue;
    if (!visible(el)) continue;
    const cs = getComputedStyle(el);
    const own = parse(cs.backgroundColor);
    // Text: elements with their own non-blank text nodes.
    let text = '';
    for (const n of el.childNodes) { if (n.nodeType === 3) text += n.textContent; }
    text = text.replace(/\s+/g, ' ').trim();
    if (text) {
      const fg = parse(cs.color);
      const bg = ground(el);
      if (fg && bg && fg.a > 0.2) {
        const eff = fg.a < 1 ? over(fg, bg) : fg;
        const r = ratio(eff, bg);
        const size = parseFloat(cs.fontSize);
        const big = size >= 24 || (size >= 18.66 && parseInt(cs.fontWeight) >= 700);
        const need = big ? 3 : 4.5;
        if (r < need && !(cs.textDecoration || '').includes('line-through') && !el.matches('[disabled], [aria-disabled=true], .is-off, .disabled, [disabled] *')) {
          out.contrast.push({ratio: Math.round(r * 10) / 10, need, text: text.slice(0, 50), fg: hex(eff), bg: hex(bg), path: path(el)});
        }
      }
    }
    // Light surfaces: an opaque fill lighter than mid-grey, of some size.
    if (own && own.a > 0.5) {
      const under = own.a < 1 ? ground(el.parentElement || el) : null; const bgc = own.a < 1 ? (under ? over(own, under) : null) : own;
      if (!bgc) continue;
      const rect = el.getBoundingClientRect();
      if (lum(bgc) > 0.4 && rect.width >= 24 && rect.height >= 16) {
        const key = path(el);
        if (!seen.has(key)) { seen.add(key); out.light.push({bg: hex(bgc), w: Math.round(rect.width), h: Math.round(rect.height), path: key, text: (el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 40)}); }
      }
    }
  }
  // Tab strips: a fill of their own that is not their parent's ground.
  for (const el of document.body.querySelectorAll('.tabs, .tab-strip, .tabbar, .form-tabs, [role=tablist], .segments, .view-switch, .mobile-tabs, .topnav, .subnav')) {
    if (!visible(el)) continue;
    const own = parse(getComputedStyle(el).backgroundColor);
    if (!own || own.a === 0) continue;
    const mine = ground(el), around = ground(el.parentElement);
    if (mine && around && ratio(mine, around) > 1.15) out.tabs.push({strip: hex(mine), around: hex(around), path: path(el)});
  }
  out.contrast.sort((a, b) => a.ratio - b.ratio);
  return out;
})()`

type finding struct {
	Ratio float64 `json:"ratio"`
	Need  float64 `json:"need"`
	Text  string  `json:"text"`
	FG    string  `json:"fg"`
	BG    string  `json:"bg"`
	Path  string  `json:"path"`
}

type light struct {
	BG   string `json:"bg"`
	W, H int
	Path string `json:"path"`
	Text string `json:"text"`
}

type tab struct {
	Strip  string `json:"strip"`
	Around string `json:"around"`
	Path   string `json:"path"`
}

type report struct {
	Contrast []finding `json:"contrast"`
	Light    []light   `json:"light"`
	Tabs     []tab     `json:"tabs"`
}

func main() {
	urls := flag.String("url", "", "page(s) to check, several separated by commas")
	cookie := flag.String("cookie", "heliosian-mode=dark", "name=value cookie(s) to set for each url's host, ; separated")
	click := flag.String("click", "", "css selector(s) to click after the page is up, | separated")
	wait := flag.String("wait", "body", "css selector that must be visible before reading")
	settle := flag.Duration("settle", 1500*time.Millisecond, "how long to wait after the load and clicks before reading")
	width := flag.Int("width", 1280, "viewport width")
	height := flag.Int("height", 1600, "viewport height, tall so lazy sections draw")
	showLight := flag.Bool("light", false, "also list every light opaque surface, for reading by eye")
	compare := flag.Bool("compare", false, "read each page by day too, and mark each finding dark only or by day too")
	minRatio := flag.Float64("min", 3, "report words whose contrast is under this, or under what their size needs if lower")
	flag.Parse()
	if *urls == "" {
		log.Fatal("[ERROR] -url is required")
	}
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("ignore-certificate-errors", true))...)
	defer cancel()
	failed := 0
	for _, u := range strings.Split(*urls, ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		rep, err := check(ctx, u, *cookie, *click, *wait, *settle, *width, *height)
		if err != nil {
			log.Printf("[ERROR] %s: %v", u, err)
			failed++
			continue
		}
		var day *report
		if *compare {
			day, err = check(ctx, u, "heliosian-mode=light", *click, *wait, *settle, *width, *height)
			if err != nil {
				log.Printf("[ERROR] %s by day: %v", u, err)
				failed++
				continue
			}
		}
		print(u, rep, day, *showLight, *minRatio)
	}
	if failed > 0 {
		log.Fatalf("[ERROR] %d page(s) could not be read", failed)
	}
}

// check reads one page in a tab of its own, which closes with it.
func check(ctx context.Context, u, cookie, click, wait string, settle time.Duration, width, height int) (*report, error) {
	ctx, cancelTab := chromedp.NewContext(ctx)
	defer cancelTab()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	parsed, err := url.Parse(u)
	if err != nil {
		return nil, err
	}
	actions := []chromedp.Action{chromedp.EmulateViewport(int64(width), int64(height))}
	for _, pair := range strings.Split(cookie, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("cookie must be name=value")
		}
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(name, value).WithDomain(parsed.Hostname()).WithPath("/").Do(ctx)
		}))
	}
	actions = append(actions, chromedp.Navigate(u), chromedp.WaitVisible(wait, chromedp.ByQuery))
	if click != "" {
		for _, sel := range strings.Split(click, "|") {
			sel = strings.TrimSpace(sel)
			actions = append(actions, chromedp.WaitVisible(sel, chromedp.ByQuery), chromedp.Click(sel, chromedp.ByQuery), chromedp.Sleep(500*time.Millisecond))
		}
	}
	var raw json.RawMessage
	actions = append(actions, chromedp.Sleep(settle), chromedp.Evaluate(script, &raw))
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, err
	}
	var rep report
	if err := json.Unmarshal(raw, &rep); err != nil {
		return nil, fmt.Errorf("read findings: %w", err)
	}
	return &rep, nil
}

// key names a finding by where it is and what colours it wears, so the
// same label on twenty cards is one line, and a light-mode run can say
// whether dark mode made it.
func (f finding) key() string { return f.Path + " " + f.FG + " " + f.BG }
func (l light) key() string   { return l.Path + " " + l.BG }
func (t tab) key() string     { return t.Path }

func print(u string, rep, day *report, showLight bool, minRatio float64) {
	fmt.Printf("== %s\n", u)
	n := 0
	inDay := map[string]bool{}
	if day != nil {
		for _, f := range day.Contrast {
			inDay[f.key()] = true
		}
		for _, t := range day.Tabs {
			inDay[t.key()] = true
		}
		for _, l := range day.Light {
			inDay[l.key()] = true
		}
	}
	mark := func(key string) string {
		if day == nil {
			return ""
		}
		if inDay[key] {
			return " [by day too]"
		}
		return " [dark only]"
	}
	counts := map[string]int{}
	var order []finding
	for _, f := range rep.Contrast {
		if f.Ratio >= minRatio && f.Ratio >= f.Need {
			continue
		}
		if counts[f.key()] == 0 {
			order = append(order, f)
		}
		counts[f.key()]++
	}
	for _, f := range order {
		n++
		fmt.Printf("  contrast %4.1f (needs %.1f) x%-3d %s on %s  %q  %s%s\n", f.Ratio, f.Need, counts[f.key()], f.FG, f.BG, f.Text, f.Path, mark(f.key()))
	}
	for _, t := range rep.Tabs {
		n++
		fmt.Printf("  tabs %s on %s  %s%s\n", t.Strip, t.Around, t.Path, mark(t.key()))
	}
	if showLight {
		sort.Slice(rep.Light, func(i, j int) bool { return rep.Light[i].W*rep.Light[i].H > rep.Light[j].W*rep.Light[j].H })
		for _, l := range rep.Light {
			n++
			fmt.Printf("  light %s %dx%d  %q  %s%s\n", l.BG, l.W, l.H, l.Text, l.Path, mark(l.key()))
		}
	}
	if n == 0 {
		fmt.Println("  (nothing found)")
	}
}
