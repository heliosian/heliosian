package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"fmt"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"heliosian/internal/db"
	"heliosian/internal/qclient"
)

func TestSharedPDFUploadsOnceAndReusesCompletedPages(t *testing.T) {
	picture := &bytes.Buffer{}
	if err := png.Encode(picture, image.NewNRGBA(image.Rect(0, 0, 20, 30))); err != nil {
		t.Fatal(err)
	}
	pdf := &bytes.Buffer{}
	if err := api.ImportImages(t.Context(), nil, pdf, []io.Reader{picture}, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                                                                                    string
		completed, thumbnail, laterIncomplete, marked, firstMarked, partial, conflict, noMarker bool
	}{
		{name: "new"},
		{name: "unrendered first document with thumbnail marker", firstMarked: true},
		{name: "completed", completed: true, thumbnail: true},
		{name: "thumbnail image without attempt marker", completed: true, thumbnail: true, noMarker: true},
		{name: "partial target", completed: true, thumbnail: true, partial: true},
		{name: "conflicting target", completed: true, thumbnail: true, partial: true, conflict: true},
		{name: "missing thumbnail", completed: true},
		{name: "missing thumbnail with marker", completed: true, marked: true},
		{name: "prefer complete source", completed: true, thumbnail: true, laterIncomplete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docs := map[string]map[string]string{
				"doc00000000001": {"content": "cnt00000000001"},
				"doc00000000002": {"content": "cnt00000000001"},
			}
			if tc.completed {
				docs["doc00000000001"]["rendered"] = "2026-10-26 20:00:00"
				docs["doc00000000001"]["page_count"] = "1"
				docs["doc-source-page"] = map[string]string{"parent": "doc00000000001", "relation": "page", "first_page": "1", "last_page": "1", "content": "cnt00000000002", "thumbnail": "cnt00000000008", "thumbnailed": "2026-10-26 20:00:00", "extracted": "2026-10-26 20:00:00"}
			}
			if tc.firstMarked {
				docs["doc00000000002"]["thumbnailed"] = "2026-10-26 20:00:00"
			}
			if tc.marked {
				docs["doc00000000001"]["thumbnailed"] = "2026-10-26 20:00:00"
			}
			if tc.thumbnail {
				docs["doc00000000001"]["thumbnail"] = "cnt00000000003"
				docs["doc00000000001"]["thumbnailed"] = "2026-10-26 20:00:00"
			}
			if tc.noMarker {
				delete(docs["doc00000000001"], "thumbnailed")
			}
			order := []string{"doc00000000002", "doc00000000001"}
			if tc.laterIncomplete {
				docs["doc00000000003"] = map[string]string{"content": "cnt00000000001", "rendered": "2026-10-26 20:00:00", "page_count": "1"}
				docs["doc-later-page"] = map[string]string{"parent": "doc00000000003", "relation": "page", "first_page": "1", "last_page": "1", "content": "cnt00000000002"}
				order = append(order, "doc00000000003")
			}
			if tc.partial {
				pageContent := "cnt00000000002"
				if tc.conflict {
					pageContent = "cnt00000000004"
				}
				docs["doc-target-page"] = map[string]string{"parent": "doc00000000002", "relation": "page", "first_page": "1", "last_page": "1", "content": pageContent}
			}
			downloads, pages, thumbnails, copies := 0, 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/q":
					if r.Method == "POST" {
						copies++
						var batch db.Batch
						if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
							t.Error(err)
							return
						}
						for _, edit := range batch.Batch {
							if edit.Insert == "DOCUMENT" {
								if edit.Row["content"] != "cnt00000000002" || edit.Row["thumbnail"] != "cnt00000000008" || edit.Row["thumbnailed"] == "" {
									t.Errorf("page lost its shared content: %v", edit.Row)
								}
								row := map[string]string{}
								for key, value := range edit.Row {
									row[key] = fmt.Sprint(value)
								}
								docs[fmt.Sprintf("doc-copy-%d", copies)] = row
							} else if edit.Set != "" {
								for key, value := range edit.Cells {
									docs[edit.Set][key] = fmt.Sprint(value)
								}
							} else {
								t.Errorf("unexpected edit: %v", edit)
							}
						}
						io.WriteString(w, "{}")
						return
					}
					ids := order
					if r.Header.Get("Content-Type") == "application/json" {
						raw, err := io.ReadAll(r.Body)
						if err != nil {
							t.Error(err)
							return
						}
						if _, err := db.ParseJSON(raw); err != nil {
							t.Error(err)
							http.Error(w, err.Error(), 400)
							return
						}
						ids = []string{}
						for id := range docs {
							ids = append(ids, id)
						}
					}
					if err := json.NewEncoder(w).Encode(qclient.Answer{Result: ids, Resources: map[string]map[string]map[string]string{"DOCUMENT": docs}}); err != nil {
						t.Error(err)
					}
					return
				case "/api/blob/cnt00000000001/blob":
					downloads++
					w.Header().Set("Content-Type", "application/pdf")
					w.Write(pdf.Bytes())
					return
				case "/api/do/pdf-page":
					pages++
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					id := r.FormValue("document")
					docs[id]["rendered"] = "2026-10-26 20:00:00"
					docs[id]["page_count"] = "1"
					docs[id+"-page"] = map[string]string{"parent": id, "relation": "page", "first_page": "1", "last_page": "1", "content": "cnt00000000002", "thumbnail": "cnt00000000008", "thumbnailed": "2026-10-26 20:00:00", "extracted": "2026-10-26 20:00:00"}
				case "/api/do/thumbnail":
					thumbnails++
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						return
					}
					defer r.MultipartForm.RemoveAll()
					doc := docs[r.FormValue("document")]
					doc["thumbnail"] = "cnt00000000003"
					doc["thumbnailed"] = "2026-10-26 20:00:00"
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				io.WriteString(w, "{}")
			}))
			defer server.Close()
			err := draw(&qclient.Client{Base: server.URL})
			if tc.conflict {
				if err == nil || copies != 0 || pages != 0 {
					t.Fatalf("conflicting partial target: err=%v copies=%d pages=%d", err, copies, pages)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := draw(&qclient.Client{Base: server.URL}); err != nil {
				t.Fatal(err)
			}
			wantDownloads, wantPages, wantThumbnails, wantCopies := 1, 1, 0, 2
			if tc.completed {
				wantPages = 0
			}
			if tc.thumbnail {
				wantDownloads, wantCopies = 0, 1
			}
			if tc.laterIncomplete {
				wantCopies++
			}
			if downloads != wantDownloads || pages != wantPages || thumbnails != wantThumbnails || copies != wantCopies {
				t.Fatalf("downloads=%d pages=%d thumbnails=%d copies=%d", downloads, pages, thumbnails, copies)
			}
		})
	}
}

