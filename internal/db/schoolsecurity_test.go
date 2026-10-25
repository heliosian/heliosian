package db

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
)

func TestDirectoryAdminsCannotJoinPrivilegedGroupsThroughSchoolAssignments(t *testing.T) {
	for _, column := range []string{"classroom_override", "crew_override", "department_override"} {
		t.Run(column, func(t *testing.T) {
			s := sample(t)
			apply(t, s, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000006", "person": parent, "member": "yes"}}})
			if s.Model().SuperAdmin("rowan.ashdown@example.org") {
				t.Fatal("the directory admin is already a super admin")
			}
			body, err := json.Marshal(Batch{Batch: []Edit{{Set: parent, Cells: map[string]any{column: "grp00000000005"}}}})
			if err != nil {
				t.Fatal(err)
			}
			rec := send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("assigning Super Admins as %s got %d: %s; became super admin: %v", column, rec.Code, rec.Body, s.Model().SuperAdmin("rowan.ashdown@example.org"))
			}
			person, _ := s.Model().Table("PERSON").Get(parent)
			if person[column] != "" || s.Model().SuperAdmin("rowan.ashdown@example.org") {
				t.Fatal("the refused assignment left changes behind")
			}
			kind := strings.TrimSuffix(column, "_override")
			var target string
			for _, g := range s.Model().Table("GROUP").All() {
				if g["kind"] == kind {
					target = g["id"]
					break
				}
			}
			if target == "" {
				t.Fatalf("the sample has no %s", kind)
			}
			body, err = json.Marshal(Batch{Batch: []Edit{{Set: parent, Cells: map[string]any{column: target}}}})
			if err != nil {
				t.Fatal(err)
			}
			rec = send(t, s, http.MethodPost, "application/json", "rowan.ashdown@example.org", string(body))
			if rec.Code != http.StatusOK {
				t.Fatalf("assigning a real %s got %d: %s", kind, rec.Code, rec.Body)
			}
			member, ok := s.Model().Table("MEMBER").Find(target, parent)
			if !ok || member["member"] != "yes" || s.Model().SuperAdmin("rowan.ashdown@example.org") {
				t.Fatal("the school assignment did not create only the expected membership")
			}
		})
	}
}

func TestImportedSchoolAssignmentsCannotNameOtherGroupKinds(t *testing.T) {
	for _, column := range []string{"vc_classroom", "vc_crew", "vc_department"} {
		t.Run(column, func(t *testing.T) {
			s := sample(t)
			_, err := Write(context.Background(), s, access.System(setupEnv.System), setupEnv, Batch{Batch: []Edit{{Set: parent, Cells: map[string]any{column: "grp00000000005"}}}})
			if err == nil || !strings.Contains(err.Error(), "must name a") {
				t.Fatalf("an imported school assignment naming Super Admins: %v", err)
			}
			if s.Model().SuperAdmin("rowan.ashdown@example.org") {
				t.Fatal("the refused import created a super admin")
			}
		})
	}
}
