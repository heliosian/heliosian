package db

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	nobody  = ""
	student = "per00000000001"
	parent  = "per00000000002"
	staff   = "per00000000003"
	guest   = "per00000000004"
)

func as(t *testing.T, s *Store, viewer, src string) []store.Row {
	t.Helper()
	return s.Model().Run(t.Context(), mustParse(t, src), Env{Viewer: viewer, Now: testNow}).Rows()
}

func TestEveryTableHasAReadPolicy(t *testing.T) {
	for _, table := range Tables {
		if len(policies.read[table.Name]) == 0 {
			t.Errorf("%s has no read policy", table.Name)
		}
	}
}

func TestPoliciesRefuse(t *testing.T) {
	for src, want := range map[string]string{
		strings.Replace(PolicySource, "(read SETTING (id app key value) true)", "(read SETTING (id app key) true)", 1): "SETTING.value has no read grant",
		PolicySource + `(read PERSON (vc_phone) (system "import"))`:                                                    "PERSON.vc_phone is private",
		PolicySource + `(read MEMBER.price true)`:                                                                      "policies are define",
	} {
		if _, _, err := compilePolicies(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("compilePolicies = %v, want %q", err, want)
		}
	}
}

func TestWhoSeesWhichRows(t *testing.T) {
	s := sample(t)
	viewers := []string{nobody, student, parent, staff, guest}
	for table, want := range map[string][]int{
		"PERSON":           {3, 4, 4, 4, 4},
		"PERSON_EMAIL":     {5, 5, 5, 5, 5},
		"PERSON_SETTING":   {0, 0, 0, 1, 0},
		"BIRTHDAY_YEAR":    {0, 0, 0, 1, 0},
		"COLLECTION":       {0, 0, 1, 0, 0},
		"COLLECTION_GROUP": {0, 0, 2, 0, 0},
		"GROUP":            {0, 26, 28, 29, 0},
		"MEMBER":           {0, 21, 22, 22, 1},
		"EFFECTIVE_MEMBER": {0, 27, 28, 29, 1},
		"RULE":             {0, 0, 0, 6, 0},
		"DOCUMENT":         {8, 8, 9, 9, 8},
		"DOCUMENT_GROUP":   {0, 0, 1, 1, 0},
		"CONTENT":          {6, 6, 6, 6, 6},
		"INBOX":            {0, 0, 0, 0, 0},
		"MESSAGE":          {0, 0, 1, 1, 0},
		"RECIPIENT":        {0, 0, 1, 1, 0},
		"CHARITY":          {0, 0, 0, 1, 0},
		"APP":              {0, 2, 2, 2, 0},
		"ALIAS":            {0, 1, 1, 1, 0},
	} {
		for i, viewer := range viewers {
			if got := len(as(t, s, viewer, "(from "+table+")")); got != want[i] {
				t.Errorf("%s as %q: %d rows, want %d", table, viewer, got, want[i])
			}
		}
	}
}

func TestBirthdaysShowToTheGroupTheAppIsOpenTo(t *testing.T) {
	s := sample(t)
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "birthday", "name": "Helios Birthday Team", "visible_to": "grp00000000020", "admins": "grp00000000003"})); err != nil {
		t.Fatal(err)
	}
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		want := []int{0, 1, 1, 1, 0}[i]
		for _, table := range []string{"BIRTHDAY_YEAR", "CHARITY"} {
			if got := len(as(t, s, viewer, "(from "+table+")")); got != want {
				t.Errorf("%s as %q: %d rows, want %d", table, viewer, got, want)
			}
		}
	}
}

func TestSuperAdminsReadEveryReport(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet, store.Insert("REPORT", store.Row{"id": "rpt00000000001", "app": "who", "kind": "bug", "summary": "Broken", "status": "new", "added_by": parent, "added": "2026-09-01 10:00:00"})); err != nil {
		t.Fatal(err)
	}
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		if got, want := len(as(t, s, viewer, "(from REPORT)")), []int{0, 0, 1, 1, 0}[i]; got != want {
			t.Errorf("as %q: %d reports, want %d", viewer, got, want)
		}
	}
}

func TestMailShowsToWhomItWasSentAndToSuperAdmins(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000020", "kind": "mail", "name": "Not yet placed"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000021", "kind": "mail", "name": "To Hummingbirds parents"}),
		store.Insert("DOCUMENT_GROUP", store.Row{"id": "dgr00000000021", "document": "doc00000000021", "group": "grp00000000030", "relation": "sent_to"}),
	); err != nil {
		t.Fatal(err)
	}
	for viewer, want := range map[string]string{nobody: "", student: "", parent: "doc00000000021", staff: "doc00000000020 doc00000000021", guest: ""} {
		got := []string{}
		for _, row := range as(t, s, viewer, `(from DOCUMENT (where (in id "doc00000000020" "doc00000000021")))`) {
			got = append(got, row["id"])
		}
		slices.Sort(got)
		if strings.Join(got, " ") != want {
			t.Errorf("as %q: %v, want %q", viewer, got, want)
		}
	}
}

