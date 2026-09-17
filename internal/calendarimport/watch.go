package calendarimport

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync"
	"time"

	gcal "google.golang.org/api/calendar/v3"

	"heliosian/internal/calendar"
)

// HookPath is where Google posts that the school's calendar changed; every inbound webhook lives under /hooks/, served with no session.
const HookPath = "/hooks/calendar"

const (
	hookAddress   = "https://when.heliosian.com" + HookPath
	sweepInterval = time.Hour
	renewWithin   = 2 * time.Hour
	channelLife   = 7 * 24 * time.Hour
)

// Watcher keeps the Google Import in step with the school's calendar: a watch channel has Google post to HookPath as it changes, and an hourly sweep runs the same import for anything missed and renews the channel.
type Watcher struct {
	opts    Options
	token   string
	refresh func()
	run     func(context.Context) error
	kick    chan struct{}
	mu      sync.Mutex
	channel *gcal.Channel
}

func NewWatcher(opts Options, token string, refresh func()) *Watcher {
	w := &Watcher{opts: opts, token: token, refresh: refresh, kick: make(chan struct{}, 1)}
	w.run = func(ctx context.Context) error { return RunGoogle(ctx, opts) }
	return w
}

func (w *Watcher) Start() {
	go w.work()
	w.renew()
	w.Kick()
	go w.sweep()
}

func (w *Watcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.channel == nil {
		return
	}
	w.stop(w.channel)
	w.channel = nil
}

// Kick starts a run now, or one more run after the current one; any number
// of kicks during a run collapse into that one, which reads everything fresh.
func (w *Watcher) Kick() {
	select {
	case w.kick <- struct{}{}:
	default:
	}
}

func (w *Watcher) work() {
	for range w.kick {
		start := time.Now()
		if err := w.run(context.Background()); err != nil {
			slog.Error("calendar import", "error", err, "took", time.Since(start).Round(time.Millisecond))
		} else {
			slog.Info("calendar import", "took", time.Since(start).Round(time.Millisecond))
		}
		w.refresh()
	}
}

func (w *Watcher) sweep() {
	for range time.Tick(sweepInterval) {
		w.renew()
		w.Kick()
	}
}

func expiry(ch *gcal.Channel) time.Time {
	return time.UnixMilli(ch.Expiration)
}

func (w *Watcher) renew() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.channel != nil && time.Until(expiry(w.channel)) > renewWithin {
		return
	}
	id := make([]byte, 16)
	rand.Read(id)
	ch, err := w.opts.Calendar.Events.Watch(calendar.SchoolCalendarID, &gcal.Channel{
		Id: hex.EncodeToString(id), Type: "web_hook", Address: hookAddress, Token: w.token,
		Expiration: time.Now().Add(channelLife).UnixMilli(),
	}).Do()
	if err != nil {
		slog.Error("calendar watch", "error", err)
		return
	}
	slog.Info("calendar watch opened", "channel", ch.Id, "expires", expiry(ch).In(calendar.Location).Format(calendar.DateTimeFormat))
	old := w.channel
	w.channel = ch
	if old != nil {
		w.stop(old)
	}
}

func (w *Watcher) stop(ch *gcal.Channel) {
	if err := w.opts.Calendar.Channels.Stop(&gcal.Channel{Id: ch.Id, ResourceId: ch.ResourceId}).Do(); err != nil {
		slog.Error("calendar watch stop", "channel", ch.Id, "error", err)
		return
	}
	slog.Info("calendar watch closed", "channel", ch.Id)
}

// ServeHTTP takes Google's post. The token is stable across restarts, so a channel
// a previous revision left open still passes and each of its posts is one more kick.
func (w *Watcher) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Goog-Channel-Token")), []byte(w.token)) != 1 {
		http.Error(rw, "unknown channel", http.StatusForbidden)
		return
	}
	state := r.Header.Get("X-Goog-Resource-State")
	slog.InfoContext(r.Context(), "calendar changed", "channel", r.Header.Get("X-Goog-Channel-ID"), "state", state, "message", r.Header.Get("X-Goog-Message-Number"))
	if state != "sync" {
		w.Kick()
	}
	rw.WriteHeader(http.StatusOK)
}
