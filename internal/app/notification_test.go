package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/domain/notification"
	"github.com/qira-dev/karyora/internal/security"
)

// The notification surface's app-level test register (the ADOPTED S6
// supply, P-S6-01/02, implemented as working candidates): the card-9 list
// operation with its bound class and cursor protocol, the Core-supplied
// unread counts, the NON-MUTATION LAW (a notification read/dismiss NEVER
// changes the underlying Job, approval, audit, or security record — the
// IMPL-7 requirement 6 mirrored; pinned byte-for-byte), no auto-retry on
// mutations, the all-or-nothing typed refusals, and the authorization
// join.

// notifEnv extends the shared test env with the notification surface wired
// (the real storage adapter over migration 0010 — the real composition)
// and a RETAINED audit sink so the non-mutation pin can read the audit
// chain. The Guard is a pointer: the swapped Audit service is the same one
// the slice authorizes through (the conversation fixture's own pattern).
func notifEnv(t *testing.T) (*testEnv, *memoryAuditStore) {
	t.Helper()
	env := newTestEnv(t)
	sink := &memoryAuditStore{}
	env.deps.Guard.Audit = security.NewAuditService(sink, 64, 1000)
	env.deps.Notifications = env.store
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatalf("slice: %v", err)
	}
	env.slice = slice
	return env, sink
}

// driveNotifJob submits and commits one Job to the requested
// notification-relevant outcome through the real composition (the slice
// for the submit; the relay port for the claim/commit — the same path the
// durable worker takes).
func driveNotifJob(t *testing.T, env *testEnv, idempotencyKey, toState string) aijobrelay.Job {
	t.Helper()
	ctx := env.ctx()
	sub, err := env.slice.Submit(ctx, SubmitRequest{
		IdempotencyKey: idempotencyKey, Task: "notification fixture task " + idempotencyKey,
		Provider: "mock", Model: "m",
	})
	if err != nil {
		t.Fatalf("submit %s: %v", idempotencyKey, err)
	}
	claim, ok, err := env.deps.Relay.ClaimNext(context.Background(),
		aijobrelay.WorkerCapability{WorkerID: "w-app-notif", ProviderIDs: []string{"mock"}, MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim %s: %v %v", idempotencyKey, ok, err)
	}
	if claim.Job.JobID != sub.Job.JobID {
		t.Fatalf("claimed %q, want %q", claim.Job.JobID, sub.Job.JobID)
	}
	commit := aijobrelay.Commit{
		JobID: sub.Job.JobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: "w-app-notif", LeaseID: claim.Attempt.LeaseID, LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState: toState, CorrelationID: sub.Job.CorrelationID,
	}
	switch toState {
	case aijobrelay.JobSucceeded:
		commit.AttemptOutcome = aijobrelay.AttemptSucceeded
		commit.ResultDigest = "sha256:" + strings.Repeat("a", 64)
		commit.ReceiptID = "rc-" + sub.Job.JobID
	case aijobrelay.JobWaiting:
		commit.WaitingReason = string(aijobrelay.WaitApproval)
		commit.AttemptOutcome = aijobrelay.AttemptRunning
	default:
		t.Fatalf("unsupported notification-relevant outcome %q", toState)
	}
	if err := env.deps.Relay.CommitAttempt(context.Background(), commit); err != nil {
		t.Fatalf("commit %s → %s: %v", sub.Job.JobID, toState, err)
	}
	job, err := env.deps.Relay.LoadJob(context.Background(), sub.Job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

// notificationIDFor reads the group entry's representative id for one
// related object through the list port (no raw SQL — the surface's own
// read).
func notificationIDFor(t *testing.T, env *testEnv, objectID string) string {
	t.Helper()
	res, err := env.slice.ListNotifications(env.ctx(), ListNotificationsRequest{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Entries {
		if e.RelatedObjectID == objectID {
			return e.NotificationID
		}
	}
	t.Fatalf("no notification group for object %q", objectID)
	return ""
}

func mustAuditRecords(t *testing.T, sink *memoryAuditStore) []security.ChainedRecord {
	t.Helper()
	records, err := sink.Records()
	if err != nil {
		t.Fatalf("audit records: %v", err)
	}
	return records
}

// TestNotificationListOperation pins the card-9 list operation: the
// per-surface default, the 1–1000 bound class (out-of-range typed
// refusals; 1000 accepted), the opaque cursor round-trip (a page walk with
// no overlap or gap; the cursorless final page), and the cursor never
// carrying structured JSON on the wire.
func TestNotificationListOperation(t *testing.T) {
	env, _ := notifEnv(t)
	ctx := env.ctx()
	const jobs = 5
	for i := 0; i < jobs; i++ {
		driveNotifJob(t, env, "notif-key-"+string(rune('a'+i)), aijobrelay.JobSucceeded)
		time.Sleep(2 * time.Millisecond)
	}
	// The full page: every group exactly once, cursorless (final page).
	res, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != jobs || res.NextCursor != "" {
		t.Fatalf("entries = %d cursor = %q, want %d entries and a cursorless final page", len(res.Entries), res.NextCursor, jobs)
	}
	// The size-2 walk: no overlap, no gap.
	var walked []string
	cursor := ""
	for {
		page, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range page.Entries {
			walked = append(walked, e.RelatedObjectID)
		}
		if page.NextCursor == "" {
			break
		}
		if strings.ContainsAny(page.NextCursor, "{}") {
			t.Fatalf("the cursor must never carry structured JSON on the wire: %q", page.NextCursor)
		}
		cursor = page.NextCursor
		if len(walked) > 50 {
			t.Fatal("the walk did not terminate")
		}
	}
	if len(walked) != jobs {
		t.Fatalf("the size-2 walk collected %d entries, want %d", len(walked), jobs)
	}
	seen := map[string]bool{}
	for _, id := range walked {
		if seen[id] {
			t.Fatalf("entry %q walked twice (overlap)", id)
		}
		seen[id] = true
	}

	// The bound class: the typed refusals and the accepted edge.
	for _, bad := range []int{-1, 1001} {
		if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Limit: bad}); err == nil {
			t.Fatalf("limit %d must refuse", bad)
		} else if codeOf(t, err) != errs.CodeValidationFailed {
			t.Fatalf("limit %d refusal must be VALIDATION_FAILED, got %v", bad, err)
		}
	}
	if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Limit: 1000}); err != nil {
		t.Fatalf("limit 1000 must be accepted, got %v", err)
	}
	// The closed-register filters refuse unknown values.
	if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Category: "knowledge"}); err == nil {
		t.Fatal("an unregistered category must refuse")
	}
	if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{ReadState: "seen"}); err == nil {
		t.Fatal("an unregistered read state must refuse")
	}
	// A malformed cursor refuses typed.
	if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{Cursor: "garbage-cursor"}); err == nil {
		t.Fatal("a malformed cursor must refuse")
	} else if codeOf(t, err) != errs.CodeValidationFailed {
		t.Fatalf("the cursor refusal must be VALIDATION_FAILED, got %v", err)
	}
	// The four-way payload fields exist on every entry (non-color; the UI's
	// rendering duty), and the current canonical state rides the group.
	for _, e := range res.Entries {
		if !notification.ValidPresentationState(e.Presentation) {
			t.Fatalf("entry %q carries an unregistered presentation state", e.NotificationID)
		}
		if e.Presentation == notification.PresentPermissionRevoked {
			t.Fatal("permission_revoked must never be emitted in the single-user Local mode")
		}
		if e.ObjectState != aijobrelay.JobSucceeded {
			t.Fatalf("the current canonical state = %q, want succeeded", e.ObjectState)
		}
	}
}

