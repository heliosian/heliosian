package config

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"heliosian/internal/data"
	"heliosian/internal/store"
)

type Cache struct {
	*store.Store[*Settings]
}

var Tabs = []store.Tab{
	{Name: SettingsTab, Columns: SettingsColumns, Key: []string{KeyColumn}},
	{Name: SuperAdminsTab, Columns: SuperAdminColumns, Key: []string{EmailColumn}},
	{Name: GradeColorsTab, Columns: GradeColorColumns, Key: []string{GradeColumn}},
	{Name: ClassroomColorsTab, Columns: ClassroomColorColumns, Key: []string{ClassroomColumn}},
	{Name: SignedOutTab, Columns: SignedOutColumns, Key: []string{EmailColumn}},
}

func NewCache(source data.Source, writer data.Writer, queue *store.Queue) (*Cache, error) {
	s, err := store.New(store.Spec[*Settings]{
		App:  App,
		Tabs: Tabs,
		Build: func(_ context.Context, tables store.Tables) (*Settings, error) {
			return Parse(tables)
		},
		Loaded: func(settings *Settings, took time.Duration) {
			slog.Info("loaded config", "superAdmins", len(settings.SuperAdmins), "gradeColors", len(settings.GradeColors),
				"classroomColors", len(settings.ClassroomColors), "took", took.Round(time.Millisecond))
		},
	}, source, writer, queue)
	if err != nil {
		return nil, err
	}
	return &Cache{Store: s}, nil
}

func (c *Cache) Settings() *Settings {
	return c.Model()
}

func (c *Cache) SuperAdmins() []string {
	return c.Settings().SuperAdmins
}

func (c *Cache) IsSuperAdmin(email string) bool {
	return slices.Contains(c.SuperAdmins(), email)
}

func (c *Cache) SignedOut(email string) (time.Time, bool) {
	at, ok := c.Settings().SignedOut[strings.ToLower(strings.TrimSpace(email))]
	return at, ok
}

func (c *Cache) SignOut(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	return c.signOut(ctx, email, email)
}

func (c *Cache) signOut(ctx context.Context, actor, email string) error {
	at := time.Now().Truncate(time.Second)
	return c.Commit(ctx, actor, store.Set(SignedOutTab, store.Row{EmailColumn: email}, store.Row{TimeColumn: at.Format(time.RFC3339)}))
}
