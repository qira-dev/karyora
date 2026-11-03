package storage

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/notification"
)

// The notification store's test register (the ADOPTED S6 supply,
// P-S6-01…03, implemented over migration 0010): the migration pin, the
// producer mapping with its same-transaction atomicity and the
// source-event dedup, the grouped list with its collapse/keyset/filters/
// four-way presentation, the unread counts, and the all-or-nothing typed
// mutations with the non-mutation pin (a notification mutation writes
// ONLY notification rows — the underlying Job, approval, audit, and
// security records are byte-identical around every mutation).

// notifSubmitFixture is a submission with its own distinct creation
// instant (the durable record's sort key; an explicitly carried instant is
// preserved — the IMPL-4 repair's rule) and its OWN provider id, so each
// fixture job is claimed deterministically (the FIFO claim order never
// drifts across the test's job set).
func notifSubmitFixture(id, key string, at time.Time) aijobrelay.Job {
	return aijobrelay.Job{
		JobID: id, IdempotencyKey: key, InputDigest: inputDigest("payload " + id),
		State: aijobrelay.JobCreated, Version: 1,
		Requirements:  aijobrelay.Requirements{ProviderID: "prov-" + id},
		CorrelationID: "corr-" + id,
		CreatedAt:     at,
	}
}

// driveJobToNotificationState submits, claims, and commits one Job to the
// requested outcome, returning the final Job. toState must be one of the
// notification-relevant outcomes (succeeded / failed / timed_out /
// waiting-approval).
func driveJobToNotificationState(t *testing.T, s *Store, jobID, key string, at time.Time, toState string) aijobrelay.Job {
	t.Helper()
	ctx := context.Background()
	if _, err := s.SubmitJob(ctx, notifSubmitFixture(jobID, key, at)); err != nil {
		t.Fatalf("submit %s: %v", jobID, err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-notif", ProviderIDs: []string{"prov-" + jobID}, MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim %s: %v %v", jobID, ok, err)
	}
	if claim.Job.JobID != jobID {
		t.Fatalf("claimed %q, want %q (the FIFO order drifted; the fixture needs isolation)", claim.Job.JobID, jobID)
	}
	commit := aijobrelay.Commit{
		JobID: jobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w-notif", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: toState, CorrelationID: "corr-" + jobID,
	}
	switch toState {
	case aijobrelay.JobSucceeded:
		commit.AttemptOutcome = aijobrelay.AttemptSucceeded
		commit.ResultDigest = inputDigest("result " + jobID)
		commit.ReceiptID = "rc-" + jobID
	case aijobrelay.JobFailed, aijobrelay.JobTimedOut:
		commit.AttemptOutcome = aijobrelay.AttemptFailedTerminal
		commit.ReceiptID = "rc-" + jobID
	case aijobrelay.JobWaiting:
		commit.WaitingReason = string(aijobrelay.WaitApproval)
		commit.AttemptOutcome = aijobrelay.AttemptRunning
	default:
		t.Fatalf("unsupported notification-relevant outcome %q", toState)
	}
	if err := s.CommitAttempt(ctx, commit); err != nil {
		t.Fatalf("commit %s → %s: %v", jobID, toState, err)
	}
	job, err := s.LoadJob(ctx, jobID)
	if err != nil {
		t.Fatalf("load %s: %v", jobID, err)
	}
	return job
}

