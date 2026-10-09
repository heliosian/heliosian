package db

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/store"
)

func TestATagIsMadeAndRunByItsManagers(t *testing.T) {
	s, queue := sampleWithQueue(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, queue, newPictures(s, queue), access.Actor{Email: "rowan.ashdown@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	ids, err := write(parent,
		Edit{Insert: "GROUP", As: "managers", Row: map[string]any{"kind": "group", "name": "Soccer Managers", "status": "open", "added_by": parent}},
		Edit{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": parent, "member": "yes"}},
		Edit{Insert: "GROUP", As: "tag", Row: map[string]any{"kind": "group", "name": "Soccer", "status": "open", "listed": true, "managed_by": "@managers", "added_by": parent}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@tag", "person": student, "member": "yes"}},
	)
	if err != nil {
		t.Fatalf("a parent can't make a tag: %v", err)
	}
	managers, own, tag := ids[0], ids[2], ids[3]
	if _, err := write(staff, Edit{Insert: "MEMBER", Row: map[string]any{"group": tag, "person": staff, "member": "yes"}}); err == nil {
		t.Error("someone who doesn't manage the tag tags themselves")
	}
	if _, err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": managers, "person": staff, "member": "yes"}}); err != nil {
		t.Errorf("the tag's maker can't share it: %v", err)
	}
	if _, err := write(staff, Edit{Set: tag, Cells: map[string]any{"name": "Soccer Team"}}); err != nil {
		t.Errorf("someone the tag is shared with can't rename it: %v", err)
	}
	if _, err := write(guest, Edit{Insert: "MEMBER", Row: map[string]any{"group": tag, "person": guest, "member": "yes"}}); err == nil {
		t.Error("a guest tags themselves")
	}
	if _, err := write(parent, Edit{Set: tag, Cells: map[string]any{"status": "closed"}}); err != nil {
		t.Errorf("a tag's manager can't close it: %v", err)
	}
	if _, err := write(parent, Edit{Delete: own}); err != nil {
		t.Errorf("a tag's manager can't leave it: %v", err)
	}
	if _, err := write(parent, Edit{Set: tag, Cells: map[string]any{"name": "Mine Again"}}); err == nil {
		t.Error("someone who left a tag still renames it")
	}

	for name, edits := range map[string][]Edit{
		"a group with mail":                       {{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "List", "status": "open", "mail": true, "added_by": parent}}},
		"a group everyone sees":                   {{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Open", "status": "open", "visible_to": "grp00000000004", "added_by": parent}}},
		"a group run by managers one isn't among": {{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Theirs", "status": "open", "managed_by": "grp00000000041", "added_by": parent}}},
		"a group made in someone else's name":     {{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Forged", "status": "open", "added_by": staff}}},
		"joining managers one didn't make":        {{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000041", "person": parent, "member": "yes"}}},
		"a made group handed to others' managers": {
			{Insert: "GROUP", As: "g", Row: map[string]any{"kind": "group", "name": "Mine", "status": "open", "added_by": parent}},
			{Set: "@g", Cells: map[string]any{"managed_by": "grp00000000041"}},
		},
	} {
		if _, err := write(parent, edits...); err == nil {
			t.Errorf("a parent makes %s", name)
		}
	}
	if _, err := write(guest, Edit{Insert: "GROUP", Row: map[string]any{"kind": "group", "name": "Guests", "status": "open", "added_by": guest}}); err == nil {
		t.Error("a guest makes a tag")
	}
}

func TestWhoWrites(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Insert("PERSON", store.Row{"id": "per00000000099", "source": "manual", "vc_name_long": "Sam Added", "consent": "listed"})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, ConfigSheet, store.Insert("SETTING", store.Row{"id": "set00000000099", "app": "platform", "key": "Staff Color", "value": "#000000"})); err != nil {
		t.Fatal(err)
	}
	insert := func(table string, row store.Row) Change { return Change{Table: table, New: row} }
	for _, c := range []struct {
		name   string
		viewer string
		change Change
		ok     bool
	}{
		{"a parent sets a child's pronouns", parent, change(t, s, "PERSON", []string{student}, store.Row{"pronouns": "she/her"}), true},
		{"a guest sets a child's pronouns", guest, change(t, s, "PERSON", []string{student}, store.Row{"pronouns": "she/her"}), false},
		{"a parent writes a stranger's facts", parent, change(t, s, "PERSON", []string{staff}, store.Row{"facts": "Likes kelp"}), false},
		{"Who?'s admin writes anyone's facts", staff, change(t, s, "PERSON", []string{student}, store.Row{"facts": "Likes kelp"}), true},
		{"a parent hides a child", parent, change(t, s, "PERSON", []string{student}, store.Row{"hidden": "Yes"}), false},
		{"Who?'s admin hides anyone", staff, change(t, s, "PERSON", []string{student}, store.Row{"hidden": "Yes"}), true},
		{"a parent overrides a child's classroom", parent, change(t, s, "PERSON", []string{student}, store.Row{"classroom_override": "grp00000000010"}), false},
		{"Who?'s admin overrides a classroom", staff, change(t, s, "PERSON", []string{student}, store.Row{"classroom_override": "grp00000000010"}), true},
		{"a parent overrides their family's address", parent, change(t, s, "GROUP", []string{"grp00000000020"}, store.Row{"address_override": "9 Kelp Lane"}), true},
		{"a parent captions another family's photo", parent, change(t, s, "GROUP", []string{"grp00000000503"}, store.Row{"description": "Us"}), false},
		{"Who?'s admin captions any family's photo", staff, change(t, s, "GROUP", []string{"grp00000000020"}, store.Row{"description": "Us"}), true},
		{"a parent colors a classroom", parent, change(t, s, "GROUP", []string{"grp00000000010"}, store.Row{"color": "#000000"}), false},
		{"Who?'s admin colors a classroom", staff, change(t, s, "GROUP", []string{"grp00000000010"}, store.Row{"color": "#000000"}), true},
		{"a parent adds a child's photo", parent, insert("PHOTO", store.Row{"person": student, "original": "photos/a.png"}), true},
		{"a parent adds a stranger's photo", parent, insert("PHOTO", store.Row{"person": staff, "original": "photos/a.png"}), false},
		{"a parent adds their family's photo", parent, insert("PHOTO", store.Row{"group": "grp00000000020", "original": "photos/a.png"}), true},
		{"a parent adds a classroom's tile", parent, insert("PHOTO", store.Row{"group": "grp00000000010", "original": "photos/a.png"}), false},
		{"Who?'s admin adds a classroom's tile", staff, insert("PHOTO", store.Row{"group": "grp00000000010", "original": "photos/a.png"}), true},
		{"a parent adds a greeting", parent, insert("GREETING", store.Row{"name": "Hi", "added_by": parent}), true},
		{"a parent adds a greeting as someone else", parent, insert("GREETING", store.Row{"name": "Hi", "added_by": staff}), false},
		{"a guest adds a greeting", guest, insert("GREETING", store.Row{"name": "Hi", "added_by": guest}), false},
		{"Who?'s admin adds someone by hand", staff, insert("PERSON", store.Row{"source": "manual"}), true},
		{"Who?'s admin adds a Veracross person", staff, insert("PERSON", store.Row{"source": "veracross"}), false},
		{"a parent adds someone by hand", parent, insert("PERSON", store.Row{"source": "manual"}), false},
		{"Who?'s admin makes someone added by hand staff", staff, insert("MEMBER", store.Row{"group": "grp00000000003", "person": "per00000000099", "member": "yes"}), true},
		{"Who?'s admin puts someone added by hand in everyone", staff, insert("MEMBER", store.Row{"group": "grp00000000004", "person": "per00000000099", "member": "yes"}), false},
		{"Who?'s admin makes a Veracross person staff", staff, insert("MEMBER", store.Row{"group": "grp00000000003", "person": student, "member": "yes"}), false},
		{"Who?'s admin deactivates someone added by hand", staff, change(t, s, "PERSON", []string{"per00000000099"}, store.Row{"deactivated": "2026-10-01 12:00"}), true},
		{"Who?'s admin deactivates a Veracross person", staff, change(t, s, "PERSON", []string{student}, store.Row{"deactivated": "2026-10-01 12:00"}), false},
		{"Who?'s admin makes a room parent", staff, insert("MEMBER", store.Row{"group": "grp00000000502", "person": staff, "member": "yes"}), true},
		{"a parent makes a room parent", parent, insert("MEMBER", store.Row{"group": "grp00000000502", "person": staff, "member": "yes"}), false},
		{"Who?'s admin sets the staff color", staff, change(t, s, "SETTING", []string{"set00000000099"}, store.Row{"value": "#111111"}), true},
		{"a parent sets the staff color", parent, change(t, s, "SETTING", []string{"set00000000099"}, store.Row{"value": "#111111"}), false},
		{"Who?'s admin sets another app's setting", staff, change(t, s, "SETTING", []string{"set00000000001"}, store.Row{"value": "Hello"}), false},
	} {
		err := s.Model().Authorize(Env{Viewer: c.viewer, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func postRecording(t *testing.T, s *Store, queue *store.Queue, pics *Pictures, as string, fields map[string]string, mimeType string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	form := multipart.NewWriter(body)
	for k, v := range fields {
		if err := form.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	part, err := form.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="recording"; filename="name"`}, "Content-Type": {mimeType}})
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	form.Close()
	mux := http.NewServeMux()
	Register(mux, s, queue, pics, []byte(testImportKey), func() time.Time { return testNow })
	r := httptest.NewRequest(http.MethodPost, "/api/do/pronunciation", body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	auth.Fixed(as, mux).ServeHTTP(rec, r)
	return rec
}

func TestAPronunciationIsRecordedForWhomTheViewerMayEdit(t *testing.T) {
	s, queue := sampleWithQueue(t)
	pics := newPictures(s, queue)
	sound := []byte("a webm recording")
	name := "pronunciation/" + blob.Name(sound, "webm")
	for _, c := range []struct {
		table, field, id string
	}{
		{"PERSON", "person", student},
		{"GROUP", "group", "grp00000000020"},
	} {
		rec := postRecording(t, s, queue, pics, "rowan.ashdown@example.org", map[string]string{c.field: c.id}, "audio/webm;codecs=opus", sound)
		if rec.Code != http.StatusOK {
			t.Fatalf("a parent records their family's %s: %d %s", c.field, rec.Code, rec.Body.String())
		}
		var out stored
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		if row, _ := s.Model().Table(c.table).Get(c.id); row["pronunciation"] != name || out.Result[0] != c.id {
			t.Fatalf("the %s's pronunciation reads %q, answered %+v", c.field, row["pronunciation"], out)
		}
	}
	if found, err := pics.bucket.Exists(context.Background(), name); err != nil || !found {
		t.Fatalf("the recording is not in the bucket: %v", err)
	}

	other := []byte("someone else's name")
	if rec := postRecording(t, s, queue, pics, "rowan.ashdown@example.org", map[string]string{"person": staff}, "audio/webm", other); rec.Code != http.StatusForbidden {
		t.Fatalf("a parent records a stranger's name: %d %s", rec.Code, rec.Body.String())
	}
	if found, _ := pics.bucket.Exists(context.Background(), "pronunciation/"+blob.Name(other, "webm")); found {
		t.Fatal("a refused recording was stored")
	}
	if rec := postRecording(t, s, queue, pics, "rowan.ashdown@example.org", map[string]string{"person": student}, "application/octet-stream", other); rec.Code != http.StatusBadRequest {
		t.Fatalf("a recording of no audio type: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postRecording(t, s, queue, pics, "rowan.ashdown@example.org", map[string]string{"person": student, "group": "grp00000000020"}, "audio/webm", other); rec.Code != http.StatusBadRequest {
		t.Fatalf("a recording of a person and a family: %d %s", rec.Code, rec.Body.String())
	}
}
