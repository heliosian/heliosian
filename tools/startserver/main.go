// Command startserver launches the app, waits for it to listen, prints the pid and an auth header, and leaves it running.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

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

	logFile, err := os.Create("/tmp/heliosian-server.log")
	if err != nil {
		log.Fatalf("[ERROR] create server log: %v", err)
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		log.Fatalf("[ERROR] start server: %v", err)
	}

	deadline := time.Now().Add(10 * time.Minute)
	for {
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			log.Fatalf("[ERROR] server did not start within 10 minutes; log: /tmp/heliosian-server.log")
		}
		content, err := os.ReadFile("/tmp/heliosian-server.log")
		if err != nil {
			log.Fatalf("[ERROR] read server log: %v", err)
		}
		if strings.Contains(string(content), "listening on ") {
			break
		}
		if cmd.ProcessState != nil || !processAlive(cmd.Process.Pid) {
			fmt.Print(string(content))
			log.Fatal("[ERROR] server exited before listening")
		}
		time.Sleep(time.Second)
	}

	payload := fmt.Sprintf("%s|%d", *email, time.Now().Add(24*time.Hour).Unix())
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	cookie := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	content, _ := os.ReadFile("/tmp/heliosian-server.log")
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		fmt.Println(line)
	}
	fmt.Printf("pid: %d\n", cmd.Process.Pid)
	fmt.Println("log: /tmp/heliosian-server.log")
	fmt.Printf("header: Cookie: session=%s\n", cookie)
	cmd.Process.Release()
}

func processAlive(pid int) bool {
	return exec.Command("kill", "-0", fmt.Sprint(pid)).Run() == nil
}
