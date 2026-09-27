package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/env"
)

func main() {
	email := flag.String("email", "", "session email address")
	flag.Parse()
	key := env.Required("SESSION_KEY")
	if *email == "" {
		log.Fatal("--email is required")
	}
	fmt.Println(auth.Token([]byte(key), *email, time.Now()))
}
