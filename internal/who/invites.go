package who

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type InviteTemplate struct {
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	Notes          string           `json:"notes,omitempty"`
	HeaderRow      bool             `json:"headerRow"`
	SupportsGroups bool             `json:"supportsGroups"`
	Columns        []TemplateColumn `json:"columns"`
}

type TemplateColumn struct {
	Name     string `json:"name"`
	Template string `json:"template"`
	order    string
}

type InviteModel struct {
	Systems   []InviteTemplate
	Greetings []GreetingTemplate
}

type GreetingTemplate struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	Grouped    bool   `json:"grouped"`
	Individual bool   `json:"individual"`
	CreatedBy  string `json:"createdBy,omitempty"`
}

const (
	invitesApp   = "invites"
	servicesTab  = "Services"
	templatesTab = "Templates"
	greetingsTab = "Greetings"
)

var (
	ServiceColumns  = []string{"Display Name", "Header Row", "Supports Groups", "Description", "Notes"}
	TemplateColumns = []string{"Service", store.OrderColumn, "Column", "Template"}
	GreetingColumns = []string{"Name", "Format", "Grouped", "Individual", "Email"}
)

func buildInvites(tables store.Tables) (*InviteModel, error) {
	systems, err := buildSystems(tables)
	if err != nil {
		return nil, err
	}
	greetings, err := buildGreetings(tables)
	if err != nil {
		return nil, err
	}
	return &InviteModel{Systems: systems, Greetings: greetings}, nil
}

func buildGreetings(tables store.Tables) ([]GreetingTemplate, error) {
	rows := tables[greetingsTab]
	greetings := []GreetingTemplate{}
	for _, row := range rows {
		name, format := row["Name"], row["Format"]
		if name == "" || format == "" {
			continue
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
			Name:       name,
			Format:     format,
			Grouped:    grouped,
			Individual: individual,
			CreatedBy:  row["Email"],
		})
	}
	return greetings, nil
}

func buildSystems(tables store.Tables) ([]InviteTemplate, error) {
	columns := map[string][]TemplateColumn{}
	for _, row := range tables[templatesTab] {
		service, name := row["Service"], row["Column"]
		if service == "" || name == "" {
			return nil, fmt.Errorf("template row %v names no service or column", row)
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
		cols, ok := columns[name]
		if !ok {
			return nil, fmt.Errorf("service %s has no template columns", name)
		}
		delete(columns, name)
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
			Name:           name,
			Description:    row["Description"],
			Notes:          row["Notes"],
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

type Invites struct {
	*store.Store[*InviteModel]
}

func NewInvites(source data.Source, writer data.Writer, queue *store.Queue) (*Invites, error) {
	s, err := store.New(store.Spec[*InviteModel]{
		App: invitesApp,
		Tabs: []store.Tab{
			{Name: servicesTab, Columns: ServiceColumns, Key: []string{"Display Name"}},
			{Name: templatesTab, Columns: TemplateColumns, Key: []string{"Service", store.OrderColumn}},
			{Name: greetingsTab, Columns: GreetingColumns, Key: []string{"Name"}},
		},
		Build: func(_ context.Context, tables store.Tables) (*InviteModel, error) {
			return buildInvites(tables)
		},
		Loaded: func(model *InviteModel, took time.Duration) {
			slog.Info("loaded invites", "systems", len(model.Systems), "greetings", len(model.Greetings), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Invites{Store: s}, nil
}

func (i *Invites) greeting(name string) (GreetingTemplate, bool) {
	for _, g := range i.Model().Greetings {
		if strings.EqualFold(g.Name, name) {
			return g, true
		}
	}
	return GreetingTemplate{}, false
}

type inviteTemplates struct {
	Systems   []InviteTemplate   `json:"systems"`
	Greetings []GreetingTemplate `json:"greetings"`
}

func RegisterInvites(mux *http.ServeMux, cache *Cache, invites *Invites) {
	mux.HandleFunc("GET /api/directory/invite-templates", serve.JSON(func(r *http.Request, _ serve.None) (inviteTemplates, error) {
		model := invites.Model()
		return inviteTemplates{Systems: model.Systems, Greetings: visibleGreetings(model.Greetings, effectiveEmail(cache, r))}, nil
	}))

	mux.HandleFunc("POST /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		format := strings.TrimSpace(r.FormValue("format"))
		original := strings.TrimSpace(r.FormValue("original"))
		actor := requestActor(cache, r)
		ops, err := invites.saveGreeting(actor, format, original, r.FormValue("grouped") == "1", r.FormValue("individual") == "1")
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := invites.Commit(r.Context(), actor, ops...); err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "greeting: saved", "actor", actor.Email, "from", original, "name", format)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		actor := requestActor(cache, r)
		ops, err := invites.deleteGreeting(actor, name)
		if err != nil {
			serve.Error(w, r, err)
			return
		}
		if err := invites.Commit(r.Context(), actor, ops...); err != nil {
			serve.Error(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "greeting: deleted", "actor", actor.Email, "name", name)
		w.WriteHeader(http.StatusNoContent)
	})
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
