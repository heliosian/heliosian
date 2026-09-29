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
)

const homeShell = "web/home/index.html"

type homeApp struct {
	cache     *HomeCache
	hooks     CalendarHooks
	documents *DocumentsCache
	search    imagesearch.Search
	style     *sharecard.Style
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
	Cache     *HomeCache
	Images    blob.Images
	Calendar  CalendarHooks
	Documents *DocumentsCache
	Search    imagesearch.Search
	Style     *sharecard.Style
}

func RegisterHome(mux *http.ServeMux, d HomeDeps) {
	d.Search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	a := homeApp{cache: d.Cache, hooks: d.Calendar, documents: d.Documents, search: d.Search, style: d.Style}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", a.shareApps)
	mux.HandleFunc("GET /api/apps/model", serve.JSON(a.view))
	mux.HandleFunc("GET /api/apps/calendar", serve.JSON(a.calendar))
	mux.HandleFunc("GET /api/apps/upcoming", serve.JSON(a.upcomingUnder))
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
	RegisterAdmins(mux, a.cache.AdminList, a.actor, a.adminState)
	mux.HandleFunc("POST /api/admin/visibility", serve.JSON(a.setVisibility))
	mux.HandleFunc("POST /api/admin/visibility/order", serve.JSON(a.setAppOrder))
	a.discoverApps()
}

func (a homeApp) discoverApps() {
	actor := access.System("app discovery")
	ops, found := a.cache.discover(actor)
	for _, app := range found {
		slog.Info("home:found a new app, listed for nobody yet", "app", app.Key)
	}
	if err := a.cache.Commit(context.Background(), actor, ops...); err != nil {
		slog.Error("home:discover apps", "error", err)
	}
}

