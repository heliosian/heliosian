package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/secretmanager/v1"

	"heliosian/internal/env"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/spreadsheets/lookup"
)

const (
	project         = "heliosian"
	localSessionKey = "local-verify-key"
)

func main() {
	ctx := context.Background()
	exportSheets(ctx)
	secrets, err := secretmanager.NewService(ctx)
	if err != nil {
		log.Fatalf("create secret manager client: %v", err)
	}
	for _, s := range env.Secrets {
		if s.Env == "SESSION_KEY" {
			export(s.Env, localSessionKey)
			continue
		}
		version, err := secrets.Projects.Secrets.Versions.Access("projects/" + project + "/secrets/" + s.Name + "/versions/latest").Context(ctx).Do()
		if err != nil {
			log.Fatalf("read secret %s: %v", s.Name, err)
		}
		value, err := base64.StdEncoding.DecodeString(version.Payload.Data)
		if err != nil {
			log.Fatalf("decode secret %s: %v", s.Name, err)
		}
		export(s.Env, string(value))
	}
}

func export(name, value string) {
	fmt.Printf("export %s='%s'\n", name, strings.ReplaceAll(value, "'", `'\''`))
}

func exportSheets(ctx context.Context) {
	ids, others := lookup.Find(ctx)
	for _, o := range others {
		fmt.Printf("# %s  %s\n", o.ID, o.Title)
	}
	for _, s := range spreadsheets.All {
		export(s.Env, ids[s.Env])
	}
}
