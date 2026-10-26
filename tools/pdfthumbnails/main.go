package main

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/blob"
	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const (
	stillToDraw = `(from DOCUMENT (where (blank rendered) (= content.mime "application/pdf")) (columns content thumbnailed))`
	sendTries   = 4
	sendWait    = 5 * time.Second
)

func render(pdf []byte) (string, []string, error) {
	dir, err := os.MkdirTemp("", "pdfpages")
	if err != nil {
		return "", nil, err
	}
	in := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return dir, nil, err
	}
	cmd := exec.Command("pdftoppm", "-png", "-r", "150", "-cropbox", in, filepath.Join(dir, "page"))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return dir, nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "page-*.png"))
	if err != nil {
		return dir, nil, err
	}
	number := func(path string) int {
		n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "page-"), ".png"))
		return n
	}
	sort.Slice(paths, func(i, j int) bool { return number(paths[i]) < number(paths[j]) })
	if len(paths) == 0 {
		return dir, nil, fmt.Errorf("the pdf produced no page images")
	}
	for i, path := range paths {
		if number(path) != i+1 {
			return dir, nil, fmt.Errorf("missing page %d", i+1)
		}
	}
	return dir, paths, nil
}

func send(c *qclient.Client, path, id, content string, page, total int, picture []byte) error {
	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)
	if err := form.WriteField("document", id); err != nil {
		return err
	}
	if err := form.WriteField("content", content); err != nil {
		return err
	}
	if page > 0 {
		if err := form.WriteField("page", strconv.Itoa(page)); err != nil {
			return err
		}
		if err := form.WriteField("pages", strconv.Itoa(total)); err != nil {
			return err
		}
	}
	part, err := form.CreateFormFile("picture", "page.png")
	if err != nil {
		return err
	}
	if _, err := part.Write(picture); err != nil {
		return err
	}
	if err := form.Close(); err != nil {
		return err
	}
	body := buf.Bytes()
	wait := sendWait
	for try := 1; ; try++ {
		var out struct{}
		err := c.Send(http.MethodPost, path, form.FormDataContentType(), bytes.NewReader(body), &out)
		var status *qclient.StatusError
		if err == nil || try == sendTries || !errors.As(err, &status) || (status.Code != http.StatusTooManyRequests && status.Code < 500) {
			return err
		}
		slog.Warn("send again", "document", id, "try", try, "in", wait, "error", err)
		time.Sleep(wait)
		wait *= 3
	}
}

func main() {
	c, err := qclient.SignedIn(qclient.ImportMode, "")
	if err != nil {
		logging.Fatal("sign in", "error", err)
	}
	a, err := c.QueryText(stillToDraw)
	if err != nil {
		logging.Fatal("read the pdfs still to draw", "error", err)
	}
	byContent := map[string][]string{}
	order := []string{}
	for _, id := range a.Result {
		content := a.Resources["DOCUMENT"][id]["content"]
		if _, ok := byContent[content]; !ok {
			order = append(order, content)
		}
		byContent[content] = append(byContent[content], id)
	}
	slog.Info("pdfs to draw", "documents", len(a.Result), "contents", len(order))
	drawn, failed, skipped := 0, 0, 0
	for i, content := range order {
		ids := byContent[content]
		var pdf []byte
		if err := c.Send(http.MethodGet, "/api/blob/"+content+"/blob", "", strings.NewReader(""), &pdf); err != nil {
			logging.Fatal("read the pdf", "content", content, "error", err)
		}
		dir, paths, err := render(pdf)
		if err != nil {
			os.RemoveAll(dir)
			failed += len(ids)
			slog.Warn("could not draw", "n", i+1, "of", len(order), "documents", ids, "bytes", len(pdf), "error", err)
			continue
		}
		for _, id := range ids {
			err := upload(c, id, content, paths, a.Resources["DOCUMENT"][id]["thumbnailed"] == "")
			var status *qclient.StatusError
			if errors.As(err, &status) && status.Code == http.StatusBadRequest {
				skipped++
				slog.Warn("skipped", "document", id, "content", content, "error", err)
				continue
			}
			if err != nil {
				logging.Fatal("send", "document", id, "error", err)
			}
			drawn++
		}
		os.RemoveAll(dir)
		slog.Info("drawn", "n", i+1, "of", len(order), "documents", ids, "pdf bytes", len(pdf), "pages", len(paths))

	}
	slog.Info("done", "documents", len(a.Result), "drawn", drawn, "failed", failed, "skipped", skipped)
}

func upload(c *qclient.Client, id, content string, paths []string, thumbnail bool) error {
	for i, path := range paths {
		picture, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if i == 0 && thumbnail {
			thumb, err := blob.Thumbnail(picture)
			if err != nil {
				return err
			}
			if err := send(c, "/api/do/thumbnail", id, content, 0, 0, thumb); err != nil {
				return err
			}
		}
		if err := send(c, "/api/do/pdf-page", id, content, i+1, len(paths), picture); err != nil {
			return err
		}
		slog.Info("uploaded a page", "document", id, "page", i+1, "of", len(paths), "bytes", len(picture))
	}
	return nil
}
