package db

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/testkit/authtest"
)

func TestPhotoCompletionStream(t *testing.T) {
	for _, action := range []string{"processed", "deleted", "hidden", "failed", "cancelled"} {
		t.Run(action, func(t *testing.T) {
			s := sample(t)
			pics := NewPictures(s, blob.NewMemoryBucket())
			upload := addPhoto(t, s, pics, importing, staff, pngOf(t, 5))
			if upload.Code != http.StatusOK {
				t.Fatalf("upload: %d %s", upload.Code, upload.Body.String())
			}
			var stored stored
			if err := json.Unmarshal(upload.Body.Bytes(), &stored); err != nil {
				t.Fatal(err)
			}
			id := stored.Result[0]
			mux := http.NewServeMux()
			Register(mux, s, pics, testTokens)
			server := httptest.NewServer(authtest.Fixed("rowan.ashdown@example.org", mux))
			t.Cleanup(server.Close)
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+blobPath+id+"/image", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Accept", "text/event-stream")
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "text/event-stream" || resp.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("stream: %d %v", resp.StatusCode, resp.Header)
			}
			photo, _ := s.Model().Table("PHOTO").Get(id)
			if photo["ready"] != "" {
				t.Fatal("the stream waited for processing before sending headers")
			}
			switch action {
			case "processed":
				go pics.run()
			case "deleted":
				apply(t, s, []Edit{{Delete: id}})
			case "hidden":
				apply(t, s, []Edit{{Set: staff, Cells: map[string]any{"hidden": "Yes"}}})
			case "failed":
				content, _ := s.Model().Table("CONTENT").Get(photo["original"])
				generation, err := pics.bucket.Generation(t.Context(), content["blob"])
				if err != nil {
					t.Fatal(err)
				}
				if err := pics.bucket.Remove(t.Context(), content["blob"], generation); err != nil {
					t.Fatal(err)
				}
				go pics.run()
			case "cancelled":
				cancel()
			}
			body, err := io.ReadAll(resp.Body)
			if action == "cancelled" {
				if err == nil {
					t.Fatal("a cancelled stream completed successfully")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if action != "processed" {
				if !strings.Contains(string(body), `"error":`) {
					t.Fatalf("the %s photo answered %s", action, body)
				}
				return
			}
			if string(body) != "data: {}\n\n" {
				t.Fatalf("completion: %q", body)
			}
			for _, path := range []string{id + "/original", "pho00000000000/image"} {
				req := httptest.NewRequest(http.MethodGet, blobPath+path, nil)
				req.Header.Set("Accept", "text/event-stream")
				rec := httptest.NewRecorder()
				serveAs(mux, rec, req, "rowan.ashdown@example.org")
				if rec.Code != http.StatusNotFound {
					t.Fatalf("unsupported stream %s: %d %s", path, rec.Code, rec.Body.String())
				}
			}
			got := fetchBlob(t, s, pics, "rowan.ashdown@example.org", id+"/image", "")
			if got.Code != http.StatusOK || got.Body.Len() == 0 {
				t.Fatalf("completed photo: %d %s", got.Code, got.Body.String())
			}
			stream := httptest.NewRequest(http.MethodGet, blobPath+id+"/image", nil)
			stream.Header.Set("Accept", "text/event-stream")
			rec := httptest.NewRecorder()
			serveAs(mux, rec, stream, superAdmin)
			if rec.Body.String() != "data: {}\n\n" {
				t.Fatalf("already completed photo: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}
