// Package imagesearch finds pictures on the web for an app's editors - stock
// libraries behind the server's own keys - and imports a picked one into the
// media bucket the way an upload lands there. HCA-Team and Heliosian share it.
package imagesearch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"heliosian/internal/blob"
)

// Search is the free stock libraries the portal's picture picker looks in,
// each behind the server's own key.
type Search struct {
	// Unsplash is an Unsplash API access key: free stock photographs, which
	// suit an event's card. Its terms ask for a credit line and a ping of the
	// photo's download endpoint when one is taken, and both are kept here.
	Unsplash string
	// Pexels and Pixabay are two more free stock libraries, each behind its
	// own key; Pixabay also carries illustrations and vectors, which the
	// photo libraries lack.
	Pexels  string
	Pixabay string
	// UserAgent names the app to the sites it fetches from.
	UserAgent string
}

// Sources lists where the portal can look, in the order the picker offers them.
func (s Search) Sources() []string {
	out := []string{}
	if s.Unsplash != "" {
		out = append(out, "Unsplash")
	}
	if s.Pexels != "" {
		out = append(out, "Pexels")
	}
	if s.Pixabay != "" {
		out = append(out, "Pixabay")
	}
	return out
}

// Hit is one result as the picker shows it: a thumbnail to show, the
// full image to import, where it came from, and its licence when known.
type Hit struct {
	Thumb   string `json:"thumb"`
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Title   string `json:"title"`
	Source  string `json:"source"`
	Context string `json:"context"`
	License string `json:"license,omitempty"`
	// Credit is the line to show for the picture ("Photo by Jane Doe on
	// Unsplash"); Download is what to ping when it is taken, as Unsplash asks.
	Credit   string `json:"credit,omitempty"`
	Download string `json:"download,omitempty"`
}

var imageClient = &http.Client{Timeout: 20 * time.Second}

// ServeSearch answers one image search (?q= and ?source=) from whichever
// library is set up, keeping the key on the server. SafeSearch is always on;
// this is a school.
func (s Search) ServeSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "say what to search for", http.StatusBadRequest)
		return
	}
	switch source := r.URL.Query().Get("source"); {
	case source == "Unsplash" && s.Unsplash != "":
		s.searchUnsplash(w, r, q)
	case source == "Pexels" && s.Pexels != "":
		s.searchPexels(w, r, q)
	case source == "Pixabay" && s.Pixabay != "":
		s.searchPixabay(w, r, q)
	default:
		http.Error(w, "there is nowhere to search for pictures", http.StatusBadRequest)
	}
}

