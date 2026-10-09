package db

import (
	"testing"
	"time"

	"heliosian/internal/store"
)

func TestAWriteBuildsEffectiveMembersBeforeAnyoneAsks(t *testing.T) {
	s, queue := sampleWithQueue(t)
	StartWarmer(s, queue)
	if err := commit(s, GroupsSheet, store.Update("GROUP", store.Row{"id": "grp00000000030"}, store.Row{"subtitle": "changed"})); err != nil {
		t.Fatal(err)
	}
	m := s.Model()
	effective, _ := Lookup("EFFECTIVE_MEMBER")
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		m.derived.mu.Lock()
		started := m.derived.sets[effective.Name] != nil
		m.derived.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no effective members were built after the write")
		}
	}
	if n := len(m.generated(effective).rows); n == 0 {
		t.Fatal("the effective members built after the write are empty")
	}
}
