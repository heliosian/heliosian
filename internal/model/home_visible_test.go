package model

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/id"
	"heliosian/internal/imagesearch"
	"heliosian/internal/store"
	"heliosian/internal/testkit"
	"heliosian/internal/testkit/mailtest"
)

type homeSheet struct {
	dir   *data.Dir
	queue *store.Queue
}

func (s homeSheet) rows(t *testing.T, tab string) []store.Row {
	t.Helper()
	return testkit.Rows(t, s.dir, s.queue, homeAppName, tab)
}

func sampleHomeCache(t *testing.T) (*Store, homeSheet) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	outsideSuperAdmin(t, dir)
	deps := sampleDeps(sampleKey)
	deps.Objects = blob.NewMemoryBucket()
	deps.Embedder = vertex(t)
	return sampleStore(t, dir, queue, deps), homeSheet{dir: dir, queue: queue}
}

func homeCalendar(s *Store, sheet homeSheet) CalendarHooks {
	return RegisterCalendar(http.NewServeMux(), CalendarDeps{
		Store:  s,
		Images: memoryImages(),
		Mail:   CalendarMail{Sender: mailtest.Discard()},
		Style:  testStyle,
		Queue:  sheet.queue,
		IDKey:  sampleKey,
	})
}

func homeServer(t *testing.T) (*Store, homeSheet, *http.ServeMux) {
	t.Helper()
	c, s := sampleHomeCache(t)
	return c, s, homeOver(t, c, s)
}

func homeOver(t *testing.T, c *Store, s homeSheet) *http.ServeMux {
	t.Helper()
	bucket := blob.NewMemoryBucket()
	images := blob.New(bucket)
	mux := http.NewServeMux()
	home := RegisterHome(mux, HomeDeps{
		Store:  c,
		Images: blob.NewImages(images, "home"),
		Search: imagesearch.Search{Stock: imagesearch.NewStock(bucket, images), Limits: imagesearch.NewLimits()},
	})
	typedRegistry(c, s.queue, DirectoryResources(c), homeCalendar(c, s).Resources(), home.Resources(), MagicTagResources()).Register(mux)
	return mux
}

func appKey(key string) string {
	return id.Of(sampleKey, kindApp, key)
}

func widgetKey(name string) string {
	return id.Of(sampleKey, kindHomeWidget, name)
}

func homeLog(t *testing.T, s homeSheet, since int) []string {
	t.Helper()
	out := []string{}
	for _, row := range homeChangeLog(t, s)[since:] {
		out = append(out, row["Actor"]+"|"+row["Action"]+"|"+row["Tab"]+"|"+row["Key"]+"|"+row["Column"])
	}
	return out
}

