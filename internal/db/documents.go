package db

import "heliosian/internal/store"

func (m *Model) DocumentReplacements(env Env, ids []string) map[string]string {
	r := m.newRun(env)
	replacements := map[string]string{}
	for _, id := range ids {
		if replacement := r.documentReplacement(id); replacement != nil {
			replacements[id] = replacement["id"]
		}
	}
	return replacements
}

func (r *run) documentReplacement(id string) store.Row {
	docs := r.m.Table("DOCUMENT")
	for id != "" {
		row, ok := docs.Get(id)
		if !ok || !readable(r, "DOCUMENT", id) {
			return nil
		}
		if next := row["superseded_by"]; next != "" {
			var replacement store.Row
			for next != "" {
				replacement, _ = docs.Get(next)
				next = replacement["superseded_by"]
			}
			if replacement != nil && readable(r, "DOCUMENT", replacement["id"]) {
				return replacement
			}
		}
		id = row["parent"]
	}
	return nil
}
