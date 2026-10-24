package imagesearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/ratelimit"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type Search struct {
	Unsplash  string
	Pexels    string
	Pixabay   string
	UserAgent string
	Stock     *db.Stock
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

type result struct {
	hit Hit
	rec db.StockHit
}

type Limits struct {
	searches *ratelimit.Limiter
	imports  *ratelimit.Limiter
}

func NewLimits() *Limits {
	return &Limits{searches: ratelimit.New(searchesPerHour, time.Hour), imports: ratelimit.New(importsPerHour, time.Hour)}
}

const (
	maxThumb        = 2 << 20
	maxImageSize    = 8 << 20
	workers         = 8
	searchesPerHour = 30
	importsPerHour  = 20
	searchRefusal   = "too many picture searches in the last hour; try again later"
	importRefusal   = "too many pictures added in the last hour; try again later"
)

var (
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

func imports(l *Limits) *ratelimit.Limiter {
	return l.imports
}

func (s Search) Register(mux *http.ServeMux, prefix string, gate func(http.HandlerFunc) http.HandlerFunc) {
	mux.HandleFunc("GET "+prefix+"/images/search", gate(s.limited(searches, searchRefusal, serve.JSON(s.search))))
	mux.HandleFunc("GET "+prefix+"/images/thumb", gate(s.serveThumb))
	mux.HandleFunc("POST "+prefix+"/images/import", gate(s.limited(imports, importRefusal, serve.JSON(s.importImage))))
}

type importRequest struct {
	ID string `json:"id"`
}

type imported struct {
	Stock string `json:"stock"`
}

func (s Search) limited(which func(*Limits) *ratelimit.Limiter, refusal string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(auth.RealEmail(r))
		if !which(s.Limits).Allow(email, time.Now()) {
			slog.WarnContext(r.Context(), "image limit reached", "path", r.URL.Path, "email", email)
			serve.Error(w, r, access.Refuse(http.StatusTooManyRequests, "%s", refusal))
			return
		}
		next(w, r)
	}
}

func (s Search) search(r *http.Request, _ serve.None) ([]Hit, error) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		return nil, access.Invalid("say what to search for")
	}
	if !s.On() {
		return nil, access.Invalid("there is nowhere to search for pictures")
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
		return nil, access.Refuse(http.StatusBadGateway, "%s", err.Error())
	}
	records := []db.StockHit{}
	for _, res := range results {
		records = append(records, res.rec)
	}
	ids, err := s.Stock.Remember(r.Context(), records)
	if err != nil {
		return nil, err
	}
	go s.prefetch(ids)
	thumbPath := path.Dir(r.URL.Path) + "/thumb"
	hits := []Hit{}
	for i, res := range results {
		hit := res.hit
		hit.ID = ids[i]
		hit.Thumb = thumbPath + "?id=" + hit.ID
		hits = append(hits, hit)
	}
	return hits, nil
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

func (s Search) prefetch(ids []string) {
	slots := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			if _, _, err := s.Stock.Keep(context.Background(), id, "preview", s.fetchPreview); err != nil {
				slog.Error("prefetch a stock preview", "stock", id, "error", err)
			}
		}()
	}
	wg.Wait()
}

func (s Search) fetchPreview(row store.Row) ([]byte, string, error) {
	return fetch(context.Background(), s.UserAgent, row["preview_url"], maxThumb)
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
			hit: Hit{Width: p.Width, Height: p.Height, Title: p.Description},
			rec: db.StockHit{
				Source: "Unsplash", SourceID: p.ID, PreviewURL: p.URLs.Small, URL: p.URLs.Regular, Download: p.Links.DownloadLocation,
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
		results = append(results, result{
			hit: Hit{Width: p.Width, Height: p.Height, Title: p.Alt},
			rec: db.StockHit{
				Source: "Pexels", SourceID: strconv.Itoa(p.ID), PreviewURL: p.Src.Medium, URL: p.Src.Large2x,
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
		results = append(results, result{
			hit: Hit{Width: h.ImageWidth, Height: h.ImageHeight, Title: h.Tags},
			rec: db.StockHit{
				Source: "Pixabay", SourceID: strconv.Itoa(h.ID), PreviewURL: h.WebformatURL, URL: h.LargeImageURL,
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
	if blob.ImageExtensions[mimeType] == "" {
		return nil, "", errNotImage
	}
	return content, mimeType, nil
}

func stockID(id string) bool {
	table, ok := db.TableOf(id)
	return ok && table == "STOCK"
}

func (s Search) serveThumb(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if !s.On() || !stockID(id) {
		http.NotFound(w, r)
		return
	}
	content, _, err := s.Stock.Keep(r.Context(), id, "preview", s.fetchPreview)
	if errors.Is(err, blob.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "stock preview", "stock", id, "error", err)
		http.Error(w, "could not fetch that picture", http.StatusBadGateway)
		return
	}
	data, mimeType, err := s.Stock.Bytes(r.Context(), content)
	if err != nil {
		slog.ErrorContext(r.Context(), "stock preview", "stock", id, "error", err)
		http.Error(w, "could not read that picture", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Write(data)
}

func (s Search) importImage(r *http.Request, body importRequest) (imported, error) {
	if !s.On() || !stockID(body.ID) {
		return imported{}, access.Missing("no such picture")
	}
	ctx := r.Context()
	_, fetched, err := s.Stock.Keep(ctx, body.ID, "picture", func(row store.Row) ([]byte, string, error) {
		return fetch(ctx, s.UserAgent, row["url"], maxImageSize)
	})
	switch {
	case errors.Is(err, blob.ErrNotFound):
		return imported{}, access.Missing("no such picture")
	case errors.Is(err, errTooLarge), errors.Is(err, errNotImage):
		return imported{}, access.Invalid("%s", err.Error())
	case err != nil:
		slog.ErrorContext(ctx, "stock picture", "stock", body.ID, "error", err)
		return imported{}, access.Refuse(http.StatusBadGateway, "could not fetch that image")
	}
	row, _ := s.Stock.Row(body.ID)
	if fetched && strings.HasPrefix(row["download"], "https://api.unsplash.com/") && s.Unsplash != "" {
		go func(endpoint string) {
			req, _ := http.NewRequest("GET", endpoint, nil)
			req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
			if resp, err := imageClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}(row["download"])
	}
	slog.InfoContext(ctx, "imported image", "stock", body.ID, "fetched", fetched)
	return imported{Stock: body.ID}, nil
}
