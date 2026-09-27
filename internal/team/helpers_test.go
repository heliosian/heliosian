package team

import (
	"time"

	"heliosian/internal/testkit"
)

func now() time.Time {
	return testkit.MustTime("2026-09-09")
}
