// Command startserver is the development server. By default it serves the
// sample community in the foreground; -capture serves it just long enough to
// screenshot one page and exits; -real serves the production assembly in the
// foreground; -detach launches -real in the background with its output in a log
// file and prints a minted session cookie plus the command to stop it. Every
// mode serves all three apps, each on its own local hostname.
//
// All sample-mode composition lives here: the production binary (main.go) and
// the shared wiring in internal/app carry no dev or sample behavior at all.
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
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
		Source:     dir,
		Writer:     dir,
		Geocoder:   geocode.Fake{},
		BrowserKey: os.Getenv("GOOGLE_MAPS_BROWSER_KEY"),
	})
	core.Mux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	core.HomeMux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	core.EventsMux.Handle("POST /auth/logout", http.RedirectHandler("/", http.StatusSeeOther))
	log.Printf("serving sample data as %s", sampleUser)
	return localTLS(app.Server(map[string]http.Handler{
		"who":  app.Public("who", auth.Fixed(sampleUser, app.Logged("who", app.Files("who", core.Gate)))),
		"home": app.Public("home", auth.Fixed(sampleUser, app.Logged("home", app.Files("home", core.Home)))),
		"hca":  app.Public("hca", auth.Fixed(sampleUser, app.Logged("hca", app.Files("hca", core.Events)))),
	}), core.Queue)
}

func localTLS(server *http.Server, queue *who.Queue) (*http.Server, *who.Queue) {
	server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{devtls.Certificate()}}
	return server, queue
}

func detachReal(email string) {
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		log.Fatal("[ERROR] SESSION_KEY is required (the server and the minted cookie must share it)")
	}
	for _, name := range []string{"DIRECTORY_SHEET", "PREFERENCES_SHEET", "INVITES_SHEET", "APPS_SHEET", "EVENTS_SHEET", "CONFIG_SHEET"} {
		if os.Getenv(name) == "" {
			log.Fatalf("[ERROR] %s is required", name)
		}
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("[ERROR] create server log: %v", err)
	}
	self, err := os.Executable()
	if err != nil {
		log.Fatalf("[ERROR] resolve own binary: %v", err)
	}
	cmd := exec.Command(self, "-real")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Its own group, so the printed stop command reaps the server and nothing else.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Fatalf("[ERROR] start server: %v", err)
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
			log.Fatalf("[ERROR] server: %v", err)
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
		log.Fatalf("[ERROR] server did not become ready on %s", base)
	}

	png, captureErr := capture.PNG(capture.Options{URL: url, Wait: wait})
	if err := server.Shutdown(context.Background()); err != nil {
		log.Printf("[ERROR] shutdown: %v", err)
	}
	<-served
	<-queue.Drain()
	if captureErr != nil {
		log.Fatalf("[ERROR] %v", captureErr)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatalf("[ERROR] create output dir: %v", err)
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		log.Fatalf("[ERROR] write %s: %v", out, err)
	}
	log.Printf("captured %s to %s", url, out)
}
