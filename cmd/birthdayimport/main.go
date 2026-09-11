// Command birthdayimport loads the Glide birthday app's table exports from imports/ into the empty Birthdays spreadsheet.
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/birthday"
)

const (
	exports   = "imports"
	yearStart = "08-14"
)

var local = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Fatalf("[ERROR] load time zone %s: %v", name, err)
	}
	return loc
}

func read(name string) []map[string]string {
	f, err := os.Open(filepath.Join(exports, name+".csv"))
	if err != nil {
		log.Fatalf("[ERROR] open %s: %v", name, err)
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
		blank := true
		for i, cell := range rec {
			if i < len(records[0]) && cell != "" {
				row[records[0][i]] = cell
				blank = false
			}
		}
		if !blank {
			rows = append(rows, row)
		}
	}
	return rows
}

func email(cell string) string {
	return strings.ToLower(strings.TrimSpace(cell))
}

// day reads any of the export's date shapes as a day at the school: an ISO
// instant, a long date, or a bare date.
func day(cell string) (time.Time, bool) {
	cell = strings.TrimSpace(cell)
	if cell == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, cell); err == nil {
		t = t.In(local)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), true
	}
	for _, layout := range []string{"Monday, January 2, 2006", "January 2, 2006", "2006-01-02", "1/2/2006"} {
		if t, err := time.Parse(layout, cell); err == nil {
			return t, true
		}
	}
	log.Fatalf("[ERROR] unreadable date %q", cell)
	return time.Time{}, false
}

func cell(t time.Time) string {
	return t.Format(birthday.DateFormat)
}

func yearOf(t time.Time) string {
	month, dayOfMonth, _ := birthday.ParseMonthDay(yearStart)
	return birthday.YearContaining(t, month, dayOfMonth).Label
}

type dated struct {
	row  map[string]string
	when time.Time
}

// latest keeps one row per key, the most recent, since the Glide app appended
// a row on every click rather than editing one.
func latest(rows []map[string]string, keyOf func(map[string]string, time.Time) string, dateColumn string) map[string]dated {
	out := map[string]dated{}
	for _, row := range rows {
		when, ok := day(row[dateColumn])
		if !ok {
			continue
		}
		key := keyOf(row, when)
		if key == "" {
			continue
		}
		if have, dup := out[key]; !dup || when.After(have.when) {
			out[key] = dated{row: row, when: when}
		}
	}
	return out
}

