package model

import (
	"net/http"
	"testing"
	"time"

	"heliosian/internal/testkit"
)

type lateStep struct {
	step, due, assignee, path string
}

func lateSteps(t *testing.T, mux http.Handler, as string) map[string]lateStep {
	t.Helper()
	r := get(t, mux, as, "/api/birthdays?late&include=person,assignee")
	out := map[string]lateStep{}
	for _, key := range r.ids(t) {
		b := r.Resources["birthdays"][key]
		late, _ := b["late"].(map[string]any)
		if late == nil {
			t.Fatalf("a late birthday carries no late: %v", b)
		}
		name, _ := r.follow(b, "person", "people")["fullName"].(string)
		assignee, _ := r.follow(b, "assignee", "people")["fullName"].(string)
		step := late["step"].(string)
		out[name+" "+step] = lateStep{step: step, due: late["due"].(string), assignee: assignee, path: b["path"].(string)}
	}
	return out
}

func TestLate(t *testing.T) {
	_, mux := birthdaysServer(t)
	all := lateSteps(t, mux, jordan)
	if len(all) != 2 {
		t.Fatalf("admin: %+v", all)
	}
	if l := all["Miguel Santos outreach"]; l.due != "2026-09-03" || l.assignee != "" || l.path != "/staff/miguel.santos" {
		t.Errorf("unassigned outreach: %+v", l)
	}
	if l := all["Ruth Amari outreach"]; l.assignee != "Mina Park" {
		t.Errorf("assigned outreach: %+v", l)
	}
	if b, _ := staff(t, mux, jordan, "miguel.santos@heliosschool.org"); b["late"] == nil {
		t.Errorf("one birthday read alone carries no late: %v", b)
	}
	dana, _ := staff(t, mux, jordan, "dana.hawkins@heliosschool.org")
	birthdaysCall(t, mux, jordan, "POST", "/api/donations/"+dana["donation"].(string)+"/unuse", nil, http.StatusNoContent)
	if l, ok := lateSteps(t, mux, jordan)["Dana Hawkins newsletter"]; !ok || l.due != "2026-08-21" {
		t.Errorf("newsletter past its day: %+v", lateSteps(t, mux, jordan))
	}
	if got := lateSteps(t, mux, parent); len(got) != 0 {
		t.Errorf("volunteer with nothing late: %+v", got)
	}
	birthdaysCall(t, mux, parent, "POST", "/api/birthdays/miguel.santos@heliosschool.org/assign", nil, http.StatusNoContent)
	if got := lateSteps(t, mux, parent); len(got) != 1 || got["Miguel Santos outreach"].due != "2026-09-03" {
		t.Errorf("volunteer's own late outreach: %+v", got)
	}
	if got := lateSteps(t, mux, "nobody@heliosschool.org"); len(got) != 0 {
		t.Errorf("off the team: %+v", got)
	}
	now = func() time.Time { return testkit.MustTime("2026-09-10") }
	all = lateSteps(t, mux, jordan)
	if l, ok := all["Ruth Amari info"]; !ok || l.due != "2026-09-09" || l.assignee != "Mina Park" {
		t.Errorf("birthday info past its due-by day: %+v", all)
	}
	if _, ok := all["Ruth Amari outreach"]; ok {
		t.Errorf("one step a birthday: %+v", all)
	}
}
