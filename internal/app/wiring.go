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

	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/calendarimport"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/geocode"
	"heliosian/internal/home"
	"heliosian/internal/imagesearch"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
	"heliosian/internal/who"
)

const deployOverlap = 30 * time.Second

const OptInPath = "/optin"

var spend = claude.NewLimiter()

type Config struct {
	Source        data.Source
	Writer        data.Writer
	Geocoder      *geocode.Client
	Bucket        *blob.Bucket
	Store         *blob.Store
	IDKey         []byte
	ChatKey       []byte
	BrowserKey    string
	ImageSearch   imagesearch.Search
	Mail          *mail.Mailgun
	CelebrateMail *mail.Mailgun
	CalendarMail  model.CalendarMail
	BirthdayMail  *mail.Mailgun
	BirthdayBase  string
	FeedbackFiler feedback.IssueFiler
	FeedbackBase  string
	Describer     *describe.Describer
	Loop          model.ListMail
	Asker         *ask.Claude
	Embedder      *artifacts.Vertex
	ArtifactsMail artifacts.Inbox
	KeyPoints     *keypoints.Claude
}

type appSpec struct {
	Key     string
	Title   string
	Mux     *http.ServeMux
	Preview func(r *http.Request) string
	Wrap    func(http.Handler) http.Handler
}

