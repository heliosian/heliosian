package claude

import (
	"errors"
	"time"

	"heliosian/internal/ratelimit"
)

const perHour = 30

var ErrTooMany = errors.New("that's a lot of Claude for one hour; try again a little later")

func NewLimiter() *ratelimit.Limiter {
	return ratelimit.New(perHour, time.Hour)
}
