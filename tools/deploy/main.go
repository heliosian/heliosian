package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"

	"heliosian/internal/app"
	"heliosian/internal/env"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/spreadsheets/lookup"
)

const (
	service  = "heliosian"
	region   = "us-west1"
	image    = "us-west1-docker.pkg.dev/heliosian/heliosian/heliosian:latest"
	identity = "directory@heliosian.iam.gserviceaccount.com"
)

func sheetEnvVars(sheets []spreadsheets.Spreadsheet, ids map[string]string) string {
	pairs := []string{}
	for _, s := range sheets {
		pairs = append(pairs, s.Env+"="+ids[s.Env])
	}
	return strings.Join(pairs, ",")
}

func secretRefs(secrets []env.Secret) string {
	pairs := []string{}
	for _, s := range secrets {
		pairs = append(pairs, s.Env+"="+s.Name+":latest")
	}
	return strings.Join(pairs, ",")
}

func gcloud(args ...string) {
	cmd := exec.Command("gcloud", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("gcloud %s: %v", args[0], err)
	}
}

func gcloudLines(args ...string) []string {
	cmd := exec.Command("gcloud", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("gcloud %s: %v", args[0], err)
	}
	return strings.Fields(string(out))
}

func mapDomains() {
	have := gcloudLines("beta", "run", "domain-mappings", "list", "--region", region, "--format", "value(metadata.name)")
	for _, host := range app.Hostnames(app.Domain) {
		if slices.Contains(have, host) {
			continue
		}
		log.Printf("mapping %s to %s", host, service)
		gcloud("beta", "run", "domain-mappings", "create", "--service", service, "--domain", host, "--region", region, "--quiet")
	}
}

func main() {
	ids, _ := lookup.Find(context.Background())
	envVars := sheetEnvVars(spreadsheets.All, ids)
	log.Printf("deploying %s to %s in %s", image, service, region)
	gcloud("run", "deploy", service,
		"--image", image,
		"--region", region,
		"--service-account", identity,
		"--allow-unauthenticated",
		"--min-instances", "1",
		"--max-instances", "1",
		"--cpu", "2",
		"--memory", "4Gi",
		"--concurrency", "250",
		"--no-cpu-throttling",
		"--use-http2",
		"--set-env-vars", envVars,
		"--set-secrets", secretRefs(env.Secrets),
		"--quiet")
	mapDomains()
}
