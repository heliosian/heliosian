package home

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/imagesearch"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
)

type app struct {
	cache       *Cache
	sources     func() model.AudienceSources
	upcoming    func(email, token string) Upcoming
	makeDefault func(ctx context.Context, email, token string) error
	month       func(email, month, token string) Month
	search      imagesearch.Search
	answer      func(ctx context.Context, email, id, answer string) error
	style       *sharecard.Style
}

type Upcoming struct {
	Events    []model.EventCard `json:"events"`
	Calendar  string            `json:"calendar,omitempty"`
	Default   string            `json:"default,omitempty"`
	Calendars []SavedCalendar   `json:"calendars,omitempty"`
}

type SavedCalendar struct {
	Token  string `json:"token"`
	Name   string `json:"name"`
	Emoji  string `json:"emoji,omitempty"`
	Locked bool   `json:"locked,omitempty"`
}

type Month struct {
	Month    string                       `json:"month"`
	Today    string                       `json:"today"`
	Days     map[string]model.CalendarDay `json:"days"`
	Events   []model.EventCard            `json:"events"`
	Calendar string                       `json:"calendar,omitempty"`
}

type Deps struct {
	Cache       *Cache
	Images      blob.Images
	Upcoming    func(email, token string) Upcoming
	Month       func(email, month, token string) Month
	Search      imagesearch.Search
	Answer      func(ctx context.Context, email, id, answer string) error
	MakeDefault func(ctx context.Context, email, token string) error
	Style       *sharecard.Style
}

func Register(mux *http.ServeMux, d Deps) {
	d.Search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	a := app{cache: d.Cache, sources: d.Cache.sources, upcoming: d.Upcoming, month: d.Month, search: d.Search, answer: d.Answer, makeDefault: d.MakeDefault, style: d.Style}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", a.shareApps)
	mux.HandleFunc("GET /api/apps/model", serve.JSON(a.model))
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
	a.search.Register(mux, "/api/apps", d.Images.Folder(), a.requireAdminFunc)
	model.RegisterAdmins(mux, a.cache.AdminList, a.actor, a.adminState)
	mux.HandleFunc("POST /api/admin/visibility", serve.JSON(a.setVisibility))
	mux.HandleFunc("POST /api/admin/visibility/order", serve.JSON(a.setAppOrder))
	a.discoverApps()
}

func (a app) discoverApps() {
	actor := access.System("app discovery")
	ops, found := a.cache.discover(actor)
	for _, app := range found {
		slog.Info("home:found a new app, listed for nobody yet", "app", app.Key)
	}
	if err := a.cache.Commit(context.Background(), actor, ops...); err != nil {
		slog.Error("home:discover apps", "error", err)
	}
}

func RegisterSwitch(mux *http.ServeMux, cache *Cache) {
	mux.HandleFunc("GET /api/apps/switch", serve.JSON(func(r *http.Request, _ serve.None) (switchView, error) {
		return switchView{Apps: append([]App{homeApp()}, cache.AppList()...), Hidden: cache.HiddenApps(auth.Email(r))}, nil
	}))
}

type switchView struct {
	Apps   []App    `json:"apps"`
	Hidden []string `json:"hidden"`
}

func homeApp() App {
	app := Home
	app.Mark = markVersion(app.Key)
	return app
}

type appView struct {
	App
	ForMe      *bool          `json:"forMe,omitempty"`
	Visibility *AppVisibility `json:"visibility,omitempty"`
}

func (a app) appViews(email string, admin bool) []appView {
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

func (a app) page(w http.ResponseWriter, r *http.Request) {
	serve.File(w, r, "web/home/index.html")
}

func (a app) actor(r *http.Request) access.Actor {
	return a.sources().Directory.Actor(r, a.cache.Held)
}

func (a app) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := requireAdmin(a.actor(r)); err != nil {
			serve.Error(w, r, err)
			return
		}
		next(w, r)
	}
}

