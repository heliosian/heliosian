package main

import (
	"context"
	"log"
	"slices"

	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
)

const (
	systemTokens    = 250
	outputTokens    = 250
	inputDollars    = 2.0
	outputDollars   = 10.0
	charsPerToken   = 4
	existsParallels = 16
)

func main() {
	sheet, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	s, err := db.NewStore(sheet, nil, store.NewQueue(), db.NewSearchIndex())
	if err != nil {
		log.Fatalf("load the data sheets: %v", err)
	}
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		log.Fatalf("media bucket: %v", err)
	}
	m := s.Model()
	texts, err := db.SearchTexts(context.Background(), m, bucket, map[string]string{})
	if err != nil {
		log.Fatalf("read the extracts: %v", err)
	}
	rows := m.SearchInputs(texts)
	inputs := map[string]string{}
	tables := map[string]map[string]bool{}
	for _, r := range rows {
		inputs[r.Object] = r.Input
		if tables[r.Table] == nil {
			tables[r.Table] = map[string]bool{}
		}
		tables[r.Table][r.Object] = true
	}
	objects := []string{}
	for object := range inputs {
		objects = append(objects, object)
	}
	slices.Sort(objects)
	held, errs := db.FanOut(len(objects), func(i int) (bool, error) {
		limit <- struct{}{}
		defer func() { <-limit }()
		return bucket.Exists(context.Background(), objects[i])
	})
	missing, chars := 0, 0
	byTable := map[string]int{}
	for i, object := range objects {
		if errs[i] != nil {
			log.Fatalf("look for %s: %v", object, errs[i])
		}
		if held[i] {
			continue
		}
		missing++
		chars += len(inputs[object])
		for table, set := range tables {
			if set[object] {
				byTable[table]++
			}
		}
	}
	in := missing*systemTokens + chars/charsPerToken
	out := missing * outputTokens
	dollars := float64(in)*inputDollars/1e6 + float64(out)*outputDollars/1e6
	log.Printf("%d rows, %d distinct inputs, %d already in the bucket, %d to make: %v", len(rows), len(objects), len(objects)-missing, missing, byTable)
	log.Printf("claude %s: about %d input and %d output tokens (assuming %d out per row, thinking included), about $%.2f", claude.SearchSummaryModel, in, out, outputTokens, dollars)
	log.Printf("vertex: about %d tokens to embed", chars/charsPerToken)
}

var limit = make(chan struct{}, existsParallels)
