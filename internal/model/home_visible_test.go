package model

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/blob"
	"heliosian/internal/data"
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

func setAppVisibility(t *testing.T, c *Store, app string, v AppVisibilityRow) {
	t.Helper()
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
	if apps[3].Key != "birthday" || apps[3].Visibility != VisibleToList || len(apps[3].Emails) != 0 {
		t.Errorf("birthday = %+v, want the new-app default, a list with nobody", apps[3])
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

func TestVisibilityRowForAnUnknownAppIsSkipped(t *testing.T) {
	rows := []store.Row{{"App": "bogus", "Visibility": "nonsense"}, {"App": "who", "Visibility": "everyone"}}
	m, err := BuildHome(context.Background(), store.Tables{homeVisibilityTab: rows}, testkit.All)
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
		{[]store.Row{{"App": "who", "Visibility": "list"}, {"App": "who", "Visibility": "everyone"}}, "two rows"},
	} {
		if _, err := BuildHome(context.Background(), store.Tables{homeVisibilityTab: c.rows}, testkit.All); err == nil || !strings.Contains(err.Error(), c.want) {
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
	tables := store.Tables{homeCategoriesTab: {categoryRow(appsID, "Helios Community Apps", "📌", StyleApps), categoryRow(schoolID, "School", "", StyleTiles)}}
	m, err := BuildHome(context.Background(), tables, testkit.All)
	if err != nil {
		t.Fatalf("build with an apps row: %v", err)
	}
	if len(m.Categories) != 3 || m.Categories[1].Style != StyleApps {
		t.Errorf("categories = %+v, want the synthesized events section, then the apps section", m.Categories)
	}
	tables[homeLinksTab] = []store.Row{{"Link ID": directoryID, "Title": "Directory", "URL": "https://who.heliosian.com/", "Category": appsID, "Visible": "Yes"}}
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "community apps") {
		t.Errorf("a link under the apps section built: %v", err)
	}
	tables[homeLinksTab] = nil
	tables[homeCategoriesTab] = append(tables[homeCategoriesTab], categoryRow(chatsID, "More Apps", "", StyleApps))
	if _, err := BuildHome(context.Background(), tables, testkit.All); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two apps rows built: %v", err)
	}
}