func TestAppsAsResources(t *testing.T) {
	c, s, mux := homeServer(t)
	apps := func(as string) []map[string]any { return listOf(t, mux, as, "/api/apps") }
	keys := func(list []map[string]any) []string {
		out := []string{}
		for _, a := range list {
			out = append(out, a["key"].(string))
		}
		return out
	}
	listed := func(list []map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, a := range list {
			out[a["key"].(string)] = a["me"].(map[string]any)["listed"] == true
		}
		return out
	}
	parent := apps("jordan.whitfield@heliosschool.org")
	if got := keys(parent); !slices.Equal(got, []string{"home", "who", "team", "celebrate", "birthday", "when", "loop", "ask"}) {
		t.Errorf("apps in order = %v", got)
	}
	if l := listed(parent); !l["home"] || !l["who"] || !l["celebrate"] || !l["birthday"] {
		t.Errorf("the sample parent's apps = %v", l)
	}
	if l := listed(apps("sam.whitfield@heliosschool.org")); !l["who"] || l["celebrate"] || l["birthday"] {
		t.Errorf("a student's apps = %v", l)
	}
	for _, a := range apps("robin.whitfield@heliosschool.org") {
		if a["visibility"] != nil || a["emails"] != nil || a["rules"] != nil {
			t.Errorf("a member sees %s's list: %v", a["key"], a)
		}
		if can(a, "edit") {
			t.Errorf("a member can edit %s", a["key"])
		}
	}
	admin := read(t, mux, homeAdmin, "/api/apps/celebrate").one(t, "apps", appKey("celebrate"))
	if admin["visibility"] != VisibleToList || len(admin["emails"].([]any)) != 2 || len(admin["rules"].([]any)) != 1 || !can(admin, "edit") || admin["order"] != "6" || admin["name"] != "Helios Celebrate" {
		t.Errorf("an admin's celebrate = %v", admin)
	}
	if home := read(t, mux, homeAdmin, "/api/apps/home").one(t, "apps", appKey("home")); can(home, "edit") || home["name"] != HomeApp.Name {
		t.Errorf("the home app = %v", home)
	}
	if rec := testkit.Call(t, mux, "robin.whitfield@heliosschool.org", "POST", "/api/apps/celebrate/edit", map[string]any{"order": "1"}); rec.Code != http.StatusForbidden {
		t.Errorf("a member's edit: %d", rec.Code)
	}
	before := len(homeChangeLog(t, s))
	write(t, mux, homeAdmin, "POST", "/api/apps/ask/edit", map[string]any{"order": "1"})
	if got := homeLog(t, s, before); !slices.Equal(got, []string{homeAdmin + "|set|" + homeVisibilityTab + "|App=ask|" + store.OrderColumn}) {
		t.Errorf("an order alone wrote %v", got)
	}
	if got := keys(apps(homeAdmin)); got[1] != "ask" {
		t.Errorf("apps after the move = %v", got)
	}
	before = len(homeChangeLog(t, s))
	write(t, mux, homeAdmin, "POST", "/api/apps/who/edit", map[string]any{"tagline": "Find anyone"})
	if got := homeLog(t, s, before); !slices.Equal(got, []string{homeAdmin + "|set|" + homeVisibilityTab + "|App=who|Tagline"}) || c.Model().Home.AppList()[1].Tagline != "Find anyone" {
		t.Errorf("a tagline alone wrote %v", got)
	}
	for _, bad := range []map[string]any{{"order": ""}, {"order": "a b"}, {"visibility": "nobody"}, {"name": ""}} {
		if rec := testkit.Call(t, mux, homeAdmin, "POST", "/api/apps/who/edit", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("edit %v: %d", bad, rec.Code)
		}
	}
	write(t, mux, homeAdmin, "POST", "/api/apps/celebrate/edit", map[string]any{"visibility": VisibleToEveryone})
	if l := listed(apps("sam.whitfield@heliosschool.org")); !l["celebrate"] {
		t.Errorf("a student's apps with the celebration everyone's = %v", l)
	}
}

func TestDiscoverKeysWhatIsMissing(t *testing.T) {
	c, s := sampleHomeCache(t)
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Delete(homeVisibilityTab, store.Row{"App": "ask"}), store.Delete(homeWidgetsTab, store.Row{"Widget": "celebrate"})); err != nil {
		t.Fatal(err)
	}
	homeOver(t, c, s)
	home := c.Model().Home
	ask, ok := home.Visibility["ask"]
	if !ok || ask.Mode != VisibleToList || len(ask.Emails) != 0 {
		t.Fatalf("the rediscovered app = %+v", ask)
	}
	for key, v := range home.Visibility {
		if key != "ask" && store.CompareKeys(v.Order, ask.Order) >= 0 {
			t.Errorf("the new app's key %q is not after %s's %q", ask.Order, key, v.Order)
		}
	}
	if got := home.WidgetOrder; !slices.Equal(got, []string{"when", "todo", "team", "school", "birthday", "celebrate"}) {
		t.Errorf("widgets after the new row = %v", got)
	}
	for _, row := range s.rows(t, homeWidgetsTab) {
		if _, err := checkOrder(row[store.OrderColumn]); err != nil {
			t.Errorf("a widget row without a key: %v", row)
		}
	}
}

