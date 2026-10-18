package db

import (
	"net/http"
	"net/url"
	"strings"

	"heliosian/internal/store"
)

const maxHops = 20

func (a *ActivityPages) Redirected(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		tr := a.tree()
		to, status := tr.canonical(r.URL.Path), http.StatusMovedPermanently
		if to == "" {
			to, status = tr.destination(r.URL.Path), http.StatusFound
		}
		if to == "" {
			next.ServeHTTP(w, r)
			return
		}
		redirectTo(w, r, to, status)
	})
}

func redirectTo(w http.ResponseWriter, r *http.Request, to string, status int) {
	if strings.HasPrefix(to, "/") {
		to = "https://" + r.Host + to
	}
	if r.URL.RawQuery != "" && !strings.Contains(to, "?") {
		to += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, to, status)
}

func (t activityTree) canonical(path string) string {
	at := redirectPath(path)
	g := t.at(at)
	if g == nil || t.path(g) == at {
		return ""
	}
	return t.path(g)
}

func (t activityTree) resolve(path string) store.Row {
	if g := t.at(redirectPath(path)); g != nil {
		return g
	}
	if to := t.destination(path); to != "" && !isURL(to) {
		return t.at(to)
	}
	return nil
}

func (t activityTree) destination(path string) string {
	start := redirectPath(path)
	if start == "" || t.at(start) != nil {
		return ""
	}
	at, seen := start, map[string]bool{strings.ToLower(start): true}
	for range maxHops {
		next := t.moved(at)
		if next == "" {
			break
		}
		if isURL(next) {
			return next
		}
		if g := t.at(next); g != nil {
			return t.path(g)
		}
		if seen[strings.ToLower(next)] {
			return ""
		}
		seen[strings.ToLower(next)] = true
		at = next
	}
	if at == start || !onSite(at) {
		return ""
	}
	return at
}

func (t activityTree) moved(at string) string {
	moved, matched := "", ""
	for _, r := range t.m.Table("REDIRECT").All() {
		if r["app"] != "team" {
			continue
		}
		old, to := redirectPath(r["old"]), redirectTarget(r["new"])
		if old == "" || to == "" {
			continue
		}
		if strings.EqualFold(old, at) {
			moved, matched = to, at
		} else if strings.HasPrefix(strings.ToLower(at), strings.ToLower(old)+"/") && len(old) > len(matched) {
			moved, matched = strings.TrimSuffix(to, "/")+at[len(old):], old
		}
	}
	return moved
}

func redirectPath(cell string) string {
	path := strings.TrimSpace(cell)
	if path == "" {
		return ""
	}
	if isURL(path) {
		u, err := url.Parse(path)
		if err != nil {
			return ""
		}
		path = u.Path
		if path == "" {
			path = "/"
		}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/g/" + path
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func redirectTarget(cell string) string {
	if to := strings.TrimSpace(cell); isURL(to) {
		return to
	}
	return redirectPath(cell)
}

func isURL(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "http://") || strings.HasPrefix(strings.ToLower(s), "https://")
}

func onSite(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.HasPrefix(path, "/\\")
}
