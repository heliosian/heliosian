package db

import (
	"sync"
	"sync/atomic"

	"heliosian/internal/store"
)

type inboxName int

const (
	mailInbox inboxName = iota
	picturesInbox
	searchInbox
	warmInbox
	consentInbox
	fetchInbox
	classifyInbox
	extractInbox
	geocodeInbox
	sweepInbox
	inviteInbox
	birthdayInbox
	inboxCount
)

var idleInboxes = []inboxName{consentInbox, fetchInbox, classifyInbox, extractInbox, geocodeInbox, sweepInbox}

type services struct {
	inboxes [inboxCount]*inbox
	writes  atomic.Uint64
}

type heldWrite struct {
	write uint64
	by    map[*inbox][]delivery
	order []*inbox
}

func newServices(index *SearchIndex) *services {
	sv := &services{}
	for i := range sv.inboxes {
		sv.inboxes[i] = &inbox{wake: newWake()}
	}
	sv.inboxes[searchInbox].wake = index.poke
	return sv
}

func (sv *services) inbox(name inboxName) *inbox {
	return sv.inboxes[name]
}

func (sv *services) collect(tx *store.Tx, in *inbox, d delivery) {
	h := tx.Value(sv, func() any {
		h := &heldWrite{write: sv.writes.Add(1), by: map[*inbox][]delivery{}}
		tx.Swapped(func() {
			for _, in := range h.order {
				in.deliverAll(h.by[in])
			}
		})
		return h
	}).(*heldWrite)
	if _, ok := h.by[in]; !ok {
		h.order = append(h.order, in)
	}
	d.write = h.write
	h.by[in] = append(h.by[in], d)
}

type delivery struct {
	change Change
	env    Env
	write  uint64
}

type inbox struct {
	mu      sync.Mutex
	pending []delivery
	wake    wake
}

func (in *inbox) deliver(d delivery) {
	in.mu.Lock()
	in.pending = append(in.pending, d)
	in.mu.Unlock()
	in.wake.poke()
}

func (in *inbox) deliverAll(ds []delivery) {
	in.mu.Lock()
	in.pending = append(in.pending, ds...)
	in.mu.Unlock()
	in.wake.poke()
}

func (in *inbox) take() []delivery {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := in.pending
	in.pending = nil
	return out
}

func (in *inbox) giveBack(ds []delivery) {
	in.mu.Lock()
	in.pending = append(ds, in.pending...)
	in.mu.Unlock()
}

func StartIdleWorkers(s *Store) {
	for _, name := range idleInboxes {
		in := s.Model().services.inbox(name)
		go func() {
			for range in.wake {
				in.take()
			}
		}()
	}
}

type wake chan struct{}

func newWake() wake {
	return make(wake, 1)
}

func (w wake) poke() {
	select {
	case w <- struct{}{}:
	default:
	}
}