func setAppVisibility(t *testing.T, c *Store, app string, v AppVisibilityRow) {
	t.Helper()
	if v.Order == "" {
		v.Order = c.Model().Home.Visibility[app].Order
	}
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Upsert(homeVisibilityTab, store.Row{"App": app}, v.cells())); err != nil {
		t.Fatalf("set %s: %v", app, err)
	}
}

func TestVisibilityNarrowsAnApp(t *testing.T) {
	c, _ := sampleHomeCache(t)
	if err := c.Commit(context.Background(), access.System("test"), homeAppName, store.Delete(homeVisibilityTab, store.Row{"App": "birthday"})); err != nil {
		t.Fatalf("drop birthday's row: %v", err)
	}
	if got := c.Model().HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from the sample parent = %v, want just the app with no row", got)
	}
	if got := c.Model().HiddenApps(" Mia.Torres@heliosschool.org "); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from mia = %v, want just the app with no row, her address normalized", got)
	}
	if got := c.Model().HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 2 || got[0] != "celebrate" || got[1] != "birthday" {
		t.Errorf("hidden from sam = %v, want [celebrate birthday]", got)
	}
	if got := c.Model().Home.MissingVisibility(); len(got) != 1 || got[0].Key != "birthday" {
		t.Errorf("apps without a row = %v, want the birthday team's", got)
	}
	apps := c.Model().Home.AppVisibilities()
	if len(apps) != len(Apps) || apps[0].Visibility != VisibleToEveryone || len(apps[0].Emails) != 0 {
		t.Errorf("app visibilities = %+v, want every app, the first everyone's with nobody listed", apps)
	}
	if apps[2].Key != "celebrate" || apps[2].Visibility != VisibleToList || len(apps[2].Emails) != 2 {
		t.Errorf("celebrate = %+v, want narrowed to two", apps[2])
	}
	if last := apps[len(apps)-1]; last.Key != "birthday" || last.Visibility != VisibleToList || len(last.Emails) != 0 || last.Order != "" {
		t.Errorf("birthday = %+v, want the new-app default, a list with nobody, last for want of a key", last)
	}

	list := c.Model().Home.AppList()
	if list[0].Tagline != "A visual directory" || list[1].Tagline != "HCA Volunteer Portal" {
		t.Errorf("taglines = %q %q, want the registry's for who and the sheet's for team", list[0].Tagline, list[1].Tagline)
	}
	setAppVisibility(t, c, "who", AppVisibilityRow{Mode: VisibleToEveryone, Tagline: "Find anyone"})
	if got := c.Model().Home.AppList()[0].Tagline; got != "Find anyone" {
		t.Errorf("who's tagline after an edit = %q", got)
	}

	setAppVisibility(t, c, "celebrate", AppVisibilityRow{Mode: VisibleToEveryone, Emails: []string{"mia.torres@heliosschool.org"}})
	if got := c.Model().HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from sam with the celebration everyone's = %v, want just birthday", got)
	}
	if got := c.Model().Home.AppVisibilities()[2].Emails; len(got) != 1 {
		t.Errorf("the celebration's list = %v, want kept while everyone's", got)
	}

	setAppVisibility(t, c, "who", AppVisibilityRow{Mode: VisibleToList})
	if got := c.Model().HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 2 || got[0] != "who" {
		t.Errorf("hidden from the sample parent with who's list empty = %v, want [who birthday]", got)
	}
}

