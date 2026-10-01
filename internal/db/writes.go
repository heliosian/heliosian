package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/cells"
	"heliosian/internal/store"
)

type write struct {
	Insert string         `json:"insert,omitempty"`
	Row    map[string]any `json:"row,omitempty"`
	Set    string         `json:"set,omitempty"`
	Cells  map[string]any `json:"cells,omitempty"`
	Delete string         `json:"delete,omitempty"`
}

type Batch struct {
	Batch []write `json:"batch"`
}

func ParseBatch(raw []byte) (Batch, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	var b Batch
	if err := d.Decode(&b); err != nil {
		return Batch{}, fmt.Errorf("the batch is not JSON of the batch's shape: %v", err)
	}
	if len(b.Batch) == 0 {
		return Batch{}, fmt.Errorf("the batch is empty")
	}
	return b, nil
}

func cellText(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case json.Number:
		return x.String(), nil
	case bool:
		return cells.YesNoCell(x), nil
	case nil:
		return "", nil
	}
	return "", fmt.Errorf("%v is not text, a number or true or false", v)
}

func cellsOf(t *Table, values map[string]any, where string) (store.Row, error) {
	out := store.Row{}
	for column, v := range values {
		c, ok := t.Column(column)
		if !ok {
			return nil, access.Invalid("%s: no column %s on %s", where, column, t.Name)
		}
		if c.Generated {
			return nil, access.Invalid("%s: %s.%s is generated and can't be written", where, t.Name, column)
		}
		text, err := cellText(v)
		if err != nil {
			return nil, access.Invalid("%s: %s: %v", where, column, err)
		}
		out[column] = text
	}
	return out, nil
}

func Write(ctx context.Context, s *Store, queue *store.Queue, actor access.Actor, env Env, b Batch) ([]string, error) {
	written := []string{}
	_, err := queue.Transact(ctx, actor, func(tx *store.Tx) error {
		for i, w := range b.Batch {
			where := fmt.Sprintf("batch[%d]", i)
			id, err := stageWrite(s, tx, env, w, where)
			if err != nil {
				return err
			}
			written = append(written, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return written, nil
}

func stageWrite(s *Store, tx *store.Tx, env Env, w write, where string) (string, error) {
	m := s.In(tx)
	switch {
	case w.Insert != "" && w.Set == "" && w.Delete == "" && w.Cells == nil:
		t, ok := Lookup(w.Insert)
		if !ok || t.Generated {
			return "", access.Invalid("%s: no table %s to insert into", where, w.Insert)
		}
		row, err := cellsOf(t, w.Row, where)
		if err != nil {
			return "", err
		}
		id := row["id"]
		if id == "" {
			id = Mint(t.Columns[0].Prefix, m.Has)
			row["id"] = id
		}
		if table, ok := TableOf(id); !ok || table != t.Name {
			return "", access.Invalid("%s: %q is not a %s id", where, id, t.Name)
		}
		if m.Has(id) {
			return "", access.Invalid("%s: %s is taken", where, id)
		}
		if err := t.Check(row); err != nil {
			return "", access.Invalid("%s: %v", where, err)
		}
		if err := m.Authorize(env, Change{Table: t.Name, New: row}); err != nil {
			return "", err
		}
		return id, s.Stage(tx, t.Sheet, store.Insert(t.Name, row))
	case w.Set != "" && w.Insert == "" && w.Delete == "" && w.Row == nil:
		t, old, err := m.existing(w.Set, where)
		if err != nil {
			return "", err
		}
		set, err := cellsOf(t, w.Cells, where)
		if err != nil {
			return "", err
		}
		if _, ok := set["id"]; ok {
			return "", access.Invalid("%s: a row's id can't be changed", where)
		}
		updated := maps.Clone(old)
		for k, v := range set {
			updated[k] = v
		}
		if err := t.Check(updated); err != nil {
			return "", access.Invalid("%s: %v", where, err)
		}
		if err := m.Authorize(env, Change{Table: t.Name, Old: old, New: updated}); err != nil {
			return "", err
		}
		return w.Set, s.Stage(tx, t.Sheet, store.Update(t.Name, store.Row{"id": w.Set}, set))
	case w.Delete != "" && w.Insert == "" && w.Set == "" && w.Row == nil && w.Cells == nil:
		t, old, err := m.existing(w.Delete, where)
		if err != nil {
			return "", err
		}
		if err := m.Authorize(env, Change{Table: t.Name, Old: old}); err != nil {
			return "", err
		}
		return w.Delete, s.Stage(tx, t.Sheet, store.Delete(t.Name, store.Row{"id": w.Delete}))
	}
	return "", access.Invalid("%s: a write is one of insert with row, set with cells, or delete", where)
}

func (m *Model) existing(id, where string) (*Table, store.Row, error) {
	table, ok := TableOf(strings.TrimSpace(id))
	if !ok {
		return nil, nil, access.Invalid("%s: %q is not an id", where, id)
	}
	rows := m.Table(table)
	row, ok := rows.Get(id)
	if !ok {
		return nil, nil, access.Missing("%s: no %s %s", where, table, id)
	}
	return rows.table, row, nil
}
