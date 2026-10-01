package model

import (
	"slices"
	"sort"

	"heliosian/internal/access"
	"heliosian/internal/mail"
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