// searchUnsplash asks Unsplash for photographs matching the words: landscape
// ones, filtered for a school, each with its photographer for the credit line
// and its download endpoint for the ping.
func (s Search) searchUnsplash(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}, "content_filter": {"high"}}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.unsplash.com/search/photos?"+params.Encode(), nil)
	req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
	req.Header.Set("Accept-Version", "v1")
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "Unsplash did not answer", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var body struct {
		Results []struct {
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
				Name  string `json:"name"`
				Links struct {
					HTML string `json:"html"`
				} `json:"links"`
			} `json:"user"`
		} `json:"results"`
		Errors []string `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, "Unsplash answered oddly", http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(r.Context(), "unsplash refused", "status", resp.StatusCode, "errors", body.Errors)
		http.Error(w, "Unsplash refused the search: "+strings.Join(body.Errors, "; "), http.StatusBadGateway)
		return
	}
	hits := make([]Hit, 0, len(body.Results))
	for _, p := range body.Results {
		hits = append(hits, Hit{
			Thumb: p.URLs.Small, URL: p.URLs.Regular, Width: p.Width, Height: p.Height,
			Title: p.Description, Source: "Unsplash", Context: p.Links.HTML, License: "Unsplash License",
			Credit: "Photo by " + p.User.Name + " on Unsplash", Download: p.Links.DownloadLocation,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// searchPexels asks Pexels for landscape photographs; its licence asks for
// nothing, and a credit line is given anyway.
func (s Search) searchPexels(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.pexels.com/v1/search?"+params.Encode(), nil)
	req.Header.Set("Authorization", s.Pexels)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "Pexels did not answer", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var body struct {
		Photos []struct {
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
		http.Error(w, "Pexels answered oddly", http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(r.Context(), "pexels refused", "status", resp.StatusCode, "error", body.Error)
		http.Error(w, "Pexels refused the search: "+body.Error, http.StatusBadGateway)
		return
	}
	hits := make([]Hit, 0, len(body.Photos))
	for _, p := range body.Photos {
		hits = append(hits, Hit{
			Thumb: p.Src.Medium, URL: p.Src.Large2x, Width: p.Width, Height: p.Height,
			Title: p.Alt, Source: "Pexels", Context: p.URL, License: "Pexels License",
			Credit: "Photo by " + p.Photographer + " on Pexels",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// searchPixabay asks Pixabay for photos, illustrations and vectors alike,
// landscape and safe; its licence asks for nothing either.
func (s Search) searchPixabay(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{
		"key": {s.Pixabay}, "q": {q}, "per_page": {"24"}, "orientation": {"horizontal"},
		"safesearch": {"true"}, "image_type": {"all"},
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://pixabay.com/api/?"+params.Encode(), nil)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "Pixabay did not answer", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		// Pixabay answers a bad key with plain text.
		slog.WarnContext(r.Context(), "pixabay refused", "status", resp.StatusCode, "body", string(raw))
		http.Error(w, "Pixabay refused the search: "+strings.TrimSpace(string(raw)), http.StatusBadGateway)
		return
	}
	var body struct {
		Hits []struct {
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
		http.Error(w, "Pixabay answered oddly", http.StatusBadGateway)
		return
	}
	hits := make([]Hit, 0, len(body.Hits))
	for _, h := range body.Hits {
		hits = append(hits, Hit{
			Thumb: h.WebformatURL, URL: h.LargeImageURL, Width: h.ImageWidth, Height: h.ImageHeight,
			Title: h.Tags, Source: "Pixabay", Context: h.PageURL, License: "Pixabay Content License",
			Credit: "Image by " + h.User + " on Pixabay",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// ServeImport fetches a picked image ({"url","download"} in the body) and
// stores it in `folder` of the store the way an upload is stored, answering
// with the name the sheet should record. A nil store (sample mode) refuses.
func (s Search) ServeImport(w http.ResponseWriter, r *http.Request, store *blob.Store, folder string, maxSize int64) {
	if store == nil {
		http.Error(w, "image uploads require real-data mode", http.StatusBadRequest)
		return
	}
	var body struct {
		URL      string `json:"url"`
		Download string `json:"download"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	// Unsplash counts a download through its own endpoint, and asks that a
	// taken photo be reported there; the answer does not matter.
	if body.Download != "" && strings.HasPrefix(body.Download, "https://api.unsplash.com/") && s.Unsplash != "" {
		go func(endpoint string) {
			req, _ := http.NewRequest("GET", endpoint, nil)
			req.Header.Set("Authorization", "Client-ID "+s.Unsplash)
			if resp, err := imageClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}(body.Download)
	}
	src, err := url.Parse(strings.TrimSpace(body.URL))
	if err != nil || (src.Scheme != "https" && src.Scheme != "http") || src.Host == "" {
		http.Error(w, "that is not a web address", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", src.String(), nil)
	req.Header.Set("User-Agent", s.UserAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "could not fetch that image", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("that site answered %d", resp.StatusCode), http.StatusBadGateway)
		return
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil || int64(len(content)) > maxSize {
		http.Error(w, "that image is too large", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, "that is not a supported image", http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := store.Put(folder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "store imported image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "imported image", "from", src.Host, "name", name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": folder + "/" + name})
}
