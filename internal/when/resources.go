package when

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/config"
	"heliosian/internal/id"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const (
	appHost          = "when"
	kindGuestList    = "guest-list"
	kindCalendarFeed = "calendar-feed"
	kindSettings     = "when-settings"
)

type World struct {
	Model     *Model
	Directory *who.Model
	app       app
	idKey     []byte
	sources   func(email string, now time.Time) []Linked
	now       time.Time
	request   *request
}

type request struct {
	eventsOnce sync.Once
	events     []*Event
	byID       map[string]*Event
	indexOnce  sync.Once
	guestLists map[string]string
	feeds      map[string]string
	answerOnce sync.Once
	responses  map[string]*Responses
}

func (w World) responses(q api.Query) map[string]*Responses {
	w.request.answerOnce.Do(func() {
		w.request.responses = w.Model.ResponsesFor(q.Actor, w.Directory)
	})
	return w.request.responses
}

func (a app) world(m *Model, d *who.Model, settings *config.Settings, idKey []byte, sources func(email string, now time.Time) []Linked) World {
	a.pinned = m
	a.directory = func() *who.Model { return d }
	a.settings = func() *config.Settings { return settings }
	return World{Model: m, Directory: d, app: a, idKey: idKey, sources: sources}
}

func (w World) At(now time.Time) World {
	w.now, w.request = now, &request{}
	w.app.linked = func(email string) []Linked { return w.sources(email, now) }
	return w
}

func (w World) guestListID(eventID string) string {
	return id.Of(w.idKey, kindGuestList, eventID)
}

func (w World) feedID(email, token string) string {
	return id.Of(w.idKey, kindCalendarFeed, email+"\x00"+token)
}

func (w World) settingsID() string {
	return id.Of(w.idKey, kindSettings, "")
}

func (w World) viewerEvents(q api.Query) ([]*Event, map[string]*Event) {
	w.request.eventsOnce.Do(func() {
		model, directory := w.Model, w.Directory
		events := model.EventsFor(q.Actor, directory, w.app.linked(q.Actor.Email))
		out := []*Event{}
		byID := map[string]*Event{}
		for _, e := range events {
			c := w.decorated(q, e)
			out = append(out, c)
			byID[c.ID] = c
		}
		w.request.events, w.request.byID = out, byID
	})
	return w.request.events, w.request.byID
}

func (w World) decorated(q api.Query, e *Event) *Event {
	c := *e
	c.Hosted = w.Model.hostedBy(w.Directory, q.Actor.Email, e)
	c.HostNames = nil
	for _, h := range e.Hosts {
		if p := w.Directory.Person(w.Directory.Resolve(config.NormalizeEmail(h))); p != nil && p.FullName != "" {
			c.HostNames = append(c.HostNames, p.FullName)
		}
	}
	return &c
}

func (w World) event(q api.Query, key string) *Event {
	if _, byID := w.viewerEvents(q); byID[key] != nil {
		return byID[key]
	}
	if e := w.app.eventFor(q.Actor, key); e != nil {
		return w.decorated(q, e)
	}
	return nil
}

func (w World) seesResponses(q api.Query, e *Event) bool {
	return q.Actor.May(SeeAll) || (e.AddedBy != "" && !e.PosterLeft && config.NormalizeEmail(e.AddedBy) == config.NormalizeEmail(q.Actor.Email))
}

func (w World) index() {
	w.request.indexOnce.Do(func() {
		lists, feeds := map[string]string{}, map[string]string{}
		all := slices.Concat(w.Model.Events, w.Model.Pending)
		for _, e := range withLinked(nil, w.app.linked("")) {
			all = append(all, e)
		}
		for _, e := range all {
			if e.keepsGuestList() {
				lists[w.guestListID(e.ID)] = e.ID
			}
		}
		for _, f := range w.Model.Feeds {
			feeds[w.feedID(f.Email, f.Token)] = f.Token
		}
		for _, p := range w.Directory.People {
			if p.Email != "" {
				feeds[w.feedID(p.Email, MyHeliosianToken)] = MyHeliosianToken + "\x00" + p.Email
			}
		}
		w.request.guestLists, w.request.feeds = lists, feeds
	})
}

