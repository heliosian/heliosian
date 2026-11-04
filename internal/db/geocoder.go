package db

import (
	"context"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"time"

	"heliosian/internal/access"
	"heliosian/internal/geocode"
	"heliosian/internal/parallel"
)

const geocodeLookups = 8

var streetNumber = regexp.MustCompile(`^\s*\d`)

func Placeable(address string) bool {
	return streetNumber.MatchString(address)
}

type geocoder struct {
	s      *Store
	client *geocode.Client
	failed map[string]bool
	in     *inbox
}

func StartGeocoder(s *Store, client *geocode.Client) {
	g := &geocoder{s: s, client: client, failed: map[string]bool{}, in: s.Model().services.inbox(geocodeInbox)}
	go g.run()
	g.in.wake.poke()
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
	for range g.in.wake {
		g.in.take()
		m := g.s.Model()
		addresses := slices.DeleteFunc(m.unplaced(), func(address string) bool { return g.failed[address] })
		gone := slices.DeleteFunc(m.unplaceable(), func(id string) bool { return g.failed[id] })
		if len(addresses) == 0 && len(gone) == 0 {
			continue
		}
		start := time.Now()
		lookups := parallel.Run(addresses, geocodeLookups, g.client.Lookup)
		edits := []Edit{}
		for _, result := range lookups {
			address, point := result.Input, result.Value
			if result.Err != nil {
				slog.Error("geocode: look up, left until a restart", "address", address, "error", result.Err)
				g.failed[address] = true
				continue
			}
			edits = append(edits, Edit{Insert: "GEOCODE", Row: map[string]any{"address": address, "lat": strconv.FormatFloat(point.Lat, 'f', -1, 64), "lng": strconv.FormatFloat(point.Lng, 'f', -1, 64)}})
		}
		placed := len(edits)
		for _, id := range gone {
			edits = append(edits, Edit{Delete: id})
		}
		if len(edits) == 0 {
			continue
		}
		env := Env{System: "geocoder", Now: time.Now()}
		if _, err := Write(context.Background(), g.s, access.System(env.System), env, Batch{Batch: edits}); err != nil {
			slog.Error("geocode: record", "placed", placed, "removed", len(gone), "error", err)
			for _, address := range addresses {
				g.failed[address] = true
			}
			for _, id := range gone {
				g.failed[id] = true
			}
			continue
		}
		slog.Info("geocode: placed family addresses", "looked_up", len(lookups), "placed", placed, "removed", len(gone), "took", time.Since(start).Round(time.Millisecond))
	}
}
