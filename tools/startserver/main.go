// Command startserver launches the app in the background with its output in a log file,
// and prints an auth header and the process group to stop it with.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"

	"heliosian/internal/auth"
)

const logPath = "/tmp/heliosian-server.log"

func main() {
	email := flag.String("email", "ian.gulliver@heliosschool.org", "session email for the minted cookie")
	flag.Parse()
	key := os.Getenv("SESSION_KEY")
	if key == "" {
		log.Fatal("[ERROR] SESSION_KEY is required (the server and the minted cookie must share it)")
	}
	if os.Getenv("DIRECTORY_SHEET") == "" {
		log.Fatal("[ERROR] DIRECTORY_SHEET is required")
	}
	if os.Getenv("PREFERENCES_SHEET") == "" {
		log.Fatal("[ERROR] PREFERENCES_SHEET is required")
	}

	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("[ERROR] create server log: %v", err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	// Its own group, because go run execs the server as a child and stopping the wrapper
	// leaves that child holding the port.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		log.Fatalf("[ERROR] start server: %v", err)
	}

	cookie := auth.Token([]byte(key), *email, time.Now().Add(24*time.Hour))
	fmt.Printf("log: %s\n", logPath)
	fmt.Printf("stop with: kill -- -%d\n", cmd.Process.Pid)
	fmt.Printf("header: Cookie: session=%s\n", cookie)
}
