// Package calendarimport is the periodic sync's calendar stage: it pulls the school's public calendar feed and its published year calendar into the Calendar sheet, and has Claude classify every event.
package calendarimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/app"
	"heliosian/internal/calendar"
	"heliosian/internal/data"
	"heliosian/internal/sheetsync"
	"heliosian/internal/who"
)

const (
	pageURL   = calendar.SchoolCalendarPage
	modelName = "claude-fable-5-1"
	actor     = "calendarimport"
	batchSize = 10
	noDayType = "None"
	noMarker  = "None"
)

var legendDayTypes = []string{"No School", "Early Dismissal"}

// Options is what one run needs: the sheets it reads and writes, the
// Calendar API client the school's calendar is read through, the directory
// model already loaded for the audience vocabulary, the Claude key, and
// whether to write anything.
type Options struct {
	Source        *data.Sheet
	Sheets        *sheets.Service
	Calendar      *gcal.Service
	CalendarSheet string
	Directory     *who.Model
	AnthropicKey  string
	DryRun        bool
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
		log.Printf("get %s: %s (retry-after %q), retrying in %s", address, resp.Status, resp.Header.Get("Retry-After"), retryWaits[attempt])
		time.Sleep(retryWaits[attempt])
	}
}

var pdfLink = regexp.MustCompile(`href="([^"]+\.pdf)"`)

func findPDF(page []byte) (string, error) {
	match := pdfLink.FindSubmatch(page)
	if match == nil {
		return "", fmt.Errorf("no pdf link on %s", pageURL)
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return "", err
	}
	link, err := url.Parse(html.UnescapeString(string(match[1])))
	if err != nil {
		return "", err
	}
	return base.ResolveReference(link).String(), nil
}

func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

const lineBreak = "\x00"

var blockTags = map[string]bool{
	"p": true, "div": true, "li": true, "blockquote": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// flatten turns a description into plain text, keeping paragraph breaks and
// the address behind every link, since a Zoom link is the point of some.
func flatten(text string) (string, error) {
	if strings.Contains(text, "<") {
		parent := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		nodes, err := html.ParseFragment(strings.NewReader(text), parent)
		if err != nil {
			return "", err
		}
		out := &strings.Builder{}
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.TextNode {
				out.WriteString(n.Data)
			}
			if n.Type == html.ElementNode && n.Data == "br" {
				out.WriteString(lineBreak)
			}
			before := out.Len()
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" && attr.Val != "" && !strings.Contains(out.String()[before:], attr.Val) {
						out.WriteString(" " + attr.Val)
					}
				}
			}
			if n.Type == html.ElementNode && blockTags[n.Data] {
				out.WriteString(lineBreak)
			}
		}
		for _, n := range nodes {
			walk(n)
		}
		text = strings.ReplaceAll(out.String(), lineBreak, "\n")
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = collapse(line)
	}
	flat := strings.Join(lines, "\n")
	for strings.Contains(flat, "\n\n\n") {
		flat = strings.ReplaceAll(flat, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(flat), nil
}

// window is the stretch of the feed worth carrying: from the start of the
// previous school year, three years on.
func window(now time.Time) (time.Time, time.Time) {
	start := now.Year() - 1
	if now.Month() < time.July {
		start--
	}
	from := time.Date(start, time.July, 1, 0, 0, 0, 0, calendar.Location)
	return from, from.AddDate(3, 0, 0)
}

const feedFields = googleapi.Field("nextPageToken,items(iCalUID,recurringEventId,originalStartTime,start,end,summary,location,description,updated,sequence,status)")

// feedRows reads every event instance of the school's calendar starting
// within [from, to) through the Calendar API, which expands repeating events
// and applies their overrides itself.
func feedRows(ctx context.Context, svc *gcal.Service, from, to time.Time) ([]map[string]string, error) {
	rows := []map[string]string{}
	call := svc.Events.List(calendar.SchoolCalendarID).Context(ctx).
		SingleEvents(true).OrderBy("startTime").MaxResults(2500).
		TimeMin(from.Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339)).
		Fields(feedFields)
	for {
		page, err := call.Do()
		if err != nil {
			return nil, err
		}
		for _, e := range page.Items {
			row, start, err := feedRow(e)
			if err != nil {
				return nil, err
			}
			if e.Status == "cancelled" || start.Before(from) || !start.Before(to) {
				continue
			}
			rows = append(rows, row)
		}
		if page.NextPageToken == "" {
			return rows, nil
		}
		call.PageToken(page.NextPageToken)
	}
}

// eventTime is a start or end as school wall-clock, and whether it is a date
// alone. An all-day end arrives exclusive, as the feed always stated it.
func eventTime(t *gcal.EventDateTime) (time.Time, bool, error) {
	if t == nil {
		return time.Time{}, false, fmt.Errorf("event with no time")
	}
	if t.Date != "" {
		day, err := time.ParseInLocation(calendar.DateFormat, t.Date, calendar.Location)
		return day, true, err
	}
	at, err := time.Parse(time.RFC3339, t.DateTime)
	return at.In(calendar.Location), false, err
}

