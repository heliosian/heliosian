package db

import "heliosian/internal/store"

func stageContentSweep(s *Store, tx *store.Tx, gone []store.Row) error {
	ops := []store.Op{}
	for _, row := range gone {
		ops = append(ops, store.Delete("CONTENT", store.Row{"id": row["id"]}))
	}
	if err := s.Stage(tx, DocumentsSheet, ops...); err != nil {
		return err
	}
	for _, row := range gone {
		if err := fire(s, tx, Change{Table: "CONTENT", Old: row}, Env{System: sweepActor}, sweepActor); err != nil {
			return err
		}
	}
	return nil
}
