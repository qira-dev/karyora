package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/knowledge"
	"github.com/qira-dev/karyora/internal/events"
)

// knowledgeDigest builds a 64-hex-char digest value in the
// algorithm-qualified form (the value FORMAT only).
func knowledgeDigest(seed byte) string {
	return "sha256:" + strings.Repeat("4", 63) + string(seed)
}

// knowledgeIntent builds a legal fresh import intent (the M-9 keying members
// carried; the store stamps the durable wall-clock values).
func knowledgeIntent(opID, key string, digest, createIntent string) knowledge.ImportIntent {
	return knowledge.ImportIntent{
		Operation: knowledge.Operation{
			OperationID:    opID,
			Kind:           knowledge.KindImport,
			State:          knowledge.StateSubmitted,
			CorrelationID:  "corr-" + opID,
			UntrustedLabel: knowledge.UntrustedLabel,
		},
		IdempotencyKey: key,
		SourceDigest:   digest,
		CreateIntent:   createIntent,
	}
}

// knowledgeCommit builds the atomic completion payload for one fresh
// pack-create import over one entry.
func knowledgeCommit(opID string, packIsNew bool) knowledge.ImportCommit {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return knowledge.ImportCommit{
		OperationID:   opID,
		CorrelationID: "corr-" + opID,
		Pack: knowledge.Pack{
			PackID:           "kpack-" + opID,
			SchemaVersion:    knowledge.PackSchemaVersion,
			Title:            "fixture",
			CreatedAt:        now,
			UpdatedAt:        now,
			CurrentVersionID: "kver-" + opID,
		},
		PackIsNew: packIsNew,
		Version: knowledge.PackVersion{
			PackID:         "kpack-" + opID,
			VersionID:      "kver-" + opID,
			SchemaVersion:  knowledge.PackSchemaVersion,
			PackRootDigest: knowledgeDigest('0'),
			EntryCount:     1,
			LicenseStatus:  "unknown",
			CreatedAt:      now,
		},
		Entries: []knowledge.EntryRecord{{
			Path:        "document.txt",
			ContentType: "text/plain",
			SizeBytes:   9,
			Digest:      knowledgeDigest('1'),
			Content:     []byte("some text"),
		}},
		Result: knowledge.OperationResult{
			Kind:      knowledge.KindImport,
			PackID:    "kpack-" + opID,
			VersionID: "kver-" + opID,
		},
	}
}

// knowledgeOpEvents reads one aggregate's journal records (the Replay
// stream is the aggregate type; the identity filter is the test's).
func knowledgeOpEvents(t *testing.T, s *Store, aggregateType, aggregateID string) []events.Record {
	t.Helper()
	recs, _, err := s.Replay(context.Background(), aggregateType, "", 100)
	if err != nil {
		t.Fatalf("replay %s: %v", aggregateType, err)
	}
	var out []events.Record
	for _, r := range recs {
		if r.AggregateID == aggregateID {
			out = append(out, r)
		}
	}
	return out
}

