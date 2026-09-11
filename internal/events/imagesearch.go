package events

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
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ImageSearch is an optional Google Programmable Search Engine to search
// images with: an API key for the Custom Search JSON API and the engine's id.
// Without one the portal searches Wikimedia Commons instead, which needs no
// key and holds free-to-use pictures - flags, foods, places, holidays - which
// suits a school better than the open web anyway. (Google has stopped
// granting new projects access to the JSON API, so Commons is the usual path.)
type ImageSearch struct {
	Key string
	CX  string
	// Unsplash is an Unsplash API access key: free stock photographs, which
	// suit an event's card better than Commons' flags and clip art. Its terms
	// ask for a credit line and a ping of the photo's download endpoint when
	// one is taken, and both are kept here.
	Unsplash string
	// Pexels and Pixabay are two more free stock libraries, each behind its
	// own key; Pixabay also carries illustrations and vectors, which the
	// photo libraries lack.
	Pexels  string
	Pixabay string
}

func (s ImageSearch) google() bool {
	return s.Key != "" && s.CX != ""
}

// Sources lists where the portal can look, in the order the picker offers them.
func (s ImageSearch) Sources() []string {
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
	if s.google() {
		out = append(out, "Google Images")
	}
	return append(out, "Wikimedia Commons")
}

