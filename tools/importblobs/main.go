// Command importblobs copies directory media into the drive folder under sane names.
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/directory"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const folderMime = "application/vnd.google-apps.folder"

var extensions = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
	"image/gif":  ".gif",
	"audio/mpeg": ".mp3",
	"audio/mp3":  ".mp3",
	"audio/mp4":  ".m4a",
	"audio/wav":  ".wav",
}

type task struct {
	folderName string
	base       string
	url        string
}

func localPart(email string) string {
	name, _, _ := strings.Cut(email, "@")
	return name
}

func findRoot(svc *drive.Service) string {
	drives, err := svc.Drives.List().Do()
	if err != nil {
		log.Fatalf("[ERROR] list shared drives: %v", err)
	}
	if len(drives.Drives) == 1 {
		log.Printf("using shared drive %q (%s)", drives.Drives[0].Name, drives.Drives[0].Id)
		return drives.Drives[0].Id
	}
	if len(drives.Drives) > 1 {
		for _, d := range drives.Drives {
			log.Printf("candidate shared drive: %s (%s)", d.Name, d.Id)
		}
		log.Fatalf("[ERROR] service account can see %d shared drives; pass -folder", len(drives.Drives))
	}
	list, err := svc.Files.List().
		Q("mimeType = '" + folderMime + "' and sharedWithMe = true and trashed = false").
		Fields("files(id, name)").Do()
	if err != nil {
		log.Fatalf("[ERROR] list shared folders: %v", err)
	}
	if len(list.Files) != 1 {
		for _, f := range list.Files {
			log.Printf("candidate folder: %s (%s)", f.Name, f.Id)
		}
		log.Fatalf("[ERROR] expected one shared drive or one shared folder, found %d folders; pass -folder", len(list.Files))
	}
	log.Printf("using folder %q (%s); note: uploads into personal drives fail on service account quota", list.Files[0].Name, list.Files[0].Id)
	return list.Files[0].Id
}

