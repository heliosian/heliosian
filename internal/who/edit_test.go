package who

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/serve"
)

const (
	asha   = "asha.chandra@heliosschool.org"
	rohan  = "rohan.chandra@heliosschool.org"
	dev    = "dev.chandra@heliosschool.org"
	elena  = "elena.torres@heliosschool.org"
	marco  = "marco.torres@heliosschool.org"
	jordan = "jordan.whitfield@heliosschool.org"
)

func TestAParentEditsTheirHouseholdAndKidButNotTheOtherParents(t *testing.T) {
	m := sampleModel(t)
	ashas, rohans := familyID(testKey, asha), familyID(testKey, rohan)
	for _, c := range []struct {
		me, target, key string
		want            bool
	}{
		{asha, "family", ashas, true},
		{asha, "family", rohans, false},
		{asha, "person", asha, true},
		{asha, "person", dev, true},
		{asha, "person", rohan, false},
		{rohan, "family", rohans, true},
		{rohan, "family", ashas, false},
		{rohan, "person", dev, true},
		{rohan, "person", asha, false},
		{dev, "person", dev, true},
		{dev, "person", asha, false},
		{dev, "family", ashas, false},
		{dev, "family", rohans, false},
		{elena, "person", marco, true},
		{elena, "person", dev, false},
		{"nobody@example.org", "person", "nobody@example.org", false},
	} {
		if got := m.familyMayEdit(c.me, c.target, c.key); got != c.want {
			t.Errorf("%s editing %s %s: %v, want %v", c.me, c.target, c.key, got, c.want)
		}
	}
}

func sampleCache(t *testing.T) *Cache {
	t.Helper()
	return newServer(t).cache
}

func TestSuperEditIsAnAdminsCookie(t *testing.T) {
	cache := sampleCache(t)
	model := cache.Model()
	rohans := familyID(testKey, rohan)
	on := httptest.NewRequest("POST", "/", nil)
	on.AddCookie(&http.Cookie{Name: superEditCookie, Value: "1"})
	off := httptest.NewRequest("POST", "/", nil)
	if err := model.mayEdit(actorOf(cache, jordan), superEditOn(on), "family", rohans); err != nil {
		t.Errorf("an admin with the pencil on cannot edit another family: %v", err)
	}
	var refusal *access.Refusal
	if err := model.mayEdit(actorOf(cache, jordan), superEditOn(off), "family", rohans); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("an admin with the pencil off edits another family: %v", err)
	}
	if err := model.mayEdit(actorOf(cache, asha), superEditOn(on), "family", rohans); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("a parent with the cookie set edits another family: %v", err)
	}
}

func TestSpoofedParentGetsNoSuperEdit(t *testing.T) {
	cache := sampleCache(t)
	key := []byte("spoof")
	signin := auth.New("", "", key, auth.Login{}, func(string) bool { return true }, nil)
	signin.Spoof = &auth.Spoof{
		Allowed: func(email string) bool { return email == jordan },
		Person:  func(email string) (auth.Person, bool) { return auth.Person{Email: email}, true },
	}
	app := app{cache: cache, lister: noLists{}}
	for _, c := range []struct {
		as   string
		want bool
	}{{"", true}, {asha, false}} {
		r := httptest.NewRequest("GET", "/api/directory/model", nil)
		r.AddCookie(&http.Cookie{Name: superEditCookie, Value: "1"})
		if c.as != "" {
			r.AddCookie(&http.Cookie{Name: "spoof", Value: auth.SpoofToken(key, jordan, c.as, time.Now().Add(time.Hour))})
		}
		w := httptest.NewRecorder()
		signin.Fixed(jordan, serve.JSON(app.model)).ServeHTTP(w, r)
		var view struct {
			User      user `json:"user"`
			SuperEdit bool `json:"superEdit"`
		}
		if err := json.NewDecoder(w.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
		if view.SuperEdit != c.want {
			t.Errorf("viewing as %q (%s): super edit %v, want %v", c.as, view.User.Email, view.SuperEdit, c.want)
		}
	}
}

type noLists struct{}

func (noLists) Lists(string) []List { return nil }
