package storage

import (
	"context"
)

// The Store implements the runtime machine's storage-side steps (C2-09 §2).
// The composition root wires them as runtime.Ports; the kernel lock held by
// the runner is what makes this the only store instance.

// Lifecycle event aggregates (stable English identifiers).
const (
	AggregateCore = "core"
)

// Migrate runs startup step 3 (C2-09 §2): the store is already open with
// the verified option matrix; this runs the integrity check and the
// ordered migration pass. MIGRATION_FAILED gates everything; no dispatch on
// a half-migrated schema, ever.
func (s *Store) Migrate(ctx context.Context, coreVersion string) error {
	if err := s.IntegrityCheck(ctx); err != nil {
		return err
	}
	return s.migrate(ctx, coreVersion)
}

// Reconcile runs step 4: journal/outbox consistency, CAS staging
// completion, and reachability-root registration; idempotent and
// re-runnable (C2-09 §2, C2-12/C2-13 policies).
func (s *Store) Reconcile(ctx context.Context) error {
	if err := s.ReconcileOutbox(ctx); err != nil {
		return err
	}
	if _, _, err := s.cas.ReconcileStaging(ctx); err != nil {
		return err
	}
	s.registerReferenceRoots()
	s.pub.Start()
	return nil
}

// registerReferenceRoots wires the reachability roots that exist in the
// Phase 2 slice: the journal's payload blob references and the durable
// cas_references rows any module owner recorded. Future module owners must
// register their reference type with the CAS before their data can ever be
// collected (C2-12 §4).
func (s *Store) registerReferenceRoots() {
	s.cas.RegisterReferenceType("events.payload", func(ctx context.Context) (map[string]bool, error) {
		rows, err := s.readers.QueryContext(ctx, `SELECT DISTINCT blob_ref FROM events_journal WHERE blob_ref != ''`)
		if err != nil {
			return nil, mapVendorError(err)
		}
		defer rows.Close()
		set := map[string]bool{}
		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				return nil, mapVendorError(err)
			}
			set[ref] = true
		}
		return set, rows.Err()
	})
	s.cas.RegisterReferenceType("cas.references", func(ctx context.Context) (map[string]bool, error) {
		rows, err := s.readers.QueryContext(ctx, `SELECT DISTINCT blob_ref FROM cas_references`)
		if err != nil {
			return nil, mapVendorError(err)
		}
		defer rows.Close()
		set := map[string]bool{}
		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				return nil, mapVendorError(err)
			}
			set[ref] = true
		}
		return set, rows.Err()
	})
}

// BumpGeneration runs step 5: the persisted monotonic recovery generation
// bumped once per startup reaching this step; the returned value is the
// local fencing token (C2-09 §3). Persisted in core_meta under the held
// kernel lock and the single-writer gate.
func (s *Store) BumpGeneration(ctx context.Context) (uint64, error) {
	var generation uint64
	err := s.withWriter(ctx, func(tx *sqlTx) error {
		if _, err := tx.Exec(`INSERT INTO core_meta (key, value) VALUES ('recovery_generation', '0')
			ON CONFLICT(key) DO NOTHING`); err != nil {
			return mapVendorError(err)
		}
		if _, err := tx.Exec(`UPDATE core_meta SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT) WHERE key = 'recovery_generation'`); err != nil {
			return mapVendorError(err)
		}
		var v string
		if err := tx.QueryRow(`SELECT value FROM core_meta WHERE key = 'recovery_generation'`).Scan(&v); err != nil {
			return mapVendorError(err)
		}
		generation = parseUint(v)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return generation, nil
}

// FencePrior runs steps 6–7 for the Phase 2 local slice: every Attempt of a
// generation older than current would be fenced here. The D-12/D-33
// exclusion means no Attempt/Job tables exist yet, so the fence is recorded
// durably as a lifecycle event and the quarantine list is empty by
// construction (visible, never silently skipped).
func (s *Store) FencePrior(ctx context.Context, correlationID string, generation uint64) error {
	return s.RecordLifecycleEvent(ctx, correlationID, AggregateCore, s.instanceUUID, "core.fence.completed", map[string]any{
		"generation":  generation,
		"quarantined": 0,
	})
}

// Persist is drain step 2: every state change already committed atomically
// with its journal/outbox rows (C2-07 §2), so required durable state is
// persisted by construction; the step exists as the explicit safe-order
// boundary and stays honest about being empty.
func (s *Store) Persist(ctx context.Context) error {
	return nil
}

// FlushEvents is drain step 3: one bounded publisher pass flushes pending
// outbox rows, then the lifecycle writer is stopped (C2-07 §2 safe order).
func (s *Store) FlushEvents(ctx context.Context) error {
	if err := s.pub.deliverPass(ctx); err != nil {
		return mapVendorError(err)
	}
	return nil
}

// StopPublisher ends the bounded poller (called on shutdown).
func (s *Store) StopPublisher() {
	s.pub.Stop()
}

// Signals exports the storage health signals (C2-05 §3): WAL/checkpoint
// group and the bounded writer queue depth. The runtime converts them into
// the report model (storage does not import the health package).
func (s *Store) Signals() (walSizeBytes uint64, starving bool, queueDepth int64, rejections int64) {
	m := s.Metrics()
	return m.WALSizeBytes, m.CheckpointStarving, m.QueueDepth, m.QueueRejections
}

// Generation reports the persisted recovery generation (diagnostics).
func (s *Store) Generation(ctx context.Context) (uint64, error) {
	cctx, cancel := s.readerCtx(ctx)
	defer cancel()
	var v string
	err := s.readers.QueryRowContext(cctx, `SELECT value FROM core_meta WHERE key = 'recovery_generation'`).Scan(&v)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return 0, nil
		}
		return 0, mapVendorError(err)
	}
	return parseUint(v), nil
}

// parseUint parses a small non-negative decimal from core_meta.
func parseUint(s string) uint64 {
	var n uint64
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + uint64(c-'0')
	}
	return n
}
