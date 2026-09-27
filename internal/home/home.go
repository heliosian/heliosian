package home

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"heliosian/internal/access"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/filter"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

const imageFolder = "link-images"

type Directory interface {
	Sources() filter.Sources
	Resolve(email string) string
}

type app struct {
	cache       *Cache
	store       *blob.Store
	superAdmins func() []string
	heroPhoto   func(string) string
	people      func() []Person
	directory   Directory
	alerts      func(string) ([]string, []string)
	upcoming    func(email, token string) Upcoming
	makeDefault func(ctx context.Context, email, token string) error
	month       func(email, month, token string) Month
	search      imagesearch.Search
	answer      func(ctx context.Context, email, id, answer string) error
	style       *sharecard.Style
}

type Upcoming struct {
	Events    []when.Card     `json:"events"`
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

type Month struct {
	Month    string              `json:"month"`
	Today    string              `json:"today"`
	Days     map[string]when.Day `json:"days"`
	Events   []when.Card         `json:"events"`
	Calendar string              `json:"calendar,omitempty"`
}

type alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, superAdmins func() []string, heroPhoto func(string) string, people func() []Person, alerts func(string) ([]string, []string), upcoming func(email, token string) Upcoming, month func(email, month, token string) Month, search imagesearch.Search, answer func(ctx context.Context, email, id, answer string) error, makeDefault func(ctx context.Context, email, token string) error, style *sharecard.Style) {
	if search.UserAgent == "" {
		search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	}
	a := app{cache: cache, store: media, superAdmins: superAdmins, heroPhoto: heroPhoto, people: people, directory: cache.directory, alerts: alerts, upcoming: upcoming, month: month, search: search, answer: answer, makeDefault: makeDefault, style: style}
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.page)
	mux.HandleFunc("GET /dl/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /open/share/apps.png", a.shareApps)
	mux.HandleFunc("GET /api/apps/model", a.model)
	mux.HandleFunc("GET /api/apps/calendar", a.calendar)
	mux.HandleFunc("GET /api/apps/upcoming", a.upcomingUnder)
	mux.HandleFunc("POST /api/apps/calendar/default", a.setDefault)
	mux.HandleFunc("POST /api/apps/link", a.saveLink)
	mux.HandleFunc("GET /api/apps/audience/options", a.audienceOptions)
	mux.HandleFunc("POST /api/apps/audience/preview", a.audiencePreview)
	mux.HandleFunc("POST /api/apps/rsvp", a.rsvp)
	mux.HandleFunc("DELETE /api/apps/link", a.deleteLink)
	mux.HandleFunc("POST /api/apps/link/move", a.moveLink)
	mux.HandleFunc("POST /api/apps/category", a.saveCategory)
	mux.HandleFunc("DELETE /api/apps/category", a.deleteCategory)
	mux.HandleFunc("POST /api/apps/categories/order", a.reorderCategories)
	mux.HandleFunc("POST /api/apps/widgets/audience", a.saveWidgetAudience)
	mux.HandleFunc("POST /api/apps/widgets/order", a.setWidgetOrder)
	a.search.Register(mux, "/api/apps", imageFolder, a.requireAdminFunc)
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/visibility", a.setVisibility)
	mux.HandleFunc("POST /api/admin/visibility/order", a.setAppOrder)
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
	mux.HandleFunc("GET /api/apps/switch", func(w http.ResponseWriter, r *http.Request) {
		view := struct {
			Apps   []App    `json:"apps"`
			Hidden []string `json:"hidden"`
		}{Apps: append([]App{homeApp()}, cache.AppList()...), Hidden: cache.HiddenApps(auth.Email(r))}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(view); err != nil {
			slog.ErrorContext(r.Context(), "encode app switch", "error", err)
		}
	})
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
	email := a.directory.Resolve(strings.ToLower(auth.Email(r)))
	return access.Actor{Email: email, Admin: a.cache.IsAdmin(email)}
}

