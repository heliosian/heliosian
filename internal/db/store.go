package db

import (
	"context"
	"log/slog"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Store = store.Store[Model]

const refreshSystem = "refresh"

func NewStore(source data.Source, writer data.Writer, queue *store.Queue, index *SearchIndex) (*Store, error) {
	var s *Store
	refreshed := func(tab string, before, after store.Row) {
		for _, in := range committedInboxes(s, tab) {
			in.deliver(delivery{change: Change{Table: tab, Old: before, New: after}, env: Env{System: refreshSystem, Now: time.Now()}})
		}
	}
	s, err := store.New(parts(index, newServices(index), refreshed), finishModel, checkModel, source, writer, queue)
	return s, err
}

func finishModel(ctx context.Context, _, m *Model) error {
	return consentStep(ctx, m)
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

func parts(index *SearchIndex, sv *services, refreshed func(tab string, before, after store.Row)) []store.Part[Model] {
	out := []store.Part[Model]{}
	for _, sheet := range Sheets {
		tabs := []store.Tab{}
		for _, t := range Tables {
			if t.In(sheet) {
				tabs = append(tabs, store.Tab{Name: t.Name, Columns: t.Stored(), Key: []string{"id"}, AppendOnly: t.AppendOnly})
			}
		}
		out = append(out, store.Part[Model]{App: sheet, Tabs: tabs, Changes: changeLog, Build: build(sheet, index, sv), Loaded: loaded(sheet), Refreshed: refreshed, Incremental: true})
	}
	return out
}

func build(sheet string, index *SearchIndex, sv *services) func(context.Context, store.Tables, *Model) error {
	return func(ctx context.Context, tables store.Tables, m *Model) error {
		m.index = index
		m.services = sv
		built := Sheet{}
		for i := range Tables {
			t := &Tables[i]
			if !t.In(sheet) {
				continue
			}
			rows, err := buildRows(ctx, t, tables[t.Name])
			if err != nil {
				return err
			}
			built[t.Name] = rows
		}
		checked := tallied(ctx, "sheet checks")
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
			if err := checkGroupCycles(built); err != nil {
				return err
			}
		}
		if sheet == DocumentsSheet {
			if err := checkDocuments(built); err != nil {
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
