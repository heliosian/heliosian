package main

import (
	"context"
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
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/capture"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/devcache"
	"heliosian/internal/devtls"
	"heliosian/internal/env"
	"heliosian/internal/geocode"
	"heliosian/internal/intercept"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
	"heliosian/internal/spreadsheets"
	"heliosian/internal/store"
	"heliosian/internal/when"
)

const logPath = "local/heliosian-server.log"

const sampleUser = "jordan.whitfield@heliosschool.org"

func main() {
	email := flag.String("email", "ian.gulliver@heliosschool.org", "session email for --detach's minted cookie")
	real := flag.Bool("real", false, "serve the production assembly in the foreground")
	detach := flag.Bool("detach", false, "launch --real in the background with a log file and a minted cookie")
	capturePath := flag.String("capture", "", "serve sample data in-process, capture this url (on any app's local hostname) as a PNG, and exit")
	out := flag.String("out", "local/screenshots/capture.png", "output png path for --capture")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing, for --capture")
	width := flag.Int("width", 1280, "viewport width for --capture")
	height := flag.Int("height", 800, "viewport height for --capture")
	click := flag.String("click", "", "css selectors to click once --wait is visible, separated by |, for --capture")
	settle := flag.Duration("settle", 0, "how long to wait after the last click before capturing, for --capture")
	as := flag.String("as", "", "view as this directory address through Spoof Mode, for --capture")
	flag.Parse()
	slog.SetDefault(logging.Console())

	switch {
	case *real:
		devcache.Install()
		app.Serve(localTLS(app.Production(app.DevDomain)))
	case *detach:
		detachReal(*email)
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

func mailDir() string {
	if dir := os.Getenv("MAIL_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), "hca-mail")
}

func sampleServer() (*http.Server, *store.Queue) {
	intercept.Install(mail.Host, mailFiles{dir: mailDir()})
	dir := &data.Dir{Root: "sampledata"}
	bucket := blob.NewMemoryBucket()
	media := blob.New(bucket)
	core := app.NewCore(app.Config{
		Source:        dir,
		Writer:        dir,
		Geocoder:      geocode.Fake{},
		Bucket:        bucket,
		Store:         media,
		FamilyIDKey:   []byte("sample"),
		ChatKey:       []byte("sample"),
		BrowserKey:    os.Getenv("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch:   app.ImageSearchKeys(),
		Describer:     sampleDescriber(),
		Mail:          mail.NewMailgun("sample", "HCA-Team <hca@example.org>"),
		CelebrateMail: mail.NewMailgun("sample", "Helios Celebrate <celebrate@example.org>"),
		CalendarMail:  when.Mail{Sender: mail.NewMailgun("sample", "Helios When <when@example.org>"), ReplyTo: "Helios When <rsvp@reply.example.org>", Key: []byte("sample")},
		BirthdayMail:  mail.NewMailgun("sample", "Helios Staff Birthdays <birthday@example.org>"),
		BirthdayBase:  "https://birthday.heliosiandev.com:" + app.Port(),
		FeedbackBase:  "https://home.heliosiandev.com:" + app.Port(),
		Loop:          loop.Mail{Sender: mail.NewMailgun("sample", ""), Key: []byte("sample"), Base: "https://loop.heliosiandev.com:" + app.Port(), Archive: loop.DirArchive{Dir: mailDir()}},
		LoopDescriber: sampleGroupDescriber(),
		Asker:         sampleAsker(),
		Embedder:      artifacts.Fake{},
		ArtifactsMail: artifacts.Inbox{Bucket: bucket},
		KeyPoints:     keypoints.Fake{},
	})
	saved, err := filepath.Glob("sampledata/artifacts/*.json")
	if err != nil {
		logging.Fatal("list the sample documents", "error", err)
	}
	for _, path := range saved {
		if err := core.Documents.FileSaved(context.Background(), access.System("sample"), path); err != nil {
			logging.Fatal("file a sample document", "path", path, "error", err)
		}
	}
	signIn := auth.New(app.DevDomain, "", []byte("sample"), auth.Login{}, core.Member, core.Sessions)
	signIn.Spoof = core.Spoof
	for _, m := range core.Muxes() {
		m.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
		signIn.RegisterSpoof(m)
	}
	slog.Info("serving sample data", "as", sampleUser)
	return localTLS(app.Server(app.DevDomain, core.Handlers(func(_ string, next http.Handler) http.Handler {
		return signIn.Fixed(sampleUser, next)
	}), core.Aliased()), core.Queue)
}

func sampleAsker() ask.Responder {
	if c := app.ClaudeAsker(); c != nil {
		return c
	}
	return ask.Fake{}
}

func sampleDescriber() birthday.Describer {
	if d := app.ClaudeDescriber(); d != nil {
		return d
	}
	return describe.Fake{}
}

func sampleGroupDescriber() loop.Describer {
	if d := app.ClaudeGroupDescriber(); d != nil {
		return d
	}
	return describe.Fake{}
}

func localTLS(server *http.Server, queue *store.Queue) (*http.Server, *store.Queue) {
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{devtls.Certificate(app.DevDomain)}}
	return server, queue
}

func detachReal(email string) {
	key := env.Required("SESSION_KEY")
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

	cookie := auth.Token([]byte(key), email, time.Now())
	fmt.Printf("log: %s\n", logPath)
	fmt.Printf("stop with: kill -- -%d\n", cmd.Process.Pid)
	fmt.Printf("header: Cookie: session=%s\n", cookie)
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
