package model

import (
	"slices"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

func (l AdminList) set(actor access.Actor, requested []string) ([]store.Op, []string, error) {
	if !actor.May(ManageAdmins(l.app)) {
		return nil, nil, access.Forbidden("admin access required")
	}
	admins := []string{}
	for _, e := range mail.NormalizeAll(requested) {
		if !l.IsSuperAdmin(e) {
			admins = append(admins, e)
		}
	}
	listed := l.listed
	ops := []store.Op{}
	for _, e := range listed {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTabName, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(listed, e) {
			ops = append(ops, store.Insert(adminsTabName, store.Row{"Email": e}))
		}
	}
	return ops, admins, nil
}
