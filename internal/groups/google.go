package groups

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
)

// Google is the Google Groups side as the sync needs it: a group made or
// brought up to date, its members read, one added or removed, a group
// deleted, and every address under the domain.
type Google interface {
	Ensure(ctx context.Context, address, title, description string) error
	Members(ctx context.Context, address string) ([]string, error)
	Add(ctx context.Context, address, email string) error
	Remove(ctx context.Context, address, email string) error
	Delete(ctx context.Context, address string) error
	Addresses(ctx context.Context) ([]string, error)
}

// Fake is Google in memory, for sample mode and tests: it holds the groups
// it is given and logs every change.
type Fake struct {
	mu     sync.Mutex
	groups map[string]*fakeGroup
	// Calls counts every call by method, for tests.
	Calls map[string]int
}

type fakeGroup struct {
	title, description string
	members            map[string]bool
}

func NewFake() *Fake {
	return &Fake{groups: map[string]*fakeGroup{}, Calls: map[string]int{}}
}

func (f *Fake) count(method string) {
	f.Calls[method]++
}

func (f *Fake) Ensure(_ context.Context, address, title, description string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Ensure")
	g, ok := f.groups[address]
	if !ok {
		f.groups[address] = &fakeGroup{title: title, description: description, members: map[string]bool{}}
		slog.Info("groups: would create a google group", "address", address, "title", title)
		return nil
	}
	if g.title != title || g.description != description {
		g.title, g.description = title, description
		slog.Info("groups: would rename a google group", "address", address, "title", title)
	}
	return nil
}

func (f *Fake) Members(_ context.Context, address string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Members")
	g, ok := f.groups[address]
	if !ok {
		return nil, fmt.Errorf("no group %s", address)
	}
	out := []string{}
	for email := range g.members {
		out = append(out, email)
	}
	sort.Strings(out)
	return out, nil
}

func (f *Fake) Add(_ context.Context, address, email string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Add")
	g, ok := f.groups[address]
	if !ok {
		return fmt.Errorf("no group %s", address)
	}
	g.members[email] = true
	slog.Info("groups: would add a member", "address", address, "email", email)
	return nil
}

func (f *Fake) Remove(_ context.Context, address, email string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Remove")
	g, ok := f.groups[address]
	if !ok {
		return fmt.Errorf("no group %s", address)
	}
	delete(g.members, email)
	slog.Info("groups: would remove a member", "address", address, "email", email)
	return nil
}

func (f *Fake) Delete(_ context.Context, address string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Delete")
	delete(f.groups, address)
	slog.Info("groups: would delete a google group", "address", address)
	return nil
}

func (f *Fake) Addresses(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.count("Addresses")
	out := []string{}
	for address := range f.groups {
		out = append(out, address)
	}
	sort.Strings(out)
	return out, nil
}

// Seed puts a group with members in place, as a test's starting state.
func (f *Fake) Seed(address, title string, members []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g := &fakeGroup{title: title, members: map[string]bool{}}
	for _, m := range members {
		g.members[m] = true
	}
	f.groups[address] = g
}

// Has says whether the fake holds a member of a group.
func (f *Fake) Has(address, email string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.groups[address]
	return ok && g.members[email]
}

// Titles is every group's title by address.
func (f *Fake) Titles() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]string{}
	for address, g := range f.groups {
		out[address] = g.title
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