func (w World) has(key string) bool {
	return w.Model.Event(key) != nil || slices.ContainsFunc(withLinked(nil, w.app.linked("")), func(e *Event) bool { return e.ID == key })
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
	User        User                         `json:"user"`
	ImageSearch bool                         `json:"imageSearch"`
	Today       string                       `json:"today"`
	Classrooms  []Classroom                  `json:"classrooms"`
	Colors      map[string]string            `json:"colors"`
	Tags        []Tag                        `json:"tags"`
	DayTypes    []DayType                    `json:"dayTypes"`
	Years       []Year                       `json:"years"`
	Days        map[string]map[string]string `json:"days"`
	GradeColors map[string]string            `json:"gradeColors,omitempty"`
	Names       map[string]string            `json:"names,omitempty"`
	Lists       []List                       `json:"lists"`
}

type guestListResource struct {
	InviteView
	Attendees []Attendee `json:"attendees,omitempty"`
	OnList    []string   `json:"onList"`
}

type feedResource struct {
	Feed
	URL string `json:"url,omitempty"`
}

type resources struct {
	cache *Cache
}

func (h Hooks) Resources() []api.Type[World] {
	r := resources{cache: h.cache}
	return []api.Type[World]{r.events(), r.guestLists(), r.inviteGroups(), r.feeds(), r.settings()}
}

func (h Hooks) World(m *Model, d *who.Model, settings *config.Settings, idKey []byte, sources func(email string, now time.Time) []Linked) World {
	return h.app.world(m, d, settings, idKey, sources)
}

func (r resources) stage(wr api.Write[World], ops []store.Op, err error) error {
	if err != nil {
		return err
	}
	return r.cache.Stage(wr.Tx, ops...)
}

func (r resources) staged(wr api.Write[World]) World {
	w := wr.S
	w.Model = r.cache.In(wr.Tx)
	w.app.pinned = w.Model
	w.request = &request{}
	return w
}

func logAfter(wr api.Write[World], msg string, args ...any) {
	actor := wr.Query.Actor.Email
	ctx := wr.Request.Context()
	wr.Tx.After(func() { slog.InfoContext(ctx, msg, append([]any{"actor", actor}, args...)...) })
}

