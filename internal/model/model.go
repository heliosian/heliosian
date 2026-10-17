package model

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/artifacts"
	"heliosian/internal/blob"
	"heliosian/internal/data"
	"heliosian/internal/mail"
	"heliosian/internal/store"
)

type Model struct {
	Config    *Config
	Directory *Directory
	Documents *Documents
}

type Deps struct {
	IDKey    []byte
	Photos   blob.Checker
	Static   blob.Checker
	Objects  *blob.Bucket
	Embedder *artifacts.Vertex
}

type Store struct {
	*store.Store[Model]
	documents *documentObjects
	unlocated chan struct{}
	located   *Directory
}

func took(d time.Duration) time.Duration {
	return d.Round(time.Millisecond)
}

func parts(deps Deps, documents *documentObjects) []store.Part[Model] {
	return []store.Part[Model]{
		{
			App: ConfigApp, Tabs: ConfigTabs,
			Build: func(_ context.Context, tables store.Tables, m *Model) (err error) {
				m.Config, err = parseConfig(tables)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				slog.Info("loaded config", "superAdmins", len(m.Config.SuperAdmins), "gradeColors", len(m.Config.GradeColors),
					"classroomColors", len(m.Config.ClassroomColors), "took", took(d))
			},
		},
		{
			App: DirectoryApp, Tabs: directoryTabs(),
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Directory, err = BuildDirectory(ctx, tables, deps.Photos, deps.Static, deps.IDKey)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				slog.Info("loaded directory model", "people", len(m.Directory.People), "families", len(m.Directory.Families),
					"classrooms", len(m.Directory.Classrooms), "crews", len(m.Directory.Crews), "unlocated", len(m.Directory.unlocated), "took", took(d))
			},
		},
		{
			App: DocumentsApp, Tabs: documentsTabs,
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Documents, err = documents.build(ctx, tables)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				documents.keep(m.Documents)
				oldest, newest := m.Documents.Span()
				slog.Info("loaded artifacts model", "documents", len(m.Documents.Documents), "fetched", m.Documents.Fetched, "chunks", m.Documents.Chunks(),
					"channels", len(m.Documents.Channels()), "oldest", oldest, "newest", newest, "took", took(d))
			},
		},
	}
}

func noConsentStep(context.Context, *Model, *Model) error {
	return nil
}

func noChecks(context.Context, *Model, *Model) error {
	return nil
}

func NewStore(source data.Source, writer data.Writer, queue *store.Queue, deps Deps) (*Store, error) {
	documents := &documentObjects{objects: deps.Objects, embedder: deps.Embedder, idKey: deps.IDKey, held: map[string]*Document{}}
	s, err := store.New(parts(deps, documents), noConsentStep, noChecks, source, writer, queue)
	if err != nil {
		return nil, err
	}
	out := &Store{Store: s, documents: documents, unlocated: make(chan struct{}, 1)}
	queue.OnSwap(out.locate)
	return out, nil
}

func (m *Model) IsSuperAdmin(email string) bool {
	return slices.Contains(m.Config.SuperAdmins, mail.Normalize(email))
}

func (s *Store) SignedOut(email string) (time.Time, bool) {
	at, ok := s.Model().Config.SignedOut[mail.Normalize(email)]
	return at, ok
}

func (s *Store) SignOut(ctx context.Context, email string) error {
	actor := access.Actor{Email: mail.Normalize(email)}
	return s.Commit(ctx, actor, ConfigApp, signedOut(actor.Email)...)
}
