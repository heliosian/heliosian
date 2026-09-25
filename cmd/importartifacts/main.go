package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
)

const (
	batch   = 200
	workers = 8
)

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func main() {
	dryRun := flag.Bool("dry-run", false, "report what each file would become, embedding and writing nothing")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "embedding every document costs real money; pass this only when the person paying has said to run it")
	limit := flag.Int("limit", 0, "import at most this many documents; 0 for all of them")
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		log.Fatal("[ERROR] name the files to import: go run ./cmd/importartifacts local/imports/mail/*.json local/imports/site/*.json")
	}
	if !*dryRun && !*permitted {
		log.Fatal("[ERROR] this run spends money embedding; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	sort.Strings(files)
	source, err := data.NewSheet(map[string]string{"artifacts": requiredEnv("ARTIFACTS_SHEET")})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	header, err := source.Header("artifacts", "Documents")
	if err != nil {
		log.Fatalf("[ERROR] read the Documents header: %v", err)
	}
	rows, err := artifacts.ReadRows(source)
	if err != nil {
		log.Fatalf("[ERROR] read the Documents tab: %v", err)
	}
	objects := map[string]string{}
	issues := map[string]string{}
	for _, row := range rows {
		objects[row["Key"]] = row["Object"]
		if row["Kind"] == artifacts.KindNewsletter {
			issues[row["Title"]+"|"+row["Date"]] = row["Key"]
		}
	}
	log.Printf("%d documents on file, %d files to consider", len(rows), len(files))
	embedder, err := artifacts.NewVertex()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	uploader, err := blob.NewUploader()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	resolver := artifacts.NewResolver()

	pending := []work{}
	skipped, failed, empty, dropped, withheld, characters := 0, 0, 0, 0, 0, 0
	for _, file := range files {
		if *limit > 0 && len(pending) >= *limit {
			log.Printf("stopping at the %d asked for", *limit)
			break
		}
		saved, err := artifacts.ReadSaved(file)
		if err != nil {
			log.Printf("[ERROR] %v", err)
			failed++
			continue
		}
		doc, err := saved.Build(resolver, embedder.Model())
		if errors.Is(err, artifacts.ErrNotBroadcast) || errors.Is(err, artifacts.ErrExcluded) {
			withheld++
			continue
		}
		if errors.Is(err, artifacts.ErrNoWords) {
			empty++
			key := saved.Key()
			if object, known := objects[key]; known && !*dryRun {
				if err := source.Delete("artifacts", "Documents", map[string]string{"Key": key}); err != nil {
					log.Fatalf("[ERROR] drop the row for %s: %v", key, err)
				}
				if err := uploader.Remove(object); err != nil {
					log.Fatalf("[ERROR] drop the object for %s: %v", key, err)
				}
				delete(objects, key)
				dropped++
			}
			continue
		}
		if err != nil {
			log.Printf("[ERROR] %s: %v", filepath.Base(file), err)
			failed++
			continue
		}
		item := work{doc: doc}
		if object, known := objects[doc.Key]; known {
			if object == doc.Object() {
				skipped++
				continue
			}
			item.replacing = object
		} else if other, dup := issues[doc.Title+"|"+doc.Date]; dup && doc.Kind == artifacts.KindNewsletter {
			log.Printf("%s: %q on %s is already on file as %s; skipped", filepath.Base(file), doc.Title, doc.Date, other)
			skipped++
			continue
		}
		objects[doc.Key] = doc.Object()
		if doc.Kind == artifacts.KindNewsletter {
			issues[doc.Title+"|"+doc.Date] = doc.Key
		}
		characters += len(doc.Markdown)
		pending = append(pending, item)
		if len(pending)%500 == 0 {
			log.Printf("read %d of %d", len(pending), len(files))
		}
	}
	chunks, again := 0, 0
	kinds := map[string]int{}
	channels := map[string]int{}
	for _, item := range pending {
		chunks += len(item.doc.Chunks)
		kinds[item.doc.Kind]++
		channels[item.doc.Channel]++
		if item.replacing != "" {
			again++
		}
	}
	log.Printf("%d documents to import (%d already on file, rendered differently now): %d chunks, %d characters of markdown, %d with no words to index, %d withheld, %d links dropped as unreadable redirects",
		len(pending), again, chunks, characters, empty, withheld, resolver.Dropped)
	if *dryRun {
		if len(pending) == 1 {
			doc := pending[0].doc
			fmt.Printf("%q on %s by %s [%s/%s], %d chunks\n\n%s\n", doc.Title, doc.Date, doc.Author, doc.Kind, doc.Channel, len(doc.Chunks), doc.Markdown)
		}
		widest := []*artifacts.Document{}
		for _, item := range pending {
			widest = append(widest, item.doc)
		}
		sort.Slice(widest, func(i, j int) bool { return len(widest[i].Chunks) > len(widest[j].Chunks) })
		fmt.Printf("\nthe widest:\n")
		for _, doc := range widest[:min(10, len(widest))] {
			fmt.Printf("  %3d chunks  %s  %s [%s]\n", len(doc.Chunks), doc.Date, doc.Title, doc.Channel)
		}
		report("by kind", kinds)
		report("by channel", channels)
		fmt.Printf("\n%d files: %d would be imported, %d already on file, %d with no words, %d withheld, %d failed\n", len(files), len(pending), skipped, empty, withheld, failed)
		return
	}

	imported := 0
	for start := 0; start < len(pending); start += batch {
		end := min(start+batch, len(pending))
		group := pending[start:end]
		if err := embedAndStore(embedder, uploader, group); err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		if err := record(source, uploader, header, group); err != nil {
			log.Fatalf("[ERROR] %v", err)
		}
		imported += len(group)
		log.Printf("imported %d of %d (%.1fM characters embedded)", imported, len(pending), float64(artifacts.Billed)/1e6)
	}
	fmt.Printf("\n%d files: %d imported, %d already on file, %d with no words (%d taken off the corpus), %d withheld, %d failed\n%d chunks, %.1fM characters embedded, %d links dropped\n",
		len(files), imported, skipped, empty, dropped, withheld, failed, chunks, float64(artifacts.Billed)/1e6, resolver.Dropped)
	report("by kind", kinds)
	report("by channel", channels)
	if failed > 0 {
		os.Exit(1)
	}
}

type work struct {
	doc       *artifacts.Document
	replacing string
}

func record(source *data.Sheet, uploader *blob.Uploader, header []string, group []work) error {
	fresh := []work{}
	updates := map[string]map[string]string{}
	for _, item := range group {
		if item.replacing == "" {
			fresh = append(fresh, item)
			continue
		}
		updates[item.doc.Key] = item.doc.Row()
	}
	if len(fresh) > 0 {
		if err := appendRows(source, header, fresh); err != nil {
			return fmt.Errorf("write the Documents rows: %w", err)
		}
	}
	if len(updates) > 0 {
		if err := source.SetMany("artifacts", "Documents", "Key", updates); err != nil {
			return fmt.Errorf("update the Documents rows: %w", err)
		}
	}
	for _, item := range group {
		if item.replacing == "" {
			continue
		}
		if err := uploader.Remove(item.replacing); err != nil {
			return err
		}
	}
	return nil
}

func embedAndStore(embedder artifacts.Embedder, uploader *blob.Uploader, group []work) error {
	var mu sync.Mutex
	var first error
	var wg sync.WaitGroup
	slots := make(chan struct{}, workers)
	for _, item := range group {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			err := store(embedder, uploader, item.doc)
			if err == nil {
				return
			}
			mu.Lock()
			if first == nil {
				first = fmt.Errorf("%s: %w", item.doc.Key, err)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return first
}

func store(embedder artifacts.Embedder, uploader *blob.Uploader, doc *artifacts.Document) error {
	if err := doc.Embed(context.Background(), embedder); err != nil {
		return err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if _, err := uploader.Put(artifacts.Folder, doc.ObjectFile(), "application/json", body); err != nil {
		return err
	}
	for i := range doc.Chunks {
		doc.Chunks[i].Vector = nil
	}
	return nil
}

func appendRows(source *data.Sheet, header []string, group []work) error {
	rows := []map[string]string{}
	for _, item := range group {
		row := item.doc.Row()
		cells := map[string]string{}
		for _, column := range header {
			cells[column] = row[column]
		}
		rows = append(rows, cells)
	}
	return source.Insert("artifacts", "Documents", rows)
}

func report(what string, counts map[string]int) {
	keys := []string{}
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	fmt.Printf("\n%s:\n", what)
	for _, key := range keys {
		fmt.Printf("  %6d  %s\n", counts[key], key)
	}
}