func countNotifications(t *testing.T, s *Store) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM notification_records`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func countRows(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.readers.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestNotificationMigrationApplied pins migration 0010: a fresh apply
// reaches schema version 10 with the notification table and its unique
// source-event index present, and the prior migrations' recorded checksums
// still verify (0007/0008/0009 byte-identical — the checksum authority
// refuses a modified SQL on every open; a recorded migration's SQL is
// never edited).
func TestNotificationMigrationApplied(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	v, err := s.SchemaVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v != schemaVersionMax {
		t.Fatalf("schema version = %d, want %d (migration 0010 applied)", v, schemaVersionMax)
	}
	for _, version := range []int64{7, 8, 9, 10} {
		state, err := s.migrationState(ctx, version)
		if err != nil {
			t.Fatal(err)
		}
		if state != migStateApplied {
			t.Fatalf("migration %d state = %s, want applied", version, state)
		}
	}
	var n int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='notification_records'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("the notification_records table does not exist")
	}
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_notifications_source_event'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("the unique source-event dedup index does not exist")
	}
}

// TestNotificationProducerJobOutcomes pins the Phase 3 producer mapping
// (the adopted S6 supply; the P3.8 §5.1 scope): results (job.succeeded)
// and failures (job.failed, job.timed_out) raise their notification in the
// SAME transaction as the state change and its event — the notification's
// source event is the journal row that raised it. The non-notifying
// register members (the claim path) produce nothing — no every-heartbeat
// notification.
func TestNotificationProducerJobOutcomes(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

	// The claim path (job.claimed) is NOT a producer event: submit + claim
	// and the notification count stays zero.
	if _, err := s.SubmitJob(ctx, notifSubmitFixture("j-n-ctrl", "key-n-ctrl", base)); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-notif", MaxMemoryMB: 1024}, time.Minute); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if got := countNotifications(t, s); got != 0 {
		t.Fatalf("the claim path produced %d notifications, want 0 (no every-heartbeat notification)", got)
	}

	// succeeded → the result class, sourced by its own journal event.
	driveJobToNotificationState(t, s, "j-n-1", "key-n-1", base.Add(time.Minute), aijobrelay.JobSucceeded)
	// failed → the failure class.
	driveJobToNotificationState(t, s, "j-n-2", "key-n-2", base.Add(2*time.Minute), aijobrelay.JobFailed)
	// timed_out → the failure class.
	driveJobToNotificationState(t, s, "j-n-3", "key-n-3", base.Add(3*time.Minute), aijobrelay.JobTimedOut)

	if got := countNotifications(t, s); got != 3 {
		t.Fatalf("notification count = %d, want 3 (succeeded/failed/timed_out)", got)
	}
	rows, err := s.readers.Query(`SELECT notification_id, category, state_class, related_object_type, related_object_id, source_event_id, correlation_id, read_state FROM notification_records ORDER BY created_at ASC, notification_id ASC`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		id, category, class, otype, oid, eventID, corr, read string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.category, &r.class, &r.otype, &r.oid, &r.eventID, &r.corr, &r.read); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantClasses := map[string]string{"j-n-1": string(notification.ClassResult), "j-n-2": string(notification.ClassFailure), "j-n-3": string(notification.ClassFailure)}
	seenObjects := map[string]bool{}
	for _, r := range got {
		if r.category != string(notification.CategoryJob) {
			t.Fatalf("category = %q, want %q (the one coded producer category)", r.category, notification.CategoryJob)
		}
		if wantClasses[r.oid] != r.class {
			t.Fatalf("object %q class = %q, want %q", r.oid, r.class, wantClasses[r.oid])
		}
		if r.otype != notification.ObjectJob {
			t.Fatalf("related object type = %q, want %q", r.otype, notification.ObjectJob)
		}
		if !strings.HasPrefix(r.id, "ntf-") {
			t.Fatalf("the Core issues the stable notification identity, got %q", r.id)
		}
		if r.read != string(notification.ReadUnread) {
			t.Fatalf("a fresh notification is unread, got %q", r.read)
		}
		if r.corr != "corr-"+r.oid {
			t.Fatalf("the correlation id links the notification to its raising event, got %q", r.corr)
		}
		// The source event identity is the journal row that raised it (the
		// same-transaction evidence; the dedup key exists).
		if !strings.HasPrefix(r.eventID, "evt-"+r.oid+"-") {
			t.Fatalf("source event id %q does not name its job %q", r.eventID, r.oid)
		}
		var journalCount int
		if err := s.readers.QueryRow(`SELECT COUNT(*) FROM events_journal WHERE event_id = ?`, r.eventID).Scan(&journalCount); err != nil {
			t.Fatal(err)
		}
		if journalCount != 1 {
			t.Fatalf("the notification's source event %q has no journal row", r.eventID)
		}
		seenObjects[r.oid] = true
	}
	for oid := range wantClasses {
		if !seenObjects[oid] {
			t.Fatalf("no notification for object %q", oid)
		}
	}
}

// TestNotificationProducerApprovalWaiting pins the approvals half of the
// producer scope: a Job durably entering Waiting{reason: approval} raises
// its approval-needed notification in the SAME transaction that mints the
// pending approval row; a transient backoff wait raises NOTHING.
func TestNotificationProducerApprovalWaiting(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)

	// The backoff wait (a transient operational state) notifies nothing.
	if _, err := s.SubmitJob(context.Background(), notifSubmitFixture("j-n-backoff", "key-n-backoff", base)); err != nil {
		t.Fatal(err)
	}
	// Commit straight to Waiting{backoff_scheduled}: queued → waiting is
	// not a legal transition (queued exits: running/cancelled/paused), so
	// drive through the claim first.
	claim, ok, err := s.ClaimNext(context.Background(), aijobrelay.WorkerCapability{WorkerID: "w-notif", ProviderIDs: []string{"prov-j-n-backoff"}, MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	if err := s.CommitAttempt(context.Background(), aijobrelay.Commit{
		JobID: claim.Job.JobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w-notif", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: aijobrelay.JobWaiting, WaitingReason: string(aijobrelay.WaitBackoffScheduled),
		AttemptOutcome: aijobrelay.AttemptRunning,
	}); err != nil {
		t.Fatalf("backoff waiting commit: %v", err)
	}
	if got := countNotifications(t, s); got != 0 {
		t.Fatalf("a backoff wait produced %d notifications, want 0 (transient operational states never notify)", got)
	}

	// The approval wait notifies AND mints its pending approval row in the
	// same transaction.
	driveJobToNotificationState(t, s, "j-n-appr", "key-n-appr", base.Add(time.Minute), aijobrelay.JobWaiting)
	if got := countNotifications(t, s); got != 1 {
		t.Fatalf("notification count = %d, want 1 (the approval wait)", got)
	}
	var class string
	if err := s.readers.QueryRow(`SELECT state_class FROM notification_records WHERE related_object_id = 'j-n-appr'`).Scan(&class); err != nil {
		t.Fatal(err)
	}
	if class != string(notification.ClassApprovalNeeded) {
		t.Fatalf("the approval wait's class = %q, want approval_needed", class)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM approvals WHERE job_id = 'j-n-appr' AND state = 'pending'`); got != 1 {
		t.Fatalf("pending approval rows = %d, want 1 (the same-transaction mint)", got)
	}

	// The user's own decision command is USER-originated, not
	// backend-originated: its outcome event raises NO notification (the
	// approval surfaces show the decision; notifying the actor for the
	// actor's own command would be noise). The recorded producer scope's
	// "backend-originated" rule binds (UI §23.1).
	approval, err := s.LoadLatestApprovalForJob(ctx, "j-n-appr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideApproval(ctx, aijobrelay.ApprovalDecision{
		JobID: "j-n-appr", ApprovalID: approval.ApprovalID, Actor: "owner", Decision: aijobrelay.DecisionReject,
	}); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got := countNotifications(t, s); got != 1 {
		t.Fatalf("the decision command produced %d additional notifications, want 0 (user-originated, not backend-originated)", got-1)
	}
}

