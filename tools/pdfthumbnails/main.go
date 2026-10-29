package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const (
	pdfDocuments = `(from DOCUMENT (where (= content.mime "application/pdf")) (columns content thumbnail rendered))`
	restartWait  = 30 * time.Second
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
	var out struct{}
	return c.Send(http.MethodPost, path, form.FormDataContentType(), buf, &out)
}

func unavailable(err error) bool {
	var status *qclient.StatusError
	if errors.As(err, &status) {
		return status.Code >= 500
	}
	var failed *url.Error
	var broken net.Error
	return errors.As(err, &failed) || errors.As(err, &broken) || errors.Is(err, io.ErrUnexpectedEOF)
}

func main() {
	c, err := qclient.SignedIn(qclient.ImportMode, "")
	if err != nil {
		logging.Fatal("sign in", "error", err)
	}
	for {
		err := draw(c)
		if err == nil {
			return
		}
		if !unavailable(err) {
			logging.Fatal("draw pdfs", "error", err)
		}
		slog.Warn("server unavailable, starting over", "in", restartWait, "error", err)
		time.Sleep(restartWait)
	}
}

func draw(c *qclient.Client) error {
	a, err := c.QueryText(pdfDocuments)
	if err != nil {
		return fmt.Errorf("read the pdfs still to draw: %w", err)
	}
	byContent := map[string][]string{}
	order := []string{}
	completed := map[string]string{}
	pending := 0
	for _, id := range a.Result {
		doc := a.Resources["DOCUMENT"][id]
		content := doc["content"]
		if doc["rendered"] != "" && doc["thumbnail"] != "" {
			completed[content] = id
			continue
		}
		pending++
		if _, ok := byContent[content]; !ok {
			order = append(order, content)
		}
		byContent[content] = append(byContent[content], id)
	}
	slog.Info("pdfs to draw", "documents", pending, "contents", len(order))
	drawn, failed, skipped := 0, 0, 0
	for i, content := range order {
		ids := byContent[content]
		if source := completed[content]; source != "" {
			for n, id := range ids {
				if err := reuse(c, id, content, source); err != nil {
					var status *qclient.StatusError
					if errors.Is(err, errNoThumbnail) {
						skipped += len(ids) - n
						slog.Warn("skipped", "documents", ids[n:], "content", content, "error", err)
						break
					}
					if errors.Is(err, errStaleDocument) || (errors.As(err, &status) && status.Code == http.StatusBadRequest) {
						skipped++
						slog.Warn("skipped", "document", id, "content", content, "error", err)
						continue
					}
					return err
				}
				drawn++
				slog.Info("reused pdf pages", "n", i+1, "of", len(order), "document", id, "source", source)
			}
			continue
		}
		var pdf []byte
		if err := c.Send(http.MethodGet, "/api/blob/"+content+"/blob", "", strings.NewReader(""), &pdf); err != nil {
			return fmt.Errorf("read pdf %s: %w", content, err)
		}
		dir, paths, err := render(pdf)
		if err != nil {
			os.RemoveAll(dir)
			failed += len(ids)
			slog.Warn("could not draw", "n", i+1, "of", len(order), "documents", ids, "bytes", len(pdf), "error", err)
			continue
		}
		sort.SliceStable(ids, func(i, j int) bool {
			return a.Resources["DOCUMENT"][ids[i]]["rendered"] != "" && a.Resources["DOCUMENT"][ids[j]]["rendered"] == ""
		})
		source, finished := "", []string{}
		for n, id := range ids {
			var err error
			if source == "" {
				if a.Resources["DOCUMENT"][id]["rendered"] != "" {
					err = completePDFThumbnail(c, id, content)
				} else {
					err = upload(c, id, content, paths)
				}
			} else {
				err = reuse(c, id, content, source)
			}
			var status *qclient.StatusError
			if errors.Is(err, errNoThumbnail) {
				skipped += len(ids) - n
				slog.Warn("skipped", "documents", ids[n:], "content", content, "error", err)
				break
			}
			if errors.Is(err, errStaleDocument) || (errors.As(err, &status) && status.Code == http.StatusBadRequest) {
				skipped++
				slog.Warn("skipped", "document", id, "content", content, "error", err)
				continue
			}
			if err != nil {
				os.RemoveAll(dir)
				return fmt.Errorf("send pdf %s: %w", id, err)
			}
			if source == "" {
				source = id
			} else {
				slog.Info("reused pdf pages", "n", i+1, "of", len(order), "document", id, "source", source)
			}
			drawn++
			finished = append(finished, id)
		}
		os.RemoveAll(dir)
		if len(finished) > 0 {
			slog.Info("drawn", "n", i+1, "of", len(order), "documents", finished, "pdf bytes", len(pdf), "pages", len(paths))
		}

	}
	slog.Info("done", "documents", pending, "drawn", drawn, "failed", failed, "skipped", skipped)
	return nil
}

func upload(c *qclient.Client, id, content string, paths []string) error {
	for i, path := range paths {
		picture, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := send(c, "/api/do/pdf-page", id, content, i+1, len(paths), picture); err != nil {
			return err
		}
		slog.Info("uploaded a page", "document", id, "page", i+1, "of", len(paths), "bytes", len(picture))
	}
	return completePDFThumbnail(c, id, content)
}
