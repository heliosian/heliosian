// Command celebrateimport loads the old Spring Celebration site's Glide tabs,
// dumped to imports/celebrate/ with cmd/dumptab, into the empty Celebrate
// spreadsheet - parties, who hosts them, and every ticket and waitlist row -
// after converting them to the new sheet's shape and proving the result loads.
//
// A dry run (the default) reports what it would write and leaves the converted
// tabs as CSVs under imports/celebrate/converted/ to look over; -write uploads
// each party's picture to the media bucket and fills the tabs, refusing unless
// every tab is still empty below its header.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/blob"
	"heliosian/internal/celebrate"
)

const (
	exports      = "imports/celebrate"
	convertedDir = "imports/celebrate/converted"
	imageFolder  = "party-images"
)

var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Fatalf("[ERROR] load time zone %s: %v", name, err)
	}
	return loc
}

// read loads one dumped tab as rows keyed by header, dropping blank cells and
// rows with nothing in them.
func read(name string) []map[string]string {
	f, err := os.Open(filepath.Join(exports, name+".csv"))
	if err != nil {
		log.Fatalf("[ERROR] open %s: %v (dump the tab with cmd/dumptab first)", name, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", name, err)
	}
	if len(records) == 0 {
		log.Fatalf("[ERROR] %s is empty", name)
	}
	rows := []map[string]string{}
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, cell := range rec {
			if i < len(records[0]) && strings.TrimSpace(cell) != "" {
				row[strings.TrimSpace(records[0][i])] = cell
			}
		}
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

var emailForm = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// email is a cell as an address, or "" for anything that is not one - the
// old sheet's lookups left #N/A and #REF! behind where a person was missing.
func email(cell string) string {
	e := strings.ToLower(strings.TrimSpace(cell))
	if !emailForm.MatchString(e) {
		return ""
	}
	return e
}

func trim(cell string) string {
	return strings.TrimSpace(cell)
}

func isTrue(cell string) bool {
	return strings.EqualFold(strings.TrimSpace(cell), "TRUE")
}

// when reads the old sheet's date shapes into the new sheet's: a Glide
// timestamp ("Saturday, September 19, 2026 at 5:00:00 PM", with a narrow
// space before PM), a bare day ("Sat Sep 6, 2025"), a form timestamp
// ("1/11/2026 14:56:26"), an ISO instant, or a month and day with the year
// supplied ("March 1"). The first form keeps its time; the rest are days.
func when(cell string, year int) string {
	cell = strings.NewReplacer("\u202f", " ", "\u00a0", " ").Replace(strings.TrimSpace(cell))
	if cell == "" {
		return ""
	}
	for _, layout := range []string{"Monday, January 2, 2006 at 3:04:05 PM", "Monday, January 2, 2006 at 3:04 PM"} {
		if t, err := time.Parse(layout, cell); err == nil {
			return t.Format(celebrate.DateTimeFormat)
		}
	}
	for _, layout := range []string{"Mon Jan 2, 2006", "Monday, January 2, 2006", "January 2, 2006", "1/2/2006 15:04:05", "1/2/2006 15:04", "1/2/2006", "2006-01-02"} {
		if t, err := time.Parse(layout, cell); err == nil {
			return t.Format(celebrate.DateFormat)
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, cell); err == nil {
		return t.In(local).Format(celebrate.DateTimeFormat)
	}
	if year > 0 {
		if t, err := time.Parse("January 2 2006", cell+" "+strconv.Itoa(year)); err == nil {
			return t.Format(celebrate.DateFormat)
		}
	}
	return ""
}

// yearOf is the year a celebration's code names: SC-2026 is 2026.
func yearOf(code string) int {
	_, digits, _ := strings.Cut(code, "-")
	year, _ := strconv.Atoi(digits)
	return year
}

func price(cell string) string {
	n, err := celebrate.ParsePrice(cell)
	if err != nil {
		log.Fatalf("[ERROR] price %q: %v", cell, err)
	}
	if n == 0 {
		return ""
	}
	return celebrate.PriceCell(n)
}

// kidsWords is what an audience says when children are among it, for the
// old rows that predate the per-kind ticket flags.
var kidsWords = regexp.MustCompile(`(?i)kid|child|famil|student|grade|\bG\d|\bK\b|ages?\b`)

type party struct {
	id, code, title string
}

type conversion struct {
	tables  *celebrate.Tables
	images  map[string]image // by the name the sheet records
	byTitle map[string]*party
	skipped map[string]int
}

type image struct {
	url, mime string
	content   []byte
}

// fetchImage downloads a party's picture from the old site's storage and
// names it for its bytes, the way the image picker does.
func fetchImage(url string) (string, image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", image{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", image{}, fmt.Errorf("%s", resp.Status)
	}
	content, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return "", image{}, err
	}
	mime := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mime]
	if ext == "" {
		return "", image{}, fmt.Errorf("%s is not a supported image", mime)
	}
	sum := sha256.Sum256(content)
	return imageFolder + "/" + fmt.Sprintf("%x", sum) + ext, image{url: url, mime: mime, content: content}, nil
}

