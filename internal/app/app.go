package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/calendarimport"
	"heliosian/internal/celebrate"
	"heliosian/internal/claude"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/feedback"
	"heliosian/internal/filter"
	"heliosian/internal/geocode"
	"heliosian/internal/home"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
	"heliosian/internal/serve"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

const deployOverlap = 30 * time.Second

const (
	Domain    = "heliosian.com"
	DevDomain = "heliosiandev.com"
)

var aliases = map[string]string{"hca": "team", "cal": "calendar", "when": "calendar"}

var spend = claude.NewLimiter()

func appFor(domain, host string) string {
	if host == domain || host == "www."+domain {
		return "home"
	}
	app, ok := strings.CutSuffix(host, "."+domain)
	if !ok || strings.Contains(app, ".") {
		return ""
	}
	if canonical, ok := aliases[app]; ok {
		return canonical
	}
	return app
}

func Hostnames() []string {
	labels := []string{"home"}
	for _, a := range home.Apps {
		labels = append(labels, a.Key)
	}
	for alias := range aliases {
		labels = append(labels, alias)
	}
	slices.Sort(labels)
	out := []string{"heliosian.com", "www.heliosian.com"}
	for _, label := range labels {
		out = append(out, label+".heliosian.com")
	}
	return out
}

type staticFiles struct{}

func (staticFiles) Has(key string) (bool, error) {
	_, err := os.Stat(filepath.Join("web/who", filepath.FromSlash(key)))
	return err == nil, nil
}

func (staticFiles) Prefetch([]string) error { return nil }

func bundled(roots []string, key string) bool {
	for _, root := range roots {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(key))); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func uploaded(store *blob.Store, key string) (bool, error) {
	if store == nil {
		return false, nil
	}
	return store.Has(key)
}

func prefetchUploaded(store *blob.Store, folder string, names []string) error {
	if store == nil {
		return nil
	}
	keys := []string{}
	for _, name := range names {
		if strings.HasPrefix(name, folder) {
			keys = append(keys, name)
		}
	}
	return store.Prefetch(keys)
}

type homeImages struct {
	store *blob.Store
}

func (h homeImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "link-images/") {
		return uploaded(h.store, key)
	}
	return bundled([]string{"web/home", "web/public/home"}, key), nil
}

func (h homeImages) Prefetch(names []string) error {
	return prefetchUploaded(h.store, "link-images/", names)
}

type teamImages struct {
	store *blob.Store
}

func (e teamImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "activity-images/") {
		return uploaded(e.store, key)
	}
	return bundled([]string{"web/team", "web/public/team"}, key), nil
}

func (e teamImages) Prefetch(names []string) error {
	return prefetchUploaded(e.store, "activity-images/", names)
}

type celebrateImages struct {
	store *blob.Store
}

func (c celebrateImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "party-images/") {
		return uploaded(c.store, key)
	}
	return bundled([]string{"web/celebrate", "web/public/celebrate"}, key), nil
}

func (c celebrateImages) Prefetch(names []string) error {
	return prefetchUploaded(c.store, "party-images/", names)
}

type calendarImages struct {
	store *blob.Store
}

func (c calendarImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "category-images/") {
		return uploaded(c.store, key)
	}
	return bundled([]string{"web/calendar", "web/public/calendar"}, key), nil
}

func (c calendarImages) Prefetch(names []string) error {
	return prefetchUploaded(c.store, "category-images/", names)
}

type directory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d directory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func (d directory) Person(email string) (string, string, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return "", "", false
	}
	return p.FullName, model.HeroPhoto(p.Email), true
}

func (d directory) Grade(email string) string {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsStudent {
		return ""
	}
	return p.Grade
}

func (d directory) Parents(email string) []string {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsStudent {
		return nil
	}
	return p.ParentContactEmails
}

func (d directory) GradeColors() map[string]string {
	return d.settings.Settings().GradeColors
}

