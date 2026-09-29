package app

import (
	"net/http"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/api"
	"heliosian/internal/artifacts"
	"heliosian/internal/feedback"
	"heliosian/internal/home"
	"heliosian/internal/model"
	"heliosian/internal/store"
)

type Snapshot struct {
	Config    *model.Config
	Who       *model.Directory
	Invites   *model.InviteTemplates
	Team      *model.Activities
	Birthday  model.BirthdaysWorld
	Celebrate *model.Parties
	When      model.CalendarWorld
	Loop      model.EmailListsWorld
	Home      *home.Model
	Artifacts *artifacts.Model
	Feedback  *feedback.Model
}

type caches struct {
	settings  *model.ConfigCache
	who       *model.DirectoryCache
	invites   *model.InviteTemplatesCache
	team      *model.ActivitiesCache
	birthday  *model.BirthdaysCache
	celebrate *model.PartiesCache
	when      *model.CalendarCache
	loop      *model.EmailListsCache
	home      *home.Cache
	artifacts *artifacts.Cache
	feedback  *feedback.Cache
	whenHooks model.CalendarHooks
	idKey     []byte
}

func (c caches) snapshot(tx *store.Tx) *Snapshot {
	directory := c.who.In(tx)
	settings := c.settings.In(tx)
	parties, activities, lists := c.celebrate.In(tx), c.team.In(tx), c.loop.In(tx)
	return &Snapshot{
		Config:    settings,
		Who:       directory,
		Invites:   c.invites.In(tx),
		Team:      activities,
		Birthday:  model.NewBirthdaysWorld(c.birthday.In(tx), directory),
		Celebrate: parties,
		When:      c.whenHooks.World(c.when.In(tx), directory, settings, parties, activities, lists, c.idKey),
		Loop:      model.NewEmailListsWorld(lists, directory, settings.GradeColors, parties, activities, c.team),
		Home:      c.home.In(tx),
		Artifacts: c.artifacts.In(tx),
		Feedback:  c.feedback.In(tx),
	}
}

func (s *Snapshot) at(q api.Query) *Snapshot {
	scoped := *s
	scoped.Loop = s.Loop.At(q.Now)
	scoped.When = s.When.At(q.Now)
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

func resources(c caches, queue *store.Queue, birthdays []api.Type[model.BirthdaysWorld], lists []api.Type[model.EmailListsWorld]) *api.Registry[*Snapshot] {
	reg := api.New(api.Config[*Snapshot]{
		Actor:  func(r *http.Request, s *Snapshot) access.Actor { return s.Who.Actor(r, c.held) },
		Held:   c.held,
		Now:    func() time.Time { return time.Now().In(model.Location) },
		Queue:  queue,
		Staged: c.snapshot,
		Scope:  (*Snapshot).at,
	})
	for _, t := range model.DirectoryResources() {
		reg.Add(api.Lift(t, func(s *Snapshot) *model.Directory { return s.Who }))
	}
	for _, t := range birthdays {
		reg.Add(api.Lift(t, func(s *Snapshot) model.BirthdaysWorld { return s.Birthday }))
	}
	for _, t := range lists {
		reg.Add(api.Lift(t, func(s *Snapshot) model.EmailListsWorld { return s.Loop }))
	}
	for _, t := range c.whenHooks.Resources() {
		reg.Add(api.Lift(t, func(s *Snapshot) model.CalendarWorld { return s.When }))
	}
	queue.OnSwap(func() { reg.Publish(c.snapshot(nil)) })
	return reg
}