func convert() *conversion {
	c := &conversion{
		tables:  &celebrate.Tables{Settings: []map[string]string{}, Admins: []map[string]string{}, Redirects: []map[string]string{}},
		images:  map[string]image{},
		byTitle: map[string]*party{},
		skipped: map[string]int{},
	}
	c.parties()
	c.tickets()
	c.waitlist()
	return c
}

// parties converts the Parties tab: one new row per named party, its hosts
// from the four contact columns, the celebrations it names, and the
// categories in the old sheet's own order.
func (c *conversion) parties() {
	old := read("Parties")
	codes := []string{}
	used := map[string]bool{}
	fetched := map[string]string{} // url -> name
	for _, row := range old {
		title := trim(row["Party Name"])
		if title == "" {
			c.skipped["party rows with no name"]++
			continue
		}
		code := trim(row["Event Code"])
		if code == "" {
			log.Fatalf("[ERROR] party %q has no event code", title)
		}
		if !slices.Contains(codes, code) {
			codes = append(codes, code)
		}
		if c.byTitle[title] != nil {
			log.Fatalf("[ERROR] two parties are named %q", title)
		}
		p := &party{id: celebrate.NewID(), code: code, title: title}
		c.byTitle[title] = p

		name := ""
		if url := trim(row["Public Image"]); url != "" {
			if name = fetched[url]; name == "" {
				var img image
				var err error
				name, img, err = fetchImage(url)
				if err != nil {
					log.Fatalf("[ERROR] party %q: fetch picture %s: %v", title, url, err)
				}
				fetched[url] = name
				c.images[name] = img
			}
		}

		status := celebrate.StatusHidden
		if isTrue(row["Show on Website"]) {
			status = celebrate.StatusOpen
		}
		tickets := "Closed"
		if isTrue(row["Allow Tickets"]) {
			tickets = "Open"
		}
		// The per-kind flags arrived with the 2026 season; a row with none
		// set is an older one, read the way the site did then: grown-ups,
		// and children when the audience names them.
		adults, kids := isTrue(row["Allow Adult Tickets"]), isTrue(row["Allow Child Tickets"])
		audience := trim(row["Target Audience"])
		if !adults && !kids {
			adults, kids = true, kidsWords.MatchString(audience)
			c.skipped["parties whose audience flags were read off the audience words"]++
		}
		summary := trim(row["Summary"])
		if strings.HasPrefix(summary, "=") {
			summary = ""
		}
		category := trim(row["Category"])
		if category != "" {
			used[category] = true
		}
		c.tables.Parties = append(c.tables.Parties, map[string]string{
			"Party ID": p.id, "Celebration": code, "Title": title, "Subtitle": trim(row["Subtitle"]), "Summary": summary,
			"Description": strings.TrimSpace(row["Party Description"]), "Need To Know": trim(row["Need to Know Info"]),
			"Hosts": trim(row["Host Names"]), "Category": category, "Audience": audience,
			"Ticket Unit": trim(row[`Included in "1 Ticket"`]), "Price": price(row["Ticket Price"]),
			"Capacity": trim(row["# Tickets Available"]), "Minimum": trim(row["Minimum Size"]),
			"Start": when(row["Date & Time Start"], 0), "End": when(row["Date & Time End"], 0),
			"Location": trim(row["Location"]), "Image": name,
			"Status": status, "Tickets": tickets, "Waitlist": celebrate.YesNo(!isTrue(row["Close Waitlist"])),
			"Parents": celebrate.YesNo(adults), "Students": celebrate.YesNo(kids), "Staff": celebrate.YesNo(adults),
			"Drop-Off": celebrate.YesNo(isTrue(row["Allow Drop-Off"])), "Parent Ticket Required": celebrate.YesNo(isTrue(row["Require Parent Ticket"])),
			"Added By": email(row["Contact Email Address 1"]), "Added": when(row["Timestamp"], 0),
		})
		hosts := []string{}
		for _, column := range []string{"Contact Email Address 1", "Contact Email Address 2", "Contact Email Address 3", "Contact Email Address 4"} {
			if e := email(row[column]); e != "" && !slices.Contains(hosts, e) {
				hosts = append(hosts, e)
				c.tables.Hosts = append(c.tables.Hosts, map[string]string{"Party ID": p.id, "Email": e})
			}
		}
	}

	// One celebration per code, the latest current; the rest of each row is
	// the admin's to fill in.
	sort.Strings(codes)
	for i, code := range codes {
		c.tables.Celebrations = append(c.tables.Celebrations, map[string]string{
			"Code": code, "Title": fmt.Sprintf("Helios Spring Celebration %d", yearOf(code)), "Current": celebrate.YesNo(i == len(codes)-1),
		})
	}
	// The categories parties use, in the order the old sheet's Event Type
	// column lists them.
	for _, row := range read("Categories") {
		if t := trim(row["Event Type"]); used[t] {
			c.tables.Categories = append(c.tables.Categories, map[string]string{"Title": t})
			delete(used, t)
		}
	}
	for _, t := range slices.Sorted(func(yield func(string) bool) {
		for t := range used {
			if !yield(t) {
				return
			}
		}
	}) {
		c.tables.Categories = append(c.tables.Categories, map[string]string{"Title": t})
	}
}

