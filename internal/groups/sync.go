package groups

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// Desired is one group as Google should hold it.
type Desired struct {
	Name        string
	Title       string
	Description string
	Members     []string
}

func (d Desired) Address() string {
	return d.Name + "@" + Domain
}

// Result is what a reconciliation did: how many groups it looked at, the
// members it added and removed, the groups it made, the addresses under the
// domain the sheet does not list, and what went wrong per group.
type Result struct {
	Groups  int
	Added   int
	Removed int
	Orphans []string
	Errors  map[string]error
}

func diff(current, wanted []string) (add, remove []string) {
	have := map[string]bool{}
	for _, m := range current {
		have[strings.ToLower(m)] = true
	}
	want := map[string]bool{}
	for _, m := range wanted {
		want[m] = true
		if !have[m] {
			add = append(add, m)
		}
	}
	for m := range have {
		if !want[m] {
			remove = append(remove, m)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	return add, remove
}

// apply brings one group's members to wanted from current: the adds and
// removes, stopping at the first failure so the count says what landed.
func apply(ctx context.Context, g Google, address string, current, wanted []string) (added, removed int, err error) {
	add, remove := diff(current, wanted)
	for _, email := range add {
		if err := g.Add(ctx, address, email); err != nil {
			return added, removed, err
		}
		added++
	}
	for _, email := range remove {
		if err := g.Remove(ctx, address, email); err != nil {
			return added, removed, err
		}
		removed++
	}
	return added, removed, nil
}

// reconcile brings one group fully in step: made or renamed, then its
// members read and corrected.
func reconcile(ctx context.Context, g Google, d Desired) (added, removed int, err error) {
	if err := g.Ensure(ctx, d.Address(), d.Title, d.Description); err != nil {
		return 0, 0, err
	}
	current, err := g.Members(ctx, d.Address())
	if err != nil {
		return 0, 0, err
	}
	return apply(ctx, g, d.Address(), current, d.Members)
}

// Reconcile is the periodic job's pass: every group made or renamed, every
// membership read from Google and corrected, and the addresses under the
// domain the sheet does not list reported, never deleted - a group goes
// when a person deletes it in the app.
func Reconcile(ctx context.Context, g Google, desired []Desired, logf func(format string, args ...any)) Result {
	result := Result{Errors: map[string]error{}}
	for _, d := range desired {
		result.Groups++
		added, removed, err := reconcile(ctx, g, d)
		result.Added += added
		result.Removed += removed
		if err != nil {
			result.Errors[d.Name] = err
			logf("[ERROR] %s: %v", d.Address(), err)
			continue
		}
		logf("%s: %d members, %d added, %d removed", d.Address(), len(d.Members), added, removed)
	}
	addresses, err := g.Addresses(ctx)
	if err != nil {
		result.Errors[""] = err
		logf("[ERROR] list the groups: %v", err)
		return result
	}
	listed := map[string]bool{}
	for _, d := range desired {
		listed[d.Address()] = true
	}
	for _, address := range addresses {
		if !listed[address] {
			result.Orphans = append(result.Orphans, address)
			logf("[WARN] %s is under %s but no group in the sheet has it; delete it by hand if it is nobody's", address, Domain)
		}
	}
	return result
}

// Status is where one group stands with Google, for its page.
type Status struct {
	Synced  time.Time `json:"synced,omitzero"`
	Error   string    `json:"error,omitempty"`
	Members int       `json:"members"`
}

// Syncer keeps Google in step with the plan as the models change: told of
// a change, it waits a moment for the rest of a burst, recomputes every
// group, and sends Google only the difference from what it last sent. Its
// first pass reads every group back from Google, since it has nothing to
// diff against.
type Syncer struct {
	google Google
	plan   func() []Desired
	mu     sync.Mutex
	last   map[string]Desired
	status map[string]Status
	timer  *time.Timer
	// running and again serialize passes: a change during a pass runs
	// another one after it rather than alongside.
	running bool
	again   bool
}

const settle = 2 * time.Second

func NewSyncer(google Google, plan func() []Desired) *Syncer {
	return &Syncer{google: google, plan: plan, status: map[string]Status{}}
}

// Notify says the models changed; a pass follows once they settle.
func (s *Syncer) Notify() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(settle, s.start)
}

func (s *Syncer) start() {
	s.mu.Lock()
	if s.running {
		s.again = true
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	for {
		s.pass(context.Background())
		s.mu.Lock()
		if !s.again {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.again = false
		s.mu.Unlock()
	}
}

// Run is one pass now, synchronously, for a start or a test.
func (s *Syncer) Run(ctx context.Context) {
	s.pass(ctx)
}

func (s *Syncer) pass(ctx context.Context) {
	start := time.Now()
	desired := s.plan()
	s.mu.Lock()
	last := s.last
	s.mu.Unlock()
	next := map[string]Desired{}
	added, removed, failed := 0, 0, 0
	for _, d := range desired {
		var a, r int
		var err error
		previous, known := last[d.Name]
		switch {
		case last == nil || !known:
			a, r, err = reconcile(ctx, s.google, d)
		default:
			if previous.Title != d.Title || previous.Description != d.Description {
				err = s.google.Ensure(ctx, d.Address(), d.Title, d.Description)
			}
			if err == nil {
				a, r, err = apply(ctx, s.google, d.Address(), previous.Members, d.Members)
			}
		}
		added += a
		removed += r
		status := Status{Synced: time.Now(), Members: len(d.Members)}
		if err != nil {
			failed++
			slog.Error("groups: sync", "group", d.Address(), "error", err)
			status = Status{Error: err.Error(), Members: len(d.Members)}
			if known {
				status.Synced = s.status[d.Name].Synced
				next[d.Name] = previous
			}
		} else {
			next[d.Name] = d
		}
		s.mu.Lock()
		s.status[d.Name] = status
		s.mu.Unlock()
	}
	for name, previous := range last {
		if slices.ContainsFunc(desired, func(d Desired) bool { return d.Name == name }) {
			continue
		}
		if err := s.google.Delete(ctx, previous.Address()); err != nil {
			failed++
			slog.Error("groups: delete", "group", previous.Address(), "error", err)
			next[name] = previous
			continue
		}
		s.mu.Lock()
		delete(s.status, name)
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.last = next
	s.mu.Unlock()
	slog.Info("groups: synced", "groups", len(desired), "added", added, "removed", removed, "failed", failed, "took", time.Since(start).Round(time.Millisecond))
}

// Status is where a group stands, zero for one not synced yet.
func (s *Syncer) Status(name string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status[name]
}