func TestDocumentsUnderAMailedPostFollowIt(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "application/pdf", "size": "200"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000003", "hash": "c3", "blob": "content/c3", "mime": "text/markdown", "size": "30"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Field trip"}),
		store.Insert("DOCUMENT_GROUP", store.Row{"id": "dgr00000000010", "document": "doc00000000010", "group": "grp00000000030", "relation": "sent_to"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002", "filename": "permission.pdf"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "extract", "parent": "doc00000000011", "content": "cnt00000000003"}),
	); err != nil {
		t.Fatal(err)
	}
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		want := []int{0, 0, 3, 3, 0}[i]
		if got := len(as(t, s, viewer, `(from DOCUMENT (where (in id "doc00000000010" "doc00000000011" "doc00000000012")))`)); got != want {
			t.Errorf("the post and what sits under it as %q: %d documents, want %d", viewer, got, want)
		}
		if got := len(as(t, s, viewer, `(from CONTENT (where (in id "cnt00000000001" "cnt00000000002" "cnt00000000003")))`)); got != want {
			t.Errorf("the post's bytes and what sits under it as %q: %d, want %d", viewer, got, want)
		}
	}
}

func TestTheImportAddsImagesUnderParts(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Clubs this week"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "linked", "parent": "doc00000000011", "url": "https://example.org/clubs.png", "fetch": "sign_in"}),
	); err != nil {
		t.Fatal(err)
	}
	linked := func(parent, content string) Change {
		return Change{Table: "DOCUMENT", New: store.Row{"relation": "linked", "parent": parent, "content": content, "url": "https://example.org/clubs.png"}}
	}
	for _, c := range []struct {
		name   string
		system string
		change Change
		ok     bool
	}{
		{"place an image under a part", "import", linked("doc00000000011", ""), true},
		{"place an image under a root", "import", linked("doc00000000010", ""), false},
		{"place an image with its bytes", "import", linked("doc00000000011", "cnt00000000002"), false},
		{"place an image as another system", "extract", linked("doc00000000011", ""), false},
		{"store an image", "import", Change{Table: "CONTENT", New: store.Row{"hash": "c3", "blob": "content/c3", "mime": "image/png", "size": "30"}}, true},
		{"store a page", "import", Change{Table: "CONTENT", New: store.Row{"hash": "c3", "blob": "content/c3", "mime": "text/html", "size": "30"}}, true},
		{"store plain text", "import", Change{Table: "CONTENT", New: store.Row{"hash": "c3", "blob": "content/c3", "mime": "text/plain", "size": "30"}}, false},
		{"fill an image still to fetch", "import", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "cnt00000000002", "fetch": ""}), true},
		{"fill a part", "import", change(t, s, "DOCUMENT", []string{"doc00000000011"}, store.Row{"content": "cnt00000000001"}), false},
		{"fill an image as another system", "extract", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "cnt00000000002"}), false},
	} {
		err := s.Model().Authorize(Env{System: c.system, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestTheImportRefusesAFetchedPage(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "mail", "content": "cnt00000000001", "name": "Clubs this week"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "part", "parent": "doc00000000010", "content": "cnt00000000002"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000012", "relation": "linked", "parent": "doc00000000011", "url": "https://example.org/dl/b33482", "content": "cnt00000000002"}),
	); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		system string
		change Change
		ok     bool
	}{
		{"refuse a fetched page", "import", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "", "fetch": "refused"}), true},
		{"blank a fetched page to fetch it again", "import", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "", "extracted": ""}), true},
		{"refuse a fetched page but keep its bytes", "import", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"fetch": "refused"}), false},
		{"mark a fetched page gone", "import", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "", "fetch": "gone"}), false},
		{"refuse a part", "import", change(t, s, "DOCUMENT", []string{"doc00000000011"}, store.Row{"content": "", "fetch": "refused"}), false},
		{"refuse a fetched page as another system", "extract", change(t, s, "DOCUMENT", []string{"doc00000000012"}, store.Row{"content": "", "fetch": "refused"}), false},
	} {
		err := s.Model().Authorize(Env{System: c.system, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestTheImportSendsADocumentBackToBeRead(t *testing.T) {
	s := sample(t)
	if err := commit(s, DocumentsSheet,
		store.Insert("CONTENT", store.Row{"id": "cnt00000000001", "hash": "a1", "blob": "content/a1", "mime": "application/pdf", "size": "100"}),
		store.Insert("CONTENT", store.Row{"id": "cnt00000000002", "hash": "b2", "blob": "content/b2", "mime": "text/markdown", "size": "20"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000010", "kind": "file", "content": "cnt00000000001", "name": "Updates", "extracted": "2026-02-12 01:48:03"}),
		store.Insert("DOCUMENT", store.Row{"id": "doc00000000011", "relation": "extract", "parent": "doc00000000010", "content": "cnt00000000002"}),
	); err != nil {
		t.Fatal(err)
	}
	docs := s.Model().Table("DOCUMENT")
	extract, _ := docs.Get("doc00000000011")
	file, _ := docs.Get("doc00000000010")
	for _, c := range []struct {
		name   string
		system string
		change Change
		ok     bool
	}{
		{"remove an extract", "import", Change{Table: "DOCUMENT", Old: extract}, true},
		{"remove a file", "import", Change{Table: "DOCUMENT", Old: file}, false},
		{"remove an extract as another system", "extract", Change{Table: "DOCUMENT", Old: extract}, false},
		{"mark a file still to read", "import", change(t, s, "DOCUMENT", []string{"doc00000000010"}, store.Row{"extracted": ""}), true},
		{"mark a file read", "import", change(t, s, "DOCUMENT", []string{"doc00000000010"}, store.Row{"extracted": "2026-02-13 01:48:03"}), false},
	} {
		err := s.Model().Authorize(Env{System: c.system, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

func TestTheImportSetsAPersonsSlug(t *testing.T) {
	s := sample(t)
	for _, c := range []struct {
		name   string
		system string
		change Change
		ok     bool
	}{
		{"slug a Veracross person", "import", change(t, s, "PERSON", []string{"per00000000001"}, store.Row{"slug": "juni.ashdown"}), true},
		{"slug a guest", "import", change(t, s, "PERSON", []string{"per00000000004"}, store.Row{"slug": "guest"}), false},
		{"slug a person as another system", "extract", change(t, s, "PERSON", []string{"per00000000001"}, store.Row{"slug": "juni.ashdown"}), false},
		{"alias a person's old slug", "import", Change{Table: "ALIAS", New: store.Row{"alias": "juni.a", "target": "per00000000001"}}, true},
		{"alias a person's old slug as another system", "extract", Change{Table: "ALIAS", New: store.Row{"alias": "juni.a", "target": "per00000000001"}}, false},
	} {
		err := s.Model().Authorize(Env{System: c.system, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000001"}, store.Row{"slug": "juni.ashdown"})); err != nil {
		t.Fatal(err)
	}
	r := runAs(t, s.Model(), "per00000000003", `(from PERSON (where (= slug "juni.ashdown")))`)
	if got := r.Resources["PERSON"]["per00000000001"]["slug"]; got != "juni.ashdown" {
		t.Errorf("a member reads the slug as %q: %v", got, r.Resources)
	}
}

func TestAPersonsOldSlugReadsWhileTheyDo(t *testing.T) {
	s := sample(t)
	if err := commit(s, ConfigSheet, store.Insert("ALIAS", store.Row{"id": "als00000000002", "alias": "maya.l", "target": "per00000000003"})); err != nil {
		t.Fatal(err)
	}
	if got := as(t, s, parent, `(from ALIAS (where (= alias "maya.l")))`); len(got) != 1 || got[0]["target"] != "per00000000003" {
		t.Fatalf("a parent reads Maya's old slug as %v", got)
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": "per00000000003"}, store.Row{"hidden": "Yes"})); err != nil {
		t.Fatal(err)
	}
	if got := as(t, s, parent, `(from ALIAS (where (= alias "maya.l")))`); len(got) != 0 {
		t.Fatalf("a parent reads a hidden person's old slug: %v", got)
	}
}

func TestAnAppsAdminsReadItsMailsContent(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000070", "kind": "admins", "name": "When Admins", "status": "open", "visible_to": "grp00000000070"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000070", "group": "grp00000000070", "person": student, "member": "yes"}),
	); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000070", "key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000070"})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, DocumentsSheet, store.Insert("CONTENT", store.Row{"id": "cnt00000000005", "hash": "e5", "blob": "content/e5", "mime": "message/rfc822", "size": "50"})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, MailSheet, store.Insert("MESSAGE", store.Row{"id": "msg00000000005", "direction": "in", "kind": "reply", "group": "grp00000000040", "content": "cnt00000000005", "created": "2026-09-27 10:00"})); err != nil {
		t.Fatal(err)
	}
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		want := []int{0, 1, 0, 1, 0}[i]
		if got := len(as(t, s, viewer, `(from CONTENT (where (= id "cnt00000000005")))`)); got != want {
			t.Errorf("a reply about the picnic's bytes as %q: %d, want %d", viewer, got, want)
		}
	}
}

func TestAWaitlistShowsToTheWaitingThePartysManagersAndAdmins(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000081", "kind": "group", "name": "Fondue Night Waitlist", "parent": "grp00000000080", "status": "open"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000082", "kind": "group", "name": "Fondue Night Managers", "status": "open", "managed_by": "grp00000000082"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000080", "kind": "party", "name": "Fondue Night", "status": "open", "visible_to": "grp00000000004", "waitlist": "grp00000000081", "managed_by": "grp00000000082"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000080", "group": "grp00000000082", "person": student, "member": "yes"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000081", "group": "grp00000000081", "person": parent, "member": "yes", "added": "2026-09-03 21:12:05"}),
	); err != nil {
		t.Fatal(err)
	}
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		if got, want := len(as(t, s, viewer, `(from MEMBER (where (= group "grp00000000081")))`)), []int{0, 1, 1, 1, 0}[i]; got != want {
			t.Errorf("places on the waitlist as %q: %d, want %d", viewer, got, want)
		}
		if got, want := len(as(t, s, viewer, `(from GROUP (where (= id "grp00000000081")))`)), []int{0, 1, 1, 1, 0}[i]; got != want {
			t.Errorf("the waitlist as %q: %d, want %d", viewer, got, want)
		}
	}
	if got := cellAs(t, s, parent, `(from GROUP (where (= id "grp00000000080")))`, "waitlist"); got != "grp00000000081" {
		t.Errorf("the party's waitlist as the parent = %q", got)
	}
	if err := commit(s, GroupsSheet, store.Insert("MEMBER", store.Row{"id": "mem00000000082", "group": "grp00000000006", "person": guest, "member": "yes"})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, guest, `(from MEMBER (where (= group "grp00000000081")))`)); n != 1 {
		t.Errorf("a Who? admin sees %d places on the waitlist, want 1", n)
	}
}

func TestManagersAreTheManagingGroupsMembers(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet,
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000090", "kind": "category", "name": "Fairs", "status": "open", "visible_to": "grp00000000004", "managed_by": "grp00000000091"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000091", "kind": "group", "name": "Fairs Managers", "status": "open", "managed_by": "grp00000000091"}),
		store.Insert("GROUP", store.Row{"posting": "members", "replying": "members", "id": "grp00000000092", "kind": "event", "name": "Book Fair", "parent": "grp00000000090", "status": "pending", "visible_to": "grp00000000004"}),
		store.Insert("MEMBER", store.Row{"id": "mem00000000090", "group": "grp00000000091", "person": parent, "member": "yes"}),
	); err != nil {
		t.Fatal(err)
	}
	fair := `(from GROUP (where (= id "grp00000000092")))`
	if n := len(as(t, s, parent, fair)); n != 1 {
		t.Error("a manager of the category can't see a pending event under it")
	}
	if n := len(as(t, s, student, fair)); n != 0 {
		t.Error("someone who manages nothing sees a pending event")
	}
	if n := len(as(t, s, student, `(from MEMBER (where (= group "grp00000000091")))`)); n != 1 {
		t.Errorf("whoever sees the category reads %d of its managers, want 1", n)
	}
	if n := len(as(t, s, student, `(from GROUP (where (= id "grp00000000091")))`)); n != 1 {
		t.Error("whoever sees the category can't read the group that manages it")
	}
	if err := commit(s, GroupsSheet, store.Update("MEMBER", store.Row{"group": "grp00000000091", "person": parent}, store.Row{"member": "excluded"})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, parent, fair)); n != 0 {
		t.Error("someone kept out of the managers group still manages")
	}
}

