package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/calendarimport"
	"heliosian/internal/celebrate"
	"heliosian/internal/claude"
	"heliosian/internal/config"
	"heliosian/internal/data"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/geocode"
	"heliosian/internal/home"
	"heliosian/internal/imagesearch"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

const deployOverlap = 30 * time.Second

var spend = claude.NewLimiter()

type Config struct {
	Source        data.Source
	Writer        data.Writer
	Geocoder      who.Geocoder
	Blobs         who.BlobChecker
	Bucket        *blob.Bucket
	Store         *blob.Store
	FamilyIDKey   []byte
	ChatKey       []byte
	BrowserKey    string
	ImageSearch   imagesearch.Search
	Mail          mail.Sender
	MailFrom      string
	CelebrateMail mail.Sender
	CelebrateFrom string
	CalendarMail  when.Mail
	BirthdayMail  mail.Sender
	BirthdayFrom  string
	BirthdayBase  string
	FeedbackFiler feedback.IssueFiler
	FeedbackBase  string
	Describer     birthday.Describer
	Loop          loop.Mail
	LoopDescriber loop.Describer
	Asker         ask.Responder
	Embedder      artifacts.Embedder
	ArtifactsMail artifacts.Inbox
	KeyPoints     keypoints.Summarizer
}

type appSpec struct {
	Key         string
	Title       string
	Mux         *http.ServeMux
	Preview     func(r *http.Request) string
	Wrap        func(http.Handler) http.Handler
	ImageFolder string
}

type Core struct {
	CalendarCache *when.Cache
	LoopCache     *loop.Cache
	Cache         *who.Cache
	Documents     *artifacts.Filer
	Queue         *store.Queue
	Spoof         *auth.Spoof
	Member        func(email string) bool
	Sessions      auth.Sessions
	apps          []appSpec
}

