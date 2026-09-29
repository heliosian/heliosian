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

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/devcache"
	"heliosian/internal/env"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

const (
	batch   = 200
	workers = 8
)

var actor = access.System("importartifacts")

func main() {
	dryRun := flag.Bool("dry-run", false, "report what each file would become, embedding and writing nothing")
	permitted := flag.Bool("i-have-user-permission-to-spend-money", false, "embedding every document costs real money; pass this only when the person paying has said to run it")
	limit := flag.Int("limit", 0, "import at most this many documents; 0 for all of them")
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		log.Fatal("name the files to import: go run ./tools/importartifacts local/imports/mail/*.json local/imports/site/*.json")
	}
	if !*dryRun && !*permitted {
		log.Fatal("this run spends money embedding; pass --i-have-user-permission-to-spend-money only when the user has said to run it")
	}
	sort.Strings(files)
	devcache.Install()
	source, err := data.NewSheet(map[string]string{"artifacts": env.Required("ARTIFACTS_SHEET")})
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		log.Fatalf("%v", err)
	}
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		log.Fatalf("%v", err)
	}
	cache, err := model.NewDocumentsCache(source, source, bucket, embedder, store.NewQueue())
	if err != nil {
		log.Fatalf("load the documents on file: %v", err)
	}
	objects := map[string]string{}
	issues := map[string]string{}
	for _, doc := range cache.Model().Documents {
		objects[doc.Key] = doc.Object()
		if doc.Kind == model.DocumentKindNewsletter {
			issues[doc.Title+"|"+doc.Date] = doc.Key
		}
	}
	log.Printf("%d documents on file, %d files to consider", len(objects), len(files))
	resolver := &model.LinkResolver{}

	pending := []work{}
	drops := []store.Op{}
	dropped := []string{}
	skipped, failed, empty, withheld, characters := 0, 0, 0, 0, 0
	for _, file := range files {
		if *limit > 0 && len(pending) >= *limit {
			log.Printf("stopping at the %d asked for", *limit)
			break
		}
		saved, err := model.ReadSaved(file)
		if err != nil {
			log.Printf("%v", err)
			failed++
			continue
		}
		doc, err := saved.Build(resolver, embedder.Model())
		if errors.Is(err, model.ErrNotBroadcast) || errors.Is(err, model.ErrExcluded) {
			withheld++
			continue
		}
		if errors.Is(err, model.ErrNoWords) {
			empty++
			key := saved.Key()
			if object, known := objects[key]; known {
				drops = append(drops, cache.Model().Drop(actor, key)...)
				dropped = append(dropped, object)
				delete(objects, key)
			}
			continue
		}
		if err != nil {
			log.Printf("%s: %v", filepath.Base(file), err)
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
		} else if other, dup := issues[doc.Title+"|"+doc.Date]; dup && doc.Kind == model.DocumentKindNewsletter {
			log.Printf("%s: %q on %s is already on file as %s; skipped", filepath.Base(file), doc.Title, doc.Date, other)
			skipped++
			continue
		}
		objects[doc.Key] = doc.Object()
		if doc.Kind == model.DocumentKindNewsletter {
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
	log.Printf("%d documents to import (%d already on file, rendered differently now): %d chunks, %d characters of markdown, %d with no words to index (%d on file to drop), %d withheld, %d links dropped as unreadable redirects",
		len(pending), again, chunks, characters, empty, len(dropped), withheld, resolver.Dropped)
	if *dryRun {
		if len(pending) == 1 {
			doc := pending[0].doc
			fmt.Printf("%q on %s by %s [%s/%s], %d chunks\n\n%s\n", doc.Title, doc.Date, doc.Author, doc.Kind, doc.Channel, len(doc.Chunks), doc.Markdown)
		}
		widest := []*model.Document{}
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

	ctx := context.Background()
	if len(drops) > 0 {
		if err := cache.CommitAndWait(ctx, actor, drops...); err != nil {
			log.Fatalf("drop the documents with no words: %v", err)
		}
		for _, object := range dropped {
			if err := bucket.Remove(ctx, object); err != nil {
				log.Fatalf("drop %s: %v", object, err)
			}
		}
	}
	imported := 0
	for start := 0; start < len(pending); start += batch {
		end := min(start+batch, len(pending))
		group := pending[start:end]
		if err := embedAndStore(ctx, cache, embedder, bucket, group); err != nil {
			log.Fatalf("%v", err)
		}
		if err := record(ctx, cache, bucket, group); err != nil {
			log.Fatalf("%v", err)
		}
		imported += len(group)
		log.Printf("imported %d of %d", imported, len(pending))
	}
	fmt.Printf("\n%d files: %d imported, %d already on file, %d with no words (%d taken off the corpus), %d withheld, %d failed\n%d chunks, %d links dropped\n",
		len(files), imported, skipped, empty, len(dropped), withheld, failed, chunks, resolver.Dropped)
	report("by kind", kinds)
	report("by channel", channels)
	if failed > 0 {
		os.Exit(1)
	}
}

type work struct {
	doc       *model.Document
	replacing string
}

func record(ctx context.Context, cache *model.DocumentsCache, bucket *blob.Bucket, group []work) error {
	docs := cache.Model()
	ops := []store.Op{}
	for _, item := range group {
		if item.replacing == "" {
			ops = append(ops, docs.Record(actor, item.doc)...)
			continue
		}
		ops = append(ops, docs.Replace(actor, item.doc)...)
	}
	if err := cache.CommitAndWait(ctx, actor, ops...); err != nil {
		return fmt.Errorf("record the documents: %w", err)
	}
	for _, item := range group {
		if item.replacing == "" {
			continue
		}
		if err := bucket.Remove(ctx, item.replacing); err != nil {
			return err
		}
	}
	return nil
}

func embedAndStore(ctx context.Context, cache *model.DocumentsCache, embedder *artifacts.Vertex, bucket *blob.Bucket, group []work) error {
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
			err := put(ctx, cache, embedder, bucket, item.doc)
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

func put(ctx context.Context, cache *model.DocumentsCache, embedder *artifacts.Vertex, bucket *blob.Bucket, doc *model.Document) error {
	if err := doc.Embed(ctx, embedder); err != nil {
		return err
	}
	body, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	if err := bucket.Put(ctx, doc.Object(), "application/json", body); err != nil {
		return err
	}
	return cache.Hold(doc)
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
