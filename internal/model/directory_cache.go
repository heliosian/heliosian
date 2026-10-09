package model

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

var DirectoryTabs = []store.Tab{
	{Name: EmailAliasesTab, Columns: EmailAliasColumns, Key: []string{EmailAliasColumn}},
	{Name: studentsTab, Columns: importColumns, Key: []string{"entry_sort_name"}},
	{Name: staffTab, Columns: staffImportColumns, Key: []string{"entry_sort_name"}},
	{Name: namesTab, Columns: []string{"Name", "Email"}, Key: []string{"Name"}},
	{Name: overridesTab, Columns: overrideColumns, Key: []string{"Email"}},
	{Name: familiesTab, Columns: familyColumns, Key: []string{"Email"}},
	{Name: WebsiteTable, Columns: WebsiteColumns, Key: []string{WebsiteID}},
	{Name: tagListTable, Columns: tagListColumns, Key: []string{tagID}},
	{Name: tagsTable, Columns: tagColumns, Key: tagColumns},
	{Name: managersTable, Columns: managerColumns, Key: managerColumns},
	{Name: photosTab, Columns: photoColumns, Key: []string{"Email", "Photo Name"}},
	{Name: imagesTab, Columns: imageColumns, Key: []string{imageKind, imageName}},
	AdminsTab,
	{Name: geocodeTable, Columns: geocodeColumns, Key: []string{geocodeAddress}, AppendOnly: true},
}

func directoryTabs() []store.Tab {
	return append(slices.Clone(DirectoryTabs),
		store.Tab{App: preferencesApp, Name: preferencesTab, Columns: []string{preferenceTimestamp, preferenceEmail, preferenceStatus, preferencePermission}, Key: []string{preferenceTimestamp, preferenceEmail}},
	)
}

func (s *Store) locate() {
	directory := s.Model().Directory
	if directory == s.located {
		return
	}
	s.located = directory
	select {
	case s.unlocated <- struct{}{}:
	default:
	}
}

func (s *Store) Locate(geocoder *geocode.Client) {
	for range s.unlocated {
		s.geocode(geocoder)
	}
}

func (s *Store) geocode(geocoder *geocode.Client) {
	directory := s.Model().Directory
	if directory == nil {
		return
	}
	missing := directory.unlocated
	if len(missing) == 0 {
		return
	}
	start := time.Now()
	jobs := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	found := map[string]geocode.Point{}
	for range 8 {
		wg.Go(func() {
			for address := range jobs {
				point, err := geocoder.Lookup(address)
				if err != nil {
					slog.Error("geocode", "error", err)
					continue
				}
				mu.Lock()
				found[address] = point
				mu.Unlock()
			}
		})
	}
	for _, address := range missing {
		jobs <- address
	}
	close(jobs)
	wg.Wait()
	actor := access.System("geocoder")
	ops := directory.locate(actor, found)
	if err := s.Commit(context.Background(), actor, DirectoryApp, ops...); err != nil {
		slog.Error("record geocoded addresses", "error", err)
	}
	slog.Info("geocoded family addresses", "looked up", len(missing), "found", len(ops), "took", time.Since(start).Round(time.Millisecond))
}
