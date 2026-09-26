package birthday

import (
	"net/http"
	"testing"
	"time"
)

// TestLate checks who the toolbar's late-birthdays alert reaches on the
// sample's 9 September: an admin sees every late step - the unassigned
// outreach too, and whose each assigned one is - a volunteer only the
// outreach they took on once it is late, someone off the team nothing, and
// a birthday whose charity is not in by its due-by day late in its own right.
func TestLate(t *testing.T) {
	cache, mux := newServer(t)
	steps := func(email string) map[string]Late {
		out := map[string]Late{}
		for _, l := range cache.Late(fakeDirectory{}, email) {
			out[l.Name+" "+l.Step] = l
		}
		return out
	}
	all := steps(admin)
	if len(all) != 3 {
		t.Fatalf("admin: %+v", all)
	}
	if l := all["Miguel Santos outreach"]; l.Due != "2026-09-03" || l.Assignee != "" || l.Path != "/staff/miguel.santos" {
		t.Errorf("unassigned outreach: %+v", l)
	}
	if l := all["Ruth Amari outreach"]; l.Assignee != "Mina Park" {
		t.Errorf("assigned outreach: %+v", l)
	}
	if _, ok := all["Grace Kim newsletter"]; !ok {
		t.Errorf("newsletter past its day: %+v", all)
	}
	if got := steps(parent); len(got) != 0 {
		t.Errorf("volunteer with nothing late: %+v", got)
	}
	if rec := call(t, mux, parent, "POST", "/api/birthday/assign", map[string]any{"email": "miguel.santos@heliosschool.org"}); rec.Code != http.StatusNoContent {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	if got := steps(parent); len(got) != 1 || got["Miguel Santos outreach"].Due != "2026-09-03" {
		t.Errorf("volunteer's own late outreach: %+v", got)
	}
	if got := steps("nobody@heliosschool.org"); len(got) != 0 {
		t.Errorf("off the team: %+v", got)
	}
	// A day on, Ruth's newsletter of the 11th is two days off - the default
	// Due By Lead Days - and her charity is not in: the outreach gives way
	// to the birthday itself being late.
	now = func() time.Time { return mustTime("2026-09-10") }
	all = steps(admin)
	if l, ok := all["Ruth Amari info"]; !ok || l.Due != "2026-09-09" || l.Assignee != "Mina Park" {
		t.Errorf("birthday info past its due-by day: %+v", all)
	}
	if _, ok := all["Ruth Amari outreach"]; ok {
		t.Errorf("one step a birthday: %+v", all)
	}
}