// TestKnowledgeImportM9ThreeOutcomes pins the M-9 digest-keyed class's
// three outcomes exactly as published: same key + same SOURCE DIGEST
// replays while live AND within the 72 h post-terminal window; same key +
// different bytes is the typed CONFLICT_IDEMPOTENCY_KEY with
// fingerprintMatch:false; a replay past the window is the typed
// CONFLICT_STATE.
func TestKnowledgeImportM9ThreeOutcomes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const key = "m9-key"
	digest := knowledgeDigest('a')
	op, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-1", key, digest, knowledge.CreateIntentPack))
	if err != nil || replayed {
		t.Fatalf("fresh submit: replayed=%v err=%v", replayed, err)
	}
	if op.State != knowledge.StateSubmitted || op.UntrustedLabel != knowledge.UntrustedLabel {
		t.Fatalf("fresh operation record wrong: %+v", op)
	}
	// The live replay: same key + same digest while the operation is live.
	again, replayedLive, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-2", key, digest, knowledge.CreateIntentPack))
	if err != nil || !replayedLive {
		t.Fatalf("live replay: replayed=%v err=%v", replayedLive, err)
	}
	if again.OperationID != op.OperationID {
		t.Fatalf("the live replay must return the EXISTING operation: %s vs %s", again.OperationID, op.OperationID)
	}
	// Same key + DIFFERENT bytes: the typed CONFLICT_IDEMPOTENCY_KEY with
	// the fingerprintMatch:false datum.
	conflict, conflictReplayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-3", key, knowledgeDigest('b'), knowledge.CreateIntentPack))
	if conflict.OperationID != "" || conflictReplayed || err == nil {
		t.Fatalf("different bytes must refuse, got op=%v replayed=%v err=%v", conflict, conflictReplayed, err)
	}
	ce, ok := err.(*KnowledgeIdempotencyConflictError)
	if !ok {
		t.Fatalf("expected the M-9 conflict carrier, got %T", err)
	}
	env, cerr := ce.Envelope(errs.NewCorrelationID())
	if cerr != nil {
		t.Fatalf("conflict envelope: %v", cerr)
	}
	if env.Code != errs.CodeConflictIdempotencyKey {
		t.Fatalf("the conflict must ride the published code: %+v", env)
	}
	if env.Data["fingerprintMatch"] != false {
		t.Fatalf("the M-9 conflict must carry fingerprintMatch:false: %+v", env)
	}
	if env.Data["keyScope"] != "knowledge.import" {
		t.Fatalf("the conflict must name its key scope: %+v", env)
	}
	// Terminal within the 72 h window: same key + same digest replays.
	if _, err := s.CompleteKnowledgeImport(ctx, knowledgeCommit("kop-1", true)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	completed, err := s.LoadKnowledgeOperation(ctx, "kop-1")
	if err != nil || completed.State != knowledge.StateCompleted {
		t.Fatalf("completed state: %+v err=%v", completed, err)
	}
	replayedOp, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-4", key, digest, knowledge.CreateIntentPack))
	if err != nil || !replayed || replayedOp.OperationID != op.OperationID {
		t.Fatalf("in-window replay: replayed=%v err=%v op=%+v", replayed, err, replayedOp)
	}
	// Past the window: the typed CONFLICT_STATE — never a fabricated
	// replay, never a second operation.
	s.now = func() time.Time { return time.Now().Add(KnowledgeReplayWindow).Add(time.Nanosecond) }
	pastOp, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-5", key, digest, knowledge.CreateIntentPack))
	if pastOp.OperationID != "" || replayed || err == nil {
		t.Fatalf("past-window replay must refuse, got op=%v replayed=%v err=%v", pastOp, replayed, err)
	}
	env2, ok := err.(*Error)
	if !ok || env2.Code != errs.CodeConflictState {
		t.Fatalf("expected the typed CONFLICT_STATE, got %v", err)
	}
}

// TestKnowledgeImportCreateIntentWindow pins the identity-create
// duplicate-create layer: a FRESH key with the same create-intent
// fingerprint (intent class + target pack + SOURCE DIGEST) inside the 1 h
// window returns the existing pack-version reference; past it the repeated
// create intent is the caller's explicit new operation.
func TestKnowledgeImportCreateIntentWindow(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	digest := knowledgeDigest('c')
	op, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-c1", "key-c1", digest, knowledge.CreateIntentPack))
	if err != nil || replayed {
		t.Fatalf("fresh create: replayed=%v err=%v", replayed, err)
	}
	if _, err := s.CompleteKnowledgeImport(ctx, knowledgeCommit("kop-c1", true)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// A fresh key, same intent fingerprint, inside the 1 h window: the
	// existing reference replays.
	dup, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-c2", "key-c2", digest, knowledge.CreateIntentPack))
	if err != nil || !replayed || dup.OperationID != op.OperationID {
		t.Fatalf("create-intent replay: replayed=%v err=%v op=%+v", replayed, err, dup)
	}
	// Past the 1 h window: the caller's explicit new operation.
	s.now = func() time.Time { return time.Now().Add(KnowledgeCreateReplayWindow).Add(time.Nanosecond) }
	fresh, replayed, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-c3", "key-c3", digest, knowledge.CreateIntentPack))
	if err != nil || replayed || fresh.OperationID != "kop-c3" {
		t.Fatalf("past-window create: replayed=%v err=%v op=%+v", replayed, err, fresh)
	}
}

// TestKnowledgeImportCommitAtomicity pins the completion commit: the pack,
// the version, the entries, and the terminal operation state land in one
// transaction; the family's events ride the journal; a stage past the
// terminal state is refused.
func TestKnowledgeImportCommitAtomicity(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	op, _, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-a", "key-a", knowledgeDigest('d'), knowledge.CreateIntentPack))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	for _, stage := range knowledge.ImportStages {
		if _, err := s.KnowledgeOperationStage(ctx, op.OperationID, op.CorrelationID, stage); err != nil {
			t.Fatalf("stage %s: %v", stage, err)
		}
	}
	final, err := s.CompleteKnowledgeImport(ctx, knowledgeCommit("kop-a", true))
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if final.State != knowledge.StateCompleted || final.Result == nil || final.Result.PackID != "kpack-kop-a" {
		t.Fatalf("completed operation wrong: %+v", final)
	}
	pack, err := s.LoadKnowledgePack(ctx, "kpack-kop-a")
	if err != nil || pack.Title != "fixture" || pack.CurrentVersionID != "kver-kop-a" {
		t.Fatalf("pack row wrong: %+v err=%v", pack, err)
	}
	version, err := s.LoadKnowledgeVersion(ctx, "kpack-kop-a", "kver-kop-a")
	if err != nil || version.EntryCount != 1 || !strings.HasPrefix(version.PackRootDigest, "sha256:") {
		t.Fatalf("version row wrong: %+v err=%v", version, err)
	}
	entries, err := s.KnowledgeEntries(ctx, "kpack-kop-a", "kver-kop-a")
	if err != nil || len(entries) != 1 || entries[0].Path != "document.txt" || string(entries[0].Content) != "some text" {
		t.Fatalf("entries wrong: %+v err=%v", entries, err)
	}
	// The journal carries the family's F10-shaped events with the register
	// aggregate/type values.
	opRecs := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgeImportOperation, op.OperationID)
	types := map[string]bool{}
	for _, r := range opRecs {
		types[r.EventType] = true
	}
	for _, want := range []string{knowledge.EventImportSubmitted, knowledge.EventImportStageEntered, knowledge.EventImportCompleted} {
		if !types[want] {
			t.Fatalf("the journal must carry %s: %v", want, opRecs)
		}
	}
	packRecs := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgePack, "kpack-kop-a")
	if len(packRecs) == 0 || packRecs[0].EventType != knowledge.EventPackCreated {
		t.Fatalf("pack event missing: %v", packRecs)
	}
	versionRecs := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgePackVersion, "kver-kop-a")
	if len(versionRecs) == 0 || versionRecs[0].EventType != knowledge.EventPackVersionCreated {
		t.Fatalf("pack-version event missing: %v", versionRecs)
	}
	// A stage on the terminal operation is refused.
	if _, err := s.KnowledgeOperationStage(ctx, op.OperationID, op.CorrelationID, knowledge.StateValidating); err == nil {
		t.Fatal("a stage past the terminal state must refuse")
	}
}

