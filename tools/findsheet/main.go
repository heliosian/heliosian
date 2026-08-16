// Command findsheet lists spreadsheets visible to the service account.
package main

import (
	"context"
	"fmt"
	"log"

	"heliosian/internal/data"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func main() {
	svc, err := drive.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(drive.DriveReadonlyScope))
	if err != nil {
		log.Fatalf("[ERROR] create drive client: %v", err)
	}
	resp, err := svc.Files.List().
		Q("mimeType = 'application/vnd.google-apps.spreadsheet'").
		Corpora("allDrives").
		IncludeItemsFromAllDrives(true).
		SupportsAllDrives(true).
		Fields("files(id, name, modifiedTime, driveId)").
		Do()
	if err != nil {
		log.Fatalf("[ERROR] list spreadsheets: %v", err)
	}
	for _, f := range resp.Files {
		fmt.Printf("%s  %s  (modified %s, drive %s)\n", f.Id, f.Name, f.ModifiedTime, f.DriveId)
	}
}
