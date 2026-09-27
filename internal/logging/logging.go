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
	as    string
	trace string
	span  string
}

type handler struct {
	inner slog.Handler
}

func Fatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
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
	if req.as != "" {
		record.AddAttrs(slog.String("as", req.as))
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

func Cloud() *slog.Logger {
	return slog.New(handler{slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: cloudKeys})})
}

func Console() *slog.Logger {
	return slog.New(handler{slog.NewTextHandler(os.Stderr, nil)})
}

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

func Requests(app string, media func(path string) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		req := request{app: app, user: strings.ToLower(auth.RealEmail(r)), as: strings.ToLower(auth.Spoofing(r))}
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
