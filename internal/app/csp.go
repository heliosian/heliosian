package app

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

const reportPath = "/csp-report"

// policy is the content security policy for a server answering domain: the
// apps' own origins - every script is a file under web/, none inline - Google
// sign-in by the sources Google documents, the Maps JavaScript API by the
// hosts a rendered map is seen to reach - its modules, RPCs and tiles from
// maps.googleapis.com, its cursors and logo from maps.gstatic.com, and
// Google Sans for its info window and hints from Google Fonts. Styles allow
// inline because Maps and sign-in inject theirs.
func policy(domain string) string {
	apps := "https://*." + domain + ":*"
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' https://accounts.google.com/gsi/client https://maps.googleapis.com",
		"style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data: blob: " + apps + " https://maps.googleapis.com https://maps.gstatic.com",
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
