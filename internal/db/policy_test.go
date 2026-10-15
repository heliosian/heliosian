package db

import (
	"context"
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
		PolicySource + `(read PERSON (vc_phone) (mode "import"))`:                                                      "PERSON.vc_phone is private",
		PolicySource + `(read MEMBER.price true)`:                                                                      "policies are define",
		PolicySource + `(show MEMBER (= person @viewer))`:                                                              "not @viewer",
		PolicySource + `(show MEMBER (mode "import"))`:                                                                 "not the request's mode",
		PolicySource + `(show MEMBER (super_admin))`:                                                                   "not @viewer",
		PolicySource + `(show PERSON true)`:                                                                            "PERSON has two show clauses",
		PolicySource + `(show EFFECTIVE_MEMBER true)`:                                                                  "is generated",
		PolicySource + `(set * true)`:                                                                                  "set *.*",
		PolicySource + `(read *.* true)`:                                                                               "set *.*",
		PolicySource + `(read * (id) true)`:                                                                            "(read * (*) condition)",
		PolicySource + `(read * (= kind "family"))`:                                                                    "names no row",
		PolicySource + `(reveal (= @row.kind "family"))`:                                                               "no row named @row",
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
		"PERSON":           {96, 97, 97, 97, 97},
		"PERSON_EMAIL":     {80, 80, 80, 80, 80},
		"PERSON_SETTING":   {0, 0, 0, 1, 0},
		"BIRTHDAY_YEAR":    {0, 0, 0, 1, 0},
		"SAVED_SEARCH":     {0, 0, 1, 1, 0},
		"GROUP":            {0, 101, 107, 108, 0},
		"MEMBER":           {0, 333, 344, 344, 1},
		"EFFECTIVE_MEMBER": {0, 483, 494, 495, 1},
		"RULE":             {0, 0, 0, 22, 0},
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "APP", Row: map[string]any{"key": "birthday", "name": "Helios Birthday Team", "visible_to": "grp00000000020", "admins": "grp00000000003"}}})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "REPORT", Row: map[string]any{"app": "who", "kind": "bug", "summary": "Broken", "status": "new", "added_by": parent, "added": "2026-09-01 10:00:00"}}})
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		if got, want := len(as(t, s, viewer, "(from REPORT)")), []int{0, 0, 1, 1, 0}[i]; got != want {
			t.Errorf("as %q: %d reports, want %d", viewer, got, want)
		}
	}
}

func TestMailShowsToWhomItWasSentAndToSuperAdmins(t *testing.T) {
	s, queue := sampleWithQueue(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "DOCUMENT", Row: map[string]any{"kind": "mail", "name": "Not yet placed"}},
		{Insert: "DOCUMENT", As: "sent", Row: map[string]any{"kind": "mail", "name": "To Hummingbirds parents"}},
		{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": "@sent", "group": "grp00000000030", "relation": "sent_to"}},
	})
	unplaced, sent := made[0], made[1]
	both := []string{unplaced, sent}
	slices.Sort(both)
	for viewer, want := range map[string]string{nobody: "", student: "", parent: sent, staff: strings.Join(both, " "), guest: ""} {
		got := []string{}
		for _, row := range as(t, s, viewer, `(from DOCUMENT (where (in id "`+unplaced+`" "`+sent+`")))`) {
			got = append(got, row["id"])
		}
		slices.Sort(got)
		if strings.Join(got, " ") != want {
			t.Errorf("as %q: %v, want %q", viewer, got, want)
		}
	}
}

func TestDocumentsUnderAMailedPostFollowIt(t *testing.T) {
	s, queue := sampleWithQueue(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "pdf", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "application/pdf", "size": "200"}},
		{Insert: "CONTENT", As: "text", Row: map[string]any{"hash": "c3", "blob": "content/c3", "mime": "text/markdown", "size": "30"}},
		{Insert: "DOCUMENT", As: "trip", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Field trip"}},
		{Insert: "DOCUMENT_GROUP", Row: map[string]any{"document": "@trip", "group": "grp00000000030", "relation": "sent_to"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@trip", "content": "@pdf", "filename": "permission.pdf"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "extract", "parent": "@part", "content": "@text"}},
	})
	documents := `(from DOCUMENT (where (in id "` + made[3] + `" "` + made[5] + `" "` + made[6] + `")))`
	contents := `(from CONTENT (where (in id "` + made[0] + `" "` + made[1] + `" "` + made[2] + `")))`
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		want := []int{0, 0, 3, 3, 0}[i]
		if got := len(as(t, s, viewer, documents)); got != want {
			t.Errorf("the post and what sits under it as %q: %d documents, want %d", viewer, got, want)
		}
		if got := len(as(t, s, viewer, contents)); got != want {
			t.Errorf("the post's bytes and what sits under it as %q: %d, want %d", viewer, got, want)
		}
	}
}

