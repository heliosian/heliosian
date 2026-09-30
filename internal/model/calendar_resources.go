package model

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	appHost          = "when"
	kindGuestList    = "guest-list"
	kindCalendarFeed = "calendar-feed"
	kindSettings     = "when-settings"
)

func (a calendarApp) responses(q api.Query) map[string]*Responses {
	s := a.pinned.scope
	s.answerOnce.Do(func() {
		s.responses = a.model().ResponsesFor(q.Actor, a.directory())
	})
	return s.responses
}

func (a calendarApp) guestListID(eventID string) string {
	return id.Of(a.idKey, kindGuestList, eventID)
}

func (a calendarApp) feedID(email, token string) string {
	return id.Of(a.idKey, kindCalendarFeed, email+"\x00"+token)
}

func (a calendarApp) settingsID() string {
	return id.Of(a.idKey, kindSettings, "")
}

func (a calendarApp) viewerEvents(q api.Query) ([]*Event, map[string]*Event) {
	s := a.pinned.scope
	s.eventsOnce.Do(func() {
		model, directory := a.model(), a.directory()
		events := model.EventsFor(q.Actor, directory, a.linked(q.Actor.Email))
		out := []*Event{}
		byID := map[string]*Event{}
		for _, e := range events {
			c := a.decorated(q, e)
			out = append(out, c)
			byID[c.ID] = c
		}
		s.events, s.eventsByID = out, byID
	})
	return s.events, s.eventsByID
}

func (a calendarApp) decorated(q api.Query, e *Event) *Event {
	c := *e
	d := a.directory()
	c.Hosted = a.model().hostedBy(d, q.Actor.Email, e)
	a.model().goingFor(d, q.Actor.Email, &c)
	c.HostNames = nil
	for _, h := range e.Hosts {
		if p := d.Person(d.Resolve(mail.Normalize(h))); p != nil && p.FullName != "" {
			c.HostNames = append(c.HostNames, p.FullName)
		}
	}
	return &c
}

func (a calendarApp) viewerEvent(q api.Query, key string) *Event {
	if _, byID := a.viewerEvents(q); byID[key] != nil {
		return byID[key]
	}
	if e := a.eventFor(q.Actor, key); e != nil {
		return a.decorated(q, e)
	}
	return nil
}

func (a calendarApp) seesResponses(q api.Query, e *Event) bool {
	return q.Actor.May(SeeAllEvents) || (e.AddedBy != "" && !e.PosterLeft && mail.Normalize(e.AddedBy) == mail.Normalize(q.Actor.Email))
}

func (a calendarApp) index() {
	s := a.pinned.scope
	s.indexOnce.Do(func() {
		lists, feeds := map[string]string{}, map[string]string{}
		model := a.model()
		all := slices.Concat(model.Events, model.Pending)
		for _, e := range withLinked(nil, a.linked("")) {
			all = append(all, e)
		}
		for _, e := range all {
			if e.keepsGuestList() {
				lists[a.guestListID(e.ID)] = e.ID
			}
		}
		for _, f := range model.Feeds {
			feeds[a.feedID(f.Email, f.Token)] = f.Token
		}
		for _, p := range a.directory().People {
			if p.Email != "" {
				feeds[a.feedID(p.Email, MyHeliosianToken)] = MyHeliosianToken + "\x00" + p.Email
			}
		}
		s.guestLists, s.feeds = lists, feeds
	})
}

func (a calendarApp) has(key string) bool {
	return a.model().Event(key) != nil || slices.ContainsFunc(withLinked(nil, a.linked("")), func(e *Event) bool { return e.ID == key })
}

type eventMe struct {
	Answer string `json:"answer,omitempty"`
}

type eventResource struct {
	*Event
	Path       string      `json:"path"`
	App        string      `json:"app"`
	Slug       string      `json:"slug,omitempty"`
	Provenance *Provenance `json:"provenance,omitempty"`
	Responses  *Responses  `json:"responses,omitempty"`
	AdminOnly  bool        `json:"adminOnly,omitempty"`
	Me         eventMe     `json:"me"`
}

type settingsResource struct {
	User        CalendarUser                 `json:"user"`
	ImageSearch bool                         `json:"imageSearch"`
	Today       string                       `json:"today"`
	Classrooms  []RosterClassroom            `json:"classrooms"`
	Colors      map[string]string            `json:"colors"`
	Tags        []CalendarTag                `json:"tags"`
	DayTypes    []DayType                    `json:"dayTypes"`
	Years       []CalendarYear               `json:"years"`
	Days        map[string]map[string]string `json:"days"`
	GradeColors map[string]string            `json:"gradeColors,omitempty"`
	Names       map[string]string            `json:"names,omitempty"`
}

type guestListResource struct {
	InviteView
	Attendees []Attendee `json:"attendees,omitempty"`
	OnList    []string   `json:"onList"`
}

