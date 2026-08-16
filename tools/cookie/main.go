// Command cookie prints a signed session cookie for local api testing.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"heliosian/internal/auth"
)

func main() {
	email := flag.String("email", "", "session email address")
	flag.Parse()
	key := os.Getenv("SESSION_KEY")
	if key == "" || *email == "" {
		log.Fatal("[ERROR] SESSION_KEY and -email are required")
	}
	fmt.Println(auth.Token([]byte(key), *email, time.Now().Add(24*time.Hour)))
}
