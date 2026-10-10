package db

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/geocode"
	"heliosian/internal/store"
)

const geocodeLookups = 8

var streetNumber = regexp.MustCompile(`^\s*\d`)

func Placeable(address string) bool {
	return streetNumber.MatchString(address)
}

type geocoder struct {
	s      *Store
	queue  *store.Queue
	pics   *Pictures
	client *geocode.Client
	failed map[string]bool
	poke   chan struct{}
}

func StartGeocoder(s *Store, queue *store.Queue, pics *Pictures, client *geocode.Client) {
	g := &geocoder{s: s, queue: queue, pics: pics, client: client, failed: map[string]bool{}, poke: make(chan struct{}, 1)}
	go g.run()
	queue.OnSwap(g.wake)
	g.wake()
}

func (g *geocoder) wake() {
	select {
	case g.poke <- struct{}{}:
	default:
	}
}

func (m *Model) unplaced() []string {
	held := map[string]bool{}
	for _, row := range m.Table("GEOCODE").All() {
		held[row["address"]] = true
	}
	out := []string{}
	for _, row := range m.Shown("GROUP").All() {
		if row["kind"] == "family" && Placeable(row["address"]) && !held[row["address"]] && !slices.Contains(out, row["address"]) {
			out = append(out, row["address"])
		}
	}
	slices.Sort(out)
	return out
}

func (m *Model) unplaceable() []string {
	out := []string{}
	for _, row := range m.Table("GEOCODE").All() {
		if !Placeable(row["address"]) {
			out = append(out, row["id"])
		}
	}
	return out
}

func (g *geocoder) run() {
	for range g.poke {
		m := g.s.Model()
		addresses := slices.DeleteFunc(m.unplaced(), func(a string) bool { return g.failed[a] })
		gone := slices.DeleteFunc(m.unplaceable(), func(id string) bool { return g.failed[id] })
		if len(addresses) == 0 && len(gone) == 0 {
			continue
		}
		start := time.Now()
		found := make([]geocode.Point, len(addresses))
		errs := make([]error, len(addresses))
		next := make(chan int)
		wg := sync.WaitGroup{}
		for range geocodeLookups {
			wg.Go(func() {
				for i := range next {
					found[i], errs[i] = g.client.Lookup(addresses[i])
				}
			})
		}
		for i := range addresses {
			next <- i
		}
		close(next)
		wg.Wait()
		edits := []Edit{}
		for i, address := range addresses {
			if errs[i] != nil {
				slog.Error("geocode: look up, left until a restart", "address", address, "error", errs[i])
				g.failed[address] = true
				continue
			}
			edits = append(edits, Edit{Insert: "GEOCODE", Row: map[string]any{"address": address, "lat": strconv.FormatFloat(found[i].Lat, 'f', -1, 64), "lng": strconv.FormatFloat(found[i].Lng, 'f', -1, 64)}})
		}
		placed := len(edits)
		for _, id := range gone {
			edits = append(edits, Edit{Delete: id})
		}
		if len(edits) == 0 {
			continue
		}
		env := Env{System: "geocoder", Now: time.Now()}
		if _, err := Write(context.Background(), g.s, g.queue, g.pics, access.System(env.System), env, Batch{Batch: edits}); err != nil {
			slog.Error("geocode: record", "placed", placed, "removed", len(gone), "error", err)
			for _, key := range slices.Concat(addresses, gone) {
				g.failed[key] = true
			}
			continue
		}
		slog.Info("geocode: placed family addresses", "looked_up", len(addresses), "placed", placed, "removed", len(gone), "took", time.Since(start).Round(time.Millisecond))
	}
}
