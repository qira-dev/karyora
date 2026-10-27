package aijobrelay

import (
	"strings"
	"testing"
)

// sha returns a grammar-valid sha256 digest for fixtures.
func sha(s string) string {
	const hexChars = "0123456789abcdef"
	out := []byte("sha256:")
	for len(out) < 7+64 {
		out = append(out, hexChars[len(out)%16])
	}
	return string(out)
}

func jobIn(state string, version uint64) Job {
	return Job{JobID: "j1", IdempotencyKey: "k1", InputDigest: sha("in"), State: state, Version: version}
}

func attemptWith(epoch uint64) Attempt {
	return Attempt{AttemptID: "att-1", JobID: "j1", Seq: 1, WorkerID: "w1", LeaseID: "l1", LeaseEpoch: epoch, Outcome: AttemptRunning}
}

func TestJobTransitions(t *testing.T) {
	legal := [][2]string{
		{JobCreated, JobQueued}, {JobQueued, JobRunning}, {JobRunning, JobWaiting},
		{JobWaiting, JobNeedsReconciliation}, {JobNeedsReconciliation, JobQueued},
		{JobRunning, JobCancelRequested}, {JobCancelRequested, JobCancelled},
		{JobQueued, JobCancelled}, {JobRunning, JobSucceeded}, {JobRunning, JobFailed},
		{JobRunning, JobTimedOut}, {JobQueued, JobPaused}, {JobPaused, JobQueued},
	}
	for _, tr := range legal {
		if !CanTransition(tr[0], tr[1]) {
			t.Fatalf("transition %s → %s should be legal", tr[0], tr[1])
		}
	}
	illegal := [][2]string{
		{JobSucceeded, JobQueued}, {JobFailed, JobRunning}, {JobCancelled, JobQueued},
		{JobCreated, JobSucceeded}, {JobQueued, JobSucceeded}, {JobTimedOut, JobQueued},
	}
	for _, tr := range illegal {
		if CanTransition(tr[0], tr[1]) {
			t.Fatalf("transition %s → %s should be illegal", tr[0], tr[1])
		}
	}
}

func TestSideEffectTransitions(t *testing.T) {
	if !CanTransitionSideEffect(SideEffectPrepared, SideEffectDispatched) {
		t.Fatal("prepared → dispatched must be legal")
	}
	if !CanTransitionSideEffect(SideEffectDispatched, SideEffectUnknown) {
		t.Fatal("dispatched → unknown must be legal")
	}
	if !CanTransitionSideEffect(SideEffectUnknown, SideEffectFailedNoEff) {
		t.Fatal("unknown → failed_no_effect must be legal")
	}
	if CanTransitionSideEffect(SideEffectSucceeded, SideEffectSucceeded) {
		t.Fatal("self transition must be illegal")
	}
}

func TestUnknownEffectCannotBlindlySucceed(t *testing.T) {
	intent := SideEffectIntent{IntentID: "i1", AttemptID: "att-1", State: SideEffectUnknown}
	err := CheckObservation(intent, 2, "att-1", 1, SideEffectSucceeded, jobIn(JobRunning, 2))
	if err == nil || !strings.Contains(err.Error(), "Unknown effect cannot become a success") {
		t.Fatalf("an Unknown effect became a success without provider-verified reconciliation: %v", err)
	}
	// Reconciliation evidence (a result digest) makes the resolution legal.
	reconciled := intent
	reconciled.ResultDigest = sha("provider-evidence")
	if err := CheckObservation(reconciled, 2, "att-1", 1, SideEffectSucceeded, jobIn(JobRunning, 2)); err != nil {
		t.Fatalf("provider-verified reconciliation should resolve Unknown: %v", err)
	}
	// A stale Job version refuses the observation outright.
	if err := CheckObservation(intent, 1, "att-1", 1, SideEffectFailedNoEff, jobIn(JobRunning, 2)); err == nil {
		t.Fatal("a stale version observation was accepted")
	}
	// An observation not fenced to the owning attempt is refused.
	if err := CheckObservation(intent, 2, "att-other", 1, SideEffectFailedNoEff, jobIn(JobRunning, 2)); err == nil {
		t.Fatal("an unfenced observation was accepted")
	}
}

func TestCheckCommitFencing(t *testing.T) {
	current := jobIn(JobRunning, 2)
	curAtt := attemptWith(1)
	good := Commit{JobID: "j1", JobVersion: 2, AttemptID: "att-1", WorkerID: "w1", LeaseID: "l1",
		LeaseEpoch: 1, ToState: JobSucceeded, AttemptOutcome: AttemptSucceeded,
		ResultDigest: sha("result"), ReceiptID: "rc-1"}
	if err := CheckCommit(good, current, curAtt); err != nil {
		t.Fatalf("a well-fenced commit must pass: %v", err)
	}
	stale := good
	stale.JobVersion = 1
	if err := CheckCommit(stale, current, curAtt); err == nil || !strings.Contains(err.Error(), "stale Job version") {
		t.Fatalf("stale version must be refused: %v", err)
	}
	staleEpoch := good
	staleEpoch.LeaseEpoch = 9
	if err := CheckCommit(staleEpoch, current, curAtt); err == nil || !strings.Contains(err.Error(), "stale lease") {
		t.Fatalf("stale lease epoch must be refused: %v", err)
	}
	otherWorker := good
	otherWorker.WorkerID = "w2"
	if err := CheckCommit(otherWorker, current, curAtt); err == nil {
		t.Fatal("a different worker's commit must be refused")
	}
	wrongPath := good
	wrongPath.ToState = JobQueued
	if err := CheckCommit(wrongPath, current, curAtt); err == nil {
		t.Fatal("running → queued must be refused by the transition table")
	}
	done := jobIn(JobSucceeded, 5)
	if err := CheckCommit(good, done, curAtt); err == nil {
		t.Fatal("a terminal job must refuse commits")
	}
	noReceipt := good
	noReceipt.ReceiptID = ""
	if err := CheckCommit(noReceipt, current, curAtt); err == nil {
		t.Fatal("a succeeded commit requires a receipt")
	}
	waiting := good
	waiting.ToState = JobWaiting
	waiting.AttemptOutcome = AttemptFailedRetryable
	waiting.ResultDigest = ""
	waiting.ReceiptID = ""
	if err := CheckCommit(waiting, current, curAtt); err == nil || !strings.Contains(err.Error(), "typed reason") {
		t.Fatalf("a Waiting commit requires a typed reason: %v", err)
	}
	waiting.WaitingReason = string(WaitBackoffScheduled)
	if err := CheckCommit(waiting, current, curAtt); err != nil {
		t.Fatalf("a typed Waiting commit should pass: %v", err)
	}
}

func TestValidateSubmit(t *testing.T) {
	job := jobIn(JobCreated, 1)
	job.IdempotencyKey = "k1"
	if err := ValidateSubmit(&job); err != nil {
		t.Fatalf("legal submission rejected: %v", err)
	}
	noKey := job
	noKey.IdempotencyKey = ""
	if err := ValidateSubmit(&noKey); err == nil {
		t.Fatal("submission without an idempotency key was accepted")
	}
	wrongState := job
	wrongState.State = JobQueued
	if err := ValidateSubmit(&wrongState); err == nil {
		t.Fatal("submission not in created state was accepted")
	}
}
