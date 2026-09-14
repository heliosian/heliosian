package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

func main() {
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("ignore-certificate-errors", true))...)
	defer cancel()
	ctx, cancel2 := chromedp.NewContext(ctx)
	defer cancel2()
	ctx, cancel3 := context.WithTimeout(ctx, 90*time.Second)
	defer cancel3()
	base := "https://birthday.local.heliosian.com:60896"
	var s string
	var a, b []byte
	err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 900),
		chromedp.Navigate(base+"/newsletters"), chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(`localStorage.setItem('birthday.superEdit','1'); true`, &s),
		chromedp.Navigate(base+"/newsletters"), chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(`[...document.querySelectorAll('.page-actions .button')].map(b => b.textContent).join(', ') + ' | issues: ' + document.querySelectorAll('.issue').length`, &s))
	fmt.Println("head:", s, err)
	// clear the future, accepting the confirm
	chromedp.Run(ctx, chromedp.Evaluate(`window.confirm = () => true; 'ok'`, &s),
		chromedp.Click(`.page-actions .button-secondary:nth-of-type(1)`, chromedp.ByQuery), chromedp.Sleep(1500*time.Millisecond),
		chromedp.Evaluate(`[...document.querySelectorAll('.page-actions .button')].map(b => b.textContent).join(', ') + ' | issues: ' + document.querySelectorAll('.issue').length + ' | last: ' + [...document.querySelectorAll('.issue-title')].map(t => t.firstChild.textContent).pop()`, &s))
	fmt.Println("after clear:", s)
	// create Thursdays through the end of the year
	chromedp.Run(ctx, chromedp.Click(`.page-actions .button-secondary`, chromedp.ByQuery), chromedp.Sleep(400*time.Millisecond),
		chromedp.CaptureScreenshot(&a),
		chromedp.Evaluate(`document.querySelector('#modal select').value = '4'; document.querySelector('#modal select').dispatchEvent(new Event('change')); [...document.querySelectorAll('#modal input')].map(i => i.value).join(' / ')`, &s))
	fmt.Println("create form:", s)
	chromedp.Run(ctx, chromedp.Click(`#modal button[type=submit]`, chromedp.ByQuery), chromedp.Sleep(2500*time.Millisecond),
		chromedp.Evaluate(`'issues: ' + document.querySelectorAll('.issue').length + ' | first future: ' + (document.querySelector('.issue.is-next .issue-title') || {}).textContent + ' | last: ' + [...document.querySelectorAll('.issue-title')].map(t => t.firstChild.textContent).pop()`, &s),
		chromedp.CaptureScreenshot(&b))
	fmt.Println("after create:", s)
	os.WriteFile(os.Args[1], a, 0o644)
	os.WriteFile(os.Args[2], b, 0o644)
}
