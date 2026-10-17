package model

import (
	"time"

	"heliosian/internal/logging"
)

const (
	DateFormat     = "2006-01-02"
	DateTimeFormat = "2006-01-02 15:04"
)

var Location = mustLocation("America/Los_Angeles")

func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		logging.Fatal("load time zone", "name", name, "error", err)
	}
	return loc
}
