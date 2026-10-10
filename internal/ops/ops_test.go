package ops_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/ops"
	"heliosian/internal/testkit"
	"heliosian/internal/vitals"
)

func settled(t *testing.T, b *ops.Board, done func(ops.Snapshot) bool) ops.Snapshot {
	t.Helper()
	changed, stop := vitals.Watch()
	defer stop()
	deadline := time.After(5 * time.Second)
	for {
		s := b.Snapshot()
		if done(s) {
			return s
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("never settled: %+v", s)
		}
	}
}

func TestTheBoardReadsEverySource(t *testing.T) {
	bucket := blob.NewMemoryBucket()
	if err := bucket.Put(context.Background(), "photos/a.jpg", "image/jpeg", []byte("123")); err != nil {
		t.Fatal(err)
	}
	b := ops.New(testkit.OpsDeps(t, map[string]ops.Measurable{"media": bucket}))
	b.Freshen()
	s := settled(t, b, func(s ops.Snapshot) bool {
		return !s.Commits.Fetched.IsZero() && !s.Builds.Fetched.IsZero() && !s.Serving.Fetched.IsZero() && !s.Issues.Fetched.IsZero() && !s.Spend.Fetched.IsZero() && !s.Buckets[0].Reading.Fetched.IsZero()
	})
	for name, err := range map[string]string{"commits": s.Commits.Error, "builds": s.Builds.Error, "serving": s.Serving.Error, "issues": s.Issues.Error, "spend": s.Spend.Error, "bucket": s.Buckets[0].Reading.Error} {
		if err != "" {
			t.Errorf("%s: %s", name, err)
		}
	}
	if len(s.Commits.Value) != 4 || s.Commits.Value[0].Message != "Sample commit still building" {
		t.Errorf("commits: %+v", s.Commits.Value)
	}
	if len(s.Builds.Value) != 4 || s.Builds.Value[0].Status != "WORKING" || s.Builds.Value[0].SHA != s.Commits.Value[0].SHA {
		t.Errorf("builds: %+v", s.Builds.Value)
	}
	if s.Serving.Value.Revision != "heliosian-00001-dev" || s.Serving.Value.SHA != s.Commits.Value[1].SHA {
		t.Errorf("serving: %+v", s.Serving.Value)
	}
	if s.Issues.Value.Open != 2 || s.Issues.Value.Items[0].Labels[0] != "app:admin" {
		t.Errorf("issues: %+v", s.Issues.Value)
	}
	today := time.Now().UTC()
	last := s.Spend.Value.Days[len(s.Spend.Value.Days)-1]
	if len(s.Spend.Value.Days) != today.Day() || last.ByModel["claude-sonnet-5-5"] != float64(4000+today.Day()*150)/100 {
		t.Errorf("spend: %+v", s.Spend.Value)
	}
	if s.Buckets[0].Reading.Value["photos"] != (blob.Usage{Objects: 1, Bytes: 3}) {
		t.Errorf("bucket: %+v", s.Buckets[0].Reading)
	}
}

func TestTheGitHubHookNeedsItsSignature(t *testing.T) {
	b := ops.New(testkit.OpsDeps(t, map[string]ops.Measurable{}))
	mux := http.NewServeMux()
	b.Register(mux, ops.Hooks{GitHubSecret: []byte("secret"), Audience: "https://admin.example.org/hooks/build", PushAccount: "push@example.org"})
	body := `{"ref":"refs/heads/main"}`
	post := func(path, signature, authorization string) int {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("X-GitHub-Event", "push")
		r.Header.Set("X-Hub-Signature-256", signature)
		r.Header.Set("Authorization", authorization)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		return rec.Code
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write([]byte(body))
	if code := post(ops.GitHubHookPath, "sha256="+hex.EncodeToString(mac.Sum(nil)), ""); code != http.StatusNoContent {
		t.Errorf("a signed push: %d", code)
	}
	if code := post(ops.GitHubHookPath, "sha256=00", ""); code != http.StatusUnauthorized {
		t.Errorf("a badly signed push: %d", code)
	}
	if code := post(ops.BuildHookPath, "", ""); code != http.StatusUnauthorized {
		t.Errorf("a build push with no token: %d", code)
	}
	if code := post(ops.BuildHookPath, "", "Bearer not-a-token"); code != http.StatusUnauthorized {
		t.Errorf("a build push with a bad token: %d", code)
	}
}
