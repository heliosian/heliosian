package app

import (
	"net/http"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/artifacts"
	"heliosian/internal/birthday"
	"heliosian/internal/celebrate"
	"heliosian/internal/config"
	"heliosian/internal/feedback"
	"heliosian/internal/home"
	"heliosian/internal/loop"
	"heliosian/internal/store"
	"heliosian/internal/team"
	"heliosian/internal/when"
	"heliosian/internal/who"
)

type Snapshot struct {
	Config    *config.Settings
	Who       *who.Model
	Invites   *who.InviteModel
	Team      *team.Model
	Birthday  *birthday.Model
	Celebrate *celebrate.Model
	When      *when.Model
	Loop      *loop.Model
	Home      *home.Model
	Artifacts *artifacts.Model
	Feedback  *feedback.Model
}

type caches struct {
	settings  *config.Cache
	who       *who.Cache
	invites   *who.Invites
	team      *team.Cache
	birthday  *birthday.Cache
	celebrate *celebrate.Cache
	when      *when.Cache
	loop      *loop.Cache
	home      *home.Cache
	artifacts *artifacts.Cache
	feedback  *feedback.Cache
}

func (c caches) snapshot() *Snapshot {
	return &Snapshot{
		Config:    c.settings.Model(),
		Who:       c.who.Model(),
		Invites:   c.invites.Model(),
		Team:      c.team.Model(),
		Birthday:  c.birthday.Model(),
		Celebrate: c.celebrate.Model(),
		When:      c.when.Model(),
		Loop:      c.loop.Model(),
		Home:      c.home.Model(),
		Artifacts: c.artifacts.Model(),
		Feedback:  c.feedback.Model(),
	}
}

func (c caches) held(email string) []access.Allowance {
	out := slices.Concat(
		c.who.Held(email), c.team.Held(email), c.birthday.Held(email), c.celebrate.Held(email),
		c.when.Held(email), c.loop.Held(email), c.home.Held(email), c.settings.SuperHeld(email),
	)
	if c.settings.IsSuperAdmin(email) {
		out = append(out, feedback.Triage)
	}
	return out
}

func resources(c caches, queue *store.Queue) *api.Registry[*Snapshot] {
	reg := api.New(api.Config[*Snapshot]{
		Actor: func(r *http.Request, s *Snapshot) access.Actor { return s.Who.Actor(r, c.held) },
		Held:  c.held,
		Now:   func() time.Time { return time.Now().In(when.Location) },
	})
	queue.OnSwap(func() { reg.Publish(c.snapshot()) })
	return reg
}