type feedResource struct {
	Feed
	URL  string               `json:"url,omitempty"`
	Days map[string][]DayKind `json:"days,omitempty"`
}

type calendarResources struct {
	app calendarApp
}

func (h CalendarHooks) Resources() []api.Type[*Model] {
	r := calendarResources{app: h.app}
	return []api.Type[*Model]{r.events(), r.guestLists(), r.inviteGroups(), r.feeds(), r.settings()}
}

func (r calendarResources) staged(wr api.Write[*Model]) calendarApp {
	return r.app.at(r.app.store.In(wr.Tx).at(wr.Query))
}

func sent(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(raw, &fields) == nil {
		for k := range fields {
			out[k] = true
		}
	}
	return out
}

type check func(a calendarApp, q api.Query, e *Event) bool

func action[In any](r calendarResources, can check, do func(wr api.Write[*Model], e *Event, in In) error) api.Action[*Model] {
	return seeded(r, can, func(*Event) In { var in In; return in }, do)
}

func seeded[In any](r calendarResources, can check, seed func(e *Event) In, do func(wr api.Write[*Model], e *Event, in In) error) api.Action[*Model] {
	return api.DoFrom(func(m *Model, q api.Query, key string) bool {
		a := r.app.at(m)
		e := a.viewerEvent(q, key)
		return e != nil && can(a, q, e)
	}, func(wr api.Write[*Model]) In {
		return seed(r.app.at(wr.S).viewerEvent(wr.Query, wr.ID))
	}, func(wr api.Write[*Model], in In) error {
		a := r.app.at(wr.S)
		e := a.viewerEvent(wr.Query, wr.ID)
		if e == nil || !can(a, wr.Query, e) {
			return access.Forbidden("that is not yours to do on this event")
		}
		return do(wr, e, in)
	})
}

func hosts(a calendarApp, q api.Query, e *Event) bool {
	return e.keepsGuestList() && a.isHost(q.Actor, e)
}

func curates(_ calendarApp, q api.Query, _ *Event) bool {
	return q.Actor.May(CurateCalendar)
}

func posted(e *Event) bool {
	return e.Source == SourceSheet
}

func (a calendarApp) inviter(q api.Query, e *Event) bool {
	if !e.keepsGuestList() {
		return false
	}
	if a.isHost(q.Actor, e) {
		return true
	}
	model := a.model()
	return model.othersInvite(e.ID) && (e.Sharing == SharingPublic || model.Invited(a.directory(), q.Actor.Email, e.ID))
}

func (a calendarApp) bringer(q api.Query, e *Event) bool {
	if !e.keepsGuestList() {
		return false
	}
	if a.isHost(q.Actor, e) {
		return true
	}
	model := a.model()
	return model.othersInvite(e.ID) && (e.guestsWithoutInvite() || model.Listed(a.directory(), q.Actor.Email, e.ID))
}

func (r calendarResources) eventWhere(keep func(a calendarApp, q api.Query, e *Event) bool) func(*Model, api.Query) func(string) bool {
	return func(m *Model, q api.Query) func(string) bool {
		a := r.app.at(m)
		return func(key string) bool {
			e := a.viewerEvent(q, key)
			return e != nil && keep(a, q, e)
		}
	}
}

func dateParam(name, value string) (string, error) {
	if _, err := time.Parse(DateFormat, value); err != nil {
		return "", access.Invalid("%s %q is not a date (YYYY-MM-DD)", name, value)
	}
	return value, nil
}

func (r calendarResources) eventFilters() map[string]api.Filter[*Model] {
	return map[string]api.Filter[*Model]{
		"from": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			from, err := dateParam("from", value)
			if err != nil {
				return nil, err
			}
			return r.eventWhere(func(_ calendarApp, _ api.Query, e *Event) bool { return e.end.Format(DateFormat) >= from })(m, q), nil
		},
		"to": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			to, err := dateParam("to", value)
			if err != nil {
				return nil, err
			}
			return r.eventWhere(func(_ calendarApp, _ api.Query, e *Event) bool { return e.start.Format(DateFormat) <= to })(m, q), nil
		},
		"calendar": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			a := r.app.at(m)
			a.index()
			if _, known := a.pinned.scope.feeds[value]; !known {
				return nil, access.Invalid("no calendar %s", value)
			}
			f, ok := a.feedOf(q, value)
			if !ok {
				return nil, access.Invalid("calendar %s is not yours", value)
			}
			classrooms, tags := a.model().calendarView(a.directory(), f.Feed)
			return r.eventWhere(func(a calendarApp, q api.Query, e *Event) bool {
				return a.model().InView(e, a.model().AnswerOf(q.Actor.Email, e.ID), classrooms, tags)
			})(m, q), nil
		},
		"waiting": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			if value != "" && value != "true" {
				return nil, access.Invalid("waiting takes no value")
			}
			today := q.Now.Format(DateFormat)
			return r.eventWhere(func(a calendarApp, q api.Query, e *Event) bool {
				return e.Invited && !e.Cancelled && a.model().AnswerOf(q.Actor.Email, e.ID) == "" && e.end.Format(DateFormat) >= today && !a.isHost(access.Actor{Email: q.Actor.Email}, e)
			})(m, q), nil
		},
		"app": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			return r.eventWhere(func(_ calendarApp, _ api.Query, e *Event) bool {
				app, _ := EventPage(e)
				return app == value
			})(m, q), nil
		},
		"status": func(m *Model, q api.Query, value string) (func(string) bool, error) {
			if !strings.EqualFold(value, StatusPending) {
				return nil, access.Invalid("status takes pending")
			}
			return r.eventWhere(func(_ calendarApp, _ api.Query, e *Event) bool { return e.Pending && !e.Declined && !e.Cancelled })(m, q), nil
		},
	}
}

