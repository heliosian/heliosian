package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"heliosian/internal/db"
)

const (
	noDayType = "None"
	noMarker  = "None"
	firstDay  = "First Day"
	lastDay   = "Last Day"
	unfilled  = "none"
	otherFill = "other"
)

var schoolYearForm = regexp.MustCompile(`^(\d{4})-(\d{4})$`)

type pdfEntry struct {
	Title      string   `json:"title"`
	Start      string   `json:"start"`
	End        string   `json:"end"`
	DayType    string   `json:"dayType"`
	Classrooms []string `json:"classrooms"`
	Marker     string   `json:"marker"`
}

type pdfExtraction struct {
	Year    string     `json:"year"`
	Entries []pdfEntry `json:"entries"`
	Shaded  []db.ShadedDay
}

type legendEntry struct {
	Label   string `json:"label"`
	Color   string `json:"color"`
	DayType string `json:"dayType"`
}

const legendSystem = `You are reading an image of a school's one-page year calendar. Somewhere on the page, usually near the bottom, is a legend: small color swatches, each beside a line of text saying what a day cell filled with that color means. Read the legend only.

For each swatch: the exact wording beside it; the swatch's fill color as a hex RGB estimate like #c9b7e6; and the day type the wording means for students, from the allowed list: a day students stay home is "No School", a day they are released early is "Early Dismissal", and a wording that means neither is "None". Headings such as "NO SCHOOL DAYS" or "HALF SCHOOL DAYS" above the swatches are not entries.`

func monthSystem(described string) string {
	return `You are reading one month grid of a school's year calendar. Some day cells have a colored background, and the calendar's legend gives each color a meaning:
` + described + `
Report the month named in the grid's title. Then go through the grid row by row; each row is one week. For every day number printed, report its cell's background: "` + unfilled + `" for a white cell; the legend wording whose color the fill matches when it is one of the legend's colors; "` + otherFill + `" for a fill in a color the legend does not name, such as orange or yellow. Some cells have a thick colored border drawn around them; a border is a box on top of the cell, not its fill, so report the color inside the border: a cell that is outlined, bolded, or circled but white inside is "` + unfilled + `", and a cell with a legend color inside an orange or black border is that legend color.`
}

func entriesSystem(v *db.Vocabulary) string {
	return v.Glossary() + `
You are reading the school's one-page year calendar PDF: twelve month grids, a legend, and an Important Dates list. Read the Important Dates list into entries.

- One entry for every line of the list, with its exact wording as the title and the date range it gives. A range like "Aug 24-Sep 2" runs across the month boundary. "Mar 11+18" is two entries, one per day.
- dayType comes only from the wording: a dismissal note such as "12:30 dismissal", "12:30p Dismissals", or "half day" means "Early Dismissal"; wording that says school is closed means "No School"; anything else is "None". Ignore the color a line is printed in. An evening event's hours are not a dismissal note.
- classrooms is every classroom the entry applies to: all of them unless the wording limits it, such as "(K only)", which is the kindergarten classroom alone.
- marker is "First Day" on the first day of school for students, "Last Day" on the last day of school, and "None" otherwise.
- year is the school year in the title, written as YYYY-YYYY. Dates are YYYY-MM-DD; the year of each date follows from which side of the winter break the month is on.`
}

func readingHash(v *db.Vocabulary) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{db.CalendarModel, legendSystem, entriesSystem(v), monthSystem("")}, "\x00")))
	return hex.EncodeToString(sum[:])[:16]
}

func enumOf(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

func pdfDocument(pdf []byte) anthropic.ContentBlockParamUnion {
	return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(pdf)})
}

