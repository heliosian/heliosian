package groups

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/auth"
)

func (h *harness) as(email, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	auth.Fixed(email, h.mux).ServeHTTP(rec, req)
	return rec
}

func (h *harness) excludedRows(name, email string) int {
	n := 0
	for _, row := range h.rows(excludedTab) {
		if row["Group"] == name && row["Email"] == email {
			n++
		}
	}
	return n
}

func TestAMemberOfAVisibleGroupTakesThemselvesOffAndBack(t *testing.T) {
	h := newHarness(t, &fakeStore{raw: []byte(post)})
	const name = "middle-school-parents"
	g := h.cache.Model().Group(name)
	member := ""
	for _, m := range h.members(name) {
		if !g.Manages(m) {
			member = m
			break
		}
	}
	if member == "" {
		t.Fatal("every member manages the group")
	}
	rec := h.as(member, http.MethodGet, "/api/groups/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model answered %d: %s", rec.Code, rec.Body)
	}
	var model struct {
		Groups []groupView `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Groups) != 1 || model.Groups[0].Name != name || model.Groups[0].Mine || !model.Groups[0].Member || model.Groups[0].Unsubscribed || !model.Groups[0].Visible {
		t.Fatalf("a member sees %+v", model.Groups)
	}

	rec = h.as(member, http.MethodPost, "/api/groups/subscription", `{"name":"`+name+`","subscribed":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe answered %d: %s", rec.Code, rec.Body)
	}
	var view groupView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Member || !view.Unsubscribed {
		t.Fatalf("after unsubscribing: member %v, unsubscribed %v", view.Member, view.Unsubscribed)
	}
	g = h.cache.Model().Group(name)
	if !g.HasExcluded(member) || g.Excluded[0].Note != "Unsubscribed by "+loopPage {
		t.Fatalf("not excluded: %+v", g.Excluded)
	}
	if slices.Contains(h.members(name), member) {
		t.Fatal("still a member")
	}
	h.waitFor("the excluded row", func() bool { return h.excludedRows(name, member) == 1 })

	rec = h.as(member, http.MethodPost, "/api/groups/subscription", `{"name":"`+name+`","subscribed":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resubscribe answered %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Member || view.Unsubscribed {
		t.Fatalf("after resubscribing: member %v, unsubscribed %v", view.Member, view.Unsubscribed)
	}
	if h.cache.Model().Group(name).HasExcluded(member) || !slices.Contains(h.members(name), member) {
		t.Fatal("not back on the group")
	}
	h.waitFor("the row to go", func() bool { return h.excludedRows(name, member) == 0 })
	if rec = h.as(member, http.MethodPost, "/api/groups/subscription", `{"name":"`+name+`","subscribed":true}`); rec.Code != http.StatusOK {
		t.Fatalf("a second resubscribe answered %d", rec.Code)
	}

	rec = h.as(member, http.MethodPost, "/api/groups/group", `{"original":"`+name+`","name":"`+name+`","title":"Taken over","managers":["`+member+`"],"rules":[]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a member's save answered %d: %s", rec.Code, rec.Body)
	}
	rec = h.as(member, http.MethodPost, "/api/groups/subscription", `{"name":"soccer-team","subscribed":false}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a group not visible answered %d: %s", rec.Code, rec.Body)
	}
	rec = h.as("mia.torres@heliosschool.org", http.MethodPost, "/api/groups/subscription", `{"name":"`+name+`","subscribed":false}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("someone not on the list answered %d: %s", rec.Code, rec.Body)
	}
	if h.cache.Model().Group(name).HasExcluded("mia.torres@heliosschool.org") {
		t.Fatal("someone not on the list was excluded")
	}
}
