package conntrack

import (
	"testing"
	"time"
)

func TestNewTracker(t *testing.T) {
	tracker := NewTracker()

	if tracker == nil {
		t.Fatal("expected tracker to be non-nil")
	}

	if tracker.conns == nil {
		t.Fatal("expected conns map to be initialized")
	}

	if tracker.Count() != 0 {
		t.Fatalf("expected empty tracker, got %d", tracker.Count())
	}
}

func TestTrackAndCleanup(t *testing.T) {
	tracker := NewTracker()

	record := &ConnRecord{
		ID:         "conn-1",
		ClientAddr: "10.0.0.1:50000",
		Backend:    "backend-1",
		StartTime:  time.Now(),
	}

	cleanup := tracker.Track(record)

	if tracker.Count() != 1 {
		t.Fatalf("expected 1 active connection, got %d", tracker.Count())
	}

	snapshot := tracker.Snapshot()

	if len(snapshot) != 1 {
		t.Fatalf("expected snapshot size 1, got %d", len(snapshot))
	}

	got := snapshot[0]

	if got.ID != record.ID {
		t.Errorf("expected ID %q, got %q", record.ID, got.ID)
	}

	if got.ClientAddr != record.ClientAddr {
		t.Errorf("expected ClientAddr %q, got %q", record.ClientAddr, got.ClientAddr)
	}

	if got.Backend != record.Backend {
		t.Errorf("expected Backend %q, got %q", record.Backend, got.Backend)
	}

	cleanup()

	if tracker.Count() != 0 {
		t.Fatalf("expected 0 active connections after cleanup, got %d", tracker.Count())
	}
}

func TestSnapshotReturnsCopy(t *testing.T) {
	tracker := NewTracker()

	record := &ConnRecord{
		ID:         "conn-1",
		ClientAddr: "10.0.0.1:50000",
		Backend:    "backend-1",
		StartTime:  time.Now(),
	}

	tracker.Track(record)

	snapshot := tracker.Snapshot()

	if len(snapshot) != 1 {
		t.Fatalf("expected snapshot size 1, got %d", len(snapshot))
	}

	// Mutate snapshot copy
	snapshot[0].Backend = "mutated-backend"

	// Original tracker data should remain unchanged
	latest := tracker.Snapshot()

	if latest[0].Backend != "backend-1" {
		t.Fatalf(
			"expected original backend to remain unchanged, got %q",
			latest[0].Backend,
		)
	}
}

func TestTrackMultipleConnections(t *testing.T) {
	tracker := NewTracker()

	cleanup1 := tracker.Track(&ConnRecord{
		ID:         "conn-1",
		ClientAddr: "10.0.0.1:50000",
		Backend:    "backend-1",
		StartTime:  time.Now(),
	})

	cleanup2 := tracker.Track(&ConnRecord{
		ID:         "conn-2",
		ClientAddr: "10.0.0.2:50001",
		Backend:    "backend-2",
		StartTime:  time.Now(),
	})

	if tracker.Count() != 2 {
		t.Fatalf("expected 2 active connections, got %d", tracker.Count())
	}

	cleanup1()

	if tracker.Count() != 1 {
		t.Fatalf("expected 1 active connection, got %d", tracker.Count())
	}

	cleanup2()

	if tracker.Count() != 0 {
		t.Fatalf("expected 0 active connections, got %d", tracker.Count())
	}
}
