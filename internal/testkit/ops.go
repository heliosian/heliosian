package testkit

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	cloudbuild "google.golang.org/api/cloudbuild/v1"
	run "google.golang.org/api/run/v2"

	"heliosian/internal/feedback"
	"heliosian/internal/intercept"
	"heliosian/internal/ops"
)

func OpsDeps(t *testing.T, buckets map[string]ops.Measurable) ops.Deps {
	t.Helper()
	intercept.GoogleLogin(t.TempDir())
	intercept.Install(intercept.GitHubHost, intercept.GitHub())
	intercept.Install(intercept.CloudBuildHost, intercept.CloudBuild())
	intercept.Install(intercept.CloudRunHost, intercept.CloudRun())
	intercept.Install(intercept.ClaudeHost, intercept.Claude())
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	builds, err := cloudbuild.NewService(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := run.NewService(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ops.Deps{GitHub: &feedback.GitHubApp{ID: "test", PrivateKey: key}, Builds: builds, Run: runs, AdminKey: "test", Buckets: buckets}
}