// instanceKey is the key a repeating event's instance has always had in the
// sheet: the UID and the instance's original start as school wall-clock.
func instanceKey(uid string, t time.Time, allDay bool) string {
	if allDay {
		return uid + "/" + t.Format("20060102")
	}
	return uid + "/" + t.Format("20060102T150405")
}

func feedRow(e *gcal.Event) (map[string]string, time.Time, error) {
	key := e.ICalUID
	if e.RecurringEventId != "" {
		orig, origAllDay, err := eventTime(e.OriginalStartTime)
		if err != nil {
			return nil, time.Time{}, fmt.Errorf("event %s original start: %w", key, err)
		}
		key = instanceKey(key, orig, origAllDay)
	}
	start, allDay, err := eventTime(e.Start)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s start: %w", key, err)
	}
	end, _, err := eventTime(e.End)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s end: %w", key, err)
	}
	updated, err := time.Parse(time.RFC3339, e.Updated)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s updated: %w", key, err)
	}
	description, err := flatten(e.Description)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("event %s description: %w", key, err)
	}
	row := map[string]string{
		"Key":         key,
		"Title":       collapse(e.Summary),
		"Location":    collapse(e.Location),
		"Description": description,
		"Updated":     updated.In(calendar.Location).Format(calendar.DateTimeFormat),
		"Sequence":    strconv.FormatInt(e.Sequence, 10),
	}
	if allDay {
		end = end.AddDate(0, 0, -1)
		if end.Before(start) {
			end = start
		}
		row["Start"], row["End"] = start.Format(calendar.DateFormat), end.Format(calendar.DateFormat)
	} else {
		row["Start"], row["End"] = start.Format(calendar.DateTimeFormat), end.Format(calendar.DateTimeFormat)
	}
	if row["Title"] == "" {
		row["Title"] = "(untitled)"
	}
	return row, start, nil
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return fmt.Sprintf("%x", sum)[:16]
}

var slugChars = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(slugChars.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
}

// ask sends one structured request and decodes the answer into out, returning
// the answer's text as well.
func ask(ctx context.Context, client anthropic.Client, system string, content []anthropic.ContentBlockParamUnion, schema map[string]any, effort anthropic.OutputConfigEffort, out any) (string, error) {
	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     modelName,
		MaxTokens: 64000,
		System: []anthropic.TextBlockParam{{
			Text:         system,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		}},
		Messages:     []anthropic.MessageParam{anthropic.NewUserMessage(content...)},
		OutputConfig: anthropic.OutputConfigParam{Effort: effort, Format: anthropic.JSONOutputFormatParam{Schema: schema}},
	})
	resp := anthropic.Message{}
	for stream.Next() {
		if err := resp.Accumulate(stream.Current()); err != nil {
			return "", err
		}
	}
	if err := stream.Err(); err != nil {
		return "", err
	}
	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", fmt.Errorf("claude refused: %s %s", resp.StopDetails.Category, resp.StopDetails.Explanation)
	}
	if resp.StopReason != anthropic.StopReasonEndTurn {
		return "", fmt.Errorf("claude stopped early: %s", resp.StopReason)
	}
	text := &strings.Builder{}
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	log.Printf("claude: %d input tokens (%d from cache), %d output tokens", resp.Usage.InputTokens+resp.Usage.CacheReadInputTokens+resp.Usage.CacheCreationInputTokens, resp.Usage.CacheReadInputTokens, resp.Usage.OutputTokens)
	if err := json.Unmarshal([]byte(text.String()), out); err != nil {
		return "", fmt.Errorf("decode claude's answer: %w: %s", err, text.String())
	}
	return text.String(), nil
}

func enumOf(values []string) map[string]any {
	return map[string]any{"type": "string", "enum": values}
}

