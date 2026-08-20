// Command migratemedia copies the drive media folders into the media bucket, generating thumbnails.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"heliosian/internal/blob"
)

const (
	folderMime = "application/vnd.google-apps.folder"
	workers    = 12
)

type source struct {
	folder   string
	id       string
	name     string
	mimeType string
}

func main() {
	ctx := context.Background()
	drv, err := drive.NewService(ctx, option.WithScopes(drive.DriveReadonlyScope))
	if err != nil {
		log.Fatalf("[ERROR] drive client: %v", err)
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("[ERROR] storage client: %v", err)
	}
	bucket := client.Bucket(blob.Bucket)

	drives, err := drv.Drives.List().Do()
	if err != nil {
		log.Fatalf("[ERROR] list shared drives: %v", err)
	}
	if len(drives.Drives) != 1 {
		log.Fatalf("[ERROR] expected one shared drive, found %d", len(drives.Drives))
	}
	root := drives.Drives[0].Id

	sources := []source{}
	for _, folder := range []string{"people", "families"} {
		folderID, err := subfolder(drv, root, folder)
		if err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		token := ""
		for {
			call := drv.Files.List().
				Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
				SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
				Fields("nextPageToken, files(id, name, mimeType)").PageSize(1000)
			if token != "" {
				call = call.PageToken(token)
			}
			list, err := call.Do()
			if err != nil {
				log.Fatalf("[ERROR] list %s: %v", folder, err)
			}
			for _, f := range list.Files {
				if f.MimeType == folderMime {
					continue
				}
				if strings.HasSuffix(strings.TrimSuffix(f.Name, path.Ext(f.Name)), "-thumb") {
					continue
				}
				sources = append(sources, source{folder: folder, id: f.Id, name: f.Name, mimeType: f.MimeType})
			}
			if list.NextPageToken == "" {
				break
			}
			token = list.NextPageToken
		}
	}
	log.Printf("migrating %d files to gs://%s", len(sources), blob.Bucket)

	start := time.Now()
	var mu sync.Mutex
	var copied, thumbed int
	var bytesWritten int64
	var failed []string
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	for _, src := range sources {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			n, thumb, err := migrate(ctx, drv, bucket, src)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Printf("[ERROR] %s/%s: %v", src.folder, src.name, err)
				failed = append(failed, src.folder+"/"+src.name)
				return
			}
			copied++
			bytesWritten += n
			if thumb {
				thumbed++
			}
		}()
	}
	wg.Wait()

	log.Printf("copied %d files (%d thumbnails, %.1f MB) in %s",
		copied, thumbed, float64(bytesWritten)/1e6, time.Since(start).Round(time.Millisecond))
	if len(failed) > 0 {
		log.Fatalf("[ERROR] %d files failed: %s", len(failed), strings.Join(failed, " "))
	}
}

func migrate(ctx context.Context, drv *drive.Service, bucket *storage.BucketHandle, src source) (int64, bool, error) {
	resp, err := drv.Files.Get(src.id).SupportsAllDrives(true).Download()
	if err != nil {
		return 0, false, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, false, fmt.Errorf("read: %w", err)
	}
	written := int64(len(content))
	if err := write(ctx, bucket, src.folder+"/"+src.name, src.mimeType, content); err != nil {
		return 0, false, err
	}
	if !strings.HasPrefix(src.mimeType, "image/") {
		return written, false, nil
	}
	thumb, err := blob.Thumbnail(content)
	if err != nil {
		return 0, false, fmt.Errorf("thumbnail: %w", err)
	}
	base := strings.TrimSuffix(src.name, path.Ext(src.name))
	if err := write(ctx, bucket, src.folder+"/"+base+"-thumb.jpg", "image/jpeg", thumb); err != nil {
		return 0, false, err
	}
	return written + int64(len(thumb)), true, nil
}

func write(ctx context.Context, bucket *storage.BucketHandle, name, mimeType string, content []byte) error {
	w := bucket.Object(name).NewWriter(ctx)
	w.ContentType = mimeType
	if _, err := w.Write(content); err != nil {
		w.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

func subfolder(drv *drive.Service, parent, name string) (string, error) {
	list, err := drv.Files.List().
		Q(fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false", name, parent, folderMime)).
		SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
		Fields("files(id)").Do()
	if err != nil {
		return "", fmt.Errorf("find folder %s: %w", name, err)
	}
	if len(list.Files) != 1 {
		return "", fmt.Errorf("expected one %s folder, found %d", name, len(list.Files))
	}
	return list.Files[0].Id, nil
}
