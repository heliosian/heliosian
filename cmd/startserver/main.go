// Command startserver is the development server. By default it serves the
// sample community in the foreground; -capture serves it just long enough to
// screenshot one page and exits; -real serves the production assembly in the
// foreground; -detach launches -real in the background with its output in a log
// file and prints a minted session cookie plus the command to stop it. Every
// mode serves every app, each on its own local hostname.
//
// All sample-mode composition lives here: the production binary (main.go) and
// the shared wiring in internal/app carry no dev or sample behavior at all.
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
	"heliosian/internal/auth"
	"heliosian/internal/capture"
	"heliosian/internal/data"
	"heliosian/internal/devtls"
	"heliosian/internal/events"
	"heliosian/internal/geocode"
	"heliosian/internal/logging"
	"heliosian/internal/who"
)

const logPath = "/tmp/heliosian-server.log"

const sampleUser = "jordan.whitfield@heliosschool.org"

func main() {
	email := flag.String("email", "ian.gulliver@heliosschool.org", "session email for -detach's minted cookie")
	real := flag.Bool("real", false, "serve the production assembly in the foreground")
	detach := flag.Bool("detach", false, "launch -real in the background with a log file and a minted cookie")
	capturePath := flag.String("capture", "", "serve sample data in-process, capture this url (on any app's local hostname) as a PNG, and exit")
	out := flag.String("out", "screenshots/capture.png", "output png path for -capture")
	wait := flag.String("wait", "body", "css selector that must be visible before capturing, for -capture")
	flag.Parse()
	slog.SetDefault(logging.Console())

	switch {
	case *real:
		app.Serve(localTLS(app.Production()))
	case *detach:
		detachReal(*email)
	case *capturePath != "":
		captureOnce(*capturePath, *out, *wait)
	default:
		app.Serve(sampleServer())
	}
}

// sampleServer assembles the fictional community: sample CSVs, fake geocoding,
// no media bucket, every request signed in as the sample parent, whom the sample
// Config sheet lists as a super admin so every admin tool is testable locally.
func sampleServer() (*http.Server, *who.Queue) {
	dir := &data.Dir{Root: "sampledata"}
	core := app.NewCore(app.Config{
		Source:      dir,
		Writer:      dir,
		Geocoder:    geocode.Fake{},
		BrowserKey:  os.Getenv("GOOGLE_MAPS_BROWSER_KEY"),
		ImageSearch: events.ImageSearch{Key: os.Getenv("GOOGLE_SEARCH_KEY"), CX: os.Getenv("GOOGLE_SEARCH_CX"), Unsplash: os.Getenv("UNSPLASH_KEY"), Pexels: os.Getenv("PEXELS_KEY"), Pixabay: os.Getenv("PIXABAY_KEY")},
	})
	core.Mux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	core.HomeMux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	core.EventsMux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	core.BirthdayMux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	slog.Info("serving sample data", "as", sampleUser)
	return localTLS(app.Server(map[string]http.Handler{
		"who":      app.Public("who", auth.Fixed(sampleUser, app.Logged("who", app.Files("who", core.Gate)))),
		"home":     app.Public("home", auth.Fixed(sampleUser, app.Logged("home", app.Files("home", core.Home)))),
		"hca":      app.Public("hca", auth.Fixed(sampleUser, app.Logged("hca", app.Files("hca", core.Events)))),
		"birthday": app.Public("birthday", auth.Fixed(sampleUser, app.Logged("birthday", app.Files("birthday", core.Birthday)))),
	}), core.Queue)
}

func localTLS(server *http.Server, queue *who.Queue) (*http.Server, *who.Queue) {
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{devtls.Certificate()}}
	return server, queue
}

func detachReal(email string) {
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		logging.Fatal("SESSION_KEY is required (the server and the minted cookie must share it)")
	}
	for _, name := range []string{"DIRECTORY_SHEET", "PREFERENCES_SHEET", "INVITES_SHEET", "APPS_SHEET", "EVENTS_SHEET", "BIRTHDAY_SHEET", "CONFIG_SHEET"} {
		if os.Getenv(name) == "" {
			logging.Fatal("environment variable is required", "name", name)
		}
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		logging.Fatal("create server log", "error", err)
	}
	self, err := os.Executable()
	if err != nil {
		logging.Fatal("resolve own binary", "error", err)
	}
	cmd := exec.Command(self, "-real")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Its own group, so the printed stop command reaps the server and nothing else.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		logging.Fatal("start server", "error", err)
	}

	cookie := auth.Token([]byte(key), email, time.Now().Add(24*time.Hour))
	fmt.Printf("log: %s\n", logPath)
	fmt.Printf("stop with: kill -- -%d\n", cmd.Process.Pid)
	fmt.Printf("header: Cookie: session=%s\n", cookie)
}

func captureOnce(url, out, wait string) {
	server, queue := sampleServer()
	served := make(chan error, 1)
	go func() {
		served <- app.ListenAndServe(server)
	}()
	base := "https://who.local.heliosian.com:" + app.Port()
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

	png, captureErr := capture.PNG(capture.Options{URL: url, Wait: wait})
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
	slog.Info("captured", "url", url, "out", out)
}
