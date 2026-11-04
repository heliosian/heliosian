package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func repeatedRuleGroups(t *testing.T, s *Store, levels int) []string {
	t.Helper()
	edits := []Edit{
		{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "name": "Rule managers", "status": "open", "added_by": parent}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": parent, "member": "yes"}},
		{Insert: "GROUP", As: "base", Row: map[string]any{"kind": "group", "name": "Base", "status": "open", "added_by": parent, "managed_by": "@managers"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@base", "person": parent, "member": "yes"}},
	}
	indices := []int{3}
	previous := "base"
	for i := range levels {
		name := fmt.Sprintf("level%d", i)
		indices = append(indices, len(edits))
		edits = append(edits, Edit{Insert: "GROUP", As: name, Row: map[string]any{"kind": "group", "name": name, "status": "open", "added_by": parent, "managed_by": "@managers"}})
		for _, order := range []string{"a", "b"} {
			edits = append(edits, Edit{Insert: "RULE", Row: map[string]any{"group": "@" + name, "target": "@" + previous, "order": order}})
		}
		previous = name
	}
	body, err := json.Marshal(Batch{Batch: edits})
	if err != nil {
		t.Fatal(err)
	}
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("a member creating their own group rules got %d: %s", rec.Code, rec.Body)
	}
	var out written
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	groups := []string{}
	for _, index := range indices {
		groups = append(groups, out.Result[index])
	}
	return groups
}

func TestRepeatedGroupRulesDoNotMultiplyMembershipWork(t *testing.T) {
	s := sample(t)
	groups := repeatedRuleGroups(t, s, 12)
	m := s.Model()
	measure := func(group string) float64 {
		t.Helper()
		allocs := testing.AllocsPerRun(1, func() {
			rows := m.effectiveOf(group, reads{})
			if len(rows) != 1 || rows[0]["person"] != parent || rows[0]["reasons"] != "rule a, rule b" {
				t.Fatalf("repeated rules resolved to %v", rows)
			}
		})
		t.Logf("%s: %.0f allocations", group, allocs)
		return allocs
	}
	small := measure(groups[6])
	large := measure(groups[12])
	if large > 4*small {
		t.Fatalf("doubling rule depth multiplied allocations from %.0f to %.0f", small, large)
	}
}

func TestMembersCanQueryRepeatedGroupRules(t *testing.T) {
	s := sample(t)
	groups := repeatedRuleGroups(t, s, 12)
	top := groups[len(groups)-1]
	check := func(person string) {
		t.Helper()
		rec := send(t, s, "QUERY", "text/plain", "rowan.ashdown@example.org", `(from EFFECTIVE_MEMBER (where (= group "`+top+`")))`)
		if rec.Code != http.StatusOK {
			t.Fatalf("querying the member's rules got %d: %s", rec.Code, rec.Body)
		}
		var out answer
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if person == "" {
			if len(out.Result) != 0 {
				t.Fatalf("empty rules returned %v", out.Result)
			}
			return
		}
		if len(out.Result) != 1 {
			t.Fatalf("the shared rules returned %v", out.Result)
		}
		row := out.Resources["EFFECTIVE_MEMBER"][out.Result[0]]
		if row["person"] != person || row["reasons"] != "rule a, rule b" || row["group"] != top {
			t.Fatalf("the shared rules returned %v", row)
		}
	}
	check(parent)
	member := memberRow(t, s, groups[0], parent)
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch":[{"delete":"`+member+`"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("removing the base member got %d: %s", rec.Code, rec.Body)
	}
	check("")
	rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", `{"batch":[{"insert":"MEMBER","row":{"group":"`+groups[0]+`","person":"`+staff+`","member":"yes"}}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("adding a base member got %d: %s", rec.Code, rec.Body)
	}
	check(staff)
}