// glossary explains the school's names to the model, built from the roster
// so a renamed classroom needs no code change.
func glossary(roster calendar.Roster) string {
	b := &strings.Builder{}
	b.WriteString("Helios School is a K-8 school. Students belong to a homeroom classroom named for a bird. Two classrooms make a grade band whose name is a portmanteau of the two classroom names. Lower School is Kindergarten through Grade 4 and Middle School is Grade 5 through Grade 8.\n\nBands and their classrooms:\n")
	bands := []string{}
	for _, c := range roster.Classrooms {
		if c.Band != "" && !slices.Contains(bands, c.Band) {
			bands = append(bands, c.Band)
		}
	}
	for _, band := range bands {
		parts := []string{}
		for _, c := range roster.Classrooms {
			if c.Band == band {
				parts = append(parts, c.Name+" ("+strings.Join(c.Grades, ", ")+")")
			}
		}
		fmt.Fprintf(b, "- %s = %s\n", band, strings.Join(parts, " + "))
	}
	b.WriteString("\nClassrooms with crews (a crew is a smaller group within a classroom, written as crew then classroom):\n")
	for _, c := range roster.Classrooms {
		if len(c.Crews) > 0 {
			fmt.Fprintf(b, "- %s: %s\n", c.Name, strings.Join(c.Crews, ", "))
		}
	}
	b.WriteString(`
Vocabulary seen in event titles:
- K or Kinder means Kindergarten. "1/2", "3/4", "5/6", "7/8" name grade pairs, which are bands. "8th Grade" means Grade 8. LS is Lower School, MS is Middle School.
- A singular band name (Cosprey, Hegret, Jayven, Halcon) means the band.
- CoL or COL is a Celebration of Learning, a showcase where students present their work to families.
- ILP is an Individual Learning Plan conference between a family and teachers.
- MAP is the MAP Growth assessment students take.
- BTSN or Back to School Night is an evening for parents.
- CAFE is a morning coffee for the parents of one band or classroom.
- Beacon Wellness is the school's counseling program; its chats are for parents.
- HCA is the Helios Community Association, the parent association; its events are for parents and families.
- PD or Professional Development is a day or half day staff work while students stay home or leave early.
- Intersession is a week of alternative programming, still a school day.
- Passion Projects are Middle School project weeks.
- Aftercare is the after-school care program.
`)
	return b.String()
}

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

// legendEntry is one swatch of the calendar's legend as the model read it:
// the wording beside it, the color it estimated, and the day type the
// wording means. The school changes the colors and the wording from year to
// year, so nothing about the legend is fixed in code.
type legendEntry struct {
	Label   string `json:"label"`
	Color   string `json:"color"`
	DayType string `json:"dayType"`
}

const (
	unfilled  = "none"
	otherFill = "other"
)

func pdfDocument(pdf []byte) anthropic.ContentBlockParamUnion {
	return anthropic.NewDocumentBlock(anthropic.Base64PDFSourceParam{Data: base64.StdEncoding.EncodeToString(pdf)})
}

// renderPage rasterizes the calendar's page with poppler at 300 dpi. The
// model reads a month grid reliably only when its cells arrive large, and
// the API scales any page it is handed down to about 1500 pixels, so the
// page is rendered here and cut up before it is sent.
func renderPage(pdf []byte) (image.Image, error) {
	dir, err := os.MkdirTemp("", "calendarimport")
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

// enhance makes every filled cell unmistakable before the model sees it. The
// legend's fills are pale tints, pastel enough that the model reads pink as
// white now and then; every pixel with any color in it is pushed to full
// saturation at middle lightness, so a fill becomes a strong flat hue while
// white cells, black text, and gray rules, which carry no color, stay as
// they are.
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

// titleBlue is the enhanced page's month title bar: a saturated blue at the
// middle lightness enhance puts every fill at.
func titleBlue(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	h, s, l := hsl(float64(r)/65535, float64(g)/65535, float64(b)/65535)
	return s > 0.8 && h > 195 && h < 235 && l > 0.4 && l < 0.7
}

// titleBars finds the month title bars on the enhanced page: horizontal bands
// of the title blue at least a tenth of the page wide and taller than a
// drawn outline, which no day cell or box is.
// Each band anchors one grid. The count and layout are checked by the
// caller, and every month read checks the title in its crop, so a page laid
// out differently fails loudly rather than reading the wrong month.
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

// monthCrops cuts the twelve grids out of the enhanced page, July to
// December down the first column and January to June down the second: each
// from its title bar to the next bar in its column, the last in a column as
// tall as the one above it.
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

const legendSystem = `You are reading an image of a school's one-page year calendar. Somewhere on the page, usually near the bottom, is a legend: small color swatches, each beside a line of text saying what a day cell filled with that color means. Read the legend only.

For each swatch: the exact wording beside it; the swatch's fill color as a hex RGB estimate like #c9b7e6; and the day type the wording means for students, from the allowed list: a day students stay home is "No School", a day they are released early is "Early Dismissal", and a wording that means neither is "None". Headings such as "NO SCHOOL DAYS" or "HALF SCHOOL DAYS" above the swatches are not entries.`

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
		log.Printf("pdf: legend %s %q means %s", e.Color, e.Label, e.DayType)
	}
	return out.Legend, nil
}

func monthSystem(described string) string {
	return `You are reading one month grid of a school's year calendar. Some day cells have a colored background, and the calendar's legend gives each color a meaning:
` + described + `
Report the month named in the grid's title. Then go through the grid row by row; each row is one week. For every day number printed, report its cell's background: "` + unfilled + `" for a white cell; the legend wording whose color the fill matches when it is one of the legend's colors; "` + otherFill + `" for a fill in a color the legend does not name, such as orange or yellow. Some cells have a thick colored border drawn around them; a border is a box on top of the cell, not its fill, so report the color inside the border: a cell that is outlined, bolded, or circled but white inside is "` + unfilled + `", and a cell with a legend color inside an orange or black border is that legend color.`
}

