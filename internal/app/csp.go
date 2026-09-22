package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const reportPath = "/csp-report"

var inlineScript = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// inlineScriptHashes is a source expression for every inline script in the
// shells under web/, read afresh at startup so an edit to one changes the
// policy with it.
func inlineScriptHashes() []string {
	seen := map[string]bool{}
	var hashes []string
	err := filepath.WalkDir("web", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(name) != ".html" {
			return nil
		}
		page, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		for _, m := range inlineScript.FindAllSubmatch(page, -1) {
			sum := sha256.Sum256(m[1])
			hash := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
			if !seen[hash] {
				seen[hash] = true
				hashes = append(hashes, hash)
			}
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return hashes
}

// policy is the content security policy for a server answering domain: the
// apps' own origins, the shells' inline scripts by hash, Google sign-in by
// the sources Google documents, the Maps JavaScript API by the hosts a
// rendered map is seen to reach - its modules, RPCs and tiles from
// maps.googleapis.com, its cursors and logo from maps.gstatic.com, and
// Google Sans for its info window and hints from Google Fonts - and the
// image search providers' thumbnails, which the pickers show straight from
// their CDNs. Styles allow inline because Maps and sign-in inject theirs.
func policy(domain string) string {
	apps := "https://*." + domain + ":*"
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' " + strings.Join(inlineScriptHashes(), " ") + " https://accounts.google.com/gsi/client https://maps.googleapis.com",
		"style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data: blob: " + apps + " https://maps.googleapis.com https://maps.gstatic.com https://images.unsplash.com https://images.pexels.com https://pixabay.com",
		"media-src 'self' blob: " + apps,
		"connect-src 'self' " + apps + " https://accounts.google.com/gsi/ https://maps.googleapis.com",
		"frame-src https://accounts.google.com/gsi/",
		"object-src 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"report-uri " + reportPath,
	}, "; ")
}

// report takes what a browser posts to report-uri and logs the page, the
// directive, what was blocked and where in the source it came from, so a
// violation shows in the server log wherever the browser is.
func report(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Report struct {
			Document   string `json:"document-uri"`
			Directive  string `json:"effective-directive"`
			Blocked    string `json:"blocked-uri"`
			SourceFile string `json:"source-file"`
			Line       int    `json:"line-number"`
			Sample     string `json:"script-sample"`
		} `json:"csp-report"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		http.Error(w, "unreadable report", http.StatusBadRequest)
		return
	}
	v := body.Report
	slog.Warn("csp violation", "document", v.Document, "directive", v.Directive, "blocked", v.Blocked, "source", v.SourceFile, "line", v.Line, "sample", v.Sample)
	w.WriteHeader(http.StatusNoContent)
}