func TestAnAppsAdminsAppointManagers(t *testing.T) {
	s, queue := sampleWithQueue(t)
	if err := commit(s, GroupsSheet, store.Delete("MEMBER", store.Row{"group": "grp00000000005", "person": staff})); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"})); err != nil {
		t.Fatal(err)
	}
	write := func(viewer string, edits ...Edit) error {
		_, err := Write(context.Background(), s, queue, newPictures(s, queue), access.Actor{Email: "maya@example.com"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
		return err
	}
	if err := write(staff, Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000041", "person": parent, "member": "yes"}}); err != nil {
		t.Errorf("a When admin can't add a host: %v", err)
	}
	if err := write(staff,
		Edit{Insert: "GROUP", As: "hosts", Row: map[string]any{"kind": "group", "name": "Fall Picnic Hosts", "status": "open"}},
		Edit{Set: "grp00000000040", Cells: map[string]any{"managed_by": "@hosts"}},
		Edit{Set: "@hosts", Cells: map[string]any{"managed_by": "@hosts"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "@hosts", "person": student, "member": "yes"}},
	); err != nil {
		t.Errorf("a When admin can't give an event a new managers group: %v", err)
	}
	for name, e := range map[string]Edit{
		"a When admin adds to a role group": {Insert: "MEMBER", Row: map[string]any{"group": "grp00000000002", "person": guest, "member": "yes"}},
		"a When admin adds to a mail list":  {Insert: "MEMBER", Row: map[string]any{"group": "grp00000000030", "person": guest, "member": "yes"}},
	} {
		if err := write(staff, e); err == nil {
			t.Errorf("%s", name)
		}
	}
	if err := write(staff,
		Edit{Set: "grp00000000040", Cells: map[string]any{"managed_by": "grp00000000005"}},
		Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000005", "person": staff, "member": "yes"}},
	); err == nil {
		t.Error("a When admin makes themselves a super admin through an event's managers")
	}
	if err := write(parent, Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000041", "person": parent, "member": "yes"}}); err == nil {
		t.Error("a parent makes themselves a host")
	}
}

func cellAs(t *testing.T, s *Store, viewer, src, column string) string {
	t.Helper()
	rows := as(t, s, viewer, src)
	if len(rows) != 1 {
		t.Fatalf("%s as %q: %d rows", src, viewer, len(rows))
	}
	return rows[0][column]
}

func TestPhoneFollowsConsent(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet,
		store.Update("PERSON", store.Row{"id": parent}, store.Row{"vc_phone": "555-0100"}),
		store.Update("PERSON", store.Row{"id": staff}, store.Row{"vc_phone": "555-0200", "phone_override": "555-0300"}),
	); err != nil {
		t.Fatal(err)
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"vc_phone": "555-0400"})); err != nil {
		t.Fatal(err)
	}
	rowan := `(from PERSON (where (= id "per00000000002")))`
	maya := `(from PERSON (where (= id "per00000000003")))`
	family := `(from GROUP (where (= id "grp00000000020")))`
	for _, viewer := range []string{student, parent, staff} {
		for _, column := range []string{"phone", "vc_phone", "phone_override"} {
			if got := cellAs(t, s, viewer, rowan, column); got != "" {
				t.Errorf("Rowan's withheld %s as %s = %q", column, viewer, got)
			}
			if got := cellAs(t, s, viewer, family, column); got != "" {
				t.Errorf("the family's %s, withheld by Rowan, as %s = %q", column, viewer, got)
			}
		}
		if got := cellAs(t, s, viewer, maya, "phone"); got != "555-0300" {
			t.Errorf("Maya's shared phone as %s = %q, want the override", viewer, got)
		}
		for _, column := range []string{"vc_phone", "phone_override"} {
			if got := cellAs(t, s, viewer, maya, column); got != "" {
				t.Errorf("Maya's %s as %s = %q, want only the generated phone", column, viewer, got)
			}
		}
	}
	if n := len(as(t, s, student, `(from PERSON (where (= phone "555-0100")))`)); n != 0 {
		t.Fatal("a withheld phone matched a condition")
	}
}

