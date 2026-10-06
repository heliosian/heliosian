package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"

	"heliosian/internal/capture"
	"heliosian/internal/db"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const (
	fetchTimeout = 5 * time.Minute
	bodyLimit    = 100 << 20
	googleSignIn = "accounts.google.com"
	signedInPage = "https://myaccount.google.com/"
	stillToFetch = `(from DOCUMENT (where (and (in relation "image" "linked") (= fetch "sign_in") (blank content))))`
	imageAccept  = "image/webp,image/png,image/jpeg,image/gif,*/*;q=0.8"
	linkAccept   = "application/pdf,text/html,image/webp,image/png,image/jpeg,image/gif,*/*;q=0.8"
	sendTries    = 4
	sendWait     = 5 * time.Second
)

type answer struct {
	Hash  string `json:"hash"`
	Fetch string `json:"fetch"`
	Why   string `json:"why"`
}

type signedIn struct {
	client    *http.Client
	userAgent string
}

func session(ctx context.Context) (signedIn, error) {
	var cookies []*network.Cookie
	var userAgent string
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		if cookies, err = storage.GetCookies().Do(ctx); err != nil {
			return err
		}
		_, _, _, userAgent, _, err = browser.GetVersion().Do(ctx)
		return err
	}))
	if err != nil {
		return signedIn{}, err
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return signedIn{}, err
	}
	for _, c := range cookies {
		u := &url.URL{Scheme: "https", Host: strings.TrimPrefix(c.Domain, "."), Path: c.Path}
		cookie := &http.Cookie{Name: c.Name, Value: c.Value, Path: c.Path, Secure: c.Secure, HttpOnly: c.HTTPOnly}
		if strings.HasPrefix(c.Domain, ".") {
			cookie.Domain = c.Domain
		}
		jar.SetCookies(u, []*http.Cookie{cookie})
	}
	slog.Info("copied the capture browser's cookies", "cookies", len(cookies))
	return signedIn{client: &http.Client{Jar: jar, Timeout: fetchTimeout}, userAgent: userAgent}, nil
}

func (s signedIn) get(address, accept string) ([]byte, int, string, error) {
	req, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", s.userAgent)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit+1))
	return body, resp.StatusCode, resp.Request.URL.String(), err
}

func host(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return u.Host
}

func sendAgain(c qclient.Client, id string, body []byte, stop string) (answer, error) {
	wait := sendWait
	for try := 1; ; try++ {
		got, err := send(c, id, body, stop)
		var status *qclient.StatusError
		if err == nil || try == sendTries || !errors.As(err, &status) || (status.Code != http.StatusTooManyRequests && status.Code < 500) {
			return got, err
		}
		slog.Warn("send again", "document", id, "try", try, "in", wait, "error", err)
		time.Sleep(wait)
		wait *= 3
	}
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
	signInGone := flag.Bool("sign-in-gone", false, "mark a document still asking for a sign-in as gone")
	flag.Parse()
	c := qclient.Client{Base: qclient.Production, Key: env.Required("IMPORT_KEY")}
	if _, err := capture.Start(); err != nil {
		logging.Fatal("start capture browser", "error", err)
	}
	tab, err := capture.NewTab()
	if err != nil {
		logging.Fatal("open tab", "error", err)
	}
	ctx, cancel := capture.Attach(tab)
	s, err := session(ctx)
	cancel()
	if err != nil {
		logging.Fatal("copy the capture browser's session", "error", err)
	}
	if _, _, final, err := s.get(signedInPage, linkAccept); err != nil || host(final) == googleSignIn {
		logging.Fatal("sign in to google in the capture browser, then run this again", "page", final, "error", err)
	}
	a, err := c.QueryText(stillToFetch)
	if err != nil {
		logging.Fatal("read the documents still to fetch", "error", err)
	}
	slog.Info("documents to fetch", "count", len(a.Result))
	counts := map[string]int{}
	for i, id := range a.Result {
		doc := a.Resources["DOCUMENT"][id]
		address, accept := doc["url"], imageAccept
		if doc["relation"] == "linked" {
			address, accept = db.ExportURL(address), linkAccept
		}
		body, status, final, err := s.get(address, accept)
		stop := ""
		switch {
		case db.Unreachable(err):
			stop = "gone"
		case err != nil:
			slog.Warn("failed", "document", id, "url", address, "error", err)
			counts["failed"]++
			continue
		case host(final) == googleSignIn || status == http.StatusUnauthorized || status == http.StatusForbidden:
			if *signInGone {
				stop = "gone"
				break
			}
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
		case len(body) > bodyLimit:
			stop = "refused"
		}
		got, err := sendAgain(c, id, body, stop)
		if err != nil {
			logging.Fatal("send", "document", id, "error", err)
		}
		outcome := got.Fetch
		if outcome == "" {
			outcome = "filled"
		}
		counts[outcome]++
		slog.Info("fetched", "n", i+1, "of", len(a.Result), "document", id, "outcome", outcome, "why", got.Why, "hash", got.Hash, "bytes", len(body), "type", http.DetectContentType(body))
	}
	slog.Info("done", "documents", len(a.Result), "filled", counts["filled"], "gone", counts["gone"], "refused", counts["refused"], "sign_in", counts["sign_in"], "failed", counts["failed"])
}
