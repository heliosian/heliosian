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
	Birthday  birthday.World
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

func (c caches) snapshot(tx *store.Tx) *Snapshot {
	directory := c.who.In(tx)
	return &Snapshot{
		Config:    c.settings.In(tx),
		Who:       directory,
		Invites:   c.invites.In(tx),
		Team:      c.team.In(tx),
		Birthday:  birthday.NewWorld(c.birthday.In(tx), directory),
		Celebrate: c.celebrate.In(tx),
		When:      c.when.In(tx),
		Loop:      c.loop.In(tx),
		Home:      c.home.In(tx),
		Artifacts: c.artifacts.In(tx),
		Feedback:  c.feedback.In(tx),
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

func resources(c caches, queue *store.Queue, birthdays []api.Type[birthday.World]) *api.Registry[*Snapshot] {
	reg := api.New(api.Config[*Snapshot]{
		Actor:  func(r *http.Request, s *Snapshot) access.Actor { return s.Who.Actor(r, c.held) },
		Held:   c.held,
		Now:    func() time.Time { return time.Now().In(when.Location) },
		Queue:  queue,
		Staged: c.snapshot,
	})
	for _, t := range who.Resources() {
		reg.Add(api.Lift(t, func(s *Snapshot) *who.Model { return s.Who }))
	}
	for _, t := range birthdays {
		reg.Add(api.Lift(t, func(s *Snapshot) birthday.World { return s.Birthday }))
	}
	queue.OnSwap(func() { reg.Publish(c.snapshot(nil)) })
	return reg
}