func TestFamilyAddressFollowsItsConsent(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"vc_address": "12 Oak St"})); err != nil {
		t.Fatal(err)
	}
	q := `(from GROUP (where (= id "grp00000000020")))`
	if got := cellAs(t, s, student, q, "address"); got != "12 Oak St" {
		t.Fatalf("a shared address reads %q", got)
	}
	if got := cellAs(t, s, student, q, "vc_address"); got != "" {
		t.Fatalf("the imported address behind a shared one reads %q", got)
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"address_consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{student, parent, staff} {
		for _, column := range []string{"address", "vc_address", "address_override"} {
			if got := cellAs(t, s, viewer, q, column); got != "" {
				t.Errorf("the withheld %s as %s = %q", column, viewer, got)
			}
		}
	}
}

func TestWithheldPeopleVanish(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": student}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{nobody, student, parent, staff} {
		for _, q := range []string{
			`(from PERSON (where (= id "per00000000001")))`,
			`(from MEMBER (where (= person "per00000000001")))`,
			`(from EFFECTIVE_MEMBER (where (= person "per00000000001")))`,
			`(from BIRTHDAY_YEAR (where (= person "per00000000001")))`,
		} {
			if n := len(as(t, s, viewer, q)); n != 0 {
				t.Errorf("%s as %q: %d rows of the withheld student", q, viewer, n)
			}
		}
	}
	if n := len(as(t, s, staff, `(from MEMBER (where (= group "grp00000000010") (= person "per00000000001")))`)); n != 0 {
		t.Errorf("the classroom still counts the withheld student: %d", n)
	}
	if _, ok := s.Model().Table("PERSON").Get(student); !ok {
		t.Fatal("the model lost the withheld student the import still keeps")
	}
	for _, row := range s.Model().effectiveRows("grp00000000030") {
		if row["person"] == parent {
			t.Errorf("a rule reached the parent through the withheld student: %v", row)
		}
	}
}

