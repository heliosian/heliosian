package who

import (
	"sync"
	"testing"

	"heliosian/internal/data"
	"heliosian/internal/geocode"
)

type countingGeocoder struct {
	mu    sync.Mutex
	calls int
}

func (g *countingGeocoder) Lookup(address string) (geocode.Point, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	return geocode.Fake{}.Lookup(address)
}

func TestGeocodeTabAnswersTheSecondRebuild(t *testing.T) {
	dir := &data.Dir{Root: "../../sampledata"}
	geocoder := &countingGeocoder{}
	queue := NewQueue()
	cache, err := NewCache(dir, dir, geocoder, noBlobs{}, noBlobs{}, nil, queue, testKey, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	first := geocoder.calls
	if first == 0 {
		t.Fatal("first build asked the geocoder nothing")
	}
	written := make(chan struct{})
	queue.Add(func() { close(written) })
	<-written
	tables, err := ReadTables(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables.Geocode) != first {
		t.Fatalf("geocode tab holds %d rows after %d lookups", len(tables.Geocode), first)
	}
	if err := cache.refresh(); err != nil {
		t.Fatal(err)
	}
	if geocoder.calls != first {
		t.Fatalf("second build asked the geocoder %d more times", geocoder.calls-first)
	}
	located := 0
	for _, family := range cache.Model().Families {
		if family.Address != "" && family.Lat != 0 {
			located++
		}
	}
	if located == 0 {
		t.Fatal("no family has coordinates after the second build")
	}
}
