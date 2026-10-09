package model

import (
	"heliosian/internal/api"
)

type dayTypeResource struct {
	Name   string  `json:"name"`
	Role   string  `json:"role,omitempty"`
	Blocks []Block `json:"blocks"`
}

func (r calendarResources) dayTypes() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "day-types",
		Shape: dayTypeResource{},
		Has:   func(m *Model, key string) bool { return m.Calendar.DayType(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			d := m.Calendar.DayType(key)
			if d == nil {
				return nil, false
			}
			return dayTypeResource{Name: d.Name, Role: d.Role, Blocks: d.Blocks}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, d := range m.Calendar.DayTypes {
				out = append(out, d.ID)
			}
			return out
		},
	}
}
