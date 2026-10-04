package db

import (
	"context"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func schoolGroupsHeld(s *Store, person string) []string {
	m := s.Model()
	out := []string{}
	for _, row := range m.Table("MEMBER").Referencing("person", person) {
		g, _ := m.Table("GROUP").Get(row["group"])
		if slices.Contains(schoolKinds, g["kind"]) {
			out = append(out, row["group"])
		}
	}
	slices.Sort(out)
	return out
}

func TestSchoolGroupsFollowThePerson(t *testing.T) {
	s, queue := sampleWithQueue(t)
	var written []string
	write := func(raw string) error {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		written, err = Write(context.Background(), s, queue, newPictures(s, queue), access.System(importReader), Env{System: importReader, Now: testNow}, b)
		return err
	}
	if got := schoolGroupsHeld(s, student); !slices.Equal(got, []string{"grp00000000010", "grp00000000011"}) {
		t.Fatalf("the sample's student is in %v", got)
	}

	err := write(`{"batch": [{"set": "per00000000001", "cells": {"vc_grade": "4"}}]}`)
	if err == nil || !strings.Contains(err.Error(), "grade-4") {
		t.Fatalf("a grade with no group: %v", err)
	}

	if err := write(`{"batch": [
		{"insert": "GROUP", "as": "grade4", "row": {"kind": "grade", "slug": "grade-4", "name": "Grade 4", "parent": "grp00000000012"}},
		{"set": "per00000000001", "cells": {"vc_grade": "4"}}]}`); err != nil {
		t.Fatal(err)
	}
	if got, want := schoolGroupsHeld(s, student), []string{"grp00000000010", written[0]}; !slices.Equal(got, want) {
		t.Fatalf("moved up a grade, the student is in %v", got)
	}

	if err := write(`{"batch": [{"set": "per00000000001", "cells": {"deactivated": "2026-09-30 12:00"}}]}`); err != nil {
		t.Fatal(err)
	}
	if got := schoolGroupsHeld(s, student); len(got) != 0 {
		t.Fatalf("deactivated, the student is in %v", got)
	}
	if _, ok := s.Model().Table("MEMBER").Find("grp00000000020", student); !ok {
		t.Fatal("deactivating took the student out of their family")
	}

	if err := write(`{"batch": [
		{"insert": "GROUP", "as": "oak", "row": {"kind": "classroom", "name": "Oak"}},
		{"insert": "PERSON", "row": {"source": "veracross", "vc_name": "Wren Ashdown", "vc_classroom": "@oak"}}]}`); err != nil {
		t.Fatal(err)
	}
	if got := schoolGroupsHeld(s, written[1]); !slices.Equal(got, []string{written[0]}) {
		t.Fatalf("a new student is in %v", got)
	}
}

func TestFamilyNameFollowsItsMembers(t *testing.T) {
	s, queue := sampleWithQueue(t)
	write := func(raw string) []string {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		written, err := Write(context.Background(), s, queue, newPictures(s, queue), access.System(importReader), Env{System: importReader, Now: testNow}, b)
		if err != nil {
			t.Fatal(err)
		}
		return written
	}
	name := func(want, when string) {
		t.Helper()
		family, _ := s.Model().Table("GROUP").Get("grp00000000020")
		if family["name"] != want {
			t.Fatalf("%s, the family is %q, not %q", when, family["name"], want)
		}
	}
	name("Ashdown Family", "as the sample has it")

	write(`{"batch": [{"set": "per00000000001", "cells": {"name_long_override": "Juni Chang-Ashdown"}}]}`)
	name("Chang-Ashdown Family", "with a hyphenated name taking in the other")

	kai := write(`{"batch": [
		{"insert": "PERSON", "as": "kai", "row": {"source": "veracross", "vc_name_long": "Kai Lindqvist", "consent": "listed"}},
		{"insert": "MEMBER", "row": {"group": "grp00000000020", "person": "@kai", "member": "yes"}}]}`)[0]
	name("Chang-Ashdown & Lindqvist Family", "once Kai joins")

	write(`{"batch": [{"set": "` + kai + `", "cells": {"consent": "withheld"}}]}`)
	name("Chang-Ashdown Family", "with Kai withheld")
}