// TestNotificationCountsAndNilStore pins the Core-supplied counts read and
// the nil-store fail-closed posture.
func TestNotificationCountsAndNilStore(t *testing.T) {
	env, _ := notifEnv(t)
	ctx := env.ctx()
	driveNotifJob(t, env, "counts-key-1", aijobrelay.JobSucceeded)
	driveNotifJob(t, env, "counts-key-2", aijobrelay.JobWaiting)
	counts, err := env.slice.NotificationUnreadCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 2 || counts.ByCategory["job"] != 2 {
		t.Fatalf("unread counts = %+v, want total 2 / job 2 (Core-supplied state only)", counts)
	}

	// The nil-store posture: typed STORAGE_UNAVAILABLE refusals, fail
	// closed (the Connections pattern).
	empty := newTestEnv(t)
	if _, err := empty.slice.ListNotifications(empty.ctx(), ListNotificationsRequest{}); err == nil {
		t.Fatal("the nil notification store must refuse the list")
	} else if codeOf(t, err) != errs.CodeStorageUnavailable {
		t.Fatalf("the nil-store refusal must be STORAGE_UNAVAILABLE, got %v", err)
	}
	if _, err := empty.slice.NotificationUnreadCounts(empty.ctx()); err == nil {
		t.Fatal("the nil notification store must refuse the counts")
	}
	if _, err := empty.slice.MarkNotificationsRead(empty.ctx(), NotificationMutationRequest{NotificationIDs: []string{"ntf-x"}}); err == nil {
		t.Fatal("the nil notification store must refuse the mutation")
	}
}