func TestSuperAdminsWriteAnythingAndServerJobsAreUnchecked(t *testing.T) {
	s, queue := sampleWithQueue(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "CONTENT", As: "mail", Row: map[string]any{"hash": "a1", "blob": "content/a1", "mime": "message/rfc822", "size": "100"}},
		{Insert: "CONTENT", As: "html", Row: map[string]any{"hash": "b2", "blob": "content/b2", "mime": "text/html; charset=utf-8", "size": "200"}},
		{Insert: "DOCUMENT", As: "clubs", Row: map[string]any{"kind": "mail", "content": "@mail", "name": "Clubs this week"}},
		{Insert: "DOCUMENT", As: "part", Row: map[string]any{"relation": "part", "parent": "@clubs", "content": "@html"}},
		{Insert: "DOCUMENT", Row: map[string]any{"relation": "linked", "parent": "@part", "url": "https://example.org/clubs.png", "fetch": "sign_in"}},
	})
	html, root, linked := made[1], made[2], made[4]
	file, _ := s.Model().Table("DOCUMENT").Get(root)
	for name, c := range map[string]Change{
		"place an image under a root":   {Table: "DOCUMENT", New: store.Row{"relation": "linked", "parent": root, "url": "https://example.org/clubs.png"}},
		"store plain text":              {Table: "CONTENT", New: store.Row{"hash": "c3", "blob": "content/c3", "mime": "text/plain", "size": "30"}},
		"fill an image still to fetch":  change(t, s, "DOCUMENT", []string{linked}, store.Row{"content": html, "fetch": ""}),
		"remove an email":               {Table: "DOCUMENT", Old: file},
		"slug a guest":                  change(t, s, "PERSON", []string{"per00000000004"}, store.Row{"slug": "guest"}),
		"alias a person's old slug":     {Table: "ALIAS", New: store.Row{"alias": "juni.a", "target": "per00000000001"}},
		"set a name Veracross supplies": change(t, s, "PERSON", []string{"per00000000001"}, store.Row{"vc_name": "Juni Ashdown"}),
	} {
		if err := s.Model().Authorize(Env{Viewer: staff, Now: testNow}, c); err != nil {
			t.Errorf("a super admin may %s: %v", name, err)
		}
		if err := s.Model().Authorize(Env{Viewer: parent, Now: testNow}, c); err == nil {
			t.Errorf("a parent may %s", name)
		}
		if err := s.Model().Authorize(Env{System: "extract", Now: testNow}, c); err != nil {
			t.Errorf("a server job is checked to %s: %v", name, err)
		}
	}
}

func TestAPersonsSlugReads(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: "per00000000001", Cells: map[string]any{"slug": "juni.ashdown"}}})
	r := runAs(t, s.Model(), "per00000000003", `(from PERSON (where (= slug "juni.ashdown")))`)
	if got := r.Resources["PERSON"]["per00000000001"]["slug"]; got != "juni.ashdown" {
		t.Errorf("a member reads the slug as %q: %v", got, r.Resources)
	}
}

func TestAPersonsOldSlugReadsWhileTheyDo(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Insert: "ALIAS", Row: map[string]any{"alias": "maya.l", "target": "per00000000003"}}})
	if got := as(t, s, parent, `(from ALIAS (where (= alias "maya.l")))`); len(got) != 1 || got[0]["target"] != "per00000000003" {
		t.Fatalf("a parent reads Maya's old slug as %v", got)
	}
	apply(t, s, queue, []Edit{{Set: "per00000000003", Cells: map[string]any{"hidden": "Yes"}}})
	if got := as(t, s, parent, `(from ALIAS (where (= alias "maya.l")))`); len(got) != 0 {
		t.Fatalf("a parent reads a hidden person's old slug: %v", got)
	}
}

