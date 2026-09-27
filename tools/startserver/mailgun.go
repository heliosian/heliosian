package main

import (
	"fmt"
	"io"
	"log/slog"
	"maps"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type mailFiles struct {
	dir string
}

func (f mailFiles) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := f.write(r); err != nil {
		slog.Error("sample mailgun: write message", "path", r.URL.Path, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"id":"<sample@example.org>","message":"Queued. Thank you."}`)
}

func (f mailFiles) write(r *http.Request) error {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return err
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return err
	}
	form := r.MultipartForm
	base := filepath.Join(f.dir, time.Now().Format("20060102-150405.000000"))
	to := ""
	if len(form.Value["to"]) > 0 {
		to = "-" + slug(strings.SplitN(form.Value["to"][0], "@", 2)[0])
	}
	if strings.HasSuffix(r.URL.Path, "/messages.mime") {
		raw, err := read(form.File["message"][0])
		if err != nil {
			return err
		}
		name := base + to + ".eml"
		slog.Info("sample mailgun: wrote raw message", "file", name, "to", form.Value["to"], "bytes", len(raw))
		return os.WriteFile(name, raw, 0o644)
	}
	name := base + "-" + slug(r.FormValue("subject")) + to
	var head strings.Builder
	for _, k := range slices.Sorted(maps.Keys(form.Value)) {
		if k != "html" && k != "text" {
			fmt.Fprintf(&head, "%s: %s\n", k, strings.Join(form.Value[k], ", "))
		}
	}
	if err := os.WriteFile(name+".html", []byte("<!--\n"+head.String()+"-->\n"+r.FormValue("html")), 0o644); err != nil {
		return err
	}
	for _, a := range form.File["attachment"] {
		content, err := read(a)
		if err != nil {
			return err
		}
		if err := os.WriteFile(name+"-"+a.Filename, content, 0o644); err != nil {
			return err
		}
	}
	slog.Info("sample mailgun: wrote message", "file", name+".html", "to", form.Value["to"], "subject", r.FormValue("subject"), "attachments", len(form.File["attachment"]))
	return nil
}

func read(header *multipart.FileHeader) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}
