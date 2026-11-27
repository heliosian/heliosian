package db

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
)

func TestAnOutsideGuestWritesTheHostsAndHearsBack(t *testing.T) {
	h := newInviteHarness(t)
	add := map[string]any{"group": h.event, "people": []map[string]string{{"name": "Coach Pat", "email": "coach@example.net"}}}
	if rec := h.do(maya, http.MethodPost, "/api/do/add-guests", add); rec.Code != http.StatusOK {
		t.Fatalf("the host adds someone from outside: %d %s", rec.Code, rec.Body)
	}
	h.setup([]Edit{{Set: h.event, Cells: map[string]any{"auto_invite": true}}})
	h.mailTo("coach@example.net", "You're invited")
	coach := h.s.Model().personAt("coach@example.net")
	path := "/open/ext/" + h.copyOf(coach)["token"]
	if rec := h.do("", http.MethodPost, path+"/comment", map[string]string{"text": "Can I bring the team banner?"}); rec.Code != http.StatusNoContent {
		t.Fatalf("an outside guest writes the hosts: %d %s", rec.Code, rec.Body)
	}
	host := h.s.Model().addressOf(staff)
	note := h.mailTo(host, "A note from Coach Pat")
	if !strings.Contains(note.Text, "team banner") {
		t.Errorf("the hosts' notice leaves out the note: %s", note.Text)
	}
	var asked string
	for _, c := range h.s.Model().Table("COMMENT").Referencing("group", h.event) {
		asked = c["id"]
	}
	if _, err := Write(context.Background(), h.s, access.Actor{Email: host}, Env{Viewer: staff, Now: testNow}, Batch{Batch: []Edit{{Insert: "COMMENT", Row: map[string]any{
		"group": h.event, "person": staff, "reply_to": asked, "text": "Please do!", "added": stamp(time.Now().Add(time.Minute)),
	}}}}); err != nil {
		t.Fatalf("the host replies: %v", err)
	}
	reply := h.mailTo("coach@example.net", "replied")
	if !strings.Contains(reply.Text, "Please do!") || !strings.Contains(reply.Text, "/ext/") {
		t.Errorf("the guest's reply notice: %s", reply.Text)
	}
	var view extView
	json.Unmarshal(h.do("", http.MethodGet, path, nil).Body.Bytes(), &view)
	if len(view.Notes) != 2 || view.Notes[0].Host || !view.Notes[1].Host || view.Notes[1].ReplyTo != asked {
		t.Errorf("the outside page's notes: %+v", view.Notes)
	}
}

func TestAnInvitationsPagesAreTheHostsToWriteAndTheGuestsToRead(t *testing.T) {
	const outsider = "per00000020050"
	s := sample(t)
	page := func(viewer string) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{
			{Insert: "INVITE_PAGE", Row: map[string]any{"group": "grp00000009001", "order": "k", "title": "Parking", "body": "On the street."}},
		}})
		return err
	}
	if err := page(parent); err == nil {
		t.Error("a guest added a page to the invitation")
	}
	if err := page(staff); err != nil {
		t.Fatalf("the host can't add a page: %v", err)
	}
	for viewer, want := range map[string]int{staff: 3, parent: 3, outsider: 0} {
		if got := len(as(t, s, viewer, `(from INVITE_PAGE (where (= group "grp00000009001")))`)); got != want {
			t.Errorf("%s reads %d pages, want %d", viewer, got, want)
		}
	}
}

func TestTheHostsGiveAnInvitationPageABackgroundColor(t *testing.T) {
	const schedule = "ipg00000009001"
	s := sample(t)
	set := func(viewer, background string) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: schedule, Cells: map[string]any{"background": background}}}})
		return err
	}
	if err := set(parent, "#ff3d8f"); err == nil {
		t.Error("a guest changed a page's background")
	}
	if err := set(staff, "url(https://example.com/x.png)"); err == nil {
		t.Error("a page took a background that is not a color")
	}
	if err := set(staff, "#ff3d8f"); err != nil {
		t.Fatalf("the host can't set a page's background: %v", err)
	}
	if got := as(t, s, parent, `(from INVITE_PAGE (where (= id "ipg00000009001")) (columns background))`); len(got) != 1 || got[0]["background"] != "#ff3d8f" {
		t.Errorf("a guest reads %v", got)
	}
}

func TestTheHostsChangeTheirInvitationsLookWithinWhatADesignTakes(t *testing.T) {
	s := sample(t)
	set := func(viewer string, cells map[string]any) error {
		_, err := Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: []Edit{{Set: "grp00000009001", Cells: cells}}})
		return err
	}
	look := map[string]any{"envelope": larpTheme, "seal": areaTheme, "liner": skatingTheme, "hosted_by": "The Lindqvists", "subtitle": "Bounce, pizza and cake", "design": `{"card": {"background": "#4a1f5c"}}`}
	if err := set(parent, look); err == nil {
		t.Error("a guest changed the invitation's look")
	}
	if err := set(staff, look); err != nil {
		t.Fatalf("the host can't change the look: %v", err)
	}
	if err := set(staff, map[string]any{"design": `{"card": {"background": "url(https://example.com/x.png)"}}`}); err == nil {
		t.Error("a design took a background that is not a color")
	}
	got := as(t, s, parent, `(from GROUP (where (= id "grp00000009001")) (columns envelope seal liner hosted_by subtitle design))`)
	if len(got) != 1 || got[0]["seal"] != areaTheme || got[0]["hosted_by"] != "The Lindqvists" || got[0]["subtitle"] != "Bounce, pizza and cake" || got[0]["design"] != `{"card": {"background": "#4a1f5c"}}` {
		t.Errorf("a guest reads %v", got)
	}
}

func TestANoteToTheHostsIsTheirsAndTheGuestHouseholds(t *testing.T) {
	const (
		theo     = "grp00000009001"
		outsider = "per00000020050"
	)
	s := sample(t)
	write := func(viewer string, edits ...Edit) ([]string, error) {
		return Write(context.Background(), s, access.Actor{Email: "test@example.org"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
	}
	note := func(viewer, replyTo, text string) Edit {
		row := map[string]any{"group": theo, "person": viewer, "text": text, "added": "2026-11-26 10:00:00"}
		if replyTo != "" {
			row["reply_to"] = replyTo
		}
		return Edit{Insert: "COMMENT", Row: row}
	}
	ids, err := write(parent, note(parent, "", "Is the bounce house outside?"))
	if err != nil {
		t.Fatalf("a guest can't write the hosts: %v", err)
	}
	asked := ids[0]
	if _, err := write(parent, note(parent, asked, "Answering myself")); err == nil {
		t.Error("a guest replied as if a host")
	}
	if _, err := write(staff, note(staff, asked, "Yes, in the backyard.")); err != nil {
		t.Fatalf("the host can't reply: %v", err)
	}
	for viewer, want := range map[string]int{staff: 2, parent: 2, student: 2, outsider: 0} {
		if got := len(as(t, s, viewer, "(from COMMENT)")); got != want {
			t.Errorf("%s reads %d notes, want %d", viewer, got, want)
		}
	}
	if _, err := write(outsider, Edit{Delete: asked}); err == nil {
		t.Error("someone else took the guest's note away")
	}
	if _, err := write(parent, Edit{Delete: asked}); err != nil {
		t.Fatalf("the guest can't take their note back: %v", err)
	}
	if got := len(as(t, s, staff, "(from COMMENT)")); got != 0 {
		t.Errorf("taking the note back left %d notes, want its reply gone too", got)
	}
}
