package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"heliosian/internal/db"
	"heliosian/internal/qclient"
)

func waitForProcessing(c *qclient.Client) error {
	deadline := time.Now().Add(5 * time.Minute)
	last := ""
	for {
		var report struct {
			Queues []struct {
				Name    string `json:"name"`
				Pending int    `json:"pending"`
			} `json:"queues"`
		}
		if err := c.Send(http.MethodGet, "/api/queues", "", strings.NewReader(""), &report); err != nil {
			return fmt.Errorf("read processing queues: %w", err)
		}
		found := map[string]bool{}
		pending := []string{}
		for _, queue := range report.Queues {
			switch queue.Name {
			case "thumbnails", "search indexing", "search processing", "pending writes":
				found[queue.Name] = true
				if queue.Pending > 0 {
					pending = append(pending, fmt.Sprintf("%s=%d", queue.Name, queue.Pending))
				}
			}
		}
		if len(found) != 4 {
			return fmt.Errorf("server queue report lacks thumbnail, search or write processing status")
		}
		if len(pending) == 0 {
			if last != "" {
				slog.Info("processing drained")
			}
			return nil
		}
		status := strings.Join(pending, ", ")
		if time.Now().After(deadline) {
			return fmt.Errorf("processing did not drain within five minutes: %s", status)
		}
		if status != last {
			slog.Info("waiting for processing", "pending", status)
			last = status
		}
		time.Sleep(time.Second)
	}
}

func completePDFThumbnail(c *qclient.Client, id, content string) error {
	a, err := c.Query([]any{"from", "DOCUMENT",
		[]any{"where", []any{"or", []any{"=", map[string]any{"n": "id"}, id}, []any{"and", []any{"=", map[string]any{"n": "parent"}, id}, []any{"=", map[string]any{"n": "relation"}, "page"}, []any{"=", map[string]any{"n": "first_page"}, 1}}}},
		[]any{"columns", "content", "parent", "thumbnail", "thumbnailed"}})
	if err != nil {
		return err
	}
	docs := a.Resources["DOCUMENT"]
	if docs[id]["content"] != content {
		return fmt.Errorf("%w: document %s no longer holds pdf %s", errStaleDocument, id, content)
	}
	if docs[id]["thumbnail"] != "" {
		return nil
	}
	for _, doc := range docs {
		if doc["parent"] != id {
			continue
		}
		if doc["thumbnail"] == "" {
			return fmt.Errorf("document %s first page has no server thumbnail", id)
		}
		_, err := c.Write(db.Batch{Batch: []db.Edit{{Set: id, Cells: map[string]any{"thumbnail": doc["thumbnail"], "thumbnailed": doc["thumbnailed"]}}}})
		if err != nil {
			return err
		}
		return waitForProcessing(c)
	}
	return fmt.Errorf("document %s has no first page", id)
}
