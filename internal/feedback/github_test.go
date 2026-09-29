package feedback

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/intercept"
)

func appServer(t *testing.T, issue string) (*map[string]any, *string) {
	t.Helper()
	var created map[string]any
	var issueAuth string
	intercept.Install(GitHubHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/installation":
			io.WriteString(w, `{"id":4242}`)
		case "/app/installations/4242/access_tokens":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
				t.Errorf("token minted without a JWT: %q", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"token":"ghs_installation","expires_at":"2099-01-01T00:00:00Z"}`)
		case "/repos/" + Repo + "/issues":
			issueAuth = r.Header.Get("Authorization")
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &created); err != nil {
				t.Errorf("payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"`+issue+`"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	return &created, &issueAuth
}

func testApp(t *testing.T) *GitHubApp {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &GitHubApp{ID: "12345", PrivateKey: key}
}

func TestGitHubAppFiles(t *testing.T) {
	created, issueAuth := appServer(t, "https://github.com/heliosian/heliosian/issues/9")
	g := testApp(t)
	issue, err := g.File(context.Background(), "A title", "A body", "Bug", []string{"app:calendar"})
	if err != nil {
		t.Fatal(err)
	}
	if issue != "https://github.com/heliosian/heliosian/issues/9" {
		t.Errorf("issue = %q", issue)
	}
	if *issueAuth != "Bearer ghs_installation" {
		t.Errorf("the issue was filed as %q, not the installation", *issueAuth)
	}
	if (*created)["title"] != "A title" {
		t.Errorf("title = %v", (*created)["title"])
	}
	if (*created)["type"] != "Bug" {
		t.Errorf("type = %v", (*created)["type"])
	}
}

func TestGitHubAppReusesItsToken(t *testing.T) {
	mints := 0
	intercept.Install(GitHubHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/installation":
			io.WriteString(w, `{"id":7}`)
		case "/app/installations/7/access_tokens":
			mints++
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"token":"t","expires_at":"2099-01-01T00:00:00Z"}`)
		default:
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"html_url":"https://github.com/x/y/issues/1"}`)
		}
	}))
	g := testApp(t)
	for range 3 {
		if _, err := g.File(context.Background(), "t", "b", "Bug", nil); err != nil {
			t.Fatal(err)
		}
	}
	if mints != 1 {
		t.Errorf("minted %d tokens for three issues", mints)
	}
}

func TestGitHubAppRefused(t *testing.T) {
	intercept.Install(GitHubHost, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
	}))
	g := testApp(t)
	_, err := g.File(context.Background(), "t", "b", "Bug", nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v", err)
	}
}

func TestGitHubAppNotSetUp(t *testing.T) {
	g, err := NewGitHubApp("", "")
	if err != nil || g != nil {
		t.Errorf("g = %v err = %v", g, err)
	}
}