func decode[T any](wr api.Write[World], into T) (T, error) {
	err := wr.Decode(&into)
	return into, err
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

type check func(w World, q api.Query, e *Event) bool

func (r resources) action(can check, do func(wr api.Write[World], e *Event) error) api.Action[World] {
	return api.Action[World]{
		Can: func(w World, q api.Query, key string) bool {
			e := w.event(q, key)
			return e != nil && can(w, q, e)
		},
		Do: func(wr api.Write[World]) error {
			e := wr.S.event(wr.Query, wr.ID)
			if e == nil || !can(wr.S, wr.Query, e) {
				return access.Forbidden("that is not yours to do on this event")
			}
			return do(wr, e)
		},
	}
}

func hosts(w World, q api.Query, e *Event) bool {
	return e.keepsGuestList() && w.app.isHost(q.Actor, e)
}

func curates(_ World, q api.Query, _ *Event) bool {
	return q.Actor.May(Curate)
}

func posted(e *Event) bool {
	return e.Source == SourceSheet
}

func (w World) inviter(q api.Query, e *Event) bool {
	if !e.keepsGuestList() {
		return false
	}
	if w.app.isHost(q.Actor, e) {
		return true
	}
	return w.Model.othersInvite(e.ID) && (e.Sharing == SharingPublic || w.Model.Invited(w.Directory, q.Actor.Email, e.ID))
}

func (w World) bringer(q api.Query, e *Event) bool {
	if !e.keepsGuestList() {
		return false
	}
	if w.app.isHost(q.Actor, e) {
		return true
	}
	return w.Model.othersInvite(e.ID) && (e.guestsWithoutInvite() || w.Model.Listed(w.Directory, q.Actor.Email, e.ID))
}

func (r resources) events() api.Type[World] {
	a := func(w World) app { return w.app }
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
	return api.Type[World]{
		Name:  "events",
		Shape: eventResource{},
		Has:   func(w World, key string) bool { return w.has(key) },
		Get: func(w World, q api.Query, key string) (any, bool) {
			e := w.event(q, key)
			if e == nil {
				return nil, false
			}
			app, path := Page(e)
			out := eventResource{Event: e, Path: path, App: app, Slug: e.Address, Me: eventMe{Answer: w.Model.AnswerOf(q.Actor.Email, e.ID)}}
			if q.Actor.May(SeeAll) {
				out.Provenance = w.Model.Provenance[e.ID]
				out.AdminOnly = !w.app.sees(access.Actor{Email: q.Actor.Email, Household: q.Actor.Household}, e)
			}
			if w.seesResponses(q, e) {
				out.Responses = &Responses{}
				if r := w.responses(q)[e.ID]; r != nil {
					out.Responses = r
				}
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			events, _ := w.viewerEvents(q)
			out := []string{}
			for _, e := range events {
				out = append(out, e.ID)
			}
			return out
		},
		Aliases: func(w World) map[string]string {
			out := map[string]string{}
			for alias, target := range w.Model.aliases {
				out[alias] = target
			}
			for address, e := range w.Model.byAddress {
				out[address] = e.ID
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"hosts": {Type: "people", Many: true, List: func(w World, q api.Query, key string) []string {
				e := w.event(q, key)
				if e == nil {
					return nil
				}
				out := []string{}
				for _, h := range w.app.hostsOf(e) {
					if p := w.Directory.Person(h); p != nil && p.ID != "" {
						out = append(out, p.ID)
					}
				}
				return out
			}},
			"guest-list": {Type: "guest-lists", List: func(w World, q api.Query, key string) []string {
				e := w.event(q, key)
				if e == nil || !e.keepsGuestList() {
					return nil
				}
				return []string{w.guestListID(e.ID)}
			}},
		},
		Create: func(wr api.Write[World]) (string, error) {
			body, err := decode(wr, eventBody{})
			if err != nil {
				return "", err
			}
			ops, ids, pending, err := a(wr.S).newEvents(wr.Query.Actor, body)
			if err := r.stage(wr, ops, err); err != nil {
				return "", err
			}
			after := r.staged(wr)
			for _, key := range ids {
				yes, _, err := after.app.answerOps(wr.Query.Actor, wr.Query.Actor.Email, key, AnswerYes, ViaPage, false)
				if err := r.stage(wr, yes, err); err != nil {
					return "", err
				}
			}
			logAfter(wr, "calendar: events added", "title", strings.TrimSpace(body.Title), "count", len(ids), "pending", pending)
			return ids[0], nil
		},
		Actions: map[string]api.Action[World]{
			"edit": r.action(func(w World, q api.Query, e *Event) bool { return posted(e) && w.app.isHost(q.Actor, e) }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, editable(e))
				if err != nil {
					return err
				}
				_, _, row, err := a(wr.S).changeEvent(wr.Query.Actor, body)
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
				return r.stage(wr, []store.Op{eventCellsOp(e.ID, cells)}, nil)
			}),
			"correct": r.action(func(w World, q api.Query, e *Event) bool {
				return e.imported() && (q.Actor.May(Curate) || w.app.isHost(q.Actor, e))
			}, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, correctable(e))
				if err != nil {
					return err
				}
				ops, key, empty, err := a(wr.S).overrideOps(wr.Query.Actor, body)
				logAfter(wr, "calendar: override set", "event", key, "cleared", empty)
				return r.stage(wr, ops, err)
			}),
			"image": r.action(func(w World, q api.Query, e *Event) bool {
				return e.imported() && (q.Actor.May(Curate) || w.app.isHost(q.Actor, e))
			}, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, overrideImageBody{})
				if err != nil {
					return err
				}
				ops, _, image, err := a(wr.S).overrideImageOps(wr.Query.Actor, e.ID, body.Image)
				logAfter(wr, "calendar: override image", "event", e.ID, "image", image)
				return r.stage(wr, ops, err)
			}),
			"keywords": r.action(curates, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, keywordsBody{})
				if err != nil {
					return err
				}
				ops, _, cell, err := a(wr.S).keywordOps(wr.Query.Actor, e.ID, body.Keywords)
				logAfter(wr, "calendar: keywords set", "event", e.ID, "keywords", cell)
				return r.stage(wr, ops, err)
			}),
			"approve": r.action(func(w World, q api.Query, e *Event) bool { return q.Actor.May(Curate) && posted(e) }, func(wr api.Write[World], e *Event) error {
				ops, _, err := a(wr.S).statusOps(wr.Query.Actor, e.ID, StatusApproved)
				logAfter(wr, "calendar: event approved", "event", e.ID, "title", e.Title, "by", e.AddedBy)
				return r.stage(wr, ops, err)
			}),
			"decline": r.action(func(w World, q api.Query, e *Event) bool {
				return q.Actor.May(Curate) && posted(e) && e.Sharing == SharingPublic
			}, func(wr api.Write[World], e *Event) error {
				ops, _, err := a(wr.S).statusOps(wr.Query.Actor, e.ID, StatusDeclined)
				logAfter(wr, "calendar: event declined", "event", e.ID, "title", e.Title, "by", e.AddedBy)
				return r.stage(wr, ops, err)
			}),
			"move": r.action(func(w World, q api.Query, e *Event) bool { return q.Actor.May(Curate) && posted(e) }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, moveBody{})
				if err != nil {
					return err
				}
				start, end := strings.TrimSpace(body.Start), strings.TrimSpace(body.End)
				if end == "" {
					end = start
				}
				ops, _, err := a(wr.S).moveOps(wr.Query.Actor, e.ID, start, end)
				logAfter(wr, "calendar: event moved", "event", e.ID, "start", start, "end", end)
				return r.stage(wr, ops, err)
			}),
			"cancel": r.action(func(w World, q api.Query, e *Event) bool { return posted(e) && hosts(w, q, e) && !e.Cancelled }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, cancelBody{})
				if err != nil {
					return err
				}
				ops, err := a(wr.S).cancelWith(wr.Query.Actor, e, body)
				logAfter(wr, "calendar: event cancelled", "event", e.ID, "title", e.Title)
				return r.stage(wr, ops, err)
			}),
			"answer": r.action(func(World, api.Query, *Event) bool { return true }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, rsvpBody{})
				if err != nil {
					return err
				}
				ops, _, err := a(wr.S).answerOps(wr.Query.Actor, wr.Query.Actor.Email, e.ID, body.Answer, ViaPage, true)
				logAfter(wr, "calendar: answered", "event", e.ID, "answer", body.Answer)
				return r.stage(wr, ops, err)
			}),
			"answer-for": r.action(func(World, api.Query, *Event) bool { return true }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, answerForBody{})
				if err != nil {
					return err
				}
				_, subject, err := a(wr.S).answerSubject(wr.Query.Actor, e.ID, body.Email, body.Answer)
				if err != nil {
					return err
				}
				ops, _, err := a(wr.S).answerOps(wr.Query.Actor, subject, e.ID, body.Answer, ViaPage, subject == wr.Query.Actor.Email)
				logAfter(wr, "calendar: answered for", "subject", subject, "event", e.ID, "answer", body.Answer)
				return r.stage(wr, ops, err)
			}),
			"invite": r.action(func(w World, q api.Query, e *Event) bool { return w.inviter(q, e) }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, inviteesBody{})
				if err != nil {
					return err
				}
				ops, _, emails, host, err := a(wr.S).inviteOps(wr.Query.Actor, e.ID, body.People)
				if err := r.stage(wr, ops, err); err != nil {
					return err
				}
				logAfter(wr, "calendar: guests added", "event", e.ID, "count", len(emails), "host", host)
				if host {
					return nil
				}
				return r.stage(wr, r.staged(wr).app.requestOps(wr.Query.Actor, e, emails, wr.Query.Actor.Email, ""), nil)
			}),
			"uninvite": r.action(func(w World, q api.Query, e *Event) bool {
				return e.keepsGuestList() && (w.app.isHost(q.Actor, e) || slices.ContainsFunc(w.Model.Invites[e.ID], func(inv Invite) bool { return inv.GuestOf != "" && w.app.mayAnswerFor(q.Actor, inv.GuestOf, e) }))
			}, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, personBody{})
				if err != nil {
					return err
				}
				ops, _, email, fromGroup, err := a(wr.S).uninviteOps(wr.Query.Actor, e.ID, body.Email)
				logAfter(wr, "calendar: guest removed", "event", e.ID, "email", email, "from group", fromGroup)
				return r.stage(wr, ops, err)
			}),
			"bring-guest": r.action(func(w World, q api.Query, e *Event) bool { return w.bringer(q, e) }, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, guestBody{})
				if err != nil {
					return err
				}
				body.ID = e.ID
				ops, g, err := a(wr.S).bringGuestOps(wr.Query.Actor, body)
				if err := r.stage(wr, ops, err); err != nil {
					return err
				}
				logAfter(wr, "calendar: guest brought", "event", e.ID, "of", g.of, "guest", g.key, "answer", g.answer, "invite", g.invite)
				if !g.invite || isGuestKey(g.key) {
					return nil
				}
				return r.stage(wr, r.staged(wr).app.requestOps(wr.Query.Actor, e, []string{g.key}, wr.Query.Actor.Email, ""), nil)
			}),
			"settings": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, settingsBody{})
				if err != nil {
					return err
				}
				body.ID = e.ID
				ops, _, _, err := a(wr.S).settingsOps(wr.Query.Actor, body)
				logAfter(wr, "calendar: guest list settings", "event", e.ID)
				return r.stage(wr, ops, err)
			}),
			"step-down": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, personBody{})
				if err != nil {
					return err
				}
				ops, _, who, poster, err := a(wr.S).stepDownOps(wr.Query.Actor, e.ID, body.Email)
				logAfter(wr, "calendar: host stepped down", "who", who, "event", e.ID, "poster", poster)
				return r.stage(wr, ops, err)
			}),
			"send": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, sendBody{})
				if err != nil {
					return err
				}
				emails, kind, err := a(wr.S).sendChoice(e, body)
				if err != nil {
					return err
				}
				logAfter(wr, "calendar: invites sent", "event", e.ID, "invites", len(emails), "kind", kind)
				return r.stage(wr, a(wr.S).requestOps(wr.Query.Actor, e, emails, wr.Query.Actor.Email, kind), nil)
			}),
			"skip": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, skipBody{})
				if err != nil {
					return err
				}
				emails := a(wr.S).skippable(e, body.Emails)
				if len(emails) == 0 {
					return access.Invalid("nobody pending to skip")
				}
				logAfter(wr, "calendar: invites skipped", "event", e.ID, "skipped", len(emails))
				return r.stage(wr, a(wr.S).skipOps(wr.Query.Actor, e, emails), nil)
			}),
			"message": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, messageBody{})
				if err != nil {
					return err
				}
				op, targets, err := a(wr.S).messageWith(wr.Query.Actor, e, body)
				if err != nil {
					return err
				}
				logAfter(wr, "calendar: message sent", "event", e.ID, "to", targets)
				return r.stage(wr, []store.Op{op}, nil)
			}),
			"change-email": r.action(hosts, func(wr api.Write[World], e *Event) error {
				body, err := decode(wr, addressBody{})
				if err != nil {
					return err
				}
				ops, c, err := a(wr.S).changeAddressOps(wr.Query.Actor, e.ID, body.Email, body.To, body.Everywhere)
				if err != nil {
					return err
				}
				if c.everywhere {
					actor, ctx, move := wr.Query.Actor, wr.Request.Context(), a(wr.S).celebrate.MoveAddress
					wr.Tx.After(func() {
						if err := move(ctx, actor, c.from, c.to, c.name); err != nil {
							slog.ErrorContext(ctx, "calendar: move address everywhere", "from", c.from, "to", c.to, "error", err)
						}
					})
					return nil
				}
				logAfter(wr, "calendar: invite address changed", "event", e.ID, "from", c.from, "to", c.to)
				return r.stage(wr, ops, nil)
			}),
			"delete-invitation": r.action(func(w World, q api.Query, e *Event) bool {
				inv := w.Model.Invitations[e.ID]
				return hosts(w, q, e) && !(posted(e) && inv != nil && inv.Sent != "")
			}, func(wr api.Write[World], e *Event) error {
				ops, _, own, err := a(wr.S).deleteInvitationOps(wr.Query.Actor, e.ID)
				logAfter(wr, "calendar: invitation deleted", "event", e.ID, "title", e.Title, "event too", own)
				return r.stage(wr, ops, err)
			}),
			"start": r.action(func(w World, q api.Query, e *Event) bool { return hosts(w, q, e) && e.linked() }, func(wr api.Write[World], e *Event) error {
				ops, _, g, err := a(wr.S).startPartyOps(wr.Query.Actor, e.ID)
				if err != nil || len(ops) == 0 {
					return err
				}
				filled, emails := a(wr.S).fillOps(wr.Query.Actor, e, g)
				logAfter(wr, "calendar: party list started", "event", e.ID, "group", g.ID, "added", len(emails))
				return r.stage(wr, append(ops, filled...), nil)
			}),
		},
	}
}

