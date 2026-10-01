package model

import (
	"fmt"
	"maps"
	"slices"

	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/store"
)

type InviteTemplate struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	HeaderRow      bool             `json:"headerRow"`
	SupportsGroups bool             `json:"supportsGroups"`
	Columns        []TemplateColumn `json:"columns"`
}

type TemplateColumn struct {
	Name     string `json:"name"`
	Template string `json:"template"`
	order    string
}

type InviteTemplates struct {
	Systems   []InviteTemplate
	Greetings []GreetingTemplate
}

type GreetingTemplate struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Format     string `json:"format"`
	Grouped    bool   `json:"grouped"`
	Individual bool   `json:"individual"`
	CreatedBy  string `json:"createdBy,omitempty"`
}

type greetingBuiltins struct {
	Default     string `json:"default"`
	WholeFamily string `json:"wholeFamily"`
	Kids        string `json:"kids"`
	Adults      string `json:"adults"`
	FullName    string `json:"fullName"`
	FirstName   string `json:"firstName"`
}

var builtinGreetings = greetingBuiltins{
	Default:     "gtg0000000001",
	Kids:        "gtg0000000002",
	Adults:      "gtg0000000003",
	WholeFamily: "gtg0000000005",
	FirstName:   "gtg0000000006",
	FullName:    "gtg0000000007",
}

func (b greetingBuiltins) ids() []string {
	return []string{b.Default, b.WholeFamily, b.Kids, b.Adults, b.FullName, b.FirstName}
}

const (
	invitesApp   = "invites"
	servicesTab  = "Services"
	templatesTab = "Templates"
	greetingsTab = "Greetings"
)

var (
	ServiceColumns  = []string{"Service ID", "Display Name", "Header Row", "Supports Groups", "Description"}
	TemplateColumns = []string{"Service", store.OrderColumn, "Column", "Template"}
	GreetingColumns = []string{"Greeting ID", "Name", "Format", "Grouped", "Individual", "Email"}
)

func buildInvites(tables store.Tables) (*InviteTemplates, error) {
	ids := map[string]bool{}
	systems, err := buildSystems(tables, ids)
	if err != nil {
		return nil, err
	}
	greetings, err := buildGreetings(tables, ids)
	if err != nil {
		return nil, err
	}
	return &InviteTemplates{Systems: systems, Greetings: greetings}, nil
}

func claimID(ids map[string]bool, what, raw string) (string, error) {
	key, ok := id.Parse(raw)
	if !ok {
		return "", fmt.Errorf("%s: id %q is not an id", what, raw)
	}
	if ids[key] {
		return "", fmt.Errorf("%s: id %s is used twice", what, key)
	}
	ids[key] = true
	return key, nil
}

func buildGreetings(tables store.Tables, ids map[string]bool) ([]GreetingTemplate, error) {
	greetings := []GreetingTemplate{}
	for _, row := range tables[greetingsTab] {
		name, format := row["Name"], row["Format"]
		if name == "" || format == "" {
			continue
		}
		key, err := claimID(ids, fmt.Sprintf("greeting %q", name), row["Greeting ID"])
		if err != nil {
			return nil, err
		}
		grouped, err := cells.YesNo(row["Grouped"], true)
		if err != nil {
			return nil, fmt.Errorf("greeting %q: grouped %w", name, err)
		}
		individual, err := cells.YesNo(row["Individual"], true)
		if err != nil {
			return nil, fmt.Errorf("greeting %q: individual %w", name, err)
		}
		greetings = append(greetings, GreetingTemplate{
			ID:         key,
			Name:       name,
			Format:     format,
			Grouped:    grouped,
			Individual: individual,
			CreatedBy:  row["Email"],
		})
	}
	for _, key := range builtinGreetings.ids() {
		i := slices.IndexFunc(greetings, func(g GreetingTemplate) bool { return g.ID == key })
		if i < 0 {
			return nil, fmt.Errorf("built-in greeting %s has no row", key)
		}
		if greetings[i].CreatedBy != "" {
			return nil, fmt.Errorf("built-in greeting %s belongs to %s", key, greetings[i].CreatedBy)
		}
	}
	return greetings, nil
}

func buildSystems(tables store.Tables, ids map[string]bool) ([]InviteTemplate, error) {
	columns := map[string][]TemplateColumn{}
	for _, row := range tables[templatesTab] {
		name := row["Column"]
		if row["Service"] == "" || name == "" {
			return nil, fmt.Errorf("template row %v names no service or column", row)
		}
		service, ok := id.Parse(row["Service"])
		if !ok {
			return nil, fmt.Errorf("template column %q: service %q is not an id", name, row["Service"])
		}
		order := row[store.OrderColumn]
		if err := store.CheckKey(order); err != nil {
			return nil, fmt.Errorf("%s template column %q: %w", service, name, err)
		}
		columns[service] = append(columns[service], TemplateColumn{Name: name, Template: row["Template"], order: order})
	}
	systems := []InviteTemplate{}
	for _, row := range tables[servicesTab] {
		name := row["Display Name"]
		if name == "" {
			continue
		}
		key, err := claimID(ids, "service "+name, row["Service ID"])
		if err != nil {
			return nil, err
		}
		cols, ok := columns[key]
		if !ok {
			return nil, fmt.Errorf("service %s has no template columns", name)
		}
		delete(columns, key)
		slices.SortStableFunc(cols, func(a, b TemplateColumn) int { return store.CompareKeys(a.order, b.order) })
		headerRow, err := cells.YesNo(row["Header Row"], true)
		if err != nil {
			return nil, fmt.Errorf("%s template: header row %w", name, err)
		}
		supportsGroups, err := cells.YesNo(row["Supports Groups"], true)
		if err != nil {
			return nil, fmt.Errorf("%s template: supports groups %w", name, err)
		}
		systems = append(systems, InviteTemplate{
			ID:             key,
			Name:           name,
			Description:    row["Description"],
			HeaderRow:      headerRow,
			SupportsGroups: supportsGroups,
			Columns:        cols,
		})
	}
	if len(columns) > 0 {
		return nil, fmt.Errorf("template columns name unknown services %v", slices.Sorted(maps.Keys(columns)))
	}
	return systems, nil
}

var invitesTabs = []store.Tab{
	{Name: servicesTab, Columns: ServiceColumns, Key: []string{"Service ID"}},
	{Name: templatesTab, Columns: TemplateColumns, Key: []string{"Service", store.OrderColumn}},
	{Name: greetingsTab, Columns: GreetingColumns, Key: []string{"Greeting ID"}},
}

func (m *InviteTemplates) greeting(raw string) (GreetingTemplate, bool) {
	key, ok := id.Parse(raw)
	if !ok {
		return GreetingTemplate{}, false
	}
	for _, g := range m.Greetings {
		if g.ID == key {
			return g, true
		}
	}
	return GreetingTemplate{}, false
}

func visibleGreetings(greetings []GreetingTemplate, email string) []GreetingTemplate {
	visible := []GreetingTemplate{}
	for _, g := range greetings {
		if g.CreatedBy == "" || g.CreatedBy == email {
			visible = append(visible, g)
		}
	}
	return visible
}