func (r calendarResources) events() api.Type[*Model] {
	stage := r.app.store.stage
	editable := func(e *Event) eventBody {
		tags := slices.DeleteFunc(slices.Clone(e.Tags), BuiltInTag)
		source := e.SourceURL
		if source == "" {
			source = e.SourceNote
		}
		return eventBody{ID: e.ID, Title: e.Title, Start: e.Start, End: e.End, Location: e.Location, Description: e.Description, Tags: tags, DayType: e.DayType, Keywords: e.Keywords, Source: source, Image: e.Image, Sharing: e.Sharing}
	}
	correctable := func(e *Event) overrideBody {
		return overrideBody{ID: e.ID, Title: e.Title, Start: e.Start, End: e.End, Location: e.Location, Description: e.Description, Tags: slices.DeleteFunc(slices.Clone(e.Tags), BuiltInTag), Keywords: e.Keywords, Address: e.Address}
	}
	return api.Type[*Model]{
		Name:  "events",
		Shape: eventResource{},
		Has:   func(m *Model, key string) bool { return r.app.at(m).has(key) },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			e := a.viewerEvent(q, key)
			if e == nil {
				return nil, false
			}
			app, path := EventPage(e)
			out := eventResource{Event: e, Path: path, App: app, Slug: e.Address, Me: eventMe{Answer: a.model().AnswerOf(q.Actor.Email, e.ID)}}
			if q.Actor.May(SeeAllEvents) {
				out.Provenance = a.model().Provenance[e.ID]
				out.AdminOnly = !a.sees(access.Actor{Email: q.Actor.Email, Household: q.Actor.Household}, e)
			}
			if a.seesResponses(q, e) {
				out.Responses = &Responses{}
				if r := a.responses(q)[e.ID]; r != nil {
					out.Responses = r
				}
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			events, _ := r.app.at(m).viewerEvents(q)
			out := []string{}
			for _, e := range events {
				out = append(out, e.ID)
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			model := r.app.at(m).model()
			out := map[string]string{}
			for alias, target := range model.aliases {
				out[alias] = target
			}
			for address, e := range model.byAddress {
				out[address] = e.ID
			}
			return out
		},
		Filters: r.eventFilters(),
		Relations: map[string]api.Relation[*Model]{
			"hosts": {Type: "people", Many: true, List: func(m *Model, q api.Query, key string) []string {
				a := r.app.at(m)
				e := a.viewerEvent(q, key)
				if e == nil {
					return nil
				}
				out := []string{}
				for _, h := range a.hostsOf(e) {
					if p := a.directory().Person(h); p != nil && p.ID != "" {
						out = append(out, p.ID)
					}
				}
				return out
			}},
			"guest-list": {Type: "guest-lists", List: func(m *Model, q api.Query, key string) []string {
				a := r.app.at(m)
				e := a.viewerEvent(q, key)
				if e == nil || !e.keepsGuestList() {
					return nil
				}
				return []string{a.guestListID(e.ID)}
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], body eventBody) (string, error) {
			ops, ids, pending, err := r.app.at(wr.S).newEvents(wr.Query.Actor, body)
			if err := stage(wr, CalendarApp, ops, err); err != nil {
				return "", err
			}
			after := r.staged(wr)
			for _, key := range ids {
				yes, _, err := after.answerOps(wr.Query.Actor, wr.Query.Actor.Email, key, AnswerYes, ViaPage, false)
				if err := stage(wr, CalendarApp, yes, err); err != nil {
					return "", err
				}
			}
			logAfter(wr, "calendar: events added", "title", strings.TrimSpace(body.Title), "count", len(ids), "pending", pending)
			return ids[0], nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": seeded(r, func(a calendarApp, q api.Query, e *Event) bool { return posted(e) && a.isHost(q.Actor, e) }, editable, func(wr api.Write[*Model], e *Event, body eventBody) error {
				_, _, row, err := r.app.at(wr.S).changeEvent(wr.Query.Actor, body)
				if err != nil {
					return err
				}
				keep := sent(wr.Body)
				owns := map[string][]string{"title": {"Title"}, "start": {"Start", "End"}, "end": {"End"}, "location": {"Location"}, "description": {"Description"}, "tags": {"Tags"}, "keywords": {"Keywords"}, "source": {"Source"}, "image": {"Image"}, "sharing": {"Sharing", "Status", "Admins Told"}}
				cells := store.Row{}
				for field, columns := range owns {
					for _, column := range columns {
						if value, ok := row[column]; ok && keep[field] {
							cells[column] = value
						}
					}
				}
				logAfter(wr, "calendar: event changed", "event", e.ID, "title", body.Title)
				return stage(wr, CalendarApp, []store.Op{eventCellsOp(e.ID, cells)}, nil)
			}),
			"correct": seeded(r, func(a calendarApp, q api.Query, e *Event) bool {
				return e.imported() && (q.Actor.May(CurateCalendar) || a.isHost(q.Actor, e))
			}, correctable, func(wr api.Write[*Model], e *Event, body overrideBody) error {
				ops, key, empty, err := r.app.at(wr.S).overrideOps(wr.Query.Actor, body)
				logAfter(wr, "calendar: override set", "event", key, "cleared", empty)
				return stage(wr, CalendarApp, ops, err)
			}),
			"image": action(r, func(a calendarApp, q api.Query, e *Event) bool {
				return e.imported() && (q.Actor.May(CurateCalendar) || a.isHost(q.Actor, e))
			}, func(wr api.Write[*Model], e *Event, body overrideImageBody) error {
				ops, _, image, err := r.app.at(wr.S).overrideImageOps(wr.Query.Actor, e.ID, body.Image)
				logAfter(wr, "calendar: override image", "event", e.ID, "image", image)
				return stage(wr, CalendarApp, ops, err)
			}),
			"keywords": action(r, curates, func(wr api.Write[*Model], e *Event, body keywordsBody) error {
				ops, _, cell, err := r.app.at(wr.S).keywordOps(wr.Query.Actor, e.ID, body.Keywords)
				logAfter(wr, "calendar: keywords set", "event", e.ID, "keywords", cell)
				return stage(wr, CalendarApp, ops, err)
			}),
			"approve": action(r, func(a calendarApp, q api.Query, e *Event) bool { return q.Actor.May(CurateCalendar) && posted(e) }, func(wr api.Write[*Model], e *Event, _ serve.None) error {
				ops, _, err := r.app.at(wr.S).statusOps(wr.Query.Actor, e.ID, StatusApproved)
				logAfter(wr, "calendar: event approved", "event", e.ID, "title", e.Title, "by", e.AddedBy)
				return stage(wr, CalendarApp, ops, err)
			}),
			"decline": action(r, func(a calendarApp, q api.Query, e *Event) bool {
				return q.Actor.May(CurateCalendar) && posted(e) && e.Sharing == SharingPublic
			}, func(wr api.Write[*Model], e *Event, _ serve.None) error {
				ops, _, err := r.app.at(wr.S).statusOps(wr.Query.Actor, e.ID, StatusDeclined)
				logAfter(wr, "calendar: event declined", "event", e.ID, "title", e.Title, "by", e.AddedBy)
				return stage(wr, CalendarApp, ops, err)
			}),
			"move": action(r, func(a calendarApp, q api.Query, e *Event) bool { return q.Actor.May(CurateCalendar) && posted(e) }, func(wr api.Write[*Model], e *Event, body moveBody) error {
				start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
				if end == "" {
					end = start
				}
				ops, _, err := r.app.at(wr.S).moveOps(wr.Query.Actor, e.ID, start, end)
				logAfter(wr, "calendar: event moved", "event", e.ID, "start", start, "end", end)
				return stage(wr, CalendarApp, ops, err)
			}),
			"cancel": action(r, func(a calendarApp, q api.Query, e *Event) bool { return posted(e) && hosts(a, q, e) && !e.Cancelled }, func(wr api.Write[*Model], e *Event, body cancelBody) error {
				ops, err := r.app.at(wr.S).cancelWith(wr.Query.Actor, e, body)
				logAfter(wr, "calendar: event cancelled", "event", e.ID, "title", e.Title)
				return stage(wr, CalendarApp, ops, err)
			}),
			"answer": action(r, func(calendarApp, api.Query, *Event) bool { return true }, func(wr api.Write[*Model], e *Event, body rsvpBody) error {
				ops, _, err := r.app.at(wr.S).answerOps(wr.Query.Actor, wr.Query.Actor.Email, e.ID, body.Answer, ViaPage, true)
				logAfter(wr, "calendar: answered", "event", e.ID, "answer", body.Answer)
				return stage(wr, CalendarApp, ops, err)
			}),
			"answer-for": action(r, func(calendarApp, api.Query, *Event) bool { return true }, func(wr api.Write[*Model], e *Event, body answerForBody) error {
				a := r.app.at(wr.S)
				_, subject, err := a.answerSubject(wr.Query.Actor, e.ID, body.Email, body.Answer)
				if err != nil {
					return err
				}
				ops, _, err := a.answerOps(wr.Query.Actor, subject, e.ID, body.Answer, ViaPage, subject == wr.Query.Actor.Email)
				logAfter(wr, "calendar: answered for", "subject", subject, "event", e.ID, "answer", body.Answer)
				return stage(wr, CalendarApp, ops, err)
			}),
			"invite": action(r, func(a calendarApp, q api.Query, e *Event) bool { return a.inviter(q, e) }, func(wr api.Write[*Model], e *Event, body inviteesBody) error {
				ops, _, emails, host, err := r.app.at(wr.S).inviteOps(wr.Query.Actor, e.ID, body.People)
				if err := stage(wr, CalendarApp, ops, err); err != nil {
					return err
				}
				logAfter(wr, "calendar: guests added", "event", e.ID, "count", len(emails), "host", host)
				if host {
					return nil
				}
				return stage(wr, CalendarApp, r.staged(wr).requestOps(wr.Query.Actor, e, emails, wr.Query.Actor.Email, ""), nil)
			}),
			"uninvite": action(r, func(a calendarApp, q api.Query, e *Event) bool {
				return e.keepsGuestList() && (a.isHost(q.Actor, e) || slices.ContainsFunc(a.model().Invites[e.ID], func(inv Invite) bool { return inv.GuestOf != "" && a.mayAnswerFor(q.Actor, inv.GuestOf, e) }))
			}, func(wr api.Write[*Model], e *Event, body personBody) error {
				ops, _, email, fromGroup, err := r.app.at(wr.S).uninviteOps(wr.Query.Actor, e.ID, body.Email)
				logAfter(wr, "calendar: guest removed", "event", e.ID, "email", email, "from group", fromGroup)
				return stage(wr, CalendarApp, ops, err)
			}),
			"bring-guest": action(r, func(a calendarApp, q api.Query, e *Event) bool { return a.bringer(q, e) }, func(wr api.Write[*Model], e *Event, body guestBody) error {
				body.ID = e.ID
				ops, g, err := r.app.at(wr.S).bringGuestOps(wr.Query.Actor, body)
				if err := stage(wr, CalendarApp, ops, err); err != nil {
					return err
				}
				logAfter(wr, "calendar: guest brought", "event", e.ID, "of", g.of, "guest", g.key, "answer", g.answer, "invite", g.invite)
				if !g.invite || isGuestKey(g.key) {
					return nil
				}
				return stage(wr, CalendarApp, r.staged(wr).requestOps(wr.Query.Actor, e, []string{g.key}, wr.Query.Actor.Email, ""), nil)
			}),
			"settings": action(r, hosts, func(wr api.Write[*Model], e *Event, body settingsBody) error {
				body.ID = e.ID
				ops, _, _, err := r.app.at(wr.S).settingsOps(wr.Query.Actor, body)
				logAfter(wr, "calendar: guest list settings", "event", e.ID)
				return stage(wr, CalendarApp, ops, err)
			}),
			"step-down": action(r, hosts, func(wr api.Write[*Model], e *Event, body personBody) error {
				ops, _, who, poster, err := r.app.at(wr.S).stepDownOps(wr.Query.Actor, e.ID, body.Email)
				logAfter(wr, "calendar: host stepped down", "who", who, "event", e.ID, "poster", poster)
				return stage(wr, CalendarApp, ops, err)
			}),
			"send": action(r, hosts, func(wr api.Write[*Model], e *Event, body sendBody) error {
				a := r.app.at(wr.S)
				emails, kind, err := a.sendChoice(e, body)
				if err != nil {
					return err
				}
				logAfter(wr, "calendar: invites sent", "event", e.ID, "invites", len(emails), "kind", kind)
				return stage(wr, CalendarApp, a.requestOps(wr.Query.Actor, e, emails, wr.Query.Actor.Email, kind), nil)
			}),
			"skip": action(r, hosts, func(wr api.Write[*Model], e *Event, body skipBody) error {
				a := r.app.at(wr.S)
				emails := a.skippable(e, body.Emails)
				if len(emails) == 0 {
					return access.Invalid("nobody pending to skip")
				}
				logAfter(wr, "calendar: invites skipped", "event", e.ID, "skipped", len(emails))
				return stage(wr, CalendarApp, a.skipOps(wr.Query.Actor, e, emails), nil)
			}),
			"message": action(r, hosts, func(wr api.Write[*Model], e *Event, body messageBody) error {
				op, targets, err := r.app.at(wr.S).messageWith(wr.Query.Actor, e, body)
				if err != nil {
					return err
				}
				logAfter(wr, "calendar: message sent", "event", e.ID, "to", targets)
				return stage(wr, CalendarApp, []store.Op{op}, nil)
			}),
			"change-email": action(r, hosts, func(wr api.Write[*Model], e *Event, body addressBody) error {
				ops, c, err := r.app.at(wr.S).changeAddressOps(wr.Query.Actor, e.ID, body.Email, body.To, body.Everywhere)
				if err != nil {
					return err
				}
				if c.everywhere {
					actor, ctx, move := wr.Query.Actor, wr.Request.Context(), r.app.moveEverywhere
					wr.Tx.After(func() {
						if err := move(ctx, actor, c.from, c.to, c.name); err != nil {
							slog.ErrorContext(ctx, "calendar: move address everywhere", "from", c.from, "to", c.to, "error", err)
						}
					})
					return nil
				}
				logAfter(wr, "calendar: invite address changed", "event", e.ID, "from", c.from, "to", c.to)
				return stage(wr, CalendarApp, ops, nil)
			}),
			"delete-invitation": action(r, func(a calendarApp, q api.Query, e *Event) bool {
				inv := a.model().Invitations[e.ID]
				return hosts(a, q, e) && !(posted(e) && inv != nil && inv.Sent != "")
			}, func(wr api.Write[*Model], e *Event, _ serve.None) error {
				ops, _, own, err := r.app.at(wr.S).deleteInvitationOps(wr.Query.Actor, e.ID)
				logAfter(wr, "calendar: invitation deleted", "event", e.ID, "title", e.Title, "event too", own)
				return stage(wr, CalendarApp, ops, err)
			}),
			"start": action(r, func(a calendarApp, q api.Query, e *Event) bool { return hosts(a, q, e) && e.linked() }, func(wr api.Write[*Model], e *Event, _ serve.None) error {
				a := r.app.at(wr.S)
				ops, _, g, err := a.startPartyOps(wr.Query.Actor, e.ID)
				if err != nil || len(ops) == 0 {
					return err
				}
				filled, emails := a.fillOps(wr.Query.Actor, e, g)
				logAfter(wr, "calendar: party list started", "event", e.ID, "group", g.ID, "added", len(emails))
				return stage(wr, CalendarApp, append(ops, filled...), nil)
			}),
		},
	}
}

