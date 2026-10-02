package model

import (
	"regexp"
	"slices"
	"strings"

	"heliosian/internal/api"
	"heliosian/internal/id"
	"heliosian/internal/serve"
)

const (
	homeHost          = "home"
	kindApp           = "app"
	kindHomeWidget    = "home-widget"
	kindHomeSettings  = "home-settings"
	kindSchoolEmail   = "school-email"
	kindAlerts        = "alerts"
	kindAdminList     = "admin-list"
	homeLinksType     = "links"
	homeCategoryType  = "link-categories"
	homeAppsType      = "apps"
	homeWidgetsType   = "home-widgets"
	homeSettingsType  = "home-settings"
	schoolEmailsType  = "school-emails"
	alertsType        = "alerts"
	adminListsType    = "admin-lists"
	schoolEmailsSince = SchoolMailWindow
)

type linkResource struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Image       string     `json:"image,omitempty"`
	ImageURL    string     `json:"imageUrl,omitempty"`
	Visible     bool       `json:"visible"`
	AddedBy     string     `json:"addedBy,omitempty"`
	Added       string     `json:"added,omitempty"`
	Order       string     `json:"order"`
	Rules       []RuleView `json:"rules,omitempty"`
	ForMe       *bool      `json:"forMe,omitempty"`
}

type linkCategoryResource struct {
	Title        string     `json:"title"`
	Emoji        string     `json:"emoji,omitempty"`
	Style        string     `json:"style"`
	Descriptions bool       `json:"descriptions"`
	Order        string     `json:"order"`
	Rules        []RuleView `json:"rules,omitempty"`
	ForMe        *bool      `json:"forMe,omitempty"`
}

type appMe struct {
	Listed bool `json:"listed"`
}

type appResource struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Tagline    string     `json:"tagline"`
	Mark       string     `json:"mark,omitempty"`
	Order      string     `json:"order,omitempty"`
	Visibility string     `json:"visibility,omitempty"`
	Emails     []string   `json:"emails,omitempty"`
	Rules      []RuleView `json:"rules,omitempty"`
	Me         appMe      `json:"me"`
}

type widgetMe struct {
	Shown bool `json:"shown"`
}

type widgetResource struct {
	Key     string     `json:"key"`
	Order   string     `json:"order"`
	Sidebar bool       `json:"sidebar"`
	Rules   []RuleView `json:"rules,omitempty"`
	Me      widgetMe   `json:"me"`
}

type homeSettingsResource struct {
	ImageSearch bool     `json:"imageSearch"`
	Roles       []string `json:"roles"`
	Relations   []string `json:"relations"`
	Layout      []string `json:"layout"`
	Layouts     []string `json:"layouts"`
}

type schoolEmailResource struct {
	SchoolEmail
	To string `json:"to"`
}

type adminListResource struct {
	App    string   `json:"app"`
	Admins []string `json:"admins"`
}

type adminsBody struct {
	Admins []string `json:"admins"`
}

func (h HomeHooks) Resources() []api.Type[*Model] {
	a := h.app
	return []api.Type[*Model]{a.linksType(), a.linkCategoriesType(), a.appsType(), a.widgetsType(), a.settingsType(), schoolEmailResources(), a.toDosType(), alertResources(), adminListResources(a.store)}
}

func derived(m *Model, kind, key string) string {
	return id.Of(m.EmailLists.idKey, kind, key)
}

func (m *Model) homeFor(q api.Query) []HomeCategory {
	s := m.scope
	s.homeOnce.Do(func() {
		s.home = m.HomeCategoriesFor(q.Actor)
		s.homeLinks = map[string]HomeLink{}
		s.homeCategories = map[string]HomeCategory{}
		for _, c := range s.home {
			s.homeCategories[c.ID] = c
			for _, l := range c.Links {
				s.homeLinks[l.ID] = l
			}
		}
	})
	return s.home
}

func (m *Model) homeLink(q api.Query, key string) (HomeLink, bool) {
	m.homeFor(q)
	l, ok := m.scope.homeLinks[key]
	return l, ok
}

func (m *Model) homeCategory(q api.Query, key string) (HomeCategory, bool) {
	m.homeFor(q)
	c, ok := m.scope.homeCategories[key]
	return c, ok
}

func (m *Model) homeRules(q api.Query, rules []Rule) []RuleView {
	if !q.Actor.May(ConfigureHome) {
		return nil
	}
	sources := m.Audience(q.Now)
	admins := m.AdminList("home").Admins()
	out := []RuleView{}
	for _, r := range rules {
		out = append(out, RuleView{Rule: r, TagLabels: sources.TagLabels(r, admins, q.Actor.Email)})
	}
	return out
}