// TestKnowledgeOperationCancel pins the cancellation ruling: unknown → the
// typed NOT_FOUND (no-leak); terminal → the typed CONFLICT_STATE; live →
// the Core-confirmed cancelled state with the import.cancelled event.
func TestKnowledgeOperationCancel(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	// Unknown: the typed NOT_FOUND.
	if _, err := s.CancelKnowledgeOperation(ctx, "kop-none", "corr-x"); err == nil {
		t.Fatal("cancel of an unknown operation must refuse")
	} else if env, ok := err.(*Error); !ok || env.Code != errs.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
	// Live: the Core-confirmed cancellation.
	op, _, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-cancel", "key-cancel", knowledgeDigest('e'), knowledge.CreateIntentPack))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	cancelled, err := s.CancelKnowledgeOperation(ctx, op.OperationID, "corr-c")
	if err != nil || cancelled.State != knowledge.StateCancelled {
		t.Fatalf("live cancel: %+v err=%v", cancelled, err)
	}
	opRecs := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgeImportOperation, op.OperationID)
	if len(opRecs) == 0 || opRecs[len(opRecs)-1].EventType != knowledge.EventImportCancelled {
		t.Fatalf("the cancelled import must carry its event: %v", opRecs)
	}
	// Terminal (cancelled): refused past the boundary.
	if _, err := s.CancelKnowledgeOperation(ctx, op.OperationID, "corr-c2"); err == nil {
		t.Fatal("cancel of a terminal operation must refuse")
	} else if env, ok := err.(*Error); !ok || env.Code != errs.CodeConflictState {
		t.Fatalf("expected CONFLICT_STATE, got %v", err)
	}
}

