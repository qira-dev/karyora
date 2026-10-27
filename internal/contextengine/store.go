package contextengine

import (
	"context"
)

// SnapshotStore is the persistence contract for immutable ContextSnapshots
// (C3-10). The storage package implements it; this package never touches a
// database.
type SnapshotStore interface {
	// SaveSnapshot stores one immutable snapshot. Saving an id that
	// already exists is a conflict, never an overwrite.
	SaveSnapshot(ctx context.Context, snap ContextSnapshot) error
	// LoadSnapshot loads exactly one stored snapshot.
	LoadSnapshot(ctx context.Context, snapshotID string) (ContextSnapshot, error)
}

// MemoryStore is the persistence contract for the scoped Memory subsystem
// (C3-11). Every query is scope-bound; cross-scope reads are denied at the
// port level by taking the scope as a required parameter.
type MemoryStore interface {
	// SaveMemory stores one record. Saving an id that already exists is a
	// conflict, never an overwrite (corrections create new records).
	SaveMemory(ctx context.Context, record MemoryRecord) error
	// LoadMemory loads one record within the named scope; a record from a
	// different scope is NOT_FOUND, never returned.
	LoadMemory(ctx context.Context, scope, memoryID string) (MemoryRecord, error)
	// ListMemory returns the records of one scope in creation order.
	ListMemory(ctx context.Context, scope string) ([]MemoryRecord, error)
	// UpdateMemoryStatus moves the visible lifecycle status of one record
	// within the named scope (corrected/expired/deleted + superseded-by).
	// Content and provenance are never touched by a status move.
	UpdateMemoryStatus(ctx context.Context, scope, memoryID, status, supersededBy string) error
}