type Core struct {
	CalendarCache *model.CalendarCache
	LoopCache     *model.EmailListsCache
	Cache         *model.DirectoryCache
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
	settings, err := model.NewConfigCache(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load config", "error", err)
	}
	homeImages := blob.NewImages(cfg.Store, "home")
	teamImages := blob.NewImages(cfg.Store, "team")
	celebrateImages := blob.NewImages(cfg.Store, "celebrate")
	whenImages := blob.NewImages(cfg.Store, "when", "celebrate", "team")
	teamCache, err := model.NewActivitiesCache(cfg.Source, cfg.Writer, teamImages, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load team data", "error", err)
	}
	birthdayCache, err := model.NewBirthdaysCache(cfg.Source, cfg.Writer, settings.SuperAdmins, queue, cfg.IDKey)
	if err != nil {
		logging.Fatal("load birthdays data", "error", err)
	}
	celebrateCache, err := model.NewPartiesCache(cfg.Source, cfg.Writer, celebrateImages, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load celebrate data", "error", err)
	}
	cache, err := model.NewDirectoryCache(cfg.Source, cfg.Writer, cfg.Store, static.Files{Root: "web/who"}, queue, cfg.IDKey, settings.SuperAdmins)
	if err != nil {
		logging.Fatal("load directory data", "error", err)
	}
	go cache.Locate(cfg.Geocoder)
	invites, err := model.NewInviteTemplatesCache(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load invites data", "error", err)
	}
	calendarCache, err := model.NewCalendarCache(cfg.Source, cfg.Writer, func() model.Roster { return cache.Model().Roster() }, whenImages, settings.SuperAdmins, queue)
	if err != nil {
		logging.Fatal("load calendar data", "error", err)
	}
	loopCache, err := model.NewEmailListsCache(cfg.Source, cfg.Writer, settings.SuperAdmins, queue, cfg.IDKey)
	if err != nil {
		logging.Fatal("load loop data", "error", err)
	}
	sources := audience(cache, teamCache, celebrateCache)
	loopSources := loopAudience(cache, teamCache, celebrateCache)
	lists := smartLists{cache, teamCache, celebrateCache, loopCache}
	homeCache, err := home.NewCache(cfg.Source, cfg.Writer, homeImages, settings.SuperAdmins, sources, queue)
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
	birthdayAbout := model.BirthdaysAbout(appName("birthday"), taglineOf("birthday"))
	loopAbout := model.EmailListsAbout(appName("loop"), taglineOf("loop"))
	homeStyle := home.CardStyle(appName("home"), taglineOf("home"))
	teamStyle := model.ActivitiesCardStyle(appName("team"), taglineOf("team"))
	celebrateStyle := model.PartiesCardStyle(appName("celebrate"), taglineOf("celebrate"))
	calendarStyle := model.CalendarCardStyle(appName("when"), taglineOf("when"))
	artifactsCache, err := artifacts.NewCache(cfg.Source, cfg.Writer, cfg.Bucket, cfg.Embedder, queue)
	if err != nil {
		logging.Fatal("load artifacts data", "error", err)
	}
	go keypoints.Run(artifactsCache, cfg.KeyPoints, cache)
	mux := http.NewServeMux()
	model.RegisterConfig(mux, settings, cache, cache.Held)
	who.Register(mux, cache, whoAbout)
	model.RegisterDirectory(mux, model.DirectoryRoutes{Cache: cache, Invites: invites, Media: cfg.Store, MapsKey: cfg.BrowserKey, Parties: celebrateCache, Activities: teamCache, EmailLists: loopCache})
	blob.Register(mux, cfg.Store, "pronunciation")
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	linked := func(email string) []model.Linked {
		return model.LinkedEvents(cache.Model(), celebrateCache.Model(), teamCache.Model(), email, time.Now().In(model.Location))
	}
	frontEvents := upcomingEvents{calendarCache, cache.Model, linked}
	calendarMux := http.NewServeMux()
	hooks := model.RegisterCalendar(calendarMux, model.CalendarDeps{
		Cache:      calendarCache,
		Images:     whenImages,
		Directory:  cache.Model,
		Settings:   settings.Config,
		Parties:    celebrateCache,
		Activities: teamCache,
		EmailLists: loopCache,
		Search:     cfg.ImageSearch,
		Mail:       cfg.CalendarMail,
		Style:      calendarStyle,
		Queue:      queue,
	})
	homeMux := http.NewServeMux()
	home.Register(homeMux, home.Deps{
		Cache:       homeCache,
		Images:      homeImages,
		Upcoming:    frontEvents.list,
		Month:       frontEvents.month,
		Search:      cfg.ImageSearch,
		Answer:      hooks.Answer,
		MakeDefault: hooks.MakeDefault,
		Style:       homeStyle,
	})
	teamMux := http.NewServeMux()
	model.RegisterActivities(teamMux, model.ActivitiesDeps{
		Cache:      teamCache,
		Images:     teamImages,
		Directory:  cache.Model,
		Settings:   settings.Config,
		Calendar:   hooks,
		Search:     cfg.ImageSearch,
		Mailer:     cfg.Mail,
		EmailLists: loopCache,
		Style:      teamStyle,
		Describer:  cfg.Describer,
	})
	birthdayMux := http.NewServeMux()
	birthdayResources := model.BirthdayResources(birthdayCache, func(ctx context.Context, email string) error {
		return home.Grant(ctx, homeCache, "birthday", email)
	})
	celebrateMux := http.NewServeMux()
	model.RegisterParties(celebrateMux, model.PartiesDeps{
		Cache:     celebrateCache,
		Images:    celebrateImages,
		Directory: cache.Model,
		Calendar:  hooks,
		Search:    cfg.ImageSearch,
		Mailer:    cfg.CelebrateMail,
		Style:     celebrateStyle,
	})
	askMux := http.NewServeMux()
	loopMail := cfg.Loop
	documents := artifacts.Register(askMux, artifactsCache, cfg.Embedder, queue, cfg.ArtifactsMail)
	loopMail.Documents = documents
	loopMux := http.NewServeMux()
	model.RegisterEmailLists(loopMux, model.EmailListsDeps{
		Cache:      loopCache,
		Directory:  cache.Model,
		Parties:    celebrateCache,
		Activities: teamCache,
		Media:      cfg.Store,
		Mail:       loopMail,
		Describer:  cfg.Describer,
		About:      loopAbout,
	})
	ask.Register(askMux, ask.Sources{
		Directory: cache.Model,
		Tags: func(owner string) []model.Tag {
			return cache.Model().Tags(owner)
		},
		Lists: func(email string) []model.MagicTag {
			return append(cache.Model().RoomParentTags(email), lists.Lists(email)...)
		},
		Settings:    settings.Config,
		Calendar:    calendarCache.Model,
		Linked:      linked,
		Team:        teamCache.Model,
		Celebrate:   celebrateCache.Model,
		Loop:        loopCache.Model,
		LoopSources: loopSources,
		Links:       homeCache.CategoriesFor,
		Artifacts:   artifactsCache.Model,
		Embedder:    cfg.Embedder,
		Admins:      ask.Admins{Team: teamCache.Held, Celebrate: celebrateCache.Held, Loop: loopCache.Held, Calendar: calendarCache.Held, Home: homeCache.Held},
		Now:         time.Now,
	}, cfg.Asker, spend, cfg.ChatKey)
	apps := []appSpec{
		{Key: "who", Title: "Helios Who?", Mux: mux, Preview: whoAbout.PreviewHead},
		{Key: "home", Title: "Heliosian: Helios Community Apps", Mux: homeMux, Preview: home.PreviewHead(homeCache, homeStyle)},
		{Key: "team", Title: "HCA Volunteer Portal", Mux: teamMux, Preview: model.ActivitiesPreviewHead(teamCache, teamStyle), Wrap: func(next http.Handler) http.Handler {
			return model.ActivitiesRedirected(teamCache, next)
		}},
		{Key: "birthday", Title: "Helios Staff Birthdays", Mux: birthdayMux, Preview: birthdayAbout.PreviewHead},
		{Key: "celebrate", Title: "Helios Celebrate: Fun(d)raiser Parties", Mux: celebrateMux, Preview: model.PartiesPreviewHead(celebrateCache, celebrateStyle)},
		{Key: "when", Title: "Helios When: The school year, day by day", Mux: calendarMux, Preview: hooks.PreviewHead()},
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
	registry := resources(caches{settings, cache, invites, teamCache, birthdayCache, celebrateCache, calendarCache, loopCache, homeCache, artifactsCache, feedbackCache, hooks, cfg.IDKey}, queue, birthdayResources, model.EmailListResources(loopCache, loopMail.Documents))
	model.RegisterBirthdays(birthdayMux, model.BirthdaysDeps{
		Cache:     birthdayCache,
		Queue:     queue,
		Directory: cache.Model,
		Describer: cfg.Describer,
		Mailer:    cfg.BirthdayMail,
		Base:      cfg.BirthdayBase,
		About:     birthdayAbout,
		Taken:     registry.Taken,
	})
	notifier := feedback.Notifier{Sender: cfg.Mail, Base: cfg.FeedbackBase, SuperAdmins: settings.SuperAdmins}
	feedbackIntake := feedback.NewIntake(feedbackCache, cfg.Bucket, notifier.Notify)
	optIn := who.OptInForm(func() string { return settings.Config().PrivacyLinks.HeliosWhoOptIn })
	suggestions := geocode.NewSuggestions(cfg.Geocoder)
	for _, a := range apps {
		home.RegisterSwitch(a.Mux, homeCache)
		registry.Register(a.Mux)
		if a.Key != "when" {
			a.Mux.HandleFunc("GET /api/apps/rsvp", hooks.RSVPs)
		}
		a.Mux.HandleFunc("GET /api/apps/approvals", waitingApprovals)
		a.Mux.HandleFunc("GET /api/apps/late", behind)
		a.Mux.HandleFunc("GET /api/apps/alerts", alerts)
		a.Mux.Handle("GET "+OptInPath, optIn)
		feedback.Register(a.Mux, a.Key, appName(a.Key), cache.Actor, settings.IsSuperAdmin, feedbackIntake)
		suggestions.Register(a.Mux)
		folders := []string{"photos"}
		if folder, ok := blob.ImageFolder(a.Key); ok {
			folders = append(folders, folder)
		}
		blob.Register(a.Mux, cfg.Store, folders...)
	}
	homeMux.HandleFunc("GET /api/apps/team", teamWidget(cache, teamCache))
	homeMux.HandleFunc("GET /api/apps/celebrate", celebrateWidget(cache, calendarCache, linked))
	homeMux.HandleFunc("GET /api/apps/school", schoolWidget(cache, artifactsCache))
	feedback.RegisterAdmin(homeMux, feedbackCache, cfg.Bucket, cfg.FeedbackFiler, cache.Actor, settings.IsSuperAdmin)
	go queue.Tick()
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		queue.Refresh()
	})
	return &Core{
		CalendarCache: calendarCache, LoopCache: loopCache, Cache: cache, Documents: documents, Queue: queue,
		Spoof:  &auth.Spoof{Allowed: settings.IsSuperAdmin, Person: spoofPerson(cache)},
		Member: cache.Member, Sessions: settings, apps: apps,
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
	sheet, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
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
	chatKey := hmac.New(sha256.New, []byte(sessionKey))
	chatKey.Write([]byte("ask chats"))
	anthropicKey := env.Required("ANTHROPIC_API_KEY")
	core := NewCore(Config{
		Source:        sheet,
		Writer:        sheet,
		Geocoder:      geocode.New(env.Required("GOOGLE_MAPS_SERVER_KEY")),
		Bucket:        bucket,
		Store:         store,
		IDKey:         []byte(env.Required("ID_KEY")),
		ChatKey:       chatKey.Sum(nil),
		BrowserKey:    env.Required("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch:   ImageSearchKeys(),
		Describer:     describe.New(anthropicKey, spend),
		Mail:          newMailer(mailFrom),
		CelebrateMail: newMailer(celebrateMailFrom),
		CalendarMail:  calendarMail(sessionKey),
		BirthdayMail:  newMailer(birthdayMailFrom),
		BirthdayBase:  birthdayBase,
		FeedbackFiler: githubApp(),
		FeedbackBase:  feedbackBase,
		Loop:          loopMail(sessionKey),
		Asker:         ask.NewClaude(anthropicKey),
		KeyPoints:     keypoints.New(anthropicKey),
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(bucket),
	})
	muxes := core.Muxes()
	model.RegisterDirectoryUpload(muxes["who"], core.Cache, store)
	client := env.Required("GOOGLE_CLIENT_ID")
	auths := map[string]*auth.Auth{}
	for _, a := range core.apps {
		gate := auth.New(domain, client, []byte(sessionKey), auth.Login{Title: a.Title}, core.Member, []string{OptInPath}, core.Sessions)
		gate.Spoof = core.Spoof
		gate.Preview = a.Preview
		gate.Register(a.Mux)
		auths[a.Key] = gate
	}
	server := Server(domain, core.Handlers(func(key string, next http.Handler) http.Handler {
		return auths[key].Wrap(next)
	}), core.Aliased())
	if os.Getenv("K_SERVICE") != "" {
		watcher := calendarWatcher(sheet, core, sessionKey, anthropicKey)
		muxes["when"].Handle("POST "+calendarimport.HookPath, watcher)
		watcher.Start()
		server.RegisterOnShutdown(func() { core.Queue.Add(watcher.Stop) })
	}
	return server, core.Queue
}

func calendarWatcher(sheet *data.Sheet, core *Core, sessionKey, anthropicKey string) *calendarimport.Watcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	opts := calendarimport.Options{
		Source: sheet, Cache: core.CalendarCache, Calendar: cal,
		Roster:       func() model.Roster { return core.Cache.Model().Roster() },
		AnthropicKey: anthropicKey,
	}
	return calendarimport.NewWatcher(opts, hex.EncodeToString(mac.Sum(nil)))
}
