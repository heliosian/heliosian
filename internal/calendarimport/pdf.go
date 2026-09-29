package calendarimport

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/model"
)

type pdfEntry struct {
	Title      string   `json:"title"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	DayType    string   `json:"dayType"`
	Classrooms []string `json:"classrooms"`
	Marker     string   `json:"marker"`
}

type shadedDay struct {
	Date    string
	Legend  string
	DayType string
}

type pdfExtraction struct {
	Year    string     `json:"year"`
	Entries []pdfEntry `json:"entries"`
	Shaded  []shadedDay
}

type legendEntry struct {
	Label   string `json:"label"`
	Color   string `json:"color"`
	DayType string `json:"dayType"`
}

const (
	unfilled  = "none"
	otherFill = "other"
)

var schoolYearForm = regexp.MustCompile(`^(\d{4})-(\d{4})$`)

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
}

func pdfDocument(pdf []byte) anthropic.ContentBlockParamUnion {
	return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(pdf)})
}

func extractLegend(ctx context.Context, client anthropic.Client, page anthropic.ContentBlockParamUnion, dayTypes []string) ([]legendEntry, error) {
	system := legendSystem
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"legend"},
		"properties": map[string]any{
			"legend": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"label", "color", "dayType"},
					"properties": map[string]any{
						"label":   map[string]any{"type": "string"},
						"color":   map[string]any{"type": "string", "description": "hex RGB like #c9b7e6"},
						"dayType": enumOf(append([]string{noDayType}, dayTypes...)),
					},
				},
			},
		},
	}
	var out struct {
		Legend []legendEntry `json:"legend"`
	}
	content := []anthropic.ContentBlockParamUnion{
		page,
		anthropic.NewTextBlock("Read the legend of this school year calendar."),
	}
	if _, err := ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out); err != nil {
		return nil, fmt.Errorf("legend: %w", err)
	}
	if len(out.Legend) == 0 {
		return nil, fmt.Errorf("legend: nothing found")
	}
	seen := map[string]bool{}
	for _, e := range out.Legend {
		if e.Label == "" || e.Label == unfilled || seen[e.Label] {
			return nil, fmt.Errorf("legend: bad or repeated wording %q", e.Label)
		}
		seen[e.Label] = true
		slog.InfoContext(ctx, "calendar import: pdf legend", "color", e.Color, "label", e.Label, "day type", e.DayType)
	}
	return out.Legend, nil
}

func extractMonth(ctx context.Context, client anthropic.Client, document anthropic.ContentBlockParamUnion, month time.Time, legend []legendEntry) ([]shadedDay, error) {
	name := month.Format("January 2006")
	labels := []string{unfilled, otherFill}
	described := &strings.Builder{}
	for _, e := range legend {
		labels = append(labels, e.Label)
		fmt.Fprintf(described, "- %s: %q\n", e.Color, e.Label)
	}
	system := monthSystem(described.String())
	last := month.AddDate(0, 1, -1).Day()
	monthNames := []string{}
	for m := time.January; m <= time.December; m++ {
		monthNames = append(monthNames, m.String())
	}
	properties := map[string]any{"month": enumOf(monthNames)}
	required := []string{"month"}
	for day := 1; day <= last; day++ {
		key := strconv.Itoa(day)
		properties[key] = enumOf(labels)
		required = append(required, key)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": properties,
	}
	content := []anthropic.ContentBlockParamUnion{
		document,
		anthropic.NewTextBlock("This should be the " + strings.ToUpper(month.Format("January")) + " grid, the month of " + name + ", which has " + strconv.Itoa(last) + " days. Report the month in its title and the background of every day cell."),
	}
	dayType := map[string]string{}
	for _, e := range legend {
		dayType[e.Label] = e.DayType
	}
	meaning := func(fill string) string {
		for label, t := range dayType {
			if strings.EqualFold(label, fill) && t != noDayType {
				return t
			}
		}
		return ""
	}
	read := func() (map[int]string, error) {
		out := map[string]string{}
		if _, err := ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out); err != nil {
			return nil, err
		}
		if !strings.EqualFold(out["month"], month.Format("January")) {
			return nil, fmt.Errorf("the grid cut out for it is titled %q; the page layout has changed", out["month"])
		}
		fills := map[int]string{}
		for day := 1; day <= last; day++ {
			fills[day] = out[strconv.Itoa(day)]
		}
		return fills, nil
	}
	readings, errs := fanOut(2, func(int) (map[int]string, error) { return read() })
	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	disputed := []int{}
	for day := 1; day <= last; day++ {
		if meaning(readings[0][day]) != meaning(readings[1][day]) {
			disputed = append(disputed, day)
			slog.WarnContext(ctx, "calendar import: pdf readings differ", "month", name, "day", day, "first", readings[0][day], "second", readings[1][day])
		}
	}
	if len(disputed) > 0 {
		slog.WarnContext(ctx, "calendar import: reading the month a third time", "month", name, "disputed", len(disputed))
		third, err := read()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		readings = append(readings, third)
	}
	days := []shadedDay{}
	for day := 1; day <= last; day++ {
		votes := map[string]int{}
		fill := map[string]string{}
		for _, r := range readings {
			m := meaning(r[day])
			votes[m]++
			fill[m] = r[day]
		}
		chosen, best := "", 0
		for m, n := range votes {
			if n > best {
				chosen, best = m, n
			}
		}
		if best*2 <= len(readings) {
			return nil, fmt.Errorf("%s: no majority for day %d", name, day)
		}
		if chosen == "" {
			continue
		}
		days = append(days, shadedDay{
			Date:    time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, model.Location).Format(model.DateFormat),
			Legend:  fill[chosen],
			DayType: chosen,
		})
	}
	slog.InfoContext(ctx, "calendar import: pdf month read", "month", name, "filled", len(days), "readings", len(readings))
	return days, nil
}

func extractEntries(ctx context.Context, client anthropic.Client, pdf []byte, roster model.Roster, dayTypes []string) (pdfExtraction, error) {
	system := entriesSystem(roster)
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"year", "entries"},
		"properties": map[string]any{
			"year": map[string]any{"type": "string", "description": "the school year as YYYY-YYYY"},
			"entries": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object", "additionalProperties": false,
					"required": []string{"title", "start", "end", "dayType", "classrooms", "marker"},
					"properties": map[string]any{
						"title":      map[string]any{"type": "string"},
						"start":      map[string]any{"type": "string", "description": "YYYY-MM-DD"},
						"end":        map[string]any{"type": "string", "description": "YYYY-MM-DD, the same as start for a single day"},
						"dayType":    enumOf(append([]string{noDayType}, dayTypes...)),
						"classrooms": map[string]any{"type": "array", "minItems": 1, "items": enumOf(roster.Names())},
						"marker":     enumOf([]string{noMarker, model.MarkerFirstDay, model.MarkerLastDay}),
					},
				},
			},
		},
	}
	content := []anthropic.ContentBlockParamUnion{
		pdfDocument(pdf),
		anthropic.NewTextBlock("Extract every entry of the Important Dates list from this school year calendar."),
	}
	var out pdfExtraction
	err := retryOnce(ctx, "calendar import: asking for the important dates again",
		func() (string, error) {
			out = pdfExtraction{}
			return ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out)
		},
		func(raw string) error {
			if schoolYearForm.MatchString(out.Year) && len(out.Entries) > 0 {
				return nil
			}
			return fmt.Errorf("the list came back with year %q and %d entries; the answer was %s", out.Year, len(out.Entries), raw)
		})
	return out, err
}

func extractPDF(ctx context.Context, client anthropic.Client, pdf []byte, roster model.Roster, dayTypes []string) (pdfExtraction, error) {
	rendered, err := renderPage(pdf)
	if err != nil {
		return pdfExtraction{}, fmt.Errorf("render the calendar page: %w", err)
	}
	page := enhance(rendered)
	crops, rects, err := monthCrops(page)
	if err != nil {
		return pdfExtraction{}, err
	}
	for i, rect := range rects {
		slog.InfoContext(ctx, "calendar import: pdf grid", "grid", i+1, "rect", rect.String())
	}
	pageBlock, err := imageBlock(page)
	if err != nil {
		return pdfExtraction{}, err
	}
	legend, err := extractLegend(ctx, client, pageBlock, dayTypes)
	if err != nil {
		return pdfExtraction{}, err
	}
	out, err := extractEntries(ctx, client, pdf, roster, dayTypes)
	if err != nil {
		return out, err
	}
	match := schoolYearForm.FindStringSubmatch(out.Year)
	if match == nil {
		return out, fmt.Errorf("extracted year %q is not YYYY-YYYY", out.Year)
	}
	startYear, _ := strconv.Atoi(match[1])
	months, errs := fanOut(12, func(i int) ([]shadedDay, error) {
		month := time.Date(startYear, time.July+time.Month(i), 1, 0, 0, 0, 0, model.Location)
		return extractMonth(ctx, client, crops[i], month, legend)
	})
	for i := range 12 {
		if errs[i] != nil {
			return out, errs[i]
		}
		out.Shaded = append(out.Shaded, months[i]...)
	}
	return out, nil
}

func (r *run) readPDF(ctx context.Context, pdf []byte, hash string) ([]map[string]string, string, error) {
	extraction, err := extractPDF(ctx, r.client, pdf, r.roster, r.dayTypes)
	if err != nil {
		return nil, "", err
	}
	rows, err := r.pdfRows(ctx, extraction, hash)
	if err != nil {
		return nil, "", err
	}
	return rows, extraction.Year, nil
}

func (r *run) pdfRows(ctx context.Context, extraction pdfExtraction, hash string) ([]map[string]string, error) {
	roster := r.roster
	match := schoolYearForm.FindStringSubmatch(extraction.Year)
	if match == nil {
		return nil, fmt.Errorf("extracted year %q is not YYYY-YYYY", extraction.Year)
	}
	first, _ := strconv.Atoi(match[1])
	second, _ := strconv.Atoi(match[2])
	if second != first+1 {
		return nil, fmt.Errorf("extracted year %q is not consecutive", extraction.Year)
	}
	rows := []map[string]string{}
	keys := map[string]bool{}
	firstDays, lastDays := 0, 0
	entries := slices.Clone(extraction.Entries)
	for _, s := range extraction.Shaded {
		if s.DayType == noDayType {
			continue
		}
		entries = append(entries, pdfEntry{Title: s.Legend, Start: s.Date, End: s.Date, DayType: s.DayType, Classrooms: roster.Names(), Marker: noMarker})
	}
	slog.InfoContext(ctx, "calendar import: pdf extracted", "listed", len(extraction.Entries), "shaded", len(extraction.Shaded))
	for _, e := range entries {
		if len(e.Classrooms) == 0 {
			return nil, fmt.Errorf("entry %q names no classrooms", e.Title)
		}
		for _, c := range e.Classrooms {
			if !slices.Contains(roster.Names(), c) {
				return nil, fmt.Errorf("entry %q names %q, which is not a classroom", e.Title, c)
			}
		}
		start, err := time.ParseInLocation(model.DateFormat, e.Start, model.Location)
		if err != nil {
			return nil, fmt.Errorf("entry %q: start %q is not a date", e.Title, e.Start)
		}
		if _, err := time.ParseInLocation(model.DateFormat, e.End, model.Location); err != nil {
			return nil, fmt.Errorf("entry %q: end %q is not a date", e.Title, e.End)
		}
		if got := model.SchoolYear(start); got != extraction.Year {
			return nil, fmt.Errorf("entry %q on %s falls in %s, not %s", e.Title, e.Start, got, extraction.Year)
		}
		key := "pdf/" + extraction.Year + "/" + e.Start + "/" + slug(e.Title)
		for n := 2; keys[key]; n++ {
			key = "pdf/" + extraction.Year + "/" + e.Start + "/" + slug(e.Title) + "-" + strconv.Itoa(n)
		}
		keys[key] = true
		tags, err := r.tagCell(e.Classrooms)
		if err != nil {
			return nil, fmt.Errorf("entry %q: %w", e.Title, err)
		}
		row := map[string]string{
			"Key": key, "Year": extraction.Year, "Start": e.Start, "End": e.End, "Title": collapse(e.Title),
			"Tags": tags, "PDF": hash,
		}
		if e.DayType != noDayType {
			if row["Day Type"], err = r.dayTypeID(e.DayType); err != nil {
				return nil, fmt.Errorf("entry %q: %w", e.Title, err)
			}
		}
		if e.Marker != noMarker {
			row["Marker"] = e.Marker
		}
		switch e.Marker {
		case model.MarkerFirstDay:
			firstDays++
		case model.MarkerLastDay:
			lastDays++
		}
		rows = append(rows, row)
	}
	if firstDays != 1 || lastDays != 1 {
		return nil, fmt.Errorf("extraction has %d first days and %d last days, want one each", firstDays, lastDays)
	}
	return rows, nil
}
