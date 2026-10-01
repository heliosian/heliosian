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
	"heliosian/internal/devcache"
	"heliosian/internal/env"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("give the question to search for")
	}
	query := strings.Join(os.Args[1:], " ")
	devcache.Install()
	source, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	reader, err := blob.Open(blob.MediaBucket)
	if err != nil {
		log.Fatalf("%v", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		log.Fatalf("%v", err)
	}
	start := time.Now()
	models, err := model.NewStore(source, nil, store.NewQueue(), model.Deps{IDKey: []byte(env.Required("ID_KEY")), Static: static.Files{Root: "web/who"}, Objects: reader, Embedder: embedder})
	if err != nil {
		log.Fatalf("load: %v", err)
	}
	docs := models.Model().Documents
	oldest, newest := docs.Span()
	fmt.Printf("%d documents, %d chunks, %s to %s, loaded in %s\n",
		len(docs.Documents), docs.Chunks(), oldest, newest, time.Since(start).Round(time.Millisecond))
	vectors, err := embedder.Embed(context.Background(), []string{query}, true)
	if err != nil {
		log.Fatalf("embed: %v", err)
	}
	for _, hit := range docs.Search(vectors[0], query, 6, func(*model.Document, int) bool { return true }) {
		d, c := hit.Document, hit.Document.Chunks[hit.Index]
		section := c.Section
		if section != "" {
			section = " › " + section
		}
		fmt.Printf("\n%.3f  %s (%s) [%s]%s\n%s\n", hit.Score, d.Title, d.Date, d.Channel, section, c.Text)
	}
}
