// Command cookie prints a signed session cookie for local api testing.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

func main() {
	email := flag.String("email", "", "session email address")
	flag.Parse()
	key := os.Getenv("SESSION_KEY")
	if key == "" || *email == "" {
		log.Fatal("[ERROR] SESSION_KEY and -email are required")
	}
	payload := fmt.Sprintf("%s|%d", *email, time.Now().Add(24*time.Hour).Unix())
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(payload))
	fmt.Println(base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}
