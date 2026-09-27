package app

import (
	"time"

	"heliosian/internal/celebrate"
	"heliosian/internal/filter"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type smartLists struct {
	cache     *who.Cache
	team      *team.Cache
	celebrate *celebrate.Cache
	loop      *loop.Cache
	sources   func() filter.Sources
}

func (s smartLists) Lists(email string) []who.List {
	model, now := s.cache.Model(), time.Now().In(when.Location)
	lists := append(s.celebrate.Model().Lists(model, email, now), s.team.Model().Lists(model, email, now)...)
	return append(lists, s.loop.Model().Lists(s.sources(), email)...)
}