// TestNotificationProducerNeedsReconciliation pins the failure-class
// mapping of a reconciliation-required transition: an Unknown side-effect
// observation and a lease expiry over an Unknown effect raise their
// needs_reconciliation notification in the SAME transaction; a plain
// requeue (no Unknown effect) raises nothing.
func TestNotificationProducerNeedsReconciliation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 11, 0, 0, 0, time.UTC)

	// A plain lease expiry requeues: NO notification.
	if _, err := s.SubmitJob(ctx, notifSubmitFixture("j-n-requeue", "key-n-requeue", base)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-notif", MaxMemoryMB: 1024}, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, err := s.ReleaseExpiredLeases(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if got := countNotifications(t, s); got != 0 {
		t.Fatalf("a plain requeue produced %d notifications, want 0", got)
	}

	// An Unknown side-effect observation forces NeedsReconciliation and
	// raises its failure-class notification in the same transaction.
	driveJobToNotificationState(t, s, "j-n-obs", "key-n-obs", base.Add(time.Minute), aijobrelay.JobWaiting)
	// The accept decision returns the Job to Queued (the user's own
	// command — no notification) so the worker can claim it again.
	approval, err := s.LoadLatestApprovalForJob(ctx, "j-n-obs")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DecideApproval(ctx, aijobrelay.ApprovalDecision{
		JobID: "j-n-obs", ApprovalID: approval.ApprovalID, Actor: "owner", Decision: aijobrelay.DecisionAccept,
	}); err != nil {
		t.Fatalf("accept: %v", err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-notif", ProviderIDs: []string{"prov-j-n-obs"}, MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("re-claim the waiting job: %v %v", ok, err)
	}
	intent := aijobrelay.SideEffectIntent{
		IntentID: "int-n-obs", JobID: "j-n-obs", AttemptID: claim.Attempt.AttemptID,
		Operation: "test.op", IdempotencyKey: "idem-n-obs",
	}
	if err := s.PrepareSideEffect(ctx, intent); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	// The intent machine: prepared → dispatched, then dispatched → unknown
	// (the Unknown rule — recorded as Unknown, never as success).
	if err := s.ObserveSideEffect(ctx, "j-n-obs", intent.IntentID, claim.Job.Version, claim.Attempt.LeaseEpoch, claim.Attempt.AttemptID, aijobrelay.SideEffectDispatched, ""); err != nil {
		t.Fatalf("observe dispatched: %v", err)
	}
	if err := s.ObserveSideEffect(ctx, "j-n-obs", intent.IntentID, claim.Job.Version, claim.Attempt.LeaseEpoch, claim.Attempt.AttemptID, aijobrelay.SideEffectUnknown, ""); err != nil {
		t.Fatalf("observe unknown: %v", err)
	}
	if got := countNotifications(t, s); got != 2 {
		t.Fatalf("notification count = %d, want 2 (the approval wait + the needs_reconciliation transition)", got)
	}
	var class, objectState string
	if err := s.readers.QueryRow(`SELECT state_class, (SELECT state FROM jobs WHERE job_id = related_object_id) FROM notification_records WHERE related_object_id = 'j-n-obs' AND state_class = 'failure'`).Scan(&class, &objectState); err != nil {
		t.Fatal(err)
	}
	if class != string(notification.ClassFailure) {
		t.Fatalf("the reconciliation transition's class = %q, want failure", class)
	}
	if objectState != aijobrelay.JobNeedsReconciliation {
		t.Fatalf("the owning job's state = %q, want needs_reconciliation", objectState)
	}
}

