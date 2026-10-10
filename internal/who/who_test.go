package who

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOldListAddressesGoToTheGroupPage(t *testing.T) {
	for from, want := range map[string]string{
		"/people?tag=grpAbc":                "/groups/grpAbc",
		"/people?list=grpAbc":               "/groups/grpAbc",
		"/people?list=party:2j4kq":          "/groups/2j4kq",
		"/people?list=room:1st%20%2F%202nd": "/groups/1st%20%2F%202nd",
	} {
		rec := httptest.NewRecorder()
		people(rec, httptest.NewRequest(http.MethodGet, from, nil))
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != want {
			t.Errorf("%s: %d to %q, want %q", from, rec.Code, rec.Header().Get("Location"), want)
		}
	}
}