func (r resources) guestLists() api.Type[World] {
	find := func(w World, q api.Query, key string) *Event {
		w.index()
		eventID, ok := w.request.guestLists[key]
		if !ok {
			return nil
		}
		return w.event(q, eventID)
	}
	return api.Type[World]{
		Name:  "guest-lists",
		Shape: guestListResource{},
		Has: func(w World, key string) bool {
			w.index()
			_, ok := w.request.guestLists[key]
			return ok
		},
		Get: func(w World, q api.Query, key string) (any, bool) {
			e := find(w, q, key)
			if e == nil {
				return nil, false
			}
			out := guestListResource{InviteView: w.app.inviteView(q.Actor, e), OnList: []string{}}
			if w.inviter(q, e) {
				if p := w.app.party(e); p != nil {
					out.Attendees = p.Attendees
				}
				for _, inv := range w.Model.Invites[e.ID] {
					out.OnList = append(out.OnList, inv.Email)
				}
			}
			return out, true
		},
		List: func(w World, q api.Query) []string {
			events, _ := w.viewerEvents(q)
			out := []string{}
			for _, e := range events {
				if e.keepsGuestList() {
					out = append(out, w.guestListID(e.ID))
				}
			}
			return out
		},
		Relations: map[string]api.Relation[World]{
			"event": {Type: "events", List: func(w World, q api.Query, key string) []string {
				if e := find(w, q, key); e != nil {
					return []string{e.ID}
				}
				return nil
			}},
			"invite-groups": {Type: "invite-groups", Many: true, List: func(w World, q api.Query, key string) []string {
				e := find(w, q, key)
				if e == nil || !hosts(w, q, e) {
					return nil
				}
				out := []string{}
				for _, g := range w.Model.Groups[e.ID] {
					out = append(out, g.ID)
				}
				return out
			}},
		},
		Actions: map[string]api.Action[World]{
			"opened": {
				Can: func(w World, q api.Query, key string) bool {
					e := find(w, q, key)
					return e != nil && len(w.app.openedOps(q.Actor, e)) > 0
				},
				Do: func(wr api.Write[World]) error {
					e := find(wr.S, wr.Query, wr.ID)
					if e == nil {
						return access.Missing("no such guest list")
					}
					return r.stage(wr, wr.S.app.openedOps(wr.Query.Actor, e), nil)
				},
			},
		},
	}
}