func (a app) admin(w http.ResponseWriter, r *http.Request) (access.Actor, bool) {
	actor := a.actor(r)
	if err := requireAdmin(actor); err != nil {
		http.Error(w, err.Error(), access.Status(err))
		return access.Actor{}, false
	}
	return actor, true
}

func (a app) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.admin(w, r); ok {
			next(w, r)
		}
	}
}

type user struct {
	Email    string `json:"email"`
	Initial  string `json:"initial"`
	PhotoURL string `json:"photoUrl,omitempty"`
	IsAdmin  bool   `json:"isAdmin"`
}

func (a app) calendar(w http.ResponseWriter, r *http.Request) {
	email := a.actor(r).Email
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.month(email, r.URL.Query().Get("month"), r.URL.Query().Get("calendar"))); err != nil {
		slog.ErrorContext(r.Context(), "encode apps calendar", "error", err)
	}
}

func (a app) rsvp(w http.ResponseWriter, r *http.Request) {
	email := a.actor(r).Email
	var body struct {
		ID     string `json:"id"`
		Answer string `json:"answer"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if a.answer == nil {
		http.Error(w, "the calendar is not set up", http.StatusBadRequest)
		return
	}
	if err := a.answer(r.Context(), email, body.ID, body.Answer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) upcomingUnder(w http.ResponseWriter, r *http.Request) {
	email := a.actor(r).Email
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.upcoming(email, r.URL.Query().Get("calendar"))); err != nil {
		slog.ErrorContext(r.Context(), "encode upcoming", "error", err)
	}
}

func (a app) setDefault(w http.ResponseWriter, r *http.Request) {
	email := a.actor(r).Email
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	if a.makeDefault == nil {
		http.Error(w, "the calendar is not set up", http.StatusBadRequest)
		return
	}
	if err := a.makeDefault(r.Context(), email, body.Token); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a app) model(w http.ResponseWriter, r *http.Request) {
	actor := a.actor(r)
	email := actor.Email
	admin := actor.Admin
	hidden := hiddenHosts(r.Host, a.cache.HiddenApps(email))
	full := a.cache.Model()
	forMe := func(rules []filter.Rule) bool {
		return len(rules) == 0 || a.cache.includes(rules, email)
	}
	categories := a.cache.CategoriesFor(actor)
	for i := range categories {
		categories[i].Links = slices.DeleteFunc(categories[i].Links, func(l Link) bool { return linksInto(hidden, l.URL) })
	}
	view := struct {
		Categories       []Category            `json:"categories"`
		User             user                  `json:"user"`
		ImageSearch      bool                  `json:"imageSearch"`
		Alerts           alerts                `json:"alerts"`
		Upcoming         []when.Card           `json:"upcoming"`
		UpcomingCalendar *Upcoming             `json:"upcomingCalendar,omitempty"`
		Calendar         Month                 `json:"calendar"`
		Apps             []appView             `json:"apps"`
		Options          *filter.Options       `json:"options,omitempty"`
		TagLabels        map[string]string     `json:"tagLabels,omitempty"`
		Widgets          map[string]widgetView `json:"widgets"`
		WidgetOrder      []string              `json:"widgetOrder"`
	}{
		Categories:  categories,
		User:        user{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.heroPhoto(email), IsAdmin: admin},
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
		options := filter.OptionsFor(a.directory.Sources(), actor.Email)
		view.Options = &options
		view.TagLabels = a.tagLabels(categories, view.Apps, actor.Email)
	}
	ahead := a.upcoming(email, "")
	view.Upcoming = ahead.Events
	if ahead.Calendar != "" {
		view.UpcomingCalendar = &Upcoming{Calendar: ahead.Calendar, Default: ahead.Default, Calendars: ahead.Calendars}
	}
	view.Alerts.Stale, view.Alerts.Privacy = a.alerts(email)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps model", "error", err)
	}
}

func (a app) tagLabels(categories []Category, apps []appView, viewer string) map[string]string {
	sources := a.directory.Sources()
	admins := a.cache.Admins()
	out := map[string]string{}
	add := func(rules []filter.Rule) {
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

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(into); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return false
	}
	return true
}

func visibleCell(visible bool) string {
	if visible {
		return "Yes"
	}
	return "No"
}

func (a app) commit(w http.ResponseWriter, r *http.Request, actor access.Actor, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func refuse(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), access.Status(err))
}

func rulesOf(model *Model, key string) []filter.Rule {
	for _, c := range model.Categories {
		if thingCategory+c.Title == key {
			return c.Rules
		}
		for _, l := range c.Links {
			if thingLink+l.Title == key {
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
	ForMe bool          `json:"forMe"`
	Rules []filter.Rule `json:"rules,omitempty"`
}

func (a app) saveWidgetAudience(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Widget string        `json:"widget"`
		Rules  []filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.saveWidgetAudience(actor, body.Widget, body.Rules)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) > 0 && !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home: set a widget's audience", "actor", actor.Email, "widget", body.Widget, "rules", len(body.Rules))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setWidgetOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Widgets []string `json:"widgets"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.setWidgetOrder(actor, body.Widgets)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) > 0 && !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home: set the widgets' order", "actor", actor.Email, "widgets", body.Widgets)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) audienceOptions(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.admin(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filter.OptionsFor(a.directory.Sources(), actor.Email)); err != nil {
		slog.ErrorContext(r.Context(), "encode audience options", "error", err)
	}
}

