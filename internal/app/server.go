package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"heliosian/internal/blob"
	"heliosian/internal/home"
	"heliosian/internal/logging"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	Domain    = "heliosian.com"
	DevDomain = "heliosiandev.com"
)

var aliases = map[string]string{"hca": "team", "cal": "calendar", "when": "calendar"}

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

func Server(domain string, apps map[string]http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Addr: ":" + Port(), Handler: secure(domain, cacheControl(route(domain, apps))), Protocols: protocols}
}

func Serve(server *http.Server, queue *store.Queue) {
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
