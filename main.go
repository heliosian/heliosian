// Heliosian serves the Helios school community apps.
package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/directory"
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

func main() {
	authn := auth.New(clientID(), sessionKey())
	cache, err := directory.NewCache(directorySource())
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	authn.Register(mux)
	directory.Register(mux, cache)
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, authn.Wrap(mux)))
}
