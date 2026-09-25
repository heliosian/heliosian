package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("[ERROR] give the question to search for")
	}
	query := strings.Join(os.Args[1:], " ")
	sheetID := os.Getenv("ARTIFACTS_SHEET")
	if sheetID == "" {
		log.Fatal("[ERROR] ARTIFACTS_SHEET is required")
	}
	source, err := data.NewSheet(map[string]string{"artifacts": sheetID})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	reader, err := blob.New("local/cache/blobs")
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	start := time.Now()
	cache, err := artifacts.NewCache(source, nil, reader, embedder, store.NewQueue())
	if err != nil {
		log.Fatalf("[ERROR] load: %v", err)
	}
	model := cache.Model()
	oldest, newest := model.Span()
	fmt.Printf("%d documents, %d chunks, %s to %s, loaded in %s\n",
		len(model.Documents), model.Chunks(), oldest, newest, time.Since(start).Round(time.Millisecond))
	vectors, err := embedder.Embed(context.Background(), []string{query}, true)
	if err != nil {
		log.Fatalf("[ERROR] embed: %v", err)
	}
	for _, hit := range model.Search(vectors[0], query, 6) {
		d, c := hit.Document, hit.Document.Chunks[hit.Index]
		section := c.Section
		if section != "" {
			section = " › " + section
		}
		fmt.Printf("\n%.3f  %s (%s) [%s]%s\n%s\n", hit.Score, d.Title, d.Date, d.Channel, section, c.Text)
	}
}
