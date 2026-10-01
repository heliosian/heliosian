package model

import (
	"context"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func dropDocumentToDos(_ store.Tables, before, after store.Row) []store.Op {
	if before == nil || after != nil {
		return nil
	}
	named := store.Row{"Document": before["Key"]}
	return []store.Op{store.Delete(toDosTab, named), store.Delete(toDoReadsTab, named)}
}

func (m *Documents) setToDos(actor access.Actor, document string, toDos []ToDo, read string) ([]store.Op, error) {
	if m.byKey[document] == nil {
		return nil, access.Missing("no document %s", document)
	}
	ops := []store.Op{store.Delete(toDosTab, store.Row{"Document": document})}
	for _, t := range toDos {
		t.Document = document
		if err := CheckToDo(t); err != nil {
			return nil, access.Invalid("%v", err)
		}
		ops = append(ops, store.Insert(toDosTab, store.Row{"Document": document, "Title": t.Title, "Summary": t.Summary, "Details": t.Details, "Link": t.Link, "Due": t.Due}))
	}
	return append(ops, store.Upsert(toDoReadsTab, store.Row{"Document": document}, store.Row{"Read": read})), nil
}

func (s *Store) SetToDos(ctx context.Context, actor access.Actor, document string, toDos []ToDo, read string) error {
	ops, err := s.Model().Documents.setToDos(actor, document, toDos, read)
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