// extractMonth reads one month's grid with the legend in hand, naming every
// day cell's fill, read twice and compared, with a third read to settle a
// disagreement.
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
	// Every day of the month is a required property, so the answer cannot end
	// before every cell has been read.
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
	// meaning is what a fill does to the day: a day type, or nothing for a
	// white cell, a color the legend does not name, or a legend entry that
	// means no day type. Readings are compared on this, since two legend
	// colors that both mean No School disagreeing is no disagreement.
	meaning := func(fill string) string {
		for label, t := range dayType {
			if strings.EqualFold(label, fill) && t != noDayType {
				return t
			}
		}
		return ""
	}
	// read is one independent pass over the grid: the fill of every day.
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
	readings := make([]map[int]string, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range readings {
		wg.Go(func() { readings[i], errs[i] = read() })
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	disputed := []int{}
	for day := 1; day <= last; day++ {
		if meaning(readings[0][day]) != meaning(readings[1][day]) {
			disputed = append(disputed, day)
			log.Printf("  [WARN] %s %d read as %q and as %q", name, day, readings[0][day], readings[1][day])
		}
	}
	if len(disputed) > 0 {
		log.Printf("  [WARN] %s: the two readings differ on %d days; reading a third time", name, len(disputed))
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
			Date:    time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, calendar.Location).Format(calendar.DateFormat),
			Legend:  fill[chosen],
			DayType: chosen,
		})
	}
	log.Printf("pdf: %s has %d filled cells from %d readings", name, len(days), len(readings))
	return days, nil
}

// extractEntries reads the Important Dates list. The list's colors are
// categories rather than day types, so the read takes a day type only from
// the wording; the grid supplies the rest afterwards.
func entriesSystem(roster calendar.Roster) string {
	return glossary(roster) + `
You are reading the school's one-page year calendar PDF: twelve month grids, a legend, and an Important Dates list. Read the Important Dates list into entries.

- One entry for every line of the list, with its exact wording as the title and the date range it gives. A range like "Aug 24-Sep 2" runs across the month boundary. "Mar 11+18" is two entries, one per day.
- dayType comes only from the wording: a dismissal note such as "12:30 dismissal", "12:30p Dismissals", or "half day" means "Early Dismissal"; wording that says school is closed means "No School"; anything else is "None". Ignore the color a line is printed in. An evening event's hours are not a dismissal note.
- classrooms is every classroom the entry applies to: all of them unless the wording limits it, such as "(K only)", which is the kindergarten classroom alone.
- marker is "First Day" on the first day of school for students, "Last Day" on the last day of school, and "None" otherwise.
- year is the school year in the title, written as YYYY-YYYY. Dates are YYYY-MM-DD; the year of each date follows from which side of the winter break the month is on.`
}

func extractEntries(ctx context.Context, client anthropic.Client, pdf []byte, roster calendar.Roster, dayTypes []string) (pdfExtraction, error) {
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
						"marker":     enumOf([]string{noMarker, calendar.MarkerFirstDay, calendar.MarkerLastDay}),
					},
				},
			},
		},
	}
	content := []anthropic.ContentBlockParamUnion{
		pdfDocument(pdf),
		anthropic.NewTextBlock("Extract every entry of the Important Dates list from this school year calendar."),
	}
	yearForm := regexp.MustCompile(`^\d{4}-\d{4}$`)
	for attempt := 1; ; attempt++ {
		var out pdfExtraction
		raw, err := ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out)
		if err != nil {
			return out, err
		}
		if yearForm.MatchString(out.Year) && len(out.Entries) > 0 {
			return out, nil
		}
		err = fmt.Errorf("the list came back with year %q and %d entries; the answer was %s", out.Year, len(out.Entries), raw)
		if attempt == 2 {
			return out, err
		}
		log.Printf("  [WARN] %v; asking again", err)
	}
}

func extractPDF(ctx context.Context, client anthropic.Client, pdf []byte, roster calendar.Roster, dayTypes []string) (pdfExtraction, error) {
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
		log.Printf("pdf: grid %d at %v", i+1, rect)
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
	yearForm := regexp.MustCompile(`^(\d{4})-\d{4}$`)
	match := yearForm.FindStringSubmatch(out.Year)
	if match == nil {
		return out, fmt.Errorf("extracted year %q is not YYYY-YYYY", out.Year)
	}
	startYear, _ := strconv.Atoi(match[1])
	months := make([][]shadedDay, 12)
	errs := make([]error, 12)
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Go(func() {
			month := time.Date(startYear, time.July+time.Month(i), 1, 0, 0, 0, 0, calendar.Location)
			months[i], errs[i] = extractMonth(ctx, client, crops[i], month, legend)
		})
	}
	wg.Wait()
	for i := range 12 {
		if errs[i] != nil {
			return out, errs[i]
		}
		out.Shaded = append(out.Shaded, months[i]...)
	}
	return out, nil
}

