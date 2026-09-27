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
	"slices"
	"strings"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/calendarimport"
	"heliosian/internal/celebrate"
	"heliosian/internal/claude"
	"heliosian/internal/config"
	"heliosian/internal/data"
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
	CalendarMail  calendar.Mail
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

type Core struct {
	Mux           *http.ServeMux
	HomeMux       *http.ServeMux
	TeamMux       *http.ServeMux
	TeamCache     *team.Cache
	BirthdayMux   *http.ServeMux
	CelebrateMux  *http.ServeMux
	CalendarMux   *http.ServeMux
	CalendarCache *calendar.Cache
	LoopMux       *http.ServeMux
	LoopCache     *loop.Cache
	AskMux        *http.ServeMux
	Cache         *who.Cache
	Documents     *artifacts.Filer
	Queue         *store.Queue
	Spoof         *auth.Spoof
	Member        func(email string) bool
	Sessions      auth.Sessions
	Home          http.Handler
	Team          http.Handler
	Birthday      http.Handler
	Celebrate     http.Handler
	Calendar      http.Handler
	Loop          http.Handler
	Ask           http.Handler
	Previews      map[string]func(r *http.Request) string
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
	superAdmin := func(email string) bool {
		return slices.Contains(settings.SuperAdmins(), strings.ToLower(strings.TrimSpace(email)))
	}
	teamCache, err := team.NewCache(cfg.Source, cfg.Writer, teamImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		logging.Fatal("load team data", "error", err)
	}
	birthdayCache, err := birthday.NewCache(cfg.Source, cfg.Writer, superAdmin, queue)
	if err != nil {
		logging.Fatal("load birthdays data", "error", err)
	}
	celebrateCache, err := celebrate.NewCache(cfg.Source, cfg.Writer, celebrateImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		logging.Fatal("load celebrate data", "error", err)
	}
	cache, err := who.NewCache(cfg.Source, cfg.Writer, cfg.Blobs, staticFiles{}, queue, cfg.FamilyIDKey, settings.SuperAdmins)
	if err != nil {
		logging.Fatal("load directory data", "error", err)
	}
	go cache.Locate(cfg.Geocoder)
	invites, err := who.NewInvites(cfg.Source, cfg.Writer, queue)
	if err != nil {
		logging.Fatal("load invites data", "error", err)
	}
	calendarCache, err := calendar.NewCache(cfg.Source, cfg.Writer, func() calendar.Roster { return CalendarRoster(cache.Model()) }, calendarImages{cfg.Store}, superAdmin, queue)
	if err != nil {
		logging.Fatal("load calendar data", "error", err)
	}
	loopCache, err := loop.NewCache(cfg.Source, cfg.Writer, superAdmin, queue)
	if err != nil {
		logging.Fatal("load loop data", "error", err)
	}
	loopDir := loopDirectory{cache, settings, teamCache, celebrateCache}
	homeCache, err := home.NewCache(cfg.Source, cfg.Writer, homeImages{cfg.Store}, settings.SuperAdmins, audienceSources{loopDir}, queue)
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
	calendarStyle := calendar.CardStyle(appName("calendar"), taglineOf("calendar"))
	artifactsCache, err := artifacts.NewCache(cfg.Source, cfg.Writer, cfg.Bucket, cfg.Embedder, queue)
	if err != nil {
		logging.Fatal("load artifacts data", "error", err)
	}
	if cfg.KeyPoints != nil {
		go keypoints.Run(artifactsCache, cfg.KeyPoints, schoolDirectory{cache})
	}
	mux := http.NewServeMux()
	config.Register(mux, settings, cache.IsAdmin)
	who.Register(mux, cache, cfg.BrowserKey, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir}, whoAbout)
	who.RegisterTags(mux, cache)
	who.RegisterAdmin(mux, cache, cfg.Store)
	who.RegisterInvites(mux, cache, invites)
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	linked := calendarLinked{celebrateCache, teamCache, celebrateDirectory{cache, settings}}.list
	calendarDir := calendarDirectory{cache, settings, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir}.Lists}
	frontEvents := upcomingEvents{calendarCache, calendarDir, linked}
	calendarMux := http.NewServeMux()
	var hooks calendar.Hooks
	moveAddress := func(ctx context.Context, actor access.Actor, old, to, name string) error {
		if _, err := celebrate.MoveAddress(ctx, celebrateCache, actor, old, to, name); err != nil {
			return err
		}
		hooks.MoveAddress(ctx, actor, old, to, name)
		return nil
	}
	partyCalendar := calendar.Celebrate{Party: partyPeople{celebrateCache}.people, IsAdmin: celebrateCache.IsAdmin, MoveAddress: moveAddress}
	hooks = calendar.Register(calendarMux, calendarCache, cfg.Store, calendarDir, settings.SuperAdmins, linked, partyCalendar, audienceSources{loopDir}.Sources, cfg.ImageSearch, cfg.CalendarMail, calendarStyle)
	homeMux := http.NewServeMux()
	home.Register(homeMux, homeCache, cfg.Store, settings.SuperAdmins, cache.HeroPhoto, directory{cache, settings}.HomePeople, directory{cache, settings}.Alerts, frontEvents.list, frontEvents.month, cfg.ImageSearch, hooks.Answer, hooks.MakeDefault, homeStyle)
	teamMux := http.NewServeMux()
	eventRSVPs := func(id string) *team.EventRSVPs {
		sent, answers, ok := calendarCache.LinkedRSVPs(linked(""), calendar.SourceTeam, id)
		if !ok {
			return nil
		}
		return &team.EventRSVPs{Sent: sent, Answers: answers}
	}
	team.Register(teamMux, teamCache, cfg.Store, directory{cache, settings}, settings.SuperAdmins, cfg.ImageSearch, cfg.Mail, cfg.MailFrom, eventRSVPs, activityEmailList(loopCache.Model), teamStyle)
	birthdayMux := http.NewServeMux()
	birthday.Register(birthdayMux, birthdayCache, birthdayDirectory{cache, settings}, settings.SuperAdmins, cfg.Describer, cfg.BirthdayMail, cfg.BirthdayFrom, cfg.BirthdayBase, func(ctx context.Context, email string) error {
		return home.Grant(ctx, homeCache, "birthday", email)
	}, birthdayAbout)
	celebrateMux := http.NewServeMux()
	partyRSVPs := func(partyID string) *celebrate.PartyRSVPs {
		sent, answers, ok := calendarCache.PartyRSVPs(partyID)
		if !ok {
			return nil
		}
		return &celebrate.PartyRSVPs{Sent: sent, Answers: answers}
	}
	celebrate.Register(celebrateMux, celebrateCache, cfg.Store, celebrateDirectory{cache, settings}, settings.SuperAdmins, cfg.ImageSearch, cfg.CelebrateMail, cfg.CelebrateFrom, partyRSVPs, func(ctx context.Context, actor access.Actor, old, to, name string) {
		hooks.MoveAddress(ctx, actor, old, to, name)
	}, celebrateStyle)
	askMux := http.NewServeMux()
	loopMail := cfg.Loop
	documents := artifacts.Register(askMux, artifactsCache, cfg.Embedder, queue, cfg.ArtifactsMail)
	loopMail.Documents = documents
	loopMux := http.NewServeMux()
	loop.Register(loopMux, loopCache, cfg.Store, loopDir, settings.SuperAdmins, loopMail, cfg.LoopDescriber, loopAbout)
	ask.Register(askMux, askSources(cache, settings, teamCache, celebrateCache, calendarCache, loopCache, homeCache, artifactsCache, cfg.Embedder, smartLists{cache, teamCache, celebrateCache, loopCache, loopDir}, loopDir, linked), cfg.Asker, spend, cfg.ChatKey)
	waitingApprovals := approvals(cache, teamCache, celebrateCache, calendarCache)
	behind := lateBirthdays(cache, birthdayCache, birthdayDirectory{cache, settings})
	for _, m := range []*http.ServeMux{mux, teamMux, birthdayMux, celebrateMux, calendarMux, loopMux, askMux} {
		home.RegisterSwitch(m, homeCache)
		if m != calendarMux {
			m.HandleFunc("GET /api/apps/rsvp", hooks.RSVPs)
		}
		m.HandleFunc("GET /api/apps/approvals", waitingApprovals)
		m.HandleFunc("GET /api/apps/late", behind)
	}
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
	for key, m := range map[string]*http.ServeMux{"who": mux, "home": homeMux, "team": teamMux, "birthday": birthdayMux, "celebrate": celebrateMux, "calendar": calendarMux, "loop": loopMux, "ask": askMux} {
		m.Handle("GET /optin", optIn)
		feedback.Register(m, key, appName(key), superAdmin, feedbackIntake)
		if suggestions != nil {
			suggestions.Register(m)
		}
	}
	homeMux.HandleFunc("GET /api/apps/rsvp", hooks.RSVPs)
	homeMux.HandleFunc("GET /api/apps/approvals", waitingApprovals)
	homeMux.HandleFunc("GET /api/apps/late", behind)
	homeMux.HandleFunc("GET /api/apps/team", teamWidget(cache, teamCache))
	homeMux.HandleFunc("GET /api/apps/celebrate", celebrateWidget(cache, calendarCache, calendarDir, linked))
	homeMux.HandleFunc("GET /api/apps/school", schoolWidget(cache, artifactsCache))
	feedback.RegisterAdmin(homeMux, feedbackCache, cfg.FeedbackFiler, superAdmin)
	blob.Register(mux, cfg.Store)
	blob.RegisterHome(homeMux, cfg.Store)
	blob.RegisterTeam(teamMux, cfg.Store)
	blob.RegisterBirthday(birthdayMux, cfg.Store)
	blob.RegisterCelebrate(celebrateMux, cfg.Store)
	blob.RegisterCalendar(calendarMux, cfg.Store)
	blob.RegisterLoop(loopMux, cfg.Store)
	blob.RegisterAsk(askMux, cfg.Store)
	go queue.Tick()
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		queue.Refresh()
	})
	return &Core{
		Mux: mux, HomeMux: homeMux, TeamMux: teamMux, TeamCache: teamCache, BirthdayMux: birthdayMux, CelebrateMux: celebrateMux,
		CalendarMux: calendarMux, CalendarCache: calendarCache, LoopMux: loopMux, LoopCache: loopCache, AskMux: askMux, Cache: cache, Documents: documents, Queue: queue,
		Spoof:  &auth.Spoof{Allowed: superAdmin, Person: directory{cache, settings}.SpoofPerson, People: directory{cache, settings}.SpoofPeople},
		Member: func(email string) bool { return who.Member(cache, email) }, Sessions: settings, Home: homeMux, Team: teamMux, Birthday: birthdayMux, Celebrate: celebrateMux, Calendar: calendarMux, Loop: loopMux, Ask: askMux,
		Previews: map[string]func(r *http.Request) string{
			"who":       whoAbout.PreviewHead,
			"home":      home.PreviewHead(homeCache, homeStyle),
			"team":      team.PreviewHead(teamCache, teamStyle),
			"birthday":  birthdayAbout.PreviewHead,
			"celebrate": celebrate.PreviewHead(celebrateCache, celebrateStyle),
			"calendar":  calendar.PreviewHead(calendarCache, linked, calendarStyle),
			"loop":      loopAbout.PreviewHead,
		},
	}
}

