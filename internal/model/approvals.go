package model

import "sort"

type Approval struct {
	App   string `json:"app"`
	Title string `json:"title"`
	Start string `json:"start,omitempty"`
	Path  string `json:"path"`
}

func (m *Model) Approvals(email string) []Approval {
	out := []Approval{}
	if m.AdminList("team").IsAdmin(email) {
		out = append(out, m.Activities.pending()...)
	}
	if m.AdminList("celebrate").IsAdmin(email) {
		out = append(out, m.Parties.pending()...)
	}
	if m.AdminList("when").IsAdmin(email) {
		out = append(out, m.Calendar.pending()...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Start == "") != (out[j].Start == "") {
			return out[j].Start == ""
		}
		return out[i].Start < out[j].Start
	})
	return out
}
