package feedback

import (
	"net/http"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

var Triage = access.Standing("feedback")

func requireSuperAdmin(actor access.Actor) error {
	if !actor.May(Triage) {
		return access.Forbidden("super admin access required")
	}
	return nil
}

func (m *Model) submit(actor access.Actor, r Report) (Report, []store.Op) {
	r.ID = serve.ID(12)
	r.Status = StatusNew
	r.Email = actor.Email
	r.SuperAdmin = actor.May(Triage)
	return r, []store.Op{store.Insert(reportsTab, r.cells())}
}

func (m *Model) filing(actor access.Actor, id string) (Report, error) {
	if err := requireSuperAdmin(actor); err != nil {
		return Report{}, err
	}
	report, ok := m.report(id)
	if !ok {
		return Report{}, access.Missing("no such report")
	}
	if report.Status == StatusFiled {
		return Report{}, access.Refuse(http.StatusConflict, "that report is already filed")
	}
	return report, nil
}

func (m *Model) handle(actor access.Actor, id string, cells store.Row) ([]store.Op, error) {
	if err := requireSuperAdmin(actor); err != nil {
		return nil, err
	}
	if _, ok := m.report(id); !ok {
		return nil, access.Missing("no such report")
	}
	return []store.Op{store.Update(reportsTab, store.Row{"ID": id}, cells)}, nil
}

func (m *Model) filed(actor access.Actor, id, issue string, now time.Time) ([]store.Op, error) {
	return m.handle(actor, id, store.Row{"Status": StatusFiled, "Issue": issue, "Handled": handledCell(now), "Handled By": actor.Email})
}

func (m *Model) dismissed(actor access.Actor, id string, now time.Time) ([]store.Op, error) {
	return m.handle(actor, id, store.Row{"Status": StatusDismissed, "Handled": handledCell(now), "Handled By": actor.Email})
}