// ImageHit is one result as the picker shows it: a thumbnail to show, the
// full image to import, where it came from, and its licence when known.
type ImageHit struct {
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

// Commons names a rendering by its width - ".../1920px-Flag.svg.png" - so a
// smaller one for the grid is the same address with the width swapped.
var thumbWidth = regexp.MustCompile(`/\d+px-`)

const userAgent = "HCA-Team image search (+https://hca.heliosian.com)"

// searchImages answers one image search from whichever source is set up,
// keeping any key on the server. SafeSearch is always on; this is a school.
func (a app) searchImages(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "say what to search for", http.StatusBadRequest)
		return
	}
	switch source := r.URL.Query().Get("source"); {
	case source == "Unsplash" && a.search.Unsplash != "":
		a.searchUnsplash(w, r, q)
		return
	case source == "Pexels" && a.search.Pexels != "":
		a.searchPexels(w, r, q)
		return
	case source == "Pixabay" && a.search.Pixabay != "":
		a.searchPixabay(w, r, q)
		return
	case source == "Google Images" && a.search.google():
	case source == "Wikimedia Commons" || !a.search.google():
		a.searchCommons(w, r, q)
		return
	}
	params := url.Values{
		"key": {a.search.Key}, "cx": {a.search.CX}, "q": {q},
		"searchType": {"image"}, "num": {"10"}, "safe": {"active"}, "imgSize": {"large"},
	}
	if start := r.URL.Query().Get("start"); start != "" {
		params.Set("start", start)
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://www.googleapis.com/customsearch/v1?"+params.Encode(), nil)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "the search did not answer", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var body struct {
		Items []struct {
			Title       string `json:"title"`
			Link        string `json:"link"`
			DisplayLink string `json:"displayLink"`
			Image       struct {
				ThumbnailLink string `json:"thumbnailLink"`
				ContextLink   string `json:"contextLink"`
				Width         int    `json:"width"`
				Height        int    `json:"height"`
			} `json:"image"`
		} `json:"items"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		http.Error(w, "the search answered oddly", http.StatusBadGateway)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.WarnContext(r.Context(), "image search refused", "status", resp.StatusCode, "message", body.Error.Message)
		http.Error(w, "the search was refused: "+body.Error.Message, http.StatusBadGateway)
		return
	}
	hits := []ImageHit{}
	for _, it := range body.Items {
		hits = append(hits, ImageHit{
			Thumb: it.Image.ThumbnailLink, URL: it.Link, Width: it.Image.Width, Height: it.Image.Height,
			Title: it.Title, Source: it.DisplayLink, Context: it.Image.ContextLink,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// searchUnsplash asks Unsplash for photographs matching the words: landscape
// ones, filtered for a school, each with its photographer for the credit line
// and its download endpoint for the ping.
func (a app) searchUnsplash(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}, "content_filter": {"high"}}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.unsplash.com/search/photos?"+params.Encode(), nil)
	req.Header.Set("Authorization", "Client-ID "+a.search.Unsplash)
	req.Header.Set("Accept-Version", "v1")
	req.Header.Set("User-Agent", userAgent)
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
	hits := make([]ImageHit, 0, len(body.Results))
	for _, p := range body.Results {
		hits = append(hits, ImageHit{
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
func (a app) searchPexels(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{"query": {q}, "per_page": {"24"}, "orientation": {"landscape"}}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.pexels.com/v1/search?"+params.Encode(), nil)
	req.Header.Set("Authorization", a.search.Pexels)
	req.Header.Set("User-Agent", userAgent)
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
	hits := make([]ImageHit, 0, len(body.Photos))
	for _, p := range body.Photos {
		hits = append(hits, ImageHit{
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
func (a app) searchPixabay(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{
		"key": {a.search.Pixabay}, "q": {q}, "per_page": {"24"}, "orientation": {"horizontal"},
		"safesearch": {"true"}, "image_type": {"all"},
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://pixabay.com/api/?"+params.Encode(), nil)
	req.Header.Set("User-Agent", userAgent)
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
	hits := make([]ImageHit, 0, len(body.Hits))
	for _, h := range body.Hits {
		hits = append(hits, ImageHit{
			Thumb: h.WebformatURL, URL: h.LargeImageURL, Width: h.ImageWidth, Height: h.ImageHeight,
			Title: h.Tags, Source: "Pixabay", Context: h.PageURL, License: "Pixabay Content License",
			Credit: "Image by " + h.User + " on Pixabay",
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// searchCommons asks Wikimedia Commons for files matching the words: the
// MediaWiki search over the File namespace, with each file's size, licence
// and a 1600px rendering - which is a PNG even for an SVG, so a flag imports
// as a picture the site can show. The thumbnail is the same rendering at
// 400px, which Commons names by width.
func (a app) searchCommons(w http.ResponseWriter, r *http.Request, q string) {
	params := url.Values{
		"action": {"query"}, "format": {"json"}, "generator": {"search"},
		"gsrsearch": {q}, "gsrnamespace": {"6"}, "gsrlimit": {"24"},
		"prop": {"imageinfo"}, "iiprop": {"url|size|mime|extmetadata"}, "iiurlwidth": {"1600"},
		"iiextmetadatafilter": {"LicenseShortName"},
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://commons.wikimedia.org/w/api.php?"+params.Encode(), nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := imageClient.Do(req)
	if err != nil {
		http.Error(w, "Wikimedia Commons did not answer", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var body struct {
		Query struct {
			Pages map[string]struct {
				Title     string `json:"title"`
				Index     int    `json:"index"`
				ImageInfo []struct {
					URL            string `json:"url"`
					DescriptionURL string `json:"descriptionurl"`
					ThumbURL       string `json:"thumburl"`
					Width          int    `json:"width"`
					Height         int    `json:"height"`
					Mime           string `json:"mime"`
					ExtMetadata    map[string]struct {
						Value string `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		http.Error(w, "Wikimedia Commons answered oddly", http.StatusBadGateway)
		return
	}
	type ranked struct {
		index int
		hit   ImageHit
	}
	found := []ranked{}
	for _, page := range body.Query.Pages {
		if len(page.ImageInfo) == 0 {
			continue
		}
		ii := page.ImageInfo[0]
		if !strings.HasPrefix(ii.Mime, "image/") || ii.ThumbURL == "" {
			continue
		}
		// Only what the site can store: the rendering is a PNG or JPEG.
		hit := ImageHit{
			Thumb: thumbWidth.ReplaceAllString(ii.ThumbURL, "/400px-"), URL: ii.ThumbURL,
			Width: ii.Width, Height: ii.Height,
			Title:  strings.TrimSuffix(strings.TrimPrefix(page.Title, "File:"), path.Ext(page.Title)),
			Source: "Wikimedia Commons", Context: ii.DescriptionURL,
		}
		if l, ok := ii.ExtMetadata["LicenseShortName"]; ok {
			hit.License = l.Value
		}
		found = append(found, ranked{page.Index, hit})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].index < found[j].index })
	hits := make([]ImageHit, 0, len(found))
	for _, f := range found {
		hits = append(hits, f.hit)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hits)
}

// importImage fetches a picked image from the web and stores it the way an
// upload is stored, answering with the name the sheet should record.
func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		http.Error(w, "image uploads require real-data mode", http.StatusBadRequest)
		return
	}
	var body struct {
		URL      string `json:"url"`
		Download string `json:"download"`
	}
	if !decode(w, r, &body) {
		return
	}
	// Unsplash counts a download through its own endpoint, and asks that a
	// taken photo be reported there; the answer does not matter.
	if body.Download != "" && strings.HasPrefix(body.Download, "https://api.unsplash.com/") && a.search.Unsplash != "" {
		go func(endpoint string) {
			req, _ := http.NewRequest("GET", endpoint, nil)
			req.Header.Set("Authorization", "Client-ID "+a.search.Unsplash)
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
	req.Header.Set("User-Agent", userAgent)
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
	content, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize+1))
	if err != nil || len(content) > maxImageSize {
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
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "store imported image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	slog.InfoContext(r.Context(), "events: imported image", "from", src.Host, "name", name)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name})
}