func configuresHome(_ *Model, q api.Query, _ string) bool {
	return q.Actor.May(ConfigureHome)
}

func (a homeApp) linksType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  homeLinksType,
		Shape: linkResource{},
		Has:   func(m *Model, key string) bool { return m.Home.link(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			l, ok := m.homeLink(q, key)
			if !ok {
				return nil, false
			}
			return linkResource{
				Title: l.Title, Description: l.Description, URL: l.URL, Image: l.Image, ImageURL: l.ImageURL, Visible: l.Visible,
				AddedBy: l.AddedBy, Added: l.Added, Order: l.Order, Rules: m.homeRules(q, l.Rules), ForMe: l.ForMe,
			}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, c := range m.homeFor(q) {
				for _, l := range c.Links {
					out = append(out, l.ID)
				}
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"category": {Type: homeCategoryType, List: func(m *Model, q api.Query, key string) []string {
				if l, ok := m.homeLink(q, key); ok {
					return []string{l.Category}
				}
				return nil
			}},
		},
		Filters: map[string]api.Filter[*Model]{
			"q": func(m *Model, q api.Query, value string) (func(string) bool, error) {
				return func(key string) bool {
					l, ok := m.homeLink(q, key)
					return ok && mentions(value, l.Title, l.Description)
				}, nil
			},
		},
		Create: api.Make(func(wr api.Write[*Model], body linkEdit) (string, error) {
			_, key, ops, err := wr.S.saveHomeLink(wr.Query.Actor, "", body, wr.Taken)
			if err := stage(wr, homeAppName, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "home: saved link", "action", "add", "id", key, "title", strings.TrimSpace(body.Title))
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(configuresHome, func(wr api.Write[*Model]) linkEdit {
				l := wr.S.Home.link(wr.ID)
				return linkEdit{Title: l.Title, Description: l.Description, URL: l.URL, Image: l.Image, Category: l.Category, Visible: l.Visible, Order: l.Order, Rules: copyRules(l.Rules)}
			}, func(wr api.Write[*Model], body linkEdit) error {
				_, key, ops, err := wr.S.saveHomeLink(wr.Query.Actor, wr.ID, body, wr.Taken)
				logAfter(wr, "home: saved link", "action", "edit", "id", key, "title", strings.TrimSpace(body.Title))
				return stage(wr, homeAppName, ops, err)
			}),
			"delete": api.Do(configuresHome, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Home.deleteLink(wr.Query.Actor, wr.ID)
				logAfter(wr, "home: deleted link", "id", wr.ID)
				return stage(wr, homeAppName, ops, err)
			}),
		},
	}
}

func (a homeApp) linkCategoriesType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  homeCategoryType,
		Shape: linkCategoryResource{},
		Has:   func(m *Model, key string) bool { return m.Home.category(key) != nil },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			c, ok := m.homeCategory(q, key)
			if !ok {
				return nil, false
			}
			return linkCategoryResource{Title: c.Title, Emoji: c.Emoji, Style: c.Style, Descriptions: c.Descriptions, Order: c.Order, Rules: m.homeRules(q, c.Rules), ForMe: c.ForMe}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, c := range m.homeFor(q) {
				out = append(out, c.ID)
			}
			return out
		},
		Relations: map[string]api.Relation[*Model]{
			"links": {Type: homeLinksType, Many: true, List: func(m *Model, q api.Query, key string) []string {
				c, ok := m.homeCategory(q, key)
				if !ok {
					return nil
				}
				out := []string{}
				for _, l := range c.Links {
					out = append(out, l.ID)
				}
				return out
			}},
		},
		Create: api.Make(func(wr api.Write[*Model], body categoryEdit) (string, error) {
			_, key, ops, err := wr.S.saveHomeCategory(wr.Query.Actor, "", body, wr.Taken)
			if err := stage(wr, homeAppName, ops, err); err != nil {
				return "", err
			}
			logAfter(wr, "home: saved category", "action", "add", "id", key, "title", strings.TrimSpace(body.Title))
			return key, nil
		}),
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(configuresHome, func(wr api.Write[*Model]) categoryEdit {
				c := wr.S.Home.category(wr.ID)
				return categoryEdit{Title: c.Title, Emoji: c.Emoji, Style: c.Style, Descriptions: c.Descriptions, Sidebar: wr.S.Home.sidebar[thingCategory+c.ID], Order: c.Order, Rules: copyRules(c.Rules)}
			}, func(wr api.Write[*Model], body categoryEdit) error {
				_, key, ops, err := wr.S.saveHomeCategory(wr.Query.Actor, wr.ID, body, wr.Taken)
				logAfter(wr, "home: saved category", "action", "edit", "id", key, "title", strings.TrimSpace(body.Title))
				return stage(wr, homeAppName, ops, err)
			}),
			"delete": api.Do(func(m *Model, q api.Query, key string) bool {
				c := m.Home.category(key)
				return q.Actor.May(ConfigureHome) && c != nil && c.Style != StyleEvents && len(c.Links) == 0
			}, func(wr api.Write[*Model], _ serve.None) error {
				ops, err := wr.S.Home.deleteCategory(wr.Query.Actor, wr.ID)
				logAfter(wr, "home: deleted category", "id", wr.ID)
				return stage(wr, homeAppName, ops, err)
			}),
		},
	}
}

