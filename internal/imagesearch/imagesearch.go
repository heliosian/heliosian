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

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/ratelimit"
)

type Search struct {
	Unsplash  string
	Pexels    string
	Pixabay   string
	UserAgent string
	Stock     *Stock
	Limits    *Limits
}

func (s Search) On() bool {
	return s.Stock != nil && (s.Unsplash != "" || s.Pexels != "" || s.Pixabay != "")
}

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

type Limits struct {
	searches *ratelimit.Limiter
	stores   *ratelimit.Limiter
}

func NewLimits() *Limits {
	return &Limits{searches: ratelimit.New(searchesPerHour, time.Hour), stores: ratelimit.New(storesPerHour, time.Hour)}
}

const (
	stockPrefix     = "stock/"
	maxThumb        = 2 << 20
	maxImageSize    = 8 << 20
	workers         = 8
	searchesPerHour = 30
	storesPerHour   = 20
	searchRefusal   = "too many picture searches in the last hour; try again later"
	storeRefusal    = "too many pictures added in the last hour; try again later"
)

var (
	idPattern   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	extensions  = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}
	errTooLarge = errors.New("that image is too large")
	errNotImage = errors.New("that is not a supported image")
	imageClient = &http.Client{Timeout: 20 * time.Second}
)

func Members(next http.HandlerFunc) http.HandlerFunc {
	return next
}

func searches(l *Limits) *ratelimit.Limiter {
	return l.searches
}

func stores(l *Limits) *ratelimit.Limiter {
	return l.stores
}

func (s Search) Register(mux *http.ServeMux, prefix, folder string, gate func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("POST "+prefix+"/image", gate(s.limited(stores, storeRefusal, func(w http.ResponseWriter, r *http.Request) {
		s.serveUpload(w, r, folder)
	})))
	mux.HandleFunc("GET "+prefix+"/images/search", gate(s.limited(searches, searchRefusal, s.serveSearch)))
	mux.HandleFunc("GET "+prefix+"/images/thumb", gate(s.serveThumb))
	mux.HandleFunc("POST "+prefix+"/images/import", gate(s.limited(stores, storeRefusal, func(w http.ResponseWriter, r *http.Request) {
		s.serveImport(w, r, folder)
	})))
}

func (s Search) limited(which func(*Limits) *ratelimit.Limiter, refusal string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(auth.RealEmail(r))
		if !which(s.Limits).Allow(email, time.Now()) {
			slog.WarnContext(r.Context(), "image limit reached", "path", r.URL.Path, "email", email)
			http.Error(w, refusal, http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func (s Search) serveUpload(w http.ResponseWriter, r *http.Request, folder string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	if extensions[mimeType] == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	s.store(w, r, folder, mimeType, content)
}

func (s Search) store(w http.ResponseWriter, r *http.Request, folder, mimeType string, content []byte) {
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + extensions[mimeType]
	if err := s.Stock.store.Put(folder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] store image", "folder", folder, "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"name": folder + "/" + name}); err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] encode image name", "error", err)
	}
}

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

func (s Search) serveSearch(w http.ResponseWriter, r *http.Request) {
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
	results, err := gather(r.Context(), q, searches)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	s.Stock.remember(results)
	go s.Stock.prefetch(results, s.UserAgent)
	thumbPath := path.Dir(r.URL.Path) + "/thumb"
	hits := []Hit{}
	for _, res := range results {
		hit := res.hit
		hit.Thumb = thumbPath + "?id=" + hit.ID
		hits = append(hits, hit)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

func gather(ctx context.Context, q string, searches []func(context.Context, string) ([]result, error)) ([]result, error) {
	lists := make([][]result, len(searches))
	errs := make([]error, len(searches))
	var wg sync.WaitGroup
	for i, search := range searches {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lists[i], errs[i] = search(ctx, q)
		}()
	}
	wg.Wait()
	answered := [][]result{}
	for i, err := range errs {
		if err != nil {
			slog.WarnContext(ctx, "image search: a library failed", "error", err)
			continue
		}
		answered = append(answered, lists[i])
	}
	if len(answered) == 0 {
		return nil, errors.Join(errs...)
	}
	return interleave(answered), nil
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
			slog.Error("[ERROR] keep a search result", "id", res.hit.ID, "error", err)
		}
	})
	st.each(results, func(res result) {
		if err := st.thumbnail(ctx, res.hit.ID, userAgent); err != nil {
			slog.Error("[ERROR] prefetch a thumbnail", "id", res.hit.ID, "error", err)
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
	results := []result{}
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
	results := []result{}
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
	results := []result{}
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

func (s Search) serveThumb(w http.ResponseWriter, r *http.Request) {
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
		slog.ErrorContext(r.Context(), "[ERROR] stock thumbnail", "id", id, "error", err)
		http.Error(w, "could not fetch that picture", http.StatusBadGateway)
		return
	}
	content, mimeType, err := s.Stock.store.Read(r.Context(), thumbName(id))
	if err != nil {
		slog.ErrorContext(r.Context(), "[ERROR] stock thumbnail", "id", id, "error", err)
		http.Error(w, "could not read that picture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(content)
}

func (s Search) serveImport(w http.ResponseWriter, r *http.Request, folder string) {
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
			content, mimeType, err = fetch(ctx, s.UserAgent, rec.URL, maxImageSize)
		}
		switch {
		case errors.Is(err, errTooLarge), errors.Is(err, errNotImage):
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		case err != nil:
			slog.ErrorContext(ctx, "[ERROR] stock image", "id", body.ID, "error", err)
			http.Error(w, "could not fetch that image", http.StatusBadGateway)
			return
		}
		if err := s.Stock.store.Write(ctx, imageName(body.ID), mimeType, content); err != nil {
			slog.ErrorContext(ctx, "[ERROR] keep stock image", "id", body.ID, "error", err)
			http.Error(w, "could not store the image", http.StatusInternalServerError)
			return
		}
		fetched = true
		download = rec.Download
	}
	if err != nil {
		slog.ErrorContext(ctx, "[ERROR] stock image", "id", body.ID, "error", err)
		http.Error(w, "could not read that image", http.StatusInternalServerError)
		return
	}
	if int64(len(content)) > maxImageSize {
		http.Error(w, errTooLarge.Error(), http.StatusBadRequest)
		return
	}
	if fetched && strings.HasPrefix(download, "https://api.unsplash.com/") && s.Unsplash != "" {
		go func(endpoint string) {
			req, _ := http.NewRequest("GET", endpoint, nil)
			req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
			if resp, err := imageClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}(download)
	}
	slog.InfoContext(ctx, "imported image", "id", body.ID, "folder", folder, "fetched", fetched)
	s.store(w, r, folder, mimeType, content)
}
