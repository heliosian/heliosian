package app

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"heliosian/internal/serve"
)

const reportPath = "/csp-report"

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

func report(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Report struct {
			Document   string `json:"document-uri"`
			Directive  string `json:"effective-directive"`
			Blocked    string `json:"blocked-uri"`
			SourceFile string `json:"source-file"`
			Line       int    `json:"line-number"`
		} `json:"csp-report"`
	}
	if !serve.Decode(w, r, &body) {
		return
	}
	v := body.Report
	slog.Warn("csp violation", "document", reportOrigin(v.Document), "directive", v.Directive, "blocked", reportOrigin(v.Blocked), "source", reportOrigin(v.SourceFile), "line", v.Line)
	w.WriteHeader(http.StatusNoContent)
}

func reportOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host
	}
	if u.Scheme != "" {
		return u.Scheme + ":"
	}
	return ""
}