type groupResource struct {
	InviteGroup
}

func (r resources) inviteGroups() api.Type[World] {
	find := func(w World, q api.Query, key string) (*Event, *InviteGroup) {
		for eventID, groups := range w.Model.Groups {
			for i := range groups {
				if groups[i].ID != key {
					continue
				}
				e := w.event(q, eventID)
				if e == nil || !hosts(w, q, e) {
					return nil, nil
				}
				return e, &groups[i]
			}
		}
		return nil, nil
	}
	return api.Type[World]{
		Name:  "invite-groups",
		Shape: groupResource{},
		Has: func(w World, key string) bool {
			for _, groups := range w.Model.Groups {
				if slices.ContainsFunc(groups, func(g InviteGroup) bool { return g.ID == key }) {
					return true
				}
			}
			return false
		},
		Get: func(w World, q api.Query, key string) (any, bool) {
			e, g := find(w, q, key)
			if g == nil {
				return nil, false
			}
			out := *g
			for _, inv := range w.Model.Invites[e.ID] {
				if inv.Via == ViaGroup+g.ID {
					out.Count++
				}
			}
			return groupResource{out}, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for eventID, groups := range w.Model.Groups {
				e := w.event(q, eventID)
				if e == nil || !hosts(w, q, e) {
					continue
				}
				for _, g := range groups {
					out = append(out, g.ID)
				}
			}
			return out
		},
		Create: func(wr api.Write[World]) (string, error) {
			body, err := decode(wr, addGroupBody{})
			if err != nil {
				return "", err
			}
			ops, e, g, err := wr.S.app.addGroupOps(wr.Query.Actor, body.ID, body.Rule, body.Auto)
			if err != nil {
				return "", err
			}
			filled, emails := wr.S.app.fillOps(wr.Query.Actor, e, g)
			logAfter(wr, "calendar: group added", "event", e.ID, "group", g.ID, "added", len(emails))
			return g.ID, r.stage(wr, append(ops, filled...), nil)
		},
		Actions: map[string]api.Action[World]{
			"edit": {
				Can: func(w World, q api.Query, key string) bool { _, g := find(w, q, key); return g != nil },
				Do: func(wr api.Write[World]) error {
					e, g := find(wr.S, wr.Query, wr.ID)
					body, err := decode(wr, setGroupBody{Auto: g.Auto})
					if err != nil {
						return err
					}
					ops, _, _, err := wr.S.app.setGroupOps(wr.Query.Actor, e.ID, g.ID, body.Auto)
					logAfter(wr, "calendar: group changed", "event", e.ID, "group", g.ID, "auto", body.Auto)
					return r.stage(wr, ops, err)
				},
			},
			"delete": {
				Can: func(w World, q api.Query, key string) bool { _, g := find(w, q, key); return g != nil },
				Do: func(wr api.Write[World]) error {
					e, g := find(wr.S, wr.Query, wr.ID)
					ops, _, _, err := wr.S.app.removeGroupOps(wr.Query.Actor, e.ID, g.ID)
					logAfter(wr, "calendar: group removed", "event", e.ID, "group", g.ID, "dropped", len(ops)-1)
					return r.stage(wr, ops, err)
				},
			},
		},
	}
}