func TestGrantAddsToAnAppsList(t *testing.T) {
	c, dir := sampleHomeCache(t)
	const email = "sam.whitfield@heliosschool.org"
	if err := c.GrantApp(context.Background(), "celebrate", " Sam.Whitfield@heliosschool.org "); err != nil {
		t.Fatal(err)
	}
	if got := c.Model().HiddenApps(email); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from sam after the grant = %v, want just birthday", got)
	}
	if err := c.GrantApp(context.Background(), "celebrate", email); err != nil {
		t.Fatal(err)
	}
	log := dir.rows(t, store.ChangeLogTab)
	if len(log) != 1 || log[0]["Actor"] != email || log[0]["Tab"] != homeVisibilityTab || log[0]["Column"] != "Emails" || log[0]["Previous"] != "jordan.whitfield@heliosschool.org, mia.torres@heliosschool.org" {
		t.Errorf("change log = %v, want one row holding the list before", log)
	}
}

func withEvents(tables store.Tables) store.Tables {
	tables[homeCategoriesTab] = append([]store.Row{eventsRow("2")}, tables[homeCategoriesTab]...)
	return tables
}

func TestVisibilityRowForAnUnknownAppIsSkipped(t *testing.T) {
	rows := []store.Row{{"App": "bogus", "Visibility": "nonsense"}, {"App": "who", "Visibility": "everyone", store.OrderColumn: "2"}}
	m, err := BuildHome(context.Background(), withEvents(store.Tables{homeVisibilityTab: rows}), testkit.All)
	if err != nil {
		t.Fatalf("a row for an app this build does not know refused the load: %v", err)
	}
	if _, ok := m.Visibility["bogus"]; ok {
		t.Error("the unknown app's row reached the model")
	}
	if v, ok := m.Visibility["who"]; !ok || v.Mode != VisibleToEveryone {
		t.Errorf("who = %+v, want its row read as usual", v)
	}
}

func TestVisibilityRowsAreChecked(t *testing.T) {
	for _, c := range []struct {
		rows []store.Row
		want string
	}{
		{[]store.Row{{"App": "who", "Visibility": "List"}}, "is not everyone or list"},
		{[]store.Row{{"App": "who"}}, "is not everyone or list"},
		{[]store.Row{{"App": "who", "Visibility": "list", store.OrderColumn: "2"}, {"App": "who", "Visibility": "everyone", store.OrderColumn: "4"}}, "two rows"},
	} {
		if _, err := BuildHome(context.Background(), withEvents(store.Tables{homeVisibilityTab: c.rows}), testkit.All); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v built: %v, want %q", c.rows, err, c.want)
		}
	}
}

func TestVisibilityEmailsCell(t *testing.T) {
	got := splitVisibilityEmails("A@x.org, b@x.org;c@x.org\nd@x.org  a@x.org")
	if strings.Join(got, " ") != "a@x.org b@x.org c@x.org d@x.org" {
		t.Errorf("split = %v", got)
	}
	if cells := (AppVisibilityRow{Mode: VisibleToList, Emails: []string{"b@x.org", "c@x.org"}}).cells(); cells["Emails"] != "b@x.org, c@x.org" {
		t.Errorf("cells = %v", cells)
	}
}

func TestAppsSection(t *testing.T) {
	tables := withEvents(store.Tables{homeCategoriesTab: {categoryRow(appsID, "Helios Community Apps", "📌", StyleApps, "4"), categoryRow(schoolID, "School", "", StyleTiles, "6")}})
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("build with an apps row: %v", err)
	}
	if len(m.Categories) != 3 || m.Categories[0].Style != StyleEvents || m.Categories[1].Style != StyleApps {
		t.Errorf("categories = %+v, want the events section, then the apps section", m.Categories)
	}
	tables[homeLinksTab] = []store.Row{{"Link ID": directoryID, "Title": "Directory", "URL": "https://who.heliosian.com/", "Category": appsID, "Visible": "Yes", store.OrderColumn: "2"}}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "community apps") {
		t.Errorf("a link under the apps section built: %v", err)
	}
	tables[homeLinksTab] = nil
	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(chatsID, "More Apps", "", StyleApps, "8"))
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two apps rows built: %v", err)
	}
}
