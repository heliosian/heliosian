package db

import (
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

func hiddenByPasses(m *Model) map[string]bool {
	hidden := map[string]bool{}
	r := m.bareRun(true, time.Now())
	for _, t := range Tables {
		show, ok := policies.show[t.Name]
		if !ok {
			continue
		}
		for _, row := range m.Table(t.Name).All() {
			if !show.eval(&frame{table: &t, row: row, name: "row", run: r}) {
				hidden[row["id"]] = true
			}
		}
	}
	for grew := true; grew; {
		grew = false
		for _, t := range Tables {
			if t.Generated {
				continue
			}
			for _, row := range m.Table(t.Name).All() {
				if hidden[row["id"]] {
					continue
				}
				if slices.ContainsFunc(t.Columns, func(c Column) bool { return c.Kind == Ref && hidden[row[c.Name]] }) {
					hidden[row["id"]] = true
					grew = true
				}
			}
		}
	}
	return hidden
}

func hiddenByViews(m *Model) map[string]bool {
	hidden := map[string]bool{}
	for _, t := range Tables {
		if t.Generated {
			continue
		}
		v := m.Shown(t.Name)
		for i, row := range v.rows.rows {
			if v.shown[i] == nil {
				hidden[row["id"]] = true
			}
		}
	}
	return hidden
}

func TestHidingFollowsEveryReferenceAsPassesDid(t *testing.T) {
	same := func(t *testing.T, when string, m *Model) {
		t.Helper()
		want, got := hiddenByPasses(m), hiddenByViews(m)
		if !maps.Equal(want, got) {
			t.Fatalf("%s: following references hid %d rows, passes over every row %d", when, len(got), len(want))
		}
	}
	scale, _, _ := scaleStoreIn(t)
	same(t, "the scale fixture", scale.Model())
	s, queue := sampleWithQueue(t)
	same(t, "the sample", s.Model())
	people, groups := []string{}, []string{}
	for _, p := range s.Model().Table("PERSON").All() {
		people = append(people, p["id"])
	}
	for _, g := range s.Model().Table("GROUP").All() {
		groups = append(groups, g["id"])
	}
	rng := rand.New(rand.NewPCG(5, 6))
	pick := func(ids []string) string { return ids[rng.IntN(len(ids))] }
	for i := range 40 {
		edit := Edit{Set: pick(people), Cells: map[string]any{"consent": pick([]string{"listed", "withheld"})}}
		switch rng.IntN(3) {
		case 1:
			edit = Edit{Set: pick(people), Cells: map[string]any{"hidden": pick([]string{"Yes", ""})}}
		case 2:
			edit = Edit{Set: pick(groups), Cells: map[string]any{"consent": pick([]string{"listed", "withheld"})}}
		}
		apply(t, s, queue, []Edit{edit})
		t.Logf("edit %d: %s %v", i, edit.Set, edit.Cells)
		same(t, "after a random edit", s.Model())
	}
}