// TestNotificationProducerRefusedCommitWritesNothing pins the atomicity:
// the producer rides the SAME transaction as the fenced commit — a refused
// commit (a stale version) leaves NO notification, NO event, and NO state
// change.
func TestNotificationProducerRefusedCommitWritesNothing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	if _, err := s.SubmitJob(ctx, notifSubmitFixture("j-n-stale", "key-n-stale", base)); err != nil {
		t.Fatal(err)
	}
	claim, ok, err := s.ClaimNext(ctx, aijobrelay.WorkerCapability{WorkerID: "w-notif", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	stale := aijobrelay.Commit{
		JobID: "j-n-stale", JobVersion: 1, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w9", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: inputDigest("result"), ReceiptID: "rc-stale",
	}
	if err := s.CommitAttempt(ctx, stale); err == nil {
		t.Fatal("a stale commit was accepted")
	}
	if got := countNotifications(t, s); got != 0 {
		t.Fatalf("a refused commit left %d notifications, want 0 (the same-transaction discipline)", got)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM events_journal WHERE aggregate_id = 'j-n-stale' AND event_type = 'job.succeeded'`); got != 0 {
		t.Fatalf("a refused commit left %d succeeded events, want 0", got)
	}
}

// TestNotificationDedupBySourceEvent pins the dedup law (P-S6-03): dedup
// is claimed ONLY by the stable Core event identity — a re-delivered event
// whose identity already raised its notification inserts nothing, never a
// duplicate.
func TestNotificationDedupBySourceEvent(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 13, 0, 0, 0, time.UTC)
	driveJobToNotificationState(t, s, "j-n-dedup", "key-n-dedup", base, aijobrelay.JobSucceeded)
	if got := countNotifications(t, s); got != 1 {
		t.Fatalf("notification count = %d, want 1", got)
	}
	// A replayed delivery of the SAME event identity inserts nothing (the
	// checked same-transaction ruling in appendNotificationTx).
	err := s.withWriter(ctx, func(tx *sqlTx) error {
		replay := notification.Notification{
			NotificationID:    "ntf-replayed-delivery",
			Category:          notification.CategoryJob,
			StateClass:        notification.ClassResult,
			RelatedObjectType: notification.ObjectJob,
			RelatedObjectID:   "j-n-dedup",
			SourceEventID:     "evt-j-n-dedup-succeeded-2",
			CorrelationID:     "corr-j-n-dedup",
			ReadState:         notification.ReadUnread,
		}
		return appendNotificationTx(tx, replay, time.Now().UTC().Format(time.RFC3339Nano))
	})
	if err != nil {
		t.Fatalf("the replayed delivery must dedup silently, got %v", err)
	}
	if got := countNotifications(t, s); got != 1 {
		t.Fatalf("notification count after the replayed delivery = %d, want 1 (never a duplicate)", got)
	}
	// A DIFFERENT event identity on the same object inserts normally.
	err = s.withWriter(ctx, func(tx *sqlTx) error {
		second := notification.Notification{
			NotificationID:    newNotificationID(),
			Category:          notification.CategoryJob,
			StateClass:        notification.ClassFailure,
			RelatedObjectType: notification.ObjectJob,
			RelatedObjectID:   "j-n-dedup",
			SourceEventID:     "evt-j-n-dedup-failed-9",
			CorrelationID:     "corr-j-n-dedup",
			ReadState:         notification.ReadUnread,
		}
		return appendNotificationTx(tx, second, time.Now().UTC().Format(time.RFC3339Nano))
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := countNotifications(t, s); got != 2 {
		t.Fatalf("notification count = %d, want 2 (a distinct identity inserts)", got)
	}
}

// TestNotificationListGroupsAndCollapse pins the grouped list read: the
// recorded grouping rule's working-candidate mechanics (one entry per
// consecutive run of one related object; repeated identical updates
// collapse into counted blocks; a meaningful state transition is its own
// block; the newest timestamp, the unread count, the current canonical
// state), the four-way presentation (stale computed; permission_revoked
// never emitted in this mode), and the closed-register filters.
func TestNotificationListGroupsAndCollapse(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 14, 0, 0, 0, time.UTC)

	// Job A: a fully backend-originated transition sequence —
	// waiting{approval} (approval_needed) → needs_reconciliation
	// (failure) → failed (failure). Its three notifications form ONE
	// group: the repeated identical failure updates collapse into ONE
	// counted block; the approval_needed block stays visible (a meaningful
	// state transition is never hidden).
	driveJobToNotificationState(t, s, "j-g-a", "key-g-a", base.Add(time.Minute), aijobrelay.JobWaiting)
	time.Sleep(2 * time.Millisecond)
	// waiting → needs_reconciliation (a legal Waiting exit; the attempt is
	// still running).
	jobA, err := s.LoadJob(ctx, "j-g-a")
	if err != nil {
		t.Fatal(err)
	}
	attemptA, err := s.LoadCurrentAttempt(ctx, "j-g-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: "j-g-a", JobVersion: jobA.Version, AttemptID: attemptA.AttemptID,
		WorkerID: attemptA.WorkerID, LeaseID: attemptA.LeaseID, LeaseEpoch: attemptA.LeaseEpoch,
		ToState: aijobrelay.JobNeedsReconciliation, AttemptOutcome: aijobrelay.AttemptRunning,
		CorrelationID: "corr-j-g-a",
	}); err != nil {
		t.Fatalf("needs_reconciliation commit: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	// needs_reconciliation → failed (terminal; the same failure class —
	// the repeated identical update that collapses).
	jobA, err = s.LoadJob(ctx, "j-g-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CommitAttempt(ctx, aijobrelay.Commit{
		JobID: "j-g-a", JobVersion: jobA.Version, AttemptID: attemptA.AttemptID,
		WorkerID: attemptA.WorkerID, LeaseID: attemptA.LeaseID, LeaseEpoch: attemptA.LeaseEpoch,
		ToState: aijobrelay.JobFailed, AttemptOutcome: aijobrelay.AttemptFailedTerminal,
		ResultDigest: inputDigest("result j-g-a"), ReceiptID: "rc-j-g-a",
		CorrelationID: "corr-j-g-a",
	}); err != nil {
		t.Fatalf("failed commit: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	// Job B: succeeded — its own group, driven after job A's sequence so
	// job A's notifications are consecutive in the newest-first order.
	driveJobToNotificationState(t, s, "j-g-b", "key-g-b", base.Add(2*time.Minute), aijobrelay.JobSucceeded)
	time.Sleep(2 * time.Millisecond) // force distinct creation instants between the batches

	page, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("entries = %d, want 2 (job A's run collapses; job B is its own group)", len(page.Entries))
	}
	if page.More || page.AfterCreatedAt != "" || page.AfterNotificationID != "" {
		t.Fatalf("the final page is cursorless, got more=%v after=(%q,%q)", page.More, page.AfterCreatedAt, page.AfterNotificationID)
	}
	// Newest-first: job B (succeeded) then job A (the failed decision).
	if page.Entries[0].RelatedObjectID != "j-g-b" || page.Entries[1].RelatedObjectID != "j-g-a" {
		t.Fatalf("entry order = [%s, %s], want [j-g-b, j-g-a] (newest first)", page.Entries[0].RelatedObjectID, page.Entries[1].RelatedObjectID)
	}
	// Job A's group: 3 notifications, all unread, TWO blocks (the failure
	// pair collapsed into one counted block; the approval_needed block
	// visible), the current canonical state is the job's failed state.
	a := page.Entries[1]
	if a.RepeatCount != 3 || a.UnreadCount != 3 {
		t.Fatalf("job A's group repeat/unread = %d/%d, want 3/3", a.RepeatCount, a.UnreadCount)
	}
	if len(a.Collapsed) != 2 {
		t.Fatalf("job A's collapsed blocks = %d, want 2 (the failure pair collapses; the transition stays visible)", len(a.Collapsed))
	}
	var failureBlock, approvalBlock *notification.CollapsedBlock
	for i := range a.Collapsed {
		switch a.Collapsed[i].StateClass {
		case notification.ClassFailure:
			failureBlock = &a.Collapsed[i]
		case notification.ClassApprovalNeeded:
			approvalBlock = &a.Collapsed[i]
		}
	}
	if failureBlock == nil || approvalBlock == nil {
		t.Fatalf("the meaningful transition is hidden: blocks = %v", a.Collapsed)
	}
	if failureBlock.Count != 2 || failureBlock.UnreadCount != 2 {
		t.Fatalf("the failure block count/unread = %d/%d, want 2/2 (repeated identical updates collapse)", failureBlock.Count, failureBlock.UnreadCount)
	}
	if approvalBlock.Count != 1 {
		t.Fatalf("the approval block count = %d, want 1", approvalBlock.Count)
	}
	if a.ObjectState != aijobrelay.JobFailed {
		t.Fatalf("the current canonical state = %q, want failed", a.ObjectState)
	}
	if a.Presentation != notification.PresentUnread {
		t.Fatalf("presentation = %q, want unread", a.Presentation)
	}
	// Job B's group: one result block, the succeeded canonical state.
	b := page.Entries[0]
	if b.RepeatCount != 1 || b.UnreadCount != 1 || len(b.Collapsed) != 1 || b.Collapsed[0].StateClass != notification.ClassResult {
		t.Fatalf("job B's group is wrong: %+v", b)
	}
	if b.ObjectState != aijobrelay.JobSucceeded || b.Presentation != notification.PresentUnread {
		t.Fatalf("job B's state/presentation = %q/%q, want succeeded/unread", b.ObjectState, b.Presentation)
	}
	for _, e := range page.Entries {
		if e.Presentation == notification.PresentPermissionRevoked {
			t.Fatal("permission_revoked must never be emitted in the single-user Local mode (the computation joins with Team mode, D-36)")
		}
	}

	// The stale computation: a notification whose related object no longer
	// resolves presents stale — no canonical state, no invented detail (the
	// no-leak class).
	err = s.withWriter(ctx, func(tx *sqlTx) error {
		ghost := notification.Notification{
			NotificationID:    newNotificationID(),
			Category:          notification.CategoryJob,
			StateClass:        notification.ClassResult,
			RelatedObjectType: notification.ObjectJob,
			RelatedObjectID:   "job-erased-elsewhere",
			SourceEventID:     "evt-erased-object-1",
			CorrelationID:     "corr-erased",
			ReadState:         notification.ReadUnread,
		}
		return appendNotificationTx(tx, ghost, time.Now().UTC().Format(time.RFC3339Nano))
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err = s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(page.Entries))
	}
	staleEntry := page.Entries[0] // the ghost is the newest insert
	if staleEntry.Presentation != notification.PresentStale {
		t.Fatalf("an unresolvable object's presentation = %q, want stale", staleEntry.Presentation)
	}
	if staleEntry.ObjectState != "" {
		t.Fatalf("a stale entry carries no canonical state, got %q", staleEntry.ObjectState)
	}

	// The filters: closed registers only; the read-state filter narrows.
	if _, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50, Category: "knowledge"}); err == nil {
		t.Fatal("an unregistered category must refuse (filter options come from the Core only)")
	}
	if _, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50, ReadState: "archived"}); err == nil {
		t.Fatal("an unregistered read state must refuse")
	}
	page, err = s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50, ReadState: string(notification.ReadUnread)})
	if err != nil {
		t.Fatal(err)
	}
	totalUnread := 0
	for _, e := range page.Entries {
		totalUnread += e.UnreadCount
	}
	if totalUnread == 0 {
		t.Fatal("the unread filter returned nothing although unread rows exist")
	}
}

// TestNotificationListKeysetPagination pins the card-9 keyset walk over
// the grouped list: a page walk collects every group exactly once (no
// overlap, no gap), the final page is cursorless, and the fail-closed
// guards refuse.
func TestNotificationListKeysetPagination(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)
	const jobs = 6
	for i := 0; i < jobs; i++ {
		id := "j-k-" + string(rune('a'+i))
		driveJobToNotificationState(t, s, id, "key-"+id, base.Add(time.Duration(i)*time.Minute), aijobrelay.JobSucceeded)
	}
	full, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Entries) != jobs {
		t.Fatalf("entries = %d, want %d", len(full.Entries), jobs)
	}
	// The size-2 walk: no overlap, no gap, the cursorless final page.
	var walked []string
	cursor := ""
	pages := 0
	for {
		q := notification.ListNotificationsQuery{Limit: 2, AfterCreatedAt: "", AfterNotificationID: ""}
		if cursor != "" {
			pos, derr := decodeNotificationListCursorForTest(cursor)
			if derr != nil {
				t.Fatalf("cursor decode: %v", derr)
			}
			q.AfterCreatedAt, q.AfterNotificationID = pos[0], pos[1]
		}
		page, err := s.ListNotificationGroups(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, e := range page.Entries {
			walked = append(walked, e.RelatedObjectID)
		}
		if !page.More {
			if page.AfterCreatedAt != "" || page.AfterNotificationID != "" {
				t.Fatal("a final page must not carry a cursor position")
			}
			break
		}
		if page.AfterCreatedAt == "" || page.AfterNotificationID == "" {
			t.Fatal("a continued page must carry its keyset position")
		}
		cursor = page.AfterCreatedAt + "\x00" + page.AfterNotificationID
		if pages > 10 {
			t.Fatal("the walk did not terminate")
		}
	}
	if len(walked) != jobs {
		t.Fatalf("the size-2 walk collected %d groups, want %d", len(walked), jobs)
	}
	seen := map[string]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Fatalf("group %q walked twice (overlap)", id)
		}
		seen[id] = true
	}
	if len(seen) != jobs {
		t.Fatalf("the walk missed groups (gap): %v", walked)
	}
	// The fail-closed guards.
	if _, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 0}); err == nil {
		t.Fatal("a non-positive limit must refuse")
	}
	if _, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 5, AfterCreatedAt: "2026-11-03T00:00:00Z"}); err == nil {
		t.Fatal("a keyset position without its tiebreak must refuse")
	}
	if _, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 5, AfterNotificationID: "ntf-x"}); err == nil {
		t.Fatal("a keyset position without its created_at must refuse")
	}
}

// decodeNotificationListCursorForTest splits the test walk's composite
// cursor back into its keyset position.
func decodeNotificationListCursorForTest(cursor string) ([2]string, error) {
	parts := strings.SplitN(cursor, "\x00", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return [2]string{}, errStoragef(errs.CodeValidationFailed, "malformed test cursor")
	}
	return [2]string{parts[0], parts[1]}, nil
}

// TestNotificationUnreadCounts pins the Core-supplied unread counts: from
// the stored read state only, cleared rows excluded, updated by the
// mutations.
func TestNotificationUnreadCounts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 16, 0, 0, 0, time.UTC)
	driveJobToNotificationState(t, s, "j-u-1", "key-u-1", base.Add(time.Minute), aijobrelay.JobSucceeded)
	driveJobToNotificationState(t, s, "j-u-2", "key-u-2", base.Add(2*time.Minute), aijobrelay.JobFailed)

	counts, err := s.UnreadNotificationCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 || counts.ByCategory[string(notification.CategoryJob)] != 2 {
		t.Fatalf("unread counts = %+v, want total 2 / job 2", counts)
	}
	// Read one; the count drops to 1 (Core-supplied state only — the count
	// follows the stored read state, never a client-side count).
	var n1 string
	if err := s.readers.QueryRow(`SELECT notification_id FROM notification_records WHERE related_object_id = 'j-u-1'`).Scan(&n1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkNotificationsRead(ctx, []string{n1}); err != nil {
		t.Fatal(err)
	}
	counts, err = s.UnreadNotificationCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 || counts.ByCategory[string(notification.CategoryJob)] != 1 {
		t.Fatalf("unread counts after the mark-read = %+v, want total 1 / job 1", counts)
	}
}

// TestNotificationMutations pins the O-6 mutation set at the store:
// all-or-nothing refusals (unknown id → NOT_FOUND, nothing written; a
// cleared id → CONFLICT_STATE), the convergent no-op, the read_at
// lifecycle, and the NON-MUTATION LAW: every mutation writes ONLY
// notification rows — the jobs, approvals, audit_records, and
// security-relevant tables are byte-identical around every mutation.
func TestNotificationMutations(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)
	driveJobToNotificationState(t, s, "j-m-1", "key-m-1", base.Add(time.Minute), aijobrelay.JobSucceeded)
	var n1 string
	if err := s.readers.QueryRow(`SELECT notification_id FROM notification_records WHERE related_object_id = 'j-m-1'`).Scan(&n1); err != nil {
		t.Fatal(err)
	}

	snapshotUnderlying := func() (job aijobrelay.Job, jobsN, approvalsN, auditN, journalN int) {
		t.Helper()
		job, err := s.LoadJob(ctx, "j-m-1")
		if err != nil {
			t.Fatal(err)
		}
		jobsN = countRows(t, s, `SELECT COUNT(*) FROM jobs`)
		approvalsN = countRows(t, s, `SELECT COUNT(*) FROM approvals`)
		auditN = countRows(t, s, `SELECT COUNT(*) FROM audit_records`)
		journalN = countRows(t, s, `SELECT COUNT(*) FROM events_journal`)
		return job, jobsN, approvalsN, auditN, journalN
	}
	jobBefore, jobsBefore, approvalsBefore, auditBefore, journalBefore := snapshotUnderlying()

	// All-or-nothing: an unknown id refuses NOT_FOUND and NOTHING changes.
	if _, err := s.MarkNotificationsRead(ctx, []string{"ntf-does-not-exist", n1}); err == nil {
		t.Fatal("an unknown id must refuse")
	} else if env, ok := err.(*Error); !ok || env.Code != errs.CodeNotFound {
		t.Fatalf("the unknown-id refusal must be typed NOT_FOUND, got %v", err)
	}
	if got := countRows(t, s, `SELECT COUNT(*) FROM notification_records WHERE read_state = 'read'`); got != 0 {
		t.Fatalf("the all-or-nothing refusal wrote %d read rows, want 0", got)
	}

	// Mark read: the typed outcome, the read_at stamp.
	outcome, err := s.MarkNotificationsRead(ctx, []string{n1})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Updated != 1 || outcome.Unchanged != 0 {
		t.Fatalf("mark-read outcome = %+v, want updated 1", outcome)
	}
	var readState, readAt string
	if err := s.readers.QueryRow(`SELECT read_state, read_at FROM notification_records WHERE notification_id = ?`, n1).Scan(&readState, &readAt); err != nil {
		t.Fatal(err)
	}
	if readState != string(notification.ReadRead) || readAt == "" {
		t.Fatalf("read state/stamp = %q/%q, want read/<stamped>", readState, readAt)
	}

	// Convergent: a repeat is a no-op that neither creates nor destroys.
	outcome, err = s.MarkNotificationsRead(ctx, []string{n1})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Updated != 0 || outcome.Unchanged != 1 {
		t.Fatalf("the convergent repeat = %+v, want unchanged 1", outcome)
	}
	if got := countNotifications(t, s); got != 1 {
		t.Fatalf("the repeat changed the record count to %d, want 1 (convergent, no duplicate)", got)
	}

	// Mark unread: the read_at stamp clears.
	outcome, err = s.MarkNotificationsUnread(ctx, []string{n1})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Updated != 1 {
		t.Fatalf("mark-unread outcome = %+v, want updated 1", outcome)
	}
	if err := s.readers.QueryRow(`SELECT read_state, read_at FROM notification_records WHERE notification_id = ?`, n1).Scan(&readState, &readAt); err != nil {
		t.Fatal(err)
	}
	if readState != string(notification.ReadUnread) || readAt != "" {
		t.Fatalf("unread state/stamp = %q/%q, want unread/<cleared>", readState, readAt)
	}

	// Clear: the soft-hide (retained, excluded from the list and counts).
	outcome, err = s.ClearNotifications(ctx, []string{n1})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Updated != 1 {
		t.Fatalf("clear outcome = %+v, want updated 1", outcome)
	}
	counts, err := s.UnreadNotificationCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 0 {
		t.Fatalf("the cleared row still counts: %+v", counts)
	}
	page, err := s.ListNotificationGroups(ctx, notification.ListNotificationsQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 0 {
		t.Fatalf("the cleared row still lists: %d entries", len(page.Entries))
	}
	var retained int
	if err := s.readers.QueryRow(`SELECT COUNT(*) FROM notification_records WHERE notification_id = ?`, n1).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatal("the cleared row must be RETAINED (the soft-hide working candidate; no retention value implemented)")
	}

	// A cleared id refuses the read-state mutation (never resurrected).
	if _, err := s.MarkNotificationsRead(ctx, []string{n1}); err == nil {
		t.Fatal("a cleared id must refuse the read-state mutation")
	} else if env, ok := err.(*Error); !ok || env.Code != errs.CodeConflictState {
		t.Fatalf("the cleared-id refusal must be typed CONFLICT_STATE, got %v", err)
	}

	// THE NON-MUTATION LAW: the underlying records are byte-identical
	// around every mutation above.
	jobAfter, jobsAfter, approvalsAfter, auditAfter, journalAfter := snapshotUnderlying()
	if jobAfter != jobBefore {
		t.Fatalf("the underlying Job changed across notification mutations:\n before %+v\n after  %+v", jobBefore, jobAfter)
	}
	if jobsAfter != jobsBefore || approvalsAfter != approvalsBefore || auditAfter != auditBefore || journalAfter != journalBefore {
		t.Fatalf("the underlying record counts changed: jobs %d→%d, approvals %d→%d, audit %d→%d, journal %d→%d",
			jobsBefore, jobsAfter, approvalsBefore, approvalsAfter, auditBefore, auditAfter, journalBefore, journalAfter)
	}
}
