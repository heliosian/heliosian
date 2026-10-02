package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/data"
	"heliosian/internal/logging"
)

const ChangeLogTab = "Change Log"

var ChangeLogColumns = []string{"Timestamp", "Actor", "Real Actor", "Action", "Tab", "Key", "Column", "Previous"}

type Row = map[string]string

type Tables map[string][]Row

type kind int

const (
	insert kind = iota
	upsert
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

func Upsert(tab string, match, cells Row) Op {
	return Op{kind: upsert, tab: tab, match: match, cells: cells}
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
	Cascade    func(tables Tables, before, after Row) []Op
	AppendOnly bool
}

type Book struct {
	app    string
	tabs   []Tab
	byName map[string]Tab
	source data.Source
	writer data.Writer
	queue  *Queue
}

type Plan struct {
	Tables Tables
	writes []Op
	log    []Row
}

func (p Plan) Empty() bool {
	return len(p.writes) == 0
}

type change struct {
	before, after Row
}

func NewBook(app string, tabs []Tab, source data.Source, writer data.Writer, queue *Queue) (*Book, error) {
	b := &Book{app: app, tabs: tabs, byName: map[string]Tab{}, source: source, writer: writer, queue: queue}
	for _, t := range tabs {
		if len(t.Key) == 0 {
			return nil, fmt.Errorf("%s: tab %s names no key", app, t.Name)
		}
		b.byName[t.Name] = t
	}
	return b, nil
}

func (b *Book) Read(ctx context.Context) (Tables, error) {
	names := map[string][]string{b.app: {}}
	for _, t := range b.tabs {
		names[b.appOf(t)] = append(names[b.appOf(t)], t.Name)
	}
	read := map[string]map[string]data.Tab{}
	for app, tabs := range names {
		headers := []string{}
		if app == b.app && b.logged() {
			headers = []string{ChangeLogTab}
		}
		got, err := b.source.Tabs(ctx, app, tabs, headers)
		if err != nil {
			return nil, err
		}
		read[app] = got
	}
	tables := Tables{}
	for _, t := range b.tabs {
		tab := read[b.appOf(t)][t.Name]
		if err := data.CheckColumns(t.Name, tab.Header, t.Columns); err != nil {
			return nil, err
		}
		tables[t.Name] = tab.Rows
	}
	if b.logged() {
		if err := data.CheckColumns(ChangeLogTab, read[b.app][ChangeLogTab].Header, ChangeLogColumns); err != nil {
			return nil, err
		}
	}
	return tables, nil
}

func (b *Book) logged() bool {
	for _, t := range b.tabs {
		if b.appOf(t) == b.app && !t.AppendOnly {
			return true
		}
	}
	return false
}

func (b *Book) appOf(t Tab) string {
	if t.App == "" {
		return b.app
	}
	return t.App
}

func (b *Book) Plan(ctx context.Context, tables Tables, actor string, ops []Op) (Plan, error) {
	tables = maps.Clone(tables)
	stamp := time.Now().Format(time.RFC3339)
	real := auth.RealEmailFrom(ctx)
	writes := []Op{}
	log := []Row{}
	for pending := ops; len(pending) > 0; {
		op := pending[0]
		pending = pending[1:]
		tab, ok := b.byName[op.tab]
		if !ok {
			return Plan{}, fmt.Errorf("%s has no tab %s", b.app, op.tab)
		}
		if b.appOf(tab) != b.app {
			return Plan{}, fmt.Errorf("%s: tab %s is %s's", b.app, op.tab, tab.App)
		}
		if tab.AppendOnly && (op.kind == upsert || op.kind == update) {
			return Plan{}, fmt.Errorf("%s: tab %s is append-only", b.app, op.tab)
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
				pending = append(pending, tab.Cascade(tables, c.before, c.after)...)
			}
		}
	}
	return Plan{Tables: tables, writes: writes, log: log}, nil
}

func (b *Book) Write(p Plan) <-chan struct{} {
	done := make(chan struct{})
	b.queue.Add(func() {
		b.write(p.writes, p.log)
		close(done)
	})
	return done
}

func (b *Book) write(writes []Op, log []Row) {
	for _, run := range b.batches(writes) {
		if err := b.put(run); err != nil {
			logging.Fatal("write", "app", b.app, "tab", run[0].tab, "error", err)
		}
	}
	if len(log) > 0 {
		if err := b.writer.Insert(b.app, ChangeLogTab, log); err != nil {
			logging.Fatal("write the change log", "app", b.app, "error", err)
		}
	}
}

