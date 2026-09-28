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

func requestAs(cache *Cache, email string, hat bool) access.Actor {
	var got access.Actor
	r := httptest.NewRequest("POST", "/", nil)
	if hat {
		r.AddCookie(&http.Cookie{Name: auth.HatCookie, Value: "1"})
	}
	auth.Fixed(email, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = requestActor(cache, r) })).ServeHTTP(httptest.NewRecorder(), r)
	return got
}

func TestEditingAnotherFamilyNeedsAnAdminUnderTheHat(t *testing.T) {
	cache := sampleCache(t)
	model := cache.Model()
	rohans := familyID(testKey, rohan)
	if err := model.mayEdit(requestAs(cache, jordan, true), "family", rohans); err != nil {
		t.Errorf("an admin with the pencil on cannot edit another family: %v", err)
	}
	var refusal *access.Refusal
	if err := model.mayEdit(requestAs(cache, jordan, false), "family", rohans); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("an admin with the pencil off edits another family: %v", err)
	}
	if err := model.mayEdit(requestAs(cache, asha, true), "family", rohans); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("a parent with the cookie set edits another family: %v", err)
	}
	if !requestAs(cache, jordan, false).May(Administer) {
		t.Error("an admin with the pencil off lost Admin Tools")
	}
}

func TestSpoofedParentGetsNoSuperEdit(t *testing.T) {
	cache := sampleCache(t)
	key := []byte("spoof")
	signin := auth.New("", "", key, auth.Login{}, func(string) bool { return true }, nil, nil)
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
		r.AddCookie(&http.Cookie{Name: auth.HatCookie, Value: "1"})
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
