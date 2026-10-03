package db

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Store = store.Store[Model]

func NewStore(source data.Source, writer data.Writer, queue *store.Queue) (*Store, error) {
	return store.New(parts(), consentStep, source, writer, queue)
}

var changeLog = &store.ChangeLog{Tab: ChangesTable, Row: func(c store.Change) store.Row {
	return store.Row{
		"id": Mint(ChangePrefix, func(string) bool { return false }), "at": c.At.In(School).Format(cells.StampFormat),
		"actor": c.Actor, "real_actor": c.Real, "action": c.Action, "table": c.Tab, "row": c.Named["id"],
		"column": c.Column, "previous": c.Previous,
	}
}}

func parts() []store.Part[Model] {
	out := []store.Part[Model]{}
	for _, sheet := range Sheets {
		tabs := []store.Tab{}
		for _, t := range Tables {
			if t.In(sheet) {
				tabs = append(tabs, store.Tab{Name: t.Name, Columns: t.Stored(), Key: []string{"id"}, AppendOnly: t.AppendOnly})
			}
		}
		part := store.Part[Model]{App: sheet, Tabs: tabs, Changes: changeLog, Build: build(sheet), Loaded: loaded(sheet)}
		// Mail reads every other sheet so that a change to any of them rebuilds it,
		// and its build checks the references across all five.
		if sheet == MailSheet {
			part.Reads = slices.DeleteFunc(slices.Clone(Sheets), func(s string) bool { return s == MailSheet })
		}
		out = append(out, part)
	}
	return out
}

func build(sheet string) func(context.Context, store.Tables, *Model) error {
	return func(_ context.Context, tables store.Tables, m *Model) error {
		built := Sheet{}
		for i := range Tables {
			t := &Tables[i]
			if !t.In(sheet) {
				continue
			}
			rows, err := buildRows(t, tables[t.Name])
			if err != nil {
				return err
			}
			built[t.Name] = rows
		}
		if sheet == PeopleSheet {
			if err := checkEmails(built); err != nil {
				return err
			}
			if err := checkPhotos(built); err != nil {
				return err
			}
		}
		if sheet == GroupsSheet {
			if err := checkRuleProperties(built); err != nil {
				return err
			}
		}
		*m.slot(sheet) = built
		m.derived = &derived{byGroup: map[string][]store.Row{}, sets: map[string]*generatedSet{}}
		if sheet == MailSheet {
			if err := m.mergeChanges(); err != nil {
				return err
			}
			return m.checkReferences()
		}
		return nil
	}
}

func (m *Model) mergeChanges() error {
	t, _ := Lookup(ChangesTable)
	rows := []store.Row{}
	for _, sheet := range Sheets {
		rows = append(rows, (*m.slot(sheet))[ChangesTable].rows...)
	}
	merged, err := indexRows(t, rows)
	if err != nil {
		return err
	}
	m.changes = merged
	return nil
}

func loaded(sheet string) func(*Model, time.Duration) {
	return func(m *Model, took time.Duration) {
		attrs := []any{"sheet", sheet}
		for _, t := range Tables {
			if t.In(sheet) {
				attrs = append(attrs, t.Name, (*m.slot(sheet))[t.Name].Len())
			}
		}
		slog.Info("loaded data sheet", append(attrs, "took", took.Round(time.Millisecond))...)
	}
}
