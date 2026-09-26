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

	"heliosian/internal/app"
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/auth"
	"heliosian/internal/birthday"
	"heliosian/internal/blob"
	"heliosian/internal/calendar"
	"heliosian/internal/capture"
	"heliosian/internal/data"
	"heliosian/internal/describe"
	"heliosian/internal/devtls"
	"heliosian/internal/geocode"
	"heliosian/internal/keypoints"
	"heliosian/internal/logging"
	"heliosian/internal/loop"
	"heliosian/internal/mail"
	"heliosian/internal/store"
	"heliosian/internal/team"
)

const logPath = "local/heliosian-server.log"

const sampleUser = "jordan.whitfield@heliosschool.org"

const blobCache = "local/cache/blobs"

func main() {
	email := flag.String("email", "ian.gulliver@heliosschool.org", "session email for --detach's minted cookie")
	real := flag.Bool("real", false, "serve the production assembly in the foreground")
	detach := flag.Bool("detach", false, "launch --real in the background with a log file and a minted cookie")
	capturePath := flag.String("capture", "", "serve sample data in-process, capture this url (on any app's local hostname) as a PNG, and exit")
	out := flag.String("out", "local/screenshots/capture.png", "output png path for --capture")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing, for --capture")
	width := flag.Int("width", 0, "viewport width for --capture (default 1280)")
	height := flag.Int("height", 0, "viewport height for --capture (default 800)")
	click := flag.String("click", "", "css selectors to click once --wait is visible, separated by |, for --capture")
	settle := flag.Duration("settle", 0, "how long to wait after the last click before capturing, for --capture")
	as := flag.String("as", "", "view as this directory address through Spoof Mode, for --capture")
	flag.Parse()
	slog.SetDefault(logging.Console())

	switch {
	case *real:
		app.Serve(localTLS(app.Production(app.DevDomain, blobCache)))
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
	dir := &data.Dir{Root: "sampledata"}
	media := blob.NewMemory()
	core := app.NewCore(app.Config{
		Source:        dir,
		Writer:        dir,
		Geocoder:      geocode.Fake{},
		Store:         media,
		FamilyIDKey:   []byte("sample"),
		ChatKey:       []byte("sample"),
		BrowserKey:    os.Getenv("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch:   app.ImageSearchKeys(),
		Describer:     sampleDescriber(),
		Mail:          mail.New("", "HCA-Team <hca@example.org>", mailDir()),
		MailFrom:      "HCA-Team <hca@example.org>",
		CelebrateMail: mail.New("", "Helios Celebrate <celebrate@example.org>", mailDir()),
		CelebrateFrom: "Helios Celebrate <celebrate@example.org>",
		CalendarMail:  calendar.Mail{Sender: mail.New("", "Helios When <when@example.org>", mailDir()), From: "Helios When <when@example.org>", ReplyTo: "Helios When <rsvp@reply.example.org>", Key: []byte("sample")},
		BirthdayMail:  mail.New("", "Helios Staff Birthdays <birthday@example.org>", mailDir()),
		BirthdayFrom:  "Helios Staff Birthdays <birthday@example.org>",
		BirthdayBase:  "https://birthday.heliosiandev.com:" + app.Port(),
		FeedbackBase:  "https://home.heliosiandev.com:" + app.Port(),
		Loop:          loop.Mail{Sender: &mail.Files{Dir: mailDir(), From: "Helios Loop"}, Key: []byte("sample"), Base: "https://loop.heliosiandev.com:" + app.Port(), Archive: loop.DirArchive{Dir: mailDir()}},
		LoopDescriber: sampleGroupDescriber(),
		Asker:         sampleAsker(),
		Embedder:      artifacts.Fake{},
		ArtifactsMail: artifacts.Inbox{Bucket: media},
		KeyPoints:     keypoints.Fake{},
	})
	saved, err := filepath.Glob("sampledata/artifacts/*.json")
	if err != nil {
		logging.Fatal("list the sample documents", "error", err)
	}
	for _, path := range saved {
		if err := core.Documents.FileSaved(context.Background(), "sample", path); err != nil {
			logging.Fatal("file a sample document", "path", path, "error", err)
		}
	}
	signIn := auth.New(app.DevDomain, "", []byte("sample"), "", core.Member, core.Sessions)
	signIn.Spoof = core.Spoof
	for _, m := range core.Muxes() {
		m.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
		signIn.RegisterSpoof(m)
	}
	slog.Info("serving sample data", "as", sampleUser)
	return localTLS(app.Server(app.DevDomain, map[string]http.Handler{
		"who":       app.Public("who", signIn.Fixed(sampleUser, app.Logged("who", app.Files("who", core.Mux)))),
		"home":      app.Public("home", signIn.Fixed(sampleUser, app.Logged("home", app.Files("home", core.Home)))),
		"team":      app.Public("team", team.Redirected(core.TeamCache, signIn.Fixed(sampleUser, app.Logged("team", app.Files("team", core.Team))))),
		"birthday":  app.Public("birthday", signIn.Fixed(sampleUser, app.Logged("birthday", app.Files("birthday", core.Birthday)))),
		"celebrate": app.Public("celebrate", signIn.Fixed(sampleUser, app.Logged("celebrate", app.Files("celebrate", core.Celebrate)))),
		"calendar":  app.Public("calendar", signIn.Fixed(sampleUser, app.Logged("calendar", app.Files("calendar", core.Calendar)))),
		"loop":      app.Public("loop", signIn.Fixed(sampleUser, app.Logged("loop", app.Files("loop", core.Loop)))),
		"ask":       app.Public("ask", signIn.Fixed(sampleUser, app.Logged("ask", app.Files("ask", core.Ask)))),
	}), core.Queue)
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
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		logging.Fatal("SESSION_KEY is required (the server and the minted cookie must share it)")
	}
	for _, name := range []string{"DIRECTORY_SHEET", "PREFERENCES_SHEET", "INVITES_SHEET", "APPS_SHEET", "EVENTS_SHEET", "BIRTHDAY_SHEET", "BIRTHDAY_SHARED_SHEET", "CELEBRATE_SHEET", "CALENDAR_SHEET", "CONFIG_SHEET", "GROUPS_SHEET", "ARTIFACTS_SHEET", "FEEDBACK_SHEET"} {
		if os.Getenv(name) == "" {
			logging.Fatal("environment variable is required", "name", name)
		}
	}

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
	// Its own group, so the printed stop command reaps the server and nothing else.
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