func extractLegend(ctx context.Context, client anthropic.Client, page anthropic.ContentBlockParamUnion, dayTypes []string) ([]legendEntry, error) {
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
	content := []anthropic.ContentBlockParamUnion{page, anthropic.NewTextBlock("Read the legend of this school year calendar.")}
	if _, err := db.Ask(ctx, client, legendSystem, content, schema, &out); err != nil {
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

func extractMonth(ctx context.Context, client anthropic.Client, crop anthropic.ContentBlockParamUnion, month time.Time, legend []legendEntry) ([]db.ShadedDay, error) {
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
	schema := map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties}
	content := []anthropic.ContentBlockParamUnion{
		crop,
		anthropic.NewTextBlock("This should be the " + strings.ToUpper(month.Format("January")) + " grid, the month of " + name + ", which has " + strconv.Itoa(last) + " days. Report the month in its title and the background of every day cell."),
	}
	meaning := func(fill string) string {
		for _, e := range legend {
			if strings.EqualFold(e.Label, fill) && e.DayType != noDayType {
				return e.DayType
			}
		}
		return ""
	}
	read := func() (map[int]string, error) {
		out := map[string]string{}
		if _, err := db.Ask(ctx, client, system, content, schema, &out); err != nil {
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
	readings, errs := db.FanOut(2, func(int) (map[int]string, error) { return read() })
	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	disputed := 0
	for day := 1; day <= last; day++ {
		if meaning(readings[0][day]) != meaning(readings[1][day]) {
			disputed++
			slog.WarnContext(ctx, "calendar import: pdf readings differ", "month", name, "day", day, "first", readings[0][day], "second", readings[1][day])
		}
	}
	if disputed > 0 {
		slog.WarnContext(ctx, "calendar import: reading the month a third time", "month", name, "disputed", disputed)
		third, err := read()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		readings = append(readings, third)
	}
	days := []db.ShadedDay{}
	for day := 1; day <= last; day++ {
		votes := map[string]int{}
		for _, r := range readings {
			votes[meaning(r[day])]++
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
		days = append(days, db.ShadedDay{Date: time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, db.School).Format(db.DateLayout), DayType: chosen})
	}
	slog.InfoContext(ctx, "calendar import: pdf month read", "month", name, "filled", len(days), "readings", len(readings))
	return days, nil
}

func extractEntries(ctx context.Context, client anthropic.Client, pdf []byte, v *db.Vocabulary) (pdfExtraction, error) {
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
						"dayType":    enumOf(append([]string{noDayType}, v.DayTypeNames()...)),
						"classrooms": map[string]any{"type": "array", "minItems": 1, "items": enumOf(v.ClassroomNames())},
						"marker":     enumOf([]string{noMarker, firstDay, lastDay}),
					},
				},
			},
		},
	}
	content := []anthropic.ContentBlockParamUnion{pdfDocument(pdf), anthropic.NewTextBlock("Extract every entry of the Important Dates list from this school year calendar.")}
	for attempt := 1; ; attempt++ {
		var out pdfExtraction
		raw, err := db.Ask(ctx, client, entriesSystem(v), content, schema, &out)
		if err != nil {
			return out, err
		}
		if schoolYearForm.MatchString(out.Year) && len(out.Entries) > 0 {
			return out, nil
		}
		if attempt == 2 {
			return out, fmt.Errorf("the list came back with year %q and %d entries; the answer was %s", out.Year, len(out.Entries), raw)
		}
		slog.WarnContext(ctx, "calendar import: asking for the important dates again")
	}
}

func extractPDF(ctx context.Context, client anthropic.Client, pdf []byte, v *db.Vocabulary) (pdfExtraction, error) {
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
	legend, err := extractLegend(ctx, client, pageBlock, v.DayTypeNames())
	if err != nil {
		return pdfExtraction{}, err
	}
	out, err := extractEntries(ctx, client, pdf, v)
	if err != nil {
		return out, err
	}
	startYear, _ := strconv.Atoi(schoolYearForm.FindStringSubmatch(out.Year)[1])
	months, errs := db.FanOut(12, func(i int) ([]db.ShadedDay, error) {
		return extractMonth(ctx, client, crops[i], time.Date(startYear, time.July+time.Month(i), 1, 0, 0, 0, 0, db.School), legend)
	})
	for i := range 12 {
		if errs[i] != nil {
			return out, errs[i]
		}
		out.Shaded = append(out.Shaded, months[i]...)
	}
	return out, nil
}

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
}

func yearCalendar(x pdfExtraction, v *db.Vocabulary, reading string) (db.YearCalendar, error) {
	match := schoolYearForm.FindStringSubmatch(x.Year)
	if match == nil {
		return db.YearCalendar{}, fmt.Errorf("extracted year %q is not YYYY-YYYY", x.Year)
	}
	first, _ := strconv.Atoi(match[1])
	second, _ := strconv.Atoi(match[2])
	if second != first+1 {
		return db.YearCalendar{}, fmt.Errorf("extracted year %q is not consecutive", x.Year)
	}
	cal := db.YearCalendar{Year: x.Year, Hash: reading}
	keys := map[string]bool{}
	firstDays, lastDays := 0, 0
	for _, e := range x.Entries {
		for _, date := range []string{e.Start, e.End} {
			if _, err := time.ParseInLocation(db.DateLayout, date, db.School); err != nil {
				return db.YearCalendar{}, fmt.Errorf("entry %q: %q is not a date", e.Title, date)
			}
		}
		if got := db.SchoolYear(e.Start); got != x.Year {
			return db.YearCalendar{}, fmt.Errorf("entry %q on %s falls in %s, not %s", e.Title, e.Start, got, x.Year)
		}
		key := "pdf/" + x.Year + "/" + e.Start + "/" + slug(e.Title)
		for n := 2; keys[key]; n++ {
			key = "pdf/" + x.Year + "/" + e.Start + "/" + slug(e.Title) + "-" + strconv.Itoa(n)
		}
		keys[key] = true
		entry := db.YearEntry{Key: key, Title: db.Collapse(e.Title), Start: e.Start, End: e.End}
		for _, name := range e.Classrooms {
			id, ok := v.ClassroomID(name)
			if !ok {
				return db.YearCalendar{}, fmt.Errorf("entry %q names %q, which is not a classroom", e.Title, name)
			}
			entry.Classrooms = append(entry.Classrooms, id)
		}
		if e.DayType != noDayType {
			entry.DayType = e.DayType
		}
		switch e.Marker {
		case firstDay:
			entry.Marker = "first_day"
			firstDays++
		case lastDay:
			entry.Marker = "last_day"
			lastDays++
		}
		cal.Entries = append(cal.Entries, entry)
	}
	if firstDays != 1 || lastDays != 1 {
		return db.YearCalendar{}, fmt.Errorf("extraction has %d first days and %d last days, want one each", firstDays, lastDays)
	}
	cal.Shaded = x.Shaded
	return cal, nil
}

