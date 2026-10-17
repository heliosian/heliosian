package db

import "heliosian/internal/store"

func stageContentSweep(s *Store, tx *store.Tx, gone []store.Row) error {
	w := &pendingWrites{}
	for _, row := range gone {
		w.add(Edit{Delete: row["id"]})
	}
	_, err := stageEdits(s, tx, Env{System: sweepActor}, w.edits, sweepActor, true)
	return err
}
