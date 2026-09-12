// Command console loads one page in headless Chrome and prints what its
// JavaScript said: every console message and every uncaught exception, so a
// page that renders blank can say why without a human opening devtools.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func main() {
	url := flag.String("url", "https://who.local.heliosian.com:8080/people", "page to load")
	wait := flag.Duration("wait", 4*time.Second, "how long to listen after the page loads")
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
	if err := chromedp.Run(ctx, chromedp.Navigate(*url), chromedp.Sleep(*wait)); err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	if n == 0 {
		fmt.Println("(nothing logged)")
	}
}
