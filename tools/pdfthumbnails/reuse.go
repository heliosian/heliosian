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

var errStaleDocument = errors.New("document content changed")

func reuse(c *qclient.Client, id, content, source string) error {
	started, logged := time.Now(), time.Now()
	for {
		ready, err := reuseOnce(c, id, content, source)
		if err != nil || ready {
			return err
		}
		if time.Since(started) >= thumbnailWait {
			return fmt.Errorf("%w: source %s pages", errNoThumbnail, source)
		}
		if time.Since(logged) >= time.Minute {
			slog.Info("waiting for the source's page thumbnails", "document", id, "source", source)
			logged = time.Now()
		}
		time.Sleep(thumbnailPoll)
	}
}

func reuseOnce(c *qclient.Client, id, content, source string) (bool, error) {
	a, err := c.Query([]any{"from", "DOCUMENT",
		[]any{"where", []any{"or", []any{"in", map[string]any{"n": "id"}, id, source}, []any{"and", []any{"in", map[string]any{"n": "parent"}, id, source}, []any{"=", map[string]any{"n": "relation"}, "page"}}}},
		[]any{"columns", "parent", "relation", "content", "thumbnail", "thumbnailed", "rendered", "page_count", "first_page", "last_page", "name", "order", "extracted"}})
	if err != nil {
		return false, err
	}
	docs := a.Resources["DOCUMENT"]
	from, to := docs[source], docs[id]
	if content == "" || from["content"] != content || to["content"] != content {
		return false, fmt.Errorf("%w: documents %s and %s no longer hold pdf %s", errStaleDocument, source, id, content)
	}
	total, err := strconv.Atoi(from["page_count"])
	if err != nil || total < 1 || from["rendered"] == "" || from["thumbnail"] == "" {
		return false, fmt.Errorf("source %s does not have complete pages and a thumbnail", source)
	}
	pages := map[string]map[string]string{}
	for _, child := range docs {
		if child["parent"] == source && child["relation"] == "page" {
			n := child["first_page"]
			if pages[n] != nil {
				return false, fmt.Errorf("source %s has duplicate page %s", source, n)
			}
			pages[n] = child
		}
	}
	if len(pages) != total {
		return false, fmt.Errorf("source %s has %d pages, expected %d", source, len(pages), total)
	}
	for n := 1; n <= total; n++ {
		page := pages[strconv.Itoa(n)]
		if page["content"] == "" || page["last_page"] != strconv.Itoa(n) {
			return false, fmt.Errorf("source %s is missing page %d", source, n)
		}
		if page["thumbnail"] == "" && page["thumbnailed"] != "" {
			return false, fmt.Errorf("%w: source %s page %d was thumbnailed without a picture", errNoThumbnail, source, n)
		}
		if page["thumbnail"] == "" {
			return false, nil
		}
	}
	if to["page_count"] != "" && to["page_count"] != from["page_count"] {
		return false, fmt.Errorf("document %s has a different page count", id)
	}
	seen := map[string]bool{}
	for _, child := range docs {
		if child["parent"] != id || child["relation"] != "page" {
			continue
		}
		n := child["first_page"]
		if seen[n] || child["last_page"] != n || child["content"] != pages[n]["content"] {
			return false, fmt.Errorf("document %s page %s holds different pixels", id, n)
		}
		seen[n] = true
	}
	writes := []db.Edit{}
	for n := 1; n <= total; n++ {
		number := strconv.Itoa(n)
		if seen[number] {
			continue
		}
		page := pages[number]
		writes = append(writes, db.Edit{Insert: "DOCUMENT", Row: map[string]any{"parent": id, "relation": "page", "content": page["content"], "first_page": number, "last_page": number, "name": page["name"], "order": page["order"], "extracted": page["extracted"], "thumbnail": page["thumbnail"], "thumbnailed": page["thumbnailed"]}})
	}
	set := map[string]any{}
	if to["rendered"] == "" {
		set["page_count"], set["rendered"] = from["page_count"], from["rendered"]
	}
	if to["thumbnail"] == "" {
		set["thumbnail"], set["thumbnailed"] = from["thumbnail"], from["thumbnailed"]
	}
	if len(set) > 0 {
		writes = append(writes, db.Edit{Set: id, Cells: set})
	}
	if len(writes) == 0 {
		return true, nil
	}
	_, err = c.Write(db.Batch{Batch: writes})
	return err == nil, err
}
