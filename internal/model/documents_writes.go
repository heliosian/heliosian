package model

import (
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func (m *Documents) Record(actor access.Actor, doc *Document) []store.Op {
	return []store.Op{store.Insert(documentsTab, doc.Row())}
}

func (m *Documents) Replace(actor access.Actor, doc *Document) []store.Op {
	return []store.Op{store.Update(documentsTab, store.Row{"Key": doc.Key}, doc.Row())}
}

func (m *Documents) Drop(actor access.Actor, key string) []store.Op {
	return []store.Op{store.Delete(documentsTab, store.Row{"Key": key})}
}

func (m *Documents) removeGroup(actor access.Actor, group string) []store.Op {
	return []store.Op{store.Delete(documentsTab, store.Row{"Kind": DocumentKindGroup, "Channel": group})}
}

func (m *Documents) setPoints(actor access.Actor, key string, points []string, audience, judged string) []store.Op {
	return []store.Op{store.Update(documentsTab, store.Row{"Key": key}, store.Row{DocumentPointsColumn: strings.Join(points, "\n"), DocumentAudienceColumn: audience, DocumentJudgedColumn: judged})}
}