func (r calendarResources) guestLists() api.Type[*Model] {
	find := func(a calendarApp, q api.Query, key string) *Event {
		a.index()
		eventID, ok := a.pinned.scope.guestLists[key]
		if !ok {
			return nil
		}
		return a.viewerEvent(q, eventID)
	}
	return api.Type[*Model]{
		Name:  "guest-lists",
		Shape: guestListResource{},
		Has: func(m *Model, key string) bool {
			a := r.app.at(m)
			a.index()
			_, ok := a.pinned.scope.guestLists[key]
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			e := find(a, q, key)
			if e == nil {
				return nil, false
			}
			out := guestListResource{InviteView: a.inviteView(q.Actor, e), OnList: []string{}}
			if a.inviter(q, e) {
				if p := a.party(e); p != nil {
					out.Attendees = p.Attendees
				}
				for _, inv := range a.model().Invites[e.ID] {
					out.OnList = append(out.OnList, inv.Email)
				}
			}
			return out, true
		},
		List: func(m *Model, q api.Query) []string {
			a := r.app.at(m)
			events, _ := a.viewerEvents(q)
			out := []string{}
			for _, e := range events {
				if e.keepsGuestList() {
					out = append(out, a.guestListID(e.ID))
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"event": {Type: "events", List: func(m *Model, q api.Query, key string) []string {
				if e := find(r.app.at(m), q, key); e != nil {
					return []string{e.ID}
				}
				return nil
			}},
			"invite-groups": {Type: "invite-groups", Many: true, List: func(m *Model, q api.Query, key string) []string {
				a := r.app.at(m)
				e := find(a, q, key)
				if e == nil || !hosts(a, q, e) {
					return nil
				}
				out := []string{}
				for _, g := range a.model().Groups[e.ID] {
					out = append(out, g.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"opened": api.Do(func(m *Model, q api.Query, key string) bool {
				a := r.app.at(m)
				e := find(a, q, key)
				return e != nil && len(a.openedOps(q.Actor, e)) > 0
			}, func(wr api.Write[*Model], _ serve.None) error {
				a := r.app.at(wr.S)
				e := find(a, wr.Query, wr.ID)
				if e == nil {
					return access.Missing("no such guest list")
				}
				return r.app.store.stage(wr, CalendarApp, a.openedOps(wr.Query.Actor, e), nil)
			}),
		},
	}
}

type groupResource struct {
	InviteGroup
}

func (r calendarResources) inviteGroups() api.Type[*Model] {
	find := func(a calendarApp, q api.Query, key string) (*Event, *InviteGroup) {
		for eventID, groups := range a.model().Groups {
			for i := range groups {
				if groups[i].ID != key {
					continue
				}
				e := a.viewerEvent(q, eventID)
				if e == nil || !hosts(a, q, e) {
					return nil, nil
				}
				return e, &groups[i]
			}
		}
		return nil, nil
	}
	owns := func(m *Model, q api.Query, key string) bool { _, g := find(r.app.at(m), q, key); return g != nil }
	return api.Type[*Model]{
		Name:  "invite-groups",
		Shape: groupResource{},
		Has: func(m *Model, key string) bool {
			for _, groups := range r.app.at(m).model().Groups {
				if slices.ContainsFunc(groups, func(g InviteGroup) bool { return g.ID == key }) {
					return true
				}
			}
			return false
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			e, g := find(a, q, key)
			if g == nil {
				return nil, false
			}
			out := *g
			for _, inv := range a.model().Invites[e.ID] {
				if inv.Via == ViaGroup+g.ID {
					out.Count++
				}
			}
			return groupResource{out}, true
		},
		List: func(m *Model, q api.Query) []string {
			a := r.app.at(m)
			out := []string{}
			for eventID, groups := range a.model().Groups {
				e := a.viewerEvent(q, eventID)
				if e == nil || !hosts(a, q, e) {
					continue
				}
				for _, g := range groups {
					out = append(out, g.ID)
				}
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body addGroupBody) (string, error) {
			a := r.app.at(wr.S)
			ops, e, g, err := a.addGroupOps(wr.Query.Actor, body.ID, body.Rule, body.Auto)
			if err != nil {
				return "", err
			}
			filled, emails := a.fillOps(wr.Query.Actor, e, g)
			logAfter(wr, "calendar: group added", "event", e.ID, "group", g.ID, "added", len(emails))
			return g.ID, r.app.store.stage(wr, CalendarApp, append(ops, filled...), nil)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(owns, func(wr api.Write[*Model]) setGroupBody {
				_, g := find(r.app.at(wr.S), wr.Query, wr.ID)
				return setGroupBody{Auto: g.Auto}
			}, func(wr api.Write[*Model], body setGroupBody) error {
				a := r.app.at(wr.S)
				e, g := find(a, wr.Query, wr.ID)
				ops, _, _, err := a.setGroupOps(wr.Query.Actor, e.ID, g.ID, body.Auto)
				logAfter(wr, "calendar: group changed", "event", e.ID, "group", g.ID, "auto", body.Auto)
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
			"delete": api.Do(owns, func(wr api.Write[*Model], _ serve.None) error {
				a := r.app.at(wr.S)
				e, g := find(a, wr.Query, wr.ID)
				ops, _, _, err := a.removeGroupOps(wr.Query.Actor, e.ID, g.ID)
				logAfter(wr, "calendar: group removed", "event", e.ID, "group", g.ID, "dropped", len(ops)-1)
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
		},
	}
}

func (a calendarApp) feedOf(q api.Query, key string) (*feedResource, bool) {
	a.index()
	token, ok := a.pinned.scope.feeds[key]
	if !ok {
		return nil, false
	}
	model := a.model()
	if home, email, mine := strings.Cut(token, "\x00"); mine && home == MyHeliosianToken {
		if email != q.Actor.Email {
			return nil, false
		}
		f := model.MyHeliosian(email)
		out := &feedResource{Feed: f}
		if t := model.Settings[email].FeedToken; t != "" {
			out.URL = "/open/feed/" + t + ".ics"
		}
		return out, true
	}
	f := model.Feed(token)
	if f == nil || (f.Email != q.Actor.Email && !q.Actor.May(FeedsForAnyone)) {
		return nil, false
	}
	return &feedResource{Feed: *f, URL: "/open/feed/" + f.Token + ".ics"}, true
}

func (r calendarResources) feeds() api.Type[*Model] {
	owned := func(m *Model, q api.Query, key string) bool {
		f, ok := r.app.at(m).feedOf(q, key)
		return ok && !f.Locked
	}
	return api.Type[*Model]{
		Name:  "calendar-feeds",
		Shape: feedResource{},
		Has: func(m *Model, key string) bool {
			a := r.app.at(m)
			a.index()
			_, ok := a.pinned.scope.feeds[key]
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			f, ok := a.feedOf(q, key)
			if !ok {
				return nil, false
			}
			classrooms, _ := a.model().calendarView(a.directory(), f.Feed)
			f.Days = a.model().dayKinds(classrooms)
			return *f, true
		},
		List: func(m *Model, q api.Query) []string {
			a := r.app.at(m)
			out := []string{}
			for _, f := range a.model().MyCalendars(q.Actor.Email) {
				out = append(out, a.feedID(q.Actor.Email, f.Token))
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body feedBody) (string, error) {
			a := r.app.at(wr.S)
			ops, cells := a.newFeed(wr.Query.Actor, body)
			logAfter(wr, "calendar: feed added", "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
			return a.feedID(wr.Query.Actor.Email, cells["Token"]), r.app.store.stage(wr, CalendarApp, ops, nil)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(func(m *Model, q api.Query, key string) bool { _, ok := r.app.at(m).feedOf(q, key); return ok }, func(wr api.Write[*Model]) feedBody {
				f, _ := r.app.at(wr.S).feedOf(wr.Query, wr.ID)
				return feedBody{Name: f.Name, Emoji: f.Emoji, Classrooms: f.Classrooms, Tags: f.Tags}
			}, func(wr api.Write[*Model], body feedBody) error {
				a := r.app.at(wr.S)
				f, _ := a.feedOf(wr.Query, wr.ID)
				body.Token = f.Token
				ops, _, err := a.changeFeed(wr.Query.Actor, body)
				logAfter(wr, "calendar: feed changed", "name", body.Name)
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
			"delete": api.Do(owned, func(wr api.Write[*Model], _ serve.None) error {
				a := r.app.at(wr.S)
				f, _ := a.feedOf(wr.Query, wr.ID)
				ops, name, err := a.dropFeed(wr.Query.Actor, f.Token)
				logAfter(wr, "calendar: feed removed", "name", name)
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
		},
	}
}

func (r calendarResources) settings() api.Type[*Model] {
	always := func(*Model, api.Query, string) bool { return true }
	return api.Type[*Model]{
		Name:  "when-settings",
		Shape: settingsResource{},
		Has:   func(m *Model, key string) bool { return key == r.app.at(m).settingsID() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			a := r.app.at(m)
			if key != a.settingsID() {
				return nil, false
			}
			v := RenderCalendar(a.model(), a.directory(), a.settings(), q.Actor, a.clock(), nil)
			out := settingsResource{
				User: v.User, ImageSearch: a.search.On(), Today: v.Today, Classrooms: v.Classrooms, Colors: v.Colors, Tags: v.Tags, DayTypes: v.DayTypes,
				Years: v.Years, Days: v.Days, GradeColors: v.GradeColors, Names: v.Names,
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{r.app.at(m).settingsID()} },
		Relations: map[string]api.Relation[*Model]{
			"feeds": {Type: "calendar-feeds", Many: true, List: func(m *Model, q api.Query, _ string) []string {
				a := r.app.at(m)
				out := []string{}
				for _, f := range a.model().MyCalendars(q.Actor.Email) {
					out = append(out, a.feedID(q.Actor.Email, f.Token))
				}
				return out
			}},
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string {
				d := r.app.at(m).directory()
				if p := d.Person(d.Resolve(q.Actor.Email)); p != nil && p.ID != "" {
					return []string{p.ID}
				}
				return nil
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"save-view": api.Do(always, func(wr api.Write[*Model], body viewBody) error {
				ops, cells := saveViewOps(wr.Query.Actor, body.Classrooms, body.Tags)
				logAfter(wr, "calendar: view saved", "classrooms", cells["Classrooms"], "tags", cells["Categories"])
				return r.app.store.stage(wr, CalendarApp, ops, nil)
			}),
			"forget-view": api.Do(func(m *Model, q api.Query, _ string) bool { return len(r.app.at(m).forgetViewOps(q.Actor)) > 0 }, func(wr api.Write[*Model], _ serve.None) error {
				logAfter(wr, "calendar: view forgotten")
				return r.app.store.stage(wr, CalendarApp, r.app.at(wr.S).forgetViewOps(wr.Query.Actor), nil)
			}),
			"order-feeds": api.Do(always, func(wr api.Write[*Model], body tokensBody) error {
				ops, err := r.app.at(wr.S).orderOps(wr.Query.Actor, body.Tokens)
				logAfter(wr, "calendar: feeds ordered", "order", strings.Join(body.Tokens, ","))
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
			"default": api.Do(always, func(wr api.Write[*Model], body tokenBody) error {
				ops, tokens, err := r.app.at(wr.S).defaultOps(wr.Query.Actor, strings.TrimSpace(body.Token))
				logAfter(wr, "calendar: default calendar set", "order", strings.Join(tokens, ","))
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
			"feed-token": api.Do(always, func(wr api.Write[*Model], _ serve.None) error {
				ops, _ := r.app.at(wr.S).feedTokenOps(wr.Query.Actor)
				logAfter(wr, "calendar: my heliosian feed made")
				return r.app.store.stage(wr, CalendarApp, ops, nil)
			}),
			"tags": api.Do(func(_ *Model, q api.Query, _ string) bool { return q.Actor.May(CurateCalendar) }, func(wr api.Write[*Model], body tagsBody) error {
				ops, added, err := r.app.at(wr.S).tagOps(wr.Query.Actor, body.Tags)
				logAfter(wr, "calendar: categories saved", "added", added)
				return r.app.store.stage(wr, CalendarApp, ops, err)
			}),
		},
	}
}
