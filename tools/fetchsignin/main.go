package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"

	"heliosian/internal/capture"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const (
	pageTimeout  = time.Minute
	googleSignIn = "accounts.google.com"
	signedInPage = "https://myaccount.google.com/"
	stillToFetch = `(from DOCUMENT (where (and (= relation "linked") (= fetch "sign_in") (blank content))))`
)

type page struct {
	mu     sync.Mutex
	id     network.RequestID
	status int64
	url    string
}

type answer struct {
	Hash  string `json:"hash"`
	Fetch string `json:"fetch"`
	Why   string `json:"why"`
}

func (p *page) listen(ev any) {
	e, ok := ev.(*network.EventResponseReceived)
	if !ok || e.Type != network.ResourceTypeDocument {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.id == "" {
		p.id, p.status, p.url = e.RequestID, e.Response.Status, e.Response.URL
	}
}

func (p *page) load(ctx context.Context, address string) ([]byte, int64, string, error) {
	p.mu.Lock()
	p.id = ""
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(address)); err != nil {
		return nil, 0, "", err
	}
	p.mu.Lock()
	id, status, final := p.id, p.status, p.url
	p.mu.Unlock()
	if id == "" {
		return nil, 0, "", errors.New("the page answered no document")
	}
	var body []byte
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		body, err = network.GetResponseBody(id).Do(ctx)
		return err
	}))
	return body, status, final, err
}

func host(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return u.Host
}

func send(c qclient.Client, id string, body []byte, stop string) (answer, error) {
	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)
	if err := form.WriteField("document", id); err != nil {
		return answer{}, err
	}
	if stop != "" {
		if err := form.WriteField("stop", stop); err != nil {
			return answer{}, err
		}
	} else {
		part, err := form.CreateFormFile("body", "body")
		if err != nil {
			return answer{}, err
		}
		if _, err := part.Write(body); err != nil {
			return answer{}, err
		}
	}
	if err := form.Close(); err != nil {
		return answer{}, err
	}
	var out answer
	return out, c.Send(http.MethodPost, "/api/do/fetched", form.FormDataContentType(), buf, &out)
}

func main() {
	c := qclient.Client{Base: qclient.Production, Key: env.Required("IMPORT_KEY")}
	if _, err := capture.Start(); err != nil {
		logging.Fatal("start capture browser", "error", err)
	}
	tab, err := capture.NewTab()
	if err != nil {
		logging.Fatal("open tab", "error", err)
	}
	ctx, cancel := capture.Attach(tab)
	defer cancel()
	p := &page{}
	chromedp.ListenTarget(ctx, p.listen)
	if err := chromedp.Run(ctx, network.Enable()); err != nil {
		logging.Fatal("watch the network", "error", err)
	}
	if _, _, final, err := p.load(ctx, signedInPage); err != nil || host(final) == googleSignIn {
		logging.Fatal("sign in to google in the capture browser, then run this again", "page", final, "error", err)
	}
	a, err := c.QueryText(stillToFetch)
	if err != nil {
		logging.Fatal("read the documents still to fetch", "error", err)
	}
	slog.Info("documents to fetch", "count", len(a.Result))
	counts := map[string]int{}
	for i, id := range a.Result {
		address := a.Resources["DOCUMENT"][id]["url"]
		body, status, final, err := p.load(ctx, address)
		stop := ""
		switch {
		case err != nil:
			slog.Warn("failed", "document", id, "url", address, "error", err)
			counts["failed"]++
			continue
		case host(final) == googleSignIn || status == http.StatusUnauthorized || status == http.StatusForbidden:
			slog.Warn("still needs a sign-in", "document", id, "url", address, "status", status, "page", final)
			counts["sign_in"]++
			continue
		case status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500:
			slog.Warn("failed", "document", id, "url", address, "status", status)
			counts["failed"]++
			continue
		case status >= 400:
			stop = "gone"
		case status != http.StatusOK:
			slog.Warn("unexpected answer", "document", id, "url", address, "status", status)
			counts["failed"]++
			continue
		}
		got, err := send(c, id, body, stop)
		if err != nil {
			logging.Fatal("send", "document", id, "error", err)
		}
		outcome := got.Fetch
		if outcome == "" {
			outcome = "filled"
		}
		counts[outcome]++
		slog.Info("fetched", "n", i+1, "of", len(a.Result), "document", id, "outcome", outcome, "why", got.Why, "hash", got.Hash)
	}
	slog.Info("done", "documents", len(a.Result), "filled", counts["filled"], "gone", counts["gone"], "refused", counts["refused"], "sign_in", counts["sign_in"], "failed", counts["failed"])
}
