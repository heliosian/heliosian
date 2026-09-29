package app

import (
	"time"

	"heliosian/internal/celebrate"
	"heliosian/internal/loop"
	"heliosian/internal/model"
	"heliosian/internal/team"
	"heliosian/internal/when"
)

type smartLists struct {
	cache     *model.DirectoryCache
	team      *team.Cache
	celebrate *celebrate.Cache
	loop      *loop.Cache
	sources   func() model.AudienceSources
}

func (s smartLists) Lists(email string) []model.MagicTag {
	directory, now := s.cache.Model(), time.Now().In(when.Location)
	lists := append(s.celebrate.Model().Lists(directory, email, now), s.team.Model().Lists(directory, email, now)...)
	return append(lists, s.loop.Model().Lists(s.sources(), email)...)
}
