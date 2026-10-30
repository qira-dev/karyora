package updater

import (
	"context"
)

// Engine is the C5-18 machine bound to one installation. The composition
// root constructs it with the installed version, the pinned root key (the
// out-of-band anchor — never a served key), the filesystem layout, and the
// wired ports. REQUIRED ports (Fetcher, Downloader, Backupper, ArtifactSig,
// Health, Activator, Locker, Clock) make the machine refuse fail-closed when
// absent; OPTIONAL ports (SupplyChain — needed only when the manifest
// carries pointers; KindValid; Auditor) degrade to their recorded
// fail-closed or skip behaviors described on each.
type Engine struct {
	InstalledVersion string
	TargetID         string // the C5-11 row id this installation consumes
	PinnedRoot       PinnedRoot
	Layout           Layout
	Clock            Clock
	Locker           Locker
	Fetcher          MetadataFetcher
	Downloader       ArtifactDownloader
	Backupper        Backupper
	SupplyChain      SupplyChainVerifier
	ArtifactSig      ArtifactSignatureVerifier
	KindValid        ArtifactKindValidator
	Health           HealthChecker
	Activator        Activator
	Auditor          Auditor
}

// ReconcileOutcome is the startup reconciliation result (C5-18 §5.2).
type ReconcileOutcome struct {
	State  string // "clean" (S0), "resolved-updated" (S10), "resolved-rollback" (S0 after S9), "paused", "external-reconciled" (X3), "external-mismatch" (X3), "self-check-mismatch", "quarantined"
	Reason Reason
	Detail string
}

// ReconcileAtStartup runs §5.2's ordering exactly:
//  1. acquire the data-directory lock (attach-or-refuse);
//  2. evaluate the clock-quarantine rule against the persisted state;
//  3. reconcile the journal (pre-intent → discard; ACTIVATION_INTENT → the
//     T8 complete-or-revert decision; ACTIVATED → re-enter S8;
//     ROLLING_BACK → complete the revert);
//  4. discard any leftover staging UNCONDITIONALLY (staging is never
//     authoritative);
//  5. the last-known-good self-check — an unexplained mismatch of the
//     running binary is the honest visible state; a mismatch WITH no journal
//     activity is the Mode-B externally-changed install (X3): verify what
//     the mode allows, re-run the health set, and advance the pointer ONLY
//     on a full pass — never the reverse.
func (e *Engine) ReconcileAtStartup(ctx context.Context) (ReconcileOutcome, error) {
	if e.Locker == nil {
		return ReconcileOutcome{}, fail(ReasonPortUnavailable, "the lock port is not wired")
	}
	if err := e.Locker.Acquire(ctx); err != nil {
		return ReconcileOutcome{}, fail(ReasonLockRefused, "the data-directory lock is held elsewhere: %v", err)
	}
	st, err := loadState(e.Layout)
	if err != nil {
		return ReconcileOutcome{}, err
	}
	quarantined := e.quarantined(st)

	// Step 3: the journal reconciliation (§5.2; C5-18 §4.2's restart column).
	j, err := loadJournal(e.Layout)
	if err != nil {
		return ReconcileOutcome{}, err
	}
	if j != nil {
		out, err := e.reconcileJournal(ctx, st, j)
		// Step 4 runs regardless of the branch taken: whatever staging is
		// left after the resolution is discarded unconditionally.
		if cerr := e.resetStaging(); cerr != nil && err == nil {
			return ReconcileOutcome{}, cerr
		}
		return out, err
	}

	// Step 4: stale staging cleanup (unconditional — R8/L1's recovery).
	if err := e.resetStaging(); err != nil {
		return ReconcileOutcome{}, err
	}

	// Step 5: the last-known-good self-check. No journal means no machine
	// activity; a mismatch of the running binary against the pointer is
	// either an external (package-manager) change — X3 — or an unexplained
	// corruption; both surface honestly, never silently self-repaired.
	if st.LastKnownGood.Version != "" {
		current, err := fileSHA256(e.Layout.InstallPath)
		if err != nil {
			return ReconcileOutcome{}, fail(ReasonPortUnavailable, "read the installed binary: %v", err)
		}
		if current != st.LastKnownGood.SHA256 {
			return e.reconcileExternal(ctx, st, current, quarantined)
		}
	}
	if quarantined {
		return ReconcileOutcome{State: "quarantined", Reason: ReasonClockQuarantine,
			Detail: "the wall clock jumped backwards; freshness decisions are quarantined this run"}, nil
	}
	return ReconcileOutcome{State: "clean"}, nil
}

