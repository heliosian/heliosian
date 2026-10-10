package qclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	authServer = "https://mcp.heliosian.com"
	clientName = "Helios tools"
	signInWait = 5 * time.Minute
)

type stored struct {
	Token   string    `json:"access_token"`
	Expires time.Time `json:"expires"`
}

func tokenPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "heliosian", "token.json"), nil
}

func SignedIn(mode, spoof string) (*Client, error) {
	c := &Client{Base: Production, Mode: mode, Spoof: spoof}
	path, err := tokenPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, c.signIn()
	}
	if err != nil {
		return nil, err
	}
	var s stored
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if time.Now().After(s.Expires) {
		return c, c.signIn()
	}
	c.Token = s.Token
	return c, nil
}

func (c *Client) signIn() error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	redirect := fmt.Sprintf("http://%s/callback", listener.Addr())
	clientID, err := register(redirect)
	if err != nil {
		return err
	}
	verifier, state := rand.Text()+rand.Text(), rand.Text()
	sum := sha256.Sum256([]byte(verifier))
	authorize := authServer + "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirect},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(sum[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}.Encode()
	codes := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/callback" || q.Get("state") != state || q.Get("code") == "" {
			http.Error(w, "not the sign-in this tool started", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Signed in. Go back to the terminal.")
		select {
		case codes <- q.Get("code"):
		default:
		}
	})}
	go server.Serve(listener)
	defer server.Shutdown(context.Background())
	slog.Info("signing in through the browser", "url", authorize)
	if err := exec.Command("open", authorize).Run(); err != nil {
		slog.Warn("open the address above by hand", "error", err)
	}
	var code string
	select {
	case code = <-codes:
	case <-time.After(signInWait):
		return fmt.Errorf("no sign-in within %s", signInWait)
	}
	return c.exchange(clientID, redirect, code, verifier)
}

func register(redirect string) (string, error) {
	body, err := json.Marshal(map[string]any{"client_name": clientName, "redirect_uris": []string{redirect}})
	if err != nil {
		return "", err
	}
	resp, err := http.Post(authServer+"/oauth/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := decode(resp, http.StatusCreated, &out); err != nil {
		return "", fmt.Errorf("register: %w", err)
	}
	return out.ClientID, nil
}

func (c *Client) exchange(clientID, redirect, code, verifier string) error {
	resp, err := http.PostForm(authServer+"/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {redirect},
		"code_verifier": {verifier},
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Token   string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	if err := decode(resp, http.StatusOK, &out); err != nil {
		return fmt.Errorf("token: %w", err)
	}
	c.Token = out.Token
	path, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(stored{Token: out.Token, Expires: time.Now().Add(time.Duration(out.Expires) * time.Second)})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func decode(resp *http.Response, want int, out any) error {
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != want {
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return json.Unmarshal(raw, out)
}