func (w World) feedOf(q api.Query, key string) (*feedResource, bool) {
	w.index()
	token, ok := w.request.feeds[key]
	if !ok {
		return nil, false
	}
	if home, email, mine := strings.Cut(token, "\x00"); mine && home == MyHeliosianToken {
		if email != q.Actor.Email {
			return nil, false
		}
		f := w.Model.MyHeliosian(email)
		out := &feedResource{Feed: f}
		if t := w.Model.Settings[email].FeedToken; t != "" {
			out.URL = "/open/feed/" + t + ".ics"
		}
		return out, true
	}
	f := w.Model.Feed(token)
	if f == nil || (f.Email != q.Actor.Email && !q.Actor.May(FeedsForAnyone)) {
		return nil, false
	}
	return &feedResource{Feed: *f, URL: "/open/feed/" + f.Token + ".ics"}, true
}

func (r resources) feeds() api.Type[World] {
	owned := func(w World, q api.Query, key string) bool {
		f, ok := w.feedOf(q, key)
		return ok && !f.Locked
	}
	return api.Type[World]{
		Name:  "calendar-feeds",
		Shape: feedResource{},
		Has: func(w World, key string) bool {
			w.index()
			_, ok := w.request.feeds[key]
			return ok
		},
		Get: func(w World, q api.Query, key string) (any, bool) {
			f, ok := w.feedOf(q, key)
			if !ok {
				return nil, false
			}
			return *f, true
		},
		List: func(w World, q api.Query) []string {
			out := []string{}
			for _, f := range w.Model.MyCalendars(q.Actor.Email) {
				out = append(out, w.feedID(q.Actor.Email, f.Token))
			}
			return out
		},
		Create: func(wr api.Write[World]) (string, error) {
			body, err := decode(wr, feedBody{})
			if err != nil {
				return "", err
			}
			ops, cells := wr.S.app.newFeed(wr.Query.Actor, body)
			logAfter(wr, "calendar: feed added", "name", cells["Name"], "classrooms", cells["Classrooms"], "tags", cells["Tags"])
			return wr.S.feedID(wr.Query.Actor.Email, cells["Token"]), r.stage(wr, ops, nil)
		},
		Actions: map[string]api.Action[World]{
			"edit": {
				Can: func(w World, q api.Query, key string) bool { _, ok := w.feedOf(q, key); return ok },
				Do: func(wr api.Write[World]) error {
					f, _ := wr.S.feedOf(wr.Query, wr.ID)
					body, err := decode(wr, feedBody{Name: f.Name, Emoji: f.Emoji, Classrooms: f.Classrooms, Tags: f.Tags})
					if err != nil {
						return err
					}
					body.Token = f.Token
					ops, _, err := wr.S.app.changeFeed(wr.Query.Actor, body)
					logAfter(wr, "calendar: feed changed", "name", body.Name)
					return r.stage(wr, ops, err)
				},
			},
			"delete": {
				Can: owned,
				Do: func(wr api.Write[World]) error {
					f, _ := wr.S.feedOf(wr.Query, wr.ID)
					ops, name, err := wr.S.app.dropFeed(wr.Query.Actor, f.Token)
					logAfter(wr, "calendar: feed removed", "name", name)
					return r.stage(wr, ops, err)
				},
			},
		},
	}
}

