package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"heliosian/internal/db"
	"heliosian/internal/qclient"
)

func TestEveryPDFPageIsRenderedAndUploaded(t *testing.T) {
	readers := []io.Reader{}
	for _, c := range []color.NRGBA{{R: 220, A: 255}, {G: 220, A: 255}, {B: 220, A: 255}} {
		img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
		for y := range 30 {
			for x := range 40 {
				img.Set(x, y, c)
			}
		}
		buf := &bytes.Buffer{}
		if err := png.Encode(buf, img); err != nil {
			t.Fatal(err)
		}
		readers = append(readers, buf)
	}
	raw := &bytes.Buffer{}
	if err := api.ImportImages(t.Context(), nil, raw, readers, nil, nil); err != nil {
		t.Fatal(err)
	}
	dir, paths, err := render(raw.Bytes())
	if dir != "" {
		defer os.RemoveAll(dir)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("rendered %d pages", len(paths))
	}
	for i, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r, g, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
		got := []uint32{r, g, b}
		if got[i] < 50000 {
			t.Fatalf("page %d lost its color: %v", i+1, got)
		}
	}
	againDir, again, err := render(raw.Bytes())
	if againDir != "" {
		defer os.RemoveAll(againDir)
	}
	if err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(again[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("page %d changes bytes when redrawn", i+1)
		}
	}
	pages, thumbnails := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if finishUpload(w, r, t) {
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			http.Error(w, "bad form", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if r.FormValue("document") != "doc00000000001" || r.FormValue("content") != "cnt00000000001" {
			t.Error("the upload lost its source identity")
		}
		if r.URL.Path == "/api/do/thumbnail" {
			thumbnails++
		} else if r.URL.Path == "/api/do/pdf-page" {
			pages++
			if r.FormValue("page") != strconv.Itoa(pages) || r.FormValue("pages") != "3" {
				t.Errorf("page upload: %v", r.Form)
			}
		} else {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
		file, _, err := r.FormFile("picture")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		if _, err := png.Decode(file); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{}")
	}))
	defer server.Close()
	c := &qclient.Client{Base: server.URL}
	if err := upload(c, "doc00000000001", "cnt00000000001", paths); err != nil {
		t.Fatal(err)
	}
	if pages != 3 || thumbnails != 0 {
		t.Fatalf("uploaded %d pages and %d thumbnails", pages, thumbnails)
	}
}

func TestLargePageUploadsUnchangedWithoutALocalThumbnail(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2481, 3508))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	buf := &bytes.Buffer{}
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(buf, img); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	if len(raw) <= 21<<20 {
		t.Fatalf("the fixture is only %d bytes", len(raw))
	}
	path := filepath.Join(t.TempDir(), "page.png")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	pages, thumbnails := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/do/thumbnail" {
			r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
		}
		if finishUpload(w, r, t) {
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			http.Error(w, "too large", 400)
			return
		}
		defer r.MultipartForm.RemoveAll()
		file, _, err := r.FormFile("picture")
		if err != nil {
			t.Error(err)
			return
		}
		defer file.Close()
		if r.URL.Path == "/api/do/pdf-page" {
			pages++
			got, err := io.ReadAll(file)
			if err != nil || !bytes.Equal(got, raw) {
				t.Error("the full page was changed")
			}
		} else {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "{}")
	}))
	defer server.Close()
	if err := upload(&qclient.Client{Base: server.URL}, "doc00000000001", "cnt00000000001", []string{path}); err != nil {
		t.Fatal(err)
	}
	if pages != 1 || thumbnails != 0 {
		t.Fatalf("uploaded %d pages and %d thumbnails", pages, thumbnails)
	}
}

func finishUpload(w http.ResponseWriter, r *http.Request, t *testing.T) bool {
	t.Helper()
	if r.URL.Path != "/api/q" {
		return false
	}
	if r.Method == http.MethodPost {
		var batch db.Batch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		if len(batch.Batch) != 1 || batch.Batch[0].Set != "doc00000000001" || batch.Batch[0].Cells["thumbnail"] != "cnt00000000002" {
			t.Errorf("pdf thumbnail was not copied from the server page: %v", batch)
		}
		io.WriteString(w, `{}`)
		return true
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
	}
	if _, err := db.ParseJSON(raw); err != nil {
		t.Error(err)
	}
	json.NewEncoder(w).Encode(qclient.Answer{Resources: map[string]map[string]map[string]string{"DOCUMENT": {
		"doc00000000001": {"content": "cnt00000000001", "page_count": "1"},
		"doc00000000101": {"parent": "doc00000000001", "first_page": "1", "thumbnail": "cnt00000000002", "thumbnailed": "2026-10-26 20:00:00"},
	}}})
	return true
}
