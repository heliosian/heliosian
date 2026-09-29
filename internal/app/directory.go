package app

import (
	"time"

	"heliosian/internal/auth"
	"heliosian/internal/mail"
	"heliosian/internal/model"
)

func loopAudience(cache *model.DirectoryCache, teamCache *model.ActivitiesCache, celebrateCache *model.PartiesCache) func() model.AudienceSources {
	return func() model.AudienceSources {
		return model.EmailListAudience(cache.Model(), celebrateCache.Model(), teamCache.Model(), teamCache, time.Now().In(model.Location))
	}
}

func spoofPerson(cache *model.DirectoryCache) func(email string) (auth.Person, bool) {
	return func(email string) (auth.Person, bool) {
		directory := cache.Model()
		p := directory.Person(directory.Resolve(mail.Normalize(email)))
		if p == nil {
			return auth.Person{}, false
		}
		return auth.Person{Email: p.Email, FullName: p.FullName, Words: p.Words()}, true
	}
}
