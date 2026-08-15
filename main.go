// Heliosian serves the Helios school community apps.
package main

import (
	"log"
	"net/http"
	"os"

	"heliosian/internal/data"
	"heliosian/internal/directory"
)

func main() {
	mux := http.NewServeMux()
	directory.Register(mux, data.Dir{Root: "sampledata"})
	mux.Handle("GET /{$}", http.RedirectHandler("/directory/", http.StatusFound))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
