package store

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
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
	reorder
)

type Op struct {
	kind   kind
	tab    string
	match  Row
	cells  Row
	column string
	keys   []string
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

func Reorder(tab, column string, keys []string) Op {
	return Op{kind: reorder, tab: tab, column: column, keys: keys}
}

type Tab struct {
	Name    string
	Columns []string
	Key     []string
	Cascade func(before, after Row) []Op
}

type Spec[M any] struct {
	App    string
	Tabs   []Tab
	Build  func(Tables) (M, error)
	Loaded func(model M, took time.Duration)
}

type Enqueuer interface {
	Add(func())
}

type Store[M any] struct {
	spec    Spec[M]
	tabs    map[string]Tab
	source  data.Source
	writer  data.Writer
	queue   Enqueuer
	commits sync.Mutex
	mu      sync.RWMutex
	tables  Tables
	model   M
	pending int
}

func New[M any](spec Spec[M], source data.Source, writer data.Writer, queue Enqueuer) (*Store[M], error) {
	s := &Store[M]{spec: spec, tabs: map[string]Tab{}, source: source, writer: writer, queue: queue}
	for _, t := range spec.Tabs {
		if len(t.Key) == 0 {
			return nil, fmt.Errorf("%s: tab %s names no key", spec.App, t.Name)
		}
		s.tabs[t.Name] = t
	}
	start := time.Now()
	tables, model, err := s.read()
	if err != nil {
		return nil, err
	}
	s.tables, s.model = tables, model
	spec.Loaded(model, time.Since(start))
	go s.refreshLoop()
	return s, nil
}

func (s *Store[M]) read() (Tables, M, error) {
	var none M
	names := make([]string, 0, len(s.spec.Tabs))
	for _, t := range s.spec.Tabs {
		names = append(names, t.Name)
	}
	tabs, err := s.source.Tabs(s.spec.App, names, []string{ChangeLogTab})
	if err != nil {
		return nil, none, err
	}
	tables := Tables{}
	for _, t := range s.spec.Tabs {
		if err := data.CheckColumns(t.Name, tabs[t.Name].Header, t.Columns); err != nil {
			return nil, none, err
		}
		tables[t.Name] = tabs[t.Name].Rows
	}
	if err := data.CheckColumns(ChangeLogTab, tabs[ChangeLogTab].Header, ChangeLogColumns); err != nil {
		return nil, none, err
	}
	model, err := s.spec.Build(tables)
	if err != nil {
		return nil, none, err
	}
	return tables, model, nil
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

func (s *Store[M]) refreshLoop() {
	for range time.Tick(refreshInterval) {
		s.Refresh()
	}
}

func (s *Store[M]) Refresh() {
	s.queue.Add(func() {
		if err := s.refresh(); err != nil {
			slog.Error("[ERROR] refresh", "app", s.spec.App, "error", err)
		}
	})
}

// refresh swaps in what it read only while no commit's writes are still queued:
// memory holds those already, and the sheet does not yet.
func (s *Store[M]) refresh() error {
	start := time.Now()
	tables, model, err := s.read()
	if err != nil {
		return err
	}
	s.commits.Lock()
	defer s.commits.Unlock()
	s.mu.Lock()
	if s.pending > 0 {
		s.mu.Unlock()
		slog.Info("refresh skipped: writes still queued", "app", s.spec.App)
		return nil
	}
	s.tables, s.model = tables, model
	s.mu.Unlock()
	s.spec.Loaded(model, time.Since(start))
	return nil
}

type change struct {
	before, after Row
	from          int
}

func (s *Store[M]) Commit(ctx context.Context, actor string, ops ...Op) error {
	s.commits.Lock()
	defer s.commits.Unlock()
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
			return fmt.Errorf("%s has no tab %s", s.spec.App, op.tab)
		}
		rows, changes, err := apply(tables[op.tab], op)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			continue
		}
		tables[op.tab] = rows
		if op.kind == reorder {
			op.keys = values(rows, op.column)
		}
		writes = append(writes, op)
		for _, c := range changes {
			log = append(log, entries(stamp, actor, real, tab, c)...)
			if tab.Cascade != nil && op.kind != reorder {
				pending = append(pending, tab.Cascade(c.before, c.after)...)
			}
		}
	}
	if len(writes) == 0 {
		return nil
	}
	model, err := s.spec.Build(tables)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.tables, s.model = tables, model
	s.pending++
	s.mu.Unlock()
	s.queue.Add(func() {
		s.write(writes, log)
	})
	return nil
}

func (s *Store[M]) written() {
	s.mu.Lock()
	s.pending--
	s.mu.Unlock()
}

func (s *Store[M]) write(writes []Op, log []Row) {
	for _, op := range writes {
		var err error
		switch op.kind {
		case insert:
			err = s.writer.Insert(s.spec.App, op.tab, []Row{op.cells})
		case set, update:
			err = s.writer.Set(s.spec.App, op.tab, op.match, op.cells)
		case remove:
			err = s.writer.Delete(s.spec.App, op.tab, op.match)
		case reorder:
			err = s.writer.Reorder(s.spec.App, op.tab, op.column, op.keys)
		}
		if err != nil {
			slog.Error("[ERROR] write", "app", s.spec.App, "tab", op.tab, "error", err)
			s.written()
			if err := s.refresh(); err != nil {
				slog.Error("[ERROR] refresh after a failed write", "app", s.spec.App, "error", err)
			}
			return
		}
	}
	if err := s.writer.Insert(s.spec.App, ChangeLogTab, log); err != nil {
		slog.Error("[ERROR] write the change log", "app", s.spec.App, "error", err)
	}
	s.written()
}

func apply(rows []Row, op Op) ([]Row, []change, error) {
	switch op.kind {
	case insert:
		row := Row{}
		fill(row, op.cells)
		if len(row) == 0 {
			return rows, nil, nil
		}
		return append(slices.Clone(rows), row), []change{{after: row}}, nil
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
		return kept, changes, nil
	case reorder:
		return order(rows, op)
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
	return next, changes, nil
}

func order(rows []Row, op Op) ([]Row, []change, error) {
	if len(op.keys) != len(rows) {
		return nil, nil, fmt.Errorf("%s has %d rows but %d were ordered", op.tab, len(rows), len(op.keys))
	}
	used := make([]bool, len(rows))
	next := make([]Row, 0, len(rows))
	changes := []change{}
	for _, key := range op.keys {
		at := -1
		for i, row := range rows {
			if !used[i] && matches(row, Row{op.column: key}) {
				at = i
				break
			}
		}
		if at < 0 {
			return nil, nil, fmt.Errorf("%s has no row with %s %q left to place", op.tab, op.column, key)
		}
		used[at] = true
		if at != len(next) {
			changes = append(changes, change{before: rows[at], after: rows[at], from: at + 1})
		}
		next = append(next, rows[at])
	}
	return next, changes, nil
}

func values(rows []Row, column string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row[column])
	}
	return out
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
	if c.from > 0 {
		return []Row{{
			"Timestamp": stamp, "Actor": actor, "Real Actor": real, "Action": "reorder",
			"Tab": tab.Name, "Key": strings.Join(key, "; "), "Previous": strconv.Itoa(c.from),
		}}
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
