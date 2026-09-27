package admins

import (
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

func TestSavingAdminsKeepsOnlyAddressesThatAreNotSuperAdmins(t *testing.T) {
	listed := []string{}
	l := New(func() []string { return []string{"boss@x.org"} }, func() []string { return listed }, nil)
	admin := access.Actor{Email: "boss@x.org", Admin: true}
	requested := []string{"jsmith", " New.Admin@X.org ", "BOSS@x.org"}
	ops, admins, err := l.set(admin, requested)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(admins, []string{"new.admin@x.org"}) || len(ops) != 1 {
		t.Fatalf("admins %v, ops %v", admins, ops)
	}
	listed = admins
	if again, _, err := l.set(admin, requested); err != nil || len(again) != 0 {
		t.Fatalf("saving the same list again wrote %v (%v)", again, err)
	}
	if ops, _, _ := l.set(admin, nil); len(ops) != 1 || !reflect.DeepEqual(ops[0], store.Delete(Tab, store.Row{"Email": "new.admin@x.org"})) {
		t.Fatalf("emptying the list: %v", ops)
	}
	var refusal *access.Refusal
	if _, _, err := l.set(access.Actor{Email: "someone@x.org"}, requested); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Fatalf("a non-admin saved the list: %v", err)
	}
}

func TestSuperAdminsAreAdminsWhateverTheCase(t *testing.T) {
	l := New(func() []string { return []string{"boss@x.org"} }, func() []string { return []string{"helper@x.org"} }, nil)
	if !l.IsSuperAdmin(" Boss@X.org ") || l.IsSuperAdmin("helper@x.org") {
		t.Fatal("super admin check")
	}
	if !l.IsAdmin("HELPER@x.org") || !l.IsAdmin("boss@x.org") || l.IsAdmin("nobody@x.org") {
		t.Fatal("admin check")
	}
	if got := l.Admins(); !slices.Equal(got, []string{"boss@x.org", "helper@x.org"}) {
		t.Fatalf("admins: %v", got)
	}
}
