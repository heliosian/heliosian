package who

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"heliosian/internal/data"
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
	greetings := make([]GreetingTemplate, 0, len(rows))
	for _, row := range rows {
		name, format := row["Name"], row["Format"]
		if name == "" || format == "" {
			continue
		}
		greetings = append(greetings, GreetingTemplate{
			Name:       name,
			Format:     format,
			Grouped:    row["Grouped"] != "No",
			Individual: row["Individual"] != "No",
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
	templates := make([]InviteTemplate, 0, len(systems))
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
		templates = append(templates, InviteTemplate{
			Name:           name,
			Sheet:          tab,
			Description:    sys["Description"],
			Notes:          sys["Notes"],
			HeaderRow:      sys["Header Row"] != "No",
			SupportsGroups: sys["Supports Groups"] != "No",
			Header:         raw[0],
			Rows:           raw[1:],
		})
	}
	return templates, nil
}

const invitesRefreshInterval = 5 * time.Minute

type Invites struct {
	*store.Store[[]GreetingTemplate]
	source  data.Source
	queue   store.Enqueuer
	mu      sync.RWMutex
	systems []InviteTemplate
}

func NewInvites(source data.Source, writer data.Writer, queue store.Enqueuer) (*Invites, error) {
	s, err := store.New(store.Spec[[]GreetingTemplate]{
		App:   invitesApp,
		Tabs:  []store.Tab{{Name: greetingsTab, Columns: GreetingColumns, Key: []string{"Name"}}},
		Build: buildGreetings,
		Loaded: func(greetings []GreetingTemplate, took time.Duration) {
			slog.Info("loaded greetings", "greetings", len(greetings), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	systems, err := loadInviteTemplates(source)
	if err != nil {
		return nil, fmt.Errorf("load invite templates: %w", err)
	}
	slog.Info("loaded invite templates", "systems", len(systems))
	i := &Invites{Store: s, source: source, queue: queue, systems: systems}
	go i.refreshLoop()
	return i, nil
}

func (i *Invites) refreshLoop() {
	for range time.Tick(invitesRefreshInterval) {
		i.queue.Add(i.refreshSystems)
	}
}

func (i *Invites) refreshSystems() {
	systems, err := loadInviteTemplates(i.source)
	if err != nil {
		slog.Error("[ERROR] load invite templates", "error", err)
		return
	}
	i.mu.Lock()
	i.systems = systems
	i.mu.Unlock()
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

func RegisterInvites(mux *http.ServeMux, cache *Cache, invites *Invites) {
	mux.HandleFunc("GET /api/directory/invite-templates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		view := struct {
			Systems   []InviteTemplate   `json:"systems"`
			Greetings []GreetingTemplate `json:"greetings"`
		}{Systems: invites.Systems(), Greetings: visibleGreetings(invites.Model(), effectiveEmail(cache, r))}
		if err := json.NewEncoder(w).Encode(view); err != nil {
			slog.ErrorContext(r.Context(), "encode invite templates", "error", err)
		}
	})

	mux.HandleFunc("POST /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		format := strings.TrimSpace(r.FormValue("format"))
		original := strings.TrimSpace(r.FormValue("original"))
		if format == "" {
			http.Error(w, "format is required", http.StatusBadRequest)
			return
		}
		if len(format) > 200 {
			http.Error(w, "format is too long", http.StatusBadRequest)
			return
		}
		email := effectiveEmail(cache, r)
		cells := store.Row{
			"Name":       format,
			"Format":     format,
			"Grouped":    yesNo(r.FormValue("grouped") == "1"),
			"Individual": yesNo(r.FormValue("individual") == "1"),
			"Email":      email,
		}
		op := store.Insert(greetingsTab, cells)
		if original != "" {
			have, ok := invites.greeting(original)
			if !ok || have.CreatedBy != email {
				http.Error(w, "you can only edit greetings you created", http.StatusForbidden)
				return
			}
			op = store.Update(greetingsTab, store.Row{"Name": original}, cells)
		}
		if err := invites.Commit(r.Context(), email, op); err != nil {
			serverError(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "greeting: saved", "actor", email, "from", original, "name", format)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("DELETE /api/directory/greetings", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		email := effectiveEmail(cache, r)
		have, ok := invites.greeting(name)
		if !ok || have.CreatedBy != email {
			http.Error(w, "you can only delete greetings you created", http.StatusForbidden)
			return
		}
		if err := invites.Commit(r.Context(), email, store.Delete(greetingsTab, store.Row{"Name": name})); err != nil {
			serverError(w, r, err)
			return
		}
		slog.InfoContext(r.Context(), "greeting: deleted", "actor", email, "name", name)
		w.WriteHeader(http.StatusNoContent)
	})
}

func visibleGreetings(greetings []GreetingTemplate, email string) []GreetingTemplate {
	visible := make([]GreetingTemplate, 0, len(greetings))
	for _, g := range greetings {
		if g.CreatedBy == "" || g.CreatedBy == email {
			visible = append(visible, g)
		}
	}
	return visible
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
