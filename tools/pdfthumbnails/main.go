package main

import (
	"bytes"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"heliosian/internal/logging"
	"heliosian/internal/qclient"
)

const (
	stillToDraw = `(from DOCUMENT (where (blank thumbnailed) (= content.mime "application/pdf")) (columns content))`
	renderWidth = "960"
	sendTries   = 4
	sendWait    = 5 * time.Second
)

func render(pdf []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "pdfthumbnail")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("pdftoppm", "-png", "-singlefile", "-f", "1", "-l", "1", "-scale-to-x", renderWidth, "-scale-to-y", "-1", in, filepath.Join(dir, "page"))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(dir, "page.png"))
}

func send(c *qclient.Client, id, content string, picture []byte) error {
	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)
	if err := form.WriteField("document", id); err != nil {
		return err
	}
	if err := form.WriteField("content", content); err != nil {
		return err
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
		err := c.Send(http.MethodPost, "/api/do/thumbnail", form.FormDataContentType(), bytes.NewReader(body), &out)
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
		picture, err := render(pdf)
		if err != nil {
			failed += len(ids)
			slog.Warn("could not draw", "n", i+1, "of", len(order), "documents", ids, "bytes", len(pdf), "error", err)
			continue
		}
		for _, id := range ids {
			err := send(c, id, content, picture)
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
		slog.Info("drawn", "n", i+1, "of", len(order), "documents", ids, "pdf bytes", len(pdf), "picture bytes", len(picture))
	}
	slog.Info("done", "documents", len(a.Result), "drawn", drawn, "failed", failed, "skipped", skipped)
}
