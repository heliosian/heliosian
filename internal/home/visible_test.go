package home

import (
	"strings"
	"testing"

	"heliosian/internal/data"
)

func sampleCache(t *testing.T) *Cache {
	t.Helper()
	tables, err := ReadTables(&data.Dir{Root: "../../sampledata"})
	if err != nil {
		t.Fatalf("read sample apps sheet: %v", err)
	}
	model, err := BuildModel(tables, noImages{})
	if err != nil {
		t.Fatalf("build sample model: %v", err)
	}
	c := &Cache{}
	c.set(tables, model)
	return c
}

// An app with no row, or one visible to everyone, is everyone's; one
// narrowed to a list is hidden from everyone not on it. The sample sheet
// narrows the celebration to the sample parent and one other.
func TestVisibilityNarrowsAnApp(t *testing.T) {
	c := sampleCache(t)
	if got := c.HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 0 {
		t.Errorf("hidden from the sample parent = %v, want nothing", got)
	}
	if got := c.HiddenApps(" Mia.Torres@heliosschool.org "); len(got) != 0 {
		t.Errorf("hidden from mia = %v, want nothing, her address normalized", got)
	}
	if got := c.HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "celebrate" {
		t.Errorf("hidden from sam = %v, want [celebrate]", got)
	}
	apps := c.AppVisibilities()
	if len(apps) != len(Apps) || apps[0].Visibility != VisibleToEveryone || len(apps[0].Emails) != 0 {
		t.Errorf("app visibilities = %+v, want every app, the first everyone's with nobody listed", apps)
	}
	if apps[2].Key != "celebrate" || apps[2].Visibility != VisibleToList || len(apps[2].Emails) != 2 {
		t.Errorf("celebrate = %+v, want narrowed to two", apps[2])
	}

	// The list is kept while the app is everyone's, and hides nobody.
	c.set(c.Tables(), mustBuild(t, c.Tables().withVisibility("celebrate", Visibility{Mode: VisibleToEveryone, Emails: []string{"mia.torres@heliosschool.org"}})))
	if got := c.HiddenApps("sam.whitfield@heliosschool.org"); len(got) != 0 {
		t.Errorf("hidden from sam with the celebration everyone's = %v, want nothing", got)
	}
	if got := c.AppVisibilities()[2].Emails; len(got) != 1 {
		t.Errorf("the celebration's list = %v, want kept while everyone's", got)
	}

	// A list with nobody on it hides the app from everyone.
	c.set(c.Tables(), mustBuild(t, c.Tables().withVisibility("who", Visibility{Mode: VisibleToList})))
	if got := c.HiddenApps("jordan.whitfield@heliosschool.org"); len(got) != 1 || got[0] != "who" {
		t.Errorf("hidden from the sample parent with who's list empty = %v, want [who]", got)
	}
}

func mustBuild(t *testing.T, tables *Tables) *Model {
	t.Helper()
	model, err := BuildModel(tables, noImages{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return model
}

// A row naming an app the switch does not list, a misspelled mode, or a
// second row for one app refuses the load.
func TestVisibilityRowsAreChecked(t *testing.T) {
	for _, c := range []struct {
		rows []map[string]string
		want string
	}{
		{[]map[string]string{{"App": "birthday", "Visibility": "list"}}, "app must be one of"},
		{[]map[string]string{{"App": "who", "Visibility": "List"}}, "is not everyone or list"},
		{[]map[string]string{{"App": "who"}}, "is not everyone or list"},
		{[]map[string]string{{"App": "who", "Visibility": "list"}, {"App": "who", "Visibility": "everyone"}}, "two rows"},
	} {
		if _, err := BuildModel(&Tables{Visibility: c.rows}, noImages{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v built: %v, want %q", c.rows, err, c.want)
		}
	}
}

// The Emails cell takes commas, semicolons, spaces or line breaks between
// addresses, forgives case, and folds duplicates; the mirror of a save
// writes it back with commas and edits the app's own row alone.
func TestVisibilityEmailsCell(t *testing.T) {
	got := splitEmails("A@x.org, b@x.org;c@x.org\nd@x.org  a@x.org")
	if strings.Join(got, " ") != "a@x.org b@x.org c@x.org d@x.org" {
		t.Errorf("split = %v", got)
	}
	tables := &Tables{Visibility: []map[string]string{{"App": "who", "Visibility": "list", "Emails": "a@x.org"}}}
	next := tables.withVisibility("celebrate", Visibility{Mode: VisibleToList, Emails: []string{"b@x.org", "c@x.org"}})
	if len(next.Visibility) != 2 || next.Visibility[1]["Emails"] != "b@x.org, c@x.org" || next.Visibility[0]["Emails"] != "a@x.org" {
		t.Errorf("rows after adding celebrate = %v", next.Visibility)
	}
	next = next.withVisibility("who", Visibility{Mode: VisibleToEveryone, Emails: []string{"a@x.org"}})
	if len(next.Visibility) != 2 || next.Visibility[0]["Visibility"] != "everyone" {
		t.Errorf("rows after flipping who = %v", next.Visibility)
	}
	if len(tables.Visibility) != 1 || tables.Visibility[0]["Visibility"] != "list" {
		t.Errorf("the tables mirrored into changed: %v", tables.Visibility)
	}
}
