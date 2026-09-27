package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"heliosian/internal/app"
)

func main() {
	variables := map[string]string{}
	for _, s := range app.Spreadsheets {
		variables[s.Title] = s.Env
	}
	svc, err := drive.NewService(context.Background(),
		option.WithScopes(drive.DriveReadonlyScope))
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
	for _, s := range app.Spreadsheets {
		id, ok := found[s.Env]
		if !ok {
			log.Fatalf("no spreadsheet found for %s", s.Env)
		}
		fmt.Printf("export %s=%s\n", s.Env, id)
	}
}
