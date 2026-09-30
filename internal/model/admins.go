package model

import (
	"net/http"
	"slices"
	"sort"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const adminsTabName = "Admins"

var AdminsTab = store.Tab{Name: adminsTabName, Columns: []string{"Email"}, Key: []string{"Email"}}

func ManageAdmins(app string) access.Allowance {
	return access.Named(app + ".admins")
}

func ReadAdmins(tables store.Tables) []string {
	emails := []string{}
	for _, row := range tables[adminsTabName] {
		emails = append(emails, row["Email"])
	}
	return mail.NormalizeAll(emails)
}

type AdminList struct {
	app         string
	sheet       string
	allowances  []access.Allowance
	listed      []string
	superAdmins []string
}

func (l AdminList) IsSuperAdmin(email string) bool {
	return slices.Contains(l.superAdmins, mail.Normalize(email))
}

func (l AdminList) IsAdmin(email string) bool {
	return l.IsSuperAdmin(email) || slices.Contains(l.listed, mail.Normalize(email))
}

func (l AdminList) Held(email string) []access.Allowance {
	if !l.IsAdmin(email) {
		return nil
	}
	return l.allowances
}

func (l AdminList) Admins() []string {
	out := mail.NormalizeAll(append(slices.Clone(l.listed), l.superAdmins...))
	sort.Strings(out)
	return out
}

func RegisterAdmins(mux *http.ServeMux, s *Store, app string, state func(m *Model, r *http.Request, actor access.Actor) map[string]any) {
	mux.HandleFunc("GET /api/admin/state", serve.JSON(func(r *http.Request, _ serve.None) (map[string]any, error) {
		m := s.Model()
		l := m.AdminList(app)
		v := m.actor(r, app)
		if !v.May(ManageAdmins(l.app)) {
			return nil, access.Forbidden("admin access required")
		}
		view := state(m, r, v)
		view["email"] = v.Email
		view["isSuperAdmin"] = l.IsSuperAdmin(v.Email)
		return view, nil
	}))
}
