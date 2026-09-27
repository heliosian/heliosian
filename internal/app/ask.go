package app

import (
	"time"

	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/filter"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

func askSources(cache *who.Cache, settings *config.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache, calendarCache *when.Cache, loopCache *loop.Cache, homeCache *home.Cache, artifactsCache *artifacts.Cache, embedder artifacts.Embedder, lists smartLists, sources func() filter.Sources, linked func(email string) []when.Linked) ask.Sources {
	return ask.Sources{
		Directory: cache.Model,
		Tags: func(owner string) map[string][]string {
			return cache.Model().Tags(owner)
		},
		Lists: func(email string) []who.List {
			return append(cache.Model().RoomParentLists(email), lists.Lists(email)...)
		},
		Settings:    settings.Settings,
		Calendar:    calendarCache.Model,
		Linked:      linked,
		Team:        teamCache.Model,
		Celebrate:   celebrateCache.Model,
		Loop:        loopCache.Model,
		LoopSources: sources,
		Links:       homeCache.CategoriesFor,
		Artifacts:   artifactsCache.Model,
		Embedder:    embedder,
		Admins:      ask.Admins{Team: teamCache.IsAdmin, Celebrate: celebrateCache.IsAdmin, Loop: loopCache.IsAdmin, Calendar: calendarCache.IsAdmin, Home: homeCache.IsAdmin},
		Now:         time.Now,
	}
}
