package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/net/html"

	"heliosian/internal/db"
)

var calendarTables = []string{"GROUP", "GROUP_SOURCE", "RULE", "MEMBER", "DOCUMENT_GROUP", "PERSON", "DOCUMENT"}

func (c client) calendarRows() (db.CalendarRows, error) {
	out := db.CalendarRows{}
	for _, name := range calendarTables {
		t, _, err := c.table(name)
		if err != nil {
			return nil, err
		}
		rows := []map[string]string{}
		for _, id := range t.order {
			rows = append(rows, t.rows[id])
		}
		out[name] = rows
	}
	return out, nil
}

func (c client) addCalendarPDF(pdf []byte, address string) (string, error) {
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	if err := form.WriteField("url", address); err != nil {
		return "", err
	}
	part, err := form.CreateFormFile("pdf", "calendar.pdf")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(pdf); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}
	var out struct {
		Result []string `json:"result"`
	}
	if err := c.Send(http.MethodPost, "/api/do/calendar-pdf", form.FormDataContentType(), body, &out); err != nil {
		return "", err
	}
	return out.Result[0], nil
}

var retryWaits = []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute}

func fetch(address string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	for attempt := 0; ; attempt++ {
		resp, err := client.Get(address)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			return body, nil
		}
		if resp.StatusCode != http.StatusTooManyRequests || attempt == len(retryWaits) {
			return nil, fmt.Errorf("get %s: %s", address, resp.Status)
		}
		slog.Warn("calendar import: fetch throttled", "url", address, "status", resp.Status, "retry-after", resp.Header.Get("Retry-After"), "wait", retryWaits[attempt])
		time.Sleep(retryWaits[attempt])
	}
}

var pdfLink = regexp.MustCompile(`href="([^"]+\.pdf)"`)

func findPDF(page []byte) (string, error) {
	match := pdfLink.FindSubmatch(page)
	if match == nil {
		return "", fmt.Errorf("no pdf link on %s", db.SchoolCalendarPage)
	}
	base, err := url.Parse(db.SchoolCalendarPage)
	if err != nil {
		return "", err
	}
	link, err := url.Parse(html.UnescapeString(string(match[1])))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(link).String(), nil
}

func current(rows db.CalendarRows, hash, reading string) (string, bool) {
	doc := ""
	for _, d := range rows["DOCUMENT"] {
		if d["kind"] == "calendar" && d["hash"] == hash {
			doc = d["id"]
		}
	}
	if doc == "" {
		return "", false
	}
	sources := 0
	for _, s := range rows["GROUP_SOURCE"] {
		if s["document"] != doc {
			continue
		}
		if s["hash"] != reading {
			return doc, false
		}
		sources++
	}
	return doc, sources > 0
}

func importCalendar(c client, anthropicKey string, dryRun bool) error {
	ctx := context.Background()
	page, err := fetch(db.SchoolCalendarPage)
	if err != nil {
		return fmt.Errorf("fetch the school calendar page: %w", err)
	}
	address, err := findPDF(page)
	if err != nil {
		return err
	}
	pdf, err := fetch(address)
	if err != nil {
		return fmt.Errorf("fetch the year calendar pdf: %w", err)
	}
	sum := sha256.Sum256(pdf)
	hash := hex.EncodeToString(sum[:])
	rows, err := c.calendarRows()
	if err != nil {
		return err
	}
	v, err := db.NewVocabulary(rows)
	if err != nil {
		return err
	}
	reading := readingHash(v)
	doc, known := current(rows, hash, reading)
	if known {
		slog.Info("calendar import: the year calendar is unchanged", "url", address, "hash", hash)
		return nil
	}
	slog.Info("calendar import: reading the year calendar", "url", address, "hash", hash)
	ai := anthropic.NewClient(option.WithAPIKey(anthropicKey))
	x, err := extractPDF(ctx, ai, pdf, v)
	if err != nil {
		return err
	}
	cal, err := yearCalendar(x, v, reading)
	if err != nil {
		return err
	}
	matched, err := v.MatchPDF(ctx, ai, rows, cal)
	if err != nil {
		return err
	}
	pending := db.PDFToClassify(rows, v, cal, matched)
	classified := v.Classify(ctx, ai, pending)
	if len(classified) != len(pending) {
		return fmt.Errorf("classified %d of %d events; nothing written", len(classified), len(pending))
	}
	if dryRun {
		cal.Document = doc
		if cal.Document == "" {
			cal.Document = "doc00000000000"
		}
	} else if cal.Document, err = c.addCalendarPDF(pdf, address); err != nil {
		return err
	}
	edits, err := db.PDFPlan(rows, v, cal, classified, matched)
	if err != nil {
		return err
	}
	slog.Info("calendar import: planned", "year", cal.Year, "entries", len(cal.Entries), "shaded", len(cal.Shaded), "edits", len(edits))
	if dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(db.Batch{Batch: edits})
	}
	if len(edits) == 0 {
		return nil
	}
	written, err := c.Write(db.Batch{Batch: edits})
	if err != nil {
		return err
	}
	slog.Info("calendar import: written", "writes", len(written))
	return nil
}
