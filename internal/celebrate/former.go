package celebrate

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/config"
)

type AddressMoved func(ctx context.Context, actor access.Actor, old, to, name string)

func MoveAddress(ctx context.Context, cache *Cache, actor access.Actor, old, to, name string) (int, error) {
	ops, moved, err := cache.Model().moveAddress(actor, old, to, name)
	if err != nil {
		return 0, err
	}
	if err := cache.Commit(ctx, actor, ops...); err != nil {
		return 0, err
	}
	return moved, nil
}

func (a app) moveAddress(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Old  string `json:"old"`
		To   string `json:"to"`
		Name string `json:"name"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, moved, err := a.cache.Model().moveUnlisted(actor, a.directory, body.Old, body.To, body.Name)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	old, to := config.NormalizeEmail(body.Old), config.NormalizeEmail(body.To)
	if a.moved != nil {
		a.moved(r.Context(), actor, old, to, strings.TrimSpace(body.Name))
	}
	slog.InfoContext(r.Context(), "celebrate: address moved", "actor", actor.Email, "from", old, "to", to, "tickets", moved)
	w.WriteHeader(http.StatusNoContent)
}

type AddressUse struct {
	PartyID string `json:"partyId"`
	Party   string `json:"party"`
	Path    string `json:"path"`
	Past    bool   `json:"past,omitempty"`
	Role    string `json:"role"`
}

type Problem struct {
	Email    string       `json:"email"`
	Name     string       `json:"name"`
	Uses     []AddressUse `json:"uses"`
	Upcoming bool         `json:"upcoming"`
}

type Moved struct {
	Old     string `json:"old"`
	New     string `json:"new"`
	Name    string `json:"name,omitempty"`
	Changed string `json:"changed,omitempty"`
}

func Problems(model *Model, directory Directory, now time.Time) []Problem {
	byEmail := map[string]*Problem{}
	order := []string{}
	note := func(email, name string, p *Party, role string) {
		email = model.CurrentAddress(directory.Resolve(config.NormalizeEmail(email)))
		if email == "" || !strings.HasSuffix(email, "@"+auth.Domain) {
			return
		}
		if _, known := directory.Person(email); known {
			return
		}
		pr := byEmail[email]
		if pr == nil {
			pr = &Problem{Email: email, Uses: []AddressUse{}}
			byEmail[email] = pr
			order = append(order, email)
		}
		if pr.Name == "" && name != "" {
			pr.Name = name
		}
		past := p.Past(now)
		use := AddressUse{PartyID: p.ID, Party: p.Title, Path: model.PathOf(p), Past: past, Role: role}
		if !slices.Contains(pr.Uses, use) {
			pr.Uses = append(pr.Uses, use)
		}
		pr.Upcoming = pr.Upcoming || !past
	}
	for _, p := range model.Parties {
		for _, h := range p.HostEmails {
			note(h, "", p, "Host")
		}
		for _, t := range p.Tickets {
			role := "Ticket"
			if t.Status == TicketWaitlist {
				role = "Waitlist"
			}
			if t.Email != "" {
				note(t.Email, t.Name, p, role)
			}
			if t.Purchaser != t.Email {
				note(t.Purchaser, "", p, "Billed")
			}
		}
	}
	out := []Problem{}
	for _, email := range order {
		pr := byEmail[email]
		if pr.Name == "" {
			pr.Name = DisplayName(email)
		}
		out = append(out, *pr)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Upcoming != out[j].Upcoming {
			return out[i].Upcoming
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (m *Model) MovedAddresses() []Moved {
	out := []Moved{}
	for old, f := range m.former {
		out = append(out, Moved{Old: old, New: f.New, Name: f.Name, Changed: f.Changed})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Changed != out[j].Changed {
			return out[i].Changed > out[j].Changed
		}
		return out[i].Old < out[j].Old
	})
	return out
}

func (a app) addresses(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	model := a.cache.Model()
	view := struct {
		Problems []Problem `json:"problems"`
		Moved    []Moved   `json:"moved"`
	}{Problems(model, a.directory, now()), model.MovedAddresses()}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "celebrate: encode addresses", "error", err)
	}
}
