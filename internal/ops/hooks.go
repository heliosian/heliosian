package ops

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"google.golang.org/api/idtoken"
)

const (
	GitHubHookPath = "/hooks/github"
	BuildHookPath  = "/hooks/build"
	hookLimit      = 1 << 20
)

type Hooks struct {
	GitHubSecret []byte
	Audience     string
	PushAccount  string
}

func (b *Board) Register(mux *http.ServeMux, h Hooks) {
	mux.HandleFunc("POST "+GitHubHookPath, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, hookLimit))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		mac := hmac.New(sha256.New, h.GitHubSecret)
		mac.Write(body)
		want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(want), []byte(r.Header.Get("X-Hub-Signature-256"))) {
			slog.WarnContext(r.Context(), "ops: github hook with a bad signature")
			http.Error(w, "bad signature", http.StatusUnauthorized)
			return
		}
		event := r.Header.Get("X-GitHub-Event")
		slog.InfoContext(r.Context(), "ops: github hook", "event", event, "delivery", r.Header.Get("X-GitHub-Delivery"))
		switch event {
		case "push":
			b.commits.Refresh()
		case "issues":
			b.issues.Refresh()
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST "+BuildHookPath, func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		payload, err := idtoken.Validate(r.Context(), token, h.Audience)
		if err != nil || payload.Claims["email"] != h.PushAccount || payload.Claims["email_verified"] != true {
			slog.WarnContext(r.Context(), "ops: build hook with a bad token", "error", err)
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		var push struct {
			Message struct {
				Attributes map[string]string `json:"attributes"`
			} `json:"message"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, hookLimit)).Decode(&push); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		status := push.Message.Attributes["status"]
		slog.InfoContext(r.Context(), "ops: build hook", "build", push.Message.Attributes["buildId"], "status", status)
		b.builds.Refresh()
		if status == "SUCCESS" {
			b.serving.Refresh()
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
