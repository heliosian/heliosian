package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/logging"
)

const (
	ChangeLogTab    = "Change Log"
	refreshInterval = 5 * time.Minute
)

var ChangeLogColumns = []string{"Timestamp", "Actor", "Real Actor", "Action", "Tab", "Key", "Column", "Previous"}

type Row = map[string]string

type Tables map[string][]Row

type kind int

const (
	insert kind = iota
	set
	update
	remove
	keyed
)

type Op struct {
	kind  kind
	tab   string
	match Row
	cells Row
}

func Insert(tab string, cells Row) Op {
	return Op{kind: insert, tab: tab, cells: cells}
}

func Set(tab string, match, cells Row) Op {
	return Op{kind: set, tab: tab, match: match, cells: cells}
}

func Update(tab string, match, cells Row) Op {
	return Op{kind: update, tab: tab, match: match, cells: cells}
}

func Delete(tab string, match Row) Op {
	return Op{kind: remove, tab: tab, match: match}
}

type Tab struct {
	App        string
	Name       string
	Columns    []string
	Key        []string
	Cascade    func(before, after Row) []Op
	AppendOnly bool
}

type Spec[M any] struct {
	App    string
	Tabs   []Tab
	Build  func(context.Context, Tables) (M, error)
	Loaded func(model M, took time.Duration)
}

type Store[M any] struct {
	spec   Spec[M]
	tabs   map[string]Tab
	source data.Source
	writer data.Writer
	queue  *Queue
	mu     sync.RWMutex
	tables Tables
	model  M
}

func New[M any](spec Spec[M], source data.Source, writer data.Writer, queue *Queue) (*Store[M], error) {
	s := &Store[M]{spec: spec, tabs: map[string]Tab{}, source: source, writer: writer, queue: queue}
	for _, t := range spec.Tabs {
		if len(t.Key) == 0 {
			return nil, fmt.Errorf("%s: tab %s names no key", spec.App, t.Name)
		}
		s.tabs[t.Name] = t
	}
	swap, err := s.load(context.Background())
	if err != nil {
		return nil, err
	}
	swap()
	queue.Register(s.load)
	return s, nil
}

func (s *Store[M]) load(ctx context.Context) (func(), error) {
	start := time.Now()
	tables, model, err := s.read(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", s.spec.App, err)
	}
	took := time.Since(start)
	return func() {
		s.mu.Lock()
		s.tables, s.model = tables, model
		s.mu.Unlock()
		s.spec.Loaded(model, took)
	}, nil
}

func (s *Store[M]) read(ctx context.Context) (Tables, M, error) {
	var none M
	names := map[string][]string{s.spec.App: {}}
	for _, t := range s.spec.Tabs {
		names[s.appOf(t)] = append(names[s.appOf(t)], t.Name)
	}
	read := map[string]map[string]data.Tab{}
	for app, tabs := range names {
		headers := []string{}
		if app == s.spec.App && s.logged() {
			headers = []string{ChangeLogTab}
		}
		got, err := s.source.Tabs(ctx, app, tabs, headers)
		if err != nil {
			return nil, none, err
		}
		read[app] = got
	}
	tables := Tables{}
	for _, t := range s.spec.Tabs {
		tab := read[s.appOf(t)][t.Name]
		if err := data.CheckColumns(t.Name, tab.Header, t.Columns); err != nil {
			return nil, none, err
		}
		tables[t.Name] = tab.Rows
	}
	if s.logged() {
		if err := data.CheckColumns(ChangeLogTab, read[s.spec.App][ChangeLogTab].Header, ChangeLogColumns); err != nil {
			return nil, none, err
		}
	}
	model, err := s.spec.Build(ctx, tables)
	if err != nil {
		return nil, none, err
	}
	return tables, model, nil
}

func (s *Store[M]) logged() bool {
	for _, t := range s.spec.Tabs {
		if s.appOf(t) == s.spec.App && !t.AppendOnly {
			return true
		}
	}
	return false
}

