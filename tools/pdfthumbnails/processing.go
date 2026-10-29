package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"heliosian/internal/db"
	"heliosian/internal/qclient"
)

const (
	thumbnailPoll = 2 * time.Second
	thumbnailWait = 15 * time.Minute
)

var errNoThumbnail = errors.New("the server made no thumbnail")

func completePDFThumbnail(c *qclient.Client, id, content string) error {
	started, logged := time.Now(), time.Now()
	for {
		a, err := c.Query([]any{"from", "DOCUMENT",
			[]any{"where", []any{"or", []any{"=", map[string]any{"n": "id"}, id}, []any{"and", []any{"=", map[string]any{"n": "parent"}, id}, []any{"=", map[string]any{"n": "relation"}, "page"}}}},
			[]any{"columns", "content", "parent", "page_count", "first_page", "thumbnail", "thumbnailed"}})
		if err != nil {
			return err
		}
		docs := a.Resources["DOCUMENT"]
		if docs[id]["content"] != content {
			return fmt.Errorf("%w: document %s no longer holds pdf %s", errStaleDocument, id, content)
		}
		total, err := strconv.Atoi(docs[id]["page_count"])
		if err != nil || total < 1 {
			return fmt.Errorf("document %s has no page count", id)
		}
		pages := map[string]map[string]string{}
		for _, doc := range docs {
			if doc["parent"] == id {
				pages[doc["first_page"]] = doc
			}
		}
		waiting := 0
		for n := 1; n <= total; n++ {
			page := pages[strconv.Itoa(n)]
			if page == nil {
				return fmt.Errorf("document %s has no page %d", id, n)
			}
			if page["thumbnail"] == "" && page["thumbnailed"] != "" {
				return fmt.Errorf("%w: document %s page %d was thumbnailed without a picture", errNoThumbnail, id, n)
			}
			if page["thumbnail"] == "" {
				waiting++
			}
		}
		if waiting == 0 {
			if docs[id]["thumbnail"] != "" {
				return nil
			}
			first := pages["1"]
			_, err := c.Write(db.Batch{Batch: []db.Edit{{Set: id, Cells: map[string]any{"thumbnail": first["thumbnail"], "thumbnailed": first["thumbnailed"]}}}})
			return err
		}
		if time.Since(started) >= thumbnailWait {
			return fmt.Errorf("%w in %s: document %s, %d of %d pages", errNoThumbnail, thumbnailWait, id, waiting, total)
		}
		if time.Since(logged) >= time.Minute {
			slog.Info("waiting for page thumbnails", "document", id, "pending", waiting, "of", total)
			logged = time.Now()
		}
		time.Sleep(thumbnailPoll)
	}
}
