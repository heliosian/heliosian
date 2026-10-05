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
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/describe"
	"heliosian/internal/digest"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/geocode"
	"heliosian/internal/imagesearch"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/serve"
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
	ImportKey     []byte
	ChatKey       []byte
	BrowserKey    string
	ImageSearch   imagesearch.Search
	Mail          *mail.Mailgun
	CelebrateMail *mail.Mailgun
	CalendarMail  model.CalendarMail
	BirthdayMail  *mail.Mailgun
	BirthdayBase  string
	FeedbackFiler *feedback.GitHubApp
	FeedbackBase  string
	Describer     *describe.Describer
	Loop          model.ListMail
	Asker         *ask.Claude
	Embedder      *artifacts.Vertex
	ArtifactsMail artifacts.Inbox
	Digest        *digest.Claude
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
	Documents *model.DocumentFiler
	Queue     *store.Queue
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
		Calendar: whenImages, Parties: celebrateImages, Activities: teamImages, Home: homeImages,
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
	settled := make(chan struct{})
	go func() {
		<-settled
		db.NewExtractor(dataStore, queue, cfg.Bucket)
	}()
	search := db.NewSearcher(dataStore, queue, cfg.Bucket, cfg.Embedder)
	go models.Locate(cfg.Geocoder)
	taglineOf := func(key string) func() string {
		return func() string {
			if key == "home" {
				return model.HomeApp.Tagline
			}
			for _, a := range models.Model().Home.AppList() {
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
				return model.HomeApp.Name
			}
			for _, a := range models.Model().Home.AppList() {
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
	homeStyle := model.HomeCardStyle(appName("home"), taglineOf("home"))
	teamStyle := model.ActivitiesCardStyle(appName("team"), taglineOf("team"))
	celebrateStyle := model.PartiesCardStyle(appName("celebrate"), taglineOf("celebrate"))
	calendarStyle := model.CalendarCardStyle(appName("when"), taglineOf("when"))
	digest.Start(models, queue, cfg.Digest)
	mux := http.NewServeMux()
	who.Register(mux, models, whoAbout)
	model.RegisterDirectoryMedia(mux, cfg.Store)
	blob.Register(mux, cfg.Store, "pronunciation")
	mux.Handle("GET /{$}", http.RedirectHandler("/people", http.StatusFound))
	calendarMux := http.NewServeMux()
	hooks := model.RegisterCalendar(calendarMux, model.CalendarDeps{
		Store:  models,
		Images: whenImages,
		Search: cfg.ImageSearch,
		Mail:   cfg.CalendarMail,
		Style:  calendarStyle,
		Queue:  queue,
		IDKey:  cfg.IDKey,
	})
	homeMux := http.NewServeMux()
	home := model.RegisterHome(homeMux, model.HomeDeps{
		Store:  models,
		Images: homeImages,
		Search: cfg.ImageSearch,
		Style:  homeStyle,
	})
	feedbackAdmin := model.RegisterFeedbackAdmin(homeMux, models, cfg.Bucket, cfg.FeedbackFiler)
	teamMux := http.NewServeMux()
	activities := model.RegisterActivities(teamMux, model.ActivitiesDeps{
		Store:     models,
		Images:    teamImages,
		Calendar:  hooks,
		Search:    cfg.ImageSearch,
		Mailer:    cfg.Mail,
		Style:     teamStyle,
		Describer: cfg.Describer,
	})
	birthdayMux := http.NewServeMux()
	celebrateMux := http.NewServeMux()
	parties := model.RegisterParties(celebrateMux, model.PartiesDeps{
		Store:    models,
		Images:   celebrateImages,
		Calendar: hooks,
		Search:   cfg.ImageSearch,
		Mailer:   cfg.CelebrateMail,
		Style:    celebrateStyle,
	})
	askMux := http.NewServeMux()
	loopMail := cfg.Loop
	documents := model.RegisterDocuments(askMux, models, cfg.Embedder, queue, cfg.ArtifactsMail)
	loopMail.Documents = documents
	loopMux := http.NewServeMux()
	model.RegisterEmailLists(loopMux, model.EmailListsDeps{
		Store:     models,
		Media:     cfg.Store,
		Mail:      loopMail,
		Describer: cfg.Describer,
		About:     loopAbout,
	})
	askAbout := ask.About(appName("ask"), taglineOf("ask"))
	adminMux := http.NewServeMux()
	adminMux.Handle("GET /{$}", http.RedirectHandler("/resources", http.StatusFound))
	for _, page := range []string{"resources", "query", "search", "policies", "erd"} {
		adminMux.HandleFunc("GET /"+page, func(w http.ResponseWriter, r *http.Request) {
			serve.File(w, r, "web/admin/"+page+"/index.html")
		})
	}
	apps := []appSpec{
		{Key: "who", Title: "Helios Who?", Mux: mux, Preview: whoAbout.PreviewHead},
		{Key: "home", Title: "Heliosian: Helios Community Apps", Mux: homeMux, Preview: model.HomePreviewHead(models, homeStyle)},
		{Key: "team", Title: "HCA Volunteer Portal", Mux: teamMux, Preview: model.ActivitiesPreviewHead(models, teamStyle), Wrap: func(next http.Handler) http.Handler {
			return model.ActivitiesRedirected(models, next)
		}},
		{Key: "birthday", Title: "Helios Staff Birthdays", Mux: birthdayMux, Preview: birthdayAbout.PreviewHead},
		{Key: "celebrate", Title: "Helios Celebrate: Fun(d)raiser Parties", Mux: celebrateMux, Preview: model.PartiesPreviewHead(models, celebrateStyle)},
		{Key: "when", Title: "Helios When: The school year, day by day", Mux: calendarMux, Preview: hooks.PreviewHead()},
		{Key: "loop", Title: "Helios Loop", Mux: loopMux, Preview: loopAbout.PreviewHead},
		{Key: "ask", Title: "Helios Ask", Mux: askMux, Preview: askAbout.PreviewHead},
		{Key: "admin", Title: "Helios Admin", Mux: adminMux},
	}
	registry := model.NewRegistry(models, queue, hooks, parties, activities, home, feedbackAdmin, documents, cfg.BrowserKey)
	ask.Register(askMux, ask.Sources{Registry: registry, Now: time.Now}, cfg.Asker, spend, cfg.ChatKey, askAbout)
	model.RegisterBirthdays(birthdayMux, model.BirthdaysDeps{
		Store:     models,
		Queue:     queue,
		Describer: cfg.Describer,
		Mailer:    cfg.BirthdayMail,
		Base:      cfg.BirthdayBase,
		About:     birthdayAbout,
		Taken:     registry.Taken,
	})
	notifier := model.FeedbackNotifier{Sender: cfg.Mail, Base: cfg.FeedbackBase, SuperAdmins: func() []string { return models.Model().Config.SuperAdmins }}
	feedbackIntake := model.NewFeedbackIntake(models, cfg.Bucket, notifier.Notify)
	optIn := who.OptInForm(func() string { return models.Model().Config.PrivacyLinks.HeliosWhoOptIn })
	suggestions := geocode.NewSuggestions(cfg.Geocoder)
	schoolNow := func() time.Time { return time.Now().In(model.Location) }
	for _, a := range apps {
		registry.Register(a.Mux)
		db.Register(a.Mux, dataStore, queue, pictures, cfg.ImportKey, schoolNow)
		db.RegisterSearch(a.Mux, dataStore, search, cfg.ImportKey, schoolNow)
		a.Mux.Handle("GET "+OptInPath, optIn)
		model.RegisterFeedback(a.Mux, a.Key, appName(a.Key), feedbackIntake)
		suggestions.Register(a.Mux)
		folders := []string{"photos"}
		if folder, ok := blob.ImageFolder(a.Key); ok {
			folders = append(folders, folder)
		}
		blob.Register(a.Mux, cfg.Store, folders...)
	}
	go queue.Tick()
	time.AfterFunc(deployOverlap, func() {
		slog.Info("reading again for the previous revision's last writes")
		queue.Refresh()
		queue.Add(func() { close(settled) })
	})
	return &Core{
		Store: models, Data: dataStore, Pictures: pictures, Search: search, Documents: documents, Queue: queue,
		Spoof:   &auth.Spoof{Allowed: models.IsSuperAdmin, Person: spoofPerson(models)},
		Settled: settled,
		apps:    apps,
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
		ImportKey:     []byte(env.Required("IMPORT_KEY")),
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
		Digest:        digest.New(anthropicKey),
		Embedder:      embedder,
		ArtifactsMail: artifactsMail(bucket),
	})
	muxes := core.Muxes()
	client := env.Required("GOOGLE_CLIENT_ID")
	auths := map[string]*auth.Auth{}
	for _, a := range core.apps {
		gate := auth.New(domain, client, []byte(sessionKey), auth.Login{Title: a.Title}, core.Store.Member, []string{OptInPath}, core.Store)
		gate.Spoof = core.Spoof
		gate.Preview = a.Preview
		gate.Register(a.Mux)
		auths[a.Key] = gate
	}
	server := Server(domain, core.Handlers(func(key string, next http.Handler) http.Handler {
		return auths[key].Wrap(next)
	}), core.Aliased())
	if os.Getenv("K_SERVICE") != "" {
		db.StartConsent(core.Data, core.Queue, core.Pictures, sheet)
		core.Search.StartMaking(anthropicKey)
		go func() {
			<-core.Settled
			db.StartClassifier(core.Data, core.Queue, bucket, anthropicKey)
			db.StartSweeper(core.Data, core.Queue, bucket)
			db.StartFetcher(core.Data, core.Queue, bucket)
		}()
		watcher := calendarWatcher(core, sessionKey, anthropicKey)
		muxes["when"].Handle("POST "+db.CalendarHookPath, watcher)
		watcher.Start()
		server.RegisterOnShutdown(func() { core.Queue.Add(watcher.Stop) })
	}
	return server, core.Queue
}

func calendarWatcher(core *Core, sessionKey, anthropicKey string) *db.CalendarWatcher {
	cal, err := gcal.NewService(context.Background(), option.WithScopes(gcal.CalendarReadonlyScope))
	if err != nil {
		logging.Fatal("calendar client", "error", err)
	}
	mac := hmac.New(sha256.New, []byte(sessionKey))
	mac.Write([]byte("calendar watch"))
	return db.NewCalendarWatcher(core.Data, core.Queue, core.Pictures, cal, anthropicKey, hex.EncodeToString(mac.Sum(nil)))
}