func TestAnAppsAdminsReadItsMailsContent(t *testing.T) {
	s, queue := sampleWithQueue(t)
	reply := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "admins", Row: map[string]any{"posting": "members", "replying": "members", "kind": "admins", "name": "When Admins", "status": "open"}},
		{Set: "@admins", Cells: map[string]any{"visible_to": "@admins"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@admins", "person": student, "member": "yes"}},
		{Insert: "APP", Row: map[string]any{"key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "@admins"}},
		{Insert: "CONTENT", As: "reply", Row: map[string]any{"hash": "e5", "blob": "content/e5", "mime": "message/rfc822", "size": "50"}},
		{Insert: "MESSAGE", Row: map[string]any{"direction": "in", "kind": "reply", "group": "grp00000000040", "content": "@reply", "created": "2026-09-27 10:00"}},
	})[4]
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		want := []int{0, 1, 0, 1, 0}[i]
		if got := len(as(t, s, viewer, `(from CONTENT (where (= id "`+reply+`")))`)); got != want {
			t.Errorf("a reply about the picnic's bytes as %q: %d, want %d", viewer, got, want)
		}
	}
}

func TestAWaitlistShowsToTheWaitingThePartysManagersAndAdmins(t *testing.T) {
	s, queue := sampleWithQueue(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "managers", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Fondue Night Managers", "status": "open"}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers"}},
		{Insert: "GROUP", As: "party", Row: map[string]any{"posting": "members", "replying": "members", "kind": "party", "name": "Fondue Night", "status": "open", "visible_to": "grp00000000004", "managed_by": "@managers"}},
		{Insert: "GROUP", As: "waitlist", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Fondue Night Waitlist", "parent": "@party", "status": "open"}},
		{Set: "@party", Cells: map[string]any{"waitlist": "@waitlist"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": student, "member": "yes"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@waitlist", "person": parent, "member": "yes", "added": "2026-09-03 21:12:05"}},
	})
	party, waitlist := made[2], made[3]
	places := `(from MEMBER (where (= group "` + waitlist + `")))`
	for i, viewer := range []string{nobody, student, parent, staff, guest} {
		if got, want := len(as(t, s, viewer, places)), []int{0, 1, 1, 1, 0}[i]; got != want {
			t.Errorf("places on the waitlist as %q: %d, want %d", viewer, got, want)
		}
		if got, want := len(as(t, s, viewer, `(from GROUP (where (= id "`+waitlist+`")))`)), []int{0, 1, 1, 1, 0}[i]; got != want {
			t.Errorf("the waitlist as %q: %d, want %d", viewer, got, want)
		}
	}
	if got := cellAs(t, s, parent, `(from GROUP (where (= id "`+party+`")))`, "waitlist"); got != waitlist {
		t.Errorf("the party's waitlist as the parent = %q", got)
	}
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000006", "person": guest, "member": "yes"}}})
	if n := len(as(t, s, guest, places)); n != 1 {
		t.Errorf("a Who? admin sees %d places on the waitlist, want 1", n)
	}
}

