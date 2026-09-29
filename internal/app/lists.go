package app

import (
	"time"

	"heliosian/internal/loop"
	"heliosian/internal/model"
)

type smartLists struct {
	cache     *model.DirectoryCache
	team      *model.ActivitiesCache
	celebrate *model.PartiesCache
	loop      *loop.Cache
	sources   func() model.AudienceSources
}

func (s smartLists) Lists(email string) []model.MagicTag {
	directory, now := s.cache.Model(), time.Now().In(model.Location)
	lists := append(s.celebrate.Model().MagicTags(directory, email, now), s.team.Model().MagicTags(directory, email, now)...)
	return append(lists, s.loop.Model().Lists(s.sources(), email)...)
}