func homeAppKeys() []string {
	return append([]string{HomeApp.Key}, appKeys()...)
}

func appByResource(m *Model, key string) (App, bool) {
	for _, k := range homeAppKeys() {
		if derived(m, kindApp, k) == key {
			if k == HomeApp.Key {
				return homeAppWithMark(), true
			}
			app, _ := appByKey(k)
			return app, true
		}
	}
	return App{}, false
}

func (a homeApp) appsType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  homeAppsType,
		Shape: appResource{},
		Has: func(m *Model, key string) bool {
			_, ok := appByResource(m, key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			app, ok := appByResource(m, key)
			if !ok {
				return nil, false
			}
			if app.Key == HomeApp.Key {
				return appResource{Key: app.Key, Name: app.Name, Tagline: app.Tagline, Mark: app.Mark, Me: appMe{Listed: true}}, true
			}
			v := appVisibilityOf(m.Home, app)
			out := appResource{Key: app.Key, Name: v.Name, Tagline: v.Tagline, Mark: markVersion(app.Key), Order: v.Order, Me: appMe{Listed: !slices.Contains(m.HiddenApps(q.Actor.Email), app.Key)}}
			if q.Actor.May(ConfigureHome) {
				out.Visibility, out.Emails, out.Rules = v.Mode, v.Emails, m.homeRules(q, v.Rules)
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{derived(m, kindApp, HomeApp.Key)}
			for _, app := range m.Home.AppList() {
				out = append(out, derived(m, kindApp, app.Key))
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, k := range homeAppKeys() {
				out[k] = derived(m, kindApp, k)
			}
			return out
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(func(m *Model, q api.Query, key string) bool {
				app, ok := appByResource(m, key)
				return ok && app.Key != HomeApp.Key && q.Actor.May(ConfigureHome)
			}, func(wr api.Write[*Model]) visibilityEdit {
				app, _ := appByResource(wr.S, wr.ID)
				v := wr.S.Home.Visibility[app.Key]
				return visibilityEdit{Visibility: v.Mode, Emails: slices.Clone(v.Emails), Tagline: v.Tagline, Name: v.Name, Order: v.Order, Rules: copyRules(v.Rules)}
			}, func(wr api.Write[*Model], body visibilityEdit) error {
				app, _ := appByResource(wr.S, wr.ID)
				v, ops, err := wr.S.setHomeVisibility(wr.Query.Actor, app.Key, body, sent(wr.Body))
				logAfter(wr, "home: set an app's visibility", "app", app.Key, "visibility", v.Mode, "emails", len(v.Emails), "name", v.Name, "tagline", v.Tagline)
				return stage(wr, homeAppName, ops, err)
			}),
		},
	}
}

func widgetByResource(m *Model, key string) (string, bool) {
	for _, w := range m.Home.WidgetOrder {
		if derived(m, kindHomeWidget, w) == key {
			return w, true
		}
	}
	return "", false
}

func (a homeApp) widgetsType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  homeWidgetsType,
		Shape: widgetResource{},
		Has: func(m *Model, key string) bool {
			_, ok := widgetByResource(m, key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			w, ok := widgetByResource(m, key)
			if !ok {
				return nil, false
			}
			rules := m.Home.WidgetRules[w]
			shown := len(rules) == 0 || m.homeIncludes(rules, q.Actor.Email)
			return widgetResource{Key: w, Order: m.Home.widgetKeys[w], Sidebar: m.Home.sidebar[w], Rules: m.homeRules(q, rules), Me: widgetMe{Shown: shown}}, true
		},
		List: func(m *Model, _ api.Query) []string {
			out := []string{}
			for _, w := range m.Home.WidgetOrder {
				out = append(out, derived(m, kindHomeWidget, w))
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, w := range m.Home.WidgetOrder {
				out[w] = derived(m, kindHomeWidget, w)
			}
			return out
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.DoFrom(configuresHome, func(wr api.Write[*Model]) widgetEdit {
				w, _ := widgetByResource(wr.S, wr.ID)
				return widgetEdit{Order: wr.S.Home.widgetKeys[w], Sidebar: wr.S.Home.sidebar[w], Rules: copyRules(wr.S.Home.WidgetRules[w])}
			}, func(wr api.Write[*Model], body widgetEdit) error {
				w, _ := widgetByResource(wr.S, wr.ID)
				ops, err := wr.S.saveHomeWidget(wr.Query.Actor, w, body)
				logAfter(wr, "home: saved a widget", "widget", w, "rules", len(body.Rules), "order", body.Order, "sidebar", body.Sidebar)
				return stage(wr, homeAppName, ops, err)
			}),
		},
	}
}

func (a homeApp) settingsType() api.Type[*Model] {
	stage := a.store.stage
	return api.Type[*Model]{
		Name:  homeSettingsType,
		Shape: homeSettingsResource{},
		Has:   func(m *Model, key string) bool { return key == derived(m, kindHomeSettings, "") },
		Get: func(m *Model, _ api.Query, key string) (any, bool) {
			if key != derived(m, kindHomeSettings, "") {
				return nil, false
			}
			return homeSettingsResource{ImageSearch: a.search.On(), Roles: AudienceRoles, Relations: AudienceRelations, Layout: m.Home.Layout, Layouts: RowLayouts}, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{derived(m, kindHomeSettings, "")} },
		Actions: map[string]api.Action[*Model]{
			"layout": api.DoFrom(configuresHome, func(wr api.Write[*Model]) layoutEdit {
				return layoutEdit{Rows: slices.Clone(wr.S.Home.Layout)}
			}, func(wr api.Write[*Model], body layoutEdit) error {
				ops, err := wr.S.Home.saveLayout(wr.Query.Actor, body)
				logAfter(wr, "home: saved the page layout", "rows", body.Rows)
				return stage(wr, homeAppName, ops, err)
			}),
		},
		Relations: map[string]api.Relation[*Model]{
			"viewer": {Type: "people", List: func(m *Model, q api.Query, _ string) []string { return m.personID(q.Actor.Email) }},
		},
	}
}

func (m *Model) schoolEmails(q api.Query) ([]string, map[string]SchoolEmail) {
	s := m.scope
	s.schoolOnce.Do(func() {
		s.schoolOrder, s.school = []string{}, map[string]SchoolEmail{}
		since := q.Now.Add(-schoolEmailsSince).Format(DateFormat)
		for _, e := range m.Documents.SchoolMail(m.Directory, q.Actor.Email, since) {
			key := derived(m, kindSchoolEmail, e.Key)
			s.schoolOrder = append(s.schoolOrder, key)
			s.school[key] = e
		}
	})
	return s.schoolOrder, s.school
}

func schoolEmailResources() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  schoolEmailsType,
		Shape: schoolEmailResource{},
		Has: func(m *Model, key string) bool {
			for _, d := range m.Documents.Documents {
				if d.School() && derived(m, kindSchoolEmail, d.Key) == key {
					return true
				}
			}
			return false
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			_, byKey := m.schoolEmails(q)
			e, ok := byKey[key]
			if !ok {
				return nil, false
			}
			return schoolEmailResource{SchoolEmail: e, To: sentTo(e)}, true
		},
		List: func(m *Model, q api.Query) []string {
			order, _ := m.schoolEmails(q)
			return order
		},
	}
}

