package model

import (
	"encoding/json"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/cells"
	"heliosian/internal/id"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	activitiesHost         = "team"
	kindVolunteer          = "volunteer"
	kindActivitiesSettings = "team-settings"
	kindActivityRedirect   = "activity-redirect"
)

type ActivitiesHooks struct {
	app activitiesApp
}

type volunteerKey struct {
	activity *Activity
	email    string
}

type activityMe struct {
	Runs     bool   `json:"runs"`
	Position string `json:"position,omitempty"`
}

type activityResource struct {
	ID                  string     `json:"id"`
	Year                string     `json:"year"`
	Title               string     `json:"title"`
	Parent              string     `json:"parent,omitempty"`
	Category            string     `json:"category,omitempty"`
	Status              string     `json:"status"`
	Description         string     `json:"description,omitempty"`
	Image               string     `json:"image,omitempty"`
	ImageURL            string     `json:"imageUrl,omitempty"`
	Flyer               string     `json:"flyer,omitempty"`
	FlyerURL            string     `json:"flyerUrl,omitempty"`
	Highlight           *Highlight `json:"highlight,omitempty"`
	Timing              string     `json:"timing,omitempty"`
	Start               string     `json:"start,omitempty"`
	End                 string     `json:"end,omitempty"`
	Location            string     `json:"location,omitempty"`
	Spots               int        `json:"spots,omitempty"`
	CoLeaderNeeded      bool       `json:"coLeaderNeeded"`
	VolunteersComplete  bool       `json:"volunteersComplete"`
	VolunteersHiddenOwn string     `json:"volunteersHiddenOwn,omitempty"`
	VolunteersHidden    bool       `json:"volunteersHidden"`
	DirectSignUpOwn     string     `json:"directSignUpOwn,omitempty"`
	DirectSignUp        bool       `json:"directSignUp"`
	CategoryHidden      bool       `json:"categoryHidden,omitempty"`
	Priority            bool       `json:"priority"`
	PrettyID            string     `json:"prettyId,omitempty"`
	AllowAddingOwn      string     `json:"allowAddingOwn,omitempty"`
	AllowAdding         string     `json:"allowAdding"`
	AddedBy             string     `json:"addedBy,omitempty"`
	Added               string     `json:"added,omitempty"`
	Taken               int        `json:"taken"`
	Full                bool       `json:"full"`
	Past                bool       `json:"past"`
	Started             bool       `json:"started,omitempty"`
	Invited             bool       `json:"invited,omitempty"`
	Day                 string     `json:"day,omitempty"`
	DayTiming           string     `json:"dayTiming,omitempty"`
	Picture             string     `json:"picture,omitempty"`
	Under               string     `json:"under,omitempty"`
	Wants               string     `json:"wants,omitempty"`
	Path                string     `json:"path"`
	App                 string     `json:"app"`
	Me                  activityMe `json:"me"`
}

type volunteerResource struct {
	Volunteer
}

type activityLinkResource struct {
	ActivityLink
}

type activitiesSettingsResource struct {
	User          ActivitiesUser     `json:"user"`
	Years         Years              `json:"years"`
	Today         string             `json:"today"`
	Settings      ActivitiesSettings `json:"settings"`
	GradeColors   map[string]string  `json:"gradeColors,omitempty"`
	ImageSearch   bool               `json:"imageSearch"`
	People        []PersonView       `json:"people,omitempty"`
	Uncategorized *ActivityCategory  `json:"uncategorized,omitempty"`
	Notify        []string           `json:"notify,omitempty"`
}

type activityOrder struct {
	IDs []string `json:"ids"`
}