// TestNotificationMutationsNeverTouchUnderlyingRecords pins THE
// NON-MUTATION LAW (P-S6-02; P3.8 §9; the IMPL-7 requirement 6 mirrored):
// a notification read/dismiss NEVER changes the underlying Job, approval,
// audit, or security record — the Job records are byte-identical around
// every mutation, the pending approval stays pending (a read approval
// notification is NOT an approved approval), and the only audit records
// added are the mutations' own decision records (scoped to the
// notification resource).
func TestNotificationMutationsNeverTouchUnderlyingRecords(t *testing.T) {
	env, sink := notifEnv(t)
	ctx := env.ctx()
	// A succeeded Job (one notification) and a waiting Job (one pending
	// approval + one notification) — two different underlying domains the
	// mutations must never touch.
	succeeded := driveNotifJob(t, env, "nonmut-key-1", aijobrelay.JobSucceeded)
	waiting := driveNotifJob(t, env, "nonmut-key-2", aijobrelay.JobWaiting)
	nSucceeded := notificationIDFor(t, env, succeeded.JobID)
	nWaiting := notificationIDFor(t, env, waiting.JobID)

	jobsBefore, err := env.deps.Relay.ListJobs(context.Background(), aijobrelay.ListJobsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	approvalBefore, err := env.deps.Relay.LoadLatestApprovalForJob(context.Background(), waiting.JobID)
	if err != nil {
		t.Fatal(err)
	}
	auditBefore := len(mustAuditRecords(t, sink))

	// Every mutation in the O-6 set, over both notifications.
	if _, err := env.slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{nSucceeded, nWaiting}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.slice.MarkNotificationsUnread(ctx, NotificationMutationRequest{NotificationIDs: []string{nWaiting}}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.slice.ClearNotifications(ctx, NotificationMutationRequest{NotificationIDs: []string{nSucceeded}}); err != nil {
		t.Fatal(err)
	}

	// The underlying Job records: byte-identical (every field).
	jobsAfter, err := env.deps.Relay.ListJobs(context.Background(), aijobrelay.ListJobsQuery{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsAfter.Jobs) != len(jobsBefore.Jobs) {
		t.Fatalf("the Job count changed across notification mutations: %d → %d", len(jobsBefore.Jobs), len(jobsAfter.Jobs))
	}
	for i := range jobsBefore.Jobs {
		if jobsAfter.Jobs[i] != jobsBefore.Jobs[i] {
			t.Fatalf("the Job %q changed across notification mutations:\n before %+v\n after  %+v",
				jobsBefore.Jobs[i].JobID, jobsBefore.Jobs[i], jobsAfter.Jobs[i])
		}
	}
	// The approval record: untouched — still pending, still undecided (a
	// read approval notification is NOT an approved approval; approval
	// happens ONLY through the explicit decision command).
	approvalAfter, err := env.deps.Relay.LoadLatestApprovalForJob(context.Background(), waiting.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if approvalAfter != approvalBefore {
		t.Fatalf("the approval record changed across notification mutations:\n before %+v\n after  %+v", approvalBefore, approvalAfter)
	}
	if approvalAfter.State != aijobrelay.ApprovalPending {
		t.Fatalf("the pending approval's state = %q, want pending (notification-read ≠ approval)", approvalAfter.State)
	}
	// The audit chain: the delta is EXACTLY the mutations' own decision
	// records — every new record scoped to the notification resource,
	// nothing else written (C2-15's own decisions; no underlying-domain
	// audit row moved).
	recordsAfter := mustAuditRecords(t, sink)
	if len(recordsAfter)-auditBefore != 3 {
		t.Fatalf("the audit delta = %d, want 3 (the three mutations' own decisions)", len(recordsAfter)-auditBefore)
	}
	for _, rec := range recordsAfter[auditBefore:] {
		var ar security.AuditRecord
		if err := json.Unmarshal(rec.Payload, &ar); err != nil {
			t.Fatal(err)
		}
		if ar.Scope != "notification" {
			t.Fatalf("the mutation's audit scope = %q, want notification (no underlying domain touched)", ar.Scope)
		}
		if ar.Action != string(security.ActionDataWrite) {
			t.Fatalf("the mutation's audit action = %q, want data.write", ar.Action)
		}
	}
}

// TestNotificationMutationNoAutoRetry pins the no-auto-retry law (P-S6-02;
// the idempotency class recorded honestly): a scripted store failure makes
// EXACTLY ONE store call, leaves nothing changed, and the caller's
// EXPLICIT retry (its own decision, not an automatic one) applies the
// mutation. The mutation is state-convergent — a repeat neither creates
// nor destroys anything — but NO idempotency-key or replay semantics are
// claimed (no C1-07 row).
func TestNotificationMutationNoAutoRetry(t *testing.T) {
	env, _ := notifEnv(t)
	ctx := env.ctx()
	job := driveNotifJob(t, env, "retry-key-1", aijobrelay.JobSucceeded)
	n1 := notificationIDFor(t, env, job.JobID)

	failing := &countingFailingNotificationStore{inner: env.store, failReads: 1}
	env.deps.Notifications = failing
	slice, err := NewSlice(env.deps)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{n1}}); err == nil {
		t.Fatal("the scripted failure must surface")
	}
	if got := failing.readCalls.Load(); got != 1 {
		t.Fatalf("the failed mutation made %d store calls, want EXACTLY 1 (no auto-retry)", got)
	}
	// The failed attempt left nothing changed.
	counts, err := env.store.UnreadNotificationCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("the failed mutation changed the unread state: %+v", counts)
	}
	// The caller's EXPLICIT retry applies it (nothing was committed by the
	// failed attempt — no duplicate to dedup, no hidden state).
	res, err := slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{n1}})
	if err != nil {
		t.Fatalf("the explicit retry: %v", err)
	}
	if res.Updated != 1 {
		t.Fatalf("the retry outcome = %+v, want updated 1", res.MutationOutcome)
	}
	if got := failing.readCalls.Load(); got != 2 {
		t.Fatalf("the retry made %d total calls, want exactly 2 (one more)", got)
	}
	// A repeat converges: no duplicate, no destroy.
	res, err = slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{n1}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 0 || res.Unchanged != 1 {
		t.Fatalf("the convergent repeat = %+v, want unchanged 1", res.MutationOutcome)
	}
	counts, err = env.store.UnreadNotificationCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 0 {
		t.Fatalf("the unread state after the convergent repeat = %+v, want 0", counts)
	}
}

