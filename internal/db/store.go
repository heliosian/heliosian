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

const refreshSystem = "refresh"

func NewStore(source data.Source, writer data.Writer, queue *store.Queue, index *SearchIndex, origin func(app string) string) (*Store, error) {
	var s *Store
	refreshed := func(tab string, before, after store.Row) {
		for _, in := range committedInboxes(s, tab) {
			in.deliver(delivery{change: Change{Table: tab, Old: before, New: after}, env: Env{System: refreshSystem, Now: time.Now()}})
		}
	}
	s, err := store.New(parts(index, newServices(index), origin, refreshed), consentStep, checkModel, source, writer, queue)
	return s, err
}

func checkModel(ctx context.Context, base, m *Model) error {
	referenced := tallied(ctx, "check references")
	defer referenced()
	return m.checkReferences(base)
}

const ChangesTab = "CHANGES"

var ChangesColumns = []string{"id", "at", "actor", "real_actor", "action", "table", "row", "column", "previous"}

var changeLog = &store.ChangeLog{Tab: ChangesTab, Columns: ChangesColumns, Row: func(c store.Change) store.Row {
	return store.Row{
		"id": Mint(ChangePrefix, func(string) bool { return false }), "at": c.At.In(School).Format(cells.StampFormat),
		"actor": c.Actor, "real_actor": c.Real, "action": c.Action, "table": c.Tab, "row": c.Named["id"],
		"column": c.Column, "previous": c.Previous,
	}
}}

func parts(index *SearchIndex, sv *services, origin func(app string) string, refreshed func(tab string, before, after store.Row)) []store.Part[Model] {
	out := []store.Part[Model]{}
	for _, sheet := range Sheets {
		tabs := []store.Tab{}
		for _, t := range Tables {
			if t.In(sheet) {
				tabs = append(tabs, store.Tab{Name: t.Name, Columns: t.Stored(), Key: []string{"id"}, AppendOnly: t.AppendOnly})
			}
		}
		out = append(out, store.Part[Model]{App: sheet, Tabs: tabs, Changes: changeLog, Build: build(sheet, index, sv, origin), Loaded: loaded(sheet), Refreshed: refreshed})
	}
	return out
}

var sheetChecks = []struct {
	tables []string
	check  func(Sheet) error
}{
	{[]string{"PERSON_EMAIL"}, checkEmails},
	{[]string{"PHOTO"}, checkPhotos},
	{[]string{"RULE"}, checkRuleProperties},
	{[]string{"GROUP", "RULE"}, checkGroupCycles},
	{[]string{"DOCUMENT"}, checkDocuments},
}

func build(sheet string, index *SearchIndex, sv *services, origin func(app string) string) func(context.Context, store.Tables, *Model) error {
	return func(ctx context.Context, tables store.Tables, m *Model) error {
		m.index = index
		m.services = sv
		prior := *m.slot(sheet)
		built := Sheet{}
		rebuilt := map[string]bool{}
		for i := range Tables {
			t := &Tables[i]
			if !t.In(sheet) {
				continue
			}
			if prior[t.Name].builtFrom(tables[t.Name]) {
				built[t.Name] = prior[t.Name]
				continue
			}
			rows, err := buildRows(ctx, t, tables[t.Name], origin)
			if err != nil {
				return err
			}
			built[t.Name] = rows
			rebuilt[t.Name] = true
		}
		checked := tallied(ctx, "sheet checks")
		for _, c := range sheetChecks {
			if !slices.ContainsFunc(c.tables, func(t string) bool { return rebuilt[t] }) {
				continue
			}
			if err := c.check(built); err != nil {
				return err
			}
		}
		checked()
		*m.slot(sheet) = built
		return nil
	}
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