func renderPage(pdf []byte) (image.Image, error) {
	dir, err := os.MkdirTemp("", "calendar")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "page.pdf")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return nil, err
	}
	cmd := exec.Command("pdftoppm", "-png", "-r", "300", "-singlefile", "-f", "1", "-l", "1", in, filepath.Join(dir, "page"))
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w", err)
	}
	f, err := os.Open(filepath.Join(dir, "page.png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func hsl(r, g, b float64) (float64, float64, float64) {
	hi, lo := max(r, g, b), min(r, g, b)
	l := (hi + lo) / 2
	if hi == lo {
		return 0, 0, l
	}
	d := hi - lo
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, l
}

func rgb(h, s, l float64) (float64, float64, float64) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}

func enhance(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	out := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			fr, fg, fb := float64(r)/65535, float64(g)/65535, float64(b)/65535
			h, s, l := hsl(fr, fg, fb)
			if s > 0.15 && l > 0.2 && l < 0.97 {
				fr, fg, fb = rgb(h, 1, 0.55)
			}
			out.Set(x, y, color.RGBA{uint8(fr*255 + 0.5), uint8(fg*255 + 0.5), uint8(fb*255 + 0.5), 255})
		}
	}
	return out
}

func imageBlock(img image.Image) (anthropic.ContentBlockParamUnion, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return anthropic.ContentBlockParamUnion{}, err
	}
	return anthropic.NewImageBlockBase64("image/png", base64.StdEncoding.EncodeToString(buf.Bytes())), nil
}

func titleBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	h, s, l := hsl(float64(r)/65535, float64(g)/65535, float64(b)/65535)
	return s > 0.8 && h > 195 && h < 235 && l > 0.4 && l < 0.7
}

func titleBars(page *image.RGBA) []image.Rectangle {
	bounds := page.Bounds()
	minRun := bounds.Dx() / 10
	bars := []image.Rectangle{}
	place := func(run image.Rectangle) {
		for i, bar := range bars {
			if run.Min.Y-bar.Max.Y <= 3 && run.Min.X < bar.Max.X && bar.Min.X < run.Max.X {
				bars[i] = bar.Union(run)
				return
			}
		}
		bars = append(bars, run)
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		start := -1
		for x := bounds.Min.X; x <= bounds.Max.X; x++ {
			blue := x < bounds.Max.X && titleBlue(page.At(x, y))
			if blue && start < 0 {
				start = x
			}
			if !blue && start >= 0 {
				if x-start >= minRun {
					place(image.Rect(start, y, x, y+1))
				}
				start = -1
			}
		}
	}
	tall := []image.Rectangle{}
	for _, bar := range bars {
		if bar.Dy() >= bounds.Dy()/150 {
			tall = append(tall, bar)
		}
	}
	return tall
}

func monthCrops(page *image.RGBA) ([]anthropic.ContentBlockParamUnion, []image.Rectangle, error) {
	bars := titleBars(page)
	if len(bars) != 12 {
		return nil, nil, fmt.Errorf("found %d month title bars on the page, want 12: %v", len(bars), bars)
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Min.X < bars[j].Min.X })
	columns := [][]image.Rectangle{{bars[0]}}
	for _, bar := range bars[1:] {
		last := columns[len(columns)-1]
		if bar.Min.X-last[0].Min.X > page.Bounds().Dx()/4 {
			columns = append(columns, []image.Rectangle{})
		}
		columns[len(columns)-1] = append(columns[len(columns)-1], bar)
	}
	if len(columns) != 2 || len(columns[0]) != 6 || len(columns[1]) != 6 {
		return nil, nil, fmt.Errorf("the month title bars are not two columns of six")
	}
	margin := page.Bounds().Dx() / 250
	crops := []anthropic.ContentBlockParamUnion{}
	rects := []image.Rectangle{}
	for _, column := range columns {
		sort.Slice(column, func(i, j int) bool { return column[i].Min.Y < column[j].Min.Y })
		for i, bar := range column {
			top := bar.Min.Y - margin
			bottom := 0
			if i+1 < len(column) {
				bottom = column[i+1].Min.Y - margin
			} else {
				bottom = top + (bar.Min.Y - column[i-1].Min.Y)
			}
			rect := image.Rect(bar.Min.X-margin, top, bar.Max.X+margin, bottom).Intersect(page.Bounds())
			out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
			draw.Draw(out, out.Bounds(), page, rect.Min, draw.Src)
			block, err := imageBlock(out)
			if err != nil {
				return nil, nil, err
			}
			crops = append(crops, block)
			rects = append(rects, rect)
		}
	}
	return crops, rects, nil
}
