package capture

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const (
	Port      = "9222"
	DevTools  = "http://localhost:" + Port
	startWait = 10 * time.Second
)

type Options struct {
	URL           string
	Wait          string
	Remote        bool
	Cookie        string
	Click         string
	Settle        time.Duration
	Width, Height int
}

func Start() (string, error) {
	profile, err := filepath.Abs("local/capture-profile")
	if err != nil {
		return "", err
	}
	if running() {
		return profile, nil
	}
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return "", err
	}
	cmd := exec.Command("open", "-na", "Google Chrome", "--args",
		"--user-data-dir="+profile,
		"--remote-debugging-port="+Port,
		"--no-first-run",
		"--no-default-browser-check")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("launch chrome: %w", err)
	}
	deadline := time.Now().Add(startWait)
	for !running() {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("capture browser did not come up on %s", DevTools)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return profile, nil
}

func running() bool {
	resp, err := http.Get(DevTools + "/json/version")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func Launch() (context.Context, context.CancelFunc) {
	return chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("ignore-certificate-errors", true))...)
}

func Attach(id string) (context.Context, context.CancelFunc) {
	ctx, cancelAllocator := chromedp.NewRemoteAllocator(context.Background(), DevTools)
	ctx, cancelTab := chromedp.NewContext(ctx, chromedp.WithTargetID(target.ID(id)))
	return ctx, func() {
		cancelTab()
		cancelAllocator()
	}
}

func NewTab() (string, error) {
	req, err := http.NewRequest(http.MethodPut, DevTools+"/json/new?url=about:blank", nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("capture browser not reachable on %s, run tools/capturebrowser first: %w", DevTools, err)
	}
	defer resp.Body.Close()
	t := struct {
		ID string `json:"id"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.ID == "" {
		return "", fmt.Errorf("capture browser did not create a tab")
	}
	return t.ID, nil
}

func Cookies(address, header string) ([]chromedp.Action, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", address, err)
	}
	actions := []chromedp.Action{}
	for _, pair := range strings.Split(header, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, fmt.Errorf("cookie must be name=value, got %q", pair)
		}
		actions = append(actions, network.SetCookie(name, value).WithDomain(u.Hostname()).WithPath("/"))
	}
	return actions, nil
}

func Clicks(selectors string) []chromedp.Action {
	actions := []chromedp.Action{}
	if selectors == "" {
		return actions
	}
	for _, sel := range strings.Split(selectors, "|") {
		sel = strings.TrimSpace(sel)
		actions = append(actions,
			chromedp.WaitVisible(sel, chromedp.ByQuery),
			chromedp.Click(sel, chromedp.ByQuery),
			chromedp.Sleep(500*time.Millisecond),
		)
	}
	return actions
}

func PNG(opts Options) ([]byte, error) {
	var ctx context.Context
	var cancel context.CancelFunc
	if opts.Remote {
		id, err := NewTab()
		if err != nil {
			return nil, err
		}
		ctx, cancel = Attach(id)
	} else {
		var cancelAllocator context.CancelFunc
		ctx, cancelAllocator = Launch()
		defer cancelAllocator()
		ctx, cancel = chromedp.NewContext(ctx)
	}
	defer cancel()
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()
	cookies, err := Cookies(opts.URL, opts.Cookie)
	if err != nil {
		return nil, err
	}
	actions := append([]chromedp.Action{chromedp.EmulateViewport(int64(opts.Width), int64(opts.Height))}, cookies...)
	actions = append(actions,
		chromedp.Navigate(opts.URL),
		chromedp.WaitVisible(opts.Wait, chromedp.ByQuery),
	)
	actions = append(actions, Clicks(opts.Click)...)
	if opts.Settle > 0 {
		actions = append(actions, chromedp.Sleep(opts.Settle))
	}
	var png []byte
	actions = append(actions, chromedp.FullScreenshot(&png, 100))
	if err := chromedp.Run(ctx, actions...); err != nil {
		return nil, fmt.Errorf("capture %s: %w", opts.URL, err)
	}
	return png, nil
}
