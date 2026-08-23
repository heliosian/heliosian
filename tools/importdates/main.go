// Command importdates seeds the Overrides updated-date columns from the legacy Glide spreadsheet.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"heliosian/internal/data"
	"heliosian/internal/directory"
)

const (
	overridesTab = "Overrides"
	studentTab   = "Student Images and Facts"
	familyTab    = "Family Images"

	photoColumn       = "Photo Updated"
	factsColumn       = "Facts Updated"
	familyPhotoColumn = "Family Photo Updated"

	glideFormat = "January 2, 2006"
	isoFormat   = "2006-01-02"
)

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}

func quoteTab(title string) string {
	return "'" + strings.ReplaceAll(title, "'", "''") + "'"
}

func indexOf(header []string, column string) int {
	for i, h := range header {
		if h == column {
			return i
		}
	}
	return -1
}

func cellAt(row []interface{}, i int) string {
	if i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(row[i]))
}

func glideDate(email, column, cell string) string {
	when, err := time.Parse(glideFormat, cell)
	if err != nil {
		log.Fatalf("[ERROR] %s has unparseable %s %q", email, column, cell)
	}
	return when.Format(isoFormat)
}

func main() {
	sourceID := flag.String("source", "", "legacy glide spreadsheet id")
	sheetID := flag.String("sheet", "", "directory spreadsheet id")
	preferencesID := flag.String("preferences", "", "preferences spreadsheet id")
	flag.Parse()
	if *sourceID == "" || *sheetID == "" || *preferencesID == "" {
		log.Fatal("[ERROR] -source, -sheet, and -preferences are required")
	}

	svc, err := sheets.NewService(context.Background(), option.WithScopes(sheets.SpreadsheetsScope))
	if err != nil {
		log.Fatalf("[ERROR] create sheets client: %v", err)
	}
	quoted := quoteTab(overridesTab)
	resp, err := svc.Spreadsheets.Values.Get(*sheetID, quoted).Do()
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", overridesTab, err)
	}
	if len(resp.Values) == 0 {
		log.Fatalf("[ERROR] %s has no header row", overridesTab)
	}
	header := []string{}
	for i := range resp.Values[0] {
		header = append(header, cellAt(resp.Values[0], i))
	}
	for _, column := range []string{photoColumn, factsColumn, familyPhotoColumn} {
		if indexOf(header, column) >= 0 {
			continue
		}
		log.Printf("adding column %q to %s", column, overridesTab)
		header = append(header, column)
	}
	headerRow := make([]interface{}, len(header))
	for i, h := range header {
		headerRow[i] = h
	}
	if _, err := svc.Spreadsheets.Values.Update(*sheetID, quoted+"!A1", &sheets.ValueRange{
		Values: [][]interface{}{headerRow},
	}).ValueInputOption("RAW").Do(); err != nil {
		log.Fatalf("[ERROR] write %s header: %v", overridesTab, err)
	}

	source, err := data.NewSheet(map[string]string{
		"directory": *sheetID, "preferences": *preferencesID, "glide": *sourceID,
	})
	if err != nil {
		log.Fatalf("[ERROR] sheet source: %v", err)
	}
	model, err := directory.LoadModel(source, nil, staticFiles{})
	if err != nil {
		log.Fatalf("[ERROR] load model: %v", err)
	}
	_, studentRows, err := source.Table("glide", studentTab)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", studentTab, err)
	}
	_, familyRows, err := source.Table("glide", familyTab)
	if err != nil {
		log.Fatalf("[ERROR] read %s: %v", familyTab, err)
	}

	updates := map[string]map[string]string{}
	set := func(email, column, value string) {
		if updates[email] == nil {
			updates[email] = map[string]string{}
		}
		if current, ok := updates[email][column]; ok && current >= value {
			return
		}
		updates[email][column] = value
	}

	photos, facts, noEmail, gone := 0, 0, 0, 0
	for _, row := range studentRows {
		photo, fact := row["Last Updated Photo"], row["Last Updated Facts"]
		if photo == "" && fact == "" {
			continue
		}
		email := strings.ToLower(row["Student Email Lower"])
		if email == "" {
			noEmail++
			continue
		}
		if model.Person(email) == nil {
			gone++
			continue
		}
		if photo != "" {
			set(email, photoColumn, glideDate(email, "Last Updated Photo", photo))
			photos++
		}
		if fact != "" {
			set(email, factsColumn, glideDate(email, "Last Updated Facts", fact))
			facts++
		}
	}

	type owner struct{ email, date string }
	byFamily := map[string]owner{}
	familyGone := 0
	for _, row := range familyRows {
		cell := row["Last Updated"]
		if cell == "" {
			continue
		}
		email := strings.ToLower(row["Family Key Email"])
		p := model.Person(email)
		if p == nil || !p.IsParent || p.FamilyKey == "" {
			familyGone++
			continue
		}
		date := glideDate(email, "Last Updated", cell)
		current, ok := byFamily[p.FamilyKey]
		if !ok || date > current.date || (date == current.date && email < current.email) {
			byFamily[p.FamilyKey] = owner{email: email, date: date}
		}
	}
	for _, o := range byFamily {
		set(o.email, familyPhotoColumn, o.date)
	}

	emailIdx := indexOf(header, "Email")
	if emailIdx < 0 {
		log.Fatalf("[ERROR] %s has no Email column", overridesTab)
	}
	rows := resp.Values[1:]
	rowByEmail := map[string]int{}
	for i, row := range rows {
		if email := strings.ToLower(cellAt(row, emailIdx)); email != "" {
			rowByEmail[email] = i
		}
	}
	appended := 0
	for email, cells := range updates {
		i, ok := rowByEmail[email]
		if !ok {
			row := make([]interface{}, len(header))
			for j := range row {
				row[j] = ""
			}
			row[emailIdx] = email
			rows = append(rows, row)
			i = len(rows) - 1
			rowByEmail[email] = i
			appended++
		}
		for column, value := range cells {
			idx := indexOf(header, column)
			for len(rows[i]) <= idx {
				rows[i] = append(rows[i], "")
			}
			rows[i][idx] = value
		}
	}

	if _, err := svc.Spreadsheets.Values.Update(*sheetID, quoted+"!A2", &sheets.ValueRange{
		Values: rows,
	}).ValueInputOption("RAW").Do(); err != nil {
		log.Fatalf("[ERROR] write %s rows: %v", overridesTab, err)
	}
	log.Printf("imported %d photo dates, %d facts dates, %d family photo dates across %d rows (%d appended)",
		photos, facts, len(byFamily), len(updates), appended)
	log.Printf("skipped %d student rows with no email, %d for people no longer in the directory, %d family rows with no current parent",
		noEmail, gone, familyGone)
}
