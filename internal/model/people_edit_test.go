package model

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

func sampleCache(t *testing.T) *DirectoryCache {
	t.Helper()
	return newServer(t).cache
}

func requestAs(cache *DirectoryCache, email string) access.Actor {
	var got access.Actor
	r := httptest.NewRequest("POST", "/", nil)
	auth.Fixed(email, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { got = requestActor(cache, r) })).ServeHTTP(httptest.NewRecorder(), r)
	return got
}

func TestEditingAnotherFamilyNeedsAnAdmin(t *testing.T) {
	cache := sampleCache(t)
	model := cache.Model()
	rohans := familyID(testKey, rohan)
	if err := model.mayEdit(requestAs(cache, jordan), "family", rohans); err != nil {
		t.Errorf("an admin cannot edit another family: %v", err)
	}
	var refusal *access.Refusal
	if err := model.mayEdit(requestAs(cache, asha), "family", rohans); !errors.As(err, &refusal) || refusal.Status != http.StatusForbidden {
		t.Errorf("a parent edits another family: %v", err)
	}
	if !requestAs(cache, jordan).May(Administer) {
		t.Error("an admin has no Admin Tools")
	}
}

func TestSpoofedParentCannotEditAnyone(t *testing.T) {
	cache := sampleCache(t)
	key := []byte("spoof")
	signin := auth.New("", "", key, auth.Login{}, func(string) bool { return true }, nil, nil)
	signin.Spoof = &auth.Spoof{
		Allowed: func(email string) bool { return email == jordan },
		Person:  func(email string) (auth.Person, bool) { return auth.Person{Email: email}, true },
	}
	routes := DirectoryRoutes{Cache: cache, MagicTags: func(string) []MagicTag { return nil }}
	for _, c := range []struct {
		as   string
		want bool
	}{{"", true}, {asha, false}} {
		r := httptest.NewRequest("GET", "/api/directory/model", nil)
		if c.as != "" {
			r.AddCookie(&http.Cookie{Name: "spoof", Value: auth.SpoofToken(key, jordan, c.as, time.Now().Add(time.Hour))})
		}
		w := httptest.NewRecorder()
		signin.Fixed(jordan, serve.JSON(routes.model)).ServeHTTP(w, r)
		var view struct {
			User       user `json:"user"`
			EditAnyone bool `json:"editAnyone"`
		}
		if err := json.NewDecoder(w.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
		if view.EditAnyone != c.want {
			t.Errorf("viewing as %q (%s): edit anyone %v, want %v", c.as, view.User.Email, view.EditAnyone, c.want)
		}
	}
}
