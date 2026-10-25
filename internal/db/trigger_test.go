package db

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
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
	s := sample(t)
	var written []string
	write := func(raw string) error {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		written, err = Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b)
		return err
	}
	if got := schoolGroupsHeld(s, student); !slices.Equal(got, []string{"grp00000000010", "grp00000000011", "grp00000000501"}) {
		t.Fatalf("the sample's student is in %v", got)
	}

	if err := write(`{"batch": [{"set": "grp00000020015", "cells": {"slug": ""}}]}`); err != nil {
		t.Fatal(err)
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
	want := []string{"grp00000000010", "grp00000000501", written[0]}
	slices.Sort(want)
	if got := schoolGroupsHeld(s, student); !slices.Equal(got, want) {
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
	s := sample(t)
	write := func(raw string) []string {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		written, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b)
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

func TestFamilyNameTriggersSkipUnrelatedEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		fire func(*Model, Change) ([]Edit, error)
		old  store.Row
		new  store.Row
	}{
		{"person phone", personFamilyNames, store.Row{"id": student, "phone": "123"}, store.Row{"id": student, "phone": "456"}},
		{"person unchanged", personFamilyNames, store.Row{"id": student, "name_long": "Juni Ashdown"}, store.Row{"id": student, "name_long": "Juni Ashdown"}},
		{"member unrelated", memberFamilyName, store.Row{"group": "grp00000000020", "person": student, "member": "yes", "why": "old"}, store.Row{"group": "grp00000000020", "person": student, "member": "yes", "why": "new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edits, err := tc.fire(nil, Change{Old: tc.old, New: tc.new})
			if err != nil || len(edits) != 0 {
				t.Fatalf("unrelated edit produced %v, %v", edits, err)
			}
		})
	}
}

