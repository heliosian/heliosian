package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/app"
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/blob"
	"heliosian/internal/capture"
	"heliosian/internal/claude"
	"heliosian/internal/data"
	"heliosian/internal/db"
	"heliosian/internal/describe"
	"heliosian/internal/devcache"
	"heliosian/internal/devtls"
	"heliosian/internal/digest"
	"heliosian/internal/env"
	"heliosian/internal/feedback"
	"heliosian/internal/geocode"
	"heliosian/internal/intercept"
	"heliosian/internal/logging"
	"heliosian/internal/mail"
	"heliosian/internal/model"
	"heliosian/internal/ops"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
)

const (
	logPath      = "local/heliosian-server.log"
	mailDir      = "local/mail"
	snapshotRoot = "local/snapshot"
)

const sampleUser = "jordan.whitfield@heliosschool.org"

func main() {
	real := flag.Bool("real", false, "serve the production assembly in the foreground")
	detach := flag.Bool("detach", false, "launch --real in the background with a log file")
	capturePath := flag.String("capture", "", "serve sample data in-process, capture this url (on any app's local hostname) as a PNG, and exit")
	out := flag.String("out", "local/screenshots/capture.png", "output png path for --capture")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing, for --capture")
	width := flag.Int("width", 1280, "viewport width for --capture")
	height := flag.Int("height", 800, "viewport height for --capture")
	click := flag.String("click", "", "css selectors to click once --wait is visible, separated by |, for --capture")
	settle := flag.Duration("settle", 0, "how long to wait after the last click before capturing, for --capture")
	as := flag.String("as", "", "view as this directory address through Spoof Mode, for --capture")
	snapshot := flag.Bool("snapshot", false, "serve the snapshot tools/snapshot took under "+snapshotRoot+", disconnected: writes stay in memory and the media bucket is read but never written")
	email := flag.String("email", "", "the directory address to sign in as, for --snapshot")
	flag.Parse()
	slog.SetDefault(logging.Console())

	switch {
	case *snapshot:
		if *email == "" {
			logging.Fatal("--snapshot needs --email, the directory address to sign in as")
		}
		app.Serve(snapshotServer(*email))
	case *real:
		devcache.Install()
		app.Serve(localTLS(app.Production(app.DevDomain, app.DevDomain+":"+app.Port())))
	case *detach:
		detachReal()
	case *capturePath != "":
		cookie := "heliosian-quan-shown=1"
		if *as != "" {
			cookie += "; spoof=" + auth.SpoofToken([]byte("sample"), sampleUser, *as, time.Now().Add(time.Hour))
		}
		captureOnce(capture.Options{URL: *capturePath, Wait: *wait, Width: *width, Height: *height, Cookie: cookie, Click: *click, Settle: *settle}, *out)
	default:
		app.Serve(sampleServer())
	}
}

func sampleServer() (*http.Server, *store.Queue) {
	intercept.GoogleLogin("local/google")
	bucket := blob.NewMemoryBucket()
	fillSampleBucket(bucket)
	core := localCore(&data.Dir{Root: "sampledata"}, bucket)
	saved, err := filepath.Glob("sampledata/artifacts/*.json")
	if err != nil {
		logging.Fatal("list the sample documents", "error", err)
	}
	for _, path := range saved {
		if err := core.Documents.FileSaved(context.Background(), access.System("sample"), path); err != nil {
			logging.Fatal("file a sample document", "path", path, "error", err)
		}
	}
	slog.Info("serving sample data", "as", sampleUser)
	return localServer(core, sampleUser)
}

func snapshotServer(email string) (*http.Server, *store.Queue) {
	devcache.Install()
	base, err := blob.Open(blob.MediaBucket)
	if err != nil {
		logging.Fatal("media bucket", "error", err)
	}
	core := localCore(&data.Dir{Root: snapshotRoot}, blob.Overlay(base))
	slog.Info("serving the snapshot, disconnected", "dir", snapshotRoot, "as", email)
	return localServer(core, email)
}

