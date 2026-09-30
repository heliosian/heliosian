package model

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/cells"
	"heliosian/internal/mail"
)

func (s *Store) movePartyAddress(ctx context.Context, actor access.Actor, old, to, name string) error {
	ops, _, err := s.Model().Parties.moveAddress(actor, old, to, name)
	if err != nil {
		return err
	}
	return s.Commit(ctx, actor, partiesAppName, ops...)
}

type addressMove struct {
	Old  string `json:"old"`
	To   string `json:"to"`
	Name string `json:"name"`
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

func Problems(m *Parties, directory *Directory, now time.Time) []Problem {
	byEmail := map[string]*Problem{}
	order := []string{}
	note := func(email, name string, p *Party, role string) {
		email = m.CurrentAddress(directory.Resolve(mail.Normalize(email)))
		if email == "" || !strings.HasSuffix(email, "@"+auth.Domain) {
			return
		}
		if directory.Member(email) {
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
		use := AddressUse{PartyID: p.ID, Party: p.Title, Path: m.PathOf(p), Past: past, Role: role}
		if !slices.Contains(pr.Uses, use) {
			pr.Uses = append(pr.Uses, use)
		}
		pr.Upcoming = pr.Upcoming || !past
	}
	for _, p := range m.Parties {
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
			pr.Name = cells.DisplayName(email)
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

func (m *Parties) MovedAddresses() []Moved {
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
