// Command browse drives the capture browser one step at a time: act, capture, report.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

type pageTarget struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

func currentTarget() (string, error) {
	resp, err := http.Get("http://localhost:9222/json/list")
	if err != nil {
		return "", fmt.Errorf("capture browser not reachable on localhost:9222, run tools/capturebrowser first: %w", err)
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
	return newTab()
}

func newTab() (string, error) {
	req, err := http.NewRequest(http.MethodPut, "http://localhost:9222/json/new?url=about:blank", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	t := pageTarget{}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.ID == "" {
		return "", fmt.Errorf("capture browser did not create a tab")
	}
	return t.ID, nil
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
	cookie := flag.String("cookie", "", "set a name=value cookie on localhost before acting")
	mobile := flag.Bool("mobile", false, "emulate a phone viewport (390x844, touch) instead of desktop 1280x800")
	size := flag.String("size", "", "viewport size as WxH, overriding the desktop default")
	dump := flag.Bool("dump", false, "print page html instead of writing a screenshot")
	eval := flag.String("eval", "", "evaluate javascript in the page and print the json result instead of writing a screenshot")
	out := flag.String("out", "screenshots/browse.png", "output png path")
	flag.Parse()

	id, err := currentTarget()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	allocCtx, _ := chromedp.NewRemoteAllocator(context.Background(), "http://localhost:9222")
	// cancelling the chromedp context closes the attached tab; the tab must outlive this process
	ctx, _ := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID(id)))
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
			log.Fatalf("[ERROR] size must be WxH, got %q", *size)
		}
		viewport = chromedp.EmulateViewport(width, height)
	}
	actions := []chromedp.Action{viewport}
	if *cookie != "" {
		name, value, ok := strings.Cut(*cookie, "=")
		if !ok {
			log.Fatalf("[ERROR] cookie must be name=value, got %q", *cookie)
		}
		actions = append(actions, network.SetCookie(name, value).WithDomain("localhost").WithPath("/"))
	}
	if *nav != "" {
		actions = append(actions, chromedp.Navigate(*nav))
	}
	if *back {
		actions = append(actions, chromedp.NavigateBack())
	}
	if *scroll != 0 {
		actions = append(actions, chromedp.Evaluate(fmt.Sprintf("window.scrollBy(0, %d)", *scroll), nil))
	}
	if *clickSel != "" {
		actions = append(actions, chromedp.Click(*clickSel, chromedp.ByQuery))
	}
	if *click != "" {
		x, y, err := parseXY(*click)
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
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
		log.Fatalf("[ERROR] browse: %v", err)
	}
	if *dump {
		fmt.Println(html)
	} else if *eval != "" {
		encoded, err := json.MarshalIndent(evalResult, "", "  ")
		if err != nil {
			log.Fatalf("[ERROR] encode eval result: %v", err)
		}
		fmt.Println(string(encoded))
	} else {
		if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
			log.Fatalf("[ERROR] create output dir: %v", err)
		}
		if err := os.WriteFile(*out, png, 0o644); err != nil {
			log.Fatalf("[ERROR] write %s: %v", *out, err)
		}
	}
	fmt.Printf("url: %s\ntitle: %s\n", location, title)
}