func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		logging.Fatal("register manifest mime type", "error", err)
	}
	queue := store.NewQueue()
	queue.Register(cfg.Store.Load)
	cfg.ImageSearch.Stock = imagesearch.NewStock(cfg.Bucket, cfg.Store)
	cfg.ImageSearch.Limits = imagesearch.NewLimits()
	settings, err := config.NewCache(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load config", "error", err)
	}
	teamCache, err := team.NewCache(cfg.Source, cfg.Writer, teamImages{cfg.Store}, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load team data", "error", err)
	}
	birthdayCache, err := birthday.NewCache(cfg.Source, cfg.Writer, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load birthdays data", "error", err)
	}
	celebrateCache, err := celebrate.NewCache(cfg.Source, cfg.Writer, celebrateImages{cfg.Store}, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load celebrate data", "error", err)
	}
	cache, err := who.NewCache(cfg.Source, cfg.Writer, cfg.Blobs, StaticFiles{Root: "web/who"}, queue, cfg.FamilyIDKey, settings.SuperAdmins)
	if err != nil {
		logging.Fatal("load directory data", "error", err)
	}
	go cache.Locate(cfg.Geocoder)
	invites, err := who.NewInvites(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load invites data", "error", err)
	}
	calendarCache, err := when.NewCache(cfg.Source, cfg.Writer, func() when.Roster { return CalendarRoster(cache.Model()) }, calendarImages{cfg.Store}, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load calendar data", "error", err)
	}
	loopCache, err := loop.NewCache(cfg.Source, cfg.Writer, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load loop data", "error", err)
	}
	sources := audience(cache, teamCache, celebrateCache)
	lists := smartLists{cache, teamCache, celebrateCache, loopCache, sources}
	homeCache, err := home.NewCache(cfg.Source, cfg.Writer, homeImages{cfg.Store}, settings.SuperAdmins, sources, queue)
	if err != nil {
		logging.Fatal("load apps data", "error", err)
	}
	taglineOf := func(key string) func() string {
		return func() string {
			if key == "home" {
				return home.Home.Tagline
			}
			for _, a := range homeCache.AppList() {
				if a.Key == key {
					return a.Tagline
				}
			}
			return ""
		}
	}
	appName := func(key string) func() string {
		return func() string {
			if key == "home" {
				return home.Home.Name
			}
			for _, a := range homeCache.AppList() {
				if a.Key == key {
					return a.Name
				}
			}
			return key
		}
	}
	whoAbout := who.About(appName("who"), taglineOf("who"))
	birthdayAbout := birthday.About(appName("birthday"), taglineOf("birthday"))
	loopAbout := loop.About(appName("loop"), taglineOf("loop"))
	homeStyle := home.CardStyle(appName("home"), taglineOf("home"))
	teamStyle := team.CardStyle(appName("team"), taglineOf("team"))
	celebrateStyle := celebrate.CardStyle(appName("celebrate"), taglineOf("celebrate"))
	calendarStyle := when.CardStyle(appName("when"), taglineOf("when"))
	artifactsCache, err := artifacts.NewCache(cfg.Source, cfg.Writer, cfg.Bucket, cfg.Embedder, queue)
	if err != nil {
		logging.Fatal("load artifacts data", "error", err)
	}
	if cfg.KeyPoints != nil {
		go keypoints.Run(artifactsCache, cfg.KeyPoints, schoolDirectory{cache})
	}
	mux := http.NewServeMux()
	config.Register(mux, settings, cache.IsAdmin)
	who.Register(mux, cache, cfg.BrowserKey, lists, whoAbout)
	who.RegisterTags(mux, cache)
	who.RegisterAdmin(mux, cache, cfg.Store)
	who.RegisterInvites(mux, cache, invites)
	blob.Register(mux, cfg.Store, "pronunciation")
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	linked := calendarLinked{celebrateCache, teamCache, cache.Model}.list
	frontEvents := upcomingEvents{calendarCache, cache.Model, linked}
	calendarMux := http.NewServeMux()
	var hooks when.Hooks
	moveAddress := func(ctx context.Context, actor access.Actor, old, to, name string) error {
		if _, err := celebrate.MoveAddress(ctx, celebrateCache, actor, old, to, name); err != nil {
			return err
		}
		hooks.MoveAddress(ctx, actor, old, to, name)
		return nil
	}
	partyCalendar := when.Celebrate{Party: partyPeople{celebrateCache}.people, IsAdmin: celebrateCache.IsAdmin, MoveAddress: moveAddress}
	hooks = when.Register(calendarMux, calendarCache, cfg.Store, cache.Model, settings.Settings, calendarLists(cache, lists.Lists), linked, partyCalendar, sources, cfg.ImageSearch, cfg.CalendarMail, calendarStyle)
	homeMux := http.NewServeMux()
	home.Register(homeMux, homeCache, cfg.Store, frontEvents.list, frontEvents.month, cfg.ImageSearch, hooks.Answer, hooks.MakeDefault, homeStyle)
	teamMux := http.NewServeMux()
	eventRSVPs := func(id string) *team.EventRSVPs {
		sent, answers, ok := calendarCache.LinkedRSVPs(linked(""), when.SourceTeam, id)
		if !ok {
			return nil
		}
		return &team.EventRSVPs{Sent: sent, Answers: answers}
	}
	team.Register(teamMux, teamCache, cfg.Store, cache.Model, settings.Settings, cfg.ImageSearch, cfg.Mail, cfg.MailFrom, eventRSVPs, activityEmailList(loopCache.Model), teamStyle)
	birthdayMux := http.NewServeMux()
	birthday.Register(birthdayMux, birthdayCache, cache.Model, cfg.Describer, cfg.BirthdayMail, cfg.BirthdayFrom, cfg.BirthdayBase, func(ctx context.Context, email string) error {
		return home.Grant(ctx, homeCache, "birthday", email)
	}, birthdayAbout)
	celebrateMux := http.NewServeMux()
	partyRSVPs := func(partyID string) *celebrate.PartyRSVPs {
		sent, answers, ok := calendarCache.LinkedRSVPs(nil, when.SourceCelebrate, partyID)
		if !ok {
			return nil
		}
		return &celebrate.PartyRSVPs{Sent: sent, Answers: answers}
	}
	celebrate.Register(celebrateMux, celebrateCache, cfg.Store, cache.Model, cfg.ImageSearch, cfg.CelebrateMail, cfg.CelebrateFrom, partyRSVPs, func(ctx context.Context, actor access.Actor, old, to, name string) {
		hooks.MoveAddress(ctx, actor, old, to, name)
	}, celebrateStyle)
	askMux := http.NewServeMux()
	loopMail := cfg.Loop
	documents := artifacts.Register(askMux, artifactsCache, cfg.Embedder, queue, cfg.ArtifactsMail)
	loopMail.Documents = documents
	loopMux := http.NewServeMux()
	loop.Register(loopMux, loopCache, cfg.Store, sources, settings.Settings, loopMail, cfg.LoopDescriber, loopAbout)
	ask.Register(askMux, askSources(cache, settings, teamCache, celebrateCache, calendarCache, loopCache, homeCache, artifactsCache, cfg.Embedder, lists, sources, linked), cfg.Asker, spend, cfg.ChatKey)
	apps := []appSpec{
		{Key: "who", Title: "Helios Who?", Mux: mux, Preview: whoAbout.PreviewHead},
		{Key: "home", Title: "Heliosian: Helios Community Apps", Mux: homeMux, Preview: home.PreviewHead(homeCache, homeStyle), ImageFolder: "link-images"},
		{Key: "team", Title: "HCA Volunteer Portal", Mux: teamMux, Preview: team.PreviewHead(teamCache, teamStyle), ImageFolder: "activity-images", Wrap: func(next http.Handler) http.Handler {
			return team.Redirected(teamCache, next)
		}},
		{Key: "birthday", Title: "Helios Staff Birthdays", Mux: birthdayMux, Preview: birthdayAbout.PreviewHead},
		{Key: "celebrate", Title: "Helios Celebrate: Fun(d)raiser Parties", Mux: celebrateMux, Preview: celebrate.PreviewHead(celebrateCache, celebrateStyle), ImageFolder: "party-images"},
		{Key: "when", Title: "Helios When: The school year, day by day", Mux: calendarMux, Preview: when.PreviewHead(calendarCache, linked, calendarStyle), ImageFolder: "category-images"},
		{Key: "loop", Title: "Helios Loop", Mux: loopMux, Preview: loopAbout.PreviewHead},
		{Key: "ask", Title: "Helios Ask", Mux: askMux},
	}
	waitingApprovals := approvals(cache, teamCache, celebrateCache, calendarCache)
	behind := lateBirthdays(cache, birthdayCache)
	alerts := staleAlerts(cache, settings)
	feedbackCache, err := feedback.NewCache(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load feedback model", "error", err)
	}
	notifier := feedback.Notifier{Sender: cfg.Mail, From: cfg.MailFrom, Base: cfg.FeedbackBase, SuperAdmins: settings.SuperAdmins}
	feedbackIntake := feedback.NewIntake(feedbackCache, notifier.Notify)
	optIn := who.OptInForm(func() string { return settings.Settings().PrivacyLinks.HeliosWhoOptIn })
	var suggestions *geocode.Suggestions
	if s, ok := cfg.Geocoder.(geocode.Suggester); ok {
		suggestions = geocode.NewSuggestions(s)
	}
	for _, a := range apps {
		home.RegisterSwitch(a.Mux, homeCache)
		if a.Key != "when" {
			a.Mux.HandleFunc("GET /api/apps/rsvp", hooks.RSVPs)
		}
		a.Mux.HandleFunc("GET /api/apps/approvals", waitingApprovals)
		a.Mux.HandleFunc("GET /api/apps/late", behind)
		a.Mux.HandleFunc("GET /api/apps/alerts", alerts)
		a.Mux.Handle("GET /optin", optIn)
		feedback.Register(a.Mux, a.Key, appName(a.Key), settings.IsSuperAdmin, feedbackIntake)
		if suggestions != nil {
			suggestions.Register(a.Mux)
		}
		folders := []string{"photos"}
		if a.ImageFolder != "" {
			folders = append(folders, a.ImageFolder)
		}
		blob.Register(a.Mux, cfg.Store, folders...)
	}
	homeMux.HandleFunc("GET /api/apps/team", teamWidget(cache, teamCache))
	homeMux.HandleFunc("GET /api/apps/celebrate", celebrateWidget(cache, calendarCache, linked))
	homeMux.HandleFunc("GET /api/apps/school", schoolWidget(cache, artifactsCache))
	feedback.RegisterAdmin(homeMux, feedbackCache, cfg.FeedbackFiler, settings.IsSuperAdmin)
	go queue.Tick()
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		queue.Refresh()
	})
	return &Core{
		CalendarCache: calendarCache, LoopCache: loopCache, Cache: cache, Documents: documents, Queue: queue,
		Spoof:  &auth.Spoof{Allowed: settings.IsSuperAdmin, Person: spoofPerson(cache), People: spoofPeople(cache)},
		Member: func(email string) bool { return who.Member(cache, email) }, Sessions: settings, apps: apps,
	}
}