func planned(op Op, changes []change) []Op {
	if (op.kind != upsert && op.kind != update) || len(op.match) != 1 {
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

func (b *Book) batches(writes []Op) [][]Op {
	out := [][]Op{}
	for _, w := range writes {
		if i := b.joinable(out, w); i >= 0 {
			out[i] = append(out[i], w)
			continue
		}
		out = append(out, []Op{w})
	}
	return out
}

func (b *Book) joinable(out [][]Op, w Op) int {
	if w.kind != insert && w.kind != keyed {
		return -1
	}
	for i := len(out) - 1; i >= 0; i-- {
		first := out[i][0]
		if first.kind == w.kind && first.tab == w.tab && slices.Equal(slices.Sorted(maps.Keys(first.match)), slices.Sorted(maps.Keys(w.match))) {
			return i
		}
		for _, p := range out[i] {
			if b.blocks(p, w) {
				return -1
			}
		}
	}
	return -1
}

func (b *Book) blocks(p, w Op) bool {
	if p.tab != w.tab {
		return (p.kind == insert || p.kind == upsert) && b.references(w, p)
	}
	if w.kind == insert {
		return p.kind == insert || data.Matches(w.cells, p.match)
	}
	if p.kind == insert {
		return data.Matches(p.cells, w.match)
	}
	if !overlap(p.match, w.match) {
		return false
	}
	return p.kind == remove || sharesColumn(p.cells, w.cells) || sharesColumn(p.cells, w.match) || sharesColumn(w.cells, p.match)
}

func (b *Book) references(w, p Op) bool {
	keys := []string{}
	for _, column := range b.byName[p.tab].Key {
		for _, row := range []Row{p.match, p.cells} {
			if k := data.Key(row[column]); k != "" {
				keys = append(keys, k)
			}
		}
	}
	for _, row := range []Row{w.match, w.cells} {
		for _, v := range row {
			for _, k := range keys {
				if strings.Contains(data.Key(v), k) {
					return true
				}
			}
		}
	}
	return false
}

func overlap(a, b Row) bool {
	for column, v := range a {
		if other, ok := b[column]; ok && data.Key(other) != data.Key(v) {
			return false
		}
	}
	return true
}

func sharesColumn(a, b Row) bool {
	for column := range a {
		if _, ok := b[column]; ok {
			return true
		}
	}
	return false
}

func (b *Book) put(run []Op) error {
	op := run[0]
	switch op.kind {
	case insert:
		rows := []Row{}
		for _, w := range run {
			rows = append(rows, w.cells)
		}
		return b.writer.Insert(b.app, op.tab, rows)
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
		return b.writer.SetMany(b.app, op.tab, column, cells)
	case remove:
		return b.writer.Delete(b.app, op.tab, op.match)
	case update:
		return b.writer.Update(b.app, op.tab, op.match, op.cells)
	}
	return b.writer.Upsert(b.app, op.tab, op.match, op.cells)
}

func apply(rows []Row, op Op) ([]Row, []change) {
	switch op.kind {
	case insert:
		row := Row{}
		data.Fill(row, op.cells)
		if len(row) == 0 {
			return rows, nil
		}
		return append(slices.Clone(rows), row), []change{{after: row}}
	case remove:
		kept := []Row{}
		changes := []change{}
		for _, row := range rows {
			if data.Matches(row, op.match) {
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
		if !data.Matches(row, op.match) {
			continue
		}
		matched = true
		updated := maps.Clone(row)
		data.Fill(updated, op.cells)
		if len(differing(row, updated)) == 0 {
			continue
		}
		next[i] = updated
		changes = append(changes, change{before: row, after: updated})
	}
	if !matched && op.kind == upsert {
		row := Row{}
		data.Fill(row, op.match)
		data.Fill(row, op.cells)
		next = append(next, row)
		changes = append(changes, change{after: row})
	}
	return next, changes
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
	key := []string{}
	for _, column := range tab.Key {
		key = append(key, column+"="+named[column])
	}
	if c.before == nil {
		return []Row{{"Timestamp": stamp, "Actor": actor, "Real Actor": real, "Action": action, "Tab": tab.Name, "Key": strings.Join(key, "; ")}}
	}
	out := []Row{}
	for _, column := range differing(c.before, c.after) {
		out = append(out, Row{
			"Timestamp": stamp, "Actor": actor, "Real Actor": real, "Action": action,
			"Tab": tab.Name, "Key": strings.Join(key, "; "), "Column": column, "Previous": c.before[column],
		})
	}
	return out
}

func differing(before, after Row) []string {
	columns := slices.Sorted(maps.Keys(before))
	for column := range after {
		if !slices.Contains(columns, column) {
			columns = append(columns, column)
		}
	}
	slices.Sort(columns)
	out := []string{}
	for _, column := range columns {
		if before[column] != after[column] {
			out = append(out, column)
		}
	}
	return out
}