func TestManagersAreTheManagingGroupsMembers(t *testing.T) {
	s, queue := sampleWithQueue(t)
	made := apply(t, s, queue, []Edit{
		{Insert: "GROUP", As: "managers", Row: map[string]any{"posting": "members", "replying": "members", "kind": "group", "name": "Fairs Managers", "status": "open"}},
		{Set: "@managers", Cells: map[string]any{"managed_by": "@managers", "visible_to": "@managers"}},
		{Insert: "GROUP", As: "fairs", Row: map[string]any{"posting": "members", "replying": "members", "kind": "category", "name": "Fairs", "status": "open", "visible_to": "grp00000000004", "managed_by": "@managers"}},
		{Insert: "GROUP", Row: map[string]any{"posting": "members", "replying": "members", "kind": "event", "name": "Book Fair", "parent": "@fairs", "status": "pending", "visible_to": "grp00000000004"}},
		{Insert: "MEMBER", Row: map[string]any{"group": "@managers", "person": parent, "member": "yes"}},
	})
	managers, managing := made[0], made[4]
	fair := `(from GROUP (where (= id "` + made[3] + `")))`
	if n := len(as(t, s, parent, fair)); n != 1 {
		t.Error("a manager of the category can't see a pending event under it")
	}
	if n := len(as(t, s, student, fair)); n != 0 {
		t.Error("someone who manages nothing sees a pending event")
	}
	if n := len(as(t, s, student, `(from MEMBER (where (= group "`+managers+`")))`)); n != 1 {
		t.Errorf("whoever sees the category reads %d of its managers, want 1", n)
	}
	if n := len(as(t, s, student, `(from GROUP (where (= id "`+managers+`")))`)); n != 1 {
		t.Error("whoever sees the category can't read the group that manages it")
	}
	apply(t, s, queue, []Edit{{Set: managers, Cells: map[string]any{"visible_to": ""}}})
	if n := len(as(t, s, student, `(from MEMBER (where (= group "`+managers+`")))`)); n != 0 {
		t.Errorf("hidden managers: whoever sees the category reads %d of them, want 0", n)
	}
	if n := len(as(t, s, parent, `(from MEMBER (where (= group "`+managers+`")))`)); n != 1 {
		t.Errorf("hidden managers: a manager reads %d of them, want 1", n)
	}
	apply(t, s, queue, []Edit{{Set: managing, Cells: map[string]any{"member": "excluded"}}})
	if n := len(as(t, s, parent, fair)); n != 0 {
		t.Error("someone kept out of the managers group still manages")
	}
}

func TestAnAppsAdminsAppointManagers(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Delete: memberRow(t, s, "grp00000000005", staff)},
		{Insert: "APP", Row: map[string]any{"key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"}},
	})
	write := func(viewer string, edits ...Edit) error {
		_, err := Write(context.Background(), s, queue, access.Actor{Email: "maya@example.com"}, Env{Viewer: viewer, Now: testNow}, Batch{Batch: edits})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Set: parent, Cells: map[string]any{"vc_phone": "555-0100"}},
		{Set: staff, Cells: map[string]any{"vc_phone": "555-0200", "phone_override": "555-0300"}},
		{Set: "grp00000000020", Cells: map[string]any{"vc_phone": "555-0400"}},
	})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: "grp00000000020", Cells: map[string]any{"vc_address": "12 Oak St"}}})
	q := `(from GROUP (where (= id "grp00000000020")))`
	if got := cellAs(t, s, student, q, "address"); got != "12 Oak St" {
		t.Fatalf("a shared address reads %q", got)
	}
	if got := cellAs(t, s, student, q, "vc_address"); got != "" {
		t.Fatalf("the imported address behind a shared one reads %q", got)
	}
	apply(t, s, queue, []Edit{{Set: "grp00000000020", Cells: map[string]any{"address_consent": "withheld"}}})
	for _, viewer := range []string{student, parent, staff} {
		for _, column := range []string{"address", "vc_address", "address_override"} {
			if got := cellAs(t, s, viewer, q, column); got != "" {
				t.Errorf("the withheld %s as %s = %q", column, viewer, got)
			}
		}
	}
}

func TestWithheldPeopleVanish(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: student, Cells: map[string]any{"consent": "withheld"}}})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: parent, Cells: map[string]any{"consent": "withheld"}}})
	m := s.Model()
	if got := m.SignedIn("rowan@example.com"); got != "" {
		t.Errorf("a withheld parent signs in as %q", got)
	}
	if got := m.PersonOf("rowan@example.com"); got != parent {
		t.Errorf("the consent import no longer finds the withheld parent: %q", got)
	}
}

func TestAGuestSharesAnAddressButNeverSignsInWithIt(t *testing.T) {
	s, queue := sampleWithQueue(t)
	if _, err := applyErr(s, queue, []Edit{{Insert: "PERSON_EMAIL", Row: map[string]any{"address": "rowan@example.com", "person": guest, "primary": "Yes", "source": "guest"}}}); err != nil {
		t.Fatalf("a guest can't share a person's address: %v", err)
	}
	_, err := applyErr(s, queue, []Edit{{Insert: "PERSON_EMAIL", Row: map[string]any{"address": "rowan@example.com", "person": staff, "source": "manual"}}})
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
	apply(t, s, queue, []Edit{{Set: parent, Cells: map[string]any{"consent": "withheld"}}})
	if got := s.Model().SignedIn("rowan@example.com"); got != "" {
		t.Errorf("a withheld parent signs in as %q through their guest", got)
	}
}

