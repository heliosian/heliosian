package app

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	entryScript = regexp.MustCompile(`<script type="module" src="([^"]+)"`)
	preloaded   = regexp.MustCompile(`<link rel="modulepreload" href="([^"]+)"`)
	bareImport  = regexp.MustCompile(`(?m)^\s*import\s*['"]([^'"]+)['"]`)
	fromImport  = regexp.MustCompile(`(?m)^\s*(?:import|export)\b[^;'"]*?\bfrom\s*['"]([^'"]+)['"]`)
)

func resolve(from, ref string) string {
	if strings.HasPrefix(ref, "/") {
		return ref
	}
	return path.Join(path.Dir(from), ref)
}

func moduleFile(app, url string) (string, bool) {
	for _, root := range []string{filepath.Join("../../web/public", app), "../../web/public/common", filepath.Join("../../web", app), "../../web/common"} {
		name := filepath.Join(root, filepath.FromSlash(url))
		if info, err := os.Stat(name); err == nil && info.Mode().IsRegular() {
			return name, true
		}
	}
	return "", false
}

func moduleGraph(t *testing.T, app, entry string) []string {
	seen := map[string]bool{}
	for pending := []string{entry}; len(pending) > 0; {
		url := pending[0]
		pending = pending[1:]
		if seen[url] {
			continue
		}
		seen[url] = true
		name, ok := moduleFile(app, url)
		if !ok {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []*regexp.Regexp{bareImport, fromImport} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				if strings.Contains(m[1], "://") {
					continue
				}
				pending = append(pending, resolve(url, m[1]))
			}
		}
	}
	return slices.Sorted(func(yield func(string) bool) {
		for url := range seen {
			if !yield(url) {
				return
			}
		}
	})
}

func TestEveryPagePreloadsItsWholeModuleGraph(t *testing.T) {
	pages, err := filepath.Glob("../../web/*/index.html")
	if err != nil {
		t.Fatal(err)
	}
	more, err := filepath.Glob("../../web/*/*/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range append(pages, more...) {
		rel, _ := filepath.Rel("../../web", page)
		parts := strings.Split(filepath.ToSlash(rel), "/")
		app, at := parts[0], "/"+strings.Join(append(parts[1:len(parts)-1], "index.html"), "/")
		html, err := os.ReadFile(page)
		if err != nil {
			t.Fatal(err)
		}
		listed := map[string]bool{}
		for _, m := range preloaded.FindAllStringSubmatch(string(html), -1) {
			listed[resolve(at, m[1])] = true
		}
		for _, m := range entryScript.FindAllStringSubmatch(string(html), -1) {
			entry := resolve(at, m[1])
			missing := []string{}
			for _, url := range moduleGraph(t, app, entry) {
				if url != entry && !listed[url] {
					missing = append(missing, url)
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s does not preload %d modules %s imports: %s", rel, len(missing), entry, strings.Join(missing, " "))
			}
		}
	}
}
