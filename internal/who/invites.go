package who

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"heliosian/internal/cells"
	"heliosian/internal/data"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

type InviteTemplate struct {
	Name           string     `json:"name"`
	Sheet          string     `json:"sheet"`
	Description    string     `json:"description,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	HeaderRow      bool       `json:"headerRow"`
	SupportsGroups bool       `json:"supportsGroups"`
	Header         []string   `json:"header"`
	Rows           [][]string `json:"rows"`
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
	servicesTab  = "_Services"
	greetingsTab = "_Greetings"
)

var GreetingColumns = []string{"Name", "Format", "Grouped", "Individual", "Email"}

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

func loadInviteTemplates(source data.Source) ([]InviteTemplate, error) {
	_, systems, err := source.Table(invitesApp, servicesTab)
	if err != nil {
		return nil, err
	}
	templates := []InviteTemplate{}
	for _, sys := range systems {
		name, tab := sys["Display Name"], sys["Sheet"]
		if name == "" || tab == "" {
			continue
		}
		raw, err := source.Raw(invitesApp, tab)
		if err != nil {
			return nil, fmt.Errorf("load %s template (tab %q): %w", name, tab, err)
		}
		if len(raw) == 0 {
			continue
		}
		headerRow, err := cells.YesNo(sys["Header Row"], true)
		if err != nil {
			return nil, fmt.Errorf("%s template: header row %w", name, err)
		}
		supportsGroups, err := cells.YesNo(sys["Supports Groups"], true)
		if err != nil {
			return nil, fmt.Errorf("%s template: supports groups %w", name, err)
		}
		templates = append(templates, InviteTemplate{
			Name:           name,
			Sheet:          tab,
			Description:    sys["Description"],
			Notes:          sys["Notes"],
			HeaderRow:      headerRow,
			SupportsGroups: supportsGroups,
			Header:         raw[0],
			Rows:           raw[1:],
		})
	}
	return templates, nil
}

type Invites struct {
	*store.Store[[]GreetingTemplate]
	source  data.Source
	mu      sync.RWMutex
	systems []InviteTemplate
}

func NewInvites(source data.Source, writer data.Writer, queue *store.Queue) (*Invites, error) {
	s, err := store.New(store.Spec[[]GreetingTemplate]{
		App:  invitesApp,
		Tabs: []store.Tab{{Name: greetingsTab, Columns: GreetingColumns, Key: []string{"Name"}}},
		Build: func(_ context.Context, tables store.Tables) ([]GreetingTemplate, error) {
			return buildGreetings(tables)
		},
		Loaded: func(greetings []GreetingTemplate, took time.Duration) {
			slog.Info("loaded greetings", "greetings", len(greetings), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	i := &Invites{Store: s, source: source}
	swap, err := i.loadSystems(context.Background())
	if err != nil {
		return nil, err
	}
	swap()
	queue.Register(i.loadSystems)
	return i, nil
}

func (i *Invites) loadSystems(context.Context) (func(), error) {
	systems, err := loadInviteTemplates(i.source)
	if err != nil {
		return nil, fmt.Errorf("load invite templates: %w", err)
	}
	return func() {
		i.mu.Lock()
		i.systems = systems
		i.mu.Unlock()
		slog.Info("loaded invite templates", "systems", len(systems))
	}, nil
}

func (i *Invites) Systems() []InviteTemplate {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.systems
}

func (i *Invites) greeting(name string) (GreetingTemplate, bool) {
	for _, g := range i.Model() {
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
		return inviteTemplates{Systems: invites.Systems(), Greetings: visibleGreetings(invites.Model(), effectiveEmail(cache, r))}, nil
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
