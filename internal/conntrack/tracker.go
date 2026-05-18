package conntrack

import (
	"sync"
	"time"
)

type ConnRecord struct {
	ID         string
	ClientAddr string
	Backend    string
	StartTime  time.Time
}

type Tracker struct {
	mu    sync.RWMutex
	conns map[string]*ConnRecord
}

func NewTracker() *Tracker {
	return &Tracker{
		conns: make(map[string]*ConnRecord),
	}
}

func (t *Tracker) Track(r *ConnRecord) func() {
	t.mu.Lock()
	t.conns[r.ID] = r
	t.mu.Unlock()

	return func() {
		t.mu.Lock()
		delete(t.conns, r.ID)
		t.mu.Unlock()
	}
}

// Snapshot returns a copy of all active connections.
// Returns a copy so callers don't hold the lock.
func (t *Tracker) Snapshot() []*ConnRecord {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*ConnRecord, 0, len(t.conns))
	for _, r := range t.conns {
		copy := *r
		result = append(result, &copy)
	}
	return result
}

// Returns the number of active connections.
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.conns)
}