var schoolListNames = map[string]string{
	"newsletter": "Newsletter", "parentsandstaff": "Parents & staff", "parentsonly": "Parents", "parentsandstudents": "Parents & students",
	"community": "Community", "parents": "All parents", "newstudentfamilies": "New families", "new.parents": "New families",
}

var gradeName = regexp.MustCompile(`^Grade \d+$`)

func andList(list []string) string {
	if len(list) > 1 {
		return strings.Join(list[:len(list)-1], ", ") + " & " + list[len(list)-1]
	}
	return strings.Join(list, "")
}

func capitalized(word string) string {
	if word == "" {
		return ""
	}
	return strings.ToUpper(word[:1]) + word[1:]
}

func sentTo(e SchoolEmail) string {
	if e.Kind != DocumentKindList {
		if e.Audience == "" || e.Audience == "Everyone" {
			return "All families"
		}
		rooms, grades, numbers := []string{}, []string{}, []string{}
		for _, n := range strings.Split(e.Audience, ",") {
			n = strings.TrimSpace(n)
			switch {
			case n == "":
			case gradeName.MatchString(n) || n == "Kindergarten":
				grades = append(grades, n)
				if n != "Kindergarten" {
					numbers = append(numbers, strings.TrimPrefix(n, "Grade "))
				}
			default:
				rooms = append(rooms, n)
			}
		}
		words := []string{}
		if slices.Contains(grades, "Kindergarten") {
			if len(numbers) > 0 {
				words = append(words, "K")
			} else {
				words = append(words, "Kindergarten")
			}
		}
		words = append(words, numbers...)
		parts := []string{}
		if len(rooms) > 0 {
			parts = append(parts, andList(rooms))
		}
		switch len(grades) {
		case 0:
		case 1:
			parts = append(parts, grades[0])
		default:
			parts = append(parts, "Grades "+andList(words))
		}
		return strings.Join(parts, " · ")
	}
	if name, ok := schoolListNames[e.Channel]; ok {
		return name
	}
	room, who, dotted := strings.Cut(e.Channel, ".")
	if dotted {
		return capitalized(room) + " " + who
	}
	parts := []string{}
	for _, p := range strings.Split(room, "and") {
		parts = append(parts, capitalized(p))
	}
	return strings.Join(parts, " & ")
}

