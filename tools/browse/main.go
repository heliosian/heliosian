package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"

	"heliosian/internal/capture"
	"heliosian/internal/logging"
)

const scrollScript = `(() => {
	const candidates = [...document.querySelectorAll("*")].filter(
		el => el.scrollHeight > el.clientHeight + 1 &&
			/auto|scroll|overlay/.test(getComputedStyle(el).overflowY)
	);
	const room = el => el.scrollHeight - el.clientHeight;
	const target = candidates.reduce((a, b) => (room(b) > room(a) ? b : a), document.scrollingElement);
	target.scrollBy(0, %d);
	return {scrollTop: target.scrollTop, scrollHeight: target.scrollHeight, clientHeight: target.clientHeight};
})()`

type pageTarget struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func currentTarget() (string, error) {
	resp, err := http.Get(capture.DevTools + "/json/list")
	if err != nil {
		return "", fmt.Errorf("capture browser not reachable on %s, run tools/capturebrowser first: %w", capture.DevTools, err)
	}
	defer resp.Body.Close()
	targets := []pageTarget{}
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return "", err
	}
	for _, t := range targets {
		if t.Type != "page" {
			continue
		}
		if strings.HasPrefix(t.URL, "devtools://") || strings.HasPrefix(t.URL, "chrome-extension://") {
			continue
		}
		return t.ID, nil
	}
	return capture.NewTab()
}

func parseXY(coords string) (float64, float64, error) {
	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("click coordinates must be x,y, got %q", coords)
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, err
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

func keyChord(name string) string {
	switch strings.ToLower(name) {
	case "enter":
		return "\r"
	case "tab":
		return "\t"
	case "escape":
		return ""
	case "backspace":
		return "\b"
	default:
		return name
	}
}

func main() {
	nav := flag.String("nav", "", "navigate to url")
	back := flag.Bool("back", false, "navigate back in history")
	scroll := flag.Int("scroll", 0, "scroll vertically by pixels, negative scrolls up")
	clickSel := flag.String("clicksel", "", "click the first element matching css selector")
	click := flag.String("click", "", "click at viewport coordinates x,y as shown in the screenshot")
	typeText := flag.String("type", "", "insert text into the focused element")
	key := flag.String("key", "", "press a key: enter, tab, escape, backspace, or a literal character")
	wait := flag.String("wait", "", "css selector that must be visible before capturing")
	cookie := flag.String("cookie", "", "name=value cookie(s) to set on the --nav url's host before loading it, ; separated")
	mobile := flag.Bool("mobile", false, "emulate a phone viewport (390x844, touch) instead of desktop 1280x800")
	size := flag.String("size", "", "viewport size as WxH, overriding the desktop default")
	dump := flag.Bool("dump", false, "print page html instead of writing a screenshot")
	eval := flag.String("eval", "", "evaluate javascript in the page and print the json result instead of writing a screenshot")
	out := flag.String("out", "local/screenshots/browse.png", "output png path")
	flag.Parse()

	if *cookie != "" && *nav == "" {
		logging.Fatal("--cookie needs --nav, whose host the cookie is set on")
	}
	id, err := currentTarget()
	if err != nil {
		logging.Fatal("find tab", "error", err)
	}
	// cancelling the chromedp context closes the attached tab; the tab must outlive this process
	ctx, _ := capture.Attach(id)
	ctx, cancelTimeout := context.WithTimeout(ctx, 15*time.Second)
	defer cancelTimeout()

	viewport := chromedp.EmulateViewport(1280, 800)
	if *mobile {
		viewport = chromedp.EmulateViewport(390, 844, chromedp.EmulateMobile)
	}
	if *size != "" {
		w, h, ok := strings.Cut(*size, "x")
		width, werr := strconv.ParseInt(w, 10, 64)
		height, herr := strconv.ParseInt(h, 10, 64)
		if !ok || werr != nil || herr != nil {
			logging.Fatal("size must be WxH", "size", *size)
		}
		viewport = chromedp.EmulateViewport(width, height)
	}
	scrolled := struct {
		ScrollTop    float64 `json:"scrollTop"`
		ScrollHeight float64 `json:"scrollHeight"`
		ClientHeight float64 `json:"clientHeight"`
	}{}
	actions := []chromedp.Action{viewport}
	if *nav != "" {
		cookies, err := capture.Cookies(*nav, *cookie)
		if err != nil {
			logging.Fatal("cookies", "error", err)
		}
		actions = append(actions, cookies...)
		actions = append(actions, chromedp.Navigate(*nav))
	}
	if *back {
		actions = append(actions, chromedp.NavigateBack())
	}
	if *scroll != 0 {
		actions = append(actions, chromedp.Evaluate(fmt.Sprintf(scrollScript, *scroll), &scrolled))
	}
	if *clickSel != "" {
		actions = append(actions, chromedp.Click(*clickSel, chromedp.ByQuery))
	}
	if *click != "" {
		x, y, err := parseXY(*click)
		if err != nil {
			logging.Fatal("click coordinates", "error", err)
		}
		actions = append(actions, chromedp.MouseClickXY(x, y))
	}
	if *typeText != "" {
		actions = append(actions, input.InsertText(*typeText))
	}
	if *key != "" {
		actions = append(actions, chromedp.KeyEvent(keyChord(*key)))
	}
	if *wait != "" {
		actions = append(actions, chromedp.WaitVisible(*wait, chromedp.ByQuery))
	}
	actions = append(actions, chromedp.Sleep(700*time.Millisecond))
	var html string
	var png []byte
	var evalResult any
	switch {
	case *dump:
		actions = append(actions, chromedp.OuterHTML("html", &html, chromedp.ByQuery))
	case *eval != "":
		actions = append(actions, chromedp.Evaluate(*eval, &evalResult))
	default:
		actions = append(actions, chromedp.CaptureScreenshot(&png))
	}
	var location, title string
	actions = append(actions, chromedp.Location(&location), chromedp.Title(&title))
	if err := chromedp.Run(ctx, actions...); err != nil {
		logging.Fatal("browse", "error", err)
	}
	if *dump {
		fmt.Println(html)
	} else if *eval != "" {
		encoded, err := json.MarshalIndent(evalResult, "", "  ")
		if err != nil {
			logging.Fatal("encode eval result", "error", err)
		}
		fmt.Println(string(encoded))
	} else {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			logging.Fatal("create output dir", "error", err)
		}
		if err := os.WriteFile(*out, png, 0o644); err != nil {
			logging.Fatal("write screenshot", "path", *out, "error", err)
		}
	}
	fmt.Printf("url: %s\ntitle: %s\n", location, title)
	if *scroll != 0 {
		fmt.Printf("scroll: %.0f of %.0f, viewport %.0f\n", scrolled.ScrollTop, scrolled.ScrollHeight, scrolled.ClientHeight)
	}
}
