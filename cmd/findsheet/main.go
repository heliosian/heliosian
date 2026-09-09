// Command findsheet prints the spreadsheet id environment as shell exports.
package main

import (
	"context"
	"fmt"
	"log"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var variables = map[string]string{
	"Directory":           "DIRECTORY_SHEET",
	"Preferences":         "PREFERENCES_SHEET",
	"Invite List Builder": "INVITES_SHEET",
	"Apps":                "APPS_SHEET",
}

func main() {
	svc, err := drive.NewService(context.Background(),
		option.WithScopes(drive.DriveReadonlyScope))
	if err != nil {
		log.Fatalf("[ERROR] create drive client: %v", err)
	}
	resp, err := svc.Files.List().
		Q("mimeType = 'application/vnd.google-apps.spreadsheet'").
		Corpora("allDrives").
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("files(id, name)").
		Do()
	if err != nil {
		log.Fatalf("[ERROR] list spreadsheets: %v", err)
	}
	found := map[string]string{}
	for _, f := range resp.Files {
		variable, ok := variables[f.Name]
		if !ok {
			fmt.Printf("# %s  %s\n", f.Id, f.Name)
			continue
		}
		if previous, dup := found[variable]; dup {
			log.Fatalf("[ERROR] two spreadsheets named %q: %s and %s", f.Name, previous, f.Id)
		}
		found[variable] = f.Id
	}
	for _, variable := range []string{"DIRECTORY_SHEET", "PREFERENCES_SHEET", "INVITES_SHEET", "APPS_SHEET"} {
		id, ok := found[variable]
		if !ok {
			log.Fatalf("[ERROR] no spreadsheet found for %s", variable)
		}
		fmt.Printf("export %s=%s\n", variable, id)
	}
}