func (d directory) HomePeople() []home.Person {
	model := d.cache.Model()
	out := make([]home.Person, 0, len(model.People))
	for _, p := range model.People {
		if p.Email != "" {
			out = append(out, home.Person{Name: p.FullName, Email: p.Email})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type audienceSources struct{ loopDirectory }

func (s audienceSources) Sources() filter.Sources {
	return loop.SourcesOf(s.loopDirectory)
}

func (d directory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

func (d directory) SpoofPerson(email string) (auth.Person, bool) {
	model := d.cache.Model()
	p := model.Person(model.Resolve(strings.ToLower(strings.TrimSpace(email))))
	if p == nil {
		return auth.Person{}, false
	}
	return auth.Person{Email: p.Email, Name: p.FullName, Words: placeWords(*p)}, true
}

func (d directory) SpoofPeople() []auth.Person {
	model := d.cache.Model()
	out := make([]auth.Person, 0, len(model.People))
	for _, p := range model.People {
		out = append(out, auth.Person{Email: p.Email, Name: p.FullName, Words: placeWords(p)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func placeWords(p who.Person) string {
	switch {
	case p.IsStaff:
		if p.JobTitle == "" {
			return "Staff"
		}
		return p.JobTitle
	case p.IsStudent:
		if p.Grade == "" {
			return "Student"
		}
		return p.Grade
	case p.IsParent:
		return "Parent"
	}
	return ""
}

func (d directory) People() []team.DirectoryPerson {
	model := d.cache.Model()
	out := make([]team.DirectoryPerson, 0, len(model.People))
	for _, p := range model.People {
		title := placeWords(p)
		person := team.DirectoryPerson{
			Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Title: title,
			IsStudent: p.IsStudent, ParentEmails: p.ParentContactEmails,
			Pronouns: p.Pronouns, Phone: p.Phone, Grade: p.Grade, Classroom: p.Classroom,
			JobTitle: p.JobTitle, Department: p.Department,
		}
		if p.IsParent {
			person.Spouses, person.Children = household(model, p)
		}
		out = append(out, person)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func household(model *who.Model, p who.Person) (adults, kids []team.Child) {
	for _, key := range model.FamilyKeysOf(p.Email) {
		family := model.Families[key]
		for _, adult := range family.AdultEmails {
			if a := model.Person(adult); a != nil && a.Email != p.Email {
				adults = append(adults, team.Child{Email: a.Email, Name: a.FullName})
			}
		}
		for _, kid := range family.KidEmails {
			if k := model.Person(kid); k != nil {
				kids = append(kids, team.Child{Email: k.Email, Name: k.FullName, Grade: k.Grade})
			}
		}
	}
	return adults, kids
}

func (d directory) Household(email string) (adults, kids []team.Child) {
	p := d.cache.Model().Person(email)
	if p == nil || !p.IsParent {
		return nil, nil
	}
	return household(d.cache.Model(), *p)
}

type upcomingEvents struct {
	cache     *calendar.Cache
	directory calendar.Directory
	linked    func(email string) []calendar.Linked
}

func (u upcomingEvents) list(email, token string) home.Upcoming {
	model := u.cache.Model()
	out := home.Upcoming{Events: model.UpcomingUnder(u.directory, email, u.linked(email), time.Now().In(calendar.Location), 6, token)}
	out.Calendars, out.Default, out.Calendar = savedCalendars(model, email, token)
	return out
}

func savedCalendars(model *calendar.Model, email, token string) (list []home.SavedCalendar, def, current string) {
	for _, f := range model.MyCalendars(email) {
		list = append(list, home.SavedCalendar{Token: f.Token, Name: f.Name, Emoji: f.Emoji, Locked: f.Locked})
	}
	def = list[0].Token
	current = def
	if slices.ContainsFunc(list, func(c home.SavedCalendar) bool { return c.Token == token }) {
		current = token
	}
	return list, def, current
}

func (u upcomingEvents) month(email, month, token string) home.Month {
	model := u.cache.Model()
	m := model.MonthUnder(u.directory, email, u.linked(email), time.Now().In(calendar.Location), month, token)
	_, _, current := savedCalendars(model, email, token)
	return home.Month{Month: m.Month, Today: m.Today, Days: m.Days, Events: m.Events, Calendar: current}
}

type birthdayDirectory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d birthdayDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func birthdayPerson(p *who.Person) birthday.Person {
	return birthday.Person{Email: p.Email, Name: p.FullName, PhotoURL: p.PhotoURL, JobTitle: p.JobTitle, Department: p.Department}
}

func (d birthdayDirectory) Person(email string) (birthday.Person, bool) {
	p := d.cache.Model().Person(email)
	if p == nil {
		return birthday.Person{}, false
	}
	return birthdayPerson(p), true
}

func (d birthdayDirectory) Staff() []birthday.Person {
	model := d.cache.Model()
	out := []birthday.Person{}
	for i := range model.People {
		if model.People[i].IsStaff {
			out = append(out, birthdayPerson(&model.People[i]))
		}
	}
	return out
}

func (d birthdayDirectory) People() []birthday.Person {
	model := d.cache.Model()
	out := []birthday.Person{}
	for i := range model.People {
		if model.People[i].Email != "" {
			out = append(out, birthdayPerson(&model.People[i]))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (d birthdayDirectory) Departments() []string {
	return d.cache.Model().Departments
}

func (d birthdayDirectory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

type celebrateDirectory struct {
	cache    *who.Cache
	settings *config.Cache
}

func (d celebrateDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func celebratePerson(model *who.Model, p *who.Person) celebrate.Person {
	title := ""
	switch {
	case p.IsStudent:
		title = p.Grade
		if title == "" {
			title = "Student"
		}
	case p.IsStaff:
		title = p.JobTitle
		if title == "" {
			title = "Staff"
		}
	case p.IsParent:
		title = "Parent"
	}
	return celebrate.Person{
		Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), IsStudent: p.IsStudent, IsParent: p.IsParent,
		IsStaff: p.IsStaff, Grade: p.Grade, JobTitle: p.JobTitle, Title: title,
		Pronouns: p.Pronouns, Phone: p.Phone, Classroom: p.Classroom, Department: p.Department, ParentEmails: p.ParentContactEmails,
	}
}

func (d celebrateDirectory) Person(email string) (celebrate.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return celebrate.Person{}, false
	}
	return celebratePerson(model, p), true
}

func (d celebrateDirectory) Household(email string) (adults, kids []celebrate.Person) {
	model := d.cache.Model()
	seen := map[string]bool{email: true}
	for _, key := range model.FamilyKeysOf(email) {
		family := model.Families[key]
		for _, adult := range family.AdultEmails {
			if a := model.Person(adult); a != nil && !seen[a.Email] {
				seen[a.Email] = true
				adults = append(adults, celebratePerson(model, a))
			}
		}
		for _, kid := range family.KidEmails {
			if k := model.Person(kid); k != nil && !seen[k.Email] {
				seen[k.Email] = true
				kids = append(kids, celebratePerson(model, k))
			}
		}
	}
	return adults, kids
}

func (d celebrateDirectory) People() []celebrate.Person {
	model := d.cache.Model()
	out := make([]celebrate.Person, 0, len(model.People))
	for i := range model.People {
		person := celebratePerson(model, &model.People[i])
		if person.IsParent {
			person.Spouses, person.Children = d.Household(person.Email)
		}
		out = append(out, person)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (d celebrateDirectory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

type calendarDirectory struct {
	cache    *who.Cache
	settings *config.Cache
	lists    func(email string) []who.List
}

func (d calendarDirectory) People() []calendar.Person {
	model := d.cache.Model()
	out := make([]calendar.Person, 0, len(model.People))
	for i := range model.People {
		out = append(out, calendarPerson(model, &model.People[i]))
	}
	return out
}

func (d calendarDirectory) Lists(email string) []calendar.List {
	out := []calendar.List{}
	tags := d.cache.Tags(email)
	for _, name := range slices.Sorted(maps.Keys(tags)) {
		out = append(out, calendar.List{Key: "tag:" + name, Name: name, Kind: "tag", People: tags[name]})
	}
	for _, shared := range d.cache.SharedTags(email) {
		out = append(out, calendar.List{Key: "shared:" + shared.Owner + ":" + shared.Name, Name: shared.Name + " (" + shared.OwnerName + "'s)", Kind: "tag", People: shared.People})
	}
	if d.lists != nil {
		for _, list := range d.lists(email) {
			if list.Archived {
				continue
			}
			out = append(out, calendar.List{Key: list.Key, Name: list.Name, Kind: list.Kind, People: list.People})
		}
	}
	return out
}

func (d calendarDirectory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func calendarPerson(model *who.Model, p *who.Person) calendar.Person {
	return calendar.Person{
		Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), IsStudent: p.IsStudent, IsParent: p.IsParent,
		IsStaff: p.IsStaff, Grade: p.Grade, Classroom: p.Classroom, EmailMasked: p.EmailMasked,
	}
}

func (d calendarDirectory) Person(email string) (calendar.Person, bool) {
	model := d.cache.Model()
	p := model.Person(email)
	if p == nil {
		return calendar.Person{}, false
	}
	return calendarPerson(model, p), true
}

func (d calendarDirectory) GradeColors() map[string]string {
	return d.settings.Settings().GradeColors
}

func (d calendarDirectory) Children(email string) []calendar.Person {
	model := d.cache.Model()
	out := []calendar.Person{}
	p := model.Person(email)
	if p == nil || !p.IsParent {
		return out
	}
	for _, key := range model.FamilyKeysOf(email) {
		for _, kid := range model.Families[key].KidEmails {
			if k := model.Person(kid); k != nil {
				out = append(out, calendarPerson(model, k))
			}
		}
	}
	return out
}

func (d calendarDirectory) Alerts(email string) (int, bool) {
	alerts := d.cache.Alerts(email, d.settings.Settings().StaleYears)
	return alerts.Stale, alerts.Privacy
}

func (d calendarDirectory) ClassroomColors() map[string]string {
	return d.settings.Settings().ClassroomColors
}

func secure(domain string, next http.Handler) http.Handler {
	csp := policy(domain)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == reportPath {
			report(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		if domain == Domain {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(self), geolocation=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/fonts/") || strings.HasPrefix(r.URL.Path, "/brand/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

func serveFrom(roots []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			rel := filepath.FromSlash(path.Clean(r.URL.Path))
			for _, root := range roots {
				name := filepath.Join(root, rel)
				if info, err := os.Stat(name); err == nil && info.Mode().IsRegular() {
					serve.File(w, r, name)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func Public(app string, next http.Handler) http.Handler {
	return serveFrom([]string{"web/public/" + app, "web/public/common"}, next)
}

func Files(app string, next http.Handler) http.Handler {
	return serveFrom([]string{"web/" + app, "web/common"}, next)
}

func Logged(app string, next http.Handler) http.Handler {
	return logging.Requests(app, blob.Media, next)
}

func route(domain string, apps map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, _ := strings.Cut(r.Host, ":")
		app, ok := apps[appFor(domain, host)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if canonical := canonicalHost(domain, host); canonical != "" && redirectable(r) {
			if port != "" {
				canonical += ":" + port
			}
			http.Redirect(w, r, "https://"+canonical+r.URL.RequestURI(), http.StatusMovedPermanently)
			return
		}
		app.ServeHTTP(w, r)
	})
}

func canonicalHost(domain, host string) string {
	switch host {
	case "calendar." + domain, "cal." + domain:
		return "when." + domain
	}
	return ""
}

func redirectable(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	return !strings.HasPrefix(r.URL.Path, "/api/") && !strings.HasPrefix(r.URL.Path, "/open/feed/")
}

func Port() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}

type Config struct {
	Source        data.Source
	Writer        data.Writer
	Geocoder      who.Geocoder
	Blobs         who.BlobChecker
	Store         *blob.Store
	FamilyIDKey   []byte
	BrowserKey    string
	ImageSearch   imagesearch.Search
	Mail          mail.Sender
	MailFrom      string
	CelebrateMail mail.Sender
	CelebrateFrom string
	CalendarMail  calendar.Mail
	WhoMail       mail.Sender
	BirthdayMail  mail.Sender
	BirthdayFrom  string
	BirthdayBase  string
	FeedbackFiler feedback.IssueFiler
	FeedbackBase  string
	Describer     birthday.Describer
	Loop          loop.Mail
	LoopDescriber loop.Describer
	Asker         ask.Responder
	Artifacts     func(previous *artifacts.Model) (*artifacts.Model, error)
	Embedder      artifacts.Embedder
	ArtifactsMail artifacts.Inbox
}

type Core struct {
	Mux            *http.ServeMux
	HomeMux        *http.ServeMux
	HomeCache      *home.Cache
	TeamMux        *http.ServeMux
	TeamCache      *team.Cache
	BirthdayMux    *http.ServeMux
	CelebrateMux   *http.ServeMux
	CelebrateCache *celebrate.Cache
	CalendarMux    *http.ServeMux
	CalendarCache  *calendar.Cache
	CalendarLinked func(email string) []calendar.Linked
	LoopMux        *http.ServeMux
	LoopCache      *loop.Cache
	AskMux         *http.ServeMux
	Cache          *who.Cache
	Queue          *who.Queue
	Spoof          *auth.Spoof
	Member         func(email string) bool
	Home           http.Handler
	Team           http.Handler
	Birthday       http.Handler
	Celebrate      http.Handler
	Calendar       http.Handler
	Loop           http.Handler
	Ask            http.Handler
}

func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		logging.Fatal("register manifest mime type", "error", err)
	}
	queue := who.NewQueue()
	settings, err := config.NewCache(cfg.Source, queue)
	if err != nil {
		logging.Fatal("load config", "error", err)
	}
	superAdmin := func(email string) bool {
		return slices.Contains(settings.SuperAdmins(), strings.ToLower(strings.TrimSpace(email)))
	}
	homeCache, err := home.NewCache(cfg.Source, homeImages{cfg.Store}, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load apps data", "error", err)
	}
	taglineOf := func(key string) func() string {
		return func() string {
			for _, a := range homeCache.AppList() {
				if a.Key == key {
					return a.Tagline
				}
			}
			return ""
		}
	}
	appName := func(key string) func() string {
		return func() string {
			if key == "home" {
				return "Heliosian"
			}
			for _, a := range homeCache.AppList() {
				if a.Key == key {
					return a.Name
				}
			}
			return key
		}
	}
	celebrate.ShareTagline(taglineOf("celebrate"))
	calendar.ShareTagline(taglineOf("calendar"))
	team.ShareWords(appName("team"), taglineOf("team"))
	who.ShareWords(appName("who"), taglineOf("who"))
	birthday.ShareWords(appName("birthday"), taglineOf("birthday"))
	loop.ShareWords(appName("loop"), taglineOf("loop"))
	teamCache, err := team.NewCache(cfg.Source, teamImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		slog.Error("load team data", "error", err)
	}
	birthdayCache, err := birthday.NewCache(cfg.Source, superAdmin, queue)
	if err != nil {
		logging.Fatal("load birthdays data", "error", err)
	}
	celebrateCache, err := celebrate.NewCache(cfg.Source, celebrateImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		slog.Error("load celebrate data", "error", err)
	}
	cache, err := who.NewCache(cfg.Source, cfg.Writer, cfg.Geocoder, cfg.Blobs, staticFiles{}, cfg.Store, queue, cfg.FamilyIDKey, settings.SuperAdmins)
	if err != nil {
		logging.Fatal("load directory data", "error", err)
	}
	calendarCache, err := calendar.NewCache(cfg.Source, func() calendar.Roster { return CalendarRoster(cache.Model()) }, calendarImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		logging.Fatal("load calendar data", "error", err)
	}
	loopCache, err := loop.NewCache(cfg.Source, superAdmin, queue)
	if err != nil {
		logging.Fatal("load loop data", "error", err)
	}
	loopDir := loopDirectory{cache, settings, teamCache, celebrateCache}
	artifactsCache, err := artifacts.NewCache(cfg.Artifacts, queue)
	if err != nil {
		logging.Fatal("load artifacts data", "error", err)
	}
	mux := http.NewServeMux()
	config.Register(mux, settings, cfg.Writer, cache.IsAdmin)
	who.Register(mux, cache, cfg.BrowserKey, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir})
	who.RegisterTags(mux, cache, cfg.Writer, queue, cfg.WhoMail)
	who.RegisterAdmin(mux, cache, cfg.Writer, queue)
	if err := who.RegisterInvites(mux, cache, cfg.Source, cfg.Writer); err != nil {
		logging.Fatal("load invites data", "error", err)
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	linked := calendarLinked{celebrateCache, teamCache, celebrateDirectory{cache, settings}}.list
	calendarDir := calendarDirectory{cache, settings, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir}.Lists}
	frontEvents := upcomingEvents{calendarCache, calendarDir, linked}
	calendarMux := http.NewServeMux()
	hooks := calendar.Register(calendarMux, calendarCache, cfg.Writer, queue, cfg.Store, calendarDir, settings.SuperAdmins, linked, partyPeople{celebrateCache}.people, audienceSources{loopDir}.Sources, cfg.ImageSearch, cfg.CalendarMail)
	homeMux := http.NewServeMux()
	home.Register(homeMux, homeCache, cfg.Writer, queue, cfg.Store, settings.SuperAdmins, cache.HeroPhoto, directory{cache, settings}.HomePeople, audienceSources{loopDir}, directory{cache, settings}.Alerts, frontEvents.list, frontEvents.month, cfg.ImageSearch, hooks.Answer, hooks.MakeDefault)
	teamMux := http.NewServeMux()
	eventRSVPs := func(id string) *team.EventRSVPs {
		sent, answers, ok := calendarCache.LinkedRSVPs(linked(""), calendar.SourceTeam, id)
		if !ok {
			return nil
		}
		return &team.EventRSVPs{Sent: sent, Answers: answers}
	}
	team.Register(teamMux, teamCache, cfg.Writer, queue, cfg.Store, directory{cache, settings}, settings.SuperAdmins, cfg.ImageSearch, cfg.Mail, cfg.MailFrom, eventRSVPs)
	birthdayMux := http.NewServeMux()
	birthday.Register(birthdayMux, birthdayCache, cfg.Writer, queue, cfg.Store, birthdayDirectory{cache, settings}, settings.SuperAdmins, cfg.Describer, cfg.BirthdayMail, cfg.BirthdayFrom, cfg.BirthdayBase, func(email string) error {
		return home.Grant(homeCache, cfg.Writer, queue, "birthday", email)
	})
	celebrateMux := http.NewServeMux()
	partyRSVPs := func(partyID string) *celebrate.PartyRSVPs {
		sent, answers, ok := calendarCache.PartyRSVPs(partyID)
		if !ok {
			return nil
		}
		return &celebrate.PartyRSVPs{Sent: sent, Answers: answers}
	}
	celebrate.Register(celebrateMux, celebrateCache, cfg.Writer, queue, cfg.Store, celebrateDirectory{cache, settings}, settings.SuperAdmins, cfg.ImageSearch, cfg.CelebrateMail, cfg.CelebrateFrom, partyRSVPs)
	askMux := http.NewServeMux()
	loopMail := cfg.Loop
	loopMail.Documents = artifacts.Register(askMux, artifactsCache, cfg.Embedder, cfg.Writer, queue, cfg.ArtifactsMail)
	loopMux := http.NewServeMux()
	loop.Register(loopMux, loopCache, cfg.Writer, queue, cfg.Store, loopDir, settings.SuperAdmins, loopMail, cfg.LoopDescriber)
	ask.Register(askMux, askSources(cache, settings, teamCache, celebrateCache, calendarCache, loopCache, homeCache, artifactsCache, cfg.Embedder, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir}, loopDir, linked), cfg.Asker, spend)
	for _, m := range []*http.ServeMux{mux, teamMux, birthdayMux, celebrateMux, calendarMux, loopMux, askMux} {
		home.RegisterSwitch(m, homeCache)
	}
	feedbackStore, err := feedback.NewStore(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load feedback model", "error", err)
	}
	notifier := feedback.Notifier{Sender: cfg.Mail, From: cfg.MailFrom, Base: cfg.FeedbackBase, SuperAdmins: settings.SuperAdmins}
	feedbackQueue := feedback.NewQueue(feedbackStore, notifier.Notify)
	optIn := who.OptInForm(func() string { return settings.Settings().PrivacyLinks.HeliosWhoOptIn })
	for key, m := range map[string]*http.ServeMux{"who": mux, "home": homeMux, "team": teamMux, "birthday": birthdayMux, "celebrate": celebrateMux, "calendar": calendarMux, "loop": loopMux, "ask": askMux} {
		m.Handle("GET /optin", optIn)
		feedback.Register(m, key, appName(key), superAdmin, feedbackQueue)
		if s, ok := cfg.Geocoder.(geocode.Suggester); ok {
			geocode.RegisterSuggest(m, s)
		}
	}
	feedback.RegisterAdmin(homeMux, feedbackStore, cfg.FeedbackFiler, superAdmin)
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		for _, c := range []interface{ Refresh() }{settings, homeCache, teamCache, birthdayCache, celebrateCache, cache, calendarCache, loopCache, artifactsCache} {
			c.Refresh()
		}
	})
	return &Core{
		Mux: mux, HomeMux: homeMux, HomeCache: homeCache, TeamMux: teamMux, TeamCache: teamCache, BirthdayMux: birthdayMux, CelebrateMux: celebrateMux, CelebrateCache: celebrateCache,
		CalendarMux: calendarMux, CalendarCache: calendarCache, CalendarLinked: linked, LoopMux: loopMux, LoopCache: loopCache, AskMux: askMux, Cache: cache, Queue: queue,
		Spoof:  &auth.Spoof{Allowed: superAdmin, Person: directory{cache, settings}.SpoofPerson, People: directory{cache, settings}.SpoofPeople},
		Member: func(email string) bool { return who.Member(cache, email) }, Home: homeMux, Team: teamMux, Birthday: birthdayMux, Celebrate: celebrateMux, Calendar: calendarMux, Loop: loopMux, Ask: askMux,
	}
}

func (c *Core) Muxes() map[string]*http.ServeMux {
	return map[string]*http.ServeMux{"who": c.Mux, "home": c.HomeMux, "team": c.TeamMux, "birthday": c.BirthdayMux, "celebrate": c.CelebrateMux, "calendar": c.CalendarMux, "loop": c.LoopMux, "ask": c.AskMux}
}

func Server(domain string, apps map[string]http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Addr: ":" + Port(), Handler: secure(domain, cacheControl(route(domain, apps))), Protocols: protocols}
}

func Serve(server *http.Server, queue *who.Queue) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-stop
		slog.Info("shutting down")
		if err := server.Shutdown(context.Background()); err != nil {
			slog.Error("shutdown", "error", err)
		}
	}()
	slog.Info("listening", "port", Port())
	if err := ListenAndServe(server); err != http.ErrServerClosed {
		logging.Fatal("serve", "error", err)
	}
	<-queue.Drain()
	slog.Info("queue drained")
}

func ListenAndServe(server *http.Server) error {
	if server.TLSConfig != nil {
		return server.ListenAndServeTLS("", "")
	}
	return server.ListenAndServe()
}

func requiredEnv(name string) string {
	value := os.Getenv(name)
	if value == "" {
		logging.Fatal("environment variable is required", "name", name)
	}
	return value
}

func clientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("local/creds/oauth-client.json")
	if err != nil {
		logging.Fatal("read local/creds/oauth-client.json (or set GOOGLE_CLIENT_ID)", "error", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		logging.Fatal("local/creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

func mailFrom() string {
	if from := os.Getenv("MAIL_FROM"); from != "" {
		return from
	}
	return "HCA-Team <team@loop.heliosian.com>"
}

func birthdayMailFrom() string {
	if from := os.Getenv("BIRTHDAY_MAIL_FROM"); from != "" {
		return from
	}
	return "Helios Staff Birthdays <birthday@reply.heliosian.com>"
}

func birthdayBase() string {
	if base := os.Getenv("BIRTHDAY_BASE_URL"); base != "" {
		return strings.TrimSuffix(base, "/")
	}
	return "https://birthday.heliosian.com"
}

func feedbackBase() string {
	if base := os.Getenv("FEEDBACK_BASE_URL"); base != "" {
		return strings.TrimSuffix(base, "/")
	}
	return "https://heliosian.com"
}

func githubApp() feedback.IssueFiler {
	app, err := feedback.NewGitHubApp(mapsKey("GITHUB_APP_ID", "local/creds/github-app.id"), mapsKey("GITHUB_APP_KEY", "local/creds/github-app.pem"))
	if err != nil {
		logging.Fatal("read the github app key", "error", err)
	}
	return app
}

func calendarMailFrom() string {
	if from := os.Getenv("CALENDAR_MAIL_FROM"); from != "" {
		return from
	}
	return "Helios When <when@reply.heliosian.com>"
}

func calendarReplyTo() string {
	if to := os.Getenv("CALENDAR_REPLY_TO"); to != "" {
		return to
	}
	return "Helios When <when@reply.heliosian.com>"
}

func calendarMail(sessionKey string) calendar.Mail {
	m := calendar.Mail{Sender: newMailer(calendarMailFrom()), From: calendarMailFrom(), ReplyTo: calendarReplyTo(), SigningKey: mailgunSigningKey(), Key: []byte(sessionKey)}
	if key := mailgunKey(); key != "" {
		m.Store = mail.NewMailgun(key, "")
	}
	return m
}

func mailgunKey() string {
	return optionalKey("MAILGUN_KEY", "local/creds/mailgun.key")
}

func mailgunSigningKey() string {
	return optionalKey("MAILGUN_WEBHOOK_KEY", "local/creds/mailgun-webhook.key")
}

func whoMailFrom() string {
	if from := os.Getenv("WHO_MAIL_FROM"); from != "" {
		return from
	}
	return "Helios Who? <who@reply.heliosian.com>"
}

func celebrateMailFrom() string {
	if from := os.Getenv("CELEBRATE_MAIL_FROM"); from != "" {
		return from
	}
	return "Helios Celebrate <celebrate@reply.heliosian.com>"
}

func newMailer(from string) mail.Sender {
	return mail.New(mailgunKey(), from, "")
}

func loopMail(sessionKey string) loop.Mail {
	archive, err := blob.NewArchive(blob.MailBucket)
	if err != nil {
		logging.Fatal("mail archive", "error", err)
	}
	m := loop.Mail{SigningKey: mailgunSigningKey(), Key: []byte(sessionKey), Base: "https://loop.heliosian.com", Archive: archive}
	if key := mailgunKey(); key != "" {
		mailgun := mail.NewMailgun(key, "")
		m.Sender = mailgun
		m.Store = mailgun
	}
	return m
}

func artifactsMail(store *blob.Store) artifacts.Inbox {
	m := artifacts.Inbox{SigningKey: mailgunSigningKey(), Bucket: store}
	if key := mailgunKey(); key != "" {
		m.Store = mail.NewMailgun(key, "")
	}
	return m
}

// A nil *describe.Describer must stay a nil interface, or the app would call it.
func ClaudeDescriber() birthday.Describer {
	if d := describe.New(optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeGroupDescriber() loop.Describer {
	if d := describe.New(optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"), spend); d != nil {
		return d
	}
	return nil
}

func ClaudeAsker() ask.Responder {
	if key := optionalKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"); key != "" {
		return ask.NewClaude(key)
	}
	return nil
}

func ImageSearchKeys() imagesearch.Search {
	return imagesearch.Search{
		Unsplash: optionalKey("UNSPLASH_KEY", "local/creds/unsplash.key"),
		Pexels:   optionalKey("PEXELS_KEY", "local/creds/pexels.key"),
		Pixabay:  optionalKey("PIXABAY_KEY", "local/creds/pixabay.key"),
	}
}

func optionalKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func mapsKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		logging.Fatal("read key file", "file", file, "or set", envName, "error", err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		logging.Fatal("key file is empty", "file", file)
	}
	return key
}

func Production(domain, blobCache string) (*http.Server, *who.Queue) {
	spreadsheets := map[string]string{
		"directory":   requiredEnv("DIRECTORY_SHEET"),
		"preferences": requiredEnv("PREFERENCES_SHEET"),
		"invites":     requiredEnv("INVITES_SHEET"),
		"apps":        requiredEnv("APPS_SHEET"),
		"events":      requiredEnv("EVENTS_SHEET"),
		"birthdays":   requiredEnv("BIRTHDAY_SHEET"),
		"celebrate":   requiredEnv("CELEBRATE_SHEET"),
		"calendar":    requiredEnv("CALENDAR_SHEET"),
		"config":      requiredEnv("CONFIG_SHEET"),
		"groups":      requiredEnv("GROUPS_SHEET"),
		"artifacts":   requiredEnv("ARTIFACTS_SHEET"),
		"feedback":    requiredEnv("FEEDBACK_SHEET"),
	}
	sessionKey := requiredEnv("SESSION_KEY")
	sheet, err := data.NewSheet(spreadsheets)
	if err != nil {
		logging.Fatal("load directory sheet", "error", err)
	}
	store, err := blob.New(blobCache)
	if err != nil {
		logging.Fatal("blob store", "error", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		logging.Fatal("vertex embedder", "error", err)
	}
	familyIDKey := hmac.New(sha256.New, []byte(sessionKey))
	familyIDKey.Write([]byte("family id"))
	core := NewCore(Config{
		Source:        sheet,
		Writer:        sheet,
		Geocoder:      geocode.New(mapsKey("GOOGLE_MAPS_SERVER_KEY", "local/creds/geocoding.key")),
		Blobs:         store,
		Store:         store,
		FamilyIDKey:   familyIDKey.Sum(nil),
		BrowserKey:    mapsKey("GOOGLE_MAPS_BROWSER_KEY", "local/creds/maps.key"),
		ImageSearch:   ImageSearchKeys(),
		Describer:     ClaudeDescriber(),
		Mail:          newMailer(mailFrom()),
		MailFrom:      mailFrom(),
		WhoMail:       newMailer(whoMailFrom()),
		CelebrateMail: newMailer(celebrateMailFrom()),
		CelebrateFrom: celebrateMailFrom(),
		CalendarMail:  calendarMail(sessionKey),
		BirthdayMail:  newMailer(birthdayMailFrom()),
		BirthdayFrom:  birthdayMailFrom(),
		BirthdayBase:  birthdayBase(),
		FeedbackFiler: githubApp(),
		FeedbackBase:  feedbackBase(),
		Loop:          loopMail(sessionKey),
		LoopDescriber: ClaudeGroupDescriber(),
		Asker:         ask.NewClaude(mapsKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key")),
		Artifacts: func(previous *artifacts.Model) (*artifacts.Model, error) {
			return artifacts.Load(sheet, store, embedder, previous)
		},
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(store),
	})
	blob.Register(core.Mux, store)
	blob.RegisterHome(core.HomeMux, store)
	blob.RegisterTeam(core.TeamMux, store)
	blob.RegisterBirthday(core.BirthdayMux, store)
	blob.RegisterCelebrate(core.CelebrateMux, store)
	blob.RegisterCalendar(core.CalendarMux, store)
	blob.RegisterLoop(core.LoopMux, store)
	blob.RegisterAsk(core.AskMux, store)
	who.RegisterUpload(core.Mux, core.Cache, sheet, store, core.Queue)
	client := clientID()
	newAuth := func(app string) *auth.Auth {
		a := auth.New(domain, client, []byte(sessionKey), "web/public/"+app+"/login.html", core.Member)
		a.Spoof = core.Spoof
		return a
	}
	whoAuth := newAuth("who")
	whoAuth.Preview = who.PreviewHead()
	whoAuth.Register(core.Mux)
	homeAuth := newAuth("home")
	homeAuth.Preview = home.PreviewHead(core.HomeCache)
	homeAuth.Register(core.HomeMux)
	teamAuth := newAuth("team")
	teamAuth.Preview = team.PreviewHead(core.TeamCache)
	teamAuth.Register(core.TeamMux)
	birthdayAuth := newAuth("birthday")
	birthdayAuth.Preview = birthday.PreviewHead()
	birthdayAuth.Register(core.BirthdayMux)
	celebrateAuth := newAuth("celebrate")
	celebrateAuth.Preview = celebrate.PreviewHead(core.CelebrateCache)
	celebrateAuth.Register(core.CelebrateMux)
	calendarAuth := newAuth("calendar")
	calendarAuth.Preview = calendar.PreviewHead(core.CalendarCache, core.CalendarLinked)
	calendarAuth.Register(core.CalendarMux)
	loopAuth := newAuth("loop")
	loopAuth.Preview = loop.PreviewHead()
	loopAuth.Register(core.LoopMux)
	askAuth := newAuth("ask")
	askAuth.Register(core.AskMux)
	server := Server(domain, map[string]http.Handler{
		"who":       Public("who", whoAuth.Wrap(Logged("who", Files("who", core.Mux)))),
		"home":      Public("home", homeAuth.Wrap(Logged("home", Files("home", core.Home)))),
		"team":      Public("team", team.Redirected(core.TeamCache, teamAuth.Wrap(Logged("team", Files("team", core.Team))))),
		"birthday":  Public("birthday", birthdayAuth.Wrap(Logged("birthday", Files("birthday", core.Birthday)))),
		"celebrate": Public("celebrate", celebrateAuth.Wrap(Logged("celebrate", Files("celebrate", core.Celebrate)))),
		"calendar":  Public("calendar", calendarAuth.Wrap(Logged("calendar", Files("calendar", core.Calendar)))),
		"loop":      Public("loop", loopAuth.Wrap(Logged("loop", Files("loop", core.Loop)))),
		"ask":       Public("ask", askAuth.Wrap(Logged("ask", Files("ask", core.Ask)))),
	})
	if os.Getenv("K_SERVICE") != "" {
		watcher := calendarWatcher(sheet, spreadsheets["calendar"], core, sessionKey)
		core.CalendarMux.Handle("POST "+calendarimport.HookPath, watcher)
		watcher.Start()
		server.RegisterOnShutdown(func() { core.Queue.Add(watcher.Stop) })
	}
	return server, core.Queue
}

func calendarWatcher(sheet *data.Sheet, calendarSheet string, core *Core, sessionKey string) *calendarimport.Watcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	opts := calendarimport.Options{
		Source: sheet, Sheets: sheet.Service(), Calendar: cal, CalendarSheet: calendarSheet,
		Roster:       func() calendar.Roster { return CalendarRoster(core.Cache.Model()) },
		AnthropicKey: mapsKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"),
	}
	return calendarimport.NewWatcher(opts, hex.EncodeToString(mac.Sum(nil)), core.CalendarCache.Refresh)
}
