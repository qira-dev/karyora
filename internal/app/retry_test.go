package app

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// The retry-eligibility read's app-level pins (the F9 family's frozen
// contract): the four typed members exactly, the decision-port
// authorization, and the typed NOT_FOUND for an unknown identity (the
// no-leak rule).

func TestRetryEligibilityRead(t *testing.T) {
	env := newTestEnv(t)

	// A queued Job is non-terminal: the verdict denies (a retry follows
	// outcome reconciliation; the outcome has not been reached).
	queued := env.submit(t, baseRequest())
	res, err := env.slice.RetryEligibility(env.ctx(), queued.Job.JobID)
	if err != nil {
		t.Fatalf("queued read: %v", err)
	}
	if res.Eligible {
		t.Fatalf("a queued job must not be retry-eligible: %+v", res)
	}
	if res.ReconciliationRequired {
		t.Fatalf("a fresh queued job reports no reconciliation requirement: %+v", res)
	}
	if res.IdempotencyClass != aijobrelay.RetryIdempotencySuppliedKeyed {
		t.Fatalf("the class must report the implemented Supplied-keyed keying: %+v", res)
	}
	if !aijobrelay.ValidRetryIdempotencyClass(res.IdempotencyClass) {
		t.Fatalf("the class must be a register member: %+v", res)
	}
	if !strings.Contains(res.RiskStatement, "at-least-once attempts") ||
		!strings.Contains(res.RiskStatement, "needs reconciliation") {
		t.Fatalf("the exact terms must ride the statement: %q", res.RiskStatement)
	}

	// A terminal failure is the canonical retry case: the verdict allows
	// (the schema-invalid executor fails the job terminally exactly as
	// production does — no false success, no blind retry).
	invalid := providers.NewMockAdapter(providers.MockBehavior{InvalidStructuredJSON: true})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(invalid.SeedDataHandling(invalid.DefaultProfile("m")))
	exec, err := providers.NewExecutor(invalid, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Providers[dprovider.ProviderMock] = exec
	req := baseRequest()
	req.IdempotencyKey = "idem-retry-eligibility"
	req.Schema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`)
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-retry-eligibility", 1, sub.Job.JobID, 16)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobFailed {
		t.Fatalf("the fixture job must settle Failed, got %s", final.State)
	}
	failed, err := env.slice.RetryEligibility(env.ctx(), sub.Job.JobID)
	if err != nil {
		t.Fatalf("failed read: %v", err)
	}
	if !failed.Eligible {
		t.Fatalf("a terminal Failed job is the canonical retry case: %+v", failed)
	}
	if failed.ReconciliationRequired {
		t.Fatalf("a terminal Failed job with no Unknown side effect reports none: %+v", failed)
	}
	if failed.RiskStatement != res.RiskStatement {
		t.Fatalf("the risk statement is the Core's own statement, state-independent: %q vs %q", failed.RiskStatement, res.RiskStatement)
	}
	if strings.Contains(strings.ToLower(failed.RiskStatement), "exactly-once") {
		t.Fatalf("no exactly-once wording anywhere: %q", failed.RiskStatement)
	}

	// An unknown identity is the typed NOT_FOUND — never a distinguishing
	// answer (the no-leak rule).
	_, err = env.slice.RetryEligibility(env.ctx(), "job-does-not-exist")
	if codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
}

// TestRetryEligibilityAuthorization applies the decision port: a request
// without a valid session credential is the typed AUTHZ_DENIED envelope —
// the read is authorized exactly like every other read.
func TestRetryEligibilityAuthorization(t *testing.T) {
	env := newTestEnv(t)
	queued := env.submit(t, baseRequest())
	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	_, err := env.slice.RetryEligibility(denied, queued.Job.JobID)
	var ee *EnvelopeError
	if !errors.As(err, &ee) || ee.Envelope.Code != errs.CodeAuthzDenied {
		t.Fatalf("expected the typed AUTHZ_DENIED envelope, got %v", err)
	}
	if _, err := env.slice.RetryEligibility(env.ctx(), queued.Job.JobID); err != nil {
		t.Fatalf("a data-scoped session must read the eligibility: %v", err)
	}
}

// retryableFixture submits and drives one Job to the terminal Failed
// disposition through the real store (the schema-invalid executor fails
// the job terminally exactly as production does), returning its id.
func retryableFixture(t *testing.T, env *testEnv, idem string) string {
	t.Helper()
	invalid := providers.NewMockAdapter(providers.MockBehavior{InvalidStructuredJSON: true})
	reg := dprovider.NewRegistry(nil)
	reg.Upsert(invalid.SeedDataHandling(invalid.DefaultProfile("m")))
	exec, err := providers.NewExecutor(invalid, providers.DefaultEgressPolicy(providers.CoreLocal), security.UnavailableStore{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	env.deps.Providers[dprovider.ProviderMock] = exec
	req := baseRequest()
	req.IdempotencyKey = idem
	req.Schema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"string"}},"required":["ok"]}`)
	sub := env.submit(t, req)
	final, err := RunToSettlement(env.ctx(), env.deps, "worker-retry-command", 1, sub.Job.JobID, 16)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if final.State != aijobrelay.JobFailed {
		t.Fatalf("the fixture job must settle Failed, got %s", final.State)
	}
	return sub.Job.JobID
}

