package db

import (
	"maps"
	"testing"

	"heliosian/internal/store"
)

func TestOriginalReferenceMigration(t *testing.T) {
	for _, table := range []string{"PHOTO", "PERSON", "GROUP"} {
		for _, change := range []string{"same bytes", "different bytes", "crop"} {
			if table != "PHOTO" && change == "crop" {
				continue
			}
			t.Run(table+"/"+change, func(t *testing.T) {
				s, queue := sampleWithQueue(t)
				column, output, prefix := "original", "thumbnail", "photos/"
				row := map[string]any{"person": staff, "order": "m", "original": "photos/old.jpg", "reencode": "photos/reencoded.jpg", "thumbnail": "photos/thumb.jpg", "ready": "Yes"}
				id := ""
				if table == "PHOTO" {
					id = apply(t, s, queue, []Edit{{Insert: table, Row: row}})[0]
				} else {
					column, output, prefix = "pronunciation_original", "pronunciation", "pronunciation/"
					id = parent
					if table == "GROUP" {
						id = "grp00000000020"
					}
					apply(t, s, queue, []Edit{{Set: id, Cells: map[string]any{column: prefix + "old.m4a"}}})
					apply(t, s, queue, []Edit{{Set: id, Cells: map[string]any{output: prefix + "made.m4a"}}})
				}
				hash := "old"
				if change == "different bytes" {
					hash = "new"
				}
				content := apply(t, s, queue, []Edit{
					{Insert: "CONTENT", Row: map[string]any{"hash": hash, "blob": "content/" + hash, "mime": "application/octet-stream", "size": "1"}},
					{Insert: "CONTENT", Row: map[string]any{"hash": "output", "blob": "content/output", "mime": "application/octet-stream", "size": "1"}},
				})
				in := s.Model().services.inbox(picturesInbox)
				in.take()
				cells := map[string]any{column: content[0], output: content[1]}
				if table == "PHOTO" {
					cells["reencode"] = content[1]
				}
				if change == "crop" {
					cells["crop_left"], cells["crop_top"], cells["crop_width"], cells["crop_height"] = "0", "0", "2", "2"
				}
				apply(t, s, queue, []Edit{{Set: id, Cells: cells}})
				got, _ := s.Model().Table(table).Get(id)
				queued := false
				for _, d := range in.take() {
					if d.change.New["id"] == id && (remake(d.change) || rerecord(d.change)) {
						queued = true
					}
				}
				wantQueue := false
				if queued != wantQueue {
					t.Fatalf("queued = %v, want %v; row = %v", queued, wantQueue, got)
				}
				if table == "PHOTO" {
					ready := "Yes"
					if wantQueue {
						ready = ""
					}
					if got["ready"] != ready || got["reencode"] != content[1] || got["thumbnail"] != content[1] {
						t.Fatalf("photo after migration: %v", got)
					}
				} else {
					want := content[1]
					if wantQueue {
						want = ""
					}
					if got[output] != want {
						t.Fatalf("recording after migration: %v", got)
					}
				}
			})
		}
	}
}

func TestPictureWorkStillFollowsOtherChanges(t *testing.T) {
	m := sample(t).Model()
	photo := store.Row{"id": "pho00000000099", "original": "photos/old.jpg"}
	recording := store.Row{"id": parent, "pronunciation_original": "pronunciation/old.m4a"}
	for _, c := range []Change{
		{Table: "PHOTO", New: photo},
		{Table: "PERSON", New: recording},
		{Table: "GROUP", New: recording},
	} {
		if !remake(c) && !rerecord(c) {
			t.Fatalf("new row was not queued: %v", c)
		}
		c.Old, c.New = c.New, nil
		if remake(c) || rerecord(c) {
			t.Fatalf("deleted row was queued: %v", c)
		}
	}
	for _, column := range cropColumns {
		next := maps.Clone(photo)
		next[column] = "1"
		if !remake(Change{Table: "PHOTO", Old: photo, New: next}) {
			t.Fatalf("%s did not queue a remake", column)
		}
	}
	next := maps.Clone(recording)
	next["pronunciation_original"] = ""
	c := Change{Table: "PERSON", Old: recording, New: next}
	if rerecord(c) {
		t.Fatal("removing an original queued a recording")
	}
	next["pronunciation"] = "pronunciation/made.m4a"
	if edits, _ := recordingNotMade(m, c); len(edits) != 1 {
		t.Fatal("removing an original did not clear its output")
	}
}

func TestOldStyleOriginalChangesStillRequestWork(t *testing.T) {
	for _, table := range []string{"PHOTO", "PERSON", "GROUP"} {
		column, original := "original", "photos/new.jpg"
		trigger := photoNotReady
		if table != "PHOTO" {
			column, original = "pronunciation_original", "pronunciation/new.m4a"
			trigger = recordingNotMade
		}
		old := store.Row{"id": parent, column: "old", "ready": "Yes", "pronunciation": "pronunciation/made.m4a"}
		next := maps.Clone(old)
		next[column] = original
		c := Change{Table: table, Old: old, New: next}
		if !remake(c) && !rerecord(c) {
			t.Fatalf("%s original change did not request work", table)
		}
		if edits, _ := trigger(nil, c); len(edits) != 1 {
			t.Fatalf("%s original change did not clear output: %v", table, edits)
		}
	}
}

func TestPictureWorkerLeavesContentOriginalsAlone(t *testing.T) {
	p := &Pictures{}
	if err := p.makePhoto("photo", store.Row{"original": "cnt00000000001"}); err != nil {
		t.Fatal(err)
	}
	if err := p.makeRecording("PERSON", parent, store.Row{"pronunciation_original": "cnt00000000001"}); err != nil {
		t.Fatal(err)
	}
}