func TestWithheldPersonSignsInAsNobody(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	m := s.Model()
	if got := m.SignedIn("rowan@example.com"); got != "" {
		t.Errorf("a withheld parent signs in as %q", got)
	}
	if got := m.PersonOf("rowan@example.com"); got != parent {
		t.Errorf("the consent import no longer finds the withheld parent: %q", got)
	}
}

func TestAGuestSharesAnAddressButNeverSignsInWithIt(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000099", "address": "rowan@example.com", "person": guest, "primary": "Yes", "source": "guest"})); err != nil {
		t.Fatalf("a guest can't share a person's address: %v", err)
	}
	err := commit(s, PeopleSheet, store.Insert("PERSON_EMAIL", store.Row{"id": "eml00000000098", "address": "rowan@example.com", "person": staff, "source": "manual"}))
	if err == nil || !strings.Contains(err.Error(), "two rows have the same") {
		t.Errorf("two people who aren't guests share an address: %v", err)
	}
	m := s.Model()
	if got := m.SignedIn("rowan@example.com"); got != parent {
		t.Errorf("the shared address signs in as %q, not the parent", got)
	}
	if got := m.PersonOf("rowan@example.com"); got != parent {
		t.Errorf("the shared address belongs to %q, not the parent", got)
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	if got := s.Model().SignedIn("rowan@example.com"); got != "" {
		t.Errorf("a withheld parent signs in as %q through their guest", got)
	}
}

