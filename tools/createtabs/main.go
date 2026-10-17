package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/env"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
)

type tab struct {
	title  string
	header []string
}

func withChangeLog(tabs []store.Tab) []tab {
	out := []tab{}
	for _, t := range tabs {
		out = append(out, tab{t.Name, t.Columns})
	}
	return append(out, tab{store.ChangeLogTab, store.ChangeLogColumns})
}

var layouts = map[string][]tab{
	"Artifacts": {
		{"Documents", model.DocumentColumns},
		{store.ChangeLogTab, store.ChangeLogColumns},
	},
	"Directory": withChangeLog(model.DirectoryTabs),
	"Staff Birthday List (Shared)": {
		{db.BirthdaySharedTab, db.BirthdaySharedColumns},
	},
	"Config": withChangeLog(model.ConfigTabs),
}

func applyLayout(source *data.Sheet, layout string) (tabs, columns int, err error) {
	title, sizes, err := source.Layout(layout)
	if err != nil {
		return 0, 0, err
	}
	if title != layout {
		return 0, 0, fmt.Errorf("spreadsheet is titled %q, want %q", title, layout)
	}
	existing := []string{}
	for _, size := range sizes {
		existing = append(existing, size.Title)
	}
	present := []string{}
	for _, t := range layouts[layout] {
		if slices.Contains(existing, t.title) {
			present = append(present, t.title)
		}
	}
	headers, err := source.Tabs(context.Background(), layout, nil, present)
	if err != nil {
		return 0, 0, err
	}
	for _, t := range layouts[layout] {
		if slices.Contains(present, t.title) {
			missing := []string{}
			for _, name := range t.header {
				if !slices.Contains(headers[t.title].Header, name) {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				if err := source.AddColumns(layout, t.title, missing); err != nil {
					return 0, 0, err
				}
				log.Printf("added %d columns to %q: %v", len(missing), t.title, missing)
				columns += len(missing)
			}
			if !dataLayouts[layout] {
				continue
			}
			moved, err := source.MoveColumns(layout, t.title, t.header)
			if err != nil {
				return 0, 0, err
			}
			if moved > 0 {
				log.Printf("moved %d columns of %q into the schema's order", moved, t.title)
			}
			continue
		}
		if err := source.AddTab(layout, t.title, t.header); err != nil {
			return 0, 0, err
		}
		log.Printf("created tab %q in %q with %d columns", t.title, layout, len(t.header))
		tabs++
	}
	return tabs, columns, nil
}

var dataLayouts = map[string]bool{}

func addDataLayouts() {
	for _, s := range spreadsheets.All {
		if !slices.Contains(db.Sheets, s.Source) {
			continue
		}
		tabs := []tab{}
		for _, t := range db.Tables {
			if t.Sheet == s.Source {
				tabs = append(tabs, tab{t.Name, t.Stored()})
			}
		}
		layouts[s.Title] = append(tabs, tab{db.ChangesTab, db.ChangesColumns})
		dataLayouts[s.Title] = true
	}
}

func main() {
	addDataLayouts()
	sheets := []spreadsheets.Spreadsheet{}
	for _, s := range spreadsheets.All {
		if _, ok := layouts[s.Title]; ok {
			sheets = append(sheets, s)
		}
	}
	ids := map[string]string{}
	for _, s := range sheets {
		ids[s.Title] = env.Required(s.Env)
	}
	source, err := data.NewSheet(ids)
	if err != nil {
		log.Fatalf("sheet source: %v", err)
	}
	tabs, columns := 0, 0
	for _, s := range sheets {
		created, added, err := applyLayout(source, s.Title)
		if err != nil {
			log.Fatalf("%s: %v", s.Env, err)
		}
		tabs += created
		columns += added
	}
	log.Printf("checked %d spreadsheets: %d tabs created, %d columns added", len(sheets), tabs, columns)
}
