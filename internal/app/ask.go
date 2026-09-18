package app

import (
	"heliosian/internal/artifacts"
	"heliosian/internal/ask"
	"heliosian/internal/calendar"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/team"
	"heliosian/internal/who"
)

// askSources hands Helios Ask every app's model as it stands when asked,
// the directory's view of a person as the calendar reads it, and the lists
// the other apps give them - the same readings the apps themselves make.
func askSources(cache *who.Cache, settings *config.Cache, teamCache *team.Cache, celebrateCache *celebrate.Cache, calendarCache *calendar.Cache, loopCache *loop.Cache, homeCache *home.Cache, artifactsCache *artifacts.Cache, embedder artifacts.Embedder, lists smartLists, loopDir loopDirectory, linked func(email string) []calendar.Linked) ask.Sources {
	return ask.Sources{
		Directory: cache.Model,
		Tags:      cache.Tags,
		Lists: func(email string) []who.List {
			return append(cache.Model().RoomParentLists(email), lists.Lists(email)...)
		},
		Calendar:          calendarCache.Model,
		CalendarDirectory: calendarDirectory{cache, settings},
		Linked:            linked,
		Team:              teamCache.Model,
		Celebrate:         celebrateCache.Model,
		Loop:              loopCache.Model,
		LoopSources:       func() loop.Sources { return loop.SourcesOf(loopDir) },
		Links:             func() []home.Category { return homeCache.Model().Categories },
		Alerts:            directory{cache, settings}.Alerts,
		Artifacts:         artifactsCache.Model,
		Embedder:          embedder,
	}
}
