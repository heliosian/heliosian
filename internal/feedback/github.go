package feedback

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Repo is where a kept report becomes an issue: the primary repository, in
// the open, which is why nothing personal may reach it (Strip).
const Repo = "heliosian/heliosian"

// jwtLife is how long the app's own assertion is good for. GitHub refuses one
// older than ten minutes and allows for a minute of clock drift, so the
// backdated issue time keeps a slightly fast clock from being refused.
const (
	jwtLife  = 9 * time.Minute
	jwtDrift = 30 * time.Second
	// tokenMargin renews an installation token before its hour is out rather
	// than discovering it has expired mid-filing.
	tokenMargin = 5 * time.Minute
)

// GitHubApp files issues as the Heliosian GitHub App rather than as a person:
// the issue's author is the app's bot account, so an issue built out of
// somebody's report is plainly the robot's doing and not an admin's own words.
// It holds the app's id and signing key, and trades them for an installation
// token as needed.
type GitHubApp struct {
	ID         string
	PrivateKey *rsa.PrivateKey
	Endpoint   string

	mu           sync.Mutex
	installation int64
	token        string
	expires      time.Time
}

// NewGitHubApp reads the PEM signing key GitHub issued with the app. An empty
// id or key means the app is not set up, and nil is what callers treat as
// "filing is unavailable".
func NewGitHubApp(id, pemKey string) (*GitHubApp, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(pemKey) == "" {
		return nil, nil
	}
	key, err := parseKey(pemKey)
	if err != nil {
		return nil, err
	}
	return &GitHubApp{ID: strings.TrimSpace(id), PrivateKey: key}, nil
}

// parseKey takes the PKCS#1 key GitHub hands out, and a PKCS#8 one besides,
// since a key round-tripped through another tool often comes back as that.
func parseKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, fmt.Errorf("feedback: the github app key is not PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("feedback: parse the github app key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("feedback: the github app key is %T, not RSA", parsed)
	}
	return key, nil
}

func (g *GitHubApp) endpoint() string {
	if g.Endpoint != "" {
		return g.Endpoint
	}
	return "https://api.github.com"
}

// assertion is the app's own JWT, which buys an installation token and nothing
// else.
func (g *GitHubApp) assertion(now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	claims := map[string]any{
		"iat": now.Add(-jwtDrift).Unix(),
		"exp": now.Add(jwtLife).Unix(),
		"iss": g.ID,
	}
	part := func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	head, err := part(header)
	if err != nil {
		return "", err
	}
	body, err := part(claims)
	if err != nil {
		return "", err
	}
	signing := head + "." + body
	sum := sha256.Sum256([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, g.PrivateKey, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (g *GitHubApp) call(ctx context.Context, method, path, auth string, payload any, want int, into any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.endpoint()+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+auth)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("feedback: github %s %s: %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(reply)))
	}
	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// accessToken is the installation's token, minted on first use and again once
// the one in hand is nearly out. The installation is looked up from the
// repository itself, so nothing has to carry its id.
func (g *GitHubApp) accessToken(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	if g.token != "" && now.Before(g.expires.Add(-tokenMargin)) {
		return g.token, nil
	}
	assertion, err := g.assertion(now)
	if err != nil {
		return "", err
	}
	if g.installation == 0 {
		var found struct {
			ID int64 `json:"id"`
		}
		if err := g.call(ctx, http.MethodGet, "/repos/"+Repo+"/installation", assertion, nil, http.StatusOK, &found); err != nil {
			return "", err
		}
		g.installation = found.ID
		slog.Info("feedback: github app installation found", "installation", found.ID, "repo", Repo)
	}
	var minted struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	path := fmt.Sprintf("/app/installations/%d/access_tokens", g.installation)
	if err := g.call(ctx, http.MethodPost, path, assertion, nil, http.StatusCreated, &minted); err != nil {
		return "", err
	}
	g.token, g.expires = minted.Token, minted.Expires
	return g.token, nil
}

func (g *GitHubApp) File(ctx context.Context, title, body, issueType string, labels []string) (string, error) {
	token, err := g.accessToken(ctx)
	if err != nil {
		return "", err
	}
	var created struct {
		URL string `json:"html_url"`
	}
	payload := map[string]any{"title": title, "body": body, "type": issueType, "labels": labels}
	if err := g.call(ctx, http.MethodPost, "/repos/"+Repo+"/issues", token, payload, http.StatusCreated, &created); err != nil {
		return "", err
	}
	slog.Info("feedback: filed", "issue", created.URL, "title", title)
	return created.URL, nil
}
