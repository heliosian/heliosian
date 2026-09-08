// Package app wires and serves the Helios community apps: a mode-free core every
// server shares, and the production assembly. Nothing here knows about sample
// data or any other dev convenience - that composition lives under tools/.
package app

import (
	"context"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/directory"
	"heliosian/internal/geocode"
)

type staticFiles struct{}

func (staticFiles) Has(key string) bool {
	_, err := os.Stat(filepath.Join("web/static", filepath.FromSlash(key)))
	return err == nil
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/fonts/") || strings.HasPrefix(r.URL.Path, "/static/brand/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
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
	Geocoder   directory.Geocoder
	Blobs      directory.BlobChecker
	Store      *blob.Store
	BrowserKey string
}

// Core is the assembled shared skeleton: the mux (still open for the caller's
// mode-specific routes), the model cache, the write queue, and the member-gated
// handler the caller wraps with its authentication.
type Core struct {
	Mux   *http.ServeMux
	Cache *directory.Cache
	Queue *directory.Queue
	Gate  http.Handler
}

// NewCore wires everything every mode serves identically. Fatal on any failure.
func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		log.Fatalf("[ERROR] register manifest mime type: %v", err)
	}
	queue := directory.NewQueue()
	cache, err := directory.NewCache(cfg.Source, cfg.Geocoder, cfg.Blobs, staticFiles{}, cfg.Store, queue)
	if err != nil {
		log.Fatalf("[ERROR] load directory data: %v", err)
	}
	mux := http.NewServeMux()
	directory.Register(mux, cache, cfg.BrowserKey)
	directory.RegisterTags(mux, cache, cfg.Writer, queue)
	directory.RegisterAdmin(mux, cache, cfg.Writer, queue)
	directory.RegisterInvites(mux, cache, cfg.Source, cfg.Writer)
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	return &Core{Mux: mux, Cache: cache, Queue: queue, Gate: directory.MemberGate(cache, mux)}
}

// Server dresses a fully wrapped handler in the shared HTTP plumbing.
func Server(handler http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Addr: ":" + Port(), Handler: cacheControl(handler), Protocols: protocols}
}

// Serve runs a server until SIGTERM or interrupt, then shuts down gracefully
// and drains the write queue.
func Serve(server *http.Server, queue *directory.Queue) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-stop
		log.Printf("shutting down")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("[ERROR] shutdown: %v", err)
		}
	}()
	log.Printf("listening on http://localhost:%s", Port())
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
	<-queue.Drain()
	log.Printf("queue drained")
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
// bucket, real geocoding, and Google sign-in. Every input is required except
// INVITES_SHEET - the Invite List Builder templates are one optional feature,
// not the app, so its absence just leaves that feature with nothing to serve
// rather than failing every other route too.
func Production() (*http.Server, *directory.Queue) {
	sheetID := requiredEnv("DIRECTORY_SHEET")
	preferencesID := requiredEnv("PREFERENCES_SHEET")
	sessionKey := requiredEnv("SESSION_KEY")
	spreadsheets := map[string]string{"directory": sheetID, "preferences": preferencesID}
	if invitesID := os.Getenv("INVITES_SHEET"); invitesID != "" {
		spreadsheets["invites"] = invitesID
	}
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
	directory.RegisterUpload(core.Mux, core.Cache, sheet, store, core.Queue)
	authn := auth.New(clientID(), []byte(sessionKey))
	authn.Register(core.Mux)
	return Server(authn.Wrap(core.Gate)), core.Queue
}
