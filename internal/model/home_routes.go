package model

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
)

const homeShell = "web/home/index.html"

type homeApp struct {
	store  *Store
	hooks  CalendarHooks
	search imagesearch.Search
	style  *sharecard.Style
}

type HomeUpcoming struct {
	Events    []EventCard     `json:"events"`
	Calendar  string          `json:"calendar,omitempty"`
	Default   string          `json:"default,omitempty"`
	Calendars []SavedCalendar `json:"calendars,omitempty"`
}

type SavedCalendar struct {
	Token  string `json:"token"`
	Name   string `json:"name"`
	Emoji  string `json:"emoji,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

type HomeMonth struct {
	Month    string                 `json:"month"`
	Today    string                 `json:"today"`
	Days     map[string]CalendarDay `json:"days"`
	Events   []EventCard            `json:"events"`
	Calendar string                 `json:"calendar,omitempty"`
}

type HomeDeps struct {
	Store    *Store
	Images   blob.Images
	Calendar CalendarHooks
	Search   imagesearch.Search
	Style    *sharecard.Style
}

func RegisterHome(mux *http.ServeMux, d HomeDeps) {
	d.Search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	a := homeApp{store: d.Store, hooks: d.Calendar, search: d.Search, style: d.Style}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", a.shareApps)
	mux.HandleFunc("GET /api/apps/model", serve.JSON(a.view))
	mux.HandleFunc("GET /api/apps/calendar", serve.JSON(a.calendar))
	mux.HandleFunc("POST /api/apps/calendar/default", serve.JSON(a.setDefault))
	mux.HandleFunc("POST /api/apps/link", serve.JSON(a.saveLink))
	mux.HandleFunc("GET /api/apps/audience/options", serve.JSON(a.audienceOptions))
	mux.HandleFunc("POST /api/apps/audience/preview", serve.JSON(a.audiencePreview))
	mux.HandleFunc("POST /api/apps/rsvp", serve.JSON(a.rsvp))
	mux.HandleFunc("DELETE /api/apps/link", serve.JSON(a.deleteLink))
	mux.HandleFunc("POST /api/apps/link/move", serve.JSON(a.moveLink))
	mux.HandleFunc("POST /api/apps/category", serve.JSON(a.saveCategory))
	mux.HandleFunc("DELETE /api/apps/category", serve.JSON(a.deleteCategory))
	mux.HandleFunc("POST /api/apps/categories/order", serve.JSON(a.reorderCategories))
	mux.HandleFunc("POST /api/apps/widgets/audience", serve.JSON(a.saveWidgetAudience))
	mux.HandleFunc("POST /api/apps/widgets/order", serve.JSON(a.setWidgetOrder))
	mux.HandleFunc("GET /api/apps/team", serve.JSON(a.teamWidget))
	mux.HandleFunc("GET /api/apps/celebrate", serve.JSON(a.celebrateWidget))
	mux.HandleFunc("GET /api/apps/school", serve.JSON(a.schoolWidget))
	a.search.Register(mux, "/api/apps", d.Images.Folder(), a.requireAdminFunc)
	RegisterAdmins(mux, a.store, "home", a.adminState)
	mux.HandleFunc("POST /api/admin/visibility", serve.JSON(a.setVisibility))
	mux.HandleFunc("POST /api/admin/visibility/order", serve.JSON(a.setAppOrder))
	a.discoverApps()
}

func (a homeApp) discoverApps() {
	actor := access.System("app discovery")
	ops, found := a.store.Model().Home.discover()
	for _, app := range found {
		slog.Info("home:found a new app, listed for nobody yet", "app", app.Key)
	}
	if err := a.store.Commit(context.Background(), actor, homeAppName, ops...); err != nil {
		slog.Error("home:discover apps", "error", err)
	}
}

func RegisterAppSwitch(mux *http.ServeMux, s *Store) {
	mux.HandleFunc("GET /api/apps/switch", serve.JSON(func(r *http.Request, _ serve.None) (switchView, error) {
		m := s.Model()
		return switchView{Apps: append([]App{homeAppWithMark()}, m.Home.AppList()...), Hidden: m.HiddenApps(auth.Email(r))}, nil
	}))
}

type switchView struct {
	Apps   []App    `json:"apps"`
	Hidden []string `json:"hidden"`
}

func homeAppWithMark() App {
	app := HomeApp
	app.Mark = markVersion(app.Key)
	return app
}

type appView struct {
	App
	ForMe      *bool          `json:"forMe,omitempty"`
	Visibility *AppVisibility `json:"visibility,omitempty"`
}

func appViews(m *Model, email string, admin bool) []appView {
	hidden := m.HiddenApps(email)
	out := []appView{}
	rows := map[string]AppVisibility{}
	if admin {
		for _, v := range m.Home.AppVisibilities() {
			rows[v.Key] = v
		}
	}
	for _, app := range m.Home.AppList() {
		mine := !slices.Contains(hidden, app.Key)
		if !mine && !admin {
			continue
		}
		view := appView{App: app}
		if admin {
			row := rows[app.Key]
			view.Visibility = &row
			if !mine {
				no := false
				view.ForMe = &no
			}
		}
		out = append(out, view)
	}
	return out
}

func (a homeApp) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, homeShell)
}

func (a homeApp) actor(r *http.Request) access.Actor {
	return a.store.Model().actor(r, "home")
}

func homeUpcoming(m *Model, email, token string) HomeUpcoming {
	out := HomeUpcoming{Events: m.Calendar.UpcomingUnder(m.Directory, email, m.LinkedEvents(email, now()), now(), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(m.Calendar, email, token)
	return out
}

func savedCalendars(calendar *Calendar, email, token string) (list []SavedCalendar, def, current string) {
	for _, f := range calendar.MyCalendars(email) {
		list = append(list, SavedCalendar{Token: f.Token, Name: f.Name, Emoji: f.Emoji, Locked: f.Locked})
	}
	def = list[0].Token
	current = def
	if slices.ContainsFunc(list, func(c SavedCalendar) bool { return c.Token == token }) {
		current = token
	}
	return list, def, current
}

func homeMonth(m *Model, email, month, token string) HomeMonth {
	shown := m.Calendar.MonthUnder(m.Directory, email, m.LinkedEvents(email, now()), now(), month, token)
	_, _, current := savedCalendars(m.Calendar, email, token)
	return HomeMonth{Month: shown.Month, Today: shown.Today, Days: shown.Days, Events: shown.Events, Calendar: current}
}

func (a homeApp) teamWidget(r *http.Request, _ serve.None) (ActivityWidget, error) {
	m := a.store.Model()
	return m.Activities.Widget(m.Directory.Resolve(auth.Email(r)), now()), nil
}

type celebrateWidgetView struct {
	Parties []EventCard `json:"parties"`
}

func (a homeApp) celebrateWidget(r *http.Request, _ serve.None) (celebrateWidgetView, error) {
	m := a.store.Model()
	email := m.Directory.Resolve(auth.Email(r))
	return celebrateWidgetView{m.Calendar.PartiesFor(m.Directory, email, m.LinkedEvents(email, now()), now())}, nil
}

type schoolWidgetView struct {
	Emails []SchoolEmail `json:"emails"`
}

func (a homeApp) schoolWidget(r *http.Request, _ serve.None) (schoolWidgetView, error) {
	m := a.store.Model()
	email := m.Directory.Resolve(auth.Email(r))
	since := now().AddDate(0, 0, -int(SchoolMailWindow/(24*time.Hour))).Format(DateFormat)
	return schoolWidgetView{m.Documents.SchoolMail(m.Directory, email, since)}, nil
}

func (a homeApp) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireHomeAdmin(a.actor(r)); err != nil {
			serve.Error(w, r, err)
			return
		}
		next(w, r)
	}
}

type homeUser struct {
	Email    string `json:"email"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

func (a homeApp) calendar(r *http.Request, _ serve.None) (HomeMonth, error) {
	m := a.store.Model()
	return homeMonth(m, m.actor(r, "home").Email, r.URL.Query().Get("month"), r.URL.Query().Get("calendar")), nil
}

type homeRSVPBody struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func (a homeApp) rsvp(r *http.Request, body homeRSVPBody) (serve.None, error) {
	return serve.None{}, a.hooks.Answer(r.Context(), a.actor(r).Email, body.ID, body.Answer)
}

type defaultBody struct {
	Token string `json:"token"`
}

func (a homeApp) setDefault(r *http.Request, body defaultBody) (serve.None, error) {
	return serve.None{}, a.hooks.MakeDefault(r.Context(), a.actor(r).Email, body.Token)
}

type homeView struct {
	Categories       []HomeCategory        `json:"categories"`
	User             homeUser              `json:"user"`
	ImageSearch      bool                  `json:"imageSearch"`
	Upcoming         []EventCard           `json:"upcoming"`
	UpcomingCalendar *HomeUpcoming         `json:"upcomingCalendar,omitempty"`
	Calendar         HomeMonth             `json:"calendar"`
	Apps             []appView             `json:"apps"`
	Options          *AudienceOptions      `json:"options,omitempty"`
	TagLabels        map[string]string     `json:"tagLabels,omitempty"`
	Widgets          map[string]widgetView `json:"widgets"`
	WidgetOrder      []string              `json:"widgetOrder"`
}

func (a homeApp) view(r *http.Request, _ serve.None) (homeView, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	email := actor.Email
	admin := actor.May(ConfigureHome)
	hidden := hiddenHosts(r.Host, m.HiddenApps(email))
	forMe := func(rules []Rule) bool {
		return len(rules) == 0 || m.homeIncludes(rules, email)
	}
	categories := m.HomeCategoriesFor(actor)
	for i := range categories {
		categories[i].Links = slices.DeleteFunc(categories[i].Links, func(l HomeLink) bool { return linksInto(hidden, l.URL) })
	}
	view := homeView{
		Categories:  categories,
		User:        homeUser{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: m.Directory.HeroPhoto(email), IsAdmin: admin},
		ImageSearch: a.search.On(),
		Calendar:    homeMonth(m, email, "", ""),
		Apps:        appViews(m, email, admin),
		Widgets:     map[string]widgetView{},
		WidgetOrder: m.Home.WidgetOrder,
	}
	for _, key := range HomeWidgets {
		rules := m.Home.WidgetRules[key]
		v := widgetView{ForMe: forMe(rules)}
		if admin {
			v.Rules = rules
		}
		view.Widgets[key] = v
	}
	if admin {
		options := m.Audience(now()).Options(actor.Email)
		view.Options = &options
		view.TagLabels = homeTagLabels(m, categories, view.Apps, actor.Email)
	}
	ahead := homeUpcoming(m, email, "")
	view.Upcoming = ahead.Events
	if ahead.Calendar != "" {
		view.UpcomingCalendar = &HomeUpcoming{Calendar: ahead.Calendar, Default: ahead.Default, Calendars: ahead.Calendars}
	}
	return view, nil
}

func homeTagLabels(m *Model, categories []HomeCategory, apps []appView, viewer string) map[string]string {
	sources := m.Audience(now())
	admins := m.AdminList("home").Admins()
	out := map[string]string{}
	add := func(rules []Rule) {
		for _, r := range rules {
			for i, label := range sources.TagLabels(r, admins, viewer) {
				out[r.Tags[i]] = label
			}
		}
	}
	for _, category := range categories {
		add(category.Rules)
		for _, link := range category.Links {
			add(link.Rules)
		}
	}
	for _, app := range apps {
		if app.Visibility != nil {
			add(app.Visibility.Rules)
		}
	}
	for _, rules := range m.Home.WidgetRules {
		add(rules)
	}
	return out
}

func hiddenHosts(pageHost string, apps []string) map[string]bool {
	if len(apps) == 0 {
		return nil
	}
	tier := tierOf(pageHost)
	hidden := map[string]bool{}
	for _, key := range apps {
		app, _ := appByKey(key)
		for _, label := range app.Hosts {
			hidden[Qualify(label, tier)] = true
		}
	}
	return hidden
}

func linksInto(hosts map[string]bool, link string) bool {
	if len(hosts) == 0 {
		return false
	}
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	return hosts[strings.ToLower(u.Host)]
}

func rulesOf(m *Home, key string) []Rule {
	for _, c := range m.Categories {
		if thingCategory+c.ID == key {
			return c.Rules
		}
		for _, l := range c.Links {
			if thingLink+l.ID == key {
				return l.Rules
			}
		}
	}
	if name, ok := strings.CutPrefix(key, thingWidget); ok {
		return m.WidgetRules[name]
	}
	return m.Visibility[strings.TrimPrefix(key, thingApp)].Rules
}

type widgetView struct {
	ForMe bool   `json:"forMe"`
	Rules []Rule `json:"rules,omitempty"`
}

type widgetAudienceBody struct {
	Widget string `json:"widget"`
	Rules  []Rule `json:"rules"`
}

func (a homeApp) commit(r *http.Request, actor access.Actor, ops []store.Op) error {
	return a.store.Commit(r.Context(), actor, homeAppName, ops...)
}

func (a homeApp) saveWidgetAudience(r *http.Request, body widgetAudienceBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.saveHomeWidgetAudience(actor, body.Widget, body.Rules)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home: set a widget's audience", "actor", actor.Email, "widget", body.Widget, "rules", len(body.Rules))
	return serve.None{}, nil
}

type widgetOrderBody struct {
	Widgets []string `json:"widgets"`
}

func (a homeApp) setWidgetOrder(r *http.Request, body widgetOrderBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.setWidgetOrder(actor, body.Widgets)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home: set the widgets' order", "actor", actor.Email, "widgets", body.Widgets)
	return serve.None{}, nil
}

func (a homeApp) audienceOptions(r *http.Request, _ serve.None) (AudienceOptions, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	if err := requireHomeAdmin(actor); err != nil {
		return AudienceOptions{}, err
	}
	return m.Audience(now()).Options(actor.Email), nil
}

type previewBody struct {
	Thing string `json:"thing"`
	Rules []Rule `json:"rules"`
}

type previewView struct {
	Count      int      `json:"count"`
	Names      []string `json:"names"`
	RuleCounts []int    `json:"ruleCounts"`
}

func (a homeApp) audiencePreview(r *http.Request, body previewBody) (previewView, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	if err := requireHomeAdmin(actor); err != nil {
		return previewView{}, err
	}
	rules, err := m.checkHomeRules(rulesOf(m.Home, body.Thing), body.Rules, actor.Email)
	if err != nil {
		return previewView{}, err
	}
	sources := m.Audience(now())
	list := Audience{Rules: rules, Editors: m.AdminList("home").Admins()}
	members := list.Members(sources)
	names := []string{}
	for _, member := range members {
		names = append(names, sources.Directory.DisplayName(member))
	}
	slices.Sort(names)
	return previewView{Count: len(members), Names: names[:min(len(names), 12)], RuleCounts: list.RuleCounts(sources)}, nil
}

func (a homeApp) saveLink(r *http.Request, body linkEdit) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	action, key, ops, err := m.saveHomeLink(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:saved link", "action", action, "id", key, "title", body.Title)
	return serve.None{}, nil
}

type idBody struct {
	ID string `json:"id"`
}

func (a homeApp) deleteLink(r *http.Request, body idBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.deleteLink(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:deleted link", "id", body.ID)
	return serve.None{}, nil
}

func (a homeApp) saveCategory(r *http.Request, body categoryEdit) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	action, key, ops, err := m.saveHomeCategory(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:saved category", "action", action, "id", key, "title", body.Title)
	return serve.None{}, nil
}

type idsBody struct {
	IDs []string `json:"ids"`
}

func (a homeApp) reorderCategories(r *http.Request, body idsBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.reorderCategories(actor, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:reordered categories", "count", len(body.IDs))
	return serve.None{}, nil
}

type moveLinkBody struct {
	ID string `json:"id"`
	By int    `json:"by"`
}

func (a homeApp) moveLink(r *http.Request, body moveLinkBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.moveLink(actor, body.ID, body.By)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:moved link", "id", body.ID, "by", body.By)
	return serve.None{}, nil
}

func (a homeApp) deleteCategory(r *http.Request, body idBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.deleteCategory(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:deleted category", "id", body.ID)
	return serve.None{}, nil
}

func (a homeApp) adminState(m *Model, _ *http.Request, _ access.Actor) map[string]any {
	return map[string]any{"apps": m.Home.AppVisibilities()}
}

func (a homeApp) setVisibility(r *http.Request, body visibilityEdit) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	key, v, ops, err := m.setHomeVisibility(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:set an app's visibility", "app", key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
	return serve.None{}, nil
}

type appsBody struct {
	Apps []string `json:"apps"`
}

func (a homeApp) setAppOrder(r *http.Request, body appsBody) (serve.None, error) {
	m := a.store.Model()
	actor := m.actor(r, "home")
	ops, err := m.Home.setAppOrder(actor, body.Apps)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.commit(r, actor, ops); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:set the apps' order", "apps", body.Apps)
	return serve.None{}, nil
}