func TestWritesCantReachWithheldRows(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: student, Cells: map[string]any{"consent": "withheld"}}})
	env := Env{Viewer: parent, Now: testNow}
	for name, c := range map[string]struct {
		w    Edit
		want string
	}{
		"set":       {Edit{Set: student, Cells: map[string]any{"name_long_override": "Juni A."}}, "no PERSON " + student},
		"reference": {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": student, "member": "yes"}}, "names no row " + student},
	} {
		_, err := Write(context.Background(), s, queue, access.Actor{Email: "rowan@example.com"}, env, Batch{Batch: []Edit{c.w}})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s on the withheld student: %v, want %q", name, err, c.want)
		}
	}
}

func TestWithheldFamilyNeverAppears(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: "grp00000000020", Cells: map[string]any{"consent": "withheld"}}})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: staff, Cells: map[string]any{"consent": ""}}})
	if _, ok := s.Model().Shown("PERSON").Get(staff); ok {
		t.Fatal("a person with no consent yet is shown")
	}
}

func TestOnlyTheImportSeesWithheldRows(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: student, Cells: map[string]any{"consent": "withheld"}}})
	q := mustParse(t, `(from PERSON (where (= id "per00000000001")))`)
	if n := len(s.Model().Run(t.Context(), q, setupEnv).IDs); n != 1 {
		t.Errorf("the import sees %d withheld students, want 1", n)
	}
	if n := len(s.Model().Run(t.Context(), q, Env{System: "other", Now: testNow}).IDs); n != 0 {
		t.Errorf("another system reader sees %d withheld students, want 0", n)
	}
}

func TestPrivateColumnsNeverShow(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: staff, Cells: map[string]any{"vc_phone": "555-0200"}}})
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
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: memberRow(t, s, "grp00000000040", guest), Cells: map[string]any{"price": "7.25"}}})
	q := `(from MEMBER (where (= person "per00000000004") (= member "yes") (= guest_of "per00000000002")))`
	for viewer, want := range map[string]string{student: "", parent: "7.25", staff: "7.25", guest: "7.25"} {
		if got := cellAs(t, s, viewer, q, "price"); got != want {
			t.Errorf("the guest's price as %s = %q, want %q", viewer, got, want)
		}
	}
	for viewer, want := range map[string]string{parent: "tok00000000001", staff: "tok00000000001"} {
		if got := cellAs(t, s, viewer, `(from RECIPIENT)`, "token"); got != want {
			t.Errorf("the token as %s = %q, want %q", viewer, got, want)
		}
	}
}

