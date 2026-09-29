package ask

import (
	"sync"

	"heliosian/internal/model"
)

type groupAccess struct {
	once  sync.Once
	names map[string]bool
}

func (v *viewer) readableGroups() map[string]bool {
	v.access.once.Do(func() {
		v.access.names = map[string]bool{}
		sources := v.audience()
		for _, g := range v.loop.Groups {
			if g.MailReadableBy(v.email, sources) {
				v.access.names[g.Name] = true
			}
		}
	})
	return v.access.names
}

func (v *viewer) canRead(d *model.Document) bool {
	if d.Kind != model.DocumentKindGroup {
		return true
	}
	return v.readableGroups()[d.Channel]
}

func (v *viewer) documents() *model.Documents {
	return v.library.Where(v.canRead)
}

func (v *viewer) groupOf(d *model.Document) string {
	if d.Kind != model.DocumentKindGroup {
		return ""
	}
	g := v.loop.Named(d.Channel)
	if g == nil {
		return d.Channel
	}
	return g.Title + " (" + g.Address() + ")"
}
