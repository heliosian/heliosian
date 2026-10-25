package db

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestJoiningAFamilyReachesTheGroupsOfItsParents(t *testing.T) {
	s := sample(t)
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	s.Model().generated(effective)
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000020", "person": "per00000000004", "member": "yes"}}})
	m := s.Model()
	fresh := *m
	fresh.derived = newDerived()
	people := func(m *Model) []string {
		out := []string{}
		for _, row := range m.effectiveRows("grp00000000030") {
			out = append(out, row["person"])
		}
		return out
	}
	if got, want := people(m), people(&fresh); !slices.Equal(got, want) {
		t.Fatalf("the Hummingbirds parents after per00000000004 joined the Ashdown family: carried %v, built %v", got, want)
	}
}

func TestAMembershipLeavesOtherGroupsEffectiveMembersCached(t *testing.T) {
	s := sample(t)
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	s.Model().generated(effective)
	apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000060", "person": "per00000000002", "member": "yes"}}})
	m := s.Model()
	m.derived.mu.Lock()
	_, everyone := m.derived.byGroup["grp00000000004"]
	_, joined := m.derived.byGroup["grp00000000060"]
	m.derived.mu.Unlock()
	if !everyone || joined {
		t.Fatalf("after joining grp00000000060, everyone's entry is cached %v and grp00000000060's %v", everyone, joined)
	}
}

func TestEffectiveMembersCarriedAcrossWritesMatchAFreshBuild(t *testing.T) {
	s := sample(t)
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	idsOf := func(table string) []string {
		out := []string{}
		for _, row := range s.Model().Table(table).All() {
			out = append(out, row["id"])
		}
		return out
	}
	groups, people, families := idsOf("GROUP"), idsOf("PERSON"), []string{}
	for _, g := range s.Model().Table("GROUP").All() {
		if g["kind"] == "family" {
			families = append(families, g["id"])
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	pick := func(ids []string) string { return ids[rng.IntN(len(ids))] }
	for i := range 200 {
		s.Model().generated(effective)
		edit := Edit{}
		switch rng.IntN(11) {
		case 9:
			edit = Edit{Insert: "MEMBER", Row: map[string]any{"group": pick(families), "person": pick(people), "member": "yes"}}
		case 10:
			family := map[string]bool{}
			for _, f := range families {
				family[f] = true
			}
			rows := []string{}
			for _, row := range s.Model().Table("MEMBER").All() {
				if family[row["group"]] {
					rows = append(rows, row["id"])
				}
			}
			if len(rows) == 0 {
				continue
			}
			edit = Edit{Delete: pick(rows)}
		case 0, 1:
			edit = Edit{Insert: "MEMBER", Row: map[string]any{"group": pick(groups), "person": pick(people), "member": pick([]string{"yes", "yes", "excluded"})}}
		case 2:
			edit = Edit{Delete: pick(idsOf("MEMBER"))}
		case 3:
			edit = Edit{Set: pick(people), Cells: map[string]any{"deactivated": pick([]string{"2026-09-30 12:00", ""})}}
		case 4:
			edit = Edit{Set: pick(people), Cells: map[string]any{"consent": pick([]string{"listed", "withheld"})}}
		case 5:
			edit = Edit{Set: pick(idsOf("RULE")), Cells: map[string]any{"target": pick(groups)}}
		case 6:
			edit = Edit{Set: pick(groups), Cells: map[string]any{"parent": ""}}
		case 7:
			row := map[string]any{"group": pick(groups), "order": fmt.Sprintf("o%03dm", i), "target": pick(groups)}
			switch rng.IntN(4) {
			case 0:
				row["under"] = "Yes"
			case 1:
				row["within"] = pick(groups)
			case 2:
				row["exclude"] = "Yes"
			case 3:
				row["replace_with"] = pick([]string{"household", "parents", "children"})
			}
			edit = Edit{Insert: "RULE", Row: row}
		case 8:
			edit = Edit{Insert: "RULE", Row: map[string]any{"group": pick(groups), "order": fmt.Sprintf("o%03dm", i), "search": pick([]string{"a", "ash", "example"})}}
		}
		if _, err := Write(t.Context(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{edit}}); err != nil {
			t.Logf("edit %d refused: %v", i, err)
			continue
		}
		t.Logf("edit %d: %+v", i, edit)
		got := s.Model()
		fresh := *got
		fresh.derived = newDerived()
		carried, built := got.generated(effective).rows, fresh.generated(effective).rows
		if !slices.EqualFunc(carried, built, func(a, b store.Row) bool { return maps.Equal(a, b) }) {
			t.Fatalf("after edit %d the carried effective members (%d rows) differ from a fresh build (%d rows)", i, len(carried), len(built))
		}
	}
}

func TestAWriteBuildsEffectiveMembersBeforeAnyoneAsks(t *testing.T) {
	s := sample(t)
	StartWarmer(s)
	apply(t, s, []Edit{{Set: "grp00000000030", Cells: map[string]any{"subtitle": "changed"}}})
	m := s.Model()
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		m.derived.mu.Lock()
		started := m.derived.sets[effective.Name] != nil
		m.derived.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no effective members were built after the write")
		}
	}
	if n := len(m.generated(effective).rows); n == 0 {
		t.Fatal("the effective members built after the write are empty")
	}
}
