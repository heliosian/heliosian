// Heliosian serves the Helios school community apps.
package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/geocode"
)

const sampleUser = "jordan.whitfield@heliosschool.org"

func sessionKey() []byte {
	if key := os.Getenv("SESSION_KEY"); key != "" {
		return []byte(key)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatalf("[ERROR] generate session key: %v", err)
	}
	log.Printf("SESSION_KEY not set, using a random key; sessions reset on restart")
	return key
}

func clientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("creds/oauth-client.json")
	if err != nil {
		log.Fatalf("[ERROR] read creds/oauth-client.json (or set GOOGLE_CLIENT_ID): %v", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		log.Fatal("[ERROR] creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/fonts/") || strings.HasPrefix(r.URL.Path, "/static/brand/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func mapsKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		log.Fatalf("[ERROR] read %s (or set %s): %v", file, envName, err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		log.Fatalf("[ERROR] %s is empty", file)
	}
	return key
}

func main() {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Fatalf("[ERROR] register manifest mime type: %v", err)
	}
	sheetID := os.Getenv("DIRECTORY_SHEET")
	var source data.Source
	var geocoder directory.Geocoder = geocode.Fake{}
	browserKey := os.Getenv("GOOGLE_MAPS_BROWSER_KEY")
	var store *blob.Store
	var blobs directory.BlobChecker
	if sheetID == "" {
		source = data.Dir{Root: "sampledata"}
		log.Printf("DIRECTORY_SHEET not set, serving sample data as %s", sampleUser)
	} else {
		preferencesID := os.Getenv("PREFERENCES_SHEET")
		if preferencesID == "" {
			log.Fatal("[ERROR] PREFERENCES_SHEET is required alongside DIRECTORY_SHEET")
		}
		sheet, err := data.NewSheet(map[string]string{"directory": sheetID, "preferences": preferencesID})
		if err != nil {
			log.Fatalf("[ERROR] load directory sheet: %v", err)
		}
		source = sheet
		geocoder = geocode.New(mapsKey("GOOGLE_MAPS_SERVER_KEY", "creds/geocoding.key"))
		browserKey = mapsKey("GOOGLE_MAPS_BROWSER_KEY", "creds/maps.key")
		store, err = blob.New()
		if err != nil {
			log.Fatalf("[ERROR] blob store: %v", err)
		}
		blobs = store
	}
	cache, err := directory.NewCache(source, geocoder, blobs, staticFiles{})
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	directory.Register(mux, cache, browserKey)
	if store != nil {
		blob.Register(mux, store)
		directory.RegisterUpload(mux, cache, source.(*data.Sheet), store)
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	var handler http.Handler = directory.MemberGate(cache, mux)
	if sheetID == "" {
		mux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
		handler = auth.Fixed(sampleUser, handler)
	} else {
		authn := auth.New(clientID(), sessionKey())
		authn.Register(mux)
		handler = authn.Wrap(handler)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Addr: ":" + port, Handler: cacheControl(handler), Protocols: protocols}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(server.ListenAndServe())
}
