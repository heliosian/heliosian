package model

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
)

var streetAddress = regexp.MustCompile(`^\s*\d`)

func located(f Family) bool {
	return f.Address != "" && streetAddress.MatchString(f.Address) && f.Lat != 0
}

func milesBetween(a, b Family) float64 {
	const earth = 3958.8
	lat1, lat2 := a.Lat*math.Pi/180, b.Lat*math.Pi/180
	dLat, dLng := lat2-lat1, (b.Lng-a.Lng)*math.Pi/180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earth * math.Asin(math.Sqrt(h))
}

func nearRanker(m *Model, _ api.Query, value string, keep func(string) bool) ([]api.Hit, error) {
	d := m.Directory
	key := strings.TrimSpace(value)
	from, ok := d.Families[key]
	if !ok {
		return nil, access.Missing("no family %s", value)
	}
	if !located(from) {
		return nil, access.Invalid("that family shares no street address to measure from")
	}
	m.scope.nearFrom = &from
	hits := []api.Hit{}
	for other, f := range d.Families {
		if other != key && located(f) && keep(other) {
			hits = append(hits, api.Hit{ID: other, Score: math.Round(milesBetween(from, f)*10) / 10})
		}
	}
	slices.SortFunc(hits, func(a, b api.Hit) int {
		if a.Score != b.Score {
			if a.Score < b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(d.Families[a.ID].Name, d.Families[b.ID].Name)
	})
	return hits, nil
}

func withinFilter(m *Model, _ api.Query, value string) (func(string) bool, error) {
	miles, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || miles < 0 {
		return nil, access.Invalid("within %q is not a distance in miles", value)
	}
	s, d := m.scope, m.Directory
	return func(key string) bool {
		f, ok := d.Families[key]
		return ok && s.nearFrom != nil && located(f) && milesBetween(*s.nearFrom, f) <= miles
	}, nil
}

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
	"within":    withinFilter,
}
