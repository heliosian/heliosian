// Command createsheet creates an empty spreadsheet in the community shared drive, beside the Directory sheet.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func main() {
	title := flag.String("title", "", "spreadsheet title, e.g. Apps")
	flag.Parse()
	if *title == "" {
		log.Fatal("[ERROR] --title is required")
	}
	directory := os.Getenv("DIRECTORY_SHEET")
	if directory == "" {
		log.Fatal("[ERROR] DIRECTORY_SHEET is required, to find the shared drive")
	}
	svc, err := drive.NewService(context.Background(), option.WithScopes(drive.DriveScope))
	if err != nil {
		log.Fatalf("[ERROR] create drive client: %v", err)
	}
	anchor, err := svc.Files.Get(directory).SupportsAllDrives(true).Fields("driveId, parents").Do()
	if err != nil {
		log.Fatalf("[ERROR] look up the directory sheet: %v", err)
	}
	if anchor.DriveId == "" || len(anchor.Parents) == 0 {
		log.Fatal("[ERROR] the directory sheet is not in a shared drive")
	}
	existing, err := svc.Files.List().
		Q(fmt.Sprintf("name = '%s' and mimeType = 'application/vnd.google-apps.spreadsheet' and trashed = false", *title)).
		Corpora("drive").DriveId(anchor.DriveId).IncludeItemsFromAllDrives(true).SupportsAllDrives(true).
		Fields("files(id, name)").Do()
	if err != nil {
		log.Fatalf("[ERROR] list spreadsheets: %v", err)
	}
	if len(existing.Files) > 0 {
		log.Fatalf("[ERROR] a spreadsheet named %q already exists: %s", *title, existing.Files[0].Id)
	}
	created, err := svc.Files.Create(&drive.File{
		Name:     *title,
		MimeType: "application/vnd.google-apps.spreadsheet",
		Parents:  anchor.Parents,
	}).SupportsAllDrives(true).Fields("id").Do()
	if err != nil {
		log.Fatalf("[ERROR] create spreadsheet: %v", err)
	}
	log.Printf("created %q beside the directory sheet", *title)
	fmt.Printf("export %s_SHEET=%s\n", strings.ToUpper(*title), created.Id)
}
