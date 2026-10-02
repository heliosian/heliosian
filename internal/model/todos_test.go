package model

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/store"
)

const (
	jaysTripKey      = "4e796e05dc8b2edd2b63d5737e4b1d01a4957c65a4d8a2473eec00596a7b2c52"
	newsletter925Key = "77c8f086f19880bb41a09c9b4c7008008c61a8575caff1b8973972eb3b5b6abc"
)

func toDoTitles(list []map[string]any) []string {
	out := []string{}
	for _, t := range list {
		out = append(out, t["title"].(string))
	}
	return out
}

func toDoNamed(t *testing.T, list []map[string]any, title string) map[string]any {
	t.Helper()
	for _, item := range list {
		if item["title"] == title {
			return item
		}
	}
	t.Fatalf("no to-do %q in %v", title, toDoTitles(list))
	return nil
}

func toDoState(item map[string]any) string {
	me, _ := item["me"].(map[string]any)
	state, _ := me["state"].(string)
	return state
}

func TestToDos(t *testing.T) {
	c, s, mux := schoolServer(t)
	if err := c.Commit(context.Background(), access.System("test"), DocumentsApp, store.Upsert(readsTab, store.Row{"Document": newsletter925Key}, store.Row{"Audience": DocumentForEveryone})); err != nil {
		t.Fatal(err)
	}
	toDos := func(as string) []map[string]any { return listOf(t, mux, as, "/api/to-dos") }
	jordan := toDos("jordan.whitfield@heliosschool.org")
	if got := toDoTitles(jordan); !slices.Equal(got, []string{"Plan 12:30pm pickups for ILP week", "Return the tide pool permission slip", "Volunteer to drive for the tide pool trip"}) {
		t.Fatalf("the Jays parent's to-dos = %v, want the dated ones by day and then the undated", got)
	}
	slip := toDoNamed(t, jordan, "Return the tide pool permission slip")
	source, _ := slip["source"].(map[string]any)
	if slip["due"] != "2026-10-05" || source["to"] != "Jays parents" || source["title"] != "Jays field trip to the tide pools" || source["date"] != "2026-09-21" || !strings.Contains(slip["details"].(string), "Monday, October 5") {
		t.Errorf("the slip = %v", slip)
	}
	if !can(slip, "complete") || !can(slip, "save") || can(slip, "clear") || toDoState(slip) != "" {
		t.Errorf("an open to-do's can and state = %v", slip)
	}
	if got := toDoTitles(toDos("ruth.amari@heliosschool.org")); !slices.Equal(got, []string{"Plan 12:30pm pickups for ILP week"}) {
		t.Errorf("a staff member without a Jays seat sees %v", got)
	}

	before := len(homeChangeLog(t, s))
	write(t, mux, "jordan.whitfield@heliosschool.org", "POST", "/api/to-dos/"+slip["id"].(string)+"/complete", nil)
	logged := homeLog(t, s, before)
	if len(logged) != 1 || !strings.HasPrefix(logged[0], "jordan.whitfield@heliosschool.org|insert|"+homeToDosTab+"|") {
		t.Errorf("completing wrote %v", logged)
	}
	done := toDoNamed(t, toDos("jordan.whitfield@heliosschool.org"), "Return the tide pool permission slip")
	if toDoState(done) != ToDoDone || can(done, "complete") || can(done, "save") || !can(done, "clear") {
		t.Errorf("a done to-do = %v", done)
	}
	if robin := toDoNamed(t, toDos("robin.whitfield@heliosschool.org"), "Return the tide pool permission slip"); toDoState(robin) != "" {
		t.Errorf("the other parent's copy is %q; a to-do is each person's own", toDoState(robin))
	}

	drivers := toDoNamed(t, jordan, "Volunteer to drive for the tide pool trip")
	write(t, mux, "jordan.whitfield@heliosschool.org", "POST", "/api/to-dos/"+drivers["id"].(string)+"/save", nil)
	clockAt(t, time.Date(2026, 10, 10, 9, 0, 0, 0, Location))
	if got := toDoTitles(toDos("jordan.whitfield@heliosschool.org")); !slices.Equal(got, []string{"Volunteer to drive for the tide pool trip"}) {
		t.Errorf("past their days, the to-dos are %v, want the saved one alone", got)
	}
	if got := toDos("robin.whitfield@heliosschool.org"); len(got) != 0 {
		t.Errorf("past their days, someone who saved nothing still has %v", toDoTitles(got))
	}
	write(t, mux, "jordan.whitfield@heliosschool.org", "POST", "/api/to-dos/"+drivers["id"].(string)+"/clear", nil)
	if got := toDos("jordan.whitfield@heliosschool.org"); len(got) != 0 {
		t.Errorf("an unsaved to-do past its window stays: %v", toDoTitles(got))
	}
}