// reconcileJournal is step 3's branches (§5.2; the T8/T10 recovery).
func (e *Engine) reconcileJournal(ctx context.Context, st ClientState, j *Journal) (ReconcileOutcome, error) {
	switch j.Phase {
	case PhaseStaged, PhaseVerified, PhaseBackedUp:
		// Pre-intent: discard the candidate and resume at S0 (§5.2). The
		// committed backup (if any) stays — it is a C5-09 record, never
		// machine-authoritative.
		if err := saveJournal(e.Layout, nil); err != nil {
			return ReconcileOutcome{}, err
		}
		return ReconcileOutcome{State: "clean",
			Detail: "a pre-activation update journal was discarded with its staged candidate"}, nil

	case PhaseIntent:
		// The designed worst case (T8/L2): the journal says an activation was
		// authorized. The CURRENT binary's digest decides.
		current, err := fileSHA256(e.Layout.InstallPath)
		if err != nil {
			return ReconcileOutcome{}, fail(ReasonPortUnavailable, "read the installed binary: %v", err)
		}
		switch {
		case current == j.CandidateDigest:
			// The swap completed: proceed to S8 (complete-and-verify).
			return e.reconcileHealth(ctx, st, j, "activation completed by startup reconciliation")
		case current == j.RetainedDigest:
			// The swap never happened: complete it from the staged candidate
			// when the staging still holds the verified bytes (the journal
			// reconciles BEFORE the staging cleanup — §5.2's order).
			stagedDigest, derr := fileSHA256(j.CandidatePath)
			if derr == nil && stagedDigest == j.CandidateDigest && e.Activator != nil {
				if serr := e.Activator.Swap(ctx, SwapRequest{
					StagedPath:        j.CandidatePath,
					InstallPath:       e.Layout.InstallPath,
					RetainPath:        e.Layout.RetainPath,
					ExpectedNewDigest: j.CandidateDigest,
				}); serr == nil {
					return e.reconcileHealth(ctx, st, j, "activation completed by startup reconciliation")
				}
			}
			// The candidate is gone or the swap failed again: revert-to-prior.
			return e.reconcileRevert(ctx, st, j, "the activation never completed; the prior binary is restored")
		default:
			// Neither old nor new: a half-swapped state — NEVER final.
			return e.reconcileRevert(ctx, st, j, "a half-swapped activation state was detected; the prior binary is restored")
		}

	case PhaseActivated:
		// The swap is durable and health was not settled: re-enter S8
		// idempotently (L3 — no state advances without a PASS).
		return e.reconcileHealth(ctx, st, j, "health re-entered by startup reconciliation")

	case PhaseRollingBack:
		// Complete the revert (T10's restart behavior): the retained prior
		// binary's digest is the decision datum.
		return e.reconcileRevert(ctx, st, j, "an interrupted rollback was completed by startup reconciliation")

	default:
		return ReconcileOutcome{}, fail(ReasonMalformed, "update journal carries unknown phase %q", j.Phase)
	}
}

