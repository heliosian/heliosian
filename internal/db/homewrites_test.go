package db

import (
	"context"
	"testing"

	"heliosian/internal/access"
)

func TestHeliosianAdminsKeepTheFrontPage(t *testing.T) {
	const (
		homeAdmins = "grp00000000015"
		everyone   = "grp00000000004"
		school     = "wdg00000000008"
		schoolLink = "grp00000000550"
		whoApp     = "app00000000001"
		layout     = "set00000000015"
	)
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": homeAdmins, "person": student, "member": "yes"}}})
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, access.Actor{Email: "juni.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	section := []Edit{
		{Insert: "GROUP", As: "section", Row: map[string]any{"kind": "group", "name": "Clubs", "status": "open", "listed": false, "posting": "none", "replying": "none", "visible_to": everyone, "added_by": student}},
		{Insert: "WIDGET", Row: map[string]any{"key": "links", "group": "@section", "name": "Clubs", "icon": "icon:star", "style": "list", "descriptions": true, "sidebar": false, "visible_to": everyone, "order": "z"}},
	}
	if _, err := write(parent, section...); err == nil {
		t.Fatal("a parent added a section to the front page")
	}
	made, err := write(student, section...)
	if err != nil {
		t.Fatalf("a Heliosian admin adds a section: %v", err)
	}
	sectionGroup, widget := made[0], made[1]
	link, err := write(student, Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "parent": sectionGroup, "name": "Chess", "url": "https://example.org/chess", "status": "open", "listed": false, "posting": "none", "replying": "none", "visible_to": everyone, "order": "i"}})
	if err != nil {
		t.Fatalf("a Heliosian admin adds a link: %v", err)
	}
	pictures := []Edit{}
	for _, group := range []string{sectionGroup, link[0]} {
		picture := Edit{Insert: "PHOTO", Row: map[string]any{"group": group, "original": "photos/mark.png", "order": "m"}}
		if _, err := write(parent, picture); err == nil {
			t.Errorf("a parent gave %s a picture", group)
		}
		made, err := write(student, picture)
		if err != nil {
			t.Fatalf("a Heliosian admin gives %s a picture: %v", group, err)
		}
		pictures = append(pictures, Edit{Delete: made[0]})
	}
	if _, err := write(student, Edit{Insert: "PHOTO", Row: map[string]any{"group": "grp00000000030", "original": "photos/mark.png", "order": "m"}}); err == nil {
		t.Error("a Heliosian admin gave a parents list a picture")
	}
	audience := []Edit{
		{Insert: "GROUP", As: "audience", Row: map[string]any{"kind": "group", "name": "Chess Viewers", "status": "open", "listed": false, "posting": "none", "replying": "none", "added_by": student}},
		{Set: link[0], Cells: map[string]any{"visible_to": "@audience"}},
		{Insert: "RULE", Row: map[string]any{"group": "@audience", "order": "i", "target": "grp00000000002"}},
	}
	if _, err := write(parent, audience...); err == nil {
		t.Fatal("a parent narrowed a link's audience")
	}
	shown, err := write(student, audience...)
	if err != nil {
		t.Fatalf("a Heliosian admin narrows a link to the parents: %v", err)
	}
	viewers := shown[0]
	if got := len(as(t, s, parent, `(from GROUP (where (= id "`+link[0]+`")))`)); got != 1 {
		t.Errorf("a parent sees %d of the parents' link", got)
	}
	for name, edits := range map[string][]Edit{
		"rename a link":           {{Set: schoolLink, Cells: map[string]any{"name": "The Directory", "description": "Everyone at school"}}},
		"move a link":             {{Set: schoolLink, Cells: map[string]any{"parent": sectionGroup, "order": "j"}}},
		"reorder a widget":        {{Set: school, Cells: map[string]any{"order": "zz", "sidebar": true, "style": "list"}}},
		"rename an app":           {{Set: whoApp, Cells: map[string]any{"name": "Who", "subtitle": "The directory", "order": "a"}}},
		"lay the rows out":        {{Set: layout, Cells: map[string]any{"value": "1/2 1/2; 1"}}},
		"show a section to a few": {{Set: widget, Cells: map[string]any{"visible_to": viewers}}},
	} {
		if _, err := write(parent, edits...); err == nil {
			t.Errorf("%s: a parent may", name)
		}
		if _, err := write(student, edits...); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, edit := range map[string]Edit{
		"a rule of the super admins":      {Insert: "RULE", Row: map[string]any{"group": "grp00000000005", "order": "z", "target": "grp00000000002"}},
		"a rule of everyone":              {Insert: "RULE", Row: map[string]any{"group": everyone, "order": "z", "target": "grp00000000002"}},
		"someone into the Who? admins":    {Insert: "MEMBER", Row: map[string]any{"group": "grp00000000006", "person": parent, "member": "yes"}},
		"another app's setting":           {Insert: "SETTING", Row: map[string]any{"app": "team", "key": "Intro", "value": "Hello"}},
		"a group that is no home section": {Set: "grp00000000030", Cells: map[string]any{"name": "Renamed"}},
	} {
		if _, err := write(student, edit); err == nil {
			t.Errorf("a Heliosian admin wrote %s", name)
		}
	}
	rules := s.Model().Table("RULE").Referencing("group", viewers)
	cleared := []Edit{{Delete: rules[0]["id"]}, {Set: link[0], Cells: map[string]any{"visible_to": everyone}}, {Set: widget, Cells: map[string]any{"visible_to": everyone}}, {Delete: viewers}}
	if _, err := write(student, cleared...); err != nil {
		t.Fatalf("a Heliosian admin opens the link to everyone again and drops its viewers group: %v", err)
	}
	if _, err := write(student, append(pictures, Edit{Delete: link[0]}, Edit{Set: schoolLink, Cells: map[string]any{"parent": "grp00000000532"}}, Edit{Delete: widget}, Edit{Delete: sectionGroup})...); err != nil {
		t.Fatalf("a Heliosian admin takes the section away: %v", err)
	}
	if _, err := write(student, Edit{Delete: "grp00000000030"}); err == nil {
		t.Error("a Heliosian admin deleted a parents list")
	}
}