func sortedKeys(m map[string]dated) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStrings(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func convert() *birthday.Tables {
	charities := []map[string]string{}
	nameByID := map[string]string{}
	seenNames := map[string]int{}
	defaultCharity := ""
	for _, row := range read("Charities") {
		name := strings.TrimSpace(row["Name"])
		seenNames[name]++
		if seenNames[name] > 1 {
			renamed := fmt.Sprintf("%s (%d)", name, seenNames[name])
			log.Printf("charity %q is listed more than once; renaming one to %q", name, renamed)
			name = renamed
		}
		link := strings.TrimSpace(row["Donation Link"])
		if !strings.Contains(link, "://") {
			link = "https://" + link
		}
		added := ""
		if t, ok := day(row["Added On"]); ok {
			added = cell(t)
		}
		allowed := strings.EqualFold(row["Allowed"], "TRUE")
		if strings.EqualFold(row["Is Default"], "TRUE") {
			defaultCharity = name
		}
		nameByID[row["Row ID"]] = name
		charities = append(charities, map[string]string{
			"Name": name, "Donation Link": link, "About": strings.TrimSpace(row["About"]), "EIN": strings.TrimSpace(row["EIN"]),
			"Allowed": birthday.YesNo(allowed), "Why Not Allowed": strings.TrimSpace(row["Why Not Allowed"]), "Added On": added,
		})
	}
	if defaultCharity == "" {
		log.Fatal("[ERROR] no charity is marked Is Default")
	}

	dates := map[string]bool{}
	for _, row := range read("Newsletter Dates") {
		if t, ok := day(row["Date"]); ok {
			dates[cell(t)] = true
		}
	}
	newsletterDates := []map[string]string{}
	for _, d := range sortedStrings(dates) {
		newsletterDates = append(newsletterDates, map[string]string{"Date": d})
	}

	overrides := map[string]string{}
	for _, row := range read("Newsletter Override") {
		if t, ok := day(row["Newsletter Date"]); ok {
			overrides[email(row["Email"])] = cell(t)
		}
	}
	birthdays := []map[string]string{}
	birthdayOf := map[string]map[string]string{}
	for _, row := range read("Birthday") {
		e := email(row["Email"])
		t, ok := day(row["Birthday"])
		if !ok || birthdayOf[e] != nil {
			continue
		}
		birthdayOf[e] = map[string]string{"Email": e, "Birthday": cell(t), "Newsletter Override": overrides[e]}
		birthdays = append(birthdays, birthdayOf[e])
	}
	for _, row := range read("Birthday Not Recognized") {
		e := email(row["Email"])
		if birthdayOf[e] == nil {
			birthdayOf[e] = map[string]string{"Email": e}
			birthdays = append(birthdays, birthdayOf[e])
		}
		birthdayOf[e]["Participation"] = birthday.LevelSkip
		birthdayOf[e]["Note"] = strings.TrimSpace(row["Note"])
	}

	assignments := []map[string]string{}
	assigned := latest(read("Assignments"), func(row map[string]string, when time.Time) string {
		if email(row["Parent Email"]) == "" {
			return ""
		}
		return email(row["Staff Email"]) + "\x00" + yearOf(when)
	}, "Date")
	for _, key := range sortedKeys(assigned) {
		d := assigned[key]
		assignments = append(assignments, map[string]string{
			"Email": email(d.row["Staff Email"]), "Year": yearOf(d.when), "Assigned To": email(d.row["Parent Email"]), "Assigned On": cell(d.when),
		})
	}

	outreach := []map[string]string{}
	contacted := latest(read("Outreach"), func(row map[string]string, when time.Time) string {
		return email(row["Email"]) + "\x00" + yearOf(when)
	}, "Date Outreach")
	for _, key := range sortedKeys(contacted) {
		d := contacted[key]
		outreach = append(outreach, map[string]string{
			"Email": email(d.row["Email"]), "Year": yearOf(d.when), "Contacted On": cell(d.when), "Contacted By": email(d.row["Outreach By"]),
		})
	}

	donations := []map[string]string{}
	byKey := map[string]map[string]string{}
	given := latest(read("Donations"), func(row map[string]string, when time.Time) string {
		return email(row["Email"]) + "\x00" + yearOf(when)
	}, "Donation Date")
	for _, key := range sortedKeys(given) {
		d := given[key]
		name, ok := nameByID[d.row["Charity ID"]]
		if !ok {
			log.Fatalf("[ERROR] donation of %s names unknown charity id %q", d.row["Email"], d.row["Charity ID"])
		}
		row := map[string]string{
			"Email": email(d.row["Email"]), "Year": yearOf(d.when), "Charity": name, "Note": strings.TrimSpace(d.row["Donation Note"]),
			"Recorded On": cell(d.when),
		}
		byKey[key] = row
		donations = append(donations, row)
	}
	used := latest(read("Used Donations"), func(row map[string]string, when time.Time) string {
		return email(row["Email"]) + "\x00" + yearOf(when)
	}, "Date Used")
	for _, key := range sortedKeys(used) {
		d := used[key]
		row, ok := byKey[key]
		if !ok {
			log.Printf("used donation of %s in %s has no donation to mark; skipping", email(d.row["Email"]), yearOf(d.when))
			continue
		}
		row["Used On"] = cell(d.when)
		row["Used By"] = email(d.row["Used By"])
	}

	settings := []map[string]string{
		{"Key": birthday.DefaultCharityKey, "Value": defaultCharity},
		{"Key": birthday.YearStartKey, "Value": yearStart},
		{"Key": birthday.EmailSubjectKey, "Value": "Your birthday donation from the Helios community"},
		{"Key": birthday.EmailBodyKey, "Value": "Hi {first name},\n\nHappy early birthday from the Helios Community Association! Each year the HCA celebrates every staff member's birthday with a donation to a charity of their choice, shared in the newsletter.\n\nWhich charity would you like this year's donation to go to, and is there a sentence or two you'd like us to share about why? If we don't hear back before the {newsletter date} newsletter, we'll give to {default charity}.\n\nThank you for everything you do for our kids!"},
		{"Key": birthday.NoNewsletterNoteKey, "Value": "We have a note that you'd rather not be mentioned in the newsletter, so we'll make the donation quietly and leave you out of it. No need to remind us."},
	}

	return &birthday.Tables{
		Birthdays: birthdays, Assignments: assignments, Outreach: outreach, Donations: donations,
		Notes: []map[string]string{}, Charities: charities, NewsletterDates: newsletterDates, Settings: settings, Admins: []map[string]string{},
	}
}

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
	sheet := os.Getenv("BIRTHDAY_SHEET")
	if sheet == "" {
		log.Fatal("[ERROR] BIRTHDAY_SHEET is required")
	}
	tables := convert()
	if _, err := birthday.BuildModel(tables); err != nil {
		log.Fatalf("[ERROR] the converted tables would not load: %v", err)
	}
	tabs := []tab{
		{"Birthdays", birthday.BirthdayColumns, tables.Birthdays},
		{"Assignments", birthday.AssignmentColumns, tables.Assignments},
		{"Outreach", birthday.OutreachColumns, tables.Outreach},
		{"Donations", birthday.DonationColumns, tables.Donations},
		{"Charities", birthday.CharityColumns, tables.Charities},
		{"Newsletter Dates", birthday.NewsletterDateColumns, tables.NewsletterDates},
		{"Settings", birthday.SettingColumns, tables.Settings},
	}
	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	for _, t := range tabs {
		check(svc, sheet, t)
	}
	for _, t := range tabs {
		write(svc, sheet, t)
	}
	log.Printf("imported into %s; add at least one row to Admins by hand", sheet)
}