// reconcileHealth re-enters S8 (the machine's own health gate — the SAME
// T9/T10 code path, so a reconciled health failure rolls back exactly like a
// live one).
func (e *Engine) reconcileHealth(ctx context.Context, st ClientState, j *Journal, detail string) (ReconcileOutcome, error) {
	out, err := e.healthGate(ctx, st, j)
	if err != nil {
		return ReconcileOutcome{}, err
	}
	switch out.State {
	case StateUpdatedStable:
		return ReconcileOutcome{State: "resolved-updated", Reason: out.Reason, Detail: detail}, nil
	case StateUnrecoverable:
		// R13/L5: the rollback mechanism itself failed — the honest worst
		// case must surface as itself, never flattened into a rollback.
		return ReconcileOutcome{State: "unrecoverable", Reason: out.Reason, Detail: out.Detail}, nil
	default:
		return ReconcileOutcome{State: "resolved-rollback", Reason: out.Reason, Detail: out.Detail}, nil
	}
}

// reconcileRevert completes the revert-to-prior (T10's restart column): the
// retained prior binary is the anchor; the failed version is recorded and
// refused; user data is never touched.
func (e *Engine) reconcileRevert(ctx context.Context, st ClientState, j *Journal, detail string) (ReconcileOutcome, error) {
	out, err := e.rollback(ctx, j, string(ReasonSwapFailed), detail)
	if err != nil {
		return ReconcileOutcome{}, err
	}
	if out.State == StateUnrecoverable {
		return ReconcileOutcome{State: "unrecoverable", Reason: out.Reason, Detail: out.Detail}, nil
	}
	return ReconcileOutcome{State: "resolved-rollback", Reason: out.Reason, Detail: detail}, nil
}

// reconcileExternal is X3 (T12/L9): the installed binary changed outside this
// machine (a package-manager transaction). The machine verifies what the
// mode allows, re-runs the health set, and advances the last-known-good
// pointer ONLY on a full pass — the externally-managed install is the
// durable state and the machine reconciles its own records to it, never the
// reverse.
func (e *Engine) reconcileExternal(ctx context.Context, st ClientState, currentDigest string, quarantined bool) (ReconcileOutcome, error) {
	if quarantined {
		// On quarantined clock evidence the machine refuses every freshness-
		// adjacent decision — an external version advance is exactly that.
		return ReconcileOutcome{State: "external-mismatch", Reason: ReasonClockQuarantine,
			Detail: "the installed binary changed externally; reconciliation is quarantined by the clock discontinuity"}, nil
	}
	if e.Health == nil {
		// Fail-closed: without the health set the external version cannot be
		// accepted — the honest mismatch state is shown (never silent).
		e.audit(ctx, EventUpdateExternalReconcile, string(ReasonPortUnavailable), "",
			"an externally changed install could not be health-checked")
		return ReconcileOutcome{State: "external-mismatch", Reason: ReasonPortUnavailable,
			Detail: "an externally changed install was detected but the health-check port is not wired"}, nil
	}
	passed, reason := e.Health.Check(ctx, HealthCheckSpec{
		Version:         "external",
		CandidateDigest: currentDigest,
	})
	if !passed {
		e.audit(ctx, EventUpdateExternalReconcile, reason, "", "an externally changed install failed the health re-run")
		return ReconcileOutcome{State: "external-mismatch", Reason: Reason(reason),
			Detail: "an externally changed install failed the health re-run; the mismatch is shown (last-known-good NOT advanced)"}, nil
	}
	// Full pass: the machine's records move to the externally-managed state.
	// The version identity is the machine's honest "external" marker — the
	// package transaction owns the real version; the machine reconciles its
	// records to the durable state, never the reverse (T12).
	st.LastKnownGood = LastKnownGood{Version: "external", SHA256: currentDigest}
	if err := saveState(e.Layout, st); err != nil {
		return ReconcileOutcome{}, err
	}
	e.audit(ctx, EventUpdateExternalReconcile, "ok", "", "an externally managed install reconciled (health pass)")
	return ReconcileOutcome{State: "external-reconciled", Detail: "the externally managed install passed the health re-run; the last-known-good pointer advanced"}, nil
}