func TestChangedPDFIsSkippedAndLaterPDFContinues(t *testing.T) {
	for _, changed := range []string{"doc00000000001", "doc00000000002"} {
		t.Run(changed, func(t *testing.T) {
			docs := map[string]map[string]string{
				"doc00000000001": {"content": "cnt00000000001", "rendered": "2026-10-26 20:00:00", "page_count": "1", "thumbnail": "cnt00000000003"},
				"doc00000000002": {"content": "cnt00000000001"},
				"doc00000000101": {"parent": "doc00000000001", "relation": "page", "first_page": "1", "last_page": "1", "content": "cnt00000000002"},
				"doc00000000004": {"content": "cnt00000000005", "rendered": "2026-10-26 20:00:00", "page_count": "1", "thumbnail": "cnt00000000007"},
				"doc00000000005": {"content": "cnt00000000005"},
				"doc00000000104": {"parent": "doc00000000004", "relation": "page", "first_page": "1", "last_page": "1", "content": "cnt00000000006", "thumbnail": "cnt00000000008", "thumbnailed": "2026-10-26 20:00:00"},
			}
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path != "/api/q" {
					t.Errorf("unexpected upload %s", r.URL.Path)
					http.Error(w, "unexpected upload", 400)
					return
				}
				if r.Method == http.MethodPost {
					writes++
					var batch db.Batch
					if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
						t.Error(err)
						return
					}
					for _, edit := range batch.Batch {
						if edit.Insert == "DOCUMENT" && (edit.Row["parent"] != "doc00000000005" || edit.Row["content"] != "cnt00000000006") {
							t.Errorf("wrote stale pages: %v", edit)
						}
						if edit.Set != "" && edit.Set != "doc00000000005" {
							t.Errorf("completed stale document: %v", edit)
						}
					}
					io.WriteString(w, "{}")
					return
				}
				if r.Header.Get("Content-Type") == "application/json" {
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						return
					}
					if _, err := db.ParseJSON(raw); err != nil {
						t.Error(err)
						http.Error(w, err.Error(), 400)
						return
					}
					docs[changed]["content"] = "cnt00000000009"
				}
				if err := json.NewEncoder(w).Encode(qclient.Answer{Result: []string{"doc00000000002", "doc00000000001", "doc00000000005", "doc00000000004"}, Resources: map[string]map[string]map[string]string{"DOCUMENT": docs}}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			if err := draw(&qclient.Client{Base: server.URL}); err != nil {
				t.Fatal(err)
			}
			if writes != 1 {
				t.Fatalf("later pdf writes=%d, expected 1", writes)
			}
		})
	}
}
