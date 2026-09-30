package model

import (
	"net/http"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
)

const (
	partiesHost          = "celebrate"
	kindParty            = "party"
	kindPartiesSettings  = "celebrate-settings"
	kindPartiesAddresses = "celebrate-addresses"
)

type partyResource struct {
	PartyView
	PartyID string   `json:"partyId"`
	Path    string   `json:"path"`
	App     string   `json:"app"`
	Admits  []string `json:"admits"`
}

type ticketResource struct {
	PartyAttendee
}

type partiesSettingsResource struct {
	User        PartiesUser     `json:"user"`
	Today       string          `json:"today"`
	Settings    PartiesSettings `json:"settings"`
	Current     string          `json:"current,omitempty"`
	Banner      string          `json:"banner,omitempty"`
	Invoicing   []InvoiceLine   `json:"invoicing,omitempty"`
	ImageSearch bool            `json:"imageSearch"`
	Billable    []string        `json:"billable"`
}

type addressesResource struct {
	Problems []Problem `json:"problems"`
	Moved    []Moved   `json:"moved"`
}

type partyStatus struct {
	Status string `json:"status"`
}

type partyCategoryBody struct {
	Title string `json:"title"`
}

type partyCategoryOrder struct {
	IDs []string `json:"ids"`
}

func (h PartiesHooks) Resources() []api.Type[*Model] {
	a := h.app
	return []api.Type[*Model]{a.partiesType(), a.ticketsType(), a.celebrationsType(), a.categoriesType(), a.settingsType(), a.addressesType()}
}

func (a partiesApp) partyKey(partyID string) string {
	return id.Of(a.calendar.idKey, kindParty, partyID)
}

func (a partiesApp) settingsKey() string {
	return id.Of(a.calendar.idKey, kindPartiesSettings, "")
}

func (a partiesApp) addressesKey() string {
	return id.Of(a.calendar.idKey, kindPartiesAddresses, "")
}

func (a partiesApp) partyAt(m *Model, key string) *Party {
	s := m.scope
	s.partiesOnce.Do(func() {
		s.partyKeys = map[string]*Party{}
		for _, p := range m.Parties.Parties {
			s.partyKeys[a.partyKey(p.ID)] = p
		}
	})
	return s.partyKeys[key]
}

func (a partiesApp) visibleParty(m *Model, q api.Query, key string) *Party {
	p := a.partyAt(m, key)
	if p == nil || !p.VisibleTo(q.Actor) {
		return nil
	}
	return p
}

func (a partiesApp) ticketAt(m *Model, q api.Query, key string) (*Ticket, *Party) {
	t, p := m.Parties.byTicket[key], m.Parties.ticketParties[key]
	if t == nil || !p.VisibleTo(q.Actor) {
		return nil, nil
	}
	return t, p
}

func (a partiesApp) viewer(m *Model, q api.Query) partyViewer {
	return partyViewer{Actor: q.Actor, directory: m.Directory, rsvps: a.calendar.at(m).guestAnswers}
}

