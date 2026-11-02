package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestMembersCannotAddAddressesToExistingGuests(t *testing.T) {
	s := sample(t)
	guest := "per00000000004"
	ids := apply(t, s, []Edit{
		{Insert: "PERSON_EMAIL", Row: map[string]any{"person": guest, "address": "guest@example.net", "source": "guest", "primary": true}},
		{Insert: "GROUP", As: "private", Row: map[string]any{"kind": "group", "name": "Guest's private list", "status": "open", "posting": "members"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@private", "person": guest, "member": "yes"}},
	})
	group, _ := s.Model().Table("GROUP").Get(ids[1])
	before := s.Model()
	if before.personAt("intruder@example.net") != "" {
		t.Fatal("the attacker's address already identifies someone")
	}
	if !before.mayPost(group, guest, false) {
		t.Fatal("the sample guest must be able to post to the private list")
	}
	for _, source := range []string{"guest", "GUEST", "Guest", " guest ", "\tGuEsT\n"} {
		t.Run(source, func(t *testing.T) {
			body := fmt.Sprintf(`{"batch":[{"insert":"PERSON_EMAIL","row":{"person":%q,"address":"intruder@example.net","source":%q,"primary":false}}]}`, guest, source)
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", body)
			if rec.Code != http.StatusForbidden {
				after := s.Model()
				person := after.personAt("intruder@example.net")
				t.Fatalf("adding an address to another guest got %d: %s; resolved identity %q; may post to guest's list: %v", rec.Code, rec.Body, person, after.mayPost(group, person, false))
			}
			if s.Model().personAt("intruder@example.net") != "" {
				t.Fatal("the refused address still identifies the guest")
			}
		})
	}
}

func TestGuestAddressesAreAssignedOnlyInTheCreationBatch(t *testing.T) {
	s := sample(t)
	create := `{"batch":[{"insert":"PERSON","as":"guest","row":{"source":"guest","name_long_override":"Pat Coach"}},{"insert":"PERSON_EMAIL","row":{"person":"@guest","address":"coach@example.net","source":"guest","primary":true}}]}`
	rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", create)
	if rec.Code != http.StatusOK || s.Model().personAt("coach@example.net") == "" {
		t.Fatalf("creating a guest and address together got %d: %s", rec.Code, rec.Body)
	}
	guest := s.Model().personAt("coach@example.net")
	for _, target := range []struct {
		person  string
		primary bool
	}{
		{guest, false},
		{"per00000000004", true},
	} {
		batch := Batch{Batch: []Edit{
			{Insert: "PERSON", As: "new", Row: map[string]any{"source": "guest", "name_long_override": "Unrelated new guest"}},
			{Insert: "PERSON_EMAIL", Row: map[string]any{"person": target.person, "address": "later@example.net", "source": "guest", "primary": target.primary}},
		}}
		body, err := json.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		before := s.Model().Table("PERSON").Len()
		rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
		if rec.Code != http.StatusForbidden || s.Model().Table("PERSON").Len() != before || s.Model().personAt("later@example.net") != "" {
			t.Fatalf("adding an address after creation got %d: %s; the refused batch must leave no guest or address behind", rec.Code, rec.Body)
		}
		rec = send(t, s, http.MethodPost, "application/json", superAdmin, string(body))
		if rec.Code != http.StatusOK || s.Model().personAt("later@example.net") != target.person {
			t.Fatalf("a super admin adding a guest address got %d: %s", rec.Code, rec.Body)
		}
		email, _ := s.Model().Table("PERSON_EMAIL").Find("later@example.net", "Yes")
		apply(t, s, []Edit{{Delete: email["id"]}})
	}
}

func TestGuestEmailSourceSpellingsNeverAdmitDirectorySignIn(t *testing.T) {
	for _, source := range []string{"guest", "GUEST", "Guest", " guest ", "\tGuEsT\n"} {
		t.Run(source, func(t *testing.T) {
			s := sample(t)
			body := fmt.Sprintf(`{"batch":[{"insert":"PERSON","as":"guest","row":{"source":"guest","name_long_override":"Pat Coach"}},{"insert":"PERSON_EMAIL","row":{"person":"@guest","address":"coach@heliosschool.org","source":%q,"primary":true}}]}`, source)
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", body)
			if rec.Code != http.StatusOK {
				t.Fatalf("creating a guest got %d: %s", rec.Code, rec.Body)
			}
			m := s.Model()
			if m.personAt("coach@heliosschool.org") == "" {
				t.Fatal("the new address does not identify the guest")
			}
			if person := m.SignedIn("coach@heliosschool.org"); person != "" {
				t.Fatalf("a guest address signs in as %s", person)
			}
		})
	}
}
