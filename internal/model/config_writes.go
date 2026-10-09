package model

import (
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

var ManageSuperAdmins = access.Named("super-admins")

var SuperAllowances = []access.Allowance{ManageSuperAdmins, Triage}

func require(actor access.Actor, allowance access.Allowance) error {
	if !actor.May(allowance) {
		return access.Forbidden("admin access required")
	}
	return nil
}

func (s *Config) setSuperAdmins(actor access.Actor, emails []string) ([]string, []store.Op, error) {
	if err := require(actor, ManageSuperAdmins); err != nil {
		return nil, nil, err
	}
	admins := mail.NormalizeAll(emails)
	if len(admins) == 0 {
		return nil, nil, access.Invalid("the super admin list cannot be empty")
	}
	ops := []store.Op{}
	for _, e := range s.SuperAdmins {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(superAdminsTab, store.Row{configEmailColumn: e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(s.SuperAdmins, e) {
			ops = append(ops, store.Insert(superAdminsTab, store.Row{configEmailColumn: e}))
		}
	}
	return admins, ops, nil
}

func signedOut(email string) []store.Op {
	at := time.Now().Truncate(time.Second)
	return []store.Op{store.Upsert(signedOutTab, store.Row{configEmailColumn: email}, store.Row{signedOutColumn: at.Format(time.RFC3339)})}
}
