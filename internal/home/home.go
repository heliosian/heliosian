package home

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/filter"
	"heliosian/internal/imagesearch"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	imageFolder  = "link-images"
	maxImageSize = 8 << 20
)

type Directory interface {
	Sources() filter.Sources
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
}

type Upcoming struct {
	Events    []calendar.Card `json:"events"`
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
	Month    string                  `json:"month"`
	Today    string                  `json:"today"`
	Days     map[string]calendar.Day `json:"days"`
	Events   []calendar.Card         `json:"events"`
	Calendar string                  `json:"calendar,omitempty"`
}

type alerts struct {
	Stale   []string `json:"stale"`
	Privacy []string `json:"privacy"`
}

func Register(mux *http.ServeMux, cache *Cache, media *blob.Store, superAdmins func() []string, heroPhoto func(string) string, people func() []Person, directory Directory, alerts func(string) ([]string, []string), upcoming func(email, token string) Upcoming, month func(email, month, token string) Month, search imagesearch.Search, answer func(ctx context.Context, email, id, answer string) error, makeDefault func(ctx context.Context, email, token string) error) {
	if search.UserAgent == "" {
		search.UserAgent = "Heliosian image search (+https://heliosian.com)"
	}
	a := app{cache: cache, store: media, superAdmins: superAdmins, heroPhoto: heroPhoto, people: people, directory: directory, alerts: alerts, upcoming: upcoming, month: month, search: search, answer: answer, makeDefault: makeDefault}
	cache.directory = directory
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /admin", a.adminPage)
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
	mux.HandleFunc("POST /api/apps/image", a.uploadImage)
	mux.HandleFunc("GET /api/apps/images/search", a.requireAdminFunc(a.search.ServeSearch))
	mux.HandleFunc("GET /api/apps/images/thumb", a.requireAdminFunc(a.search.ServeThumb))
	mux.HandleFunc("POST /api/apps/images/import", a.requireAdminFunc(a.importImage))
	mux.HandleFunc("GET /api/admin/state", a.adminState)
	mux.HandleFunc("POST /api/admin/admins", a.setAdmins)
	mux.HandleFunc("POST /api/admin/visibility", a.setVisibility)
	mux.HandleFunc("POST /api/admin/visibility/order", a.setAppOrder)
	RegisterSwitch(mux, cache)
	a.discoverApps()
}