func TestFamilyNameTriggersKeepRelevantChanges(t *testing.T) {
	s := sample(t)
	b, err := ParseBatch([]byte(`{"batch": [{"set": "grp00000000020", "cells": {"name": "Stale Family"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Model().Table("PERSON").Get(student)
	member, _ := s.Model().Table("MEMBER").Find("grp00000000020", student)
	for _, tc := range []struct {
		name    string
		fire    func(*Model, Change) ([]Edit, error)
		row     store.Row
		columns []string
	}{
		{"person", personFamilyNames, p, []string{"vc_name_long", "name_long_override", "hidden", "deactivated", "consent", "source"}},
		{"member", memberFamilyName, member, []string{"group", "person", "member"}},
	} {
		check := func(t *testing.T, c Change) {
			t.Helper()
			edits, err := tc.fire(s.Model(), c)
			if err != nil || len(edits) != 1 || edits[0].Set != "grp00000000020" || edits[0].Cells["name"] != "Ashdown Family" {
				t.Fatalf("relevant change produced %v, %v", edits, err)
			}
		}
		for _, column := range tc.columns {
			t.Run(tc.name+"/"+column, func(t *testing.T) {
				old := maps.Clone(tc.row)
				old[column] = "different"
				check(t, Change{Old: old, New: tc.row})
			})
		}
		t.Run(tc.name+"/insert", func(t *testing.T) {
			check(t, Change{New: tc.row})
		})
		t.Run(tc.name+"/delete", func(t *testing.T) {
			check(t, Change{Old: tc.row})
		})
	}
}

func TestACommittedTriggerSeesWhatAStagedTriggerWrote(t *testing.T) {
	s := sample(t)
	b, err := ParseBatch([]byte(`{"batch": [{"set": "per00000000001", "cells": {"name_long_override": "Juni Chang-Ashdown"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b); err != nil {
		t.Fatal(err)
	}
	seen := []string{}
	for _, d := range s.Model().services.inbox(consentInbox).take() {
		seen = append(seen, d.change.Table+" "+d.change.New["id"]+" "+d.change.New["name"])
	}
	if want := []string{"PERSON per00000000001 ", "GROUP grp00000000020 Chang-Ashdown Family"}; !slices.Equal(seen, want) {
		t.Fatalf("the consent inbox holds %q, not %q", seen, want)
	}
}

func TestASentPostIsFiledAsMail(t *testing.T) {
	s := sample(t)
	write := func(raw string) []string {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		written, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b)
		if err != nil {
			t.Fatal(err)
		}
		return written
	}
	filed := func(message string) []map[string]string {
		t.Helper()
		out := []map[string]string{}
		for _, d := range s.Model().Table("DOCUMENT").Referencing("message", message) {
			out = append(out, d)
		}
		return out
	}
	content := write(`{"batch": [{"insert": "CONTENT", "row": {"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}}]}`)[0]

	post := write(`{"batch": [{"insert": "MESSAGE", "row": {"direction": "in", "kind": "post", "group": "grp00000000030", "from_person": "per00000000002", "subject": "Tide pools", "created": "2026-09-18 19:51", "content": "` + content + `", "state": "received"}}]}`)[0]
	if n := len(filed(post)); n != 0 {
		t.Fatalf("a post not yet sent on was filed %d times", n)
	}
	write(`{"batch": [{"set": "` + post + `", "cells": {"state": "sent"}}]}`)
	docs := filed(post)
	if len(docs) != 1 {
		t.Fatalf("the post was filed %d times", len(docs))
	}
	d := docs[0]
	if d["kind"] != "mail" || d["content"] != content || d["name"] != "Tide pools" || d["published"] != "2026-09-18 19:51" || d["author"] != "per00000000002" {
		t.Fatalf("the post's document reads %v", d)
	}
	if _, ok := s.Model().Table("DOCUMENT_GROUP").Find(d["id"], "grp00000000030", "sent_to"); !ok {
		t.Fatal("the post's document was not sent to its list")
	}

	dropped := write(`{"batch": [{"insert": "MESSAGE", "row": {"direction": "in", "kind": "post", "group": "grp00000000030", "subject": "Accepted: Tide pools", "created": "2026-09-18 20:00", "content": "` + content + `", "state": "dropped", "detail": "auto-submitted mail"}}]}`)[0]
	if n := len(filed(dropped)); n != 0 {
		t.Fatalf("a dropped post was filed %d times", n)
	}

	bare := write(`{"batch": [{"insert": "MESSAGE", "row": {"direction": "in", "kind": "post", "group": "grp00000000030", "subject": "Raw message to come", "created": "2026-09-19 08:00", "state": "sent"}}]}`)[0]
	if n := len(filed(bare)); n != 0 {
		t.Fatalf("a post with no raw message was filed %d times", n)
	}
	before, _ := s.Model().Table("MESSAGE").Get(bare)
	after := maps.Clone(before)
	after["content"] = content
	if edits, err := postDocument(s.Model(), Change{Table: "MESSAGE", Old: before, New: after}); err != nil || len(edits) != 1 || edits[0].Insert != "DOCUMENT" {
		t.Fatalf("a post whose raw message came later is filed by %v, %v", edits, err)
	}
	if edits, err := postDocument(s.Model(), Change{Table: "MESSAGE", Old: after, New: after}); err != nil || len(edits) != 0 {
		t.Fatalf("a post already holding its raw message is filed again by %v, %v", edits, err)
	}
	held, _ := s.Model().Table("MESSAGE").Get(post)
	if edits, err := postDocument(s.Model(), Change{Table: "MESSAGE", New: held}); err != nil || len(edits) != 0 {
		t.Fatalf("a post already filed is filed again by %v, %v", edits, err)
	}

	out := write(`{"batch": [{"insert": "MESSAGE", "row": {"direction": "out", "kind": "post", "group": "grp00000000030", "parent": "` + post + `", "subject": "Tide pools", "created": "2026-09-18 19:51", "content": "` + content + `"}}]}`)[0]
	if n := len(filed(out)); n != 0 {
		t.Fatalf("an outbound copy was filed %d times", n)
	}
}

func TestAnswerGroupsAreSharedAsTheirEvent(t *testing.T) {
	s := sample(t)
	seen := func() int {
		t.Helper()
		return len(as(t, s, student, `(from GROUP (where (in id "grp00000000040" "grp00000000042" "grp00000000043")))`))
	}
	if n := seen(); n != 3 {
		t.Fatalf("before the picnic is made invite-only, the student sees %d of it and its answer groups, not 3", n)
	}
	apply(t, s, []Edit{{Set: "grp00000000040", Cells: map[string]any{"listed": false, "visible_to": "grp00000000040", "members_visible_to": ""}}})
	for _, id := range []string{"grp00000000042", "grp00000000043"} {
		if g, _ := s.Model().Table("GROUP").Get(id); g["visible_to"] != "grp00000000040" || g["members_visible_to"] != "" {
			t.Errorf("once the picnic is invite-only, %s is shown to %q and its members to %q", id, g["visible_to"], g["members_visible_to"])
		}
	}
	if n := seen(); n != 0 {
		t.Fatalf("once the picnic is invite-only, the student still sees %d of it and its answer groups", n)
	}
}

func TestServingGroupsAreRenamedWithTheirGroup(t *testing.T) {
	s := sample(t)
	write := func(raw string) {
		t.Helper()
		b, err := ParseBatch([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, b); err != nil {
			t.Fatal(err)
		}
	}
	name := func(want, when string) {
		t.Helper()
		managers, _ := s.Model().Table("GROUP").Get("grp00000000041")
		if managers["name"] != want {
			t.Fatalf("%s, the picnic's managers are %q, not %q", when, managers["name"], want)
		}
	}
	write(`{"batch": [{"set": "grp00000000040", "cells": {"name": "Autumn Picnic"}}]}`)
	name("Autumn Picnic Managers", "once the picnic is renamed")
	for id, want := range map[string]string{"grp00000000042": "Autumn Picnic Going", "grp00000000043": "Autumn Picnic Not Going"} {
		if g, _ := s.Model().Table("GROUP").Get(id); g["name"] != want {
			t.Errorf("once the picnic is renamed, %s is %q, not %q", id, g["name"], want)
		}
	}

	write(`{"batch": [{"set": "grp00000000041", "cells": {"name": "Picnic Crew"}}]}`)
	write(`{"batch": [{"set": "grp00000000040", "cells": {"name": "Harvest Picnic"}}]}`)
	name("Picnic Crew", "with a name chosen by hand")
}
