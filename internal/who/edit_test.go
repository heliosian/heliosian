package who

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
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
		if got := mayEdit(m, c.me, c.target, c.key); got != c.want {
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
	u := uploader{cache: cache}
	rohans := familyID(testKey, rohan)
	on := httptest.NewRequest("POST", "/", nil)
	on.AddCookie(&http.Cookie{Name: superEditCookie, Value: "1"})
	off := httptest.NewRequest("POST", "/", nil)
	if !u.mayEdit(on, cache.Model(), jordan, "family", rohans) {
		t.Error("an admin with the pencil on cannot edit another family")
	}
	if u.mayEdit(off, cache.Model(), jordan, "family", rohans) {
		t.Error("an admin with the pencil off edits another family")
	}
	if u.mayEdit(on, cache.Model(), asha, "family", rohans) {
		t.Error("a parent with the cookie set edits another family")
	}
}

func TestSettingSuperEditWritesTheCookie(t *testing.T) {
	a := admin{cache: sampleCache(t)}
	for _, enabled := range []bool{true, false} {
		r := httptest.NewRequest("POST", "/api/admin/super-edit", strings.NewReader(`{"enabled":`+map[bool]string{true: "true", false: "false"}[enabled]+`}`))
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		auth.Fixed(jordan, http.HandlerFunc(a.setSuperEdit)).ServeHTTP(w, r)
		if w.Code != http.StatusNoContent {
			t.Fatalf("enabled %v: status %d", enabled, w.Code)
		}
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != superEditCookie || !cookies[0].Secure || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatalf("enabled %v: cookies %+v", enabled, cookies)
		}
		if enabled && (cookies[0].Value != "1" || cookies[0].MaxAge <= 0) || !enabled && cookies[0].MaxAge >= 0 {
			t.Errorf("enabled %v: cookie %+v", enabled, cookies[0])
		}
	}
	w := httptest.NewRecorder()
	auth.Fixed(asha, http.HandlerFunc(a.setSuperEdit)).ServeHTTP(w, httptest.NewRequest("POST", "/api/admin/super-edit", strings.NewReader(`{"enabled":true}`)))
	if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Errorf("a parent turning the pencil on: status %d, cookies %v", w.Code, w.Result().Cookies())
	}
}

func TestSpoofedParentGetsNoSuperEdit(t *testing.T) {
	cache := sampleCache(t)
	key := []byte("spoof")
	signin := auth.New("", "", key, "", func(string) bool { return true }, nil)
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
		signin.Fixed(jordan, http.HandlerFunc(app.model)).ServeHTTP(w, r)
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