type user struct {
	Email    string `json:"email"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

func (a app) calendar(r *http.Request, _ serve.None) (Month, error) {
	return a.month(a.actor(r).Email, r.URL.Query().Get("month"), r.URL.Query().Get("calendar")), nil
}

type rsvpBody struct {
	ID     string `json:"id"`
	Answer string `json:"answer"`
}

func (a app) rsvp(r *http.Request, body rsvpBody) (serve.None, error) {
	return serve.None{}, a.answer(r.Context(), a.actor(r).Email, body.ID, body.Answer)
}

func (a app) upcomingUnder(r *http.Request, _ serve.None) (Upcoming, error) {
	return a.upcoming(a.actor(r).Email, r.URL.Query().Get("calendar")), nil
}

type defaultBody struct {
	Token string `json:"token"`
}

func (a app) setDefault(r *http.Request, body defaultBody) (serve.None, error) {
	return serve.None{}, a.makeDefault(r.Context(), a.actor(r).Email, body.Token)
}

type modelView struct {
	Categories       []Category             `json:"categories"`
	User             user                   `json:"user"`
	ImageSearch      bool                   `json:"imageSearch"`
	Upcoming         []model.EventCard      `json:"upcoming"`
	UpcomingCalendar *Upcoming              `json:"upcomingCalendar,omitempty"`
	Calendar         Month                  `json:"calendar"`
	Apps             []appView              `json:"apps"`
	Options          *model.AudienceOptions `json:"options,omitempty"`
	TagLabels        map[string]string      `json:"tagLabels,omitempty"`
	Widgets          map[string]widgetView  `json:"widgets"`
	WidgetOrder      []string               `json:"widgetOrder"`
}

func (a app) model(r *http.Request, _ serve.None) (modelView, error) {
	actor := a.actor(r)
	email := actor.Email
	admin := actor.May(Configure)
	hidden := hiddenHosts(r.Host, a.cache.HiddenApps(email))
	full := a.cache.Model()
	forMe := func(rules []model.Rule) bool {
		return len(rules) == 0 || a.cache.includes(rules, email)
	}
	categories := a.cache.CategoriesFor(actor)
	for i := range categories {
		categories[i].Links = slices.DeleteFunc(categories[i].Links, func(l Link) bool { return linksInto(hidden, l.URL) })
	}
	view := modelView{
		Categories:  categories,
		User:        user{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.sources().Directory.HeroPhoto(email), IsAdmin: admin},
		ImageSearch: a.search.On(),
		Calendar:    a.month(email, "", ""),
		Apps:        a.appViews(email, admin),
		Widgets:     map[string]widgetView{},
		WidgetOrder: full.WidgetOrder,
	}
	for _, key := range Widgets {
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
		view.UpcomingCalendar = &Upcoming{Calendar: ahead.Calendar, Default: ahead.Default, Calendars: ahead.Calendars}
	}
	return view, nil
}

func (a app) tagLabels(categories []Category, apps []appView, viewer string) map[string]string {
	sources := a.sources()
	admins := a.cache.Admins()
	out := map[string]string{}
	add := func(rules []model.Rule) {
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

func rulesOf(model *Model, key string) []model.Rule {
	for _, c := range model.Categories {
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
		return model.WidgetRules[name]
	}
	return model.Visibility[strings.TrimPrefix(key, thingApp)].Rules
}

type widgetView struct {
	ForMe bool         `json:"forMe"`
	Rules []model.Rule `json:"rules,omitempty"`
}

type widgetAudienceBody struct {
	Widget string       `json:"widget"`
	Rules  []model.Rule `json:"rules"`
}

func (a app) saveWidgetAudience(r *http.Request, body widgetAudienceBody) (serve.None, error) {
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

func (a app) setWidgetOrder(r *http.Request, body widgetOrderBody) (serve.None, error) {
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

func (a app) audienceOptions(r *http.Request, _ serve.None) (model.AudienceOptions, error) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		return model.AudienceOptions{}, err
	}
	return a.sources().Options(actor.Email), nil
}

type previewBody struct {
	Thing string       `json:"thing"`
	Rules []model.Rule `json:"rules"`
}

type previewView struct {
	Count      int      `json:"count"`
	Names      []string `json:"names"`
	RuleCounts []int    `json:"ruleCounts"`
}

func (a app) audiencePreview(r *http.Request, body previewBody) (previewView, error) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		return previewView{}, err
	}
	rules, err := a.cache.checkRules(rulesOf(a.cache.Model(), body.Thing), body.Rules, actor.Email)
	if err != nil {
		return previewView{}, err
	}
	sources := a.sources()
	list := model.Audience{Rules: rules, Editors: a.cache.Admins()}
	members := list.Members(sources)
	names := []string{}
	for _, m := range members {
		names = append(names, sources.Directory.DisplayName(m))
	}
	slices.Sort(names)
	return previewView{Count: len(members), Names: names[:min(len(names), 12)], RuleCounts: list.RuleCounts(sources)}, nil
}

func (a app) saveLink(r *http.Request, body linkEdit) (serve.None, error) {
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

func (a app) deleteLink(r *http.Request, body idBody) (serve.None, error) {
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

func (a app) saveCategory(r *http.Request, body categoryEdit) (serve.None, error) {
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

func (a app) reorderCategories(r *http.Request, body idsBody) (serve.None, error) {
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

type moveBody struct {
	ID string `json:"id"`
	By int    `json:"by"`
}

func (a app) moveLink(r *http.Request, body moveBody) (serve.None, error) {
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

func (a app) deleteCategory(r *http.Request, body idBody) (serve.None, error) {
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

func (a app) adminState(*http.Request, access.Actor) map[string]any {
	return map[string]any{"apps": a.cache.AppVisibilities()}
}

func (a app) setVisibility(r *http.Request, body visibilityEdit) (serve.None, error) {
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

func (a app) setAppOrder(r *http.Request, body appsBody) (serve.None, error) {
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