func TestSettingAReadingReplacesADocumentsOwn(t *testing.T) {
	c, _, _ := schoolServer(t)
	ctx, system := context.Background(), access.System("test")
	reading := Reading{
		Points:   []string{"The Jays visit the tide pools on Fri, Oct 9.", "Four parent drivers are needed."},
		Audience: "Jays",
		ToDos:    []ToDo{{Title: "Drive on the trip", Summary: "Drivers wanted", Details: "Four drivers.", Due: "2026-10-09", Point: 2}},
	}
	if err := c.SetReading(ctx, system, jaysTripKey, reading, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	docs := c.Model().Documents
	got := []ToDo{}
	for _, toDo := range docs.ToDos {
		if toDo.Document == jaysTripKey {
			got = append(got, *toDo)
		}
	}
	if len(got) != 1 || got[0].Title != "Drive on the trip" || got[0].Point != 2 || docs.Read[jaysTripKey] != "2026-10-01" {
		t.Errorf("after setting, the trip's to-dos are %+v, read %q", got, docs.Read[jaysTripKey])
	}
	if !slices.Equal(docs.Points[jaysTripKey], reading.Points) || docs.Audience[jaysTripKey] != "Jays" {
		t.Errorf("after setting, the trip's points are %v to %q", docs.Points[jaysTripKey], docs.Audience[jaysTripKey])
	}
	for _, bad := range []ToDo{{Title: "Pay", Link: "not a link"}, {Title: "Pay", Due: "Friday"}, {Title: "Pay", Point: 3}} {
		if err := c.SetReading(ctx, system, jaysTripKey, Reading{Points: reading.Points, ToDos: []ToDo{bad}}, "2026-10-01"); err == nil {
			t.Errorf("a to-do %+v was taken", bad)
		}
	}
	ilp := ""
	for _, toDo := range c.Model().Documents.ToDos {
		if toDo.Document == newsletter925Key {
			ilp = toDo.ID
		}
	}
	repeating := Reading{Points: reading.Points, ToDos: reading.ToDos, Repeats: map[int]string{1: ilp}}
	if err := c.SetReading(ctx, system, jaysTripKey, repeating, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	if got := c.Model().Documents.Repeats[jaysTripKey]; len(got) != 1 || got[1] != ilp {
		t.Errorf("after setting, the trip's repeats are %v, want point 1 on %s", got, ilp)
	}
	own := c.Model().Documents.ToDos[0].ID
	for _, bad := range []map[int]string{{3: ilp}, {1: "nosuchtodo00"}} {
		if err := c.SetReading(ctx, system, jaysTripKey, Reading{Points: reading.Points, Repeats: bad}, "2026-10-01"); err == nil {
			t.Errorf("repeats %v were taken", bad)
		}
	}
	for _, toDo := range c.Model().Documents.ToDos {
		if toDo.Document == jaysTripKey {
			own = toDo.ID
		}
	}
	if err := c.SetReading(ctx, system, jaysTripKey, Reading{Points: reading.Points, ToDos: reading.ToDos, Repeats: map[int]string{1: own}}, "2026-10-01"); err == nil {
		t.Errorf("a point repeating the email's own to-do was taken")
	}
	if err := c.SetReading(ctx, system, "no-such-document", Reading{}, "2026-10-01"); err == nil {
		t.Errorf("a reading of a document not on file was taken")
	}
	if err := c.Commit(ctx, system, DocumentsApp, c.Model().Documents.Drop(system, jaysTripKey)...); err != nil {
		t.Fatal(err)
	}
	if docs := c.Model().Documents; slices.ContainsFunc(docs.ToDos, func(t *ToDo) bool { return t.Document == jaysTripKey }) || docs.Read[jaysTripKey] != "" || docs.Points[jaysTripKey] != nil {
		t.Errorf("dropping the document left its reading")
	}
}