// readPDF is the whole PDF stage: the rows for the school year the document
// covers, and that year's label.
func readPDF(ctx context.Context, client anthropic.Client, pdf []byte, hash string, roster calendar.Roster, dayTypes []string) ([]map[string]string, string, error) {
	extraction, err := extractPDF(ctx, client, pdf, roster, dayTypes)
	if err != nil {
		return nil, "", err
	}
	rows, err := pdfRows(extraction, hash, roster)
	if err != nil {
		return nil, "", err
	}
	return rows, extraction.Year, nil
}

func pdfRows(extraction pdfExtraction, hash string, roster calendar.Roster) ([]map[string]string, error) {
	yearForm := regexp.MustCompile(`^(\d{4})-(\d{4})$`)
	match := yearForm.FindStringSubmatch(extraction.Year)
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
	// The list's entries and the grid's shaded cells are two sets of facts and
	// become two sets of rows: an entry says what happens and to whom, a cell
	// says what kind of day it is for the whole school.
	entries := slices.Clone(extraction.Entries)
	for _, s := range extraction.Shaded {
		if s.DayType == noDayType {
			continue
		}
		entries = append(entries, pdfEntry{Title: s.Legend, Start: s.Date, End: s.Date, DayType: s.DayType, Classrooms: roster.Names(), Marker: noMarker})
	}
	log.Printf("pdf: %d listed entries, %d shaded days", len(extraction.Entries), len(extraction.Shaded))
	for _, e := range entries {
		if len(e.Classrooms) == 0 {
			return nil, fmt.Errorf("entry %q names no classrooms", e.Title)
		}
		for _, c := range e.Classrooms {
			if !slices.Contains(roster.Names(), c) {
				return nil, fmt.Errorf("entry %q names %q, which is not a classroom", e.Title, c)
			}
		}
		start, err := time.ParseInLocation(calendar.DateFormat, e.Start, calendar.Location)
		if err != nil {
			return nil, fmt.Errorf("entry %q: start %q is not a date", e.Title, e.Start)
		}
		if _, err := time.ParseInLocation(calendar.DateFormat, e.End, calendar.Location); err != nil {
			return nil, fmt.Errorf("entry %q: end %q is not a date", e.Title, e.End)
		}
		if got := calendar.SchoolYear(start); got != extraction.Year {
			return nil, fmt.Errorf("entry %q on %s falls in %s, not %s", e.Title, e.Start, got, extraction.Year)
		}
		key := "pdf/" + extraction.Year + "/" + e.Start + "/" + slug(e.Title)
		for n := 2; keys[key]; n++ {
			key = "pdf/" + extraction.Year + "/" + e.Start + "/" + slug(e.Title) + "-" + strconv.Itoa(n)
		}
		keys[key] = true
		row := map[string]string{
			"Key": key, "Year": extraction.Year, "Start": e.Start, "End": e.End, "Title": collapse(e.Title),
			"Tags": calendar.JoinList(e.Classrooms), "PDF": hash,
		}
		if e.DayType != noDayType {
			row["Day Type"] = e.DayType
		}
		if e.Marker != noMarker {
			row["Marker"] = e.Marker
		}
		switch e.Marker {
		case calendar.MarkerFirstDay:
			firstDays++
		case calendar.MarkerLastDay:
			lastDays++
		}
		rows = append(rows, row)
	}
	if firstDays != 1 || lastDays != 1 {
		return nil, fmt.Errorf("extraction has %d first days and %d last days, want one each", firstDays, lastDays)
	}
	return rows, nil
}

type enrichInput struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Location    string `json:"location,omitempty"`
	Description string `json:"description,omitempty"`
}

type enrichOutput struct {
	ID       string   `json:"-"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	DayType  string   `json:"dayType"`
	Keywords []string `json:"keywords"`
}

func classifierSystem(roster calendar.Roster, tags []calendar.Tag) string {
	described := &strings.Builder{}
	for _, t := range tags {
		fmt.Fprintf(described, "- %s: %s\n", t.Name, t.Description)
	}
	return glossary(roster) + `
You classify events from the school calendar for a family-facing app. For each event, using only its title, dates, location and description:

- tags: every tag that applies. First the classrooms the event is for, as the narrowest set the event supports: a band's two classrooms when the title names a band, a grade pair, or both classrooms; one classroom when it names one; the Lower School or Middle School classrooms when it says LS or MS; every classroom when nothing narrows it. An event for parents or staff is still for the classrooms whose families or staff it concerns, every classroom when school-wide. Then every one of these that applies:
` + described.String() + `- dayType: for an all-day event only, the day type it imposes on the students it applies to, or "None". "No School" for holidays, breaks and professional development days; "Early Dismissal" for early dismissal and half days; any other listed type only when the title says so plainly. An event with a time of day, or one that merely happens on a school day, is "None".
- keywords: three to eight lowercase search words a parent might type that are in neither the title nor the tags: expansions of abbreviations, synonyms, the occasion, what happens there. Never a classroom, band, or grade, and never a tag.

Answer under every event's id, repeating its title exactly as given.`
}

