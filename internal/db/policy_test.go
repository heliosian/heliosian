package db

import (
	"context"
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
	return s.Model().Run(mustParse(t, src), Env{Viewer: viewer, Now: testNow}).Rows()
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
		strings.Replace(policySource, "(read SETTING (id app key value) true)", "(read SETTING (id app key) true)", 1): "SETTING.value has no read grant",
		policySource + `(read PERSON (vc_phone) (system "import"))`:                                                    "PERSON.vc_phone is private",
		policySource + `(read MEMBER.price true)`:                                                                      "policies are define",
		policySource + `(define (nobody) false)`:                                                                       "nobody is never used",
	} {
		if _, err := compilePolicies(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("compilePolicies = %v, want %q", err, want)
		}
	}
}

func TestWhoSeesWhichRows(t *testing.T) {
	s := sample(t)
	viewers := []string{nobody, student, parent, staff, guest}
	for table, want := range map[string][]int{
		"PERSON":           {3, 3, 4, 4, 4},
		"PERSON_EMAIL":     {4, 4, 4, 4, 4},
		"PERSON_SETTING":   {0, 0, 0, 1, 0},
		"BIRTHDAY_YEAR":    {1, 1, 1, 1, 1},
		"SAVED_VIEW":       {0, 0, 1, 0, 0},
		"GROUP":            {10, 10, 10, 12, 10},
		"MEMBER":           {15, 15, 15, 16, 15},
		"EFFECTIVE_MEMBER": {15, 15, 15, 17, 15},
		"RULE":             {0, 0, 0, 1, 0},
		"GROUP_CATEGORY":   {1, 1, 1, 1, 1},
		"DOCUMENT":         {1, 1, 1, 1, 1},
		"DOCUMENT_GROUP":   {1, 1, 1, 1, 1},
		"INBOX":            {0, 1, 1, 0, 0},
		"MESSAGE":          {0, 0, 1, 1, 0},
		"RECIPIENT":        {0, 0, 1, 1, 0},
		"CATEGORY":         {1, 1, 1, 1, 1},
		"CHARITY":          {1, 1, 1, 1, 1},
		"APP":              {0, 1, 1, 1, 0},
		"ALIAS":            {0, 0, 0, 1, 0},
	} {
		for i, viewer := range viewers {
			if got := len(as(t, s, viewer, "(from "+table+")")); got != want[i] {
				t.Errorf("%s as %q: %d rows, want %d", table, viewer, got, want[i])
			}
		}
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
	if n := len(as(t, s, staff, `(from MEMBER (where (= group "grp00000000010")))`)); n != 0 {
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
	if got := m.signedIn("rowan@example.com"); got != "" {
		t.Errorf("a withheld parent signs in as %q", got)
	}
	if got := m.PersonOf("rowan@example.com"); got != parent {
		t.Errorf("the consent import no longer finds the withheld parent: %q", got)
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
		"reference": {Edit{Insert: "MEMBER", Row: map[string]any{"group": "grp00000000040", "person": student, "role": "member", "status": "yes"}}, "names no row " + student},
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
	if n := len(s.Model().Run(q, Env{System: importReader, Now: testNow}).IDs); n != 1 {
		t.Errorf("the import sees %d withheld students, want 1", n)
	}
	if n := len(s.Model().Run(q, Env{System: "other", Now: testNow}).IDs); n != 0 {
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
	if err := commit(s, GroupsSheet, store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": guest, "role": "member"}, store.Row{"price": "7.25"})); err != nil {
		t.Fatal(err)
	}
	q := `(from MEMBER (where (= group "grp00000000040") (= role "member") (= guest_of "per00000000002")))`
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
	if n := len(as(t, s, guest, `(from PERSON (where (= id "per00000000001")))`)); n != 0 {
		t.Fatal("a hidden person is listed")
	}
	if got := cellAs(t, s, guest, `(from MEMBER (where (= group "grp00000000010")))`, "person"); got != "" {
		t.Fatalf("a classroom row names a hidden person: %q", got)
	}
	if n := len(as(t, s, guest, `(from MEMBER (where (= person.name_short "Juni")))`)); n != 0 {
		t.Fatal("a path reached a hidden person")
	}
	if n := len(as(t, s, guest, `(from GROUP @g (where (exists MEMBER (= group @g) (= person "per00000000001"))))`)); n != 0 {
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
	if err := commit(s, GroupsSheet, store.Delete("MEMBER", store.Row{"group": "grp00000000040", "person": staff, "role": "manager"})); err != nil {
		t.Fatal(err)
	}
	if n := len(as(t, s, staff, q)); n != 0 {
		t.Fatal("a pending event is visible to someone neither host nor When admin")
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "when", "admins": "grp00000000006"})); err != nil {
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
	picnic := func(person string) []string { return []string{"grp00000000040", person, "member"} }
	for _, c := range []struct {
		name   string
		viewer string
		change Change
		ok     bool
	}{
		{"answer for oneself", parent, change(t, s, "MEMBER", picnic(parent), store.Row{"status": "no"}), true},
		{"answer for one's guest", parent, change(t, s, "MEMBER", picnic(guest), store.Row{"status": "maybe"}), true},
		{"answer for a stranger", student, change(t, s, "MEMBER", picnic(parent), store.Row{"status": "no"}), false},
		{"exclude oneself", parent, change(t, s, "MEMBER", picnic(parent), store.Row{"status": "excluded"}), false},
		{"a host excludes", staff, change(t, s, "MEMBER", picnic(parent), store.Row{"status": "excluded"}), true},
		{"answer and change one's note", parent, change(t, s, "MEMBER", picnic(parent), store.Row{"status": "no", "note": "sorry"}), false},
		{"a parent renames a child", parent, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), true},
		{"a guest renames a child", guest, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), false},
		{"Who?'s admin renames anyone", staff, change(t, s, "PERSON", []string{student}, store.Row{"name_long_override": "June Ashdown"}), true},
		{"nobody approves without being an admin", staff, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"}), false},
		{"nobody may add rows yet", staff, Change{Table: "MEMBER", New: store.Row{"group": "grp00000000040", "person": student, "role": "member"}}, false},
		{"nobody may remove rows yet", staff, Change{Table: "MEMBER", Old: store.Row{"group": "grp00000000040", "person": parent, "role": "member"}}, false},
	} {
		err := s.Model().Authorize(Env{Viewer: c.viewer, Now: testNow}, c.change)
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	if err := commit(s, GroupsSheet, store.Update("MEMBER", store.Row{"group": "grp00000000040", "person": parent, "role": "member"}, store.Row{"lead": "Yes"})); err != nil {
		t.Fatal(err)
	}
	if err := s.Model().Authorize(Env{Viewer: parent, Now: testNow}, change(t, s, "MEMBER", picnic(guest), store.Row{"status": "excluded"})); err == nil {
		t.Fatal("an event's lead may set any status, as if they managed it")
	}
	if err := commit(s, ConfigSheet, store.Insert("APP", store.Row{"id": "app00000000002", "key": "when", "admins": "grp00000000006"})); err != nil {
		t.Fatal(err)
	}
	if err := s.Model().Authorize(Env{Viewer: staff, Now: testNow}, change(t, s, "GROUP", []string{"grp00000000040"}, store.Row{"status": "pending"})); err != nil {
		t.Fatalf("a When admin may not change an event's status: %v", err)
	}
}

func TestClientsGetNoPolicyLanguage(t *testing.T) {
	for src, want := range map[string]string{
		`(from GROUP @g (where (in id (ancestors @g))))`: "belongs to policies",
		`(from GROUP (where (system "import")))`:         "belongs to policies",
		`(from GROUP (where (manages id)))`:              "no condition manages",
		`(from GROUP @row)`:                              "is taken",
	} {
		if _, err := Parse(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", src, err, want)
		}
	}
}
