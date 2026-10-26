package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"image/color"
	"mime"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/describe"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/geocode"
	"heliosian/internal/hosts"
	"heliosian/internal/imagesearch"
	"heliosian/internal/intercept"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/mcp"
	"heliosian/internal/ops"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/tools"
	"heliosian/internal/vitals"
)

const localMail = "local/mail"

const OptInPath = "/optin"

var spend = claude.NewLimiter()

type Config struct {
	Domain        string
	Source        data.Source
	Writer        data.Writer
	Geocoder      *geocode.Client
	Bucket        *blob.Bucket
	ChatKey       []byte
	AccessKey     []byte
	BrowserKey    string
	ImageSearch   imagesearch.Search
	Mail          *mail.Mailgun
	CelebrateMail *mail.Mailgun
	CalendarMail  CalendarMail
	BirthdayMail  *mail.Mailgun
	FeedbackFiler *feedback.GitHubApp
	FeedbackBase  string
	Describer     *describe.Describer
	ListMail      db.ListMailConfig
	Asker         *ask.Claude
	Embedder      *artifacts.Vertex
	ArtifactsMail artifacts.Inbox
	Composer      *db.Composer
	Ops           ops.Deps
	Hooks         ops.Hooks
}

type appSpec struct {
	Key     string
	Title   string
	Mux     *http.ServeMux
	Preview func(r *http.Request) string
	Wrap    func(http.Handler) http.Handler
	Moved   map[string]string
}

type Core struct {
	Sessions  *db.Sessions
	Data      *db.Store
	Pictures  *db.Pictures
	Search    *db.Searcher
	ListMail  *db.ListMailer
	Invites   *db.Invitations
	Birthdays *db.BirthdayMailer
	Spoof     *auth.Spoof
	Settled   <-chan struct{}
	apps      []appSpec
	api       *http.ServeMux
}

