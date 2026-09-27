package lookup

import (
	"context"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"heliosian/internal/logging"
	"heliosian/internal/spreadsheets"
)

type Other struct {
	ID    string
	Title string
}

func Find(ctx context.Context) (map[string]string, []Other) {
	variables := map[string]string{}
	for _, s := range spreadsheets.All {
		variables[s.Title] = s.Env
	}
	svc, err := drive.NewService(ctx, option.WithScopes(drive.DriveReadonlyScope))
	if err != nil {
		logging.Fatal("create drive client", "error", err)
	}
	resp, err := svc.Files.List().
		Q("mimeType = 'application/vnd.google-apps.spreadsheet'").
		Corpora("allDrives").
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("files(id, name)").
		Context(ctx).
		Do()
	if err != nil {
		logging.Fatal("list spreadsheets", "error", err)
	}
	found := map[string]string{}
	others := []Other{}
	for _, f := range resp.Files {
		variable, ok := variables[f.Name]
		if !ok {
			others = append(others, Other{ID: f.Id, Title: f.Name})
			continue
		}
		if previous, dup := found[variable]; dup {
			logging.Fatal("two spreadsheets share a title", "title", f.Name, "first", previous, "second", f.Id)
		}
		found[variable] = f.Id
	}
	for _, s := range spreadsheets.All {
		if _, ok := found[s.Env]; !ok {
			logging.Fatal("no spreadsheet found", "env", s.Env, "title", s.Title)
		}
	}
	return found, others
}
