package model

import (
	"context"
	"maps"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/store"
)

func TestInviteTemplatesComeInOrder(t *testing.T) {
	systems := sampleInvites(t).Model().Invites.Systems
	names := []string{}
	for _, s := range systems {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"Greenvelope", "Evite", "Paperless Post", "Punchbowl", "Partiful"}) {
		t.Fatalf("systems %v", names)
	}
	if systems[0].ID != "svc0000000001" {
		t.Fatalf("greenvelope id %q", systems[0].ID)
	}
	green := systems[0].Columns
	if len(green) != 10 || green[0].Template != "{{ greeting }}" || green[8].Name != green[9].Name || green[9].Template != "{{ member_6 }}" {
		t.Fatalf("greenvelope columns %+v", green)
	}
	if systems[1].HeaderRow || !systems[0].SupportsGroups {
		t.Fatalf("flags %+v %+v", systems[0], systems[1])
	}
}

func TestInviteTemplatesRefuseStrayColumns(t *testing.T) {
	services := []store.Row{{"Service ID": "svc0000000001", "Display Name": "Evite", "Header Row": "No", "Supports Groups": "No"}}
	columns := []store.Row{
		{"Service": "svc0000000001", store.OrderColumn: "1", "Column": "Name", "Template": "{{ greeting }}"},
		{"Service": "svc0000000009", store.OrderColumn: "1", "Column": "Name", "Template": "{{ greeting }}"},
	}
	for name, c := range map[string]struct {
		tables store.Tables
		want   string
	}{
		"unknown service":    {store.Tables{servicesTab: services, templatesTab: columns}, "unknown services"},
		"service unfilled":   {store.Tables{servicesTab: services, templatesTab: columns[1:]}, "no template columns"},
		"bad order":          {store.Tables{servicesTab: services, templatesTab: []store.Row{withCell(columns[0], store.OrderColumn, "A")}}, "order"},
		"column name blank":  {store.Tables{servicesTab: services, templatesTab: []store.Row{withCell(columns[0], "Column", "")}}, "no service or column"},
		"service by name":    {store.Tables{servicesTab: services, templatesTab: []store.Row{withCell(columns[0], "Service", "Evite")}}, "is not an id"},
		"service id missing": {store.Tables{servicesTab: []store.Row{withCell(services[0], "Service ID", "")}, templatesTab: columns[:1]}, "is not an id"},
		"service id twice":   {store.Tables{servicesTab: []store.Row{services[0], withCell(services[0], "Display Name", "Evite Again")}, templatesTab: columns[:1]}, "used twice"},
	} {
		if _, err := buildSystems(c.tables, map[string]bool{}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestGreetingsNeedIDsAndTheBuiltIns(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	_, greetings, err := dir.Table(invitesApp, greetingsTab)
	if err != nil {
		t.Fatal(err)
	}
	_, services, err := dir.Table(invitesApp, servicesTab)
	if err != nil {
		t.Fatal(err)
	}
	_, templates, err := dir.Table(invitesApp, templatesTab)
	if err != nil {
		t.Fatal(err)
	}
	replaced := func(i int, column, value string) []store.Row {
		out := slices.Clone(greetings)
		out[i] = withCell(out[i], column, value)
		return out
	}
	for name, c := range map[string]struct {
		greetings []store.Row
		want      string
	}{
		"id missing":        {replaced(3, "Greeting ID", ""), "is not an id"},
		"id twice":          {replaced(3, "Greeting ID", greetings[0]["Greeting ID"]), "used twice"},
		"id of a service":   {replaced(3, "Greeting ID", services[0]["Service ID"]), "used twice"},
		"built-in missing":  {greetings[1:], "built-in greeting " + builtinGreetings.Default},
		"built-in is owned": {replaced(0, "Email", "asha.chandra@heliosschool.org"), "belongs to"},
	} {
		tables := store.Tables{servicesTab: services, templatesTab: templates, greetingsTab: c.greetings}
		if _, err := buildInvites(tables); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRenamingAServiceKeepsItsTemplates(t *testing.T) {
	invites := sampleInvites(t)
	actor := access.Actor{Email: jordan}
	if err := invites.Commit(context.Background(), actor, invitesApp, store.Update(servicesTab, store.Row{"Service ID": "svc0000000001"}, store.Row{"Display Name": "Greenvelope Plus"})); err != nil {
		t.Fatal(err)
	}
	renamed := invites.Model().Invites.Systems[0]
	if renamed.ID != "svc0000000001" || renamed.Name != "Greenvelope Plus" || len(renamed.Columns) != 10 {
		t.Fatalf("renamed service %+v", renamed)
	}
}

func sampleInvites(t *testing.T) *Store {
	t.Helper()
	dir := &data.Dir{Root: "../../sampledata"}
	return sampleStore(t, dir, store.NewQueue(), sampleDeps(sampleKey))
}

func withCell(row store.Row, column, value string) store.Row {
	out := maps.Clone(row)
	out[column] = value
	return out
}
