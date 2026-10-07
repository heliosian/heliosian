package db

import "heliosian/internal/store"

func stageContentSweep(s *Store, tx *store.Tx, gone []store.Row) error {
	ops := []store.Op{}
	for _, row := range gone {
		ops = append(ops, store.Delete("CONTENT", store.Row{"id": row["id"]}))
	}
	return s.Stage(tx, DocumentsSheet, ops...)
}
