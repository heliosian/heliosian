package model

import (
	"time"

	"heliosian/internal/store"
)

func signedOut(email string) []store.Op {
	at := time.Now().Truncate(time.Second)
	return []store.Op{store.Upsert(signedOutTab, store.Row{configEmailColumn: email}, store.Row{signedOutColumn: at.Format(time.RFC3339)})}
}
