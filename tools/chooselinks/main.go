package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"heliosian/internal/blob"
	"heliosian/internal/db"
	"heliosian/internal/env"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

func row(c *qclient.Client, table, id string) map[string]string {
	a, err := c.Query(map[string]any{"from": table, "where": []any{map[string]any{"=": []any{map[string]any{"path": "id"}, id}}}})
	if err != nil {
		logging.Fatal("read", "table", table, "id", id, "error", err)
	}
	if len(a.Result) != 1 {
		logging.Fatal("no such row", "table", table, "id", id)
	}
	return a.Resources[table][id]
}

func main() {
	document := flag.String("document", "", "the email's HTML part, a DOCUMENT id")
	flag.Parse()
	if *document == "" {
		logging.Fatal("--document is required")
	}
	ctx := context.Background()
	c, err := qclient.SignedIn(qclient.ImportMode, "")
	if err != nil {
		logging.Fatal("sign in", "error", err)
	}
	client := anthropic.NewClient(option.WithAPIKey(env.Required("ANTHROPIC_API_KEY")))
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		logging.Fatal("open bucket", "error", err)
	}
	part := row(c, "DOCUMENT", *document)
	root := part
	for root["parent"] != "" {
		root = row(c, "DOCUMENT", root["parent"])
	}
	raw, _, err := bucket.Get(ctx, row(c, "CONTENT", part["content"])["blob"])
	if err != nil {
		logging.Fatal("read html", "error", err)
	}
	choices, err := db.ChooseLinks(ctx, client, db.LinkEmail{Subject: root["name"], Kind: root["kind"], Sent: root["published"]}, raw)
	if err != nil {
		logging.Fatal("choose", "error", err)
	}
	fmt.Printf("%s (%s, %s): %d links\n\n", root["name"], root["kind"], root["published"], len(choices))
	for i, l := range choices {
		verdict := "FETCH"
		if l.Skip != "" {
			verdict = "skip: " + l.Skip
		}
		fmt.Printf("%3d %-19s %s\n    words: %s\n    sentence: %s\n", i+1, verdict, l.URL, l.Text, l.Context)
	}
}
