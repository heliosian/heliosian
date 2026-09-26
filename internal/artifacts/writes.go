package artifacts

import (
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func (m *Model) Record(actor access.Actor, doc *Document) []store.Op {
	return []store.Op{store.Insert(documentsTab, doc.Row())}
}

func (m *Model) Replace(actor access.Actor, doc *Document) []store.Op {
	return []store.Op{store.Update(documentsTab, store.Row{"Key": doc.Key}, doc.Row())}
}

func (m *Model) Drop(actor access.Actor, key string) []store.Op {
	return []store.Op{store.Delete(documentsTab, store.Row{"Key": key})}
}

func (m *Model) removeGroup(actor access.Actor, group string) []store.Op {
	return []store.Op{store.Delete(documentsTab, store.Row{"Kind": KindGroup, "Channel": group})}
}

func (m *Model) setPoints(actor access.Actor, key string, points []string, audience, judged string) []store.Op {
	return []store.Op{store.Update(documentsTab, store.Row{"Key": key}, store.Row{PointsColumn: strings.Join(points, "\n"), AudienceColumn: audience, JudgedColumn: judged})}
}