// invoice is one INVOICING row still to be matched to a ticket: the guest's
// name where the row has one, and where the money stands.
type invoice struct {
	guest, status string
}

// tickets converts the Attendees tab, one ticket per row still attending,
// with where the invoice stands from the INVOICING tab. An invoice row finds
// its ticket by party and purchaser, then by the guest's name where both
// sides have one - the old sheet wrote the purchaser's own name as the guest
// for their own ticket, which the attendee row leaves blank.
func (c *conversion) tickets() {
	invoices := map[string][]invoice{}
	byYear := map[string]int{}
	for _, row := range read("INVOICING") {
		title := trim(row["Party Title"])
		p := c.byTitle[title]
		if p == nil {
			c.skipped["invoice rows for auction items rather than parties"]++
			continue
		}
		byYear[p.code]++
		key := title + "\x00" + email(row["Purchaser Email"])
		invoices[key] = append(invoices[key], invoice{guest: trim(row["Guest Name"]), status: trim(row["Invoice"])})
	}
	takeInvoice := func(p *party, purchaser, guest string) string {
		key := p.title + "\x00" + purchaser
		pool := invoices[key]
		at := -1
		for i, inv := range pool {
			if guest != "" && strings.EqualFold(inv.guest, guest) {
				at = i
				break
			}
		}
		if at < 0 && len(pool) > 0 {
			// The purchaser's own ticket, or a guest the two tabs name
			// differently: the next of their rows for the party.
			at = 0
		}
		if at < 0 {
			if byYear[p.code] == 0 {
				c.skipped["tickets of a season the INVOICING tab does not cover ("+p.code+"), left uninvoiced"]++
			} else {
				c.skipped["tickets with no invoice row"]++
			}
			return ""
		}
		status := pool[at].status
		invoices[key] = append(pool[:at:at], pool[at+1:]...)
		switch status {
		case celebrate.InvoiceSent, celebrate.InvoicePaid:
			return status
		case "Removed":
			c.skipped["tickets whose invoice was marked Removed, imported uninvoiced"]++
		}
		return ""
	}

	for _, row := range read("Attendees") {
		title := trim(row["Party Name"])
		p := c.byTitle[title]
		if p == nil {
			c.skipped["attendee rows naming no party"]++
			continue
		}
		if !isTrue(row["Attending"]) {
			c.skipped["attendee rows no longer attending"]++
			continue
		}
		attendee, name := email(row["Attendee Email"]), trim(row["Child/Guest Name"])
		purchaser := email(row["Purchaser Email"])
		if purchaser == "" {
			purchaser = attendee
		}
		if purchaser == "" {
			c.skipped["attendee rows with no purchaser address"]++
			continue
		}
		// A named child under the purchaser's own address is a guest by
		// name; the address was the old site's way of reaching the parent.
		if name != "" && attendee == purchaser {
			attendee = ""
		}
		if attendee == "" && name == "" {
			c.skipped["attendee rows naming nobody"]++
			continue
		}
		c.tables.Tickets = append(c.tables.Tickets, map[string]string{
			"Ticket ID": celebrate.NewID(), "Party ID": p.id, "Email": attendee, "Name": name, "Purchaser": purchaser,
			"Status": celebrate.TicketSold, "Price": price(row["Cost"]),
			"Invoice":  takeInvoice(p, purchaser, name),
			"Added By": purchaser, "Added": when(row["Date"], yearOf(p.code)),
		})
	}
}

