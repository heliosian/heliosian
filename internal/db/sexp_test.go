package db

import (
	"strings"
	"testing"
)

func (s *sexp) equal(o *sexp) bool {
	if s.isList != o.isList {
		return false
	}
	if !s.isList {
		return s.kind == o.kind && s.text == o.text
	}
	if len(s.list) != len(o.list) {
		return false
	}
	for i := range s.list {
		if !s.list[i].equal(o.list[i]) {
			return false
		}
	}
	return true
}

func TestDeepQueryFormattingDoesLinearWork(t *testing.T) {
	const depth = 500
	source := `(from PERSON (where ` + strings.Repeat(`(not `, depth) + `true` + strings.Repeat(`)`, depth) + `) (limit 1))`
	q := mustParse(t, source)
	allocations := testing.AllocsPerRun(1, func() { _ = q.String() })
	if allocations > 20*depth {
		t.Fatalf("formatting a %d-byte query made %.0f allocations", len(source), allocations)
	}
	if got := q.String(); !q.tree.equal(mustParse(t, got).tree) {
		t.Fatal("formatting changed the query")
	}
}

func TestDeepQueryFormattingStopsIndentingAtTheLineWidth(t *testing.T) {
	const depth = 100
	source := `(from PERSON (where ` + strings.Repeat(`(and true `, depth) + `true` + strings.Repeat(`)`, depth) + `) (limit 1))`
	q := mustParse(t, source)
	got := q.String()
	if len(got) > 4*len(source) {
		t.Fatalf("formatting expanded %d bytes into %d", len(source), len(got))
	}
	if !q.tree.equal(mustParse(t, got).tree) {
		t.Fatal("formatting changed the query")
	}
}

func TestQueryFormattingKeepsItsLayout(t *testing.T) {
	for source, want := range map[string]string{
		`(from PERSON)`: `(from PERSON)`,
		`(from PERSON (where (= name_show "a quoted \"name\"")) (order name_sort asc) (columns id name_show))`:            "(from PERSON\n  (where (= name_show \"a quoted \\\"name\\\"\"))\n  (order name_sort asc)\n  (columns id name_show))",
		`(from MEMBER (where (= group "grp00000000040") (= member "yes")) (order person.name_sort asc) (include person))`: "(from MEMBER\n  (where (= group \"grp00000000040\") (= member \"yes\"))\n  (order person.name_sort asc)\n  (include person))",
	} {
		q := mustParse(t, source)
		got := q.String()
		if got != want {
			t.Fatalf("formatted %s as %q, want %q", source, got, want)
		}
		if !q.tree.equal(mustParse(t, got).tree) {
			t.Fatalf("formatting changed %s into %s", source, got)
		}
	}
}

func TestNestedScansCompileWithBoundedAllocations(t *testing.T) {
	const depth = 1000
	source := `(from PERSON (where ` + strings.Repeat(`(exists PERSON `, depth) + `true` + strings.Repeat(`)`, depth) + `) (limit 1))`
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			if _, err := Parse(source); err != nil {
				b.Fatal(err)
			}
		}
	})
	if result.AllocedBytesPerOp() > 4096*depth {
		t.Fatalf("compiling a %d-byte nested scan allocated %d bytes", len(source), result.AllocedBytesPerOp())
	}
}

func TestQueryFormattingAtTheLineWidth(t *testing.T) {
	source := `(and true true true true true)`
	tree, err := readSexp(source)
	if err != nil {
		t.Fatal(err)
	}
	for column, want := range map[int]string{
		renderWidth - 1: "(and true\n" + strings.Repeat(" ", renderWidth) + "true\n" + strings.Repeat(" ", renderWidth) + "true\n" + strings.Repeat(" ", renderWidth) + "true\n" + strings.Repeat(" ", renderWidth) + "true)",
		renderWidth:     source,
		renderWidth + 1: source,
	} {
		if got := tree.render(column); got != want {
			t.Errorf("at column %d got %q, want %q", column, got, want)
		}
	}
}
