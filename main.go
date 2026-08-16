// Heliosian serves the Helios school community apps.
package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/geocode"
)

func directorySource() data.Source {
	sheetID := os.Getenv("DIRECTORY_SHEET")
	if sheetID == "" {
		return data.Dir{Root: "sampledata"}
	}
	source, err := data.NewSheet(map[string]string{"directory": sheetID})
	if err != nil {
		log.Fatalf("[ERROR] load directory sheet: %v", err)
	}
	return source
}

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

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
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
	authn := auth.New(clientID(), sessionKey())
	serverKey := mapsKey("GOOGLE_MAPS_SERVER_KEY", "creds/geocoding.key")
	browserKey := mapsKey("GOOGLE_MAPS_BROWSER_KEY", "creds/maps.key")
	source := directorySource()
	cache, err := directory.NewCache(source, geocode.New(serverKey))
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	authn.Register(mux)
	directory.Register(mux, cache, browserKey)
	if os.Getenv("DIRECTORY_SHEET") != "" {
		store, err := blob.New()
		if err != nil {
			log.Fatalf("[ERROR] blob store: %v", err)
		}
		blob.Register(mux, store)
		directory.RegisterUpload(mux, cache, source.(*data.Sheet), store)
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, noCache(authn.Wrap(mux))))
}
