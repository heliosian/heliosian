package model

import (
	"context"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func dropDocumentReading(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after != nil {
		return nil
	}
	named := store.Row{"Document": before["Key"]}
	return []store.Op{store.Delete(toDosTab, named), store.Delete(readsTab, named)}
}

func (m *Documents) setReading(actor access.Actor, document string, reading Reading, read string) ([]store.Op, error) {
	if m.byKey[document] == nil {
		return nil, access.Missing("no document %s", document)
	}
	if strings.Contains(reading.Summary, "\n") {
		return nil, access.Invalid("the summary of %s is more than one line", document)
	}
	ops := []store.Op{store.Delete(toDosTab, store.Row{"Document": document})}
	for _, t := range reading.ToDos {
		t.Document = document
		if err := CheckToDo(t); err != nil {
			return nil, access.Invalid("%v", err)
		}
		if t.Point > len(reading.Points) {
			return nil, access.Invalid("to-do %q names point %d of %d", t.Title, t.Point, len(reading.Points))
		}
		point := ""
		if t.Point > 0 {
			point = strconv.Itoa(t.Point)
		}
		ops = append(ops, store.Insert(toDosTab, store.Row{"Document": document, "Title": t.Title, "Summary": t.Summary, "Details": t.Details, "Link": t.Link, "Due": t.Due, "Point": point}))
	}
	for point, toDo := range reading.Repeats {
		listed := m.toDoIDs[toDo]
		if point < 1 || point > len(reading.Points) || listed == nil || listed.Document == document {
			return nil, access.Invalid("point %d repeats %q, not another email's to-do", point, toDo)
		}
	}
	row := store.Row{"Read": read, "Summary": reading.Summary, "Key Points": strings.Join(reading.Points, "\n"), "Audience": reading.Audience, "Repeats": repeatsCell(reading.Repeats)}
	return append(ops, store.Upsert(readsTab, store.Row{"Document": document}, row)), nil
}

func (s *Store) SetReading(ctx context.Context, actor access.Actor, document string, reading Reading, read string) error {
	ops, err := s.Model().Documents.setReading(actor, document, reading, read)
	if err != nil {
		return err
	}
	return s.Commit(ctx, actor, DocumentsApp, ops...)
}

func (m *Home) setToDoState(actor access.Actor, toDo, state string, at time.Time) []store.Op {
	match := store.Row{"Email": actor.Email, "To Do": toDo}
	if state == "" {
		return []store.Op{store.Delete(homeToDosTab, match)}
	}
	return []store.Op{store.Upsert(homeToDosTab, match, store.Row{"State": state, "Changed": at.In(Location).Format(toDoChangedTime)})}
}