func enrich(ctx context.Context, client anthropic.Client, inputs []enrichInput, roster calendar.Roster, dayTypes []string, tags []calendar.Tag) ([]enrichOutput, error) {
	names := []string{}
	for _, t := range tags {
		names = append(names, t.Name)
	}
	system := classifierSystem(roster, tags)
	// Events that read the same are asked once: repeats of one event and a
	// break split across weeks would otherwise sit in the batch as near-twins
	// the model conflates.
	sameAs := map[string]string{}
	representatives := []enrichInput{}
	for _, in := range inputs {
		text := digest(in.Title, in.Location, in.Description)
		if first, ok := sameAs[text]; ok {
			sameAs[in.ID] = first
			continue
		}
		sameAs[text] = in.ID
		sameAs[in.ID] = in.ID
		representatives = append(representatives, in)
	}
	// The answer is an object with one required property per event, so a
	// skipped event, a doubled one, or one nobody asked about is impossible
	// by construction rather than something to check for.
	answer := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"title", "tags", "dayType", "keywords"},
		"properties": map[string]any{
			"title":    map[string]any{"type": "string", "description": "the event's title, exactly as given"},
			"tags":     map[string]any{"type": "array", "minItems": 1, "items": enumOf(append(roster.Names(), names...))},
			"dayType":  enumOf(append([]string{noDayType}, dayTypes...)),
			"keywords": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
	}
	sent := make([]enrichInput, len(representatives))
	byHandle := map[string]string{}
	properties := map[string]any{}
	required := []string{}
	for i, in := range representatives {
		handle := "e" + strconv.Itoa(i+1)
		byHandle[handle] = in.ID
		sent[i] = in
		sent[i].ID = handle
		properties[handle] = map[string]any{"$ref": "#/$defs/answer"}
		required = append(required, handle)
	}
	schema := map[string]any{
		"type": "object", "additionalProperties": false, "required": required, "properties": properties,
		"$defs": map[string]any{"answer": answer},
	}
	encoded, err := json.MarshalIndent(sent, "", " ")
	if err != nil {
		return nil, err
	}
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("Classify each of these events:\n\n" + string(encoded)),
	}
	answers := map[string]enrichOutput{}
	for attempt := 1; ; attempt++ {
		out := map[string]enrichOutput{}
		raw, err := ask(ctx, client, system, content, schema, anthropic.OutputConfigEffort("max"), &out)
		if err != nil {
			return nil, err
		}
		err = nil
		for i, in := range representatives {
			o := out["e"+strconv.Itoa(i+1)]
			if collapse(o.Title) != collapse(in.Title) {
				err = fmt.Errorf("claude answered %s with the title %q, not %q; the answer was %s", in.ID, o.Title, in.Title, raw)
				break
			}
			o.ID = in.ID
			answers[in.ID] = o
		}
		if err == nil {
			break
		}
		if attempt == 2 {
			return nil, err
		}
		log.Printf("  [WARN] %v; asking again", err)
	}
	ordered := []enrichOutput{}
	for _, in := range inputs {
		o := answers[sameAs[in.ID]]
		o.ID = in.ID
		ordered = append(ordered, o)
	}
	return ordered, nil
}

func syncTab(svc *sheets.Service, sheet, tab string, header []string, rows []map[string]string, keyCol string, apply bool) (*sheetsync.Result, error) {
	result, err := sheetsync.Sync(svc, sheet, tab, header, rows, keyCol, sheetsync.Mirror, apply)
	if err != nil {
		return nil, err
	}
	log.Printf("%s: %d cells updated, %d rows added, %d removed", tab, len(result.Edits), len(result.Added), len(result.Removed))
	return result, nil
}

func changes(tab string, result *sheetsync.Result, titles map[string]string, stamp string) [][]string {
	rows := [][]string{}
	for _, key := range result.Added {
		rows = append(rows, []string{stamp, actor, "added", tab, key, "Title", "", titles[key]})
	}
	for _, edit := range result.Edits {
		rows = append(rows, []string{stamp, actor, "changed", tab, edit.Key, edit.Column, edit.From, edit.To})
	}
	for _, key := range result.Removed {
		rows = append(rows, []string{stamp, actor, "removed", tab, key, "Title", titles[key], ""})
	}
	return rows
}

