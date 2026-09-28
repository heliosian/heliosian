package admins

import (
	"slices"

	"heliosian/internal/access"
	"heliosian/internal/config"
	"heliosian/internal/store"
)

func (l List) set(actor access.Actor, requested []string) ([]store.Op, []string, error) {
	if !actor.May(Manage(l.app)) {
		return nil, nil, access.Forbidden("admin access required")
	}
	admins := []string{}
	for _, e := range config.NormalizeEmails(requested) {
		if !l.IsSuperAdmin(e) {
			admins = append(admins, e)
		}
	}
	listed := l.listed()
	ops := []store.Op{}
	for _, e := range listed {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(Tab, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(listed, e) {
			ops = append(ops, store.Insert(Tab, store.Row{"Email": e}))
		}
	}
	return ops, admins, nil
}
