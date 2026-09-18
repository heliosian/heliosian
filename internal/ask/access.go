package ask

import (
	"slices"
	"sync"

	"heliosian/internal/artifacts"
	"heliosian/internal/loop"
)

type groupAccess struct {
	once  sync.Once
	names map[string]bool
}

func (v *viewer) readableGroups() map[string]bool {
	v.access.once.Do(func() {
		v.access.names = map[string]bool{}
		sources := v.sources.LoopSources()
		for _, g := range v.loop.Groups {
			if g.Manages(v.email) {
				v.access.names[g.Name] = true
				continue
			}
			if g.Visibility != loop.VisibilityEveryone && g.Visibility != loop.VisibilityMembers {
				continue
			}
			if slices.Contains(loop.Members(g, sources), v.email) {
				v.access.names[g.Name] = true
			}
		}
	})
	return v.access.names
}

func (v *viewer) canRead(d *artifacts.Document) bool {
	if d.Kind != artifacts.KindGroup {
		return true
	}
	return v.readableGroups()[d.Channel]
}

func (v *viewer) documents() *artifacts.Model {
	return v.artifacts.Where(v.canRead)
}

func (v *viewer) groupOf(d *artifacts.Document) string {
	if d.Kind != artifacts.KindGroup {
		return ""
	}
	g := v.loop.Group(d.Channel)
	if g == nil {
		return d.Channel
	}
	return g.Title + " (" + g.Address() + ")"
}
