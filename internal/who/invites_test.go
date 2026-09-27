package who

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

func TestInviteTemplatesComeInOrder(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	invites, err := NewInvites(dir, dir, store.NewQueue())
	if err != nil {
		t.Fatal(err)
	}
	systems := invites.Model().Systems
	names := []string{}
	for _, s := range systems {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"Greenvelope", "Evite", "Paperless Post", "Punchbowl", "Partiful"}) {
		t.Fatalf("systems %v", names)
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
	services := []store.Row{{"Display Name": "Evite", "Header Row": "No", "Supports Groups": "No"}}
	columns := []store.Row{
		{"Service": "Evite", store.OrderColumn: "1", "Column": "Name", "Template": "{{ greeting }}"},
		{"Service": "Evitee", store.OrderColumn: "1", "Column": "Name", "Template": "{{ greeting }}"},
	}
	for name, c := range map[string]struct {
		tables store.Tables
		want   string
	}{
		"unknown service":   {store.Tables{servicesTab: services, templatesTab: columns}, "unknown services"},
		"service unfilled":  {store.Tables{servicesTab: services, templatesTab: columns[1:]}, "no template columns"},
		"bad order":         {store.Tables{servicesTab: services, templatesTab: []store.Row{withCell(columns[0], store.OrderColumn, "A")}}, "order"},
		"column name blank": {store.Tables{servicesTab: services, templatesTab: []store.Row{withCell(columns[0], "Column", "")}}, "no service or column"},
	} {
		if _, err := buildSystems(c.tables); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func withCell(row store.Row, column, value string) store.Row {
	out := maps.Clone(row)
	out[column] = value
	return out
}
