// Package blob serves directory media from drive, held fully in memory with startup-generated thumbnails.
package blob

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"heliosian/internal/data"
)

const (
	folderMime      = "application/vnd.google-apps.folder"
	refreshInterval = 5 * time.Minute
	thumbWidth      = 480
)

type entry struct {
	id       string
	mimeType string
	data     []byte
	thumb    []byte
}

type listed struct {
	id       string
	mimeType string
}

type Store struct {
	service *drive.Service
	root    string
	mu      sync.RWMutex
	entries map[string]*entry
}

func New() (*Store, error) {
	service, err := drive.NewService(context.Background(),
		option.WithCredentialsFile(data.KeyFile),
		option.WithScopes(drive.DriveScope))
	if err != nil {
		return nil, err
	}
	drives, err := service.Drives.List().Do()
	if err != nil {
		return nil, fmt.Errorf("list shared drives: %w", err)
	}
	if len(drives.Drives) != 1 {
		return nil, fmt.Errorf("expected one shared drive visible to the service account, found %d", len(drives.Drives))
	}
	s := &Store{service: service, root: drives.Drives[0].Id, entries: map[string]*entry{}}
	if err := s.refresh(); err != nil {
		return nil, err
	}
	go s.refreshLoop()
	return s, nil
}

func Register(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /blob/{folder}/{name}", s.serve)
}

func (s *Store) refreshLoop() {
	for range time.Tick(refreshInterval) {
		if err := s.refresh(); err != nil {
			log.Printf("[ERROR] blob refresh: %v", err)
		}
	}
}

func (s *Store) refresh() error {
	start := time.Now()
	listing := map[string]listed{}
	for _, folderName := range []string{"people", "families"} {
		folderID, err := s.subfolder(folderName)
		if err != nil {
			return err
		}
		token := ""
		for {
			call := s.service.Files.List().
				Q(fmt.Sprintf("'%s' in parents and trashed = false", folderID)).
				SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
				Fields("nextPageToken, files(id, name, mimeType)").PageSize(1000)
			if token != "" {
				call = call.PageToken(token)
			}
			list, err := call.Do()
			if err != nil {
				return fmt.Errorf("list %s: %w", folderName, err)
			}
			for _, f := range list.Files {
				if f.MimeType == folderMime {
					continue
				}
				base := strings.TrimSuffix(f.Name, path.Ext(f.Name))
				if strings.HasSuffix(base, "-thumb") {
					continue
				}
				listing[folderName+"/"+base] = listed{id: f.Id, mimeType: f.MimeType}
			}
			if list.NextPageToken == "" {
				break
			}
			token = list.NextPageToken
		}
	}

	s.mu.RLock()
	missing := []string{}
	for key, l := range listing {
		if cached, ok := s.entries[key]; !ok || cached.id != l.id {
			missing = append(missing, key)
		}
	}
	s.mu.RUnlock()

	fetched := map[string]*entry{}
	var fetchedMu sync.Mutex
	work := make(chan string)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range work {
				l := listing[key]
				fetchStart := time.Now()
				body, err := s.download(l.id)
				if err == nil {
					log.Printf("blob fetch: %s %d bytes in %s", key, len(body), time.Since(fetchStart).Round(time.Millisecond))
				} else {
					log.Printf("[ERROR] blob fetch: %s: %v", key, err)
				}
				if err == nil && strings.HasPrefix(l.mimeType, "image/") {
					var thumb []byte
					thumb, err = thumbnail(body)
					if err == nil {
						fetchedMu.Lock()
						fetched[key] = &entry{id: l.id, mimeType: l.mimeType, data: body, thumb: thumb}
						fetchedMu.Unlock()
						continue
					}
				} else if err == nil {
					fetchedMu.Lock()
					fetched[key] = &entry{id: l.id, mimeType: l.mimeType, data: body}
					fetchedMu.Unlock()
					continue
				}
				select {
				case errs <- fmt.Errorf("load %s: %w", key, err):
				default:
				}
				return
			}
		}()
	}
	for _, key := range missing {
		work <- key
	}
	close(work)
	wg.Wait()
	select {
	case err := <-errs:
		return err
	default:
	}

	next := make(map[string]*entry, len(listing))
	var totalBytes int64
	s.mu.Lock()
	for key, l := range listing {
		if e, ok := fetched[key]; ok {
			next[key] = e
		} else if cached, ok := s.entries[key]; ok && cached.id == l.id {
			next[key] = cached
		}
	}
	s.entries = next
	for _, e := range next {
		totalBytes += int64(len(e.data) + len(e.thumb))
	}
	s.mu.Unlock()
	log.Printf("blob store: %d files, %d fetched, %.1f MB in memory in %s",
		len(next), len(fetched), float64(totalBytes)/1e6, time.Since(start).Round(time.Millisecond))
	return nil
}