func TestHiddenPersonIsUnreachable(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: student, Cells: map[string]any{"hidden": "Yes"}}})
	if n := len(as(t, s, parent, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a hidden person is listed")
	}
	if n := len(as(t, s, parent, `(from MEMBER (where (= id "mem00000000008")))`)); n != 0 {
		t.Fatal("a classroom row naming a hidden person shows")
	}
	if n := len(as(t, s, parent, `(from MEMBER (where (= person.name_short "Juni")))`)); n != 0 {
		t.Fatal("a path reached a hidden person")
	}
	if n := len(as(t, s, parent, `(from GROUP @g (where (exists MEMBER (= group @g) (= person "per00000000001"))))`)); n != 0 {
		t.Fatal("exists probed a hidden person")
	}
	if n := len(as(t, s, student, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a hidden person sees themselves")
	}
	if n := len(as(t, s, staff, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a super admin outside import mode sees a hidden person")
	}
	if n := len(importingAs(t, s, staff, `(from PERSON (where (= id "per00000000001")))`)); n != 1 {
		t.Fatal("a super admin in import mode can't see a hidden person")
	}
	if n := len(importingAs(t, s, parent, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a parent asking for import mode sees a hidden person")
	}
}

func importingAs(t *testing.T, s *Store, viewer, src string) []store.Row {
	t.Helper()
	return s.Model().Run(t.Context(), mustParse(t, src), Env{Viewer: viewer, Mode: "import", Now: testNow}).Rows()
}

func TestPersonFlagsAreKeptFromReaders(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{
		{Set: student, Cells: map[string]any{"deactivated": "2026-09-30 12:00"}},
		{Set: parent, Cells: map[string]any{"signed_out": "2026-09-30 11:00"}},
	})
	juni := `(from PERSON (where (= id "per00000000001")))`
	rowan := `(from PERSON (where (= id "per00000000002")))`
	for _, c := range []struct {
		viewer, query, column, want string
	}{
		{student, juni, "deactivated", ""},
		{staff, juni, "deactivated", "2026-09-30 12:00"},
		{parent, rowan, "signed_out", ""},
		{staff, rowan, "signed_out", "2026-09-30 11:00"},
	} {
		if got := cellAs(t, s, c.viewer, c.query, c.column); got != c.want {
			t.Errorf("%s as %s = %q, want %q", c.column, c.viewer, got, c.want)
		}
	}
}

func TestPendingIsForManagersAndAdmins(t *testing.T) {
	s, queue := sampleWithQueue(t)
	apply(t, s, queue, []Edit{{Set: "grp00000000040", Cells: map[string]any{"status": "pending"}}})
	q := `(from GROUP (where (= id "grp00000000040")))`
	if n := len(as(t, s, parent, q)); n != 0 {
		t.Fatal("a pending event is visible")
	}
	if n := len(as(t, s, staff, q)); n != 1 {
		t.Fatal("a host can't see their pending event")
	}
	apply(t, s, queue, []Edit{{Set: "grp00000000040", Cells: map[string]any{"managed_by": ""}}})
	if n := len(as(t, s, staff, q)); n != 1 {
		t.Fatal("a super admin can't see a pending event")
	}
	apply(t, s, queue, []Edit{{Delete: memberRow(t, s, "grp00000000005", staff)}})
	if n := len(as(t, s, staff, q)); n != 0 {
		t.Fatal("a pending event is visible to someone neither host nor When admin")
	}
	apply(t, s, queue, []Edit{{Insert: "APP", Row: map[string]any{"key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"}}})
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
	s, queue := sampleWithQueue(t)
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
	apply(t, s, queue, []Edit{{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000044", "person": parent, "member": "yes"}}})
	if err := s.Model().Authorize(Env{Viewer: parent, Now: testNow}, change(t, s, "MEMBER", picnic(guest), store.Row{"member": "excluded"})); err == nil {
		t.Fatal("an event's lead may exclude anyone, as if they managed it")
	}
	apply(t, s, queue, []Edit{{Delete: memberRow(t, s, "grp00000000005", staff)}})
	if err := s.Model().Authorize(Env{Viewer: staff, Now: testNow}, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"})); err == nil {
		t.Fatal("someone neither super admin nor When admin may change an event's status")
	}
	apply(t, s, queue, []Edit{{Insert: "APP", Row: map[string]any{"key": "when", "name": "Helios Calendar", "visible_to": "grp00000000004", "admins": "grp00000000003"}}})
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
	if got := ids(`(from GROUP (where (mode "import")))`); len(got) != 0 {
		t.Errorf("a query with no mode holds in import mode: %v", got)
	}
	if _, err := Parse(`(from GROUP @viewer)`); err == nil || !strings.Contains(err.Error(), "can't name a row") {
		t.Errorf("naming a row @viewer: %v", err)
	}
}

func TestTheJSONFormCarriesDefinitionsAndAncestors(t *testing.T) {
	text := `(from GROUP @g (where (in parent (ancestors @g)) (visible @g) (contains name "picnic") (exists MEMBER (= group @g) (in person (household @viewer))) (mode "import")))`
	q, err := Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`["from", "GROUP", {"n": "@g"}, ["where", ["in", {"n": "parent"}, ["ancestors", {"n": "@g"}]], ["visible", {"n": "@g"}], ["contains", {"n": "name"}, "picnic"], ["exists", "MEMBER", ["=", {"n": "group"}, {"n": "@g"}], ["in", {"n": "person"}, ["household", {"n": "@viewer"}]]], ["mode", "import"]]]`)
	back, err := ParseJSON(raw)
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	if back.String() != q.String() {
		t.Errorf("round trip %s, want %s", back.String(), q.String())
	}
}