type activityLinkBody struct {
	Activity    string `json:"activity"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Image       string `json:"image"`
}

type volunteerEdit struct {
	Position string `json:"position"`
	Note     string `json:"note"`
}

type redirectForm struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func (h ActivitiesHooks) Resources() []api.Type[*Model] {
	a := h.app
	return []api.Type[*Model]{a.activitiesType(), a.volunteersType(), a.linksType(), a.activityCategoriesType(), a.teamSettingsType(), a.redirectsType()}
}

func (a activitiesApp) volunteerID(activityID, email string) string {
	return id.Of(a.calendar.idKey, kindVolunteer, activityID+"\x00"+email)
}

func (a activitiesApp) teamSettingsKey() string {
	return id.Of(a.calendar.idKey, kindActivitiesSettings, "")
}

func (a activitiesApp) redirectKey(old string) string {
	return id.Of(a.calendar.idKey, kindActivityRedirect, strings.ToLower(old))
}

func (a activitiesApp) volunteerAt(m *Model, key string) (volunteerKey, bool) {
	s := m.scope
	s.volunteersOnce.Do(func() {
		s.volunteerKeys = map[string]volunteerKey{}
		for _, act := range m.Activities.byID {
			for _, v := range act.Volunteers {
				s.volunteerKeys[a.volunteerID(act.ID, v.Email)] = volunteerKey{activity: act, email: v.Email}
			}
		}
	})
	k, ok := s.volunteerKeys[key]
	return k, ok
}

func (a activitiesApp) eventOf(m *Model, activityID string) string {
	s := m.scope
	s.linkedOnce.Do(func() {
		s.activityEvents = map[string]string{}
		cal := a.calendar.at(m)
		for _, e := range withLinked(m.Calendar.Events, cal.linked("")) {
			if e.linked() {
				s.activityEvents[e.LinkedID] = e.ID
			}
		}
	})
	return s.activityEvents[activityID]
}

func (a activitiesApp) rsvpsOf(m *Model, activityID string) *GuestAnswers {
	eventID := a.eventOf(m, activityID)
	if eventID == "" {
		return nil
	}
	return a.calendar.at(m).guestAnswers(eventID)
}

func (a activitiesApp) visibleActivity(m *Model, q api.Query, key string) *Activity {
	act := m.Activities.byID[key]
	if act == nil || !m.Activities.VisibleTo(act, q.Actor) {
		return nil
	}
	return act
}

func pastActivity(m *Activities, act *Activity, now time.Time) bool {
	if act.Status == StatusDone {
		return true
	}
	dated := act
	for dated.Start == "" && dated.Timing == "" && dated.Parent != "" && m.byID[dated.Parent] != nil {
		dated = m.byID[dated.Parent]
	}
	cell := dated.End
	if cell == "" {
		cell = dated.Start
	}
	if cell == "" {
		return false
	}
	last, err := cells.When(cell)
	if err != nil {
		return false
	}
	return last.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC))
}

func (a activitiesApp) activityResource(m *Model, q api.Query, raw *Activity) activityResource {
	acts := m.Activities
	shown := acts.activityFor(raw, q.Actor)
	runs := acts.Runs(raw, q.Actor.Email)
	out := activityResource{
		ID: raw.ID, Year: raw.Year, Title: raw.Title, Parent: raw.Parent, Category: raw.Category, Status: raw.Status,
		Description: raw.Description, Image: raw.Image, ImageURL: raw.ImageURL, Flyer: raw.Flyer, FlyerURL: raw.FlyerURL, Highlight: raw.Highlight,
		Timing: raw.Timing, Start: raw.Start, End: raw.End, Location: raw.Location, Spots: raw.Spots, CoLeaderNeeded: raw.CoLeaderNeeded,
		VolunteersComplete: raw.VolunteersComplete, VolunteersHiddenOwn: raw.VolunteersHiddenOwn, VolunteersHidden: raw.VolunteersHidden,
		DirectSignUpOwn: raw.DirectSignUpOwn, DirectSignUp: raw.DirectSignUp, CategoryHidden: raw.CategoryHidden, Priority: raw.Priority,
		PrettyID: raw.PrettyID, AllowAddingOwn: raw.AllowAdding, AllowAdding: raw.Adding, AddedBy: raw.AddedBy, Added: raw.Added,
		Taken: shown.Taken, Full: raw.full(), Past: pastActivity(acts, raw, q.Now),
		Day: dayOf(timed(acts, raw).Start), DayTiming: timed(acts, raw).Timing, Picture: acts.picture(raw), Under: lineage(acts, raw),
		Path: acts.PathOf(raw), App: activitiesHost, Me: activityMe{Runs: runs},
	}
	if raw.Status == StatusOpen {
		out.Wants = acts.openNote(raw)
	}
	if v, on := acts.volunteerOn(raw, q.Actor.Email); on {
		out.Me.Position = v.Position
	}
	if acts.Sees(raw, q.Actor) {
		if r := a.rsvpsOf(m, raw.ID); r != nil {
			out.Started, out.Invited = true, r.Sent
		}
	}
	return out
}

func (a activitiesApp) shownVolunteers(m *Model, q api.Query, raw *Activity) []Volunteer {
	shown := m.Activities.activityFor(raw, q.Actor).Volunteers
	sort.SliceStable(shown, func(i, j int) bool {
		return shown[i].Position == PositionCoChair && shown[j].Position != PositionCoChair
	})
	return shown
}

func (a activitiesApp) volunteerResource(m *Model, q api.Query, raw *Activity, vol Volunteer) volunteerResource {
	v := activityViewer{Actor: q.Actor, directory: m.Directory}
	out := v.volunteers([]Volunteer{vol})[0]
	if m.Activities.Sees(raw, q.Actor) {
		if r := a.rsvpsOf(m, raw.ID); r != nil && r.Sent {
			out.RSVP = r.Answers[m.Directory.Resolve(strings.ToLower(out.Email))]
		}
	}
	return volunteerResource{out}
}

type activityCheck func(m *Model, q api.Query, act *Activity) bool

func activityWhere(keep func(m *Model, q api.Query, act *Activity, value string) bool) api.Filter[*Model] {
	return func(m *Model, q api.Query, value string) (func(string) bool, error) {
		value = strings.TrimSpace(value)
		return func(key string) bool {
			act := m.Activities.byID[key]
			return act != nil && keep(m, q, act, value)
		}, nil
	}
}

func (a activitiesApp) onActivity(can activityCheck) func(*Model, api.Query, string) bool {
	return func(m *Model, q api.Query, key string) bool {
		act := a.visibleActivity(m, q, key)
		return act != nil && can(m, q, act)
	}
}

func (a activitiesApp) guarded(wr api.Write[*Model], can activityCheck) (*Activity, error) {
	act := a.visibleActivity(wr.S, wr.Query, wr.ID)
	if act == nil || !can(wr.S, wr.Query, act) {
		return nil, access.Forbidden("that is not yours to do here")
	}
	return act, nil
}

func activityDo[In any](a activitiesApp, can activityCheck, do func(wr api.Write[*Model], act *Activity, in In) error) api.Action[*Model] {
	return api.Do(a.onActivity(can), func(wr api.Write[*Model], in In) error {
		act, err := a.guarded(wr, can)
		if err != nil {
			return err
		}
		return do(wr, act, in)
	})
}

func activityMaking[In any](a activitiesApp, can activityCheck, do func(wr api.Write[*Model], act *Activity, in In) (string, error)) api.Action[*Model] {
	return api.DoMaking(a.onActivity(can), func(wr api.Write[*Model], in In) (string, error) {
		act, err := a.guarded(wr, can)
		if err != nil {
			return "", err
		}
		return do(wr, act, in)
	})
}

func edits(m *Model, q api.Query, act *Activity) bool {
	return m.Activities.Edits(act, q.Actor)
}

func pending(m *Model, q api.Query, act *Activity) bool {
	return act.Status == StatusPending
}

func (a activitiesApp) create(wr api.Write[*Model], patch activityPatch) (string, error) {
	delete(patch, "id")
	saved, err := wr.S.Activities.saveActivity(wr.Query.Actor, patch)
	if err := a.store.stage(wr, activitiesAppName, saved.ops, err); err != nil {
		return "", err
	}
	logAfter(wr, "team: saved activity", "action", saved.action, "activity", saved.title, "id", saved.id, "year", saved.year, "status", saved.status)
	r, by, key := wr.Request, wr.Query.Actor.Email, saved.id
	wr.Tx.After(func() { a.mailNewActivity(r, a.activities().Activity(key), by) })
	return saved.id, nil
}

func (a activitiesApp) saveOver(wr api.Write[*Model], key string, patch activityPatch) error {
	raw, err := json.Marshal(key)
	if err != nil {
		return err
	}
	patch["id"] = raw
	saved, err := wr.S.Activities.saveActivity(wr.Query.Actor, patch)
	if err := a.store.stage(wr, activitiesAppName, saved.ops, err); err != nil {
		return err
	}
	logAfter(wr, "team: saved activity", "action", saved.action, "activity", saved.title, "id", saved.id, "year", saved.year, "status", saved.status)
	return nil
}

func statusPatch(status string) activityPatch {
	raw, _ := json.Marshal(status)
	return activityPatch{"status": raw}
}

func (a activitiesApp) activitiesType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "activities",
		Shape: activityResource{},
		Has:   func(m *Model, key string) bool { return m.Activities.byID[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			act := a.visibleActivity(m, q, key)
			if act == nil {
				return nil, false
			}
			return a.activityResource(m, q, act), true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			var walk func(list []*Activity)
			walk = func(list []*Activity) {
				for _, act := range list {
					if m.Activities.VisibleTo(act, q.Actor) {
						out = append(out, act.ID)
						walk(act.Children)
					}
				}
			}
			walk(m.Activities.Activities)
			return out
		},
		Aliases: func(m *Model) map[string]string {
			acts := m.Activities
			out := map[string]string{}
			for old, target := range acts.aliases {
				if acts.byID[target] != nil {
					out[old] = target
				}
			}
			for pretty, act := range acts.pretty {
				out[pretty] = act.ID
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"parent": {Type: "activities", List: func(m *Model, q api.Query, key string) []string {
				if act := a.visibleActivity(m, q, key); act != nil && act.Parent != "" {
					return []string{act.Parent}
				}
				return nil
			}},
			"children": {Type: "activities", Many: true, List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				out := []string{}
				for _, c := range act.Children {
					if m.Activities.VisibleTo(c, q.Actor) {
						out = append(out, c.ID)
					}
				}
				return out
			}},
			"volunteers": {Type: "volunteers", Many: true, List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				out := []string{}
				for _, v := range a.shownVolunteers(m, q, act) {
					out = append(out, a.volunteerID(act.ID, v.Email))
				}
				return out
			}},
			"links": {Type: "activity-links", Many: true, List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				out := []string{}
				for _, l := range act.Links {
					out = append(out, l.ID)
				}
				return out
			}},
			"categories": {Type: "activity-categories", Many: true, List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				out := []string{}
				for _, c := range act.Categories {
					out = append(out, c.ID)
				}
				return out
			}},
			"category": {Type: "activity-categories", List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				if c := m.Activities.categories[act.Category]; c == nil || c.BuiltIn {
					return nil
				}
				return []string{act.Category}
			}},
			"event": {Type: "events", List: func(m *Model, q api.Query, key string) []string {
				act := a.visibleActivity(m, q, key)
				if act == nil {
					return nil
				}
				eventID := a.eventOf(m, act.ID)
				if eventID == "" || a.calendar.at(m).viewerEvent(q, eventID) == nil {
					return nil
				}
				return []string{eventID}
			}},
		},
		Filters: map[string]api.Filter[*Model]{
			"q": activityWhere(func(_ *Model, _ api.Query, act *Activity, value string) bool {
				return mentions(value, act.Title, act.Description)
			}),
			"year": activityWhere(func(_ *Model, q api.Query, act *Activity, value string) bool {
				if value == "current" {
					value = ActivityYear(q.Now)
				}
				return act.Year == value
			}),
			"past": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				want, err := strconv.ParseBool(value)
				if err != nil {
					return nil, access.Invalid("past takes true or false")
				}
				return activityWhere(func(m *Model, q api.Query, act *Activity, _ string) bool {
					return pastActivity(m.Activities, act, q.Now) == want
				})(m, q, value)
			},
			"under": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				root := m.Activities.byID[strings.TrimSpace(value)]
				if root == nil {
					return nil, access.Missing("no activity %s", value)
				}
				below := map[string]bool{}
				for _, d := range root.Descendants() {
					below[d.ID] = true
				}
				return func(key string) bool { return below[key] }, nil
			},
		},
		Create: api.Make(func(wr api.Write[*Model], patch activityPatch) (string, error) {
			return a.create(wr, patch)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": activityDo(a, edits, func(wr api.Write[*Model], act *Activity, patch activityPatch) error {
				return a.saveOver(wr, act.ID, patch)
			}),
			"approve": activityDo(a, func(m *Model, q api.Query, act *Activity) bool {
				return pending(m, q, act) && (q.Actor.May(CurateActivities) || (act.Parent != "" && m.Activities.Runs(m.Activities.byID[act.Parent], q.Actor.Email)))
			}, func(wr api.Write[*Model], act *Activity, _ serve.None) error {
				return a.saveOver(wr, act.ID, statusPatch(StatusOpen))
			}),
			"decline": activityDo(a, func(m *Model, q api.Query, act *Activity) bool {
				return pending(m, q, act) && q.Actor.May(CurateActivities)
			}, func(wr api.Write[*Model], act *Activity, _ serve.None) error {
				return a.saveOver(wr, act.ID, statusPatch(StatusHidden))
			}),
			"add": api.DoMaking(a.onActivity(func(m *Model, q api.Query, act *Activity) bool {
				return permitted(second(m.Activities.addedStatus(q.Actor, act.ID, "")))
			}), func(wr api.Write[*Model], patch activityPatch) (string, error) {
				raw, err := json.Marshal(wr.ID)
				if err != nil {
					return "", err
				}
				patch["parent"] = raw
				return a.create(wr, patch)
			}),
			"delete": activityDo(a, func(m *Model, q api.Query, act *Activity) bool {
				return q.Actor.May(CurateActivities) && len(act.Volunteers) == 0 && len(act.Children) == 0
			}, func(wr api.Write[*Model], act *Activity, _ serve.None) error {
				deleted, ops, err := wr.S.Activities.deleteActivity(wr.Query.Actor, act.ID)
				if err != nil {
					return err
				}
				logAfter(wr, "team: deleted activity", "activity", deleted.Title, "year", deleted.Year)
				return stage(wr, activitiesAppName, ops, nil)
			}),
			"order": activityDo(a, edits, func(wr api.Write[*Model], act *Activity, body activityOrder) error {
				parent, ops, err := wr.S.Activities.orderChildren(wr.Query.Actor, act.ID, body.IDs)
				if err != nil {
					return err
				}
				logAfter(wr, "team: reordered", "parent", parent.Title, "year", parent.Year, "changed", len(ops))
				return stage(wr, activitiesAppName, ops, nil)
			}),
			"order-categories": activityDo(a, func(m *Model, q api.Query, act *Activity) bool {
				return act.Parent == "" && permitted(m.Activities.editsCategories(q.Actor, act.ID))
			}, func(wr api.Write[*Model], act *Activity, body activityOrder) error {
				ops, err := wr.S.Activities.reorderCategories(wr.Query.Actor, act.ID, body.IDs)
				logAfter(wr, "team: reordered categories", "event", act.ID, "count", len(body.IDs))
				return stage(wr, activitiesAppName, ops, err)
			}),
			"copy": activityMaking(a, func(m *Model, q api.Query, act *Activity) bool {
				if !q.Actor.May(CurateActivities) || act.Parent != "" {
					return false
				}
				year := ShiftYearSpan(act.Year, 1)
				return !slices.ContainsFunc(m.Activities.Activities, func(o *Activity) bool { return o.Year == year && o.Title == act.Title })
			}, func(wr api.Write[*Model], act *Activity, _ serve.None) (string, error) {
				copied, key, year, ops, err := wr.S.Activities.copyActivity(wr.Query.Actor, act.ID)
				if err := stage(wr, activitiesAppName, ops, err); err != nil {
					return "", err
				}
				logAfter(wr, "team: copied activity", "activity", copied.Title, "from", copied.Year, "to", year, "id", key)
				return key, nil
			}),
			"sign-up": api.Do(a.onActivity(func(m *Model, q api.Query, act *Activity) bool {
				return permitted(second(m.Activities.saveVolunteer(q.Actor, m.Directory, volunteerBody{ID: act.ID, Position: PositionVolunteer})))
			}), func(wr api.Write[*Model], body volunteerBody) error {
				body.ID = wr.ID
				return a.signUp(wr, body)
			}),
		},
	}
}

func second[T any](_ T, err error) error {
	return err
}

func (a activitiesApp) signUp(wr api.Write[*Model], body volunteerBody) error {
	s, err := wr.S.Activities.saveVolunteer(wr.Query.Actor, wr.S.Directory, body)
	if err := a.store.stage(wr, activitiesAppName, s.ops, err); err != nil {
		return err
	}
	logAfter(wr, "team: saved volunteer", "action", s.action, "email", s.email, "activity", s.act.Title, "year", s.act.Year)
	r, by := wr.Request, wr.Query.Actor.Email
	wr.Tx.After(func() { a.mailSignUp(r, s.act, s.email, s.position, s.note, by, s.existed, s.was) })
	return nil
}

func (a activitiesApp) visibleVolunteer(m *Model, q api.Query, key string) (*Activity, Volunteer, bool) {
	k, ok := a.volunteerAt(m, key)
	if !ok || !m.Activities.VisibleTo(k.activity, q.Actor) {
		return nil, Volunteer{}, false
	}
	for _, v := range a.shownVolunteers(m, q, k.activity) {
		if v.Email == k.email {
			return k.activity, v, true
		}
	}
	return nil, Volunteer{}, false
}

func (a activitiesApp) onVolunteer(can func(m *Model, q api.Query, act *Activity, v Volunteer) bool) func(*Model, api.Query, string) bool {
	return func(m *Model, q api.Query, key string) bool {
		act, v, ok := a.visibleVolunteer(m, q, key)
		return ok && can(m, q, act, v)
	}
}

func changesVolunteer(m *Model, q api.Query, act *Activity, v Volunteer) bool {
	return q.Actor.Mine(v.Email) || m.Activities.Edits(act, q.Actor)
}

func (a activitiesApp) volunteersType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "volunteers",
		Shape: volunteerResource{},
		Has: func(m *Model, key string) bool {
			_, ok := a.volunteerAt(m, key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			act, v, ok := a.visibleVolunteer(m, q, key)
			if !ok {
				return nil, false
			}
			return a.volunteerResource(m, q, act, v), true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			var walk func(list []*Activity)
			walk = func(list []*Activity) {
				for _, act := range list {
					if !m.Activities.VisibleTo(act, q.Actor) {
						continue
					}
					for _, v := range a.shownVolunteers(m, q, act) {
						out = append(out, a.volunteerID(act.ID, v.Email))
					}
					walk(act.Children)
				}
			}
			walk(m.Activities.Activities)
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"activity": {Type: "activities", List: func(m *Model, q api.Query, key string) []string {
				if act, _, ok := a.visibleVolunteer(m, q, key); ok {
					return []string{act.ID}
				}
				return nil
			}},
			"person": {Type: "people", List: func(m *Model, q api.Query, key string) []string {
				if _, v, ok := a.visibleVolunteer(m, q, key); ok {
					return m.personID(v.Email)
				}
				return nil
			}},
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(a.onVolunteer(changesVolunteer), func(wr api.Write[*Model]) volunteerEdit {
				_, v, _ := a.visibleVolunteer(wr.S, wr.Query, wr.ID)
				return volunteerEdit{Position: v.Position, Note: v.Note}
			}, func(wr api.Write[*Model], body volunteerEdit) error {
				act, v, ok := a.visibleVolunteer(wr.S, wr.Query, wr.ID)
				if !ok || !changesVolunteer(wr.S, wr.Query, act, v) {
					return access.Forbidden("only a co-chair or admin can change someone else's sign-up")
				}
				return a.signUp(wr, volunteerBody{ID: act.ID, Email: v.Email, Position: body.Position, Note: body.Note})
			}),
			"delete": api.Do(a.onVolunteer(changesVolunteer), func(wr api.Write[*Model], _ serve.None) error {
				act, v, ok := a.visibleVolunteer(wr.S, wr.Query, wr.ID)
				if !ok {
					return access.Missing("no such sign-up")
				}
				removed, email, ops, err := wr.S.Activities.removeVolunteer(wr.Query.Actor, act.ID, v.Email)
				if err := stage(wr, activitiesAppName, ops, err); err != nil {
					return err
				}
				logAfter(wr, "team: removed volunteer", "email", email, "activity", removed.Title, "year", removed.Year)
				r, by := wr.Request, wr.Query.Actor.Email
				wr.Tx.After(func() { a.mailRemoved(r, removed, email, by) })
				return nil
			}),
		},
	}
}

func (a activitiesApp) visibleLink(m *Model, q api.Query, key string) (*Activity, ActivityLink, bool) {
	act := m.Activities.links[key]
	if act == nil || !m.Activities.VisibleTo(act, q.Actor) {
		return nil, ActivityLink{}, false
	}
	i := slices.IndexFunc(act.Links, func(l ActivityLink) bool { return l.ID == key })
	if i < 0 {
		return nil, ActivityLink{}, false
	}
	return act, act.Links[i], true
}

func (a activitiesApp) onLink(m *Model, q api.Query, key string) bool {
	act, _, ok := a.visibleLink(m, q, key)
	return ok && m.Activities.Edits(act, q.Actor)
}

func (a activitiesApp) linksType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "activity-links",
		Shape: activityLinkResource{},
		Has:   func(m *Model, key string) bool { return m.Activities.links[key] != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			_, l, ok := a.visibleLink(m, q, key)
			if !ok {
				return nil, false
			}
			return activityLinkResource{l}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, act := range m.Activities.byID {
				if !m.Activities.VisibleTo(act, q.Actor) {
					continue
				}
				for _, l := range act.Links {
					out = append(out, l.ID)
				}
			}
			slices.Sort(out)
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for old, target := range m.Activities.aliases {
				if m.Activities.links[target] != nil {
					out[old] = target
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"activity": {Type: "activities", List: func(m *Model, q api.Query, key string) []string {
				if act, _, ok := a.visibleLink(m, q, key); ok {
					return []string{act.ID}
				}
				return nil
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], body activityLinkBody) (string, error) {
			act, key, ops, err := wr.S.Activities.saveLink(wr.Query.Actor, linkBody{ID: body.Activity, Title: body.Title, URL: body.URL, Description: body.Description, Image: body.Image})
			if err := stage(wr, activitiesAppName, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "team: saved link", "action", "add", "link", strings.TrimSpace(body.Title), "activity", act.Title, "year", act.Year)
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(a.onLink, func(wr api.Write[*Model]) activityLinkBody {
				_, l, _ := a.visibleLink(wr.S, wr.Query, wr.ID)
				return activityLinkBody{Title: l.Title, URL: l.URL, Description: l.Description, Image: l.Image}
			}, func(wr api.Write[*Model], body activityLinkBody) error {
				act, _, ops, err := wr.S.Activities.saveLink(wr.Query.Actor, linkBody{Link: wr.ID, Title: body.Title, URL: body.URL, Description: body.Description, Image: body.Image})
				if err := stage(wr, activitiesAppName, ops, err); err != nil {
					return err
				}
				logAfter(wr, "team: saved link", "action", "edit", "link", strings.TrimSpace(body.Title), "activity", act.Title, "year", act.Year)
				return nil
			}),
			"delete": api.Do(a.onLink, func(wr api.Write[*Model], _ serve.None) error {
				act, ops, err := wr.S.Activities.deleteLink(wr.Query.Actor, wr.ID)
				if err := stage(wr, activitiesAppName, ops, err); err != nil {
					return err
				}
				logAfter(wr, "team: deleted link", "link", wr.ID, "activity", act.Title, "year", act.Year)
				return nil
			}),
		},
	}
}

func (a activitiesApp) visibleCategory(m *Model, q api.Query, key string) *ActivityCategory {
	c := m.Activities.categories[key]
	if c == nil || c.BuiltIn {
		return nil
	}
	if c.EventID != "" && a.visibleActivity(m, q, c.EventID) == nil {
		return nil
	}
	return c
}

func (a activitiesApp) onCategory(can func(m *Model, q api.Query, c *ActivityCategory) bool) func(*Model, api.Query, string) bool {
	return func(m *Model, q api.Query, key string) bool {
		c := a.visibleCategory(m, q, key)
		return c != nil && can(m, q, c)
	}
}

func editsCategory(m *Model, q api.Query, c *ActivityCategory) bool {
	return permitted(m.Activities.editsCategories(q.Actor, c.EventID))
}

func categoryBodyOf(c *ActivityCategory) categoryBody {
	onMain := c.ShowOnMain
	return categoryBody{ID: c.ID, EventID: c.EventID, Title: c.Title, Description: c.Description, Image: c.Image, AllowAdding: c.AllowAdding, ShowOnMain: &onMain}
}

func addsUnder(m *Model, q api.Query, c *ActivityCategory) bool {
	return permitted(second(m.Activities.addedStatus(q.Actor, c.EventID, c.ID)))
}

func (a activitiesApp) activityCategoriesType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  "activity-categories",
		Shape: ActivityCategory{},
		Has: func(m *Model, key string) bool {
			c := m.Activities.categories[key]
			return c != nil && !c.BuiltIn
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			c := a.visibleCategory(m, q, key)
			if c == nil {
				return nil, false
			}
			return *c, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, c := range m.Activities.Categories {
				if !c.BuiltIn {
					out = append(out, c.ID)
				}
			}
			for _, root := range m.Activities.Activities {
				if !m.Activities.VisibleTo(root, q.Actor) {
					continue
				}
				for _, c := range root.Categories {
					out = append(out, c.ID)
				}
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for old, target := range m.Activities.aliases {
				if c := m.Activities.categories[target]; c != nil && !c.BuiltIn {
					out[old] = target
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"event": {Type: "activities", List: func(m *Model, q api.Query, key string) []string {
				if c := a.visibleCategory(m, q, key); c != nil && c.EventID != "" {
					return []string{c.EventID}
				}
				return nil
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], body categoryBody) (string, error) {
			body.ID = ""
			s, err := wr.S.Activities.saveCategory(wr.Query.Actor, body)
			if err != nil {
				return "", err
			}
			logAfter(wr, "team: saved category", "action", s.action, "category", s.title, "id", s.id, "event", s.eventID)
			return s.id, stage(wr, activitiesAppName, []store.Op{s.op}, nil)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(a.onCategory(editsCategory), func(wr api.Write[*Model]) categoryBody {
				return categoryBodyOf(wr.S.Activities.categories[wr.ID])
			}, func(wr api.Write[*Model], body categoryBody) error {
				body.ID = wr.ID
				s, err := wr.S.Activities.saveCategory(wr.Query.Actor, body)
				if err != nil {
					return err
				}
				logAfter(wr, "team: saved category", "action", s.action, "category", s.title, "id", s.id, "event", s.eventID)
				return stage(wr, activitiesAppName, []store.Op{s.op}, nil)
			}),
			"settings": api.Do(a.onCategory(func(m *Model, q api.Query, c *ActivityCategory) bool {
				return c.EventID != "" && editsCategory(m, q, c)
			}), func(wr api.Write[*Model], body categoryFlagsBody) error {
				body.ID = wr.ID
				op, c, err := wr.S.Activities.saveCategoryFlags(wr.Query.Actor, body)
				if err != nil {
					return err
				}
				logAfter(wr, "team: saved category settings", "category", c.Title, "id", c.ID, "flags", body.Flags)
				return stage(wr, activitiesAppName, []store.Op{op}, nil)
			}),
			"add": api.DoMaking(a.onCategory(addsUnder), func(wr api.Write[*Model], patch activityPatch) (string, error) {
				c := a.visibleCategory(wr.S, wr.Query, wr.ID)
				category, err := json.Marshal(c.ID)
				if err != nil {
					return "", err
				}
				patch["category"] = category
				if c.EventID == "" {
					delete(patch, "parent")
				}
				if _, sent := patch["parent"]; !sent && c.EventID != "" {
					patch["parent"], _ = json.Marshal(c.EventID)
				}
				return a.create(wr, patch)
			}),
			"delete": api.Do(a.onCategory(func(m *Model, q api.Query, c *ActivityCategory) bool {
				if !editsCategory(m, q, c) {
					return false
				}
				for _, act := range m.Activities.byID {
					if act.Category == c.ID {
						return false
					}
				}
				return true
			}), func(wr api.Write[*Model], _ serve.None) error {
				c, ops, err := a.store.deleteActivityCategory(wr.Query.Actor, wr.ID)
				if err != nil {
					return err
				}
				logAfter(wr, "team: deleted category", "category", c.Title, "id", c.ID)
				return stage(wr, activitiesAppName, ops, nil)
			}),
		},
	}
}

func widgetList(pick func(activityWidget) []*Activity) func(*Model, api.Query, string) []string {
	return func(m *Model, q api.Query, _ string) []string {
		s := m.scope
		s.teamWidgetOnce.Do(func() { s.teamWidget = m.Activities.widget(q.Actor.Email, q.Now) })
		out := []string{}
		for _, act := range pick(s.teamWidget) {
			out = append(out, act.ID)
		}
		return out
	}
}

func (a activitiesApp) teamSettingsType() api.Type[*Model] {
	stage := a.store.stage
	configures := may(ConfigureActivities)
	return api.Type[*Model]{
		Name:  "team-settings",
		Shape: activitiesSettingsResource{},
		Has:   func(_ *Model, key string) bool { return key == a.teamSettingsKey() },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != a.teamSettingsKey() {
				return nil, false
			}
			acts := m.Activities
			v := activityViewer{Actor: q.Actor, directory: m.Directory}
			name, photo := v.person(q.Actor.Email)
			spouses, children := activityHousehold(m.Directory, m.Directory.Person(q.Actor.Email))
			current := ActivityYear(q.Now)
			out := activitiesSettingsResource{
				User:          ActivitiesUser{Email: q.Actor.Email, Name: name, Initial: strings.ToUpper(name[:1]), PhotoURL: photo, Spouses: spouses, Children: children},
				Years:         Years{Current: current, Last: ShiftYearSpan(current, -1), Next: ShiftYearSpan(current, 1)},
				Today:         q.Now.Format(DateFormat),
				Settings:      acts.Settings,
				GradeColors:   m.Config.GradeColors,
				ImageSearch:   a.search.On(),
				Uncategorized: acts.categories[UncategorizedID],
			}
			if q.Actor.May(SeeAllActivities) {
				out.People = v.people(acts)
			}
			if q.Actor.May(ConfigureActivities) {
				prefs := acts.notifyPrefs(q.Actor.Email)
				out.Notify = []string{}
				for _, k := range NotifyKinds {
					if prefs[k] {
						out.Notify = append(out.Notify, k)
					}
				}
			}
			return out, true
		},
		List: func(*Model, api.Query) []string { return []string{a.teamSettingsKey()} },
		Relations: map[string]api.Relation[*Model]{
			"viewer":   {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
			"mine":     {Type: "activities", Many: true, List: widgetList(func(w activityWidget) []*Activity { return w.mine })},
			"needed":   {Type: "activities", Many: true, List: widgetList(func(w activityWidget) []*Activity { return w.needed })},
			"priority": {Type: "activities", Many: true, List: widgetList(func(w activityWidget) []*Activity { return w.priority })},
		},
		Actions: map[string]api.Action[*Model]{
			"settings": api.DoFrom(configures, func(wr api.Write[*Model]) activitySettingsBody {
				s := wr.S.Activities.Settings
				return activitySettingsBody{ExpenseFormURL: s.ExpenseFormURL, Intro: s.Intro}
			}, func(wr api.Write[*Model], body activitySettingsBody) error {
				ops, err := activitySettingsOps(wr.Query.Actor, body.ExpenseFormURL, body.Intro)
				logAfter(wr, "team: changed the settings")
				return stage(wr, activitiesAppName, ops, err)
			}),
			"notify": api.Do(configures, func(wr api.Write[*Model], body notifyBody) error {
				value, ops, err := activityNotifyOps(wr.Query.Actor, body.Kinds)
				logAfter(wr, "team: set notifications", "kinds", value)
				return stage(wr, activitiesAppName, ops, err)
			}),
			"order-categories": api.Do(configures, func(wr api.Write[*Model], body activityOrder) error {
				ops, err := wr.S.Activities.reorderCategories(wr.Query.Actor, "", body.IDs)
				logAfter(wr, "team: reordered categories", "count", len(body.IDs))
				return stage(wr, activitiesAppName, ops, err)
			}),
		},
	}
}

func (a activitiesApp) redirectAt(m *Model, q api.Query, key string) *ActivityRedirect {
	if !q.Actor.May(ConfigureActivities) {
		return nil
	}
	for i, r := range m.Activities.Redirects {
		if a.redirectKey(r.Old) == key {
			return &m.Activities.Redirects[i]
		}
	}
	return nil
}

func (a activitiesApp) redirectsType() api.Type[*Model] {
	stage := a.store.stage
	configures := may(ConfigureActivities)
	return api.Type[*Model]{
		Name:  "activity-redirects",
		Shape: ActivityRedirect{},
		Has: func(m *Model, key string) bool {
			return slices.ContainsFunc(m.Activities.Redirects, func(r ActivityRedirect) bool { return a.redirectKey(r.Old) == key })
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			r := a.redirectAt(m, q, key)
			if r == nil {
				return nil, false
			}
			return *r, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			if !q.Actor.May(ConfigureActivities) {
				return out
			}
			for _, r := range m.Activities.Redirects {
				out = append(out, a.redirectKey(r.Old))
			}
			return out
		},
		Create: api.Make(func(wr api.Write[*Model], body redirectForm) (string, error) {
			s, err := wr.S.Activities.saveRedirect(wr.Query.Actor, "", body.Old, body.New)
			if err != nil {
				return "", err
			}
			logAfter(wr, "team: saved redirect", "action", s.action, "old", s.old, "new", s.to)
			return a.redirectKey(s.old), stage(wr, activitiesAppName, []store.Op{s.op}, nil)
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(configures, func(wr api.Write[*Model]) redirectForm {
				r := a.redirectAt(wr.S, wr.Query, wr.ID)
				return redirectForm{Old: r.Old, New: r.New}
			}, func(wr api.Write[*Model], body redirectForm) error {
				s, err := wr.S.Activities.saveRedirect(wr.Query.Actor, a.redirectAt(wr.S, wr.Query, wr.ID).Old, body.Old, body.New)
				if err != nil {
					return err
				}
				logAfter(wr, "team: saved redirect", "action", s.action, "old", s.old, "new", s.to)
				return stage(wr, activitiesAppName, []store.Op{s.op}, nil)
			}),
			"delete": api.Do(configures, func(wr api.Write[*Model], _ serve.None) error {
				r, ops, err := wr.S.Activities.deleteRedirect(wr.Query.Actor, a.redirectAt(wr.S, wr.Query, wr.ID).Old)
				if err != nil {
					return err
				}
				logAfter(wr, "team: deleted redirect", "old", r.Old)
				return stage(wr, activitiesAppName, ops, nil)
			}),
		},
	}
}
