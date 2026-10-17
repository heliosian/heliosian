package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"image/color"
	"log/slog"
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
	"heliosian/internal/model"
	"heliosian/internal/ops"
	"heliosian/internal/serve"
	"heliosian/internal/sharecard"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/static"
	"heliosian/internal/store"
	"heliosian/internal/tools"
	"heliosian/internal/vitals"
)

const deployOverlap = 30 * time.Second

const localMail = "local/mail"

const OptInPath = "/optin"

var spend = claude.NewLimiter()

type Config struct {
	Domain        string
	Source        data.Source
	Writer        data.Writer
	Geocoder      *geocode.Client
	Bucket        *blob.Bucket
	Store         *blob.Store
	IDKey         []byte
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
}

type Core struct {
	Store     *model.Store
	Data      *db.Store
	Pictures  *db.Pictures
	Search    *db.Searcher
	Queue     *store.Queue
	ListMail  *db.ListMailer
	Invites   *db.Invitations
	Birthdays *db.BirthdayMailer
	Spoof     *auth.Spoof
	Settled   <-chan struct{}
	apps      []appSpec
}

func NewCore(cfg Config) *Core {
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		logging.Fatal("register manifest mime type", "error", err)
	}
	queue := store.NewQueue()
	queue.Register(cfg.Store.Load)
	cfg.ImageSearch.Stock = imagesearch.NewStock(cfg.Bucket, cfg.Store)
	cfg.ImageSearch.Limits = imagesearch.NewLimits()
	homeImages := blob.NewImages(cfg.Store, "home")
	teamImages := blob.NewImages(cfg.Store, "team")
	celebrateImages := blob.NewImages(cfg.Store, "celebrate")
	whenImages := blob.NewImages(cfg.Store, "when", "celebrate", "team")
	models, err := model.NewStore(cfg.Source, cfg.Writer, queue, model.Deps{
		IDKey: cfg.IDKey, Photos: cfg.Store, Static: static.Files{Root: "web/who"},
		Objects: cfg.Bucket, Embedder: cfg.Embedder,
	})
	if err != nil {
		logging.Fatal("load the models", "error", err)
	}
	dataStore, err := db.NewStore(cfg.Source, cfg.Writer, queue, db.NewSearchIndex())
	if err != nil {
		logging.Fatal("load the data sheets", "error", err)
	}
	pictures := db.NewPictures(dataStore, queue, cfg.Bucket)
	db.NewMembershipMailer(dataStore, queue, db.MembershipMailConfig{Sender: cfg.Mail, Celebrate: cfg.CelebrateMail, Origin: hosts.Origin(cfg.Domain)})
	search := db.NewSearcher(dataStore, queue, cfg.Bucket, cfg.Embedder, hosts.Origin(cfg.Domain))
	go models.Locate(cfg.Geocoder)
	taglineOf := func(key string) func() string {
		return func() string {
			return dataStore.Model().App(key)["subtitle"]
		}
	}
	appName := func(key string) func() string {
		return func() string {
			return dataStore.Model().App(key)["name"]
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
	mux := http.NewServeMux()
	registerDirectoryPages(mux, whoAbout, cfg.BrowserKey)
	blob.Register(mux, cfg.Store, "pronunciation")
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	calendarMux := http.NewServeMux()
	for _, page := range []string{"/{$}", "/c/{token}", "/day/{date}", "/mine", "/mine/{list}", "/admin", "/e/{id...}", "/events/{id...}"} {
		calendarMux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/when/index.html")
		})
	}
	whenSearch := cfg.ImageSearch
	whenSearch.UserAgent = "Helios When image search (+https://when.heliosian.com)"
	whenSearch.Register(calendarMux, "/api/when", whenImages.Folder(), imagesearch.Members)
	homeMux := http.NewServeMux()
	homePages := db.NewHomePages(dataStore, homeStyle)
	homePages.Register(homeMux)
	homeSearch := cfg.ImageSearch
	homeSearch.UserAgent = "Heliosian image search (+https://heliosian.com)"
	homeSearch.Register(homeMux, "/api/home", homeImages.Folder(), imagesearch.Members)
	teamMux := http.NewServeMux()
	for _, page := range []string{"/{$}", "/my", "/my/{email}", "/calendar", "/approvals", "/admin", "/years/{year}", "/activities/{path...}", "/v/{path...}"} {
		teamMux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/team/index.html")
		})
	}
	teamSearch := cfg.ImageSearch
	teamSearch.UserAgent = "HCA-Team image search (+https://team.heliosian.com)"
	teamSearch.Register(teamMux, "/api/team", teamImages.Folder(), imagesearch.Members)
	activityPages := db.NewActivityPages(dataStore, cfg.Bucket, teamStyle)
	activityPages.Register(teamMux)
	birthdayMux := http.NewServeMux()
	birthdayMux.Handle("GET /open/share/about.png", birthdayAbout)
	for _, page := range []string{"/{$}", "/jobs", "/process", "/calendar", "/charities", "/charities/{name}", "/newsletters", "/newsletters/{date}", "/skipped", "/unassigned", "/admin", "/staff/{handle}"} {
		birthdayMux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/birthday/index.html")
		})
	}
	celebrateMux := http.NewServeMux()
	celebrateSearch := cfg.ImageSearch
	celebrateSearch.UserAgent = "Helios Celebrate image search (+https://celebrate.heliosian.com)"
	celebrateSearch.Register(celebrateMux, "/api/celebrate", celebrateImages.Folder(), imagesearch.Members)
	partyPages := db.NewPartyPages(dataStore, cfg.Bucket, celebrateStyle)
	partyPages.Register(celebrateMux)
	askMux := http.NewServeMux()
	db.RegisterMailHook(askMux, dataStore, queue, pictures, cfg.ArtifactsMail.SigningKey)
	loopMux := http.NewServeMux()
	loopMux.Handle("GET /open/share/about.png", loopAbout)
	for _, page := range []string{"/{$}", "/new", "/groups/{key}"} {
		loopMux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/loop/index.html")
		})
	}
	listMail := db.RegisterListMail(loopMux, dataStore, queue, pictures, cfg.ListMail)
	deliveries := db.NewDeliveries(dataStore, queue, cfg.ListMail.SigningKey)
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
	for _, page := range []string{"/{$}", "/new", "/p/{path...}"} {
		wikiMux.HandleFunc("GET "+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/wiki/index.html")
		})
	}
	wikiImages, _ := blob.ImageFolder("wiki")
	cfg.ImageSearch.Register(wikiMux, "/api/wiki", wikiImages, imagesearch.Members)
	wikiShare := db.NewWikiShare(dataStore, pictures, appName("wiki"))
	wikiShare.Register(wikiMux)
	schoolNow := func() time.Time { return time.Now().In(model.Location) }
	tokens := auth.Tokens{
		Key:      cfg.AccessKey,
		Sessions: models,
		SignedIn: func(email string) bool { return dataStore.Model().SignedIn(email) != "" },
		Now:      schoolNow,
	}
	db.RegisterFeeds(calendarMux, dataStore, schoolNow)
	whenPages := db.NewWhenPages(dataStore, cfg.Bucket, calendarStyle)
	whenPages.Register(calendarMux)
	invitations := db.RegisterInvitations(calendarMux, dataStore, queue, pictures, tokens, schoolNow, db.InvitationConfig{
		Sender: cfg.CalendarMail.Sender, ReplyTo: cfg.CalendarMail.ReplyTo, SigningKey: cfg.CalendarMail.SigningKey, Origin: hosts.Origin(cfg.Domain),
	})
	birthdays := db.NewBirthdayMailer(dataStore, queue, db.BirthdayMailConfig{Sender: cfg.BirthdayMail, Origin: hosts.Origin(cfg.Domain), Shared: cfg.Writer})
	shared := tools.New(tools.Deps{Data: dataStore, Search: search, Bucket: cfg.Bucket, Origin: hosts.Origin(cfg.Domain), Now: schoolNow})
	mcpMux := http.NewServeMux()
	mcp.Register(mcpMux, mcp.Deps{
		Data:     dataStore,
		Tools:    shared,
		Key:      cfg.AccessKey,
		Sessions: models,
		Now:      schoolNow,
	})
	apps := []appSpec{
		{Key: "who", Title: "Helios Who?", Mux: mux, Preview: whoAbout.PreviewHead},
		{Key: "home", Title: "Heliosian: Helios Community Apps", Mux: homeMux, Preview: homePages.PreviewHead},
		{Key: "team", Title: "HCA Volunteer Portal", Mux: teamMux, Preview: activityPages.PreviewHead, Wrap: activityPages.Redirected},
		{Key: "birthday", Title: "Helios Staff Birthdays", Mux: birthdayMux, Preview: birthdayAbout.PreviewHead},
		{Key: "celebrate", Title: "Helios Celebrate: Fun(d)raiser Parties", Mux: celebrateMux, Preview: partyPages.PreviewHead, Wrap: partyPages.Redirected},
		{Key: "when", Title: "Helios When: The school year, day by day", Mux: calendarMux, Preview: whenPages.PreviewHead},
		{Key: "loop", Title: "Helios Loop", Mux: loopMux, Preview: loopAbout.PreviewHead},
		{Key: "ask", Title: "Helios Ask", Mux: askMux, Preview: askAbout.PreviewHead},
		{Key: "admin", Title: "Helios Admin", Mux: adminMux},
		{Key: "wiki", Title: "Helios Wiki", Mux: wikiMux, Preview: wikiShare.PreviewHead},
		{Key: "mcp", Title: "Helios MCP", Mux: mcpMux},
	}
	ask.Register(askMux, ask.Sources{Data: dataStore, Tools: shared, Origin: hosts.Origin(cfg.Domain), Now: schoolNow}, cfg.Asker, spend, cfg.ChatKey, askAbout)
	feedback := db.NewFeedback(dataStore, queue, cfg.Bucket, tokens, schoolNow, db.FeedbackConfig{Sender: cfg.Mail, Base: cfg.FeedbackBase, Filer: cfg.FeedbackFiler})
	feedback.RegisterTriage(homeMux, func(key string) string { return appName(key)() })
	optIn := optInForm(func() string { return models.Model().Config.PrivacyLinks.HeliosWhoOptIn })
	suggestions := geocode.NewSuggestions(cfg.Geocoder)
	for _, a := range apps {
		db.Register(a.Mux, dataStore, queue, pictures, tokens, schoolNow)
		db.RegisterSearch(a.Mux, dataStore, search, tokens, schoolNow)
		a.Mux.Handle("GET "+OptInPath, optIn)
		feedback.Register(a.Mux, a.Key, appName(a.Key))
		suggestions.Register(a.Mux)
		folders := []string{"photos"}
		if folder, ok := blob.ImageFolder(a.Key); ok {
			folders = append(folders, folder)
		}
		blob.Register(a.Mux, cfg.Store, folders...)
	}
	db.RegisterCompose(adminMux, dataStore, cfg.Composer, tokens, schoolNow)
	db.RegisterDrafts(loopMux, dataStore, cfg.Describer, tokens, schoolNow)
	db.RegisterDrafts(teamMux, dataStore, cfg.Describer, tokens, schoolNow)
	db.RegisterDrafts(birthdayMux, dataStore, cfg.Describer, tokens, schoolNow)
	db.RegisterDrafts(homeMux, dataStore, cfg.Describer, tokens, schoolNow)
	db.RegisterQueues(adminMux, dataStore, queue, tokens, schoolNow)
	cfg.Ops.Buckets = map[string]ops.Measurable{blob.MediaBucket: cfg.Bucket}
	external := ops.New(cfg.Ops)
	external.Register(adminMux, cfg.Hooks)
	db.RegisterDashboard(adminMux, dataStore, queue, external)
	go queue.Tick()
	go vitals.Sampler()
	go func() {
		<-queue.Refreshed()
		pictures.Start()
	}()
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		queue.Refresh()
	})
	return &Core{
		Store: models, Data: dataStore, Pictures: pictures, Search: search, Queue: queue, ListMail: listMail, Invites: invitations, Birthdays: birthdays,
		Spoof:   spoofing(dataStore),
		Settled: queue.Refreshed(),
		apps:    apps,
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

func Production(domain, site string) (*http.Server, *store.Queue) {
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
	store := blob.New(bucket)
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
		Store:         store,
		IDKey:         []byte(env.Required("ID_KEY")),
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
		ArtifactsMail: artifactsMail(bucket),
		Ops:           OpsDeps(github, env.Required("ANTHROPIC_ADMIN_KEY")),
		Hooks:         ops.Hooks{GitHubSecret: []byte(env.Required("GITHUB_WEBHOOK_SECRET")), Audience: "https://admin." + site + ops.BuildHookPath, PushAccount: runtimeAccount},
	})
	muxes := core.Muxes()
	client := env.Required("GOOGLE_CLIENT_ID")
	auths := map[string]*auth.Auth{}
	for _, a := range core.apps {
		gate := auth.New(domain, client, []byte(sessionKey), auth.Login{Title: a.Title}, core.SignedIn, []string{OptInPath}, core.Store)
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
		server.RegisterOnShutdown(func() { core.Queue.Add(watcher.Stop) })
		go func() {
			<-core.Settled
			core.ListMail.Start()
			core.Invites.Start()
			core.Birthdays.Start()
			db.StartConsent(core.Data, core.Queue, sheet)
			core.Search.StartMaking(anthropicKey)
			watcher.Start()
			db.NewExtractor(core.Data, core.Queue, bucket, anthropicKey)
			db.StartClassifier(core.Data, core.Queue, bucket, anthropicKey)
			db.StartDigest(core.Data, core.Queue, bucket, anthropicKey, time.Now)
			db.StartSweeper(core.Data, core.Queue, bucket)
			db.StartFetcher(core.Data, core.Queue, bucket)
			db.StartGeocoder(core.Data, core.Queue, geocoder)
		}()
	} else {
		db.StartIdleWorkers(core.Data)
	}
	return server, core.Queue
}

const memoryLimit = 3 << 30

func calendarWatcher(core *Core, sessionKey, anthropicKey string) *db.CalendarWatcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	return db.NewCalendarWatcher(core.Data, core.Queue, cal, anthropicKey, hex.EncodeToString(mac.Sum(nil)))
}
