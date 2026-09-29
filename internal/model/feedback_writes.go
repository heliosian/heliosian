package model

import (
	"net/http"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

var Triage = access.Named("feedback")

func requireSuperAdmin(actor access.Actor) error {
	if !actor.May(Triage) {
		return access.Forbidden("super admin access required")
	}
	return nil
}

func (m *Feedback) submit(actor access.Actor, r Report) (Report, []store.Op) {
	r.ID = id.New(m.taken)
	r.Status = ReportStatusNew
	r.Email = actor.Email
	r.SuperAdmin = actor.May(Triage)
	return r, []store.Op{store.Insert(reportsTab, r.cells())}
}

func (m *Feedback) filing(actor access.Actor, id string) (Report, error) {
	if err := requireSuperAdmin(actor); err != nil {
		return Report{}, err
	}
	report, ok := m.report(id)
	if !ok {
		return Report{}, access.Missing("no such report")
	}
	if report.Status == ReportStatusFiled {
		return Report{}, access.Refuse(http.StatusConflict, "that report is already filed")
	}
	return report, nil
}

func (m *Feedback) handle(actor access.Actor, key string, cells store.Row) ([]store.Op, error) {
	if err := requireSuperAdmin(actor); err != nil {
		return nil, err
	}
	report, ok := m.report(key)
	if !ok {
		return nil, access.Missing("no such report")
	}
	return []store.Op{store.Update(reportsTab, store.Row{"ID": report.ID}, cells)}, nil
}

func (m *Feedback) filed(actor access.Actor, id, issue string, now time.Time) ([]store.Op, error) {
	return m.handle(actor, id, store.Row{"Status": ReportStatusFiled, "Issue": issue, "Handled": handledCell(now), "Handled By": actor.Email})
}

func (m *Feedback) dismissed(actor access.Actor, id string, now time.Time) ([]store.Op, error) {
	return m.handle(actor, id, store.Row{"Status": ReportStatusDismissed, "Handled": handledCell(now), "Handled By": actor.Email})
}