func TestWritesCantReachWithheldRows(t *testing.T) {
	s, queue := sampleWithQueue(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": student}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	env := Env{Viewer: parent, Now: testNow}
	for name, c := range map[string]struct {
		w    Edit
		want string
	}{
		"set":       {Edit{Set: student, Cells: map[string]any{"name_long_override": "Juni A."}}, "no PERSON " + student},
		"reference": {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": student, "member": "yes"}}, "names no row " + student},
	} {
		_, err := Write(context.Background(), s, queue, newPictures(s, queue), access.Actor{Email: "rowan@example.com"}, env, Batch{Batch: []Edit{c.w}})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s on the withheld student: %v, want %q", name, err, c.want)
		}
	}
}

func TestWithheldFamilyNeverAppears(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{nobody, student, parent, staff} {
		if n := len(as(t, s, viewer, `(from GROUP (where (= id "grp00000000020")))`)); n != 0 {
			t.Errorf("the withheld family as %q: %d rows", viewer, n)
		}
		if n := len(as(t, s, viewer, `(from MEMBER (where (= group "grp00000000020")))`)); n != 0 {
			t.Errorf("the withheld family's memberships as %q: %d rows", viewer, n)
		}
	}
}

func TestGuestsAreAlwaysShown(t *testing.T) {
	s := sample(t)
	if p, _ := s.Model().Table("PERSON").Get(guest); p["consent"] != "" {
		t.Fatalf("the sample guest has consent %q", p["consent"])
	}
	if _, ok := s.Model().Shown("PERSON").Get(guest); !ok {
		t.Fatal("a guest with no consent is hidden")
	}
}

func TestConsentFailsClosed(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": staff}, store.Row{"consent": ""})); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Model().Shown("PERSON").Get(staff); ok {
		t.Fatal("a person with no consent yet is shown")
	}
}

func TestOnlyTheImportSeesWithheldRows(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": student}, store.Row{"consent": "withheld"})); err != nil {
		t.Fatal(err)
	}
	q := mustParse(t, `(from PERSON (where (= id "per00000000001")))`)
	if n := len(s.Model().Run(t.Context(), q, Env{System: importReader, Now: testNow}).IDs); n != 1 {
		t.Errorf("the import sees %d withheld students, want 1", n)
	}
	if n := len(s.Model().Run(t.Context(), q, Env{System: "other", Now: testNow}).IDs); n != 0 {
		t.Errorf("another system reader sees %d withheld students, want 0", n)
	}
}

func TestPrivateColumnsNeverShow(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": staff}, store.Row{"vc_phone": "555-0200"})); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"PERSON", "GROUP"} {
		ts, _ := Lookup(table)
		for _, row := range s.Model().Shown(table).All() {
			for _, c := range ts.Columns {
				if _, ok := row[c.Name]; c.Private && ok {
					t.Errorf("%s %s shows private %s", table, row["id"], c.Name)
				}
			}
		}
	}
	if p, _ := s.Model().Table("PERSON").Get(staff); p["vc_phone"] != "555-0200" {
		t.Errorf("the model lost the imported phone the import compares with: %q", p["vc_phone"])
	}
}

func TestPriceAndToken(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": guest}, store.Row{"price": "7.25"})); err != nil {
		t.Fatal(err)
	}
	q := `(from MEMBER (where (= person "per00000000004") (= member "yes") (= guest_of "per00000000002")))`
	for viewer, want := range map[string]string{student: "", parent: "7.25", staff: "7.25", guest: "7.25"} {
		if got := cellAs(t, s, viewer, q, "price"); got != want {
			t.Errorf("the guest's price as %s = %q, want %q", viewer, got, want)
		}
	}
	for viewer, want := range map[string]string{parent: "tok00000000001", staff: ""} {
		if got := cellAs(t, s, viewer, `(from RECIPIENT)`, "token"); got != want {
			t.Errorf("the token as %s = %q, want %q", viewer, got, want)
		}
	}
}