// TestRetryCommand is the M-7 command's app-level pin (IMPL-21, the F9
// frozen semantics): a WRITE through the decision port; a NEW Attempt of
// the SAME Job — never a new Job; eligibility-gated by the SAME truth the
// read reports; the Supplied-keyed reference (replay — never
// regenerated; a contradicting re-issue is the typed CONFLICT_STATE);
// UnknownOutcome never blindly retried; the no-leak NOT_FOUND; the
// required reference.
func TestRetryCommand(t *testing.T) {
	env := newTestEnv(t)

	// A queued Job refuses through the same verdict (no eligibility, no
	// command effect; the reference is still validated first — the
	// Supplied-keyed class requires the operation to carry its reference).
	queued := env.submit(t, baseRequest())
	if _, err := env.slice.RetryJob(env.ctx(), queued.Job.JobID, ""); err != nil {
		if codeOf(t, err) != errs.CodeValidationFailed {
			t.Fatalf("the missing reference must be VALIDATION_FAILED, got %v", err)
		}
	} else {
		t.Fatal("the reference is required")
	}
	if _, err := env.slice.RetryJob(env.ctx(), queued.Job.JobID, "ref-queued"); err == nil {
		t.Fatal("a queued job must refuse the retry")
	} else if codeOf(t, err) != errs.CodeConflictState {
		t.Fatalf("the ineligible retry must be the typed CONFLICT_STATE, got %v", err)
	}
	statusJob, _, err := env.slice.Status(env.ctx(), queued.Job.JobID)
	if err != nil || statusJob.State != aijobrelay.JobQueued {
		t.Fatalf("a refused retry must leave the Job untouched: %+v %v", statusJob, err)
	}

	// The canonical case: the terminal Failed Job applies, re-queues the
	// SAME Job, and the scheduler's next claim creates the NEW Attempt.
	failedID := retryableFixture(t, env, "idem-retry-command")
	res, err := env.slice.RetryJob(env.ctx(), failedID, "ref-1")
	if err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if res.Replayed || res.Job.JobID != failedID || res.Job.State != aijobrelay.JobQueued {
		t.Fatalf("the retry must re-queue the SAME Job fresh: %+v", res)
	}
	replayed, err := env.slice.RetryJob(env.ctx(), failedID, "ref-1")
	if err != nil {
		t.Fatalf("retry replay: %v", err)
	}
	if !replayed.Replayed {
		t.Fatalf("the same reference must replay: %+v", replayed)
	}
	if _, err := env.slice.RetryJob(env.ctx(), failedID, "ref-other"); err == nil {
		t.Fatal("a contradicting re-issue must refuse")
	} else if codeOf(t, err) != errs.CodeConflictState {
		t.Fatalf("the contradicting re-issue must be the typed CONFLICT_STATE, got %v", err)
	}

	// The no-leak rule: an unknown identity is the typed NOT_FOUND.
	if _, err := env.slice.RetryJob(env.ctx(), "job-does-not-exist", "ref-1"); err == nil {
		t.Fatal("an unknown identity must refuse")
	} else if codeOf(t, err) != errs.CodeNotFound {
		t.Fatalf("expected the typed NOT_FOUND, got %v", err)
	}
}

// TestRetryCommandAuthorization applies the decision port to the WRITE
// path: an invalid session credential is the typed AUTHZ_DENIED envelope —
// a retry is a WRITE (ActionDataWrite), authorized exactly like every
// command.
func TestRetryCommandAuthorization(t *testing.T) {
	env := newTestEnv(t)
	queued := env.submit(t, baseRequest())
	denied := WithCaller(env.ctx(), "local", "kry1-not-a-real-token")
	if _, err := env.slice.RetryJob(denied, queued.Job.JobID, "ref-1"); err == nil {
		t.Fatal("an unauthorized retry must refuse")
	} else if codeOf(t, err) != errs.CodeAuthzDenied {
		t.Fatalf("expected the typed AUTHZ_DENIED envelope, got %v", err)
	}
}
