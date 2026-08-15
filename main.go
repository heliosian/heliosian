// Heliosian serves the Helios school community apps.
package main

import (
	"log"
	"net/http"
	"os"

	"heliosian/internal/data"
	"heliosian/internal/directory"
)

func directorySource() data.Source {
	sheetID := os.Getenv("DIRECTORY_SHEET")
	if sheetID == "" {
		return data.Dir{Root: "sampledata"}
	}
	keyFile, err := data.KeyFile()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	source, err := data.NewSheet(keyFile, map[string]string{"directory": sheetID})
	if err != nil {
		log.Fatalf("[ERROR] load directory sheet: %v", err)
	}
	return source
}

func main() {
	cache, err := directory.NewCache(directorySource())
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	directory.Register(mux, cache)
	mux.Handle("GET /{$}", http.RedirectHandler("/directory/", http.StatusFound))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
