package db

import (
	"strings"
	"testing"

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
		if table.Name == "INBOX" {
			continue
		}
		if len(policies.read[table.Name]) == 0 {
			t.Errorf("%s has no read policy", table.Name)
		}
	}
}

func TestWhoSeesWhichRows(t *testing.T) {
	s := sample(t)
	viewers := []string{nobody, student, parent, staff, guest}
	for table, want := range map[string][]int{
		"PERSON":           {3, 3, 4, 4, 4},
		"PERSON_EMAIL":     {3, 3, 3, 3, 3},
		"PERSON_SETTING":   {0, 0, 0, 1, 0},
		"BIRTHDAY_YEAR":    {1, 1, 1, 1, 1},
		"SAVED_VIEW":       {0, 0, 1, 0, 0},
		"GROUP":            {10, 10, 10, 12, 10},
		"MEMBER":           {16, 16, 16, 17, 16},
		"EFFECTIVE_MEMBER": {15, 15, 15, 17, 15},
		"RULE":             {0, 0, 0, 0, 0},
		"GROUP_CATEGORY":   {1, 1, 1, 1, 1},
		"DOCUMENT":         {1, 1, 1, 1, 1},
		"DOCUMENT_GROUP":   {1, 1, 1, 1, 1},
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
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{"phone": "555-0100"})); err != nil {
		t.Fatal(err)
	}
	q := `(from PERSON (where (= id "per00000000002")))`
	for viewer, want := range map[string]string{student: "", parent: "555-0100", staff: "555-0100"} {
		if got := cellAs(t, s, viewer, q, "phone"); got != want {
			t.Errorf("Rowan's phone as %s = %q, want %q", viewer, got, want)
		}
	}
	if n := len(as(t, s, student, `(from PERSON (where (= phone "555-0100")))`)); n != 0 {
		t.Fatal("a withheld phone matched a condition")
	}
}

func TestFamilyAddressFollowsItsAdults(t *testing.T) {
	s := sample(t)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000020"}, store.Row{"vc_address": "12 Oak St"})); err != nil {
		t.Fatal(err)
	}
	q := `(from GROUP (where (= id "grp00000000020")))`
	if got := cellAs(t, s, student, q, "vc_address"); got != "12 Oak St" {
		t.Fatalf("a shared address reads %q", got)
	}
	if err := commit(s, PeopleSheet, store.Update("PERSON", store.Row{"id": parent}, store.Row{"share_address": "No"})); err != nil {
		t.Fatal(err)
	}
	for viewer, want := range map[string]string{student: "", parent: "12 Oak St", staff: ""} {
		if got := cellAs(t, s, viewer, q, "vc_address"); got != want {
			t.Errorf("the address as %s = %q, want %q", viewer, got, want)
		}
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

func TestPendingIsForLeadsAndAdmins(t *testing.T) {
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
	if err := commit(s, GroupsSheet, store.Delete("MEMBER", store.Row{"group": "grp00000000040", "person": staff, "role": "lead"})); err != nil {
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
		`(from GROUP (where (leads id)))`:                "no condition leads",
		`(from GROUP @row)`:                              "is taken",
	} {
		if _, err := Parse(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", src, err, want)
		}
	}
}