func (c *Core) Muxes() map[string]*http.ServeMux {
	out := map[string]*http.ServeMux{}
	for _, a := range c.apps {
		out[a.Key] = a.Mux
	}
	return out
}

func (c *Core) Handlers(gate func(key string, next http.Handler) http.Handler) map[string]http.Handler {
	out := map[string]http.Handler{}
	for _, a := range c.apps {
		h := gate(a.Key, Logged(a.Key, Files(a.Key, a.Mux)))
		if a.Wrap != nil {
			h = a.Wrap(h)
		}
		out[a.Key] = Public(a.Key, h)
	}
	return out
}

func (c *Core) Aliased() map[string]http.Handler {
	out := map[string]http.Handler{}
	for _, a := range c.apps {
		if a.Wrap != nil {
			out[a.Key] = a.Wrap(http.NotFoundHandler())
		}
	}
	return out
}

func Production(domain string) (*http.Server, *store.Queue) {
	sessionKey := env.Required("SESSION_KEY")
	sheet, err := data.NewSheet(SheetIDs(Spreadsheets))
	if err != nil {
		logging.Fatal("load directory sheet", "error", err)
	}
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		logging.Fatal("media bucket", "error", err)
	}
	store := blob.New(bucket)
	embedder, err := artifacts.NewVertex()
	if err != nil {
		logging.Fatal("vertex embedder", "error", err)
	}
	familyIDKey := hmac.New(sha256.New, []byte(sessionKey))
	familyIDKey.Write([]byte("family id"))
	chatKey := hmac.New(sha256.New, []byte(sessionKey))
	chatKey.Write([]byte("ask chats"))
	core := NewCore(Config{
		Source:        sheet,
		Writer:        sheet,
		Geocoder:      geocode.New(env.Key("GOOGLE_MAPS_SERVER_KEY", "local/creds/geocoding.key")),
		Blobs:         store,
		Bucket:        bucket,
		Store:         store,
		FamilyIDKey:   familyIDKey.Sum(nil),
		ChatKey:       chatKey.Sum(nil),
		BrowserKey:    env.Key("GOOGLE_MAPS_BROWSER_KEY", "local/creds/maps.key"),
		ImageSearch:   ImageSearchKeys(),
		Describer:     ClaudeDescriber(),
		Mail:          newMailer(mailFrom),
		MailFrom:      mailFrom,
		CelebrateMail: newMailer(celebrateMailFrom),
		CelebrateFrom: celebrateMailFrom,
		CalendarMail:  calendarMail(sessionKey),
		BirthdayMail:  newMailer(birthdayMailFrom),
		BirthdayFrom:  birthdayMailFrom,
		BirthdayBase:  birthdayBase,
		FeedbackFiler: githubApp(),
		FeedbackBase:  feedbackBase,
		Loop:          loopMail(sessionKey),
		LoopDescriber: ClaudeGroupDescriber(),
		Asker:         ask.NewClaude(env.Key("ANTHROPIC_API_KEY", "local/creds/anthropic.key")),
		KeyPoints:     ClaudeKeyPoints(),
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(bucket),
	})
	muxes := core.Muxes()
	who.RegisterUpload(muxes["who"], core.Cache, store)
	client := env.ClientID()
	auths := map[string]*auth.Auth{}
	for _, a := range core.apps {
		gate := auth.New(domain, client, []byte(sessionKey), auth.Login{Title: a.Title}, core.Member, core.Sessions)
		gate.Spoof = core.Spoof
		gate.Preview = a.Preview
		gate.Register(a.Mux)
		auths[a.Key] = gate
	}
	server := Server(domain, core.Handlers(func(key string, next http.Handler) http.Handler {
		return auths[key].Wrap(next)
	}), core.Aliased())
	if os.Getenv("K_SERVICE") != "" {
		watcher := calendarWatcher(sheet, core, sessionKey)
		muxes["when"].Handle("POST "+calendarimport.HookPath, watcher)
		watcher.Start()
		server.RegisterOnShutdown(func() { core.Queue.Add(watcher.Stop) })
	}
	return server, core.Queue
}

func calendarWatcher(sheet *data.Sheet, core *Core, sessionKey string) *calendarimport.Watcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	opts := calendarimport.Options{
		Source: sheet, Cache: core.CalendarCache, Calendar: cal,
		Roster:       func() when.Roster { return CalendarRoster(core.Cache.Model()) },
		AnthropicKey: env.Key("ANTHROPIC_API_KEY", "local/creds/anthropic.key"),
	}
	return calendarimport.NewWatcher(opts, hex.EncodeToString(mac.Sum(nil)))
}