func (s *Store[M]) appOf(t Tab) string {
	if t.App == "" {
		return s.spec.App
	}
	return t.App
}

func (s *Store[M]) Model() M {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

func (s *Store[M]) Count(tab string, match Row) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, row := range s.tables[tab] {
		if matches(row, match) {
			n++
		}
	}
	return n
}

type change struct {
	before, after Row
}

func (s *Store[M]) Commit(ctx context.Context, actor string, ops ...Op) error {
	_, err := s.commit(ctx, actor, ops)
	return err
}

func (s *Store[M]) CommitAndWait(ctx context.Context, actor string, ops ...Op) error {
	done, err := s.commit(ctx, actor, ops)
	if err != nil || done == nil {
		return err
	}
	<-done
	return nil
}

func (s *Store[M]) commit(ctx context.Context, actor string, ops []Op) (<-chan struct{}, error) {
	s.queue.commits.Lock()
	defer s.queue.commits.Unlock()
	s.mu.RLock()
	tables := maps.Clone(s.tables)
	s.mu.RUnlock()
	stamp := time.Now().Format(time.RFC3339)
	real := auth.RealEmailFrom(ctx)
	writes := []Op{}
	log := []Row{}
	for pending := ops; len(pending) > 0; {
		op := pending[0]
		pending = pending[1:]
		tab, ok := s.tabs[op.tab]
		if !ok {
			return nil, fmt.Errorf("%s has no tab %s", s.spec.App, op.tab)
		}
		if s.appOf(tab) != s.spec.App {
			return nil, fmt.Errorf("%s: tab %s is %s's", s.spec.App, op.tab, tab.App)
		}
		if tab.AppendOnly && (op.kind == set || op.kind == update) {
			return nil, fmt.Errorf("%s: tab %s is append-only", s.spec.App, op.tab)
		}
		rows, changes := apply(tables[op.tab], op)
		if len(changes) == 0 {
			continue
		}
		tables[op.tab] = rows
		writes = append(writes, planned(op, changes)...)
		for _, c := range changes {
			if !tab.AppendOnly {
				log = append(log, entries(stamp, actor, real, tab, c)...)
			}
			if tab.Cascade != nil {
				pending = append(pending, tab.Cascade(c.before, c.after)...)
			}
		}
	}
	if len(writes) == 0 {
		return nil, nil
	}
	model, err := s.spec.Build(ctx, tables)
	if err != nil {
		return nil, err
	}
	s.queue.interrupt()
	s.mu.Lock()
	s.tables, s.model = tables, model
	s.mu.Unlock()
	done := make(chan struct{})
	s.queue.Add(func() {
		s.write(writes, log)
		close(done)
	})
	return done, nil
}

// A write the sheet refuses leaves memory ahead of it, and every write queued
// behind may build on the refused one, so nothing after it may run.
func (s *Store[M]) write(writes []Op, log []Row) {
	for len(writes) > 0 {
		run := batch(writes)
		writes = writes[len(run):]
		if err := s.put(run); err != nil {
			logging.Fatal("[ERROR] write", "app", s.spec.App, "tab", run[0].tab, "error", err)
		}
	}
	if len(log) > 0 {
		if err := s.writer.Insert(s.spec.App, ChangeLogTab, log); err != nil {
			logging.Fatal("[ERROR] write the change log", "app", s.spec.App, "error", err)
		}
	}
}

func planned(op Op, changes []change) []Op {
	if (op.kind != set && op.kind != update) || len(op.match) != 1 {
		return []Op{op}
	}
	column := slices.Collect(maps.Keys(op.match))[0]
	if _, renames := op.cells[column]; renames {
		return []Op{op}
	}
	out := []Op{}
	for _, c := range changes {
		key := c.before[column]
		if c.before == nil || key == "" || key != strings.TrimSpace(key) {
			return []Op{op}
		}
		out = append(out, Op{kind: keyed, tab: op.tab, match: Row{column: key}, cells: op.cells})
	}
	return out
}

