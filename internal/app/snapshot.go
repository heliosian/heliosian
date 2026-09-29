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
	Loop      loop.World
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
	settings := c.settings.In(tx)
	parties, activities := c.celebrate.In(tx), c.team.In(tx)
	return &Snapshot{
		Config:    settings,
		Who:       directory,
		Invites:   c.invites.In(tx),
		Team:      activities,
		Birthday:  birthday.NewWorld(c.birthday.In(tx), directory),
		Celebrate: parties,
		When:      c.when.In(tx),
		Loop:      loop.NewWorld(c.loop.In(tx), directory, settings.GradeColors, magicTags(directory, parties, activities), magicTagKeys(parties, activities)),
		Home:      c.home.In(tx),
		Artifacts: c.artifacts.In(tx),
		Feedback:  c.feedback.In(tx),
	}
}

func (s *Snapshot) at(q api.Query) *Snapshot {
	scoped := *s
	scoped.Loop = s.Loop.At(q.Now)
	return &scoped
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

func resources(c caches, queue *store.Queue, birthdays []api.Type[birthday.World], lists []api.Type[loop.World]) *api.Registry[*Snapshot] {
	reg := api.New(api.Config[*Snapshot]{
		Actor:  func(r *http.Request, s *Snapshot) access.Actor { return s.Who.Actor(r, c.held) },
		Held:   c.held,
		Now:    func() time.Time { return time.Now().In(when.Location) },
		Queue:  queue,
		Staged: c.snapshot,
		Scope:  (*Snapshot).at,
	})
	for _, t := range who.Resources() {
		reg.Add(api.Lift(t, func(s *Snapshot) *who.Model { return s.Who }))
	}
	for _, t := range birthdays {
		reg.Add(api.Lift(t, func(s *Snapshot) birthday.World { return s.Birthday }))
	}
	for _, t := range lists {
		reg.Add(api.Lift(t, func(s *Snapshot) loop.World { return s.Loop }))
	}
	queue.OnSwap(func() { reg.Publish(c.snapshot(nil)) })
	return reg
}