// countingFailingNotificationStore wraps the real store and fails the
// first N read mutations, counting every call (the scripted-failure
// fixture for the no-auto-retry pin).
type countingFailingNotificationStore struct {
	inner     NotificationStore
	mu        sync.Mutex
	failReads int
	readCalls atomicCounter
}

type atomicCounter struct {
	mu sync.Mutex
	v  int
}

func (a *atomicCounter) Load() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

func (a *atomicCounter) Inc() {
	a.mu.Lock()
	a.v++
	a.mu.Unlock()
}

func (f *countingFailingNotificationStore) ListNotificationGroups(ctx context.Context, q notification.ListNotificationsQuery) (notification.GroupedListPage, error) {
	return f.inner.ListNotificationGroups(ctx, q)
}

func (f *countingFailingNotificationStore) UnreadNotificationCounts(ctx context.Context) (notification.UnreadCounts, error) {
	return f.inner.UnreadNotificationCounts(ctx)
}

func (f *countingFailingNotificationStore) MarkNotificationsRead(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	f.readCalls.Inc()
	f.mu.Lock()
	fail := f.failReads > 0
	if fail {
		f.failReads--
	}
	f.mu.Unlock()
	if fail {
		return notification.MutationOutcome{}, errors.New("scripted notification store outage")
	}
	return f.inner.MarkNotificationsRead(ctx, ids)
}

func (f *countingFailingNotificationStore) MarkNotificationsUnread(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	return f.inner.MarkNotificationsUnread(ctx, ids)
}

func (f *countingFailingNotificationStore) ClearNotifications(ctx context.Context, ids []string) (notification.MutationOutcome, error) {
	return f.inner.ClearNotifications(ctx, ids)
}

