package model

import (
	"sync"
	"time"

	"heliosian/internal/api"
)

type scope struct {
	now time.Time

	placedOnce  sync.Once
	placed      placement
	keysOnce    sync.Once
	suggestions map[string]bool

	eventsOnce sync.Once
	events     []*Event
	eventsByID map[string]*Event
	indexOnce  sync.Once
	guestLists map[string]string
	feeds      map[string]string
	answerOnce sync.Once
	responses  map[string]*Responses
}

func (m *Model) at(q api.Query) *Model {
	scoped := *m
	scoped.scope = &scope{now: q.Now}
	return &scoped
}