// waitlist converts the Parties Waitlist tab: a Waitlist ticket per row,
// unless the same purchaser went on to hold a ticket for the party.
func (c *conversion) waitlist() {
	holds := map[string]bool{}
	for _, t := range c.tables.Tickets {
		holds[t["Party ID"]+"\x00"+t["Purchaser"]] = true
	}
	for _, row := range read("Parties Waitlist") {
		title := trim(row["Party Name"])
		p := c.byTitle[title]
		if p == nil {
			c.skipped["waitlist rows naming no party"]++
			continue
		}
		purchaser := email(row["Primary Email"])
		if purchaser == "" {
			c.skipped["waitlist rows with no address"]++
			continue
		}
		if holds[p.id+"\x00"+purchaser] {
			c.skipped["waitlist rows whose purchaser got a ticket"]++
			continue
		}
		name := trim(row["Child/Guest Name"])
		attendee := purchaser
		if name != "" {
			attendee = ""
		}
		c.tables.Tickets = append(c.tables.Tickets, map[string]string{
			"Ticket ID": celebrate.NewID(), "Party ID": p.id, "Email": attendee, "Name": name, "Purchaser": purchaser,
			"Status": celebrate.TicketWaitlist, "Added By": purchaser, "Added": when(row["Date"], 0),
		})
	}
}

// converted answers the loader's image checks from the pictures just fetched,
// so the tables can be proven to load before anything is in the bucket.
type converted map[string]image

func (c converted) Has(key string) (bool, error) { _, ok := c[key]; return ok, nil }
func (c converted) Prefetch([]string) error      { return nil }

type tab struct {
	title   string
	columns []string
	rows    []map[string]string
}

func quoted(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}

// check refuses a tab whose header is not the layout createtabs wrote or that
// already holds rows, so the import can only ever fill an empty sheet.
func check(svc *sheets.Service, sheet string, t tab) {
	resp, err := svc.Spreadsheets.Values.Get(sheet, quoted(t.title)).Do()
	if err != nil {
		log.Fatalf("[ERROR] read tab %s: %v", t.title, err)
	}
	if len(resp.Values) == 0 {
		log.Fatalf("[ERROR] tab %s has no header row; run createtabs first", t.title)
	}
	if len(resp.Values) != 1 {
		log.Fatalf("[ERROR] tab %s already has %d rows; the import only fills an empty sheet", t.title, len(resp.Values)-1)
	}
	header := make([]string, len(resp.Values[0]))
	for i, c := range resp.Values[0] {
		header[i] = strings.TrimSpace(fmt.Sprint(c))
	}
	if strings.Join(header, "\x00") != strings.Join(t.columns, "\x00") {
		log.Fatalf("[ERROR] tab %s header is %q, want %q", t.title, header, t.columns)
	}
}

