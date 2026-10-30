package updater

import (
	"context"
)

// The check path (C5-18 §9.3's delegated default: check on demand; never a
// silent install). CheckForUpdate runs the machine's S1/S2 front door ONLY —
// the C5-17 §7 binding order through the version decision — and stops BEFORE
// any staging: nothing downloads, nothing swaps, and the consent gate is
// never crossed. The persisted monotonic state still advances on an accepted
// check (T2 — the rollback protection survives any later failure), exactly
// as the apply path persists it.

// metadataPhase is RunUpdate's and CheckForUpdate's shared front door: the
// mid-flight-journal guard, S1 METADATA_CHECK (T1/T2) and S2's version
// decision (P-DOWNGRADE + refused-through). It returns the persisted state,
// the accepted manifest, and ok=false with the typed Outcome when the machine
// refused, paused, or found no applicable update. The explicit-consent audit
// (EventUpdateSelected) stays with the caller: a check does not select an
// update for install.
func (e *Engine) metadataPhase(ctx context.Context) (ClientState, ReleaseManifest, Outcome, bool, error) {
	st, err := loadState(e.Layout)
	if err != nil {
		return st, ReleaseManifest{}, Outcome{}, false, err
	}
	j, err := loadJournal(e.Layout)
	if err != nil {
		return st, ReleaseManifest{}, Outcome{}, false, err
	}
	if j != nil {
		// §5.2: an unresolved journal means reconciliation must run first —
		// the machine never starts a new update on a mid-flight one.
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonJournalPending,
			Detail: "an update is mid-flight; run startup reconciliation first"}, false, nil
	}

	// --- S1 METADATA_CHECK (T1/T2; the C5-17 §7 order) ---
	bundle, err := e.Fetcher.Fetch(ctx)
	if err != nil {
		e.audit(ctx, EventUpdateCheck, string(ReasonFetchFailed), "", "metadata fetch failed")
		// R1/M9: nothing downloaded, nothing changed; the persisted state
		// (including any cached/expired metadata) authorizes nothing.
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonFetchFailed, Detail: err.Error()}, false, nil
	}
	// T1's audit event records the fetch result on the success path too.
	e.audit(ctx, EventUpdateCheck, "ok", "", "the metadata bundle was fetched")
	if e.quarantined(st) {
		// R15/L8: clock discontinuity — no freshness decision, no install.
		e.audit(ctx, EventUpdateCheck, string(ReasonClockQuarantine), "", "wall-clock discontinuity quarantines freshness decisions")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonClockQuarantine,
			Detail: "the wall clock jumped backwards; freshness decisions are quarantined this run"}, false, nil
	}

	// Root metadata (optional in the bundle): verify under the PINNED root
	// key, check ITS expiry (per-release expiry binds the root too), and
	// enforce root-version monotonicity (the forged-transition refusal —
	// M7b). Acceptance updates the persisted delegation (the chain-of-trust
	// step — M7a).
	signerKey := st.ReleaseSignerKey
	signerFP := st.ReleaseSignerKeyFingerprint
	rootVersion := st.RootVersionSeen
	rootExpiry := st.RootExpiresAt
	if bundle.Root != nil {
		rp, err := VerifyRoot(bundle.Root, e.PinnedRoot.Key)
		if err != nil {
			e.audit(ctx, EventUpdateCheck, string(reasonOf(err)), "", "root metadata refused")
			return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: reasonOf(err), Detail: err.Error()}, false, nil
		}
		if !e.Clock().Before(rp.ExpiresAt) {
			// The root metadata itself is expired: the whole bundle is not
			// fresh (C5-17 §9.1's binding rule: the values apply identically
			// to the manifest and root metadata structures) → X2.
			out, err := e.pause(ctx, st, string(ReasonExpired), "root metadata expired at "+rp.ExpiresAt.UTC().Format(timeRFC3339))
			return st, ReleaseManifest{}, out, false, err
		}
		if rp.RootVersion < rootVersion {
			e.audit(ctx, EventUpdateCheck, string(ReasonVersionRollback), "", "root metadata version rolled back")
			return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonVersionRollback,
				Detail: "root metadata version is below the persisted highest-seen"}, false, nil
		}
		// Chain maintained (M7a): a NEWER root version under the pinned
		// root's own signature is the legitimate transition; the persisted
		// delegation advances atomically with it (no split-brain: the
		// manifest below must bind THIS root version).
		rootVersion = rp.RootVersion
		rootExpiry = rp.ExpiresAt
		signerKey = rp.ReleaseSigner.PublicKey
		signerFP = Fingerprint(rp.ReleaseSigner.PublicKey)
	}
	if len(signerKey) == 0 {
		// No root in the bundle and no persisted delegation: the manifest has
		// no trust anchor — refuse (fail-closed; never trust a bare key).
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonSignatureInvalid,
			Detail: "no trusted release-signer delegation is available to verify the manifest"}, false, nil
	}

	// Manifest: signature over the exact payload bytes under the delegated
	// signer, then the payload bounds (validity FIRST — never freshness).
	manifest, payloadBytes, err := VerifyManifest(&bundle.Manifest, signerKey)
	if err != nil {
		e.audit(ctx, EventUpdateCheck, string(reasonOf(err)), "", "manifest refused")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: reasonOf(err), Detail: err.Error()}, false, nil
	}
	// Consistency (M10): the manifest must bind the root version whose
	// delegation verified it — any cross-release combination is refused.
	if manifest.RootVersion != rootVersion {
		e.audit(ctx, EventUpdateCheck, string(ReasonInconsistent), "", "manifest/root version mismatch")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonInconsistent,
			Detail: "the manifest binds a different root version than the verified root metadata"}, false, nil
	}

	// Freshness AFTER validity (C5-17 §7): expiry + the persisted monotonic
	// state. The expiry value travels inside the signed metadata itself —
	// the machine never carries an expiry number (per-release expiry).
	if !e.Clock().Before(manifest.ExpiresAt) {
		out, err := e.pause(ctx, st, string(ReasonExpired), "manifest expired at "+manifest.ExpiresAt.UTC().Format(timeRFC3339))
		return st, ReleaseManifest{}, out, false, err
	}
	manifestDigest := sha256Hex(payloadBytes)
	if manifest.MetadataVersion < st.HighestSeenMetadataVersion {
		// R4/M2/M3: stale replay or a signed rollback — the persisted state
		// governs; nothing is reset, nothing is downloaded.
		e.audit(ctx, EventUpdateCheck, string(ReasonVersionRollback), manifest.ReleaseVersion, "metadata_version below the persisted highest-seen")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonVersionRollback, Version: manifest.ReleaseVersion,
			Detail: "metadata_version is below the persisted highest-seen (rollback refused)"}, false, nil
	}
	if manifest.MetadataVersion == st.HighestSeenMetadataVersion {
		if manifestDigest != st.HighestSeenManifestDigest {
			// M11: version reuse — the same monotonic version with DIFFERENT
			// bytes is refused (a reset/reused version is never legal).
			e.audit(ctx, EventUpdateCheck, string(ReasonVersionReuse), manifest.ReleaseVersion, "metadata_version reused with different payload")
			return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonVersionReuse, Version: manifest.ReleaseVersion,
				Detail: "metadata_version equals the highest-seen but the payload differs"}, false, nil
		}
		// Idempotent replay of the exact same manifest: no applicable update.
		e.audit(ctx, EventUpdateCheck, "replay", manifest.ReleaseVersion, "identical manifest; nothing to do")
		return st, ReleaseManifest{}, Outcome{State: StateNoUpdate, Version: manifest.ReleaseVersion}, false, nil
	}

	// T2: the metadata is accepted — persist the monotonic state NOW (the
	// rollback protection survives any later failure), record the
	// delegation, clear any pause (a fresh valid check exits X2 — the
	// delegated freeze behavior), and stamp the last-validated time (the
	// clock-quarantine datum).
	st.HighestSeenMetadataVersion = manifest.MetadataVersion
	st.HighestSeenManifestDigest = manifestDigest
	st.RootVersionSeen = rootVersion
	st.ReleaseSignerKey = signerKey
	st.ReleaseSignerKeyFingerprint = signerFP
	st.RootExpiresAt = rootExpiry
	st.LastValidated = e.Clock()
	st.Paused = false
	st.PauseReason = ""
	if err := saveState(e.Layout, st); err != nil {
		return st, ReleaseManifest{}, Outcome{}, false, err
	}
	e.audit(ctx, EventUpdateMetadataAccepted, "", manifest.ReleaseVersion, "manifest verified and fresh")

	// --- S2 VERSION_DECISION (P-DOWNGRADE: no downgrades in v1) ---
	if cmp := CompareVersions(manifest.ReleaseVersion, e.InstalledVersion); cmp < 0 {
		e.audit(ctx, EventUpdateCheck, string(ReasonDowngradeRefused), manifest.ReleaseVersion, "the served release is older than the installed version")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonDowngradeRefused, Version: manifest.ReleaseVersion,
			Detail: "no downgrades in v1 (C5-17 §9.5): the updater never installs an older release"}, false, nil
	}
	if cmp := CompareVersions(manifest.ReleaseVersion, e.InstalledVersion); cmp == 0 {
		return st, ReleaseManifest{}, Outcome{State: StateNoUpdate, Version: manifest.ReleaseVersion}, false, nil
	}
	if refusedThrough(st, manifest.ReleaseVersion) {
		e.audit(ctx, EventUpdateCheck, string(ReasonRefusedVersion), manifest.ReleaseVersion, "the release version is at/below a failed update")
		return st, ReleaseManifest{}, Outcome{State: StateRefused, Reason: ReasonRefusedVersion, Version: manifest.ReleaseVersion,
			Detail: "refused until a NEW release exists (the failed update's version is recorded)"}, false, nil
	}
	return st, manifest, Outcome{}, true, nil
}

// CheckForUpdate runs one on-demand check (S1/S2 only): verify the metadata,
// decide the version, and report — never download, never stage, never swap
// (the explicit-consent gate is the apply path's; C5-18 §9.3's delegated
// default). A newer release is reported as S0 with the candidate's version:
// the machine did not transition past S2, so nothing was selected.
func (e *Engine) CheckForUpdate(ctx context.Context) (Outcome, error) {
	_, manifest, out, ok, err := e.metadataPhase(ctx)
	if err != nil || !ok {
		return out, err
	}
	return Outcome{State: StateNoUpdate, Version: manifest.ReleaseVersion,
		Detail: "an update to " + manifest.ReleaseVersion + " is available; the apply command installs it explicitly"}, nil
}
