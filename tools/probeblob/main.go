// Command probeblob times the download of a few drive media files.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func main() {
	svc, err := drive.NewService(context.Background(),
		option.WithScopes(drive.DriveScope))
	if err != nil {
		log.Fatalf("[ERROR] drive client: %v", err)
	}
	list, err := svc.Files.List().
		Q("mimeType != 'application/vnd.google-apps.folder' and trashed = false").
		SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
		Fields("files(id, name, size)").PageSize(5).Do()
	if err != nil {
		log.Fatalf("[ERROR] list: %v", err)
	}
	for _, f := range list.Files {
		start := time.Now()
		resp, err := svc.Files.Get(f.Id).SupportsAllDrives(true).Download()
		if err != nil {
			fmt.Printf("%s (%d bytes): request error after %s: %v\n", f.Name, f.Size, time.Since(start).Round(time.Millisecond), err)
			continue
		}
		n, err := io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		fmt.Printf("%s: %d bytes in %s (err %v)\n", f.Name, n, time.Since(start).Round(time.Millisecond), err)
	}
}
