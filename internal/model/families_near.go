package model

import (
	"slices"
	"strings"

	"heliosian/internal/api"
)

func familyWhere(keep func(d *Directory, key string, f Family, value string) bool) api.Filter[*Model] {
	return func(m *Model, _ api.Query, value string) (func(string) bool, error) {
		value = strings.TrimSpace(value)
		d := m.Directory
		return func(key string) bool {
			f, ok := d.Families[key]
			return ok && keep(d, key, f, value)
		}, nil
	}
}

func kidsWhere(place func(*Person) string) api.Filter[*Model] {
	return familyWhere(func(d *Directory, key string, _ Family, value string) bool {
		_, kids := d.Members(key)
		return slices.ContainsFunc(kids, func(p *Person) bool { return strings.EqualFold(place(p), value) })
	})
}

var familyFilters = map[string]api.Filter[*Model]{
	"q": familyWhere(func(d *Directory, key string, f Family, value string) bool {
		if mentions(value, f.Name) {
			return true
		}
		adults, kids := d.Members(key)
		return slices.ContainsFunc(append(adults, kids...), func(p *Person) bool { return mentions(value, p.FullName, p.PreferredName) })
	}),
	"classroom": kidsWhere(func(p *Person) string { return p.Classroom }),
	"grade":     kidsWhere(func(p *Person) string { return p.Grade }),
}
