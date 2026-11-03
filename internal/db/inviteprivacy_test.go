package db

import (
	"fmt"
	"testing"
	"time"
)

func TestDeclinedInvitationsArePrivateAcrossQueryTables(t *testing.T) {
	const host, outsider = "per00000020001", "per00000020020"
	for _, kind := range []string{"event", "activity", "party"} {
		t.Run(kind, func(t *testing.T) {
			s := sample(t)
			ids := apply(t, s, []Edit{
				{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Movie Hosts", "status": "open"}},
				{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": host, "member": "yes"}},
				{Insert: "GROUP", Row: map[string]any{"kind": kind, "name": "Movie Night", "status": "open", "managed_by": "@hosts", "visible_to": "grp00000000004", "members_visible_to": "grp00000000004"}},
			})
			g, _ := s.Model().Table("GROUP").Get(ids[2])
			apply(t, s, answerGroupEdits(g))
			g, _ = s.Model().Table("GROUP").Get(ids[2])
			apply(t, s, s.Model().answerEdits(g, student, "no", student, time.Now()))
			for _, table := range []string{"MEMBER", "EFFECTIVE_MEMBER"} {
				query := fmt.Sprintf(`(from %s (where (= group %q) (= person %q)) (columns person))`, table, g["rsvp_no"], student)
				for _, viewer := range []string{host, parent, student, staff} {
					if got := len(as(t, s, viewer, query)); got != 1 {
						t.Errorf("%s reader %s saw %d private answers", table, viewer, got)
					}
				}
				if got := len(as(t, s, outsider, query)); got != 0 {
					t.Errorf("%s leaked %d declined answers to unrelated parent", table, got)
				}
			}
			apply(t, s, s.Model().answerEdits(g, parent, "yes", parent, time.Now()))
			for _, table := range []string{"MEMBER", "EFFECTIVE_MEMBER"} {
				if got := len(as(t, s, outsider, fmt.Sprintf(`(from %s (where (= group %q)))`, table, g["rsvp_yes"]))); got != 1 {
					t.Errorf("%s public yes count = %d", table, got)
				}
				if got := len(as(t, s, outsider, fmt.Sprintf(`(from PERSON (where (in id (select %s.person (= group %q)))))`, table, g["rsvp_no"]))); got != 0 {
					t.Errorf("%s select leaked %d decliners", table, got)
				}
			}
			if can, err := s.Model().passes(t.Context(), Env{Viewer: outsider}, "GROUP", g["rsvp_no"], "(sees_members @row)"); err != nil || can {
				t.Errorf("a stranger may use the decline group as a membership rule target: %t %v", can, err)
			}
			brought := apply(t, s, []Edit{
				{Insert: "PERSON", As: "brought", Row: map[string]any{"source": "guest", "name_long_override": "Family Friend"}},
				{Insert: "MEMBER", Row: map[string]any{"group": g["id"], "person": "@brought", "member": "yes", "guest_of": outsider}},
			})[0]
			apply(t, s, s.Model().answerEdits(g, brought, "no", outsider, time.Now()))
			for _, table := range []string{"MEMBER", "EFFECTIVE_MEMBER"} {
				if got := len(as(t, s, outsider, fmt.Sprintf(`(from %s (where (= group %q) (= person %q)))`, table, g["rsvp_no"], brought))); got != 1 {
					t.Errorf("%s did not show the answer of a brought guest", table)
				}
			}
			adminGroup := apply(t, s, []Edit{
				{Insert: "GROUP", As: "admins", Row: map[string]any{"kind": "admins", "name": "App Admins", "status": "open"}},
				{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": outsider, "member": "yes"}},
				{Set: "app00000000001", Cells: map[string]any{"admins": "@admins"}},
				{Set: "app00000000004", Cells: map[string]any{"admins": "@admins"}},
			})[0]
			for _, table := range []string{"MEMBER", "EFFECTIVE_MEMBER"} {
				if got := len(as(t, s, outsider, fmt.Sprintf(`(from %s (where (= group %q) (= person %q)))`, table, g["rsvp_no"], student))); got != 0 {
					t.Errorf("%s leaked %d declined answers to unrelated directory/home admin", table, got)
				}
			}
			app := map[string]string{"event": whenApp, "activity": "app00000000005", "party": "app00000000002"}[kind]
			apply(t, s, []Edit{{Set: app, Cells: map[string]any{"admins": adminGroup}}})
			for _, table := range []string{"MEMBER", "EFFECTIVE_MEMBER"} {
				if got := len(as(t, s, outsider, fmt.Sprintf(`(from %s (where (= group %q) (= person %q)))`, table, g["rsvp_no"], student))); got != 1 {
					t.Errorf("%s app admin saw %d declined answers", table, got)
				}
			}
		})
	}
}
