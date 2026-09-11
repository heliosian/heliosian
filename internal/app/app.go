// Package app wires and serves the Helios community apps: a mode-free core every
// server shares, the host routing that picks an app per request, and the
// production assembly. Nothing here knows about sample data or any other dev
// convenience - that composition lives under cmd/.
package app

import (
	"context"
	"encoding/json"
	"log/slog"
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

	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/events"
	"heliosian/internal/geocode"
	"heliosian/internal/home"
	"heliosian/internal/logging"
	"heliosian/internal/serve"
	"heliosian/internal/who"
)

// appFor reads the app out of a hostname: <app>.heliosian.com in production,
// <app>.lab.heliosian.com hosted alongside it, <app>.local.heliosian.com on a
// developer's machine. Home also answers as the bare and www apex.
func appFor(host string) string {
	switch host {
	case "heliosian.com", "www.heliosian.com":
		return "home"
	}
	name, ok := strings.CutSuffix(host, ".heliosian.com")
	if !ok {
		return ""
	}
	app, tier, _ := strings.Cut(name, ".")
	if tier != "" && tier != "lab" && tier != "local" {
		return ""
	}
	return app
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

// uploaded asks the bucket for a name the sheet records; with no bucket, as in
// sample mode, nothing uploaded exists.
func uploaded(store *blob.Store, key string) (bool, error) {
	if store == nil {
		return false, nil
	}
	return store.Has(key)
}

// prefetchUploaded hands the bucket every recorded name under its folder at
// once, so the fetches run side by side; bundled names are left alone.
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

// homeImages resolves the Image cells of the Apps sheet: an uploaded object in
// the bucket, or a bundled file under home's own served trees.
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

// eventsImages resolves the Image cells of the Events sheet the same way: an
// uploaded object in the bucket, or a bundled file under the portal's own trees.
type eventsImages struct {
	store *blob.Store
}

func (e eventsImages) Has(key string) (bool, error) {
	if strings.HasPrefix(key, "activity-images/") {
		return uploaded(e.store, key)
	}
	return bundled([]string{"web/hca", "web/public/hca"}, key), nil
}

func (e eventsImages) Prefetch(names []string) error {
	return prefetchUploaded(e.store, "activity-images/", names)
}

// directory hands the volunteer portal the directory's view of a person: the
// address they are keyed by, and their name and photo.
type directory struct {
	cache *who.Cache
}

func (d directory) Resolve(email string) string {
	return d.cache.Model().Resolve(email)
}

func (d directory) Person(email string) (string, string, bool) {
	p := d.cache.Model().Person(email)
	if p == nil {
		return "", "", false
	}
	return p.FullName, p.PhotoURL, true
}

// People is the directory as a picker sees it: everyone, with the one word that
// places them - a staff member's job, a student's grade, or "Parent".
func (d directory) People() []events.DirectoryPerson {
	model := d.cache.Model()
	out := make([]events.DirectoryPerson, 0, len(model.People))
	for _, p := range model.People {
		title := ""
		switch {
		case p.IsStaff:
			title = p.JobTitle
			if title == "" {
				title = "Staff"
			}
		case p.IsStudent:
			title = p.Grade
			if title == "" {
				title = "Student"
			}
		case p.IsParent:
			title = "Parent"
		}
		person := events.DirectoryPerson{
			Email: p.Email, Name: p.FullName, PhotoURL: model.HeroPhoto(p.Email), Title: title,
			IsStudent: p.IsStudent, ParentEmails: p.ParentContactEmails,
			Pronouns: p.Pronouns, Phone: p.Phone, Grade: p.Grade, Classroom: p.Classroom,
			JobTitle: p.JobTitle, Department: p.Department,
		}
		// A parent's children come through the household, as the directory
		// itself lists them.
		if p.IsParent {
			for _, key := range model.FamilyKeysOf(p.Email) {
				for _, kid := range model.Families[key].KidEmails {
					if k := model.Person(kid); k != nil {
						person.Children = append(person.Children, events.Child{Name: k.FullName, Grade: k.Grade})
					}
				}
			}
		}
		out = append(out, person)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// birthdayDirectory hands the birthday app the directory's view of people: who
// an address resolves to, what is known about one person, and every staff
// member, with departments in the order the directory lists them.
type birthdayDirectory struct {
	cache *who.Cache
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

func (d birthdayDirectory) Departments() []string {
	return d.cache.Model().Departments
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

// serveFrom answers a request from the first root holding a regular file at
// its path, else hands it on.
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

// Public serves web/public/<app>/ then web/public/common/ ahead of sign-in:
// the manifest, icons, splash art, and fonts that browsers fetch without
// credentials and the login page needs.
func Public(app string, next http.Handler) http.Handler {
	return serveFrom([]string{"web/public/" + app, "web/public/common"}, next)
}

// Files serves web/<app>/ then web/common/ and sits inside sign-in.
func Files(app string, next http.Handler) http.Handler {
	return serveFrom([]string{"web/" + app, "web/common"}, next)
}

// Logged sits inside sign-in too, so every record a request produces names
// the app and the user, and every request past the media routes gets one.
func Logged(app string, next http.Handler) http.Handler {
	return logging.Requests(app, blob.Media, next)
}

func route(apps map[string]http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := strings.Cut(r.Host, ":")
		app, ok := apps[appFor(host)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		app.ServeHTTP(w, r)
	})
}

// Port is the port the server listens on, shared with anything that needs to
// reach it in-process (startserver's capture mode).
func Port() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}

// Config carries the mode-specific pieces the core is assembled from. The
// caller decides what backs each one; the core never inspects the environment
// or branches on how it was built.
type Config struct {
	Source     data.Source
	Writer     data.Writer
	Geocoder   who.Geocoder
	Blobs      who.BlobChecker
	Store      *blob.Store
	BrowserKey string
}

// Core is the assembled shared skeleton: each app's mux (still open for the
// caller's mode-specific routes), the directory model cache, the shared write
// queue, and each app's handler for the caller to wrap with its authentication.
type Core struct {
	Mux         *http.ServeMux
	HomeMux     *http.ServeMux
	EventsMux   *http.ServeMux
	EventsCache *events.Cache
	BirthdayMux *http.ServeMux
	Cache       *who.Cache
	Queue       *who.Queue
	Gate        http.Handler
	Home        http.Handler
	Events      http.Handler
	Birthday    http.Handler
}

// NewCore wires everything every mode serves identically. Fatal on any failure.
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
	// The small sheets load first, so a bad column in any of them refuses the
	// start in seconds rather than after the directory has fetched every photo.
	homeCache, err := home.NewCache(cfg.Source, homeImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		logging.Fatal("load apps data", "error", err)
	}
	// The portal's sheet is edited by hand more than the others, so a load
	// failure keeps only the portal down: it answers with the reason and comes
	// back on its own once the sheet loads.
	eventsCache, err := events.NewCache(cfg.Source, eventsImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		slog.Error("load events data", "error", err)
	}
	birthdayCache, err := birthday.NewCache(cfg.Source, superAdmin, queue)
	if err != nil {
		logging.Fatal("load birthdays data", "error", err)
	}
	cache, err := who.NewCache(cfg.Source, cfg.Geocoder, cfg.Blobs, staticFiles{}, cfg.Store, queue, settings.SuperAdmins)
	if err != nil {
		logging.Fatal("load directory data", "error", err)
	}
	mux := http.NewServeMux()
	config.Register(mux, settings, cfg.Writer, cache.IsAdmin)
	who.Register(mux, cache, cfg.BrowserKey, func() string { return settings.Settings().PrivacyLinks.HeliosWhoOptIn })
	who.RegisterTags(mux, cache, cfg.Writer, queue)
	who.RegisterAdmin(mux, cache, cfg.Writer, queue)
	if err := who.RegisterInvites(mux, cache, cfg.Source, cfg.Writer); err != nil {
		logging.Fatal("load invites data", "error", err)
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	homeMux := http.NewServeMux()
	home.Register(homeMux, homeCache, cfg.Writer, queue, cfg.Store, settings.SuperAdmins, cache.HeroPhoto)
	eventsMux := http.NewServeMux()
	events.Register(eventsMux, eventsCache, cfg.Writer, queue, cfg.Store, directory{cache}, settings.SuperAdmins)
	birthdayMux := http.NewServeMux()
	birthday.Register(birthdayMux, birthdayCache, cfg.Writer, queue, birthdayDirectory{cache}, settings.SuperAdmins)
	return &Core{
		Mux: mux, HomeMux: homeMux, EventsMux: eventsMux, EventsCache: eventsCache, BirthdayMux: birthdayMux, Cache: cache, Queue: queue,
		Gate: who.MemberGate(cache, mux), Home: homeMux, Events: eventsMux, Birthday: birthdayMux,
	}
}

// Server dresses the apps, each fully wrapped and keyed by name, in the shared
// HTTP plumbing: host routing and cache headers.
func Server(apps map[string]http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Addr: ":" + Port(), Handler: cacheControl(route(apps)), Protocols: protocols}
}

// Serve runs a server until SIGTERM or interrupt, then shuts down gracefully
// and drains the write queue.
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

// ListenAndServe speaks TLS only when the server carries a TLSConfig, which
// local development sets; Cloud Run terminates TLS in front of production.
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
	raw, err := os.ReadFile("creds/oauth-client.json")
	if err != nil {
		logging.Fatal("read creds/oauth-client.json (or set GOOGLE_CLIENT_ID)", "error", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		logging.Fatal("creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
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

// Production assembles the real service: the production spreadsheets, the media
// bucket, real geocoding, and Google sign-in. Every input is required.
func Production() (*http.Server, *who.Queue) {
	spreadsheets := map[string]string{
		"directory":   requiredEnv("DIRECTORY_SHEET"),
		"preferences": requiredEnv("PREFERENCES_SHEET"),
		"invites":     requiredEnv("INVITES_SHEET"),
		"apps":        requiredEnv("APPS_SHEET"),
		"events":      requiredEnv("EVENTS_SHEET"),
		"birthdays":   requiredEnv("BIRTHDAY_SHEET"),
		"config":      requiredEnv("CONFIG_SHEET"),
	}
	sessionKey := requiredEnv("SESSION_KEY")
	sheet, err := data.NewSheet(spreadsheets)
	if err != nil {
		logging.Fatal("load directory sheet", "error", err)
	}
	store, err := blob.New()
	if err != nil {
		logging.Fatal("blob store", "error", err)
	}
	core := NewCore(Config{
		Source:     sheet,
		Writer:     sheet,
		Geocoder:   geocode.New(mapsKey("GOOGLE_MAPS_SERVER_KEY", "creds/geocoding.key")),
		Blobs:      store,
		Store:      store,
		BrowserKey: mapsKey("GOOGLE_MAPS_BROWSER_KEY", "creds/maps.key"),
	})
	blob.Register(core.Mux, store)
	blob.RegisterHome(core.HomeMux, store)
	blob.RegisterEvents(core.EventsMux, store)
	blob.RegisterBirthday(core.BirthdayMux, store)
	who.RegisterUpload(core.Mux, core.Cache, sheet, store, core.Queue)
	client := clientID()
	whoAuth := auth.New(client, []byte(sessionKey), "web/public/who/login.html")
	whoAuth.Register(core.Mux)
	homeAuth := auth.New(client, []byte(sessionKey), "web/public/home/login.html")
	homeAuth.Register(core.HomeMux)
	hcaAuth := auth.New(client, []byte(sessionKey), "web/public/hca/login.html")
	// A shared link to an event previews in chat apps: the sign-in page it
	// leads to carries the event's Open Graph tags.
	hcaAuth.Preview = events.PreviewHead(core.EventsCache)
	hcaAuth.Register(core.EventsMux)
	birthdayAuth := auth.New(client, []byte(sessionKey), "web/public/birthday/login.html")
	birthdayAuth.Register(core.BirthdayMux)
	return Server(map[string]http.Handler{
		"who":      Public("who", whoAuth.Wrap(Logged("who", Files("who", core.Gate)))),
		"home":     Public("home", homeAuth.Wrap(Logged("home", Files("home", core.Home)))),
		"hca":      Public("hca", hcaAuth.Wrap(Logged("hca", Files("hca", core.Events)))),
		"birthday": Public("birthday", birthdayAuth.Wrap(Logged("birthday", Files("birthday", core.Birthday)))),
	}), core.Queue
}