func householdOf(d *Directory, email string) []*Person {
	out := []*Person{}
	if me := d.Person(d.Resolve(email)); me != nil {
		out = append(out, me)
	}
	adults, kids := d.Household(email)
	for _, p := range slices.Concat(adults, kids) {
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func (a partiesApp) partyResource(m *Model, q api.Query, p *Party) partyResource {
	out := partyResource{PartyView: a.viewer(m, q).party(p, q.Now), PartyID: p.ID, Path: m.Parties.PathOf(p), App: partiesHost, Admits: []string{}}
	for _, person := range householdOf(m.Directory, q.Actor.Email) {
		held := slices.ContainsFunc(p.Tickets, func(t Ticket) bool { return t.Email == person.Email && t.Status == TicketSold })
		if p.Admits(person) && !held {
			out.Admits = append(out.Admits, person.Email)
		}
	}
	return out
}

func (a partiesApp) partyAliases(m *Model) map[string]string {
	ps := m.Parties
	out := map[string]string{}
	for _, p := range ps.Parties {
		key := a.partyKey(p.ID)
		out[p.ID] = key
		if p.PrettyID != "" {
			out[p.PrettyID] = key
		}
	}
	for old, target := range ps.aliases {
		if p := ps.byParty[target]; p != nil {
			out[old] = a.partyKey(p.ID)
		}
	}
	for _, r := range ps.Redirects {
		p := ps.Resolve(r.Old)
		if p == nil {
			continue
		}
		segment := r.Old[strings.LastIndex(r.Old, "/")+1:]
		if _, taken := out[segment]; !taken {
			out[segment] = a.partyKey(p.ID)
		}
	}
	return out
}

func (a partiesApp) onParty(can func(m *Model, q api.Query, p *Party) bool) func(*Model, api.Query, string) bool {
	return func(m *Model, q api.Query, key string) bool {
		p := a.visibleParty(m, q, key)
		return p != nil && can(m, q, p)
	}
}

func (a partiesApp) onTicket(can func(m *Model, q api.Query, t *Ticket, p *Party) bool) func(*Model, api.Query, string) bool {
	return func(m *Model, q api.Query, key string) bool {
		t, p := a.ticketAt(m, q, key)
		return t != nil && can(m, q, t, p)
	}
}

func may(allowance access.Allowance) func(*Model, api.Query, string) bool {
	return func(_ *Model, q api.Query, _ string) bool { return q.Actor.May(allowance) }
}

func (a partiesApp) mailTaken(r *http.Request, got taken, actor string) {
	byPurchaser := map[string][]map[string]string{}
	order := []string{}
	for _, cells := range got.added {
		if _, seen := byPurchaser[cells["Purchaser"]]; !seen {
			order = append(order, cells["Purchaser"])
		}
		byPurchaser[cells["Purchaser"]] = append(byPurchaser[cells["Purchaser"]], cells)
	}
	for _, who := range order {
		a.mailTickets(r, got.party, who, byPurchaser[who], actor)
	}
}

func (a partiesApp) partiesType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "parties",
		Shape: partyResource{},
		Has:   func(m *Model, key string) bool { return a.partyAt(m, key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			p := a.visibleParty(m, q, key)
			if p == nil {
				return nil, false
			}
			return a.partyResource(m, q, p), true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, p := range m.Parties.SortedParties("") {
				if p.VisibleTo(q.Actor) {
					out = append(out, a.partyKey(p.ID))
				}
			}
			return out
		},
		Aliases: a.partyAliases,
		Filters: map[string]api.Filter[*Model]{
			"celebration": func(m *Model, _ api.Query, value string) (func(string) bool, error) {
				c := m.Parties.CelebrationByID(value)
				if c == nil {
					return nil, access.Invalid("no celebration %s", value)
				}
				return func(key string) bool {
					p := a.partyAt(m, key)
					return p != nil && p.Celebration == c.ID
				}, nil
			},
		},
		Relations: map[string]api.Relation[*Model]{
			"event": {Type: "events", List: func(m *Model, q api.Query, key string) []string {
				p := a.visibleParty(m, q, key)
				if p == nil {
					return nil
				}
				if e := a.calendar.at(m).viewerEvent(q, p.ID); e != nil {
					return []string{e.ID}
				}
				return nil
			}},
			"hosts": {Type: "people", Many: true, List: func(m *Model, q api.Query, key string) []string {
				p := a.visibleParty(m, q, key)
				if p == nil {
					return nil
				}
				out := []string{}
				for _, h := range p.HostEmails {
					out = append(out, m.personID(h)...)
				}
				return out
			}},
			"tickets": {Type: "tickets", Many: true, List: func(m *Model, q api.Query, key string) []string {
				p := a.visibleParty(m, q, key)
				if p == nil {
					return nil
				}
				out := []string{}
				for _, t := range p.Tickets {
					out = append(out, t.ID)
				}
				return out
			}},
			"celebration": {Type: "celebrations", List: func(m *Model, q api.Query, key string) []string {
				if p := a.visibleParty(m, q, key); p != nil {
					return []string{p.Celebration}
				}
				return nil
			}},
			"category": {Type: "party-categories", List: func(m *Model, q api.Query, key string) []string {
				if p := a.visibleParty(m, q, key); p != nil && p.Category != "" {
					return []string{p.Category}
				}
				return nil
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(a.onParty(func(_ *Model, q api.Query, p *Party) bool { return p.Edits(q.Actor) }), func(wr api.Write[*Model]) partyBody {
				return partyBodyOf(a.partyAt(wr.S, wr.ID))
			}, func(wr api.Write[*Model], body partyBody) error {
				body.ID = a.partyAt(wr.S, wr.ID).ID
				saved, err := wr.S.Parties.saveParty(wr.Query.Actor, body, sent(wr.Body))
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: saved party", "action", "edit", "party", saved.title, "status", saved.status)
				return stage(wr, partiesAppName, saved.ops, nil)
			}),
			"delete": api.Do(a.onParty(func(_ *Model, q api.Query, p *Party) bool {
				return q.Actor.May(CurateParties) && len(p.Tickets) == 0
			}), func(wr api.Write[*Model], _ serve.None) error {
				p, ops, err := wr.S.Parties.deleteParty(wr.Query.Actor, a.partyAt(wr.S, wr.ID).ID)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: removed party", "party", p.Title)
				return stage(wr, partiesAppName, ops, nil)
			}),
			"status": api.Do(a.onParty(func(_ *Model, q api.Query, _ *Party) bool { return q.Actor.May(CurateParties) }), func(wr api.Write[*Model], body partyStatus) error {
				p, ops, err := wr.S.Parties.setStatus(wr.Query.Actor, a.partyAt(wr.S, wr.ID).ID, body.Status)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: set party status", "party", p.Title, "status", body.Status)
				return stage(wr, partiesAppName, ops, nil)
			}),
			"buy": api.Do(a.onParty(func(m *Model, q api.Query, p *Party) bool {
				return p.Edits(q.Actor) || (!isKid(m.Directory, q.Actor.Email) && p.Availability(q.Now) == Available)
			}), func(wr api.Write[*Model], body ticketOrder) error {
				body.PartyID = a.partyAt(wr.S, wr.ID).ID
				got, err := wr.S.Parties.takeTickets(wr.Query.Actor, wr.S.Directory, body)
				if err := stage(wr, partiesAppName, got.ops, err); err != nil {
					return err
				}
				logAfter(wr, "celebrate: tickets taken", "party", got.party.Title, "purchaser", got.purchaser, "sold", got.sold, "waitlisted", got.waitlisted)
				r, by := wr.Request, wr.Query.Actor.Email
				wr.Tx.After(func() { a.mailTaken(r, got, by) })
				return nil
			}),
			"join-waitlist": api.Do(a.onParty(func(m *Model, q api.Query, p *Party) bool {
				return !isKid(m.Directory, q.Actor.Email) && (p.Edits(q.Actor) || p.Availability(q.Now) == Waitlist)
			}), func(wr api.Write[*Model], body waitlistOrder) error {
				body.PartyID = a.partyAt(wr.S, wr.ID).ID
				got, err := wr.S.Parties.joinWaitlist(wr.Query.Actor, wr.S.Directory, body)
				if err := stage(wr, partiesAppName, got.ops, err); err != nil {
					return err
				}
				event := "celebrate: joined waitlist"
				if got.changed {
					event = "celebrate: waitlist request changed"
				}
				logAfter(wr, event, "party", got.party.Title, "purchaser", got.purchaser, "quantity", body.Quantity)
				r, by := wr.Request, wr.Query.Actor.Email
				wr.Tx.After(func() { a.mailTickets(r, got.party, got.purchaser, []map[string]string{got.cells}, by) })
				return nil
			}),
		},
	}
}

func (a partiesApp) ticketsType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "tickets",
		Shape: ticketResource{},
		Has:   func(m *Model, key string) bool { return m.Parties.byTicket[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			t, p := a.ticketAt(m, q, key)
			if t == nil {
				return nil, false
			}
			shown := p.For(q.Actor, m.Directory)
			i := slices.IndexFunc(shown.Tickets, func(s Ticket) bool { return s.ID == t.ID })
			return ticketResource{a.viewer(m, q).attendee(shown.Tickets[i], p.Sees(q.Actor))}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, p := range m.Parties.SortedParties("") {
				if !p.VisibleTo(q.Actor) {
					continue
				}
				for _, t := range p.Tickets {
					out = append(out, t.ID)
				}
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for old, target := range m.Parties.aliases {
				if m.Parties.byTicket[target] != nil {
					out[old] = target
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"party": {Type: "parties", List: func(m *Model, q api.Query, key string) []string {
				if t, p := a.ticketAt(m, q, key); t != nil {
					return []string{a.partyKey(p.ID)}
				}
				return nil
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(a.onTicket(func(_ *Model, q api.Query, t *Ticket, p *Party) bool {
				return p.Edits(q.Actor) || owns(t, q.Actor)
			}), func(wr api.Write[*Model], body ticketEdit) error {
				body.TicketID = wr.ID
				t, p, ops, details, err := wr.S.Parties.editTicket(wr.Query.Actor, body)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: ticket edited", "party", p.Title, "who", ticketHolder(t), "details", details)
				return stage(wr, partiesAppName, ops, nil)
			}),
			"delete": api.Do(a.onTicket(func(_ *Model, q api.Query, t *Ticket, p *Party) bool {
				return p.Edits(q.Actor) || (owns(t, q.Actor) && t.Status == TicketWaitlist && !p.Past(q.Now))
			}), func(wr api.Write[*Model], _ serve.None) error {
				t, p, ops, err := wr.S.Parties.removeTicket(wr.Query.Actor, wr.ID)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: ticket removed", "party", p.Title, "who", ticketHolder(t), "was", t.Status)
				return stage(wr, partiesAppName, ops, nil)
			}),
			"reassign": api.Do(a.onTicket(func(m *Model, q api.Query, t *Ticket, p *Party) bool {
				return t.Status == TicketSold && (p.Edits(q.Actor) || (owns(t, q.Actor) && !isKid(m.Directory, q.Actor.Email) && !p.Past(q.Now)))
			}), func(wr api.Write[*Model], body reassignment) error {
				body.TicketID = wr.ID
				t, p, ops, who, err := wr.S.Parties.reassignTicket(wr.Query.Actor, wr.S.Directory, body)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: ticket reassigned", "party", p.Title, "from", ticketHolder(t), "to", who)
				return stage(wr, partiesAppName, ops, nil)
			}),
			"offer": api.Do(a.onTicket(func(_ *Model, q api.Query, t *Ticket, p *Party) bool {
				return p.Edits(q.Actor) && t.Status == TicketWaitlist
			}), func(wr api.Write[*Model], body offer) error {
				body.TicketID = wr.ID
				got, err := wr.S.Parties.offerTickets(wr.Query.Actor, wr.S.Directory, body)
				if err := stage(wr, partiesAppName, got.ops, err); err != nil {
					return err
				}
				purchaser := got.ticket.Purchaser
				logAfter(wr, "celebrate: offered tickets", "party", got.party.Title, "purchaser", purchaser, "offered", got.offered, "left", got.left)
				r, by := wr.Request, wr.Query.Actor.Email
				wr.Tx.After(func() { a.mailOffered(r, got.party, purchaser, got.added, by) })
				return nil
			}),
		},
	}
}

func celebrationFormOf(c *Celebration) celebrationForm {
	return celebrationForm{
		ID: c.ID, Code: c.Code, Title: c.Title, Subtitle: c.Subtitle, Start: c.Start, End: c.End, Location: c.Location, Address: c.Address,
		Description: c.Description, Image: c.Image, ButtonText: c.ButtonText, ButtonURL: c.ButtonURL, Current: c.Current, Banner: c.Banner,
	}
}

func (a partiesApp) celebrationsType() api.Type[*Model] {
	stage := a.store.stage
	configures := may(ConfigureParties)
	return api.Type[*Model]{
		Name:  "celebrations",
		Shape: Celebration{},
		Has:   func(m *Model, key string) bool { return m.Parties.CelebrationByID(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			c := m.Parties.CelebrationByID(key)
			if c == nil {
				return nil, false
			}
			return *c, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Parties.Celebrations {
				out = append(out, c.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, c := range m.Parties.Celebrations {
				out[c.Code] = c.ID
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body celebrationForm) (string, error) {
			body.ID = ""
			ops, key, err := wr.S.Parties.saveCelebration(wr.Query.Actor, body)
			if err := stage(wr, partiesAppName, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "celebrate: saved celebration", "action", "add", "code", strings.TrimSpace(body.Code))
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(configures, func(wr api.Write[*Model]) celebrationForm {
				return celebrationFormOf(wr.S.Parties.CelebrationByID(wr.ID))
			}, func(wr api.Write[*Model], body celebrationForm) error {
				body.ID = wr.ID
				ops, _, err := wr.S.Parties.saveCelebration(wr.Query.Actor, body)
				logAfter(wr, "celebrate: saved celebration", "action", "edit", "code", strings.TrimSpace(body.Code))
				return stage(wr, partiesAppName, ops, err)
			}),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				return q.Actor.May(ConfigureParties) && !slices.ContainsFunc(m.Parties.Parties, func(p *Party) bool { return p.Celebration == key })
			}, func(wr api.Write[*Model], _ serve.None) error {
				c, ops, err := a.store.deleteCelebration(wr.Query.Actor, wr.ID)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: removed celebration", "code", c.Code)
				return stage(wr, partiesAppName, ops, nil)
			}),
		},
	}
}

func (a partiesApp) categoriesType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "party-categories",
		Shape: PartyCategory{},
		Has:   func(m *Model, key string) bool { return m.Parties.Category(key) != nil },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			c := m.Parties.Category(key)
			if c == nil {
				return nil, false
			}
			return *c, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, c := range m.Parties.Categories {
				out = append(out, c.ID)
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body partyCategoryBody) (string, error) {
			ops, key, err := wr.S.Parties.saveCategory(wr.Query.Actor, "", body.Title)
			if err := stage(wr, partiesAppName, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "celebrate: saved category", "action", "add", "category", strings.TrimSpace(body.Title))
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(may(ConfigureParties), func(wr api.Write[*Model]) partyCategoryBody {
				return partyCategoryBody{Title: wr.S.Parties.Category(wr.ID).Title}
			}, func(wr api.Write[*Model], body partyCategoryBody) error {
				ops, _, err := wr.S.Parties.saveCategory(wr.Query.Actor, wr.ID, body.Title)
				logAfter(wr, "celebrate: saved category", "action", "edit", "category", strings.TrimSpace(body.Title))
				return stage(wr, partiesAppName, ops, err)
			}),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				return q.Actor.May(ConfigureParties) && !slices.ContainsFunc(m.Parties.Parties, func(p *Party) bool { return p.Category == key })
			}, func(wr api.Write[*Model], _ serve.None) error {
				c, ops, err := a.store.deletePartyCategory(wr.Query.Actor, wr.ID)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: removed category", "category", c.Title)
				return stage(wr, partiesAppName, ops, nil)
			}),
		},
	}
}

func (a partiesApp) settingsType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "celebrate-settings",
		Shape: partiesSettingsResource{},
		Has:   func(_ *Model, key string) bool { return key == a.settingsKey() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != a.settingsKey() {
				return nil, false
			}
			ps := m.Parties
			out := partiesSettingsResource{
				User: partiesUserOf(m.Directory, q.Actor.Email), Today: q.Now.Format(DateFormat), Settings: ps.Settings,
				ImageSearch: a.search.On(), Billable: Billable(m.Directory, q.Actor.Email),
			}
			if c := ps.Current(); c != nil {
				out.Current = c.ID
			}
			if c := ps.Banner(); c != nil {
				out.Banner = c.ID
			}
			if q.Actor.May(SeeAllParties) {
				out.Invoicing = ps.Invoicing
			}
			return out, true
		},
		List: func(*Model, api.Query) []string { return []string{a.settingsKey()} },
		Relations: map[string]api.Relation[*Model]{
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
		},
		Actions: map[string]api.Action[*Model]{
			"host": api.Do(func(m *Model, q api.Query, _ string) bool {
				return q.Actor.May(CurateParties) || m.Parties.Settings.HostingOpen
			}, func(wr api.Write[*Model], body partyBody) error {
				body.ID = ""
				saved, err := wr.S.Parties.saveParty(wr.Query.Actor, body, nil)
				if err != nil {
					return err
				}
				logAfter(wr, "celebrate: saved party", "action", "add", "party", saved.title, "status", saved.status)
				return stage(wr, partiesAppName, saved.ops, nil)
			}),
			"settings": api.DoFrom(may(ConfigureParties), func(wr api.Write[*Model]) PartiesSettings {
				return wr.S.Parties.Settings
			}, func(wr api.Write[*Model], body PartiesSettings) error {
				ops, err := wr.S.Parties.saveSettings(wr.Query.Actor, body)
				logAfter(wr, "celebrate: changed the settings")
				return stage(wr, partiesAppName, ops, err)
			}),
			"order-categories": api.Do(may(ConfigureParties), func(wr api.Write[*Model], body partyCategoryOrder) error {
				ops, err := wr.S.Parties.reorderCategories(wr.Query.Actor, body.IDs)
				logAfter(wr, "celebrate: reordered categories", "changed", len(ops))
				return stage(wr, partiesAppName, ops, err)
			}),
			"move-address": api.Do(may(MoveAddresses), func(wr api.Write[*Model], body addressMove) error {
				ops, moved, err := wr.S.Parties.moveUnlisted(wr.Query.Actor, wr.S.Directory, body.Old, body.To, body.Name)
				if err := stage(wr, partiesAppName, ops, err); err != nil {
					return err
				}
				actor, ctx := wr.Query.Actor, wr.Request.Context()
				old, to, name := mail.Normalize(body.Old), mail.Normalize(body.To), strings.TrimSpace(body.Name)
				logAfter(wr, "celebrate: address moved", "from", old, "to", to, "tickets", moved)
				wr.Tx.After(func() { a.calendar.moveAddress(ctx, actor, old, to, name) })
				return nil
			}),
		},
	}
}

func (a partiesApp) addressesType() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  "celebrate-addresses",
		Shape: addressesResource{},
		Has:   func(_ *Model, key string) bool { return key == a.addressesKey() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != a.addressesKey() || !q.Actor.May(MoveAddresses) {
				return nil, false
			}
			return addressesResource{Problems: Problems(m.Parties, m.Directory, q.Now), Moved: m.Parties.MovedAddresses()}, true
		},
		List: func(*Model, api.Query) []string { return []string{a.addressesKey()} },
	}
}
