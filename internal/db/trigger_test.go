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
		written, err = Write(context.Background(), s, queue, access.System(importReader), Env{System: importReader, Now: testNow}, b)
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
		{"insert": "GROUP", "as": "grade4", "row": {"kind": "grade", "slug": "grade-4", "title": "Grade 4", "parent": "grp00000000012"}},
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
	if _, ok := s.Model().Table("MEMBER").Find("grp00000000020", student, "member"); !ok {
		t.Fatal("deactivating took the student out of their family")
	}

	if err := write(`{"batch": [
		{"insert": "GROUP", "as": "oak", "row": {"kind": "classroom", "title": "Oak"}},
		{"insert": "PERSON", "row": {"source": "veracross", "vc_name": "Wren Ashdown", "vc_classroom": "@oak"}}]}`); err != nil {
		t.Fatal(err)
	}
	if got := schoolGroupsHeld(s, written[1]); !slices.Equal(got, []string{written[0]}) {
		t.Fatalf("a new student is in %v", got)
	}
}