// TestKnowledgeExportLifecycle pins the export's lifecycle: no idempotency
// (two submits mint two operations), the completed state with the CAS
// digest as the artifact reference, and the export.failed event on failure.
func TestKnowledgeExportLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	op1, err := s.SubmitKnowledgeExport(ctx, knowledge.Operation{
		OperationID: "kop-e1", Kind: knowledge.KindExport, State: knowledge.StateSubmitted, CorrelationID: "corr-e1",
	})
	if err != nil {
		t.Fatalf("submit 1: %v", err)
	}
	op2, err := s.SubmitKnowledgeExport(ctx, knowledge.Operation{
		OperationID: "kop-e2", Kind: knowledge.KindExport, State: knowledge.StateSubmitted, CorrelationID: "corr-e2",
	})
	if err != nil || op1.OperationID == op2.OperationID {
		t.Fatalf("the export class carries no idempotency: %+v %+v err=%v", op1, op2, err)
	}
	artifact := []byte("export artifact bytes")
	final, err := s.CompleteKnowledgeExport(ctx, knowledge.ExportCommit{
		OperationID:   op1.OperationID,
		CorrelationID: op1.CorrelationID,
		Result:        knowledge.OperationResult{Kind: knowledge.KindExport},
		Artifact:      artifact,
	})
	if err != nil || final.State != knowledge.StateCompleted || final.Result == nil || final.Result.ExportRef == "" {
		t.Fatalf("completed export: %+v err=%v", final, err)
	}
	// The artifact is readable back from the CAS under its digest.
	got, err := s.ReadObject(ctx, final.Result.ExportRef)
	if err != nil || string(got) != string(artifact) {
		t.Fatalf("artifact readback: %q err=%v", got, err)
	}
	// The failed state on an export carries the export.failed event.
	failed, err := s.FailKnowledgeOperation(ctx, op2.OperationID, op2.CorrelationID, "INTERNAL", "bounded internal reason")
	if err != nil || failed.State != knowledge.StateFailed || failed.FailureCode != "INTERNAL" {
		t.Fatalf("failed export: %+v err=%v", failed, err)
	}
	opRecs := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgeExportOperation, op2.OperationID)
	if len(opRecs) == 0 || opRecs[len(opRecs)-1].EventType != knowledge.EventExportFailed {
		t.Fatalf("the failed export must carry its event: %v", opRecs)
	}
	// A failed IMPORT emits no event — the closed register carries no
	// import.failed type; the state endpoint is the source of truth.
	op3, _, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-f3", "key-f3", knowledgeDigest('f'), knowledge.CreateIntentPack))
	if err != nil {
		t.Fatalf("submit import: %v", err)
	}
	if _, err := s.FailKnowledgeOperation(ctx, op3.OperationID, op3.CorrelationID, "INTERNAL", "bounded internal reason"); err != nil {
		t.Fatalf("fail import: %v", err)
	}
	recs3 := knowledgeOpEvents(t, s, knowledge.AggregateKnowledgeImportOperation, op3.OperationID)
	for _, r := range recs3 {
		if strings.HasSuffix(r.EventType, ".failed") {
			t.Fatalf("no failed event type exists for imports: %v", recs3)
		}
	}
}

// TestKnowledgeListKeyset pins the keyset list semantics: created-at
// descending pages, the More flag driving the cursor, the final page
// marked by the flag's absence.
func TestKnowledgeListKeyset(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		id := string(rune('a' + i))
		op, _, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-l"+id, "key-l"+id, knowledgeDigest(byte('a'+i)), knowledge.CreateIntentPack))
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		if _, err := s.CompleteKnowledgeImport(ctx, knowledgeCommit(op.OperationID, true)); err != nil {
			t.Fatalf("complete %d: %v", i, err)
		}
		time.Sleep(2 * time.Millisecond) // distinct created_at sort keys
	}
	page1, err := s.ListKnowledgePacks(ctx, knowledge.ListQuery{Limit: 2})
	if err != nil || len(page1.Packs) != 2 || !page1.More {
		t.Fatalf("page 1: %+v err=%v", page1, err)
	}
	last := page1.Packs[1]
	page2, err := s.ListKnowledgePacks(ctx, knowledge.ListQuery{Limit: 2, AfterCreatedAt: last.CreatedAt, AfterID: last.PackID})
	if err != nil || len(page2.Packs) != 1 || page2.More {
		t.Fatalf("page 2: %+v err=%v", page2, err)
	}
}

// TestKnowledgeOperationRecordResultShape pins the operation record's
// persisted result decode and the no-fabricated-percentage law.
func TestKnowledgeOperationRecordResultShape(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	op, _, err := s.SubmitKnowledgeImport(ctx, knowledgeIntent("kop-r", "key-r", knowledgeDigest('9'), knowledge.CreateIntentPack))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if _, err := s.CompleteKnowledgeImport(ctx, knowledgeCommit("kop-r", true)); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := s.LoadKnowledgeOperation(ctx, op.OperationID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	b, _ := json.Marshal(got)
	var probe map[string]any
	if err := json.Unmarshal(b, &probe); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{"operation_id", "kind", "state", "correlation_id", "created_at", "updated_at"} {
		if _, ok := probe[key]; !ok {
			t.Fatalf("the operation record must carry %s: %s", key, b)
		}
	}
	// No percentage member exists anywhere in the state record (the
	// no-fabricated-percentage law).
	for key := range probe {
		if strings.Contains(strings.ToLower(key), "percent") || strings.Contains(strings.ToLower(key), "progress") {
			t.Fatalf("no percentage member may exist: %s", key)
		}
	}
}