func (s *Store) subfolder(name string) (string, error) {
	return s.subfolderIn(s.root, name, false)
}

func (s *Store) subfolderIn(parent, name string, create bool) (string, error) {
	list, err := s.service.Files.List().
		Q(fmt.Sprintf("name = '%s' and '%s' in parents and mimeType = '%s' and trashed = false", name, parent, folderMime)).
		SupportsAllDrives(true).IncludeItemsFromAllDrives(true).Corpora("allDrives").
		Fields("files(id)").Do()
	if err != nil {
		return "", fmt.Errorf("find folder %s: %w", name, err)
	}
	if len(list.Files) == 0 && create {
		folder, err := s.service.Files.Create(&drive.File{
			Name:     name,
			MimeType: folderMime,
			Parents:  []string{parent},
		}).SupportsAllDrives(true).Fields("id").Do()
		if err != nil {
			return "", fmt.Errorf("create folder %s: %w", name, err)
		}
		return folder.Id, nil
	}
	if len(list.Files) != 1 {
		return "", fmt.Errorf("expected one %s folder, found %d", name, len(list.Files))
	}
	return list.Files[0].Id, nil
}

func (s *Store) Refresh() error {
	return s.refresh()
}

func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[key]
	return ok
}

func (s *Store) Upload(folder, base, ext, mimeType string, content []byte) (string, error) {
	folderID, err := s.subfolder(folder)
	if err != nil {
		return "", err
	}
	archived := ""
	s.mu.RLock()
	existing, exists := s.entries[folder+"/"+base]
	s.mu.RUnlock()
	if exists {
		current, err := s.service.Files.Get(existing.id).SupportsAllDrives(true).Fields("name").Do()
		if err != nil {
			return "", fmt.Errorf("look up current %s/%s: %w", folder, base, err)
		}
		archiveID, err := s.subfolderIn(folderID, "archive", true)
		if err != nil {
			return "", err
		}
		archived = strings.TrimSuffix(current.Name, path.Ext(current.Name)) +
			"-" + time.Now().UTC().Format("20060102-150405") + path.Ext(current.Name)
		_, err = s.service.Files.Update(existing.id, &drive.File{Name: archived}).
			AddParents(archiveID).RemoveParents(folderID).SupportsAllDrives(true).Do()
		if err != nil {
			return "", fmt.Errorf("archive %s/%s: %w", folder, base, err)
		}
	}
	_, err = s.service.Files.Create(&drive.File{
		Name:     base + "." + ext,
		MimeType: mimeType,
		Parents:  []string{folderID},
	}).SupportsAllDrives(true).Media(bytes.NewReader(content)).Do()
	if err != nil {
		return "", fmt.Errorf("upload %s/%s: %w", folder, base, err)
	}
	if err := s.refresh(); err != nil {
		return "", fmt.Errorf("refresh after upload: %w", err)
	}
	return archived, nil
}

func (s *Store) download(id string) ([]byte, error) {
	resp, err := s.service.Files.Get(id).SupportsAllDrives(true).Download()
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (s *Store) serve(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("folder") + "/" + r.PathValue("name")
	s.mu.RLock()
	e, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("thumb") == "1" {
		if e.thumb == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(e.thumb))
		return
	}
	w.Header().Set("Content-Type", e.mimeType)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(e.data))
}

func thumbnail(src []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	if bounds.Dx() > thumbWidth {
		height := bounds.Dy() * thumbWidth / bounds.Dx()
		scaled := image.NewRGBA(image.Rect(0, 0, thumbWidth, height))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, bounds, draw.Over, nil)
		img = scaled
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