func (c *Core) Muxes() map[string]*http.ServeMux {
	return map[string]*http.ServeMux{"who": c.Mux, "home": c.HomeMux, "team": c.TeamMux, "birthday": c.BirthdayMux, "celebrate": c.CelebrateMux, "calendar": c.CalendarMux, "loop": c.LoopMux, "ask": c.AskMux}
}

func Production(domain string) (*http.Server, *store.Queue) {
	spreadsheets := map[string]string{
		"directory":      requiredEnv("DIRECTORY_SHEET"),
		"preferences":    requiredEnv("PREFERENCES_SHEET"),
		"invites":        requiredEnv("INVITES_SHEET"),
		"apps":           requiredEnv("APPS_SHEET"),
		"events":         requiredEnv("EVENTS_SHEET"),
		"birthdays":      requiredEnv("BIRTHDAY_SHEET"),
		"birthdayshared": requiredEnv("BIRTHDAY_SHARED_SHEET"),
		"celebrate":      requiredEnv("CELEBRATE_SHEET"),
		"calendar":       requiredEnv("CALENDAR_SHEET"),
		"config":         requiredEnv("CONFIG_SHEET"),
		"groups":         requiredEnv("GROUPS_SHEET"),
		"artifacts":      requiredEnv("ARTIFACTS_SHEET"),
		"feedback":       requiredEnv("FEEDBACK_SHEET"),
	}
	sessionKey := requiredEnv("SESSION_KEY")
	sheet, err := data.NewSheet(spreadsheets)
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
		Geocoder:      geocode.New(mapsKey("GOOGLE_MAPS_SERVER_KEY", "local/creds/geocoding.key")),
		Blobs:         store,
		Bucket:        bucket,
		Store:         store,
		FamilyIDKey:   familyIDKey.Sum(nil),
		ChatKey:       chatKey.Sum(nil),
		BrowserKey:    mapsKey("GOOGLE_MAPS_BROWSER_KEY", "local/creds/maps.key"),
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
		Asker:         ask.NewClaude(mapsKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key")),
		KeyPoints:     ClaudeKeyPoints(),
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(bucket),
	})
	who.RegisterUpload(core.Mux, core.Cache, store)
	client := clientID()
	newAuth := func(login auth.Login) *auth.Auth {
		a := auth.New(domain, client, []byte(sessionKey), login, core.Member, core.Sessions)
		a.Spoof = core.Spoof
		return a
	}
	whoAuth := newAuth(auth.Login{Title: "Helios Who?"})
	whoAuth.Preview = core.Previews["who"]
	whoAuth.Register(core.Mux)
	homeAuth := newAuth(auth.Login{Title: "Heliosian: Helios Community Apps"})
	homeAuth.Preview = core.Previews["home"]
	homeAuth.Register(core.HomeMux)
	teamAuth := newAuth(auth.Login{Title: "HCA Volunteer Portal"})
	teamAuth.Preview = core.Previews["team"]
	teamAuth.Register(core.TeamMux)
	birthdayAuth := newAuth(auth.Login{Title: "Helios Staff Birthdays"})
	birthdayAuth.Preview = core.Previews["birthday"]
	birthdayAuth.Register(core.BirthdayMux)
	celebrateAuth := newAuth(auth.Login{Title: "Helios Celebrate: Fun(d)raiser Parties"})
	celebrateAuth.Preview = core.Previews["celebrate"]
	celebrateAuth.Register(core.CelebrateMux)
	calendarAuth := newAuth(auth.Login{Title: "Helios When: The school year, day by day"})
	calendarAuth.Preview = core.Previews["calendar"]
	calendarAuth.Register(core.CalendarMux)
	loopAuth := newAuth(auth.Login{Title: "Helios Loop"})
	loopAuth.Preview = core.Previews["loop"]
	loopAuth.Register(core.LoopMux)
	askAuth := newAuth(auth.Login{Title: "Helios Ask"})
	askAuth.Register(core.AskMux)
	server := Server(domain, map[string]http.Handler{
		"who":       Public("who", whoAuth.Wrap(Logged("who", Files("who", core.Mux)))),
		"home":      Public("home", homeAuth.Wrap(Logged("home", Files("home", core.Home)))),
		"team":      Public("team", team.Redirected(core.TeamCache, teamAuth.Wrap(Logged("team", Files("team", core.Team))))),
		"birthday":  Public("birthday", birthdayAuth.Wrap(Logged("birthday", Files("birthday", core.Birthday)))),
		"celebrate": Public("celebrate", celebrateAuth.Wrap(Logged("celebrate", Files("celebrate", core.Celebrate)))),
		"calendar":  Public("calendar", calendarAuth.Wrap(Logged("calendar", Files("calendar", core.Calendar)))),
		"loop":      Public("loop", loopAuth.Wrap(Logged("loop", Files("loop", core.Loop)))),
		"ask":       Public("ask", askAuth.Wrap(Logged("ask", Files("ask", core.Ask)))),
	})
	if os.Getenv("K_SERVICE") != "" {
		watcher := calendarWatcher(sheet, core, sessionKey)
		core.CalendarMux.Handle("POST "+calendarimport.HookPath, watcher)
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
		Roster:       func() calendar.Roster { return CalendarRoster(core.Cache.Model()) },
		AnthropicKey: mapsKey("ANTHROPIC_API_KEY", "local/creds/anthropic.key"),
	}
	return calendarimport.NewWatcher(opts, hex.EncodeToString(mac.Sum(nil)))
}
