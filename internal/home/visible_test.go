package home

import (
	"context"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

type sheet struct {
	dir   *data.Dir
	queue *store.Queue
}

func (s sheet) rows(t *testing.T, tab string) []store.Row {
	t.Helper()
	s.queue.Flush()
	_, rows, err := s.dir.Table(appName, tab)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func sampleCache(t *testing.T) (*Cache, sheet) {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	queue := store.NewQueue()
	c, err := NewCache(dir, dir, noImages{}, func() []string { return nil }, queue)
	if err != nil {
		t.Fatalf("load sample apps sheet: %v", err)
	}
	return c, sheet{dir: dir, queue: queue}
}

func setVisibility(t *testing.T, c *Cache, app string, v Visibility) {
	t.Helper()
	if err := c.Commit(context.Background(), "test", store.Set(visibilityTab, store.Row{"App": app}, v.cells())); err != nil {
		t.Fatalf("set %s: %v", app, err)
	}
}

func TestVisibilityNarrowsAnApp(t *testing.T) {
	c, _ := sampleCache(t)
	if got := c.HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from the sample parent = %v, want just the app with no row", got)
	}
	if got := c.HiddenApps(" Mia.Torres@heliosschool.org "); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from mia = %v, want just the app with no row, her address normalized", got)
	}
	if got := c.HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 2 || got[0] != "celebrate" || got[1] != "birthday" {
		t.Errorf("hidden from sam = %v, want [celebrate birthday]", got)
	}
	if got := c.MissingVisibility(); len(got) != 1 || got[0].Key != "birthday" {
		t.Errorf("apps without a row = %v, want the birthday team's", got)
	}
	apps := c.AppVisibilities()
	if len(apps) != len(Apps) || apps[0].Visibility != VisibleToEveryone || len(apps[0].Emails) != 0 {
		t.Errorf("app visibilities = %+v, want every app, the first everyone's with nobody listed", apps)
	}
	if apps[2].Key != "celebrate" || apps[2].Visibility != VisibleToList || len(apps[2].Emails) != 2 {
		t.Errorf("celebrate = %+v, want narrowed to two", apps[2])
	}
	if apps[3].Key != "birthday" || apps[3].Visibility != VisibleToList || len(apps[3].Emails) != 0 {
		t.Errorf("birthday = %+v, want the new-app default, a list with nobody", apps[3])
	}

	list := c.AppList()
	if list[0].Tagline != "A visual directory" || list[1].Tagline != "HCA Volunteer Portal" {
		t.Errorf("taglines = %q %q, want the registry's for who and the sheet's for team", list[0].Tagline, list[1].Tagline)
	}
	setVisibility(t, c, "who", Visibility{Mode: VisibleToEveryone, Tagline: "Find anyone"})
	if got := c.AppList()[0].Tagline; got != "Find anyone" {
		t.Errorf("who's tagline after an edit = %q", got)
	}

	setVisibility(t, c, "celebrate", Visibility{Mode: VisibleToEveryone, Emails: []string{"mia.torres@heliosschool.org"}})
	if got := c.HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from sam with the celebration everyone's = %v, want just birthday", got)
	}
	if got := c.AppVisibilities()[2].Emails; len(got) != 1 {
		t.Errorf("the celebration's list = %v, want kept while everyone's", got)
	}

	setVisibility(t, c, "who", Visibility{Mode: VisibleToList})
	if got := c.HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 2 || got[0] != "who" {
		t.Errorf("hidden from the sample parent with who's list empty = %v, want [who birthday]", got)
	}
}

func TestGrantAddsToAnAppsList(t *testing.T) {
	c, dir := sampleCache(t)
	const email = "sam.whitfield@heliosschool.org"
	if err := Grant(context.Background(), c, "celebrate", " Sam.Whitfield@heliosschool.org "); err != nil {
		t.Fatal(err)
	}
	if got := c.HiddenApps(email); len(got) != 1 || got[0] != "birthday" {
		t.Errorf("hidden from sam after the grant = %v, want just birthday", got)
	}
	if err := Grant(context.Background(), c, "celebrate", email); err != nil {
		t.Fatal(err)
	}
	log := dir.rows(t, store.ChangeLogTab)
	if len(log) != 1 || log[0]["Actor"] != email || log[0]["Tab"] != visibilityTab || log[0]["Column"] != "Emails" || log[0]["Previous"] != "jordan.whitfield@heliosschool.org, mia.torres@heliosschool.org" {
		t.Errorf("change log = %v, want one row holding the list before", log)
	}
}

func TestVisibilityRowForAnUnknownAppIsSkipped(t *testing.T) {
	rows := []store.Row{{"App": "bogus", "Visibility": "nonsense"}, {"App": "who", "Visibility": "everyone"}}
	m, err := BuildModel(context.Background(), store.Tables{visibilityTab: rows}, noImages{})
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
		if _, err := BuildModel(context.Background(), store.Tables{visibilityTab: c.rows}, noImages{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v built: %v, want %q", c.rows, err, c.want)
		}
	}
}

func TestVisibilityEmailsCell(t *testing.T) {
	got := splitEmails("A@x.org, b@x.org;c@x.org\nd@x.org  a@x.org")
	if strings.Join(got, " ") != "a@x.org b@x.org c@x.org d@x.org" {
		t.Errorf("split = %v", got)
	}
	if cells := (Visibility{Mode: VisibleToList, Emails: []string{"b@x.org", "c@x.org"}}).cells(); cells["Emails"] != "b@x.org, c@x.org" {
		t.Errorf("cells = %v", cells)
	}
}

func TestAppsSection(t *testing.T) {
	tables := store.Tables{categoriesTab: {category("Helios Community Apps", "📌", StyleApps), category("School", "", StyleTiles)}}
	m, err := BuildModel(context.Background(), tables, noImages{})
	if err != nil {
		t.Fatalf("build with an apps row: %v", err)
	}
	if len(m.Categories) != 3 || m.Categories[1].Style != StyleApps {
		t.Errorf("categories = %+v, want the synthesized events section, then the apps section", m.Categories)
	}
	tables[linksTab] = []store.Row{{"Title": "Directory", "URL": "https://who.heliosian.com/", "Category": "Helios Community Apps", "Visible": "Yes"}}
	if _, err := BuildModel(context.Background(), tables, noImages{}); err == nil || !strings.Contains(err.Error(), "community apps") {
		t.Errorf("a link under the apps section built: %v", err)
	}
	tables[linksTab] = nil
	tables[categoriesTab] = append(tables[categoriesTab], category("More Apps", "", StyleApps))
	if _, err := BuildModel(context.Background(), tables, noImages{}); err == nil || !strings.Contains(err.Error(), "only one") {
		t.Errorf("two apps rows built: %v", err)
	}
}