func batch(writes []Op) []Op {
	first := writes[0]
	if first.kind != insert && first.kind != keyed {
		return writes[:1]
	}
	n := 1
	for n < len(writes) && writes[n].kind == first.kind && writes[n].tab == first.tab && slices.Equal(slices.Sorted(maps.Keys(writes[n].match)), slices.Sorted(maps.Keys(first.match))) {
		n++
	}
	return writes[:n]
}

func (s *Store[M]) put(run []Op) error {
	op := run[0]
	switch op.kind {
	case insert:
		rows := []Row{}
		for _, w := range run {
			rows = append(rows, w.cells)
		}
		return s.writer.Insert(s.spec.App, op.tab, rows)
	case keyed:
		column := slices.Collect(maps.Keys(op.match))[0]
		cells := map[string]map[string]string{}
		for _, w := range run {
			key := w.match[column]
			if cells[key] == nil {
				cells[key] = Row{}
			}
			maps.Copy(cells[key], w.cells)
		}
		return s.writer.SetMany(s.spec.App, op.tab, column, cells)
	case remove:
		return s.writer.Delete(s.spec.App, op.tab, op.match)
	}
	return s.writer.Set(s.spec.App, op.tab, op.match, op.cells)
}

func apply(rows []Row, op Op) ([]Row, []change) {
	switch op.kind {
	case insert:
		row := Row{}
		fill(row, op.cells)
		if len(row) == 0 {
			return rows, nil
		}
		return append(slices.Clone(rows), row), []change{{after: row}}
	case remove:
		kept := []Row{}
		changes := []change{}
		for _, row := range rows {
			if matches(row, op.match) {
				changes = append(changes, change{before: row})
				continue
			}
			kept = append(kept, row)
		}
		return kept, changes
	}
	next := slices.Clone(rows)
	changes := []change{}
	matched := false
	for i, row := range next {
		if !matches(row, op.match) {
			continue
		}
		matched = true
		updated := maps.Clone(row)
		fill(updated, op.cells)
		if maps.Equal(updated, row) {
			continue
		}
		next[i] = updated
		changes = append(changes, change{before: row, after: updated})
	}
	if !matched && op.kind == set {
		row := Row{}
		fill(row, op.match)
		fill(row, op.cells)
		next = append(next, row)
		changes = append(changes, change{after: row})
	}
	return next, changes
}

func fill(row, cells Row) {
	for column, value := range cells {
		if value == "" {
			delete(row, column)
			continue
		}
		row[column] = value
	}
}

func matches(row, match Row) bool {
	for column, value := range match {
		if !strings.EqualFold(strings.TrimSpace(row[column]), strings.TrimSpace(value)) {
			return false
		}
	}
	return true
}

func entries(stamp, actor, real string, tab Tab, c change) []Row {
	named := c.after
	action := "set"
	switch {
	case c.before == nil:
		action = "insert"
	case c.after == nil:
		action = "delete"
		named = c.before
	}
	key := make([]string, 0, len(tab.Key))
	for _, column := range tab.Key {
		key = append(key, column+"="+named[column])
	}
	if c.before == nil {
		return []Row{{"Timestamp": stamp, "Actor": actor, "Real Actor": real, "Action": action, "Tab": tab.Name, "Key": strings.Join(key, "; ")}}
	}
	columns := slices.Sorted(maps.Keys(c.before))
	for column := range c.after {
		if !slices.Contains(columns, column) {
			columns = append(columns, column)
		}
	}
	slices.Sort(columns)
	out := []Row{}
	for _, column := range columns {
		if c.before[column] == c.after[column] {
			continue
		}
		out = append(out, Row{
			"Timestamp": stamp, "Actor": actor, "Real Actor": real, "Action": action,
			"Tab": tab.Name, "Key": strings.Join(key, "; "), "Column": column, "Previous": c.before[column],
		})
	}
	return out
}