// save writes a converted tab as a CSV beside the dumps, so the result can be
// looked over before it goes anywhere.
func save(t tab) {
	if err := os.MkdirAll(convertedDir, 0o755); err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	f, err := os.Create(filepath.Join(convertedDir, t.title+".csv"))
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(t.columns); err != nil {
		log.Fatalf("[ERROR] write %s: %v", t.title, err)
	}
	for _, row := range t.rows {
		rec := make([]string, len(t.columns))
		for j, c := range t.columns {
			rec[j] = row[c]
		}
		if err := w.Write(rec); err != nil {
			log.Fatalf("[ERROR] write %s: %v", t.title, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Fatalf("[ERROR] write %s: %v", t.title, err)
	}
}

func write(svc *sheets.Service, sheet string, t tab) {
	if len(t.rows) == 0 {
		log.Printf("%s: nothing to write", t.title)
		return
	}
	values := make([][]interface{}, len(t.rows))
	for i, row := range t.rows {
		rec := make([]interface{}, len(t.columns))
		for j, c := range t.columns {
			rec[j] = row[c]
		}
		values[i] = rec
	}
	_, err := svc.Spreadsheets.Values.Update(sheet, quoted(t.title)+"!A2", &sheets.ValueRange{Values: values}).ValueInputOption("RAW").Do()
	if err != nil {
		log.Fatalf("[ERROR] write %s: %v", t.title, err)
	}
	log.Printf("%s: wrote %d rows", t.title, len(values))
}

func main() {
	apply := flag.Bool("write", false, "upload the pictures and fill the sheet; without it, report what would be written")
	flag.Parse()
	sheet := os.Getenv("CELEBRATE_SHEET")
	if sheet == "" {
		log.Fatal("[ERROR] CELEBRATE_SHEET is required")
	}

	c := convert()
	model, err := celebrate.BuildModel(c.tables, converted(c.images))
	if err != nil {
		log.Fatalf("[ERROR] the converted tables would not load: %v", err)
	}
	sold, waiting := 0, 0
	for _, p := range model.Parties {
		for _, t := range p.Tickets {
			if t.Status == celebrate.TicketWaitlist {
				waiting++
			} else {
				sold++
			}
		}
	}
	log.Printf("converted: %d celebrations, %d categories, %d parties, %d hosts, %d tickets (%d sold, %d waiting), %d pictures",
		len(c.tables.Celebrations), len(c.tables.Categories), len(c.tables.Parties), len(c.tables.Hosts), len(c.tables.Tickets), sold, waiting, len(c.images))
	for _, reason := range slices.Sorted(func(yield func(string) bool) {
		for reason := range c.skipped {
			if !yield(reason) {
				return
			}
		}
	}) {
		log.Printf("  %d %s", c.skipped[reason], reason)
	}
	for reason, n := range model.Skipped {
		log.Printf("  the loader skipped %d: %s", n, reason)
	}

	tabs := []tab{
		{"Celebrations", celebrate.CelebrationColumns, c.tables.Celebrations},
		{"Categories", celebrate.CategoryColumns, c.tables.Categories},
		{"Parties", celebrate.PartyColumns, c.tables.Parties},
		{"Hosts", celebrate.HostColumns, c.tables.Hosts},
		{"Tickets", celebrate.TicketColumns, c.tables.Tickets},
	}
	for _, t := range tabs {
		save(t)
	}
	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	for _, t := range tabs {
		check(svc, sheet, t)
	}
	if !*apply {
		log.Printf("dry run: the converted tabs are under %s/ to look over, and the sheet's tabs are empty and ready; run again with -write", convertedDir)
		return
	}

	uploader, err := blob.NewUploader()
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	uploaded := 0
	for name, img := range c.images {
		written, err := uploader.Put(imageFolder, strings.TrimPrefix(name, imageFolder+"/"), img.mime, img.content)
		if err != nil {
			log.Fatalf("[ERROR] upload %s: %v", name, err)
		}
		if written {
			uploaded++
		}
	}
	log.Printf("pictures: %d uploaded, %d already in the bucket", uploaded, len(c.images)-uploaded)
	for _, t := range tabs {
		write(svc, sheet, t)
	}
	log.Printf("imported into %s; add at least one row to Admins by hand, and fill in the Celebrations rows", sheet)
}
