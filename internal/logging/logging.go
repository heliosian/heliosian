// Package logging emits structured records that Cloud Logging indexes: severity, the signed-in user, the app, and the request's trace.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"heliosian/internal/auth"
)

const project = "heliosian"

type contextKey struct{}

type request struct {
	app   string
	user  string
	trace string
	span  string
}

type handler struct {
	inner slog.Handler
}

func (h handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h handler) Handle(ctx context.Context, record slog.Record) error {
	req, ok := ctx.Value(contextKey{}).(request)
	if !ok {
		return h.inner.Handle(ctx, record)
	}
	record = record.Clone()
	record.AddAttrs(slog.String("app", req.app))
	if req.user != "" {
		record.AddAttrs(slog.String("user", req.user))
	}
	if req.trace != "" {
		record.AddAttrs(
			slog.String("logging.googleapis.com/trace", req.trace),
			slog.String("logging.googleapis.com/spanId", req.span),
		)
	}
	return h.inner.Handle(ctx, record)
}

func (h handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return handler{h.inner.WithAttrs(attrs)}
}

func (h handler) WithGroup(name string) slog.Handler {
	return handler{h.inner.WithGroup(name)}
}

func severity(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	}
	return "DEBUG"
}

func cloudKeys(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.LevelKey:
		return slog.String("severity", severity(a.Value.Any().(slog.Level)))
	case slog.MessageKey:
		a.Key = "message"
	}
	return a
}

// Cloud is the production logger: one JSON object per line on stdout, keyed
// the way Cloud Logging's agent promotes severity, message, and trace.
func Cloud() *slog.Logger {
	return slog.New(handler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
}

// Console is the development logger: readable text on stderr carrying the
// same per-request fields.
func Console() *slog.Logger {
	return slog.New(handler{slog.NewTextHandler(os.Stderr, nil)})
}

// traceOf reads Cloud Run's X-Cloud-Trace-Context header, TRACE_ID/SPAN_ID;o=1,
// into the trace resource name and the hex span id Cloud Logging correlates on.
func traceOf(header string) (string, string) {
	id, rest, ok := strings.Cut(header, "/")
	if !ok || len(id) != 32 {
		return "", ""
	}
	span, _, _ := strings.Cut(rest, ";")
	n, err := strconv.ParseUint(span, 10, 64)
	if err != nil {
		return "", ""
	}
	return "projects/" + project + "/traces/" + id, fmt.Sprintf("%016x", n)
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// Requests tags every record logged while handling a request with the app, the
// signed-in user, and the request's trace, and writes one record per request
// with its status and latency. Media requests are tagged but get no record of
// their own: a photo-heavy page fans out hundreds, and Cloud Run's request log
// already lists them.
func Requests(app string, media func(path string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		req := request{app: app, user: strings.ToLower(auth.Email(r))}
		req.trace, req.span = traceOf(r.Header.Get("X-Cloud-Trace-Context"))
		ctx := context.WithValue(r.Context(), contextKey{}, req)
		r = r.WithContext(ctx)
		if media(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		rec := &recorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelInfo
		if rec.status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(ctx, level, "request", slog.Group("httpRequest",
			"requestMethod", r.Method,
			"requestUrl", r.URL.RequestURI(),
			"status", rec.status,
			"latency", fmt.Sprintf("%.6fs", time.Since(start).Seconds()),
			"userAgent", r.UserAgent(),
		))
	})
}