func ensureFolder(svc *drive.Service, parent, name string) string {
	list, err := svc.Files.List().
		Q(fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false", name, parent, folderMime)).
		SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
		Fields("files(id)").Do()
	if err != nil {
		log.Fatalf("[ERROR] find folder %s: %v", name, err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id
	}
	created, err := svc.Files.Create(&drive.File{Name: name, MimeType: folderMime, Parents: []string{parent}}).
		SupportsAllDrives(true).Fields("id").Do()
	if err != nil {
		log.Fatalf("[ERROR] create folder %s: %v", name, err)
	}
	return created.Id
}

func listBases(svc *drive.Service, folderID string) map[string]bool {
	bases := map[string]bool{}
	deleted := 0
	token := ""
	for {
		call := svc.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
			SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
			Fields("nextPageToken, files(id, name)").PageSize(1000)
		if token != "" {
			call = call.PageToken(token)
		}
		list, err := call.Do()
		if err != nil {
			log.Fatalf("[ERROR] list folder contents: %v", err)
		}
		for _, f := range list.Files {
			base := strings.TrimSuffix(f.Name, path.Ext(f.Name))
			if strings.HasSuffix(base, "-thumb") {
				_, err := svc.Files.Update(f.Id, &drive.File{Trashed: true}).SupportsAllDrives(true).Do()
				if err != nil {
					log.Fatalf("[ERROR] trash stale thumb %s: %v", f.Name, err)
				}
				deleted++
				continue
			}
			bases[base] = true
		}
		if list.NextPageToken == "" {
			if deleted > 0 {
				log.Printf("deleted %d stale thumbs", deleted)
			}
			return bases
		}
		token = list.NextPageToken
	}
}

func folderStats(svc *drive.Service, folderID string) (int64, int64) {
	var files, bytes int64
	token := ""
	for {
		call := svc.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
			SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
			Fields("nextPageToken, files(size)").PageSize(1000)
		if token != "" {
			call = call.PageToken(token)
		}
		list, err := call.Do()
		if err != nil {
			log.Fatalf("[ERROR] list folder for stats: %v", err)
		}
		for _, f := range list.Files {
			files++
			bytes += f.Size
		}
		if list.NextPageToken == "" {
			return files, bytes
		}
		token = list.NextPageToken
	}
}

func fetch(client *http.Client, url string) ([]byte, string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType, _, _ := strings.Cut(resp.Header.Get("Content-Type"), ";")
	return body, strings.TrimSpace(contentType), nil
}

func extension(contentType, url string) string {
	if ext, ok := extensions[contentType]; ok {
		return ext
	}
	if ext := path.Ext(strings.SplitN(path.Base(url), "?", 2)[0]); ext != "" && len(ext) <= 5 {
		return ext
	}
	return ".bin"
}

func main() {
	sheetID := flag.String("sheet", "", "directory spreadsheet id")
	folderID := flag.String("folder", "", "drive folder id (default: the folder shared with the service account)")
	flag.Parse()
	if *sheetID == "" {
		log.Fatal("[ERROR] -sheet is required")
	}

	source, err := data.NewSheet(map[string]string{"directory": *sheetID})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source)
	if err != nil {
		log.Fatalf("[ERROR] load model: %v", err)
	}

	svc, err := drive.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(drive.DriveScope))
	if err != nil {
		log.Fatalf("[ERROR] drive client: %v", err)
	}

	root := *folderID
	if root == "" {
		root = findRoot(svc)
	}
	folders := map[string]string{
		"people":   ensureFolder(svc, root, "people"),
		"families": ensureFolder(svc, root, "families"),
	}

	tasks := []task{}
	for _, p := range model.People {
		if p.PhotoURL != "" {
			tasks = append(tasks, task{"people", localPart(p.Email) + "-photo", p.PhotoURL})
		}
		if p.PronunciationURL != "" {
			tasks = append(tasks, task{"people", localPart(p.Email) + "-pronunciation", p.PronunciationURL})
		}
	}
	for key, f := range model.Families {
		if f.PhotoURL != "" {
			tasks = append(tasks, task{"families", localPart(key) + "-photo", f.PhotoURL})
		}
		if f.PronunciationURL != "" {
			tasks = append(tasks, task{"families", localPart(key) + "-pronunciation", f.PronunciationURL})
		}
	}

	pending := []task{}
	skipped := 0
	for folderName, id := range folders {
		bases := listBases(svc, id)
		for _, t := range tasks {
			if t.folderName != folderName {
				continue
			}
			if bases[t.base] {
				skipped++
			} else {
				pending = append(pending, t)
			}
		}
	}
	log.Printf("%d media files in model, %d already imported, %d to fetch", len(tasks), skipped, len(pending))

	client := &http.Client{Timeout: 60 * time.Second}
	var mu sync.Mutex
	uploaded := 0
	work := make(chan task)
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range work {
				body, contentType, err := fetch(client, t.url)
				if err != nil {
					log.Fatalf("[ERROR] fetch %s/%s: %v", t.folderName, t.base, err)
				}
				name := t.base + extension(contentType, t.url)
				_, err = svc.Files.Create(&drive.File{Name: name, Parents: []string{folders[t.folderName]}}).
					Media(bytes.NewReader(body), googleapi.ContentType(contentType)).
					SupportsAllDrives(true).Fields("id").Do()
				if err != nil {
					log.Fatalf("[ERROR] upload %s/%s: %v", t.folderName, name, err)
				}
				mu.Lock()
				uploaded++
				if uploaded%50 == 0 {
					log.Printf("uploaded %d/%d", uploaded, len(pending))
				}
				mu.Unlock()
			}
		}()
	}
	for _, t := range pending {
		work <- t
	}
	close(work)
	wg.Wait()
	log.Printf("done: %d uploaded, %d skipped", uploaded, skipped)

	var totalFiles, totalBytes int64
	for _, folderName := range []string{"people", "families"} {
		files, bytes := folderStats(svc, folders[folderName])
		totalFiles += files
		totalBytes += bytes
		log.Printf("%s: %d files, %.1f MB", folderName, files, float64(bytes)/1e6)
	}
	log.Printf("total: %d files, %d bytes (%.1f MB)", totalFiles, totalBytes, float64(totalBytes)/1e6)
}