func alertResources() api.Type[*Model] {
	return api.Type[*Model]{
		Name:  alertsType,
		Shape: Alerts{},
		Has:   func(m *Model, key string) bool { return key == derived(m, kindAlerts, "") },
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if key != derived(m, kindAlerts, "") {
				return nil, false
			}
			out := m.Directory.Alerts(q.Actor.Email, m.Config.StaleYears, q.Now)
			if out.Stale == nil {
				out.Stale = []string{}
			}
			if out.Privacy == nil {
				out.Privacy = []string{}
			}
			return out, true
		},
		List: func(m *Model, _ api.Query) []string { return []string{derived(m, kindAlerts, "")} },
	}
}

const superAdminList = "super"

func adminListKeys() []string {
	out := []string{}
	for _, a := range adminApps {
		out = append(out, a.key)
	}
	return append(out, superAdminList)
}

func adminListByResource(m *Model, key string) (string, bool) {
	for _, k := range adminListKeys() {
		if derived(m, kindAdminList, k) == key {
			return k, true
		}
	}
	return "", false
}

func managesAdmins(m *Model, q api.Query, key string) bool {
	app, ok := adminListByResource(m, key)
	if app == superAdminList {
		return q.Actor.May(ManageSuperAdmins)
	}
	return ok && q.Actor.May(ManageAdmins(app))
}

func adminListResources(s *Store) api.Type[*Model] {
	return api.Type[*Model]{
		Name:  adminListsType,
		Shape: adminListResource{},
		Has: func(m *Model, key string) bool {
			_, ok := adminListByResource(m, key)
			return ok
		},
		Get: func(m *Model, q api.Query, key string) (any, bool) {
			if !managesAdmins(m, q, key) {
				return nil, false
			}
			app, _ := adminListByResource(m, key)
			if app == superAdminList {
				return adminListResource{App: app, Admins: slices.Clone(m.Config.SuperAdmins)}, true
			}
			return adminListResource{App: app, Admins: m.AdminList(app).Admins()}, true
		},
		List: func(m *Model, q api.Query) []string {
			out := []string{}
			for _, k := range adminListKeys() {
				if key := derived(m, kindAdminList, k); managesAdmins(m, q, key) {
					out = append(out, key)
				}
			}
			return out
		},
		Aliases: func(m *Model) map[string]string {
			out := map[string]string{}
			for _, k := range adminListKeys() {
				out[k] = derived(m, kindAdminList, k)
			}
			return out
		},
		Actions: map[string]api.Action[*Model]{
			"edit": api.Do(managesAdmins, func(wr api.Write[*Model], body adminsBody) error {
				app, _ := adminListByResource(wr.S, wr.ID)
				if app == superAdminList {
					admins, ops, err := wr.S.Config.setSuperAdmins(wr.Query.Actor, body.Admins)
					logAfter(wr, "config: set the super admin list", "admins", admins)
					return s.stage(wr, ConfigApp, ops, err)
				}
				l := wr.S.AdminList(app)
				ops, admins, err := l.set(wr.Query.Actor, body.Admins)
				logAfter(wr, app+": set the admin list", "admins", admins)
				return s.stage(wr, l.sheet, ops, err)
			}),
		},
	}
}
