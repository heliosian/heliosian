package intercept

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	GitHubHost     = "api.github.com"
	CloudBuildHost = "cloudbuild.googleapis.com"
	CloudRunHost   = "run.googleapis.com"
	sampleRevision = "projects/heliosian/locations/us-west1/services/heliosian/revisions/heliosian-00001-dev"
)

var sampleCommits = []struct {
	sha, message string
	ago          time.Duration
	build        string
}{
	{"5a1e000000000000000000000000000000000003", "Sample commit still building", 3 * time.Minute, "WORKING"},
	{"5a1e000000000000000000000000000000000002", "Sample commit now serving", 40 * time.Minute, "SUCCESS"},
	{"5a1e000000000000000000000000000000000001", "Sample commit whose build failed", 2 * time.Hour, "FAILURE"},
	{"5a1e000000000000000000000000000000000000", "Sample commit deployed earlier", 5 * time.Hour, "SUCCESS"},
}

func sampleDigest(sha string) string {
	return "sha256:" + strings.Repeat(sha[len(sha)-1:], 64)
}

func GitHub() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/installation"):
			writeJSON(w, map[string]any{"id": 1})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"token":"intercept","expires_at":%q}`, now.Add(time.Hour).Format(time.RFC3339))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/commits"):
			out := []any{}
			for _, c := range sampleCommits {
				out = append(out, map[string]any{"sha": c.sha, "html_url": "https://github.com/heliosian/heliosian/commit/" + c.sha, "commit": map[string]any{"message": c.message, "author": map[string]any{"name": "Sample Author", "date": now.Add(-c.ago).Format(time.RFC3339)}}})
			}
			writeJSON(w, out)
		case r.Method == http.MethodGet && r.URL.Path == "/search/issues":
			writeJSON(w, map[string]any{"total_count": 2, "items": []any{
				map[string]any{"number": 2, "title": "Sample open issue", "html_url": "https://github.com/heliosian/heliosian/issues/2", "created_at": now.Add(-time.Hour).Format(time.RFC3339), "labels": []any{map[string]any{"name": "app:admin"}}},
				map[string]any{"number": 1, "title": "Another sample issue", "html_url": "https://github.com/heliosian/heliosian/issues/1", "created_at": now.Add(-48 * time.Hour).Format(time.RFC3339), "labels": []any{}},
			}})
		default:
			http.Error(w, "intercept has no answer for "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
}

func CloudBuild() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/builds") {
			http.Error(w, "intercept answers only a builds list", http.StatusNotFound)
			return
		}
		now := time.Now().UTC()
		builds := []any{}
		for i, c := range sampleCommits {
			created := now.Add(-c.ago + 10*time.Second)
			b := map[string]any{"id": fmt.Sprintf("sample-build-%d", i), "status": c.build, "createTime": created.Format(time.RFC3339), "startTime": created.Add(5 * time.Second).Format(time.RFC3339), "substitutions": map[string]string{"COMMIT_SHA": c.sha}, "logUrl": "https://console.cloud.google.com/cloud-build/builds"}
			if c.build != "WORKING" {
				b["finishTime"] = created.Add(165 * time.Second).Format(time.RFC3339)
			}
			if c.build == "SUCCESS" {
				b["results"] = map[string]any{"images": []any{map[string]any{"name": "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian", "digest": sampleDigest(c.sha)}}}
			}
			builds = append(builds, b)
		}
		writeJSON(w, map[string]any{"builds": builds})
	})
}

func CloudRun() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services/heliosian"):
			writeJSON(w, map[string]any{"latestReadyRevision": sampleRevision})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, sampleRevision):
			writeJSON(w, map[string]any{"name": sampleRevision, "createTime": time.Now().UTC().Add(-37 * time.Minute).Format(time.RFC3339), "containers": []any{map[string]any{"image": "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian@" + sampleDigest(sampleCommits[1].sha)}}})
		default:
			http.Error(w, "intercept has no answer for "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		}
	})
}

func costReport(w http.ResponseWriter) {
	now := time.Now().UTC()
	days := []any{}
	for d := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC); !d.After(now); d = d.AddDate(0, 0, 1) {
		days = append(days, map[string]any{"starting_at": d.Format(time.RFC3339), "ending_at": d.AddDate(0, 0, 1).Format(time.RFC3339), "results": []any{
			map[string]any{"amount": fmt.Sprintf("%d", 4000+d.Day()*150), "model": "claude-sonnet-5-5", "description": "Claude Sonnet 5.5 - Input Tokens"},
			map[string]any{"amount": fmt.Sprintf("%d", 300+d.Day()*20), "model": "claude-opus-5-5", "description": "Claude Opus 5.5 - Output Tokens"},
		}})
	}
	writeJSON(w, map[string]any{"data": days, "has_more": false, "next_page": nil})
}