func TestHiddenPersonIsUnreachable(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": student}, store.Row{"hidden": "Yes"})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, parent, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a hidden person is listed")
	}
	if got := cellAs(t, s, parent, `(from MEMBER (where (= id "mem00000000008")))`, "person"); got != "" {
		t.Fatalf("a classroom row names a hidden person: %q", got)
	}
	if n := len(as(t, s, parent, `(from MEMBER (where (= person.name_short "Juni")))`)); n != 0 {
		t.Fatal("a path reached a hidden person")
	}
	if n := len(as(t, s, parent, `(from GROUP @g (where (exists MEMBER (= group @g) (= person "per00000000001"))))`)); n != 0 {
		t.Fatal("exists probed a hidden person")
	}
	if n := len(as(t, s, student, `(from PERSON (where (= id "per00000000001")))`)); n != 1 {
		t.Fatal("a hidden person can't see themselves")
	}
	if n := len(as(t, s, staff, `(from PERSON (where (= id "per00000000001")))`)); n != 1 {
		t.Fatal("Who?'s admin can't see a hidden person")
	}
}

func TestPersonFlagsAreKeptFromReaders(t *testing.T) {
	s := sample(t)
	if err := commit(s, PeopleSheet,
		store.Update("PERSON", store.Row{"id": student}, store.Row{"hidden": "Yes", "deactivated": "2026-09-30 12:00"}),
		store.Update("PERSON", store.Row{"id": parent}, store.Row{"signed_out": "2026-09-30 11:00"})); err != nil {
		t.Fatal(err)
	}
	juni := `(from PERSON (where (= id "per00000000001")))`
	for _, c := range []struct {
		viewer, query, column, want string
	}{
		{staff, juni, "hidden", "Yes"},
		{student, juni, "hidden", ""},
		{staff, juni, "deactivated", ""},
		{student, juni, "deactivated", ""},
		{parent, `(from PERSON (where (= id "per00000000002")))`, "signed_out", ""},
		{staff, `(from PERSON (where (= id "per00000000002")))`, "signed_out", ""},
	} {
		if got := cellAs(t, s, c.viewer, c.query, c.column); got != c.want {
			t.Errorf("%s as %s = %q, want %q", c.column, c.viewer, got, c.want)
		}
	}
}

func TestPendingIsForManagersAndAdmins(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"status": "pending"})); err != nil {
		t.Fatal(err)
	}
	q := `(from GROUP (where (= id "grp00000000040")))`
	if n := len(as(t, s, parent, q)); n != 0 {
		t.Fatal("a pending event is visible")
	}
	if n := len(as(t, s, staff, q)); n != 1 {
		t.Fatal("a host can't see their pending event")
	}
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000040"}, store.Row{"managed_by": ""})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, staff, q)); n != 1 {
		t.Fatal("a super admin can't see a pending event")
	}
	if err := commit(s, GroupsSheet, store.Delete("MEMBER", store.Row{"group": "grp00000000005", "person": staff})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, staff, q)); n != 0 {
		t.Fatal("a pending event is visible to someone neither host nor When admin")
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, staff, q)); n != 1 {
		t.Fatal("a When admin can't see a pending event")
	}
}

func change(t *testing.T, s *Store, table string, key []string, cells store.Row) Change {
	t.Helper()
	rows := s.Model().Table(table)
	var old store.Row
	var ok bool
	if len(key) == 1 {
		old, ok = rows.Get(key[0])
	} else {
		old, ok = rows.Find(key...)
	}
	if !ok {
		t.Fatalf("no %s %v", table, key)
	}
	updated := store.Row{}
	for k, v := range old {
		updated[k] = v
	}
	for k, v := range cells {
		updated[k] = v
	}
	return Change{Table: table, Old: old, New: updated}
}

