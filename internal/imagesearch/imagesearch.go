// Package imagesearch finds pictures on the web for an app's editors - stock
// libraries behind the server's own keys - and imports a picked one into the
// media bucket the way an upload lands there. HCA-Team and Heliosian share it.
package imagesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/blob"
)

// Search is the free stock libraries the portal's picture picker looks in,
// each behind the server's own key.
type Search struct {
	// Unsplash is an Unsplash API access key: free stock photographs, which
	// suit an event's card. Its terms ask for a ping of the photo's download
	// endpoint when one is taken, and that is kept here.
	Unsplash string
	// Pexels and Pixabay are two more free stock libraries, each behind its
	// own key; Pixabay also carries illustrations and vectors, which the
	// photo libraries lack.
	Pexels  string
	Pixabay string
	// UserAgent names the app to the sites it fetches from.
	UserAgent string
	Stock     *Stock
}

// On reports whether there is anywhere to search.
func (s Search) On() bool {
	return s.Stock != nil && (s.Unsplash != "" || s.Pexels != "" || s.Pixabay != "")
}

// Hit is one result as the picker shows it: a thumbnail to show and the id
// to import it by.
type Hit struct {
	ID     string `json:"id"`
	Thumb  string `json:"thumb"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Title  string `json:"title"`
}

type record struct {
	Source   string `json:"source"`
	SourceID string `json:"sourceId"`
	Thumb    string `json:"thumb"`
	URL      string `json:"url"`
	Download string `json:"download,omitempty"`
	Credit   string `json:"credit"`
	License  string `json:"license"`
	Context  string `json:"context"`
}

type result struct {
	hit Hit
	rec record
}

type Stock struct {
	store   *blob.Store
	slots   chan struct{}
	mu      sync.Mutex
	records map[string]record
	have    map[string]bool
	pending map[string]chan struct{}
}

func NewStock(store *blob.Store) *Stock {
	return &Stock{store: store, slots: make(chan struct{}, workers), records: map[string]record{}, have: map[string]bool{}, pending: map[string]chan struct{}{}}
}

const (
	stockPrefix = "stock/"
	maxThumb    = 2 << 20
	workers     = 8
)

var (
	idPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	extensions  = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}
	errTooLarge = errors.New("that image is too large")
	errNotImage = errors.New("that is not a supported image")
	imageClient = &http.Client{Timeout: 20 * time.Second}
)

func key(source, sourceID string) string {
	sum := sha256.Sum256([]byte(source + "/" + sourceID))
	return hex.EncodeToString(sum[:])
}

func recordName(id string) string {
	return stockPrefix + id + ".json"
}

func thumbName(id string) string {
	return stockPrefix + id + "-thumb"
}

func imageName(id string) string {
	return stockPrefix + id
}

// ServeSearch answers one image search (?q=) from every library that is set
// up, keeping the keys on the server, and sets about fetching the thumbnails
// it does not have yet. SafeSearch is always on; this is a school.
func (s Search) ServeSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "say what to search for", http.StatusBadRequest)
		return
	}
	if !s.On() {
		http.Error(w, "there is nowhere to search for pictures", http.StatusBadRequest)
		return
	}
	searches := []func(context.Context, string) ([]result, error){}
	if s.Unsplash != "" {
		searches = append(searches, s.searchUnsplash)
	}
	if s.Pexels != "" {
		searches = append(searches, s.searchPexels)
	}
	if s.Pixabay != "" {
		searches = append(searches, s.searchPixabay)
	}
	lists := make([][]result, len(searches))
	errs := make([]error, len(searches))
	var wg sync.WaitGroup
	for i, search := range searches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lists[i], errs[i] = search(r.Context(), q)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	results := interleave(lists)
	s.Stock.remember(results)
	go s.Stock.prefetch(results, s.UserAgent)
	thumbPath := path.Dir(r.URL.Path) + "/thumb"
	hits := make([]Hit, 0, len(results))
	for _, res := range results {
		hit := res.hit
		hit.Thumb = thumbPath + "?id=" + hit.ID
		hits = append(hits, hit)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

func interleave(lists [][]result) []result {
	out := []result{}
	for i := 0; ; i++ {
		more := false
		for _, list := range lists {
			if i < len(list) {
				out = append(out, list[i])
				more = true
			}
		}
		if !more {
			return out
		}
	}
}

func (st *Stock) remember(results []result) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, res := range results {
		st.records[res.hit.ID] = res.rec
	}
}

func (st *Stock) prefetch(results []result, userAgent string) {
	ctx := context.Background()
	st.each(results, func(res result) {
		body, err := json.Marshal(res.rec)
		if err == nil {
			err = st.store.Write(ctx, recordName(res.hit.ID), "application/json", body)
		}
		if err != nil {
			slog.Error("keep a search result", "id", res.hit.ID, "error", err)
		}
	})
	st.each(results, func(res result) {
		if err := st.thumbnail(ctx, res.hit.ID, userAgent); err != nil {
			slog.Error("prefetch a thumbnail", "id", res.hit.ID, "error", err)
		}
	})
}

func (st *Stock) each(results []result, f func(result)) {
	var wg sync.WaitGroup
	for _, res := range results {
		wg.Add(1)
		st.slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-st.slots }()
			f(res)
		}()
	}
	wg.Wait()
}

func (st *Stock) record(ctx context.Context, id string) (record, error) {
	st.mu.Lock()
	rec, ok := st.records[id]
	st.mu.Unlock()
	if ok {
		return rec, nil
	}
	body, _, err := st.store.Read(ctx, recordName(id))
	if err != nil {
		return record{}, err
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return record{}, fmt.Errorf("read the record of %s: %w", id, err)
	}
	st.mu.Lock()
	st.records[id] = rec
	st.mu.Unlock()
	return rec, nil
}

func (st *Stock) thumbnail(ctx context.Context, id, userAgent string) error {
	for {
		st.mu.Lock()
		if st.have[id] {
			st.mu.Unlock()
			return nil
		}
		wait, fetching := st.pending[id]
		if !fetching {
			wait = make(chan struct{})
			st.pending[id] = wait
		}
		st.mu.Unlock()
		if !fetching {
			break
		}
		select {
		case <-wait:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	err := st.fetchThumbnail(ctx, id, userAgent)
	st.mu.Lock()
	if err == nil {
		st.have[id] = true
	}
	close(st.pending[id])
	delete(st.pending, id)
	st.mu.Unlock()
	return err
}

func (st *Stock) fetchThumbnail(ctx context.Context, id, userAgent string) error {
	name := thumbName(id)
	held, err := st.store.Exists(ctx, name)
	if err != nil || held {
		return err
	}
	rec, err := st.record(ctx, id)
	if err != nil {
		return err
	}
	content, mimeType, err := fetch(ctx, userAgent, rec.Thumb, maxThumb)
	if err != nil {
		return err
	}
	return st.store.Write(ctx, name, mimeType, content)
}

// searchUnsplash asks Unsplash for photographs matching the words: landscape
// ones, filtered for a school, each with its download endpoint for the ping.
func (s Search) searchUnsplash(ctx context.Context, q string) ([]result, error) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}, "content_filter": {"high"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.unsplash.com/search/photos?"+params.Encode(), nil)
	req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
	req.Header.Set("Accept-Version", "v1")
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, errors.New("Unsplash did not answer")
	}
	defer resp.Body.Close()
	var body struct {
		Results []struct {
			ID          string `json:"id"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Description string `json:"alt_description"`
			URLs        struct {
				Regular string `json:"regular"`
				Small   string `json:"small"`
			} `json:"urls"`
			Links struct {
				HTML             string `json:"html"`
				DownloadLocation string `json:"download_location"`
			} `json:"links"`
			User struct {
				Name string `json:"name"`
			} `json:"user"`
		} `json:"results"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("Unsplash answered oddly")
	}
	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(ctx, "unsplash refused", "status", resp.StatusCode, "errors", body.Errors)
		return nil, errors.New("Unsplash refused the search: " + strings.Join(body.Errors, "; "))
	}
	results := make([]result, 0, len(body.Results))
	for _, p := range body.Results {
		results = append(results, result{
			hit: Hit{ID: key("Unsplash", p.ID), Width: p.Width, Height: p.Height, Title: p.Description},
			rec: record{
				Source: "Unsplash", SourceID: p.ID, Thumb: p.URLs.Small, URL: p.URLs.Regular, Download: p.Links.DownloadLocation,
				Credit: "Photo by " + p.User.Name + " on Unsplash", License: "Unsplash License", Context: p.Links.HTML,
			},
		})
	}
	return results, nil
}

// searchPexels asks Pexels for landscape photographs.
func (s Search) searchPexels(ctx context.Context, q string) ([]result, error) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://api.pexels.com/v1/search?"+params.Encode(), nil)
	req.Header.Set("Authorization", s.Pexels)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, errors.New("Pexels did not answer")
	}
	defer resp.Body.Close()
	var body struct {
		Photos []struct {
			ID           int    `json:"id"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			URL          string `json:"url"`
			Photographer string `json:"photographer"`
			Alt          string `json:"alt"`
			Src          struct {
				Large2x string `json:"large2x"`
				Medium  string `json:"medium"`
			} `json:"src"`
		} `json:"photos"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, errors.New("Pexels answered oddly")
	}
	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(ctx, "pexels refused", "status", resp.StatusCode, "error", body.Error)
		return nil, errors.New("Pexels refused the search: " + body.Error)
	}
	results := make([]result, 0, len(body.Photos))
	for _, p := range body.Photos {
		id := strconv.Itoa(p.ID)
		results = append(results, result{
			hit: Hit{ID: key("Pexels", id), Width: p.Width, Height: p.Height, Title: p.Alt},
			rec: record{
				Source: "Pexels", SourceID: id, Thumb: p.Src.Medium, URL: p.Src.Large2x,
				Credit: "Photo by " + p.Photographer + " on Pexels", License: "Pexels License", Context: p.URL,
			},
		})
	}
	return results, nil
}

// searchPixabay asks Pixabay for photos, illustrations and vectors alike,
// landscape and safe.
func (s Search) searchPixabay(ctx context.Context, q string) ([]result, error) {
	params := url.Values{
		"key": {s.Pixabay}, "q": {q}, "per_page": {"24"}, "orientation": {"horizontal"},
		"safesearch": {"true"}, "image_type": {"all"},
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://pixabay.com/api/?"+params.Encode(), nil)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, errors.New("Pixabay did not answer")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		// Pixabay answers a bad key with plain text.
		slog.WarnContext(ctx, "pixabay refused", "status", resp.StatusCode, "body", string(raw))
		return nil, errors.New("Pixabay refused the search: " + strings.TrimSpace(string(raw)))
	}
	var body struct {
		Hits []struct {
			ID            int    `json:"id"`
			ImageWidth    int    `json:"imageWidth"`
			ImageHeight   int    `json:"imageHeight"`
			PageURL       string `json:"pageURL"`
			Tags          string `json:"tags"`
			User          string `json:"user"`
			WebformatURL  string `json:"webformatURL"`
			LargeImageURL string `json:"largeImageURL"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.New("Pixabay answered oddly")
	}
	results := make([]result, 0, len(body.Hits))
	for _, h := range body.Hits {
		id := strconv.Itoa(h.ID)
		results = append(results, result{
			hit: Hit{ID: key("Pixabay", id), Width: h.ImageWidth, Height: h.ImageHeight, Title: h.Tags},
			rec: record{
				Source: "Pixabay", SourceID: id, Thumb: h.WebformatURL, URL: h.LargeImageURL,
				Credit: "Image by " + h.User + " on Pixabay", License: "Pixabay Content License", Context: h.PageURL,
			},
		})
	}
	return results, nil
}