func localCore(dir *data.Dir, bucket *blob.Bucket) *app.Core {
	intercept.Install(mail.Host, mailFiles{dir: mailDir})
	intercept.Install(intercept.ClaudeHost, intercept.Claude())
	intercept.Install(intercept.GeocodeHost, intercept.Geocode())
	intercept.Install(intercept.PlacesHost, intercept.Places())
	intercept.Install(intercept.VertexHost, intercept.Vertex())
	intercept.Install(intercept.GitHubHost, intercept.GitHub())
	intercept.Install(intercept.CloudBuildHost, intercept.CloudBuild())
	intercept.Install(intercept.CloudRunHost, intercept.CloudRun())
	githubKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		logging.Fatal("sample github app key", "error", err)
	}
	embedder, err := artifacts.NewVertex()
	if err != nil {
		logging.Fatal("vertex embedder", "error", err)
	}
	media := blob.New(bucket)
	core := app.NewCore(app.Config{
		Domain:        app.DevDomain + ":" + app.Port(),
		Source:        dir,
		Writer:        dir,
		Geocoder:      geocode.New("sample"),
		Bucket:        bucket,
		Store:         media,
		IDKey:         []byte("sample"),
		ChatKey:       []byte("sample"),
		AccessKey:     []byte("sample"),
		BrowserKey:    os.Getenv("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch:   app.ImageSearchKeys(),
		Describer:     describe.New("sample", claude.NewLimiter()),
		Mail:          mail.NewMailgun("sample", "HCA-Team <hca@example.org>"),
		CelebrateMail: mail.NewMailgun("sample", "Helios Celebrate <celebrate@example.org>"),
		CalendarMail:  model.CalendarMail{Sender: mail.NewMailgun("sample", "Helios When <when@example.org>"), ReplyTo: "Helios When <rsvp@reply.example.org>", Key: []byte("sample")},
		BirthdayMail:  mail.NewMailgun("sample", "Helios Staff Birthdays <birthday@example.org>"),
		BirthdayBase:  "https://birthday.heliosiandev.com:" + app.Port(),
		FeedbackBase:  "https://home.heliosiandev.com:" + app.Port(),
		ListMail:      db.ListMailConfig{Sender: mail.NewMailgun("sample", ""), Key: []byte("sample"), Base: "https://loop.heliosiandev.com:" + app.Port()},
		Asker:         ask.NewClaude("sample"),
		Embedder:      embedder,
		ArtifactsMail: artifacts.Inbox{Bucket: bucket},
		Digest:        digest.New("sample"),
		Composer:      db.NewComposer("sample", claude.NewLimiter()),
		Ops:           app.OpsDeps(&feedback.GitHubApp{ID: "sample", PrivateKey: githubKey}, "sample"),
		Hooks:         ops.Hooks{GitHubSecret: []byte("sample"), Audience: "https://admin." + app.DevDomain + ":" + app.Port() + ops.BuildHookPath, PushAccount: "sample@example.org"},
	})
	core.Search.StartMaking("sample")
	return core
}

func localServer(core *app.Core, user string) (*http.Server, *store.Queue) {
	signIn := auth.New(app.DevDomain, "", []byte("sample"), auth.Login{}, core.SignedIn, []string{app.OptInPath}, core.Store)
	signIn.Spoof = core.Spoof
	for _, m := range core.Muxes() {
		m.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
		signIn.RegisterSpoof(m)
	}
	return localTLS(app.Server(app.DevDomain, core.Handlers(func(_ string, next http.Handler) http.Handler {
		return signIn.Fixed(user, next)
	}), core.Aliased()), core.Queue)
}

func fillSampleBucket(bucket *blob.Bucket) {
	if err := bucket.FillFrom("sampledata/bucket"); err != nil {
		logging.Fatal("fill the sample bucket", "error", err)
	}
}

func localTLS(server *http.Server, queue *store.Queue) (*http.Server, *store.Queue) {
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{devtls.Certificate(app.DevDomain)}}
	return server, queue
}

func detachReal() {
	env.Required("SESSION_KEY")
	spreadsheets.IDs(spreadsheets.All)

	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		logging.Fatal("create local directory", "error", err)
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		logging.Fatal("create server log", "error", err)
	}
	self, err := os.Executable()
	if err != nil {
		logging.Fatal("resolve own binary", "error", err)
	}
	cmd := exec.Command(self, "--real")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logging.Fatal("start server", "error", err)
	}

	fmt.Printf("log: %s\n", logPath)
	fmt.Printf("stop with: kill -- -%d\n", cmd.Process.Pid)
}

func captureOnce(opts capture.Options, out string) {
	server, queue := sampleServer()
	served := make(chan error, 1)
	go func() {
		served <- app.ListenAndServe(server)
	}()
	base := "https://who.heliosiandev.com:" + app.Port()
	probe := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	ready := false
	for range 100 {
		select {
		case err := <-served:
			logging.Fatal("server", "error", err)
		default:
		}
		resp, err := probe.Get(base + "/people")
		if err == nil {
			resp.Body.Close()
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		logging.Fatal("server did not become ready", "base", base)
	}

	png, captureErr := capture.PNG(opts)
	if err := server.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown", "error", err)
	}
	<-served
	<-queue.Drain()
	if captureErr != nil {
		logging.Fatal("capture", "error", captureErr)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		logging.Fatal("create output dir", "error", err)
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		logging.Fatal("write capture", "out", out, "error", err)
	}
	slog.Info("captured", "url", opts.URL, "out", out)
}
