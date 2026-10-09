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
	"heliosian/internal/logging"
	"heliosian/internal/model"
	"heliosian/internal/serve"
	"heliosian/internal/store"
)

const (
	Domain    = "heliosian.com"
	DevDomain = "heliosiandev.com"
)

type destination struct {
	app       string
	canonical string
}

func hostsFor(domain string) map[string]destination {
	out := map[string]destination{}
	for _, a := range append([]model.App{model.HomeApp}, model.Apps...) {
		out[model.Qualify(a.Hosts[0], domain)] = destination{app: a.Key}
		for _, alias := range a.Hosts[1:] {
			out[model.Qualify(alias, domain)] = destination{app: a.Key, canonical: model.Qualify(a.Hosts[0], domain)}
		}
	}
	return out
}

func Hostnames(domain string) []string {
	out := []string{}
	for host := range hostsFor(domain) {
		out = append(out, host)
	}
	slices.Sort(out)
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

func route(domain string, apps, aliased map[string]http.Handler) http.Handler {
	hosts := hostsFor(domain)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, _ := strings.Cut(r.Host, ":")
		dest := hosts[host]
		app, ok := apps[dest.app]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if dest.canonical == "" {
			app.ServeHTTP(w, r)
			return
		}
		canonical := dest.canonical
		if port != "" {
			canonical += ":" + port
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "https://"+canonical+"/", http.StatusMovedPermanently)
			return
		}
		alias, ok := aliased[dest.app]
		if !ok {
			http.NotFound(w, r)
			return
		}
		r.Host = canonical
		alias.ServeHTTP(w, r)
	})
}

func Port() string {
	if port := os.Getenv("PORT"); port != "" {
		return port
	}
	return "8080"
}

func Server(domain string, apps, aliased map[string]http.Handler) *http.Server {
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)
	return &http.Server{Addr: ":" + Port(), Handler: compress(secure(domain, cacheControl(route(domain, apps, aliased)))), Protocols: protocols}
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