func (a app) audiencePreview(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.admin(w, r)
	if !ok {
		return
	}
	var body struct {
		Thing string        `json:"thing"`
		Rules []filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	rules, err := a.cache.checkRules(rulesOf(a.cache.Model(), body.Thing), body.Rules, actor.Email)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sources := a.directory.Sources()
	list := filter.List{Rules: rules, Editors: a.cache.Admins()}
	members := filter.Members(list, sources)
	names := []string{}
	for _, m := range members {
		names = append(names, sources.Directory.DisplayName(m))
	}
	slices.Sort(names)
	view := struct {
		Count      int      `json:"count"`
		Names      []string `json:"names"`
		RuleCounts []int    `json:"ruleCounts"`
	}{Count: len(members), Names: names[:min(len(names), 12)], RuleCounts: filter.RuleCounts(list, sources)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode audience preview", "error", err)
	}
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	var body linkEdit
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	action, title, ops, err := a.cache.saveLink(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:saved link", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.deleteLink(actor, body.Title)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:deleted link", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	var body categoryEdit
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	action, title, ops, err := a.cache.saveCategory(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:saved category", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Titles []string `json:"titles"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.reorderCategories(actor, body.Titles)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:reordered categories", "count", len(body.Titles))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) moveLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
		By    int    `json:"by"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.moveLink(actor, body.Title, body.By)
	if err != nil {
		refuse(w, err)
		return
	}
	if len(ops) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:moved link", "title", body.Title, "by", body.By)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.deleteCategory(actor, body.Title)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:deleted category", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.admin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email        string          `json:"email"`
		Admins       []string        `json:"admins"`
		Apps         []AppVisibility `json:"apps"`
		People       []Person        `json:"people"`
		IsSuperAdmin bool            `json:"isSuperAdmin"`
	}{Email: actor.Email, Admins: a.cache.Admins(), Apps: a.cache.AppVisibilities(), People: a.people(), IsSuperAdmin: a.cache.IsSuperAdmin(actor.Email)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	admins, ops, err := a.cache.setAdmins(actor, body.Admins)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:set the admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setVisibility(w http.ResponseWriter, r *http.Request) {
	var body visibilityEdit
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	key, v, ops, err := a.cache.setVisibility(actor, body)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:set an app's visibility", "app", key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setAppOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Apps []string `json:"apps"`
	}
	if !decode(w, r, &body) {
		return
	}
	actor := a.actor(r)
	ops, err := a.cache.setAppOrder(actor, body.Apps)
	if err != nil {
		refuse(w, err)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home:set the apps' order", "apps", body.Apps)
	w.WriteHeader(http.StatusNoContent)
}