func fetch(ctx context.Context, userAgent, src string, limit int64) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", src, nil)
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", src, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", src, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch %s: answered %d", src, resp.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", src, err)
	}
	if int64(len(content)) > limit {
		return nil, "", errTooLarge
	}
	mimeType := http.DetectContentType(content)
	if extensions[mimeType] == "" {
		return nil, "", errNotImage
	}
	return content, mimeType, nil
}

func (s Search) ServeThumb(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !s.On() || !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return
	}
	err := s.Stock.thumbnail(r.Context(), id, s.UserAgent)
	if errors.Is(err, blob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "stock thumbnail", "id", id, "error", err)
		http.Error(w, "could not fetch that picture", http.StatusBadGateway)
		return
	}
	content, mimeType, err := s.Stock.store.Read(r.Context(), thumbName(id))
	if err != nil {
		slog.ErrorContext(r.Context(), "stock thumbnail", "id", id, "error", err)
		http.Error(w, "could not read that picture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(content)
}

// ServeImport takes a picked result ({"id"} in the body) into `folder` of
// the store the way an upload is stored, answering with the name the sheet
// should record.
func (s Search) ServeImport(w http.ResponseWriter, r *http.Request, folder string, maxSize int64) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if !s.On() || !idPattern.MatchString(body.ID) {
		http.Error(w, "no such picture", http.StatusNotFound)
		return
	}
	ctx := r.Context()
	content, mimeType, err := s.Stock.store.Read(ctx, imageName(body.ID))
	fetched := false
	download := ""
	if errors.Is(err, blob.ErrNotFound) {
		var rec record
		rec, err = s.Stock.record(ctx, body.ID)
		if errors.Is(err, blob.ErrNotFound) {
			http.Error(w, "no such picture", http.StatusNotFound)
			return
		}
		if err == nil {
			content, mimeType, err = fetch(ctx, s.UserAgent, rec.URL, maxSize)
		}
		switch {
		case errors.Is(err, errTooLarge), errors.Is(err, errNotImage):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case err != nil:
			slog.ErrorContext(ctx, "stock image", "id", body.ID, "error", err)
			http.Error(w, "could not fetch that image", http.StatusBadGateway)
			return
		}
		if err := s.Stock.store.Write(ctx, imageName(body.ID), mimeType, content); err != nil {
			slog.ErrorContext(ctx, "keep stock image", "id", body.ID, "error", err)
			http.Error(w, "could not store the image", http.StatusInternalServerError)
			return
		}
		fetched = true
		download = rec.Download
	}
	if err != nil {
		slog.ErrorContext(ctx, "stock image", "id", body.ID, "error", err)
		http.Error(w, "could not read that image", http.StatusInternalServerError)
		return
	}
	if int64(len(content)) > maxSize {
		http.Error(w, errTooLarge.Error(), http.StatusBadRequest)
		return
	}
	// Unsplash counts a download through its own endpoint, and asks that a
	// taken photo be reported there; the answer does not matter.
	if fetched && strings.HasPrefix(download, "https://api.unsplash.com/") && s.Unsplash != "" {
		go func(endpoint string) {
			req, _ := http.NewRequest("GET", endpoint, nil)
			req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
			if resp, err := imageClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}(download)
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + extensions[mimeType]
	if err := s.Stock.store.Put(folder, name, mimeType, content); err != nil {
		slog.ErrorContext(ctx, "store imported image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	slog.InfoContext(ctx, "imported image", "id", body.ID, "name", name, "fetched", fetched)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": folder + "/" + name})
}