func (r resources) settings() api.Type[World] {
	always := func(World, api.Query, string) bool { return true }
	settingsAction := func(can func(World, api.Query, string) bool, do func(wr api.Write[World]) error) api.Action[World] {
		return api.Action[World]{Can: can, Do: do}
	}
	return api.Type[World]{
		Name:  "when-settings",
		Shape: settingsResource{},
		Has:   func(w World, key string) bool { return key == w.settingsID() },
		Get: func(w World, q api.Query, key string) (any, bool) {
			if key != w.settingsID() {
				return nil, false
			}
			v := Render(w.Model, w.Directory, w.app.settings(), q.Actor, w.now, nil)
			out := settingsResource{
				User: v.User, ImageSearch: w.app.search.On(), Today: v.Today, Classrooms: v.Classrooms, Colors: v.Colors, Tags: v.Tags, DayTypes: v.DayTypes,
				Years: v.Years, Days: v.Days, GradeColors: v.GradeColors, Names: v.Names, Lists: []List{},
			}
			if lists := w.app.lists(q.Actor.Email); lists != nil {
				out.Lists = lists
			}
			return out, true
		},
		List: func(w World, _ api.Query) []string { return []string{w.settingsID()} },
		Relations: map[string]api.Relation[World]{
			"feeds": {Type: "calendar-feeds", Many: true, List: func(w World, q api.Query, _ string) []string {
				out := []string{}
				for _, f := range w.Model.MyCalendars(q.Actor.Email) {
					out = append(out, w.feedID(q.Actor.Email, f.Token))
				}
				return out
			}},
			"viewer": {Type: "people", List: func(w World, q api.Query, _ string) []string {
				if p := w.Directory.Person(w.Directory.Resolve(q.Actor.Email)); p != nil && p.ID != "" {
					return []string{p.ID}
				}
				return nil
			}},
		},
		Actions: map[string]api.Action[World]{
			"save-view": settingsAction(always, func(wr api.Write[World]) error {
				body, err := decode(wr, viewBody{})
				if err != nil {
					return err
				}
				ops, cells := saveViewOps(wr.Query.Actor, body.Classrooms, body.Tags)
				logAfter(wr, "calendar: view saved", "classrooms", cells["Classrooms"], "tags", cells["Categories"])
				return r.stage(wr, ops, nil)
			}),
			"forget-view": settingsAction(func(w World, q api.Query, _ string) bool { return len(w.app.forgetViewOps(q.Actor)) > 0 }, func(wr api.Write[World]) error {
				logAfter(wr, "calendar: view forgotten")
				return r.stage(wr, wr.S.app.forgetViewOps(wr.Query.Actor), nil)
			}),
			"order-feeds": settingsAction(always, func(wr api.Write[World]) error {
				body, err := decode(wr, tokensBody{})
				if err != nil {
					return err
				}
				ops, err := wr.S.app.orderOps(wr.Query.Actor, body.Tokens)
				logAfter(wr, "calendar: feeds ordered", "order", strings.Join(body.Tokens, ","))
				return r.stage(wr, ops, err)
			}),
			"default": settingsAction(always, func(wr api.Write[World]) error {
				body, err := decode(wr, tokenBody{})
				if err != nil {
					return err
				}
				ops, tokens, err := wr.S.app.defaultOps(wr.Query.Actor, strings.TrimSpace(body.Token))
				logAfter(wr, "calendar: default calendar set", "order", strings.Join(tokens, ","))
				return r.stage(wr, ops, err)
			}),
			"feed-token": settingsAction(always, func(wr api.Write[World]) error {
				ops, _ := wr.S.app.feedTokenOps(wr.Query.Actor)
				logAfter(wr, "calendar: my heliosian feed made")
				return r.stage(wr, ops, nil)
			}),
			"tags": settingsAction(func(_ World, q api.Query, _ string) bool { return q.Actor.May(Curate) }, func(wr api.Write[World]) error {
				body, err := decode(wr, tagsBody{})
				if err != nil {
					return err
				}
				ops, added, err := wr.S.app.tagOps(wr.Query.Actor, body.Tags)
				logAfter(wr, "calendar: categories saved", "added", added)
				return r.stage(wr, ops, err)
			}),
		},
	}
}
