package db

import "sync"

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
	inboxCount
)

var idleInboxes = []inboxName{consentInbox, fetchInbox, classifyInbox, extractInbox, geocodeInbox, sweepInbox}

type services struct {
	inboxes [inboxCount]*inbox
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

type delivery struct {
	change Change
	env    Env
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