func titlesOf(rows []map[string]string, keyCol string) map[string]string {
	out := map[string]string{}
	for _, row := range rows {
		out[row[keyCol]] = row["Title"]
	}
	return out
}

// Run is one import. A stage that fails leaves the rows it would have
// replaced standing and the run carries on, so nothing another stage
// completed is thrown away; everything that did complete is written, and
// only then does the run report every failure as its error.
func Run(ctx context.Context, opts Options) error {
	source, svc, calendarSheet, dryRun := opts.Source, opts.Sheets, opts.CalendarSheet, opts.DryRun
	if dryRun {
		log.Printf("dry run: nothing will be written to the sheet")
	}
	roster := app.CalendarRoster(opts.Directory)
	log.Printf("roster: %d classrooms", len(roster.Classrooms))
	tables, err := calendar.ReadTables(source)
	if err != nil {
		return fmt.Errorf("read calendar tables: %w", err)
	}
	dayTypes := []string{}
	for _, row := range tables.DayTypes {
		dayTypes = append(dayTypes, row["Day Type"])
	}
	tags := []calendar.Tag{}
	for _, row := range tables.Tags {
		tags = append(tags, calendar.Tag{Name: row["Tag"], Description: row["Description"]})
	}
	if len(tags) == 0 {
		return fmt.Errorf("%s needs rows before the import can run", calendar.TagsTab)
	}
	for _, name := range append([]string{calendar.RegularDayType}, legendDayTypes...) {
		if !slices.Contains(dayTypes, name) {
			return fmt.Errorf("%s needs a %q row before the import can run", calendar.DayTypesTab, name)
		}
	}
	client := anthropic.NewClient(option.WithAPIKey(opts.AnthropicKey))

	from, to := window(time.Now().In(calendar.Location))
	google, err := feedRows(ctx, opts.Calendar, from, to)
	if err != nil {
		return fmt.Errorf("read the school calendar: %w", err)
	}
	log.Printf("feed: %d events from %s on", len(google), from.Format(calendar.DateFormat))
	// Only the window is imported and mirrored; rows outside it, whatever
	// their year, are carried as they are and never removed.
	inWindow := func(row map[string]string) bool {
		start, err := time.ParseInLocation(calendar.DateFormat, row["Start"][:min(len(row["Start"]), len(calendar.DateFormat))], calendar.Location)
		return err == nil && !start.Before(from) && start.Before(to)
	}
	googleRowsNow := slices.Clone(google)
	for _, row := range tables.Google {
		if !inWindow(row) {
			googleRowsNow = append(googleRowsNow, row)
		}
	}

	page, err := fetch(pageURL)
	if err != nil {
		return fmt.Errorf("fetch the school calendar page: %w", err)
	}
	pdfURL, err := findPDF(page)
	if err != nil {
		return err
	}
	pdf, err := fetch(pdfURL)
	if err != nil {
		return fmt.Errorf("fetch the year calendar pdf: %w", err)
	}
	// The hash covers how the PDF is read as well as its bytes, so a change to
	// the reading re-reads an unchanged document.
	pdfHash := digest(string(pdf), legendSystem, entriesSystem(roster), monthSystem(""))[:12]
	// A stage that fails leaves the rows it would have replaced standing and
	// the run carries on, so nothing another stage completed is thrown away;
	// the run reports every failure at the end and exits non-zero.
	failures := []string{}
	pdfRowsNow := tables.PDF
	known := false
	for _, row := range tables.PDF {
		if row["PDF"] == pdfHash {
			known = true
		}
	}
	if known {
		log.Printf("pdf: %s unchanged (%s), %d rows kept", pdfURL, pdfHash, len(tables.PDF))
	} else {
		log.Printf("pdf: %s is new (%s), reading it", pdfURL, pdfHash)
		fresh, year, err := readPDF(ctx, client, pdf, pdfHash, roster, dayTypes)
		if err != nil {
			log.Printf("[ERROR] read the year calendar pdf: %v; keeping the %d rows already there", err, len(tables.PDF))
			failures = append(failures, "the year calendar pdf")
		} else {
			pdfRowsNow = []map[string]string{}
			for _, row := range tables.PDF {
				if row["Year"] != year {
					pdfRowsNow = append(pdfRowsNow, row)
				}
			}
			pdfRowsNow = append(pdfRowsNow, fresh...)
			log.Printf("pdf: %d entries for %s", len(fresh), year)
		}
	}

	// The classifier's whole prompt is in every event's input hash, so a change
	// to the rules, the glossary, or a tag's description re-classifies
	// everything, and nothing else does.
	vocabulary := digest(classifierSystem(roster, tags), strings.Join(roster.Names(), ","), strings.Join(dayTypes, ","))
	existing := map[string]map[string]string{}
	for _, row := range tables.Enrichment {
		existing[row["Event ID"]] = row
	}
	enrichment := []map[string]string{}
	pending := []enrichInput{}
	pendingHash := map[string]string{}
	isPDF := map[string]bool{}
	considered := map[string]bool{}
	consider := func(id string, row map[string]string, pdf bool) {
		considered[id] = true
		hash := digest(row["Title"], row["Description"], row["Start"], row["End"], row["Location"], vocabulary)
		if kept, ok := existing[id]; ok && kept["Input Hash"] == hash {
			enrichment = append(enrichment, kept)
			return
		}
		pending = append(pending, enrichInput{ID: id, Title: row["Title"], Start: row["Start"], End: row["End"], Location: row["Location"], Description: row["Description"]})
		pendingHash[id] = hash
		isPDF[id] = pdf
	}
	for _, row := range google {
		consider(row["Key"], row, false)
	}
	for _, row := range pdfRowsNow {
		consider(row["Key"], row, true)
	}
	for _, row := range tables.Enrichment {
		if !considered[row["Event ID"]] {
			enrichment = append(enrichment, row)
		}
	}
	log.Printf("enrichment: %d rows current, %d events to classify", len(enrichment), len(pending))
	today := time.Now().In(calendar.Location).Format(calendar.DateFormat)
	batches := [][]enrichInput{}
	for start := 0; start < len(pending); start += batchSize {
		batches = append(batches, pending[start:min(start+batchSize, len(pending))])
	}
	results := make([][]enrichOutput, len(batches))
	errs := make([]error, len(batches))
	var wg sync.WaitGroup
	for i, batch := range batches {
		wg.Go(func() {
			results[i], errs[i] = enrich(ctx, client, batch, roster, dayTypes, tags)
		})
	}
	wg.Wait()
	for i, batch := range batches {
		if errs[i] != nil {
			log.Printf("[ERROR] classify events, batch %d of %d: %v", i+1, len(batches), errs[i])
			failures = append(failures, fmt.Sprintf("classification batch %d of %d", i+1, len(batches)))
			for _, in := range batch {
				if kept, ok := existing[in.ID]; ok {
					enrichment = append(enrichment, kept)
				}
			}
			continue
		}
		for _, a := range results[i] {
			if isPDF[a.ID] {
				a.DayType = noDayType
			}
			row := map[string]string{
				"Event ID": a.ID, "Tags": calendar.JoinList(a.Tags),
				"Keywords": calendar.JoinList(a.Keywords), "Input Hash": pendingHash[a.ID], "Model": modelName, "Enriched": today,
			}
			if a.DayType != noDayType {
				row["Day Type"] = a.DayType
			}
			enrichment = append(enrichment, row)
		}
		log.Printf("enrichment: batch %d of %d classified %d events", i+1, len(batches), len(batch))
	}

	byTag := map[string]int{}
	for _, row := range enrichment {
		for _, t := range calendar.SplitList(row["Tags"]) {
			byTag[t]++
		}
	}
	log.Printf("rows: %d feed, %d pdf, %d enriched", len(googleRowsNow), len(pdfRowsNow), len(enrichment))
	for _, t := range tags {
		if byTag[t.Name] > 0 {
			log.Printf("  %s: %d", t.Name, byTag[t.Name])
		}
	}

	stamp := time.Now().In(calendar.Location).Format(calendar.DateTimeFormat)
	logRows := [][]string{}
	for _, s := range []struct {
		tab    string
		header []string
		rows   []map[string]string
		before []map[string]string
		keyCol string
	}{
		{calendar.PDFTab, calendar.PDFColumns, pdfRowsNow, tables.PDF, "Key"},
		{calendar.GoogleTab, calendar.GoogleColumns, googleRowsNow, tables.Google, "Key"},
		{calendar.EnrichmentTab, calendar.EnrichmentColumns, enrichment, tables.Enrichment, "Event ID"},
	} {
		result, err := syncTab(svc, calendarSheet, s.tab, s.header, s.rows, s.keyCol, !dryRun)
		if err != nil {
			return fmt.Errorf("sync %s: %w", s.tab, err)
		}
		titles := titlesOf(s.before, s.keyCol)
		for k, v := range titlesOf(s.rows, s.keyCol) {
			titles[k] = v
		}
		logRows = append(logRows, changes(s.tab, result, titles, stamp)...)
	}
	if dryRun {
		log.Printf("dry run: %d change log rows not written", len(logRows))
		if len(failures) > 0 {
			return fmt.Errorf("%d stages failed: %s", len(failures), strings.Join(failures, "; "))
		}
		return nil
	}
	if err := source.AppendAll("calendar", calendar.ChangeLogTab, logRows); err != nil {
		return fmt.Errorf("append the change log: %w", err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d stages failed and will be retried next run: %s", len(failures), strings.Join(failures, "; "))
	}
	log.Printf("change log: %d rows appended", len(logRows))
	return nil
}
