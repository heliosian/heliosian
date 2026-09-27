package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/secretmanager/v1"

	"heliosian/internal/env"
	"heliosian/internal/spreadsheets"
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
	variables := map[string]string{}
	for _, s := range spreadsheets.All {
		variables[s.Title] = s.Env
	}
	svc, err := drive.NewService(ctx, option.WithScopes(drive.DriveReadonlyScope))
	if err != nil {
		log.Fatalf("create drive client: %v", err)
	}
	resp, err := svc.Files.List().
		Q("mimeType = 'application/vnd.google-apps.spreadsheet'").
		Corpora("allDrives").
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("files(id, name)").
		Do()
	if err != nil {
		log.Fatalf("list spreadsheets: %v", err)
	}
	found := map[string]string{}
	for _, f := range resp.Files {
		variable, ok := variables[f.Name]
		if !ok {
			fmt.Printf("# %s  %s\n", f.Id, f.Name)
			continue
		}
		if previous, dup := found[variable]; dup {
			log.Fatalf("two spreadsheets named %q: %s and %s", f.Name, previous, f.Id)
		}
		found[variable] = f.Id
	}
	for _, s := range spreadsheets.All {
		id, ok := found[s.Env]
		if !ok {
			log.Fatalf("no spreadsheet found for %s", s.Env)
		}
		export(s.Env, id)
	}
}