func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		logging.Fatal("register manifest mime type", "error", err)
	}
	dataStore, err := db.NewStore(cfg.Source, cfg.Writer, db.NewSearchIndex(), hosts.Origin(cfg.Domain))
	if err != nil {
		logging.Fatal("load the data sheets", "error", err)
	}
	cfg.ImageSearch.Stock = db.NewStock(dataStore, cfg.Bucket)
	cfg.ImageSearch.Limits = imagesearch.NewLimits()
	sessions := db.NewSessions(dataStore)
	pictures := db.NewPictures(dataStore, cfg.Bucket)
	db.NewMembershipMailer(dataStore, db.MembershipMailConfig{Sender: cfg.Mail, Celebrate: cfg.CelebrateMail, Origin: hosts.Origin(cfg.Domain)})
	search := db.NewSearcher(dataStore, cfg.Bucket, cfg.Embedder)
	taglineOf := func(key string) func() string {
		return func() string {
			return dataStore.Model().App(key)["subtitle"]
		}
	}
	appNameOf := func(key string) string {
		return dataStore.Model().App(key)["name"]
	}
	appName := func(key string) func() string {
		return func() string {
			return appNameOf(key)
		}
	}
	whoAbout := directoryAbout(appName("who"), taglineOf("who"))
	birthdayAbout := birthdaysAbout(appName("birthday"), taglineOf("birthday"))
	loopAbout := listAbout(appName("loop"), taglineOf("loop"))
	homeStyle := db.HomeCardStyle(appName("home"), taglineOf("home"))
	teamStyle := &sharecard.Style{
		Palette: sharecard.Palette{
			Page: color.RGBA{0xee, 0xf6, 0xea, 0xff}, Brand: color.RGBA{0x0c, 0x4c, 0x54, 0xff}, Accent: color.RGBA{0x00, 0x74, 0x6f, 0xff},
			Ink: color.RGBA{0x0e, 0x3a, 0x42, 0xff}, Yellow: color.RGBA{0xf8, 0xd9, 0x08, 0xff}, Panel: color.RGBA{0xdc, 0xe9, 0xe4, 0xff},
		},
		Name: appName("team"), Tagline: taglineOf("team"),
		Mark: "web/public/team/brand/logo-mark.png", Lockup: "web/public/team/brand/logo-lockup.png", Corner: "web/team/toolbar_background.png",
	}
	celebrateStyle := db.PartyCardStyle(appName("celebrate"), taglineOf("celebrate"))
	calendarStyle := db.WhenCardStyle(appName("when"), taglineOf("when"))
	api := http.NewServeMux()
	mux := http.NewServeMux()
	registerDirectoryPages(mux, whoAbout, cfg.BrowserKey)
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	calendarMux := http.NewServeMux()
	serve.Pages(calendarMux, "web/when/index.html", "/{$}", "/c/{token}", "/day/{date}", "/mine", "/mine/{list}", "/admin", "/g/{key}")
	whenSearch := cfg.ImageSearch
	whenSearch.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	whenSearch.Register(api, "/api/when")
	homeMux := http.NewServeMux()
	homePages := db.NewHomePages(dataStore, homeStyle)
	homePages.Register(homeMux)
	homeSearch := cfg.ImageSearch
	homeSearch.UserAgent = "Heliosian image search (+https://heliosian.com)"
	homeSearch.Register(api, "/api/home")
	teamMux := http.NewServeMux()
	serve.Pages(teamMux, "web/team/index.html", "/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/g/{path...}")
	teamSearch := cfg.ImageSearch
	teamSearch.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	teamSearch.Register(api, "/api/team")
	activityPages := db.NewActivityPages(dataStore, cfg.Bucket, teamStyle)
	activityPages.Register(teamMux)
	birthdayMux := http.NewServeMux()
	birthdayMux.Handle("GET /open/share/about.png", birthdayAbout)
	serve.Pages(birthdayMux, "web/birthday/index.html", "/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}")
	celebrateMux := http.NewServeMux()
	celebrateSearch := cfg.ImageSearch
	celebrateSearch.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	celebrateSearch.Register(api, "/api/celebrate")
	partyPages := db.NewPartyPages(dataStore, cfg.Bucket, celebrateStyle)
	partyPages.Register(celebrateMux)
	askMux := http.NewServeMux()
	db.RegisterMailHook(askMux, dataStore, pictures, cfg.ArtifactsMail.SigningKey)
	loopMux := http.NewServeMux()
	loopMux.Handle("GET /open/share/about.png", loopAbout)
	serve.Pages(loopMux, "web/loop/index.html", "/{$}", "/new", "/g/{key}")
	listMail := db.RegisterListMail(loopMux, dataStore, pictures, cfg.ListMail)
	deliveries := db.NewDeliveries(dataStore, cfg.ListMail.SigningKey)
	deliveries.Register(loopMux)
	deliveries.Register(calendarMux)
	askAbout := ask.About(appName("ask"), taglineOf("ask"))
	adminMux := http.NewServeMux()
	for _, page := range []string{"resources", "query", "search", "queues", "policies", "erd"} {
		adminMux.HandleFunc("GET /"+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/admin/"+page+"/index.html")
		})
	}
	wikiMux := http.NewServeMux()
	serve.Pages(wikiMux, "web/wiki/index.html", "/{$}", "/new", "/p/{path...}")
	cfg.ImageSearch.Register(api, "/api/wiki")
	wikiShare := db.NewWikiShare(dataStore, pictures, appName("wiki"))
	wikiShare.Register(wikiMux)
	schoolNow := func() time.Time { return time.Now().In(db.School) }
	tokens := auth.Tokens{
		Key:      cfg.AccessKey,
		Sessions: sessions,
		SignedIn: func(email string) bool { return dataStore.Model().SignedIn(email) != "" },
		Now:      schoolNow,
	}
	db.RegisterFeeds(calendarMux, dataStore, schoolNow)
	whenPages := db.NewWhenPages(dataStore, cfg.Bucket, calendarStyle)
	whenPages.Register(calendarMux)
	invitations := db.RegisterInvitations(calendarMux, api, dataStore, pictures, tokens, db.InvitationConfig{
		Sender: cfg.CalendarMail.Sender, ReplyTo: cfg.CalendarMail.ReplyTo, SigningKey: cfg.CalendarMail.SigningKey, Origin: hosts.Origin(cfg.Domain),
	})
	birthdays := db.NewBirthdayMailer(dataStore, db.BirthdayMailConfig{Sender: cfg.BirthdayMail, Origin: hosts.Origin(cfg.Domain), Shared: cfg.Writer})
	shared := tools.New(tools.Deps{Search: search, Bucket: cfg.Bucket, Now: schoolNow})
	mcpMux := http.NewServeMux()
	mcp.Register(mcpMux, api, mcp.Deps{Data: dataStore, Tools: shared, Tokens: tokens})
	apps := []appSpec{
		{Key: "who", Title: "Helios Who?", Mux: mux, Preview: whoAbout.PreviewHead, Moved: map[string]string{"/groups/": "/g/", "/families/": "/g/", "/classrooms/": "/g/", "/grades/": "/g/", "/people/": "/p/"}},
		{Key: "home", Title: "Heliosian: Helios Community Apps", Mux: homeMux, Preview: homePages.PreviewHead},
		{Key: "team", Title: "HCA-Team: HCA Volunteer Portal", Mux: teamMux, Preview: activityPages.PreviewHead, Wrap: activityPages.Redirected, Moved: map[string]string{"/v/": "/g/", "/activities/": "/g/"}},
		{Key: "birthday", Title: "Helios Staff Birthdays", Mux: birthdayMux, Preview: birthdayAbout.PreviewHead},
		{Key: "celebrate", Title: "Helios Celebrate: Fun(d)raiser Parties", Mux: celebrateMux, Preview: partyPages.PreviewHead, Wrap: partyPages.Redirected, Moved: map[string]string{"/p/": "/g/", "/parties/": "/g/"}},
		{Key: "when", Title: "Helios When: The school year, day by day", Mux: calendarMux, Preview: whenPages.PreviewHead, Moved: map[string]string{"/e/": "/g/", "/events/": "/g/"}},
		{Key: "loop", Title: "Helios Loop", Mux: loopMux, Preview: loopAbout.PreviewHead, Moved: map[string]string{"/groups/": "/g/"}},
		{Key: "ask", Title: "Helios Ask", Mux: askMux, Preview: askAbout.PreviewHead},
		{Key: "admin", Title: "Helios Admin", Mux: adminMux},
		{Key: "wiki", Title: "Helios Wiki", Mux: wikiMux, Preview: wikiShare.PreviewHead},
		{Key: "mcp", Title: "Helios MCP", Mux: mcpMux},
	}
	ask.Register(askMux, api, ask.Sources{Data: dataStore, Tools: shared, Now: schoolNow}, cfg.Asker, spend, cfg.ChatKey, askAbout)
	feedback := db.NewFeedback(dataStore, cfg.Bucket, tokens, db.FeedbackConfig{Sender: cfg.Mail, Base: cfg.FeedbackBase, Filer: cfg.FeedbackFiler})
	feedback.RegisterTriage(api, appNameOf)
	feedback.Register(api, cfg.Domain, appNameOf)
	db.Register(api, dataStore, pictures, tokens)
	db.RegisterSearch(api, dataStore, search, tokens)
	geocode.NewSuggestions(cfg.Geocoder).Register(api)
	db.RegisterCompose(api, dataStore, cfg.Composer, tokens)
	db.RegisterDrafts(api, dataStore, cfg.Describer, tokens)
	db.RegisterQueues(api, dataStore, tokens)
	optIn := optInForm(func() string { return dataStore.Model().Setting("platform", "Helios Who Opt-In URL") })
	for _, a := range apps {
		a.Mux.Handle("GET "+OptInPath, optIn)
	}
	cfg.Ops.Bucket = cfg.Bucket
	external := ops.New(cfg.Ops)
	external.Register(adminMux, cfg.Hooks)
	db.RegisterDashboard(adminMux, api, dataStore, external)
	go dataStore.Tick()
	go vitals.Sampler()
	go func() {
		<-dataStore.Refreshed()
		pictures.Start()
	}()
	return &Core{
		Sessions: sessions, Data: dataStore, Pictures: pictures, Search: search, ListMail: listMail, Invites: invitations, Birthdays: birthdays,
		Spoof:   spoofing(dataStore),
		Settled: dataStore.Refreshed(),
		apps:    apps,
		api:     api,
	}
}

