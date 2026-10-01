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
	As     string         `json:"as,omitempty"`
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

func cellsOf(t *Table, values map[string]any, where string, names map[string]string) (store.Row, error) {
	out := store.Row{}
	for column, v := range values {
		c, ok := t.Column(column)
		if !ok {
			return nil, access.Invalid("%s: no column %s on %s", where, column, t.Name)
		}
		if c.Generated {
			return nil, access.Invalid("%s: %s.%s is generated and can't be written", where, t.Name, column)
		}
		if c.Kind == ID {
			return nil, access.Invalid("%s: %s.%s is minted by the server and can't be written", where, t.Name, column)
		}
		text, err := cellText(v)
		if err != nil {
			return nil, access.Invalid("%s: %s: %v", where, column, err)
		}
		if c.Kind == Ref || c.Kind == Refs {
			if text, err = resolve(text, c.Kind == Refs, names); err != nil {
				return nil, access.Invalid("%s: %s: %v", where, column, err)
			}
		}
		out[column] = text
	}
	return out, nil
}

func resolve(text string, list bool, names map[string]string) (string, error) {
	parts := []string{text}
	if list {
		parts = cells.SplitList(text)
	}
	for i, part := range parts {
		name, ok := strings.CutPrefix(part, "@")
		if !ok {
			continue
		}
		id, ok := names[name]
		if !ok {
			return "", fmt.Errorf("@%s names no earlier insert in the batch", name)
		}
		parts[i] = id
	}
	if list {
		return cells.JoinList(parts), nil
	}
	return parts[0], nil
}

func Write(ctx context.Context, s *Store, queue *store.Queue, actor access.Actor, env Env, b Batch) ([]string, error) {
	written := []string{}
	authorize := func(m *Model, c Change) error { return m.Authorize(env, c) }
	names := map[string]string{}
	_, err := queue.Transact(ctx, actor, func(tx *store.Tx) error {
		for i, w := range b.Batch {
			where := fmt.Sprintf("batch[%d]", i)
			if w.As != "" {
				if w.Insert == "" || strings.HasPrefix(w.As, "@") {
					return access.Invalid("%s: as names an insert's row, without the @", where)
				}
				if _, dup := names[w.As]; dup {
					return access.Invalid("%s: the batch names two rows %s", where, w.As)
				}
			}
			id, c, err := stageWrite(s, tx, w, where, names, authorize)
			if err != nil {
				return err
			}
			if w.As != "" {
				names[w.As] = id
			}
			if err := fire(s, tx, c, where); err != nil {
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

func stageWrite(s *Store, tx *store.Tx, w write, where string, names map[string]string, authorize func(*Model, Change) error) (string, Change, error) {
	m := s.In(tx)
	switch {
	case w.Insert != "" && w.Set == "" && w.Delete == "" && w.Cells == nil:
		t, ok := Lookup(w.Insert)
		if !ok || t.Generated {
			return "", Change{}, access.Invalid("%s: no table %s to insert into", where, w.Insert)
		}
		row, err := cellsOf(t, w.Row, where, names)
		if err != nil {
			return "", Change{}, err
		}
		for _, c := range t.Columns {
			if c.Kind == ID && c.Required {
				row[c.Name] = Mint(c.Prefix, m.Has)
			}
		}
		id := row["id"]
		if err := t.Check(row); err != nil {
			return "", Change{}, access.Invalid("%s: %v", where, err)
		}
		c := Change{Table: t.Name, New: row}
		if err := authorize(m, c); err != nil {
			return "", Change{}, err
		}
		return id, c, s.Stage(tx, t.Sheet, store.Insert(t.Name, row))
	case w.Set != "" && w.Insert == "" && w.Delete == "" && w.Row == nil:
		target, err := resolve(w.Set, false, names)
		if err != nil {
			return "", Change{}, access.Invalid("%s: %v", where, err)
		}
		t, old, err := m.existing(target, where)
		if err != nil {
			return "", Change{}, err
		}
		set, err := cellsOf(t, w.Cells, where, names)
		if err != nil {
			return "", Change{}, err
		}
		updated := maps.Clone(old)
		for k, v := range set {
			updated[k] = v
		}
		if err := t.Check(updated); err != nil {
			return "", Change{}, access.Invalid("%s: %v", where, err)
		}
		c := Change{Table: t.Name, Old: old, New: updated}
		if err := authorize(m, c); err != nil {
			return "", Change{}, err
		}
		return target, c, s.Stage(tx, t.Sheet, store.Update(t.Name, store.Row{"id": target}, set))
	case w.Delete != "" && w.Insert == "" && w.Set == "" && w.Row == nil && w.Cells == nil:
		target, err := resolve(w.Delete, false, names)
		if err != nil {
			return "", Change{}, access.Invalid("%s: %v", where, err)
		}
		t, old, err := m.existing(target, where)
		if err != nil {
			return "", Change{}, err
		}
		c := Change{Table: t.Name, Old: old}
		if err := authorize(m, c); err != nil {
			return "", Change{}, err
		}
		return target, c, s.Stage(tx, t.Sheet, store.Delete(t.Name, store.Row{"id": target}))
	}
	return "", Change{}, access.Invalid("%s: a write is one of insert with row, set with cells, or delete", where)
}

func (m *Model) existing(id, where string) (*Table, store.Row, error) {
	table, ok := TableOf(strings.TrimSpace(id))
	if !ok {
		return nil, nil, access.Invalid("%s: %q is not an id", where, id)
	}
	if t, _ := Lookup(table); t.Generated {
		return nil, nil, access.Invalid("%s: %s is generated and can't be written", where, table)
	}
	rows := m.Table(table)
	row, ok := rows.Get(id)
	if !ok {
		return nil, nil, access.Missing("%s: no %s %s", where, table, id)
	}
	return rows.table, row, nil
}
