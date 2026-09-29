package loop

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

func (h *harness) as(email, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	auth.Fixed(email, h.mux).ServeHTTP(rec, req)
	return rec
}

type reply struct {
	Result    json.RawMessage                      `json:"result"`
	Resources map[string]map[string]map[string]any `json:"resources"`
}

func (h *harness) get(as, path string) reply {
	h.t.Helper()
	rec := h.as(as, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET %s as %s: %d %s", path, as, rec.Code, rec.Body)
	}
	var out reply
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		h.t.Fatalf("GET %s: %v", path, err)
	}
	return out
}

func (h *harness) want(as, method, path, body string, code int) {
	h.t.Helper()
	if rec := h.as(as, method, path, body); rec.Code != code {
		h.t.Fatalf("%s %s as %s: %d %s, want %d", method, path, as, rec.Code, rec.Body, code)
	}
}

func (r reply) ids(t *testing.T) []string {
	t.Helper()
	out := []string{}
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func (r reply) id(t *testing.T) string {
	t.Helper()
	var out string
	if err := json.Unmarshal(r.Result, &out); err != nil {
		t.Fatalf("result %s: %v", r.Result, err)
	}
	return out
}

func (r reply) email(of map[string]any) string {
	if email, ok := of["email"].(string); ok {
		return email
	}
	person, _ := of["person"].(string)
	email, _ := r.Resources["people"][person]["email"].(string)
	return email
}

func strings2(v any) []string {
	out := []string{}
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}

func (h *harness) list(as, groupID string) map[string]any {
	h.t.Helper()
	return h.get(as, "/api/email-lists/"+groupID).Resources["email-lists"][groupID]
}

func me(list map[string]any) map[string]any {
	return list["me"].(map[string]any)
}

func allowed(list map[string]any) map[string]any {
	return list["can"].(map[string]any)
}

func (h *harness) excludedRows(groupID, email string) int {
	n := 0
	for _, row := range h.rows(excludedTab) {
		if row["Group"] == groupID && row["Email"] == email {
			n++
		}
	}
	return n
}

func (h *harness) plainMember(name string) string {
	h.t.Helper()
	g := h.cache.Model().Named(name)
	for _, m := range h.members(name) {
		if !g.Manages(m) {
			return m
		}
	}
	h.t.Fatal("every member manages the group")
	return ""
}

func TestAMemberOfAVisibleGroupTakesThemselvesOffAndBack(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	member := h.plainMember(name)
	if got := h.get(member, "/api/email-lists").ids(t); !slices.Equal(got, []string{middleID}) {
		t.Fatalf("a member lists %v", got)
	}
	list := h.list(member, middleID)
	if list["name"] != name || list["visibility"] != VisibilityEveryone || me(list)["managing"] != false || me(list)["member"] != true || me(list)["unsubscribed"] != false {
		t.Fatalf("a member sees %+v", list)
	}
	if allowed(list)["unsubscribe"] != true || allowed(list)["resubscribe"] != false || allowed(list)["edit"] != false || allowed(list)["delete"] != false {
		t.Fatalf("a member may %+v", allowed(list))
	}

	h.want(member, http.MethodPost, "/api/email-lists/"+middleID+"/unsubscribe", "", http.StatusNoContent)
	g := h.cache.Model().Group(middleID)
	if !g.HasExcluded(member) || g.Excluded[0].Note != "Unsubscribed by "+loopPage {
		t.Fatalf("not excluded: %+v", g.Excluded)
	}
	if slices.Contains(h.members(name), member) {
		t.Fatal("still a member")
	}
	if list = h.list(member, middleID); me(list)["member"] != true || me(list)["unsubscribed"] != true || allowed(list)["resubscribe"] != true || allowed(list)["unsubscribe"] != false {
		t.Fatalf("after unsubscribing: %+v", list)
	}
	h.waitFor("the excluded row", func() bool { return h.excludedRows(middleID, member) == 1 })

	h.want(member, http.MethodPost, "/api/email-lists/"+middleID+"/resubscribe", "", http.StatusNoContent)
	if h.cache.Model().Group(middleID).HasExcluded(member) || !slices.Contains(h.members(name), member) {
		t.Fatal("not back on the group")
	}
	h.waitFor("the row to go", func() bool { return h.excludedRows(middleID, member) == 0 })
	h.want(member, http.MethodPost, "/api/email-lists/"+middleID+"/resubscribe", "", http.StatusBadRequest)

	h.want(member, http.MethodPost, "/api/email-lists/"+middleID+"/edit", `{"title":"Taken over"}`, http.StatusForbidden)
	h.want(member, http.MethodDelete, "/api/email-lists/"+middleID, "", http.StatusForbidden)
	h.want(member, http.MethodPost, "/api/email-lists/"+soccerID+"/unsubscribe", "", http.StatusNotFound)
	h.want("mia.torres@heliosschool.org", http.MethodPost, "/api/email-lists/"+middleID+"/unsubscribe", "", http.StatusForbidden)
	if h.cache.Model().Group(middleID).HasExcluded("mia.torres@heliosschool.org") {
		t.Fatal("someone not on the list was excluded")
	}
}

func TestTheExcludedListAndTheReasonsGoToManagersAlone(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	g := h.cache.Model().Named(name)
	member := h.plainMember(name)
	h.want(member, http.MethodPost, "/api/email-lists/"+g.ID+"/unsubscribe", "", http.StatusNoContent)
	list := h.list(member, g.ID)
	if me(list)["unsubscribed"] != true || list["excluded"] != nil || list["rules"] != nil || list["additions"] != nil {
		t.Fatalf("a member sees %+v", list)
	}
	list = h.list(g.Managers[0], g.ID)
	excluded, _ := list["excluded"].([]any)
	if len(excluded) != 1 || excluded[0].(map[string]any)["email"] != member || excluded[0].(map[string]any)["note"] != "Unsubscribed by "+loopPage || len(list["rules"].([]any)) == 0 {
		t.Fatalf("the manager sees %+v", list)
	}
	for as, reasons := range map[string]bool{"mia.torres@heliosschool.org": false, g.Managers[0]: true} {
		out := h.get(as, "/api/email-lists/"+g.ID+"?include=members.person")
		keys := strings2(out.Resources["email-lists"][g.ID]["members"])
		if len(keys) != len(h.members(name)) || float64(len(keys)) != out.Resources["email-lists"][g.ID]["memberCount"] {
			t.Fatalf("%s sees %d members of %d", as, len(keys), len(h.members(name)))
		}
		for _, key := range keys {
			m := out.Resources["email-list-members"][key]
			if _, has := m["reasons"]; has != reasons {
				t.Fatalf("%s sees reasons %v on %+v", as, has, m)
			}
			if email := out.email(m); email == "" || email == member {
				t.Fatalf("member %+v reads as %q", m, email)
			}
		}
	}
}

func TestAnEditSavesOnlyTheFieldsItSends(t *testing.T) {
	h := newHarness(t)
	g := *h.cache.Model().Group(soccerID)
	manager := g.Managers[0]
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"title":"Soccer Families","visibility":"members"}`, http.StatusNoContent)
	after := h.cache.Model().Group(soccerID)
	if after.Title != "Soccer Families" || after.Visibility != VisibilityMembers || after.Description != g.Description || !slices.EqualFunc(after.Rules, g.Rules, sameRule) || len(after.Additions) != len(g.Additions) || !slices.Equal(after.Managers, g.Managers) {
		t.Fatalf("the edit left %+v from %+v", after, g)
	}
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"name":"soccer-two"}`, http.StatusBadRequest)
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"managers":[]}`, http.StatusBadRequest)
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/edit", `{"managers":["`+manager+`","ruth.amari@heliosschool.org"]}`, http.StatusNoContent)
	if managers := h.cache.Model().Group(soccerID).Managers; !slices.Equal(managers, []string{manager, "ruth.amari@heliosschool.org"}) {
		t.Fatalf("managers %v", managers)
	}
	if list := h.list("ruth.amari@heliosschool.org", soccerID); me(list)["managing"] != true || allowed(list)["edit"] != true {
		t.Fatalf("a new manager sees %+v", list)
	}
}

func (h *harness) changeLogRow(action, groupID string) map[string]string {
	for _, row := range h.rows(store.ChangeLogTab) {
		if row["Tab"] == excludedTab && row["Action"] == action && strings.HasPrefix(row["Key"], "Group="+groupID+";") {
			return row
		}
	}
	return nil
}

func TestTheChangeLogNamesWhoIsReallySignedIn(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	const admin = "admin@heliosschool.org"
	g := h.cache.Model().Named(name)
	member := h.plainMember(name)
	key := []byte("key")
	a := auth.New("heliosian.com", "client", key, auth.Login{}, func(string) bool { return true }, nil, nil)
	a.Spoof = &auth.Spoof{
		Allowed: func(email string) bool { return email == admin },
		Person:  func(email string) (auth.Person, bool) { return auth.Person{Email: email}, true },
	}
	req := httptest.NewRequest(http.MethodPost, "/api/email-lists/"+g.ID+"/unsubscribe", nil)
	req.AddCookie(&http.Cookie{Name: "spoof", Value: auth.SpoofToken(key, admin, member, time.Now().Add(time.Hour))})
	rec := httptest.NewRecorder()
	a.Fixed(admin, h.mux).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the spoofed unsubscribe answered %d: %s", rec.Code, rec.Body)
	}
	h.waitFor("the unsubscribe's log row", func() bool { return h.changeLogRow("insert", g.ID) != nil })
	if row := h.changeLogRow("insert", g.ID); row["Actor"] != member || row["Real Actor"] != admin {
		t.Fatalf("the spoofed unsubscribe logged actor %q, real actor %q", row["Actor"], row["Real Actor"])
	}
	h.want(member, http.MethodPost, "/api/email-lists/"+g.ID+"/resubscribe", "", http.StatusNoContent)
	h.waitFor("the resubscribe's log row", func() bool { return h.changeLogRow("delete", g.ID) != nil })
	if row := h.changeLogRow("delete", g.ID); row["Actor"] != member || row["Real Actor"] != member {
		t.Fatalf("the member's own resubscribe logged actor %q, real actor %q", row["Actor"], row["Real Actor"])
	}
}

func (h *harness) groupNames(email string) []string {
	out := h.get(email, "/api/email-lists")
	names := []string{}
	for _, key := range out.ids(h.t) {
		names = append(names, out.Resources["email-lists"][key]["name"].(string))
	}
	return names
}

func TestAGroupOpenToItsMembersReachesThemAlone(t *testing.T) {
	h := newHarness(t)
	const name = "middle-school-parents"
	g := *h.cache.Model().Named(name)
	member := h.plainMember(name)
	const outsider = "mia.torres@heliosschool.org"
	if slices.Contains(h.members(name), outsider) {
		t.Fatal("the outsider is on the group")
	}
	visible := func(to string) {
		t.Helper()
		if err := h.cache.Commit(context.Background(), access.System("test"), store.Update(groupsTab, store.Row{idColumn: g.ID}, store.Row{visibleColumn: to})); err != nil {
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
	h.want(outsider, http.MethodPost, "/api/email-lists/"+g.ID+"/unsubscribe", "", http.StatusNotFound)
	h.want(member, http.MethodPost, "/api/email-lists/"+g.ID+"/unsubscribe", "", http.StatusNoContent)
	if !slices.Equal(h.groupNames(member), []string{name}) {
		t.Fatalf("an unsubscribed member sees %v", h.groupNames(member))
	}
	visible(VisibilityHidden)
	if len(h.groupNames(member)) != 0 {
		t.Fatalf("a member sees a hidden group: %v", h.groupNames(member))
	}
}

func TestAManagerSeesTheirHiddenGroupAndArchivesIt(t *testing.T) {
	h := newHarness(t)
	manager := h.cache.Model().Group(soccerID).Managers[0]
	if !slices.Contains(h.groupNames(manager), "soccer-team") {
		t.Fatalf("the manager's own hidden group is missing from %v", h.groupNames(manager))
	}
	list := h.list(manager, soccerID)
	if me(list)["managing"] != true || me(list)["archived"] != false || allowed(list)["archive"] != true || allowed(list)["unarchive"] != false || allowed(list)["edit"] != true {
		t.Fatalf("the manager sees %+v", list)
	}
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/archive", "", http.StatusNoContent)
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/archive", "", http.StatusBadRequest)
	if list = h.list(manager, soccerID); me(list)["archived"] != true || allowed(list)["unarchive"] != true {
		t.Fatalf("after archiving %+v", list)
	}
	h.want(manager, http.MethodPost, "/api/email-lists/"+soccerID+"/unarchive", "", http.StatusNoContent)
	if h.cache.Model().Archived(soccerID, manager) {
		t.Fatal("still archived")
	}
}

func TestTheSettingsNameTheDomainAndTheViewer(t *testing.T) {
	h := newHarness(t)
	const viewer = "jordan.whitfield@heliosschool.org"
	out := h.get(viewer, "/api/loop-settings?include=viewer")
	keys := out.ids(t)
	if len(keys) != 1 {
		t.Fatalf("settings %v", keys)
	}
	s := out.Resources["loop-settings"][keys[0]]
	if s["domain"] != Domain || len(s["roles"].([]any)) == 0 || len(s["relations"].([]any)) == 0 {
		t.Fatalf("settings %+v", s)
	}
	if person := out.Resources["people"][s["viewer"].(string)]; person["email"] != viewer {
		t.Fatalf("the viewer is %+v", person)
	}
	if again := h.get("ruth.amari@heliosschool.org", "/api/loop-settings/"+keys[0]); again.id(t) != keys[0] {
		t.Fatal("the settings are not one resource")
	}
}

func TestASuggestionIsMineWhenIHostItOrWhatItSitsUnder(t *testing.T) {
	h := newHarness(t)
	const viewer, other = "jordan.whitfield@heliosschool.org", "ruth.amari@heliosschool.org"
	lists := []model.MagicTag{
		{Key: "activity:run", Kind: model.MagicTagActivity, Hosts: []string{viewer}},
		{Key: "activity:under", Kind: model.MagicTagActivity, Parent: "activity:run", Hosts: []string{other}},
		{Key: "activity:theirs", Kind: model.MagicTagActivity, Hosts: []string{other}},
	}
	w := NewWorld(h.cache.Model(), h.sources().Directory, nil, func(string, time.Time) []model.MagicTag { return lists }, func() []string { return nil })
	mine := map[string]bool{}
	for _, s := range w.suggestions(viewer) {
		mine[s.key] = s.mine
	}
	if want := map[string]bool{"tag:dtg0000000001": true, "activity:run": true, "activity:under": true, "activity:theirs": false}; !maps.Equal(mine, want) {
		t.Fatalf("mine = %v, want %v", mine, want)
	}
}

func TestSuggestionsAreTheViewersOwn(t *testing.T) {
	h := newHarness(t)
	for _, email := range []string{"jordan.whitfield@heliosschool.org", "ruth.amari@heliosschool.org"} {
		out := h.get(email, "/api/email-list-suggestions?include=managers")
		for _, key := range out.ids(t) {
			s := out.Resources["email-list-suggestions"][key]
			managers := strings2(s["managers"])
			if len(managers) == 0 || out.Resources["people"][managers[0]]["email"] != email {
				t.Fatalf("%s's suggestion %+v starts with %v", email, s, managers)
			}
			if s["kind"] == SuggestionTag {
				h.want("mia.torres@heliosschool.org", http.MethodGet, "/api/email-list-suggestions/"+key, "", http.StatusNotFound)
			}
		}
	}
}