func (c *Core) SignedIn(email string) bool {
	return c.Data.Model().SignedIn(email) != ""
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
		h := moved(a.Moved, gate(a.Key, Logged(a.Key, withAPI(c.api, Files(a.Key, a.Mux)))))
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

func Production(domain, site string) (*http.Server, *db.Store) {
	cloud := os.Getenv("K_SERVICE") != ""
	if !cloud {
		intercept.Install(mail.Host, intercept.Mail(localMail))
	}
	sessionKey := env.Required("SESSION_KEY")
	sheet, err := data.NewSheet(spreadsheets.IDs(spreadsheets.All))
	if err != nil {
		logging.Fatal("load directory sheet", "error", err)
	}
	bucket, err := blob.Open(blob.MediaBucket)
	if err != nil {
		logging.Fatal("media bucket", "error", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		logging.Fatal("vertex embedder", "error", err)
	}
	chatKey := hmac.New(sha256.New, []byte(sessionKey))
	chatKey.Write([]byte("ask chats"))
	anthropicKey := env.Required("ANTHROPIC_API_KEY")
	geocoder := geocode.New(env.Required("GOOGLE_MAPS_SERVER_KEY"))
	github := githubApp()
	core := NewCore(Config{
		Domain:        site,
		Source:        sheet,
		Writer:        sheet,
		Geocoder:      geocoder,
		Bucket:        bucket,
		ChatKey:       chatKey.Sum(nil),
		AccessKey:     auth.AccessKey([]byte(sessionKey)),
		BrowserKey:    env.Required("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch:   ImageSearchKeys(),
		Describer:     describe.New(anthropicKey, spend),
		Mail:          newMailer(mailFrom),
		CelebrateMail: newMailer(celebrateMailFrom),
		CalendarMail:  calendarMail(),
		BirthdayMail:  newMailer(birthdayMailFrom),
		FeedbackFiler: github,
		FeedbackBase:  feedbackBase,
		ListMail:      listMail(sessionKey),
		Asker:         ask.NewClaude(anthropicKey),
		Composer:      db.NewComposer(anthropicKey, spend),
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(),
		Ops:           OpsDeps(github, env.Required("ANTHROPIC_ADMIN_KEY")),
		Hooks:         ops.Hooks{GitHubSecret: []byte(env.Required("GITHUB_WEBHOOK_SECRET")), Audience: "https://admin." + site + ops.BuildHookPath, PushAccount: runtimeAccount},
	})
	muxes := core.Muxes()
	client := env.Required("GOOGLE_CLIENT_ID")
	auths := map[string]*auth.Auth{}
	for _, a := range core.apps {
		gate := auth.New(domain, client, []byte(sessionKey), auth.Login{Title: a.Title}, core.SignedIn, []string{OptInPath}, core.Sessions)
		gate.Spoof = core.Spoof
		gate.Preview = a.Preview
		gate.Register(a.Mux)
		auths[a.Key] = gate
	}
	server := Server(domain, core.Handlers(func(key string, next http.Handler) http.Handler {
		return auths[key].Wrap(next)
	}), core.Aliased())
	db.StartWarmer(core.Data)
	if cloud {
		debug.SetMemoryLimit(memoryLimit)
		watcher := calendarWatcher(core, sessionKey, anthropicKey)
		muxes["when"].Handle("POST "+db.CalendarHookPath, watcher)
		server.RegisterOnShutdown(func() { core.Data.Add(watcher.Stop) })
		go func() {
			<-core.Settled
			core.ListMail.Start()
			core.Invites.Start()
			core.Birthdays.Start()
			db.StartConsent(core.Data, sheet)
			core.Search.StartMaking(anthropicKey)
			watcher.Start()
			db.NewExtractor(core.Data, bucket, anthropicKey)
			db.StartClassifier(core.Data, bucket, anthropicKey)
			db.StartDigest(core.Data, bucket, anthropicKey, time.Now)
			db.StartSweeper(core.Data, bucket)
			db.StartFetcher(core.Data, bucket)
			db.StartThumbnailer(core.Data, bucket)
			db.StartGeocoder(core.Data, geocoder)
		}()
	} else {
		db.StartIdleWorkers(core.Data)
	}
	<-core.Search.Ready()
	return server, core.Data
}

const memoryLimit = 3 << 30

func calendarWatcher(core *Core, sessionKey, anthropicKey string) *db.CalendarWatcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	return db.NewCalendarWatcher(core.Data, cal, anthropicKey, hex.EncodeToString(mac.Sum(nil)))
}
