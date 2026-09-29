package app

import (
	"time"

	"heliosian/internal/model"
)

type smartLists struct {
	cache     *model.DirectoryCache
	team      *model.ActivitiesCache
	celebrate *model.PartiesCache
	loop      *model.EmailListsCache
}

func (s smartLists) Lists(email string) []model.MagicTag {
	return model.ManagedMagicTags(s.cache.Model(), s.celebrate.Model(), s.team.Model(), s.loop.Model(), s.team, email, time.Now().In(model.Location))
}