func RegisterAppSwitch(mux *http.ServeMux, cache *HomeCache) {
	mux.HandleFunc("GET /api/apps/switch", serve.JSON(func(r *http.Request, _ serve.None) (switchView, error) {
		return switchView{Apps: append([]App{homeAppWithMark()}, cache.AppList()...), Hidden: cache.HiddenApps(auth.Email(r))}, nil
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

func (a homeApp) appViews(email string, admin bool) []appView {
	hidden := a.cache.HiddenApps(email)
	out := []appView{}
	rows := map[string]AppVisibility{}
	if admin {
		for _, v := range a.cache.AppVisibilities() {
			rows[v.Key] = v
		}
	}
	for _, app := range a.cache.AppList() {
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

func (a homeApp) sources() AudienceSources {
	return a.cache.sources()
}

func (a homeApp) actor(r *http.Request) access.Actor {
	return a.cache.directory.Model().Actor(r, a.cache.Held)
}

func (a homeApp) upcoming(email, token string) HomeUpcoming {
	calendar := a.hooks.cache.Model()
	out := HomeUpcoming{Events: calendar.UpcomingUnder(a.cache.directory.Model(), email, a.cache.linked(email), now(), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(calendar, email, token)
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

func (a homeApp) month(email, month, token string) HomeMonth {
	calendar := a.hooks.cache.Model()
	m := calendar.MonthUnder(a.cache.directory.Model(), email, a.cache.linked(email), now(), month, token)
	_, _, current := savedCalendars(calendar, email, token)
	return HomeMonth{Month: m.Month, Today: m.Today, Days: m.Days, Events: m.Events, Calendar: current}
}

func (a homeApp) teamWidget(r *http.Request, _ serve.None) (ActivityWidget, error) {
	email := a.cache.directory.Model().Resolve(auth.Email(r))
	return a.cache.activities.Widget(email, now()), nil
}

type celebrateWidgetView struct {
	Parties []EventCard `json:"parties"`
}

func (a homeApp) celebrateWidget(r *http.Request, _ serve.None) (celebrateWidgetView, error) {
	people := a.cache.directory.Model()
	email := people.Resolve(auth.Email(r))
	return celebrateWidgetView{a.hooks.cache.Model().PartiesFor(people, email, a.cache.linked(email), now())}, nil
}

type schoolWidgetView struct {
	Emails []SchoolEmail `json:"emails"`
}

func (a homeApp) schoolWidget(r *http.Request, _ serve.None) (schoolWidgetView, error) {
	people := a.cache.directory.Model()
	email := people.Resolve(auth.Email(r))
	since := now().AddDate(0, 0, -int(SchoolMailWindow/(24*time.Hour))).Format(DateFormat)
	return schoolWidgetView{a.documents.Model().SchoolMail(people, email, since)}, nil
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
	return a.month(a.actor(r).Email, r.URL.Query().Get("month"), r.URL.Query().Get("calendar")), nil
}

type homeRSVPBody struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func (a homeApp) rsvp(r *http.Request, body homeRSVPBody) (serve.None, error) {
	return serve.None{}, a.hooks.Answer(r.Context(), a.actor(r).Email, body.ID, body.Answer)
}

func (a homeApp) upcomingUnder(r *http.Request, _ serve.None) (HomeUpcoming, error) {
	return a.upcoming(a.actor(r).Email, r.URL.Query().Get("calendar")), nil
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
	actor := a.actor(r)
	email := actor.Email
	admin := actor.May(ConfigureHome)
	hidden := hiddenHosts(r.Host, a.cache.HiddenApps(email))
	full := a.cache.Model()
	forMe := func(rules []Rule) bool {
		return len(rules) == 0 || a.cache.includes(rules, email)
	}
	categories := a.cache.CategoriesFor(actor)
	for i := range categories {
		categories[i].Links = slices.DeleteFunc(categories[i].Links, func(l HomeLink) bool { return linksInto(hidden, l.URL) })
	}
	view := homeView{
		Categories:  categories,
		User:        homeUser{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.sources().Directory.HeroPhoto(email), IsAdmin: admin},
		ImageSearch: a.search.On(),
		Calendar:    a.month(email, "", ""),
		Apps:        a.appViews(email, admin),
		Widgets:     map[string]widgetView{},
		WidgetOrder: full.WidgetOrder,
	}
	for _, key := range HomeWidgets {
		rules := full.WidgetRules[key]
		v := widgetView{ForMe: forMe(rules)}
		if admin {
			v.Rules = rules
		}
		view.Widgets[key] = v
	}
	if admin {
		options := a.sources().Options(actor.Email)
		view.Options = &options
		view.TagLabels = a.tagLabels(categories, view.Apps, actor.Email)
	}
	ahead := a.upcoming(email, "")
	view.Upcoming = ahead.Events
	if ahead.Calendar != "" {
		view.UpcomingCalendar = &HomeUpcoming{Calendar: ahead.Calendar, Default: ahead.Default, Calendars: ahead.Calendars}
	}
	return view, nil
}

func (a homeApp) tagLabels(categories []HomeCategory, apps []appView, viewer string) map[string]string {
	sources := a.sources()
	admins := a.cache.Admins()
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
	for _, rules := range a.cache.Model().WidgetRules {
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

func (a homeApp) saveWidgetAudience(r *http.Request, body widgetAudienceBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.saveWidgetAudience(actor, body.Widget, body.Rules)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home: set a widget's audience", "actor", actor.Email, "widget", body.Widget, "rules", len(body.Rules))
	return serve.None{}, nil
}

type widgetOrderBody struct {
	Widgets []string `json:"widgets"`
}

func (a homeApp) setWidgetOrder(r *http.Request, body widgetOrderBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.setWidgetOrder(actor, body.Widgets)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home: set the widgets' order", "actor", actor.Email, "widgets", body.Widgets)
	return serve.None{}, nil
}

func (a homeApp) audienceOptions(r *http.Request, _ serve.None) (AudienceOptions, error) {
	actor := a.actor(r)
	if err := requireHomeAdmin(actor); err != nil {
		return AudienceOptions{}, err
	}
	return a.sources().Options(actor.Email), nil
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
	actor := a.actor(r)
	if err := requireHomeAdmin(actor); err != nil {
		return previewView{}, err
	}
	rules, err := a.cache.checkRules(rulesOf(a.cache.Model(), body.Thing), body.Rules, actor.Email)
	if err != nil {
		return previewView{}, err
	}
	sources := a.sources()
	list := Audience{Rules: rules, Editors: a.cache.Admins()}
	members := list.Members(sources)
	names := []string{}
	for _, m := range members {
		names = append(names, sources.Directory.DisplayName(m))
	}
	slices.Sort(names)
	return previewView{Count: len(members), Names: names[:min(len(names), 12)], RuleCounts: list.RuleCounts(sources)}, nil
}

func (a homeApp) saveLink(r *http.Request, body linkEdit) (serve.None, error) {
	actor := a.actor(r)
	action, key, ops, err := a.cache.saveLink(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:saved link", "action", action, "id", key, "title", body.Title)
	return serve.None{}, nil
}

type idBody struct {
	ID string `json:"id"`
}

func (a homeApp) deleteLink(r *http.Request, body idBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.deleteLink(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:deleted link", "id", body.ID)
	return serve.None{}, nil
}

func (a homeApp) saveCategory(r *http.Request, body categoryEdit) (serve.None, error) {
	actor := a.actor(r)
	action, key, ops, err := a.cache.saveCategory(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:saved category", "action", action, "id", key, "title", body.Title)
	return serve.None{}, nil
}

type idsBody struct {
	IDs []string `json:"ids"`
}

func (a homeApp) reorderCategories(r *http.Request, body idsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.reorderCategories(actor, body.IDs)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
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
	actor := a.actor(r)
	ops, err := a.cache.moveLink(actor, body.ID, body.By)
	if err != nil {
		return serve.None{}, err
	}
	if len(ops) == 0 {
		return serve.None{}, nil
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:moved link", "id", body.ID, "by", body.By)
	return serve.None{}, nil
}

func (a homeApp) deleteCategory(r *http.Request, body idBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.deleteCategory(actor, body.ID)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:deleted category", "id", body.ID)
	return serve.None{}, nil
}

func (a homeApp) adminState(*http.Request, access.Actor) map[string]any {
	return map[string]any{"apps": a.cache.AppVisibilities()}
}

func (a homeApp) setVisibility(r *http.Request, body visibilityEdit) (serve.None, error) {
	actor := a.actor(r)
	key, v, ops, err := a.cache.setVisibility(actor, body)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:set an app's visibility", "app", key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
	return serve.None{}, nil
}

type appsBody struct {
	Apps []string `json:"apps"`
}

func (a homeApp) setAppOrder(r *http.Request, body appsBody) (serve.None, error) {
	actor := a.actor(r)
	ops, err := a.cache.setAppOrder(actor, body.Apps)
	if err != nil {
		return serve.None{}, err
	}
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		return serve.None{}, err
	}
	slog.InfoContext(r.Context(), "home:set the apps' order", "apps", body.Apps)
	return serve.None{}, nil
}
