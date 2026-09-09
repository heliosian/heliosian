// Package app wires and serves the Helios community apps: a mode-free core every
// server shares, the host routing that picks an app per request, and the
// production assembly. Nothing here knows about sample data or any other dev
// convenience - that composition lives under cmd/.
package app

import (
	"context"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/geocode"
	"heliosian/internal/home"
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

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/who", filepath.FromSlash(key)))
	return err == nil
}

// homeImages resolves the Image cells of the Apps sheet: an uploaded object in
// the bucket, or a bundled file under home's own served trees.
type homeImages struct {
	store *blob.Store
}

func (h homeImages) Has(key string) bool {
	if strings.HasPrefix(key, "link-images/") {
		return h.store != nil && h.store.Has(key)
	}
	for _, root := range []string{"web/home", "web/public/home"} {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(key))); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
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
					http.ServeFile(w, r, name)
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
	Mux     *http.ServeMux
	HomeMux *http.ServeMux
	Cache   *who.Cache
	Queue   *who.Queue
	Gate    http.Handler
	Home    http.Handler
}

// NewCore wires everything every mode serves identically. Fatal on any failure.
func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Fatalf("[ERROR] register manifest mime type: %v", err)
	}
	queue := who.NewQueue()
	settings, err := config.NewCache(cfg.Source, queue)
	if err != nil {
		log.Fatalf("[ERROR] load config: %v", err)
	}
	cache, err := who.NewCache(cfg.Source, cfg.Geocoder, cfg.Blobs, staticFiles{}, cfg.Store, queue, settings.SuperAdmins)
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	config.Register(mux, settings, cfg.Writer, cache.IsAdmin)
	who.Register(mux, cache, cfg.BrowserKey, func() string { return settings.Settings().PrivacyLinks.HeliosWhoOptIn })
	who.RegisterTags(mux, cache, cfg.Writer, queue)
	who.RegisterAdmin(mux, cache, cfg.Writer, queue)
	if err := who.RegisterInvites(mux, cache, cfg.Source, cfg.Writer); err != nil {
		log.Fatalf("[ERROR] load invites data: %v", err)
	}
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	homeCache, err := home.NewCache(cfg.Source, homeImages{cfg.Store}, cache.IsSuperAdmin, queue)
	if err != nil {
		log.Fatalf("[ERROR] load apps data: %v", err)
	}
	homeMux := http.NewServeMux()
	home.Register(homeMux, homeCache, cfg.Writer, queue, cfg.Store, settings.SuperAdmins)
	return &Core{Mux: mux, HomeMux: homeMux, Cache: cache, Queue: queue, Gate: who.MemberGate(cache, mux), Home: homeMux}
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
		log.Printf("shutting down")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("[ERROR] shutdown: %v", err)
		}
	}()
	log.Printf("listening on :%s", Port())
	if err := ListenAndServe(server); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-queue.Drain()
	log.Printf("queue drained")
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
		log.Fatalf("[ERROR] %s is required", name)
	}
	return value
}

func clientID() string {
	if id := os.Getenv("GOOGLE_CLIENT_ID"); id != "" {
		return id
	}
	raw, err := os.ReadFile("creds/oauth-client.json")
	if err != nil {
		log.Fatalf("[ERROR] read creds/oauth-client.json (or set GOOGLE_CLIENT_ID): %v", err)
	}
	var parsed struct {
		Web struct {
			ClientID string `json:"client_id"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Web.ClientID == "" {
		log.Fatal("[ERROR] creds/oauth-client.json is not an oauth web client file")
	}
	return parsed.Web.ClientID
}

func mapsKey(envName, file string) string {
	if key := os.Getenv(envName); key != "" {
		return key
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		log.Fatalf("[ERROR] read %s (or set %s): %v", file, envName, err)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" {
		log.Fatalf("[ERROR] %s is empty", file)
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
		"config":      requiredEnv("CONFIG_SHEET"),
	}
	sessionKey := requiredEnv("SESSION_KEY")
	sheet, err := data.NewSheet(spreadsheets)
	if err != nil {
		log.Fatalf("[ERROR] load directory sheet: %v", err)
	}
	store, err := blob.New()
	if err != nil {
		log.Fatalf("[ERROR] blob store: %v", err)
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
	blob.RegisterLinkImages(core.HomeMux, store)
	who.RegisterUpload(core.Mux, core.Cache, sheet, store, core.Queue)
	client := clientID()
	whoAuth := auth.New(client, []byte(sessionKey), "web/public/who/login.html")
	whoAuth.Register(core.Mux)
	homeAuth := auth.New(client, []byte(sessionKey), "web/public/home/login.html")
	homeAuth.Register(core.HomeMux)
	return Server(map[string]http.Handler{
		"who":  Public("who", whoAuth.Wrap(Files("who", core.Gate))),
		"home": Public("home", homeAuth.Wrap(Files("home", core.Home))),
	}), core.Queue
}
