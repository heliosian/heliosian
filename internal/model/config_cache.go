package model

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type ConfigCache struct {
	*store.Store[*Config]
}

func NewConfigCache(source data.Source, writer data.Writer, queue *store.Queue) (*ConfigCache, error) {
	s, err := store.New(store.Spec[*Config]{
		App:  ConfigApp,
		Tabs: ConfigTabs,
		Build: func(_ context.Context, tables store.Tables) (*Config, error) {
			return parseConfig(tables)
		},
		Loaded: func(settings *Config, took time.Duration) {
			slog.Info("loaded config", "superAdmins", len(settings.SuperAdmins), "gradeColors", len(settings.GradeColors),
				"classroomColors", len(settings.ClassroomColors), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &ConfigCache{Store: s}, nil
}

func (c *ConfigCache) Config() *Config {
	return c.Model()
}

func (c *ConfigCache) SuperAdmins() []string {
	return c.Config().SuperAdmins
}

func (c *ConfigCache) IsSuperAdmin(email string) bool {
	return slices.Contains(c.SuperAdmins(), mail.Normalize(email))
}

func (c *ConfigCache) SuperHeld(email string) []access.Allowance {
	if !c.IsSuperAdmin(email) {
		return nil
	}
	return SuperAllowances
}

func (c *ConfigCache) SignedOut(email string) (time.Time, bool) {
	at, ok := c.Config().SignedOut[mail.Normalize(email)]
	return at, ok
}

func (c *ConfigCache) SignOut(ctx context.Context, email string) error {
	actor := access.Actor{Email: mail.Normalize(email)}
	return c.Commit(ctx, actor, signedOut(actor.Email)...)
}
