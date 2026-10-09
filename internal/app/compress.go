package app

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strings"
	"sync"
)

var gzips = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

func acceptsGzip(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		token, params, _ := strings.Cut(part, ";")
		if strings.EqualFold(strings.TrimSpace(token), "gzip") && strings.ReplaceAll(strings.TrimSpace(params), " ", "") != "q=0" {
			return true
		}
	}
	return false
}

func compressible(contentType string) bool {
	media, _, _ := mime.ParseMediaType(contentType)
	switch {
	case media == "text/event-stream":
		return false
	case strings.HasPrefix(media, "text/"):
		return true
	}
	switch media {
	case "application/json", "application/javascript", "application/manifest+json", "image/svg+xml":
		return true
	}
	return false
}

type gzipWriter struct {
	http.ResponseWriter
	r       *http.Request
	decided bool
	gz      *gzip.Writer
}

func (w *gzipWriter) decide(status int) {
	if w.decided {
		return
	}
	w.decided = true
	h := w.Header()
	if w.r.Method == http.MethodHead || status < http.StatusOK || status == http.StatusNoContent || status == http.StatusNotModified {
		return
	}
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" || !compressible(h.Get("Content-Type")) {
		return
	}
	h.Set("Content-Encoding", "gzip")
	h.Del("Content-Length")
	w.gz = gzips.Get().(*gzip.Writer)
	w.gz.Reset(w.ResponseWriter)
}

func (w *gzipWriter) WriteHeader(status int) {
	w.decide(status)
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	if !w.decided {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.decide(http.StatusOK)
	}
	if w.gz == nil {
		return w.ResponseWriter.Write(b)
	}
	return w.gz.Write(b)
}

func (w *gzipWriter) Flush() {
	if w.gz != nil {
		w.gz.Flush()
	}
	http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *gzipWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipWriter) close() {
	if w.gz == nil {
		return
	}
	w.gz.Close()
	gzips.Put(w.gz)
}

func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, r: r}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}
