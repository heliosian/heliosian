package model

import (
	"context"
	"log/slog"
	"net/http"
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
	Config     *Config
	Directory  *Directory
	Invites    *InviteTemplates
	Calendar   *Calendar
	Parties    *Parties
	Activities *Activities
	Birthdays  *Birthdays
	EmailLists *EmailLists
	Home       *Home
	Feedback   *Feedback
	Documents  *Documents
	scope      *scope
}

type Deps struct {
	IDKey      []byte
	Photos     blob.Checker
	Static     blob.Checker
	Calendar   blob.Checker
	Parties    blob.Checker
	Activities blob.Checker
	Home       blob.Checker
	Objects    *blob.Bucket
	Embedder   *artifacts.Vertex
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
			App: invitesApp, Tabs: invitesTabs,
			Build: func(_ context.Context, tables store.Tables, m *Model) (err error) {
				m.Invites, err = buildInvites(tables)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				slog.Info("loaded invites", "systems", len(m.Invites.Systems), "greetings", len(m.Invites.Greetings), "took", took(d))
			},
		},
		{
			App: CalendarApp, Tabs: calendarTabs, Reads: []string{DirectoryApp},
			Build: func(ctx context.Context, tables store.Tables, m *Model) error {
				calendar, err := BuildCalendar(tables, m.Directory.Roster())
				if err != nil {
					return err
				}
				resolveImages(ctx, deps.Calendar, calendar)
				m.Calendar = calendar
				return nil
			},
			Loaded: func(m *Model, d time.Duration) {
				c := m.Calendar
				slog.Info("loaded calendar model", "events", len(c.Events), "hidden", c.Hidden, "days", len(c.Days),
					"feeds", len(c.Feeds), "skipped", c.Skipped, "took", took(d))
			},
		},
		{
			App: partiesAppName, Tabs: partiesTabs,
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Parties, err = BuildParties(ctx, tables, deps.Parties)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				tickets := 0
				for _, p := range m.Parties.Parties {
					tickets += len(p.Tickets)
				}
				slog.Info("loaded celebrate model", "celebrations", len(m.Parties.Celebrations), "parties", len(m.Parties.Parties),
					"tickets", tickets, "skipped", m.Parties.Skipped, "took", took(d))
			},
		},
		{
			App: activitiesAppName, Tabs: activitiesTabs,
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Activities, err = BuildActivities(ctx, tables, deps.Activities)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				children, volunteers := 0, 0
				for _, a := range m.Activities.Activities {
					children += len(a.Descendants())
					for _, n := range append([]*Activity{a}, a.Descendants()...) {
						volunteers += len(n.Volunteers)
					}
				}
				slog.Info("loaded events model", "categories", len(m.Activities.Categories), "roots", len(m.Activities.Activities),
					"children", children, "volunteers", volunteers, "skipped", m.Activities.Skipped, "took", took(d))
			},
		},
		{
			App: birthdaysAppName, Tabs: birthdaysTabs, Reads: []string{DirectoryApp},
			Build: func(_ context.Context, tables store.Tables, m *Model) error {
				birthdays, err := BuildBirthdays(tables, deps.IDKey)
				if err != nil {
					return err
				}
				birthdays.index(m.Directory)
				m.Birthdays = birthdays
				return nil
			},
			Loaded: func(m *Model, d time.Duration) {
				b := m.Birthdays
				slog.Info("loaded birthday model", "birthdays", len(b.Birthdays), "charities", len(b.Charities),
					"donations", len(b.Donations), "newsletters", len(b.NewsletterDates), "took", took(d))
			},
		},
		{
			App: sharedSheet, Tabs: sharedTabs,
			Build: func(context.Context, store.Tables, *Model) error { return nil },
			Loaded: func(_ *Model, d time.Duration) {
				slog.Info("loaded shared newsletter", "took", took(d))
			},
		},
		{
			App: emailListsAppName, Tabs: emailListsTabs,
			Build: func(_ context.Context, tables store.Tables, m *Model) (err error) {
				m.EmailLists, err = BuildEmailLists(tables, deps.IDKey)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				rules := 0
				for _, g := range m.EmailLists.Groups {
					rules += len(g.Rules)
				}
				slog.Info("loaded groups model", "groups", len(m.EmailLists.Groups), "rules", rules, "messages", len(m.EmailLists.Messages), "deliveries", len(m.EmailLists.Deliveries), "took", took(d))
			},
		},
		{
			App: homeAppName, Tabs: homeTabs,
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Home, err = BuildHome(ctx, tables, deps.Home)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				links := 0
				for _, category := range m.Home.Categories {
					links += len(category.Links)
				}
				slog.Info("loaded apps model", "categories", len(m.Home.Categories), "links", links, "took", took(d))
			},
		},
		{
			App: feedbackAppName, Tabs: feedbackTabs,
			Build: func(ctx context.Context, tables store.Tables, m *Model) (err error) {
				m.Feedback, err = buildFeedback(ctx, tables)
				return err
			},
			Loaded: func(m *Model, d time.Duration) {
				slog.Info("loaded feedback model", "reports", len(m.Feedback.reports), "took", took(d))
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

func noConsentStep(context.Context, *Model) error {
	return nil
}

func NewStore(source data.Source, writer data.Writer, queue *store.Queue, deps Deps) (*Store, error) {
	documents := &documentObjects{objects: deps.Objects, embedder: deps.Embedder, idKey: deps.IDKey, held: map[string]*Document{}}
	s, err := store.New(parts(deps, documents), noConsentStep, source, writer, queue)
	if err != nil {
		return nil, err
	}
	out := &Store{Store: s, documents: documents, unlocated: make(chan struct{}, 1)}
	queue.OnSwap(out.locate)
	return out, nil
}

type adminApp struct {
	key        string
	sheet      string
	allowances []access.Allowance
	listed     func(m *Model) []string
}

var adminApps = []adminApp{
	{"who", DirectoryApp, WhoAdminAllowances, func(m *Model) []string { return m.Directory.admins }},
	{"team", activitiesAppName, ActivitiesAdminAllowances, func(m *Model) []string { return m.Activities.admins }},
	{"birthday", birthdaysAppName, BirthdaysAdminAllowances, func(m *Model) []string { return m.Birthdays.admins }},
	{"celebrate", partiesAppName, PartiesAdminAllowances, func(m *Model) []string { return m.Parties.admins }},
	{"when", CalendarApp, CalendarAdminAllowances, func(m *Model) []string { return m.Calendar.admins }},
	{"loop", emailListsAppName, EmailListsAdminAllowances, func(m *Model) []string { return m.EmailLists.admins }},
	{"home", homeAppName, HomeAdminAllowances, func(m *Model) []string { return m.Home.admins }},
}

func (m *Model) AdminList(app string) AdminList {
	for _, a := range adminApps {
		if a.key == app {
			return AdminList{app: a.key, sheet: a.sheet, allowances: append(slices.Clone(a.allowances), ManageAdmins(a.key)), listed: a.listed(m), superAdmins: m.Config.SuperAdmins}
		}
	}
	panic("model: no admin list for " + app)
}

func (m *Model) IsSuperAdmin(email string) bool {
	return slices.Contains(m.Config.SuperAdmins, mail.Normalize(email))
}

func (m *Model) SuperHeld(email string) []access.Allowance {
	if !m.IsSuperAdmin(email) {
		return nil
	}
	return SuperAllowances
}

func (m *Model) Held(email string) []access.Allowance {
	out := []access.Allowance{}
	for _, a := range adminApps {
		out = append(out, m.AdminList(a.key).Held(email)...)
	}
	return append(out, m.SuperHeld(email)...)
}

func (m *Model) actor(r *http.Request, app string) access.Actor {
	return m.Directory.Actor(r, m.AdminList(app).Held)
}

func (m *Model) actorOf(email, app string) access.Actor {
	return m.Directory.ActorOf(email, m.AdminList(app).Held(email))
}

func (s *Store) Held(email string) []access.Allowance {
	return s.Model().Held(email)
}

func (s *Store) IsSuperAdmin(email string) bool {
	return s.Model().IsSuperAdmin(email)
}

func (s *Store) SignedOut(email string) (time.Time, bool) {
	at, ok := s.Model().Config.SignedOut[mail.Normalize(email)]
	return at, ok
}

func (s *Store) SignOut(ctx context.Context, email string) error {
	actor := access.Actor{Email: mail.Normalize(email)}
	return s.Commit(ctx, actor, ConfigApp, signedOut(actor.Email)...)
}

func (s *Store) Member(email string) bool {
	d := s.Model().Directory
	return d.Member(d.Resolve(mail.Normalize(email)))
}