// TestNotificationValidationAndRefusals pins the mutation frame's shape
// law and the all-or-nothing refusal class: an empty or over-bound id set
// refuses typed; an unknown id refuses NOT_FOUND with NOTHING written (a
// mixed set never partially applies); a cleared id refuses CONFLICT_STATE.
func TestNotificationValidationAndRefusals(t *testing.T) {
	env, _ := notifEnv(t)
	ctx := env.ctx()
	job := driveNotifJob(t, env, "valid-key-1", aijobrelay.JobSucceeded)
	n1 := notificationIDFor(t, env, job.JobID)

	// The empty set and the over-bound set refuse typed.
	if _, err := env.slice.MarkNotificationsRead(ctx, NotificationMutationRequest{}); err == nil {
		t.Fatal("an empty id set must refuse")
	} else if codeOf(t, err) != errs.CodeValidationFailed {
		t.Fatalf("the empty-set refusal must be VALIDATION_FAILED, got %v", err)
	}
	overBound := make([]string, MaxNotificationMutationIDs+1)
	for i := range overBound {
		overBound[i] = "ntf-x"
	}
	if _, err := env.slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: overBound}); err == nil {
		t.Fatal("an over-bound id set must refuse")
	}

	// All-or-nothing: an unknown id in a mixed set refuses NOT_FOUND and
	// the known id stays unread.
	if _, err := env.slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{"ntf-does-not-exist", n1}}); err == nil {
		t.Fatal("an unknown id must refuse")
	} else if codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("the unknown-id refusal must be NOT_FOUND, got %v", err)
	}
	counts, err := env.slice.NotificationUnreadCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.Total != 1 {
		t.Fatalf("the refused mutation partially applied: %+v", counts)
	}

	// The unauthenticated caller denies typed (the ONE decision port).
	if _, err := env.slice.MarkNotificationsRead(context.Background(), NotificationMutationRequest{NotificationIDs: []string{n1}}); err == nil {
		t.Fatal("an unauthenticated mutation must deny")
	} else if codeOf(t, err) != errs.CodeAuthzDenied {
		t.Fatalf("the unauthenticated refusal must be AUTHZ_DENIED, got %v", err)
	}
	// The clear path: the same all-or-nothing law.
	if _, err := env.slice.ClearNotifications(ctx, NotificationMutationRequest{NotificationIDs: []string{"ntf-does-not-exist"}}); err == nil {
		t.Fatal("an unknown id must refuse the clear")
	}
	if _, err := env.slice.ClearNotifications(ctx, NotificationMutationRequest{NotificationIDs: []string{n1}}); err != nil {
		t.Fatal(err)
	}
	// The cleared id refuses the read-state mutation (never resurrected).
	if _, err := env.slice.MarkNotificationsRead(ctx, NotificationMutationRequest{NotificationIDs: []string{n1}}); err == nil {
		t.Fatal("a cleared id must refuse the read-state mutation")
	} else if codeOf(t, err) != errs.CodeConflictState {
		t.Fatalf("the cleared-id refusal must be CONFLICT_STATE, got %v", err)
	}
}

// TestNotificationAuthorizationJoin pins the additive register join (the
// recorded register-extension class): every operation denies the
// unauthenticated caller; the audit record carries the notification scope;
// the fail-closed default-deny machinery is UNCHANGED — an unknown
// resource type still denies with RESOURCE_OUT_OF_SCOPE.
func TestNotificationAuthorizationJoin(t *testing.T) {
	env, sink := notifEnv(t)
	ctx := env.ctx()
	driveNotifJob(t, env, "authz-key-1", aijobrelay.JobSucceeded)

	// The list read denies without a valid session credential.
	if _, err := env.slice.ListNotifications(context.Background(), ListNotificationsRequest{}); err == nil {
		t.Fatal("an unauthenticated list must deny")
	} else if codeOf(t, err) != errs.CodeAuthzDenied {
		t.Fatalf("the unauthenticated list refusal must be AUTHZ_DENIED, got %v", err)
	}
	// The authenticated read carries its own audited decision scoped to the
	// notification resource.
	if _, err := env.slice.ListNotifications(ctx, ListNotificationsRequest{}); err != nil {
		t.Fatal(err)
	}
	records := mustAuditRecords(t, sink)
	var last security.AuditRecord
	if err := json.Unmarshal(records[len(records)-1].Payload, &last); err != nil {
		t.Fatal(err)
	}
	if last.Scope != "notification" || last.Action != string(security.ActionDataRead) {
		t.Fatalf("the list read's audit record = scope %q action %q, want notification/data.read", last.Scope, last.Action)
	}

	// The default-deny machinery UNCHANGED: an unknown resource type still
	// denies RESOURCE_OUT_OF_SCOPE exactly as before (the additive join
	// changed no existing row or predicate).
	p := security.Principal{Kind: security.PrincipalLocalUser, ID: "local"}
	pctx := security.PolicyContext{CorrelationID: "corr-default-deny", Credential: env.token}
	decision, _ := env.deps.Guard.Authorize(p, security.Resource{Type: "unregistered-resource"}, security.ActionDataRead, pctx)
	if decision.Allow || decision.ReasonCode != security.ReasonResourceOutOfScope {
		t.Fatalf("the default-deny machinery changed: %+v", decision)
	}
}
