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

	mailOnce     sync.Once
	mailReadable map[string]bool

	partiesOnce sync.Once
	partyKeys   map[string]*Party

	volunteersOnce sync.Once
	volunteerKeys  map[string]volunteerKey
	linkedOnce     sync.Once
	activityEvents map[string]string

	teamWidgetOnce sync.Once
	teamWidget     activityWidget

	homeOnce       sync.Once
	home           []HomeCategory
	homeLinks      map[string]HomeLink
	homeCategories map[string]HomeCategory
	schoolOnce     sync.Once
	schoolOrder    []string
	school         map[string]SchoolEmail
	toDosOnce      sync.Once
	toDoOrder      []string
	toDos          map[string]*ToDo

	magicKeysOnce sync.Once
	magicKeys     map[string]bool
	heldOnce      sync.Once
	heldOrder     []string
	held          map[string]MagicTag

	recordsOnce sync.Once
	records     map[string]string
}

func (m *Model) at(q api.Query) *Model {
	scoped := *m
	scoped.scope = &scope{now: q.Now}
	return &scoped
}
