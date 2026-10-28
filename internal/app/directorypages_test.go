package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"heliosian/internal/db"
	"heliosian/internal/workload"
)

func TestOldListAddressesGoToTheGroupPage(t *testing.T) {
	for from, want := range map[string]string{
		"/people?tag=grpAbc":                "/g/grpAbc",
		"/people?list=grpAbc":               "/g/grpAbc",
		"/people?list=party:2j4kq":          "/g/2j4kq",
		"/people?list=room:1st%20%2F%202nd": "/g/1st%20%2F%202nd",
	} {
		rec := httptest.NewRecorder()
		directoryPeople(rec, httptest.NewRequest(http.MethodGet, from, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want %q", from, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}

func TestOldPagePrefixesMoveBeforeSignIn(t *testing.T) {
	c := &Core{Data: &db.Store{Work: workload.New()}, api: http.NewServeMux(), apps: []appSpec{
		{Key: "who", Mux: http.NewServeMux(), Moved: map[string]string{"/families/": "/g/", "/grades/": "/g/", "/groups/": "/g/", "/people/": "/p/"}},
		{Key: "when", Mux: http.NewServeMux(), Moved: map[string]string{"/e/": "/g/"}},
		{Key: "team", Mux: http.NewServeMux(), Moved: map[string]string{"/activities/": "/g/"}, Wrap: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/activities/nsSPomxFcPrSfoRCAgzI" {
					http.Redirect(w, r, "/v/inight", http.StatusFound)
					return
				}
				next.ServeHTTP(w, r)
			})
		}},
	}}
	signIn := func(string, http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })
	}
	handlers := c.Handlers(signIn)
	for _, h := range []struct{ app, from, want string }{
		{"who", "/people/rowan.ashdown", "/p/rowan.ashdown"},
		{"who", "/families/grp00000000020", "/g/grp00000000020"},
		{"who", "/grades/grade-3?tab=x", "/g/grade-3?tab=x"},
		{"who", "/groups/1st%20%2F%202nd", "/g/1st%20%2F%202nd"},
		{"when", "/e/fall-picnic", "/g/fall-picnic"},
		{"team", "/activities/nsSPomxFcPrSfoRCAgzI", "/v/inight"},
		{"team", "/activities/fall", "/g/fall"},
	} {
		rec := httptest.NewRecorder()
		handlers[h.app].ServeHTTP(rec, httptest.NewRequest(http.MethodGet, h.from, nil))
		if (rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusFound) || rec.Header().Get("Location") != h.want {
			t.Errorf("%s %s: %d to %q, want %q", h.app, h.from, rec.Code, rec.Header().Get("Location"), h.want)
		}
	}
	rec := httptest.NewRecorder()
	handlers["when"].ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/g/fall-picnic", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("/g/fall-picnic skipped sign-in: %d", rec.Code)
	}
}
