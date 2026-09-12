// Command console loads one page in headless Chrome and prints what its
// JavaScript said: every console message and every uncaught exception, so a
// page that renders blank can say why without a human opening devtools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func main() {
	url := flag.String("url", "https://who.local.heliosian.com:8080/people", "page to load")
	wait := flag.Duration("wait", 4*time.Second, "how long to listen after the page loads")
	click := flag.String("click", "", "css selector(s) to click once the page is up, several separated by |, so an error behind a button shows too")
	out := flag.String("out", "", "also save a viewport screenshot here after the clicks, as the page then stands")
	flag.Parse()
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("ignore-certificate-errors", true))...)
	defer cancel()
	ctx, cancelBrowser := chromedp.NewContext(ctx)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(ctx, 40*time.Second)
	defer cancelTimeout()
	n := 0
	chromedp.ListenTarget(ctx, func(ev any) {
		switch e := ev.(type) {
		case *runtime.EventConsoleAPICalled:
			n++
			fmt.Printf("console.%s:", e.Type)
			for _, arg := range e.Args {
				if arg.Value != nil {
					fmt.Printf(" %s", arg.Value)
				} else {
					fmt.Printf(" %s", arg.Description)
				}
			}
			fmt.Println()
		case *runtime.EventExceptionThrown:
			n++
			d := e.ExceptionDetails
			where := ""
			if d.URL != "" {
				where = fmt.Sprintf(" at %s:%d:%d", d.URL, d.LineNumber, d.ColumnNumber)
			}
			text := d.Text
			if d.Exception != nil && d.Exception.Description != "" {
				text = d.Exception.Description
			}
			fmt.Printf("exception%s: %s\n", where, text)
		}
	})
	// The same viewport cmd/screenshot captures at, so a click lands on the
	// desktop layout rather than the phone one.
	actions := []chromedp.Action{chromedp.EmulateViewport(1280, 800), chromedp.Navigate(*url), chromedp.Sleep(*wait / 2)}
	if *click != "" {
		for _, sel := range strings.Split(*click, "|") {
			sel = strings.TrimSpace(sel)
			actions = append(actions, chromedp.WaitVisible(sel, chromedp.ByQuery), chromedp.Click(sel, chromedp.ByQuery), chromedp.Sleep(500*time.Millisecond))
		}
	}
	actions = append(actions, chromedp.Sleep(*wait/2))
	var png []byte
	if *out != "" {
		actions = append(actions, chromedp.CaptureScreenshot(&png))
	}
	if err := chromedp.Run(ctx, actions...); err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	if *out != "" {
		if err := os.WriteFile(*out, png, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
	}
	if n == 0 {
		fmt.Println("(nothing logged)")
	}
}
