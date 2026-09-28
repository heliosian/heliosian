package birthday

import (
	"net/http"
	"testing"
	"time"

	"heliosian/internal/testkit"
)

func TestLate(t *testing.T) {
	cache, mux := newServer(t)
	steps := func(email string) map[string]Late {
		out := map[string]Late{}
		for _, l := range cache.Late(directory, email) {
			out[l.Name+" "+l.Step] = l
		}
		return out
	}
	all := steps(admin)
	if len(all) != 2 {
		t.Fatalf("admin: %+v", all)
	}
	if l := all["Miguel Santos outreach"]; l.Due != "2026-09-03" || l.Assignee != "" || l.Path != "/staff/miguel.santos" {
		t.Errorf("unassigned outreach: %+v", l)
	}
	if l := all["Ruth Amari outreach"]; l.Assignee != "Mina Park" {
		t.Errorf("assigned outreach: %+v", l)
	}
	dana, _ := staff(t, mux, admin, "dana.hawkins@heliosschool.org")
	call(t, mux, admin, "POST", "/api/donations/"+dana["donation"].(string)+"/unuse", nil, http.StatusNoContent)
	if l, ok := steps(admin)["Dana Hawkins newsletter"]; !ok || l.Due != "2026-08-21" {
		t.Errorf("newsletter past its day: %+v", steps(admin))
	}
	if got := steps(parent); len(got) != 0 {
		t.Errorf("volunteer with nothing late: %+v", got)
	}
	call(t, mux, parent, "POST", "/api/birthdays/miguel.santos@heliosschool.org/assign", nil, http.StatusNoContent)
	if got := steps(parent); len(got) != 1 || got["Miguel Santos outreach"].Due != "2026-09-03" {
		t.Errorf("volunteer's own late outreach: %+v", got)
	}
	if got := steps("nobody@heliosschool.org"); len(got) != 0 {
		t.Errorf("off the team: %+v", got)
	}
	now = func() time.Time { return testkit.MustTime("2026-09-10") }
	all = steps(admin)
	if l, ok := all["Ruth Amari info"]; !ok || l.Due != "2026-09-09" || l.Assignee != "Mina Park" {
		t.Errorf("birthday info past its due-by day: %+v", all)
	}
	if _, ok := all["Ruth Amari outreach"]; ok {
		t.Errorf("one step a birthday: %+v", all)
	}
}
