package db

import (
	"slices"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	s := sample(t)
	run := func(viewer, src string) Result {
		t.Helper()
		return s.Model().Run(t.Context(), mustParse(t, src), Env{Viewer: viewer, Now: testNow})
	}
	if got := run(parent, `(from GROUP (where (= id (slug "Hummingbirds"))))`); got.Err != nil || !slices.Equal(got.IDs, []string{"grp00000000010"}) {
		t.Errorf("the hummingbirds slug found %v, %v", got.IDs, got.Err)
	}
	bySlug := run(parent, `(from PERSON (where (in id (members (slug "hummingbirds") "parents"))))`)
	byID := run(parent, `(from PERSON (where (in id (members "grp00000000010" "parents"))))`)
	if bySlug.Err != nil || len(bySlug.IDs) == 0 || !slices.Equal(bySlug.IDs, byID.IDs) {
		t.Errorf("members by slug %v, %v; by id %v", bySlug.IDs, bySlug.Err, byID.IDs)
	}
	for _, c := range []struct{ viewer, slug string }{{parent, "nope"}, {guest, "hummingbirds"}} {
		got := run(c.viewer, `(from GROUP @e (where (covers @e (members (slug "`+c.slug+`")))))`)
		if got.Err == nil || !strings.Contains(got.Err.Error(), `no group has the slug "`+c.slug+`"`) || len(got.IDs) != 0 {
			t.Errorf("as %s the slug %s answered %v, %v", c.viewer, c.slug, got.IDs, got.Err)
		}
	}
	if got := run(staff, `(from GROUP (where (= id (slug "super-admins"))))`); got.Err != nil || len(got.IDs) != 1 {
		t.Errorf("a super admin's slug super-admins answered %v, %v", got.IDs, got.Err)
	}
	if got := run(guest, `(from PERSON (where (= id @viewer) (admin_of "who")))`); got.Err != nil || len(got.IDs) != 0 {
		t.Errorf("a guest asking whether they are a Who admin answered %v, %v", got.IDs, got.Err)
	}
	if got := run(parent, `(from GROUP (where (manages (slug "nope"))))`); got.Err == nil || !strings.Contains(got.Err.Error(), `no group has the slug "nope"`) {
		t.Errorf("a missing slug passed to a define answered %v, %v", got.IDs, got.Err)
	}
	for src, want := range map[string]string{
		`(from GROUP (where (= id (slug hummingbirds))))`:           "slug takes quoted slugs",
		`(from GROUP (where (= id (slug "hummingbirds" "staff"))))`: "slug takes one quoted slug where it stands for one group",
	} {
		if _, err := Parse(src); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", src, err, want)
		}
	}
	roles := run(parent, `(from EFFECTIVE_MEMBER (where (in group (slug "students" "staff"))) (columns group))`)
	bySelect := run(parent, `(from EFFECTIVE_MEMBER (where (in group (select GROUP.id (in slug "students" "staff")))) (columns group))`)
	slices.Sort(roles.IDs)
	slices.Sort(bySelect.IDs)
	if roles.Err != nil || len(roles.IDs) == 0 || !slices.Equal(roles.IDs, bySelect.IDs) {
		t.Errorf("memberships by two slugs %v, %v; by a select %v", roles.IDs, roles.Err, bySelect.IDs)
	}
	united := run(parent, `(from PERSON (where (in id (members (slug "students" "staff")))))`)
	if united.Err != nil || len(united.IDs) < 2 {
		t.Errorf("the members of two slugs are %v, %v", united.IDs, united.Err)
	}
	if got := run(parent, `(from EFFECTIVE_MEMBER (where (in group (slug "students" "nope"))))`); got.Err == nil || !strings.Contains(got.Err.Error(), `no group has the slug "nope"`) {
		t.Errorf("a missing slug among several answered %v, %v", got.IDs, got.Err)
	}
}
