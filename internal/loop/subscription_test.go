package loop

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/store"
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
	h := newHarness(t)
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
	rec := h.as(member, http.MethodGet, "/api/loop/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model answered %d: %s", rec.Code, rec.Body)
	}
	var model struct {
		Groups []groupView `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Groups) != 1 || model.Groups[0].Name != name || model.Groups[0].Mine || !model.Groups[0].Member || model.Groups[0].Unsubscribed || model.Groups[0].Visibility != VisibilityEveryone {
		t.Fatalf("a member sees %+v", model.Groups)
	}

	rec = h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":false}`)
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

	rec = h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":true}`)
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
	if rec = h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":true}`); rec.Code != http.StatusOK {
		t.Fatalf("a second resubscribe answered %d", rec.Code)
	}

	rec = h.as(member, http.MethodPost, "/api/loop/group", `{"original":"`+name+`","name":"`+name+`","title":"Taken over","managers":["`+member+`"],"rules":[]}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a member's save answered %d: %s", rec.Code, rec.Body)
	}
	rec = h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"soccer-team","subscribed":false}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("a group not visible answered %d: %s", rec.Code, rec.Body)
	}
	rec = h.as("mia.torres@heliosschool.org", http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":false}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("someone not on the list answered %d: %s", rec.Code, rec.Body)
	}
	if h.cache.Model().Group(name).HasExcluded("mia.torres@heliosschool.org") {
		t.Fatal("someone not on the list was excluded")
	}
}

func TestTheExcludedListGoesToManagersAlone(t *testing.T) {
	h := newHarness(t)
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
	rec := h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe answered %d: %s", rec.Code, rec.Body)
	}
	var view groupView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Unsubscribed || len(view.Excluded) != 0 {
		t.Fatalf("the member's own answer: unsubscribed %v, excluded %+v", view.Unsubscribed, view.Excluded)
	}
	rec = h.as(member, http.MethodGet, "/api/loop/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("model answered %d: %s", rec.Code, rec.Body)
	}
	var model struct {
		Groups []groupView `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	if len(model.Groups) != 1 || !model.Groups[0].Unsubscribed || len(model.Groups[0].Excluded) != 0 {
		t.Fatalf("a member sees %+v", model.Groups)
	}
	rec = h.as(g.Managers[0], http.MethodGet, "/api/loop/model", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("the manager's model answered %d: %s", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &model); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, mg := range model.Groups {
		if mg.Name == name {
			found = len(mg.Excluded) == 1 && mg.Excluded[0].Email == member && mg.Excluded[0].Note == "Unsubscribed by "+loopPage
		}
	}
	if !found {
		t.Fatalf("the manager sees %+v", model.Groups)
	}
}

func (h *harness) changeLogRow(action, name string) map[string]string {
	for _, row := range h.rows(store.ChangeLogTab) {
		if row["Tab"] == excludedTab && row["Action"] == action && strings.HasPrefix(row["Key"], "Group="+name+";") {
			return row
		}
	}
	return nil
}

func TestTheChangeLogNamesWhoIsReallySignedIn(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	const admin = "admin@heliosschool.org"
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
	key := []byte("key")
	a := auth.New("heliosian.com", "client", key, "", func(string) bool { return true }, nil)
	a.Spoof = &auth.Spoof{
		Allowed: func(email string) bool { return email == admin },
		Person:  func(email string) (auth.Person, bool) { return auth.Person{Email: email}, true },
	}
	req := httptest.NewRequest(http.MethodPost, "/api/loop/subscription", strings.NewReader(`{"name":"`+name+`","subscribed":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "spoof", Value: auth.SpoofToken(key, admin, member, time.Now().Add(time.Hour))})
	rec := httptest.NewRecorder()
	a.Fixed(admin, h.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("the spoofed unsubscribe answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the unsubscribe's log row", func() bool { return h.changeLogRow("insert", name) != nil })
	if row := h.changeLogRow("insert", name); row["Actor"] != member || row["Real Actor"] != admin {
		t.Fatalf("the spoofed unsubscribe logged actor %q, real actor %q", row["Actor"], row["Real Actor"])
	}

	if rec = h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":true}`); rec.Code != http.StatusOK {
		t.Fatalf("the member's resubscribe answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the resubscribe's log row", func() bool { return h.changeLogRow("delete", name) != nil })
	if row := h.changeLogRow("delete", name); row["Actor"] != member || row["Real Actor"] != member {
		t.Fatalf("the member's own resubscribe logged actor %q, real actor %q", row["Actor"], row["Real Actor"])
	}
}

func (h *harness) groupNames(email string) []string {
	rec := h.as(email, http.MethodGet, "/api/loop/model", "")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("model answered %d: %s", rec.Code, rec.Body)
	}
	var model struct {
		Groups []groupView `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &model); err != nil {
		h.t.Fatal(err)
	}
	names := []string{}
	for _, g := range model.Groups {
		names = append(names, g.Name)
	}
	return names
}

func TestAGroupOpenToItsMembersReachesThemAlone(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	g := *h.cache.Model().Group(name)
	member := ""
	for _, m := range h.members(name) {
		if !g.Manages(m) {
			member = m
			break
		}
	}
	const outsider = "mia.torres@heliosschool.org"
	if member == "" || slices.Contains(h.members(name), outsider) {
		t.Fatal("no member who does not manage the group, or the outsider is on it")
	}
	visible := func(to string) {
		t.Helper()
		if err := h.cache.Commit(context.Background(), "test", store.Update(groupsTab, store.Row{"Name": name}, store.Row{visibleColumn: to})); err != nil {
			t.Fatal(err)
		}
	}
	visible(VisibilityMembers)
	if !slices.Equal(h.groupNames(member), []string{name}) {
		t.Fatalf("a member sees %v", h.groupNames(member))
	}
	if len(h.groupNames(outsider)) != 0 {
		t.Fatalf("someone not on the group sees %v", h.groupNames(outsider))
	}
	if rec := h.as(outsider, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":false}`); rec.Code != http.StatusNotFound {
		t.Fatalf("someone not on the group answered %d: %s", rec.Code, rec.Body)
	}
	if rec := h.as(member, http.MethodPost, "/api/loop/subscription", `{"name":"`+name+`","subscribed":false}`); rec.Code != http.StatusOK {
		t.Fatalf("a member's unsubscribe answered %d: %s", rec.Code, rec.Body)
	}
	if !slices.Equal(h.groupNames(member), []string{name}) {
		t.Fatalf("an unsubscribed member sees %v", h.groupNames(member))
	}
	visible(VisibilityHidden)
	if len(h.groupNames(member)) != 0 {
		t.Fatalf("a member sees a hidden group: %v", h.groupNames(member))
	}
}
