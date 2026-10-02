package db

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Store = store.Store[Model]

func NewStore(source data.Source, writer data.Writer, queue *store.Queue) (*Store, error) {
	return store.New(parts(), consentStep, source, writer, queue)
}

func parts() []store.Part[Model] {
	out := []store.Part[Model]{}
	for _, sheet := range Sheets {
		tabs := []store.Tab{}
		for _, t := range Tables {
			if t.Sheet == sheet {
				tabs = append(tabs, store.Tab{Name: t.Name, Columns: t.Stored(), Key: []string{"id"}, AppendOnly: t.AppendOnly})
			}
		}
		part := store.Part[Model]{App: sheet, Tabs: tabs, Build: build(sheet), Loaded: loaded(sheet)}
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
			if t.Sheet != sheet {
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
			return m.checkReferences()
		}
		return nil
	}
}

func loaded(sheet string) func(*Model, time.Duration) {
	return func(m *Model, took time.Duration) {
		attrs := []any{"sheet", sheet}
		for _, t := range Tables {
			if t.Sheet == sheet {
				attrs = append(attrs, t.Name, m.Table(t.Name).Len())
			}
		}
		slog.Info("loaded data sheet", append(attrs, "took", took.Round(time.Millisecond))...)
	}
}