func (a app) discoverApps() {
	ops := []store.Op{}
	for _, app := range a.cache.MissingVisibility() {
		ops = append(ops, store.Insert(visibilityTab, store.Row{"App": app.Key, "Visibility": VisibleToList, "Tagline": app.Tagline, "Name": app.Name}))
		slog.Info("apps: found a new app, listed for nobody yet", "app", app.Key)
	}
	if err := a.cache.Commit(context.Background(), "app discovery", ops...); err != nil {
		slog.Error("[ERROR] apps: discover apps", "error", err)
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

func (a app) adminPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	serve.File(w, r, "web/home/admin.html")
}

func (a app) requireAdmin(w http.ResponseWriter, r *http.Request) (string, bool) {
	email := strings.ToLower(auth.Email(r))
	if !a.cache.IsAdmin(email) {
		http.Error(w, "admin access required", http.StatusForbidden)
		return "", false
	}
	return email, true
}

func (a app) requireAdminFunc(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.requireAdmin(w, r); ok {
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
	email := strings.ToLower(auth.Email(r))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.month(email, r.URL.Query().Get("month"), r.URL.Query().Get("calendar"))); err != nil {
		slog.ErrorContext(r.Context(), "encode apps calendar", "error", err)
	}
}

func (a app) rsvp(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
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
	email := strings.ToLower(auth.Email(r))
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(a.upcoming(email, r.URL.Query().Get("calendar"))); err != nil {
		slog.ErrorContext(r.Context(), "encode upcoming", "error", err)
	}
}

func (a app) setDefault(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(auth.Email(r))
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
	email := strings.ToLower(auth.Email(r))
	admin := a.cache.IsAdmin(email)
	hidden := hiddenHosts(r.Host, a.cache.HiddenApps(email))
	full := a.cache.Model()
	forMe := func(rules []filter.Rule) bool {
		return len(rules) == 0 || a.cache.includes(rules, email)
	}
	categories := make([]Category, 0, len(full.Categories))
	for _, category := range full.Categories {
		sectionMine := forMe(category.Rules)
		if !sectionMine && !admin {
			continue
		}
		shown := Category{Title: category.Title, Emoji: category.Emoji, Style: category.Style, Max: category.Max, Links: []Link{}, Virtual: category.Virtual, Rules: []filter.Rule{}}
		if admin {
			shown.Rules = category.Rules
		}
		if !sectionMine {
			no := false
			shown.ForMe = &no
		}
		for _, link := range category.Links {
			mine := forMe(link.Rules)
			if ((link.Visible && mine) || admin) && !linksInto(hidden, link.URL) {
				if !mine {
					no := false
					link.ForMe = &no
				}
				if !admin {
					link.Rules = []filter.Rule{}
				}
				shown.Links = append(shown.Links, link)
			}
		}
		categories = append(categories, shown)
	}
	view := struct {
		Categories       []Category            `json:"categories"`
		User             user                  `json:"user"`
		ImageSearch      bool                  `json:"imageSearch"`
		Alerts           alerts                `json:"alerts"`
		Upcoming         []calendar.Card       `json:"upcoming"`
		UpcomingCalendar *Upcoming             `json:"upcomingCalendar,omitempty"`
		Calendar         Month                 `json:"calendar"`
		Apps             []appView             `json:"apps"`
		Options          *filter.Options       `json:"options,omitempty"`
		TagLabels        map[string]string     `json:"tagLabels,omitempty"`
		Widgets          map[string]widgetView `json:"widgets"`
	}{
		Categories:  categories,
		User:        user{Email: email, Initial: strings.ToUpper(email[:1]), PhotoURL: a.heroPhoto(email), IsAdmin: admin},
		ImageSearch: a.search.On(),
		Calendar:    a.month(email, "", ""),
		Apps:        a.appViews(email, admin),
		Widgets:     map[string]widgetView{},
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
		options := filter.OptionsFor(a.directory.Sources(), email)
		view.Options = &options
		view.TagLabels = a.tagLabels(categories, view.Apps, email)
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
	for _, app := range apps {
		hidden[app+"."+tier] = true
		if app == "team" {
			hidden["hca."+tier] = true
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

func (a app) commit(w http.ResponseWriter, r *http.Request, actor string, ops ...store.Op) bool {
	if err := a.cache.Commit(r.Context(), actor, ops...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func (a app) importImage(w http.ResponseWriter, r *http.Request) {
	a.search.ServeImport(w, r, imageFolder, maxImageSize)
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

// widgetView is one widget as the page takes it: whether it is for the
// viewer, and - for an admin - the rules that say who it is for.
type widgetView struct {
	ForMe bool          `json:"forMe"`
	Rules []filter.Rule `json:"rules,omitempty"`
}

// saveWidgetAudience is POST /api/apps/widgets/audience: an admin sets who
// one of the front page's widgets is for, by rules as a section's are.
func (a app) saveWidgetAudience(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Widget string        `json:"widget"`
		Rules  []filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !slices.Contains(Widgets, body.Widget) {
		http.Error(w, "no such widget", http.StatusNotFound)
		return
	}
	key := thingWidget + body.Widget
	was := rulesOf(a.cache.Model(), key)
	rules, err := a.checkRules(was, body.Rules, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if ops := audience(key, was, rules); len(ops) > 0 && !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "home: set a widget's audience", "actor", actor, "widget", body.Widget, "rules", len(rules))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) checkRules(existing, rules []filter.Rule, actor string) ([]filter.Rule, error) {
	options := filter.OptionsFor(a.directory.Sources(), actor)
	out := make([]filter.Rule, 0, len(rules))
	for _, r := range rules {
		r = filter.Clean(r)
		if err := filter.Check(r); err != nil {
			return nil, err
		}
		for _, g := range r.Grades {
			if !slices.Contains(options.Grades, g) {
				return nil, fmt.Errorf("the directory has no grade %s", g)
			}
		}
		for _, c := range r.Classrooms {
			if !slices.Contains(options.Classrooms, c) {
				return nil, fmt.Errorf("the directory has no classroom %s", c)
			}
		}
		out = append(out, r)
	}
	if err := filter.Writable(a.directory.Sources(), actor, a.cache.Admins(), existing, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a app) audienceOptions(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(filter.OptionsFor(a.directory.Sources(), email)); err != nil {
		slog.ErrorContext(r.Context(), "encode audience options", "error", err)
	}
}

func (a app) audiencePreview(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
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
	rules, err := a.checkRules(rulesOf(a.cache.Model(), body.Thing), body.Rules, email)
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

func audience(key string, was, rules []filter.Rule) []store.Op {
	if slices.EqualFunc(was, rules, func(x, y filter.Rule) bool { return maps.Equal(filter.RuleCells(x), filter.RuleCells(y)) }) {
		return nil
	}
	ops := []store.Op{store.Delete(audienceTab, store.Row{"Thing": key})}
	for _, r := range rules {
		cells := filter.RuleCells(r)
		cells["Thing"] = key
		ops = append(ops, store.Insert(audienceTab, cells))
	}
	return ops
}

func (a app) link(title string) *Link {
	for _, c := range a.cache.Model().Categories {
		for _, l := range c.Links {
			if strings.EqualFold(l.Title, strings.TrimSpace(title)) {
				return &l
			}
		}
	}
	return nil
}

func categoryOrder(model *Model, titles []string, events store.Row) ([]store.Op, error) {
	byTitle := map[string]Category{}
	for _, c := range model.Categories {
		byTitle[c.Title] = c
	}
	if len(titles) != len(byTitle) {
		return nil, fmt.Errorf("the order must name every category exactly once")
	}
	current := make([]string, len(titles))
	virtual := make([]bool, len(titles))
	for i, title := range titles {
		c, ok := byTitle[title]
		if !ok {
			return nil, fmt.Errorf("unknown category %s", title)
		}
		delete(byTitle, title)
		current[i], virtual[i] = c.order, c.Virtual
	}
	keys := store.Order(current)
	ops := []store.Op{}
	for i, title := range titles {
		switch {
		case virtual[i]:
			row := maps.Clone(events)
			row[store.OrderColumn] = keys[i]
			ops = append(ops, store.Insert(categoriesTab, row))
		case keys[i] != current[i]:
			ops = append(ops, store.Update(categoriesTab, store.Row{"Title": title}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	return ops, nil
}

func (a app) category(title string) *Category {
	for _, c := range a.cache.Model().Categories {
		if c.Title == title {
			return &c
		}
	}
	return nil
}

func (a app) saveLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original    string        `json:"original"`
		Title       string        `json:"title"`
		Description string        `json:"description"`
		URL         string        `json:"url"`
		Image       string        `json:"image"`
		Category    string        `json:"category"`
		Visible     bool          `json:"visible"`
		Rules       []filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || len(title) > maxTitleLength || len(body.Description) > maxDescLength {
		http.Error(w, "title is required and fields must be short", http.StatusBadRequest)
		return
	}
	existing := a.link(body.Original)
	if body.Original != "" && existing == nil {
		http.Error(w, "no such link", http.StatusNotFound)
		return
	}
	was := rulesOf(a.cache.Model(), thingLink+body.Original)
	rules, err := a.checkRules(was, body.Rules, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cells := store.Row{
		"Title": title, "Description": strings.TrimSpace(body.Description), "URL": strings.TrimSpace(body.URL),
		"Image": strings.TrimSpace(body.Image), "Category": strings.TrimSpace(body.Category), "Visible": visibleCell(body.Visible),
	}
	if existing != nil && existing.Category != cells["Category"] {
		cells[store.OrderColumn] = ""
	}
	action := "edit"
	op := store.Update(linksTab, store.Row{"Title": body.Original}, cells)
	if body.Original == "" {
		action = "add"
		cells["Added By"] = actor
		cells["Added"] = time.Now().Format(addedFormat)
		op = store.Insert(linksTab, cells)
	}
	ops := []store.Op{op}
	if changed := audience(thingLink+title, was, rules); changed != nil {
		if body.Original != "" && body.Original != title {
			ops = append(ops, store.Delete(audienceTab, store.Row{"Thing": thingLink + body.Original}))
		}
		ops = append(ops, changed...)
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: saved link", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	if !a.commit(w, r, actor, store.Delete(linksTab, store.Row{"Title": body.Title})) {
		return
	}
	slog.InfoContext(r.Context(), "apps: deleted link", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) saveCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Original string        `json:"original"`
		Title    string        `json:"title"`
		Emoji    string        `json:"emoji"`
		Style    string        `json:"style"`
		Max      string        `json:"max"`
		Rules    []filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || len(title) > maxTitleLength {
		http.Error(w, "title is required and must be short", http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	if body.Original != "" && a.category(body.Original) == nil {
		http.Error(w, "no such category", http.StatusNotFound)
		return
	}
	was := rulesOf(model, thingCategory+body.Original)
	rules, err := a.checkRules(was, body.Rules, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	style, err := checkStyle(strings.TrimSpace(body.Style))
	if err != nil {
		http.Error(w, "style "+err.Error(), http.StatusBadRequest)
		return
	}
	emoji := strings.TrimSpace(body.Emoji)
	if err := checkEmoji(emoji); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	virtual := a.virtualEvents(body.Original)
	if virtual {
		style = StyleEvents
	} else if body.Original != "" && a.styleOf(body.Original) == StyleEvents {
		style = StyleEvents
	} else if style == StyleEvents {
		http.Error(w, "the events section is the one the page already has", http.StatusBadRequest)
		return
	}
	if style == StyleApps {
		if other := a.titleOf(StyleApps); other != "" && other != body.Original {
			http.Error(w, fmt.Sprintf("%q is already the community apps section", other), http.StatusBadRequest)
			return
		}
		if c := a.category(body.Original); c != nil && c.Style != StyleApps && len(c.Links) > 0 {
			http.Error(w, "move or delete its links first: the community apps section holds no links", http.StatusBadRequest)
			return
		}
	}
	if _, err := checkMax(body.Max); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cells := store.Row{"Title": title, "Emoji": emoji, "Style": style, "Max": strings.TrimSpace(body.Max)}
	action := "edit"
	var ops []store.Op
	switch {
	case virtual:
		titles := []string{}
		for _, c := range model.Categories {
			titles = append(titles, c.Title)
		}
		if ops, err = categoryOrder(model, titles, cells); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	case body.Original == "":
		action = "add"
		ops = []store.Op{store.Insert(categoriesTab, cells)}
	default:
		ops = []store.Op{store.Update(categoriesTab, store.Row{"Title": body.Original}, cells)}
	}
	if changed := audience(thingCategory+title, was, rules); changed != nil {
		if body.Original != "" && body.Original != title {
			ops = append(ops, store.Delete(audienceTab, store.Row{"Thing": thingCategory + body.Original}))
		}
		ops = append(ops, changed...)
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: saved category", "action", action, "title", title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) reorderCategories(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Titles []string `json:"titles"`
	}
	if !decode(w, r, &body) {
		return
	}
	ops, err := categoryOrder(a.cache.Model(), body.Titles, store.Row{"Title": EventsTitle, "Emoji": EventsEmoji, "Style": StyleEvents})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: reordered categories", "count", len(body.Titles))
	w.WriteHeader(http.StatusNoContent)
}

func (a app) moveLink(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
		By    int    `json:"by"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.By != 1 && body.By != -1 {
		http.Error(w, "by must be 1 or -1", http.StatusBadRequest)
		return
	}
	var titles, current []string
	at := -1
	for _, c := range a.cache.Model().Categories {
		for i, l := range c.Links {
			if l.Title == body.Title {
				at = i
			}
		}
		if at >= 0 {
			for _, l := range c.Links {
				titles, current = append(titles, l.Title), append(current, l.order)
			}
			break
		}
	}
	if at < 0 {
		http.Error(w, "unknown link "+body.Title, http.StatusBadRequest)
		return
	}
	to := at + body.By
	if to < 0 || to >= len(titles) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	titles[at], titles[to] = titles[to], titles[at]
	current[at], current[to] = current[to], current[at]
	keys := store.Order(current)
	ops := []store.Op{}
	for i, title := range titles {
		if keys[i] != current[i] {
			ops = append(ops, store.Update(linksTab, store.Row{"Title": title}, store.Row{store.OrderColumn: keys[i]}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: moved link", "title", body.Title, "by", body.By)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) deleteCategory(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if !decode(w, r, &body) {
		return
	}
	if a.styleOf(body.Title) == StyleEvents {
		http.Error(w, "the events section can be renamed or moved, not deleted", http.StatusBadRequest)
		return
	}
	if c := a.category(body.Title); c != nil && len(c.Links) > 0 {
		http.Error(w, "move or delete its links first", http.StatusBadRequest)
		return
	}
	if !a.commit(w, r, actor, store.Delete(categoriesTab, store.Row{"Title": body.Title})) {
		return
	}
	slog.InfoContext(r.Context(), "apps: deleted category", "title", body.Title)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) styleOf(title string) string {
	for _, c := range a.cache.Model().Categories {
		if c.Title == title {
			return c.Style
		}
	}
	return ""
}

func (a app) titleOf(style string) string {
	for _, c := range a.cache.Model().Categories {
		if c.Style == style {
			return c.Title
		}
	}
	return ""
}

func (a app) virtualEvents(title string) bool {
	for _, c := range a.cache.Model().Categories {
		if c.Title == title {
			return c.Virtual
		}
	}
	return false
}

func (a app) uploadImage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImageSize)
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "an image file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "could not read the image", http.StatusBadRequest)
		return
	}
	mimeType := http.DetectContentType(content)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[mimeType]
	if ext == "" {
		http.Error(w, fmt.Sprintf("%s is not a supported image", header.Filename), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:]) + ext
	if err := a.store.Put(imageFolder, name, mimeType, content); err != nil {
		slog.ErrorContext(r.Context(), "store link image", "error", err)
		http.Error(w, "could not store the image", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"name": imageFolder + "/" + name}); err != nil {
		slog.ErrorContext(r.Context(), "encode image name", "error", err)
	}
}

func (a app) adminState(w http.ResponseWriter, r *http.Request) {
	email, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	view := struct {
		Email        string          `json:"email"`
		Admins       []string        `json:"admins"`
		Apps         []AppVisibility `json:"apps"`
		People       []Person        `json:"people"`
		IsSuperAdmin bool            `json:"isSuperAdmin"`
	}{Email: email, Admins: a.cache.Admins(), Apps: a.cache.AppVisibilities(), People: a.people(), IsSuperAdmin: a.cache.IsSuperAdmin(email)}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(view); err != nil {
		slog.ErrorContext(r.Context(), "encode apps admin state", "error", err)
	}
}

func (a app) setAdmins(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Admins []string `json:"admins"`
	}
	if !decode(w, r, &body) {
		return
	}
	super := map[string]bool{}
	for _, e := range a.superAdmins() {
		super[e] = true
	}
	admins := []string{}
	for _, e := range normalizeEmails(body.Admins) {
		if !super[e] {
			admins = append(admins, e)
		}
	}
	current := a.cache.Model().admins
	ops := []store.Op{}
	for _, e := range current {
		if !slices.Contains(admins, e) {
			ops = append(ops, store.Delete(adminsTab, store.Row{"Email": e}))
		}
	}
	for _, e := range admins {
		if !slices.Contains(current, e) {
			ops = append(ops, store.Insert(adminsTab, store.Row{"Email": e}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: set the admin list", "admins", admins)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setVisibility(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		App        string         `json:"app"`
		Visibility string         `json:"visibility"`
		Emails     []string       `json:"emails"`
		Tagline    string         `json:"tagline"`
		Name       string         `json:"name"`
		Rules      *[]filter.Rule `json:"rules"`
	}
	if !decode(w, r, &body) {
		return
	}
	key := strings.ToLower(strings.TrimSpace(body.App))
	if !appKnown(key) {
		http.Error(w, "app must be one of "+strings.Join(appKeys(), ", "), http.StatusBadRequest)
		return
	}
	if body.Visibility != VisibleToEveryone && body.Visibility != VisibleToList {
		http.Error(w, "visibility must be "+VisibleToEveryone+" or "+VisibleToList, http.StatusBadRequest)
		return
	}
	tagline := strings.TrimSpace(body.Tagline)
	if tagline == "" || len(tagline) > maxDescLength {
		http.Error(w, "a short tagline is required", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > maxTitleLength {
		http.Error(w, "a short name is required", http.StatusBadRequest)
		return
	}
	app, _ := appByKey(key)
	was := visibilityOf(a.cache.Model(), app)
	rules := was.Rules
	if body.Rules != nil {
		checked, err := a.checkRules(was.Rules, *body.Rules, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		rules = checked
	}
	v := Visibility{Mode: body.Visibility, Emails: normalizeEmails(body.Emails), Tagline: tagline, Name: name, Order: was.Order, Rules: rules}
	ops := append([]store.Op{store.Set(visibilityTab, store.Row{"App": key}, v.cells())}, audience(thingApp+key, was.Rules, rules)...)
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: set an app's visibility", "app", key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
	w.WriteHeader(http.StatusNoContent)
}

func (a app) setAppOrder(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	var body struct {
		Apps []string `json:"apps"`
	}
	if !decode(w, r, &body) {
		return
	}
	keys := make([]string, 0, len(body.Apps))
	for _, key := range body.Apps {
		keys = append(keys, strings.ToLower(strings.TrimSpace(key)))
	}
	slices.Sort(keys)
	want := appKeys()
	slices.Sort(want)
	if !slices.Equal(keys, want) {
		http.Error(w, "the order must list every app once: "+strings.Join(appKeys(), ", "), http.StatusBadRequest)
		return
	}
	model := a.cache.Model()
	apps := make([]App, 0, len(body.Apps))
	current := make([]string, 0, len(body.Apps))
	for _, key := range body.Apps {
		app, _ := appByKey(strings.ToLower(strings.TrimSpace(key)))
		apps, current = append(apps, app), append(current, visibilityOf(model, app).Order)
	}
	next := store.Order(current)
	ops := []store.Op{}
	for i, app := range apps {
		if next[i] != current[i] {
			ops = append(ops, store.Set(visibilityTab, store.Row{"App": app.Key}, store.Row{"Visibility": visibilityOf(model, app).Mode, store.OrderColumn: next[i]}))
		}
	}
	if !a.commit(w, r, actor, ops...) {
		return
	}
	slog.InfoContext(r.Context(), "apps: set the apps' order", "apps", body.Apps)
	w.WriteHeader(http.StatusNoContent)
}