func TestAuthorize(t *testing.T) {
	s := sample(t)
	picnic := func(person string) []string { return []string{"grp00000000040", person} }
	notGoing := func(person string) Change {
		return Change{Table: "MEMBER", New: store.Row{"group": "grp00000000043", "person": person, "member": "yes"}}
	}
	going := func(person string) store.Row {
		row, ok := s.Model().Table("MEMBER").Find("grp00000000042", person)
		if !ok {
			t.Fatalf("%s is not going", person)
		}
		return row
	}
	for _, c := range []struct {
		name   string
		viewer string
		change Change
		ok     bool
	}{
		{"answer for oneself", parent, notGoing(parent), true},
		{"answer for one's guest", parent, notGoing(guest), true},
		{"answer for someone in one's family", student, notGoing(parent), true},
		{"answer for a stranger", student, notGoing(guest), false},
		{"answer about an event one can't see", guest, notGoing(guest), false},
		{"a host answers for anyone", staff, notGoing(student), true},
		{"answer maybe by taking back a yes", parent, Change{Table: "MEMBER", Old: going(parent)}, true},
		{"take back a stranger's yes", guest, Change{Table: "MEMBER", Old: going(parent)}, false},
		{"answer without being in", parent, Change{Table: "MEMBER", New: store.Row{"group": "grp00000000043", "person": parent, "member": "excluded"}}, false},
		{"a child leaves their family", student, change(t, s, "MEMBER", []string{"grp00000000020", student}, store.Row{"member": "cancelled"}), false},
		{"a parent takes a child out of their family", parent, change(t, s, "MEMBER", []string{"grp00000000020", student}, store.Row{"member": "excluded"}), false},
		{"give up one's place", parent, change(t, s, "MEMBER", picnic(parent), store.Row{"member": "cancelled"}), true},
		{"exclude oneself", parent, change(t, s, "MEMBER", picnic(parent), store.Row{"member": "excluded"}), false},
		{"make oneself a host", parent, Change{Table: "MEMBER", New: store.Row{"group": "grp00000000041", "person": parent, "member": "yes"}}, false},
		{"hand one's event to another group", parent, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"managed_by": "grp00000000020"}), false},
		{"a host excludes", staff, change(t, s, "MEMBER", picnic(parent), store.Row{"member": "excluded"}), true},
		{"a parent renames a child", parent, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), true},
		{"a guest renames a child", guest, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), false},
		{"Who?'s admin renames anyone", staff, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), true},
		{"a super admin is When's admin", staff, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"}), true},
		{"a host adds someone by hand", staff, Change{Table: "MEMBER", New: store.Row{"group": "grp00000000040", "person": student, "member": "yes"}}, true},
		{"a host takes someone off", staff, Change{Table: "MEMBER", Old: store.Row{"group": "grp00000000040", "person": parent, "member": "yes"}}, true},
		{"a guest adds someone by hand", guest, Change{Table: "MEMBER", New: store.Row{"group": "grp00000000040", "person": guest, "member": "yes"}}, false},
	} {
		err := s.Model().Authorize(Env{Viewer: c.viewer, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if err := commit(s, GroupsSheet, store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": parent}, store.Row{"lead": "Yes"})); err != nil {
		t.Fatal(err)
	}
	if err := s.Model().Authorize(Env{Viewer: parent, Now: testNow}, change(t, s, "MEMBER", picnic(guest), store.Row{"member": "excluded"})); err == nil {
		t.Fatal("an event's lead may exclude anyone, as if they managed it")
	}
	if err := commit(s, GroupsSheet, store.Delete("MEMBER", store.Row{"group": "grp00000000005", "person": staff})); err != nil {
		t.Fatal(err)
	}
	if err := s.Model().Authorize(Env{Viewer: staff, Now: testNow}, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"})); err == nil {
		t.Fatal("someone neither super admin nor When admin may change an event's status")
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"})); err != nil {
		t.Fatal(err)
	}
	if err := s.Model().Authorize(Env{Viewer: staff, Now: testNow}, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"})); err != nil {
		t.Fatalf("a When admin may not change an event's status: %v", err)
	}
}

func TestClientsGetThePolicyLanguage(t *testing.T) {
	s := sample(t)
	ids := func(src string) []string {
		out := []string{}
		for _, row := range as(t, s, parent, src) {
			out = append(out, row["id"])
		}
		return out
	}
	for call, body := range map[string]string{
		`(from MEMBER (where (in person (household @viewer))))`: `(from MEMBER (where (in person (select MEMBER.person (in group (select MEMBER.group (= person @viewer) (= member "yes") (= group.kind "family"))) (= member "yes")))))`,
		`(from GROUP @g (where (manages @g)))`:                  `(from GROUP @g (where (exists EFFECTIVE_MEMBER @e (= person @viewer) (exists GROUP (= managed_by @e.group) (in id (ancestors @g))))))`,
		`(from PERSON @row (where (person_visible @row)))`:      `(from PERSON @row (where (or (= @row @viewer) (and (= @row.source "guest") (exists MEMBER (in group (select MEMBER.group (= person @viewer))) (= person @row))) (and (!= @row.source "guest") (not @row.hidden) (blank @row.deactivated)))))`,
	} {
		got, want := ids(call), ids(body)
		if len(want) == 0 || !slices.Equal(got, want) {
			t.Errorf("%s answered %v, its body %v", call, got, want)
		}
	}
	if got := ids(`(from GROUP (where (system "import")))`); len(got) != 0 {
		t.Errorf("a person's query is the import's: %v", got)
	}
	if _, err := Parse(`(from GROUP @viewer)`); err == nil || !strings.Contains(err.Error(), "can't name a row") {
		t.Errorf("naming a row @viewer: %v", err)
	}
}

func TestTheJSONFormCarriesDefinitionsAndAncestors(t *testing.T) {
	text := `(from GROUP @g (where (in parent (ancestors @g)) (visible @g) (contains name "picnic") (exists MEMBER (= group @g) (in person (household @viewer))) (system "import")))`
	q, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(q.Tree())
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseJSON(raw)
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	if back.String() != q.String() {
		t.Errorf("round trip %s, want %s", back.String(), q.String())
	}
}
