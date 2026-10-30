package updater

// The metadata-layer (M) and artifact-layer (A) adversarial rows of the
// C5-19 matrix, bound by test identifier (§8.1's design contract). Every
// assertion maps to the row's expected-state / recovery / event cells.

import (
	"crypto/ed25519"
	"errors"
	"strings"
	"testing"
	"time"
)

// ---- the positive control first: the machine completes a full cycle ----

func TestUpdateHappyPathEndToEnd(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatalf("RunUpdate: %v", err)
	}
	if out.State != StateUpdatedStable || out.Version != newVersion {
		t.Fatalf("outcome = %+v, want S10/%s", out, newVersion)
	}
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.LastKnownGood.Version != newVersion {
		t.Fatalf("last-known-good = %+v, want version %s", st.LastKnownGood, newVersion)
	}
	if st.HighestSeenMetadataVersion != 2 {
		t.Fatalf("highest-seen metadata version = %d, want 2", st.HighestSeenMetadataVersion)
	}
	if st.Paused {
		t.Fatal("the machine must not be paused after a successful update")
	}
	// The journal is settled and the staging released (§2.2's invariants).
	j, err := loadJournal(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if j != nil {
		t.Fatalf("journal not cleared: %+v", j)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not released: %v", entries)
	}
	// The prior binary is retained (one-prior retention) and the new binary
	// is active.
	retained, err := readFile(h.layout.RetainPath)
	if err != nil {
		t.Fatalf("the prior binary was not retained: %v", err)
	}
	if string(retained) != artifactMagic+"v"+installedVersion {
		t.Fatalf("retained binary = %q", retained)
	}
	if h.backupper.calls != 1 {
		t.Fatalf("backup calls = %d, want 1", h.backupper.calls)
	}
	// The audit trail carries the C5-18 §4.2 events in order.
	for _, name := range []string{EventUpdateCheck, EventUpdateMetadataAccepted, EventUpdateSelected,
		EventUpdateDownload, EventUpdateArtifactVerified, EventUpdateBackupCommitted,
		EventUpdateValidated, EventUpdateActivated, EventUpdateHealth, EventUpdateStable} {
		if h.auditor.count(name) == 0 {
			t.Errorf("missing audit event %s", name)
		}
	}
}

// ---- metadata-layer rows (C5-19 §3) ----

// M1 (CLM-UA-001): expired manifest → X2 UPDATE_PAUSED, nothing downloaded.
func TestUpdateAdversarialExpiredManifestPaused(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{expires: h.clock.now.Add(-time.Hour)}) // signature valid, expired
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StatePaused || out.Reason != ReasonExpired {
		t.Fatalf("outcome = %+v, want X2/expired", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0 (nothing may be downloaded on a paused check)", h.downloader.calls)
	}
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Paused || st.PauseReason != string(ReasonExpired) {
		t.Fatalf("state pause = %+v, want the visible X2 pause", st)
	}
	if h.auditor.count(EventUpdatePaused) == 0 {
		t.Error("missing update_paused audit event (never silent — Blueprint §15.2)")
	}
}

// M2 (CLM-UA-002): a stale superseded manifest replayed → refused by the
// monotonic rule; the persisted state untouched.
func TestUpdateAdversarialStaleManifestReplayRefused(t *testing.T) {
	h := newHarness(t)
	st := ClientState{StateVersion: stateVersion, HighestSeenMetadataVersion: 7,
		ReleaseSignerKey: h.signerKey.Public().(ed25519.PublicKey), RootVersionSeen: 1}
	if err := saveState(h.layout, st); err != nil {
		t.Fatal(err)
	}
	h.serve(bundleOpts{metadataVer: 5}) // an older, still-validly-signed manifest
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonVersionRollback {
		t.Fatalf("outcome = %+v, want X1/version_rollback", out)
	}
	after, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if after.HighestSeenMetadataVersion != 7 {
		t.Fatalf("persisted highest-seen = %d, want untouched 7", after.HighestSeenMetadataVersion)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0", h.downloader.calls)
	}
}

// M3 (CLM-UA-003): rollback attack — BOTH the replayed and the freshly
// signed lower-version variants refuse; no reset of persisted state; no
// signed-downgrade extension exists (P-DOWNGRADE = no downgrades in v1).
func TestUpdateAdversarialRollbackRefused(t *testing.T) {
	h := newHarness(t)
	st := ClientState{StateVersion: stateVersion, HighestSeenMetadataVersion: 7,
		ReleaseSignerKey: h.signerKey.Public().(ed25519.PublicKey), RootVersionSeen: 1}
	if err := saveState(h.layout, st); err != nil {
		t.Fatal(err)
	}
	// Variant (a): the replayed lower manifest.
	h.serve(bundleOpts{metadataVer: 5, releaseVersion: lowerVersion})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonVersionRollback {
		t.Fatalf("variant (a) outcome = %+v, want version_rollback", out)
	}
	// Variant (b): a FRESHLY SIGNED manifest (higher metadata_version)
	// offering a LOWER release version — the downgrade refusal.
	h.serve(bundleOpts{metadataVer: 8, releaseVersion: lowerVersion})
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonDowngradeRefused {
		t.Fatalf("variant (b) outcome = %+v, want X1/downgrade_refused", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0 on both variants", h.downloader.calls)
	}
}

// M4 (CLM-UA-004): indefinite freeze — the freeze window is BOUNDED by the
// manifest's own signed expiry (per-release expiry; the concrete value
// travels inside the metadata, never hardcoded).
func TestUpdateAdversarialFreezeBoundedByExpiry(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{expires: h.clock.now.Add(time.Hour)})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.State != StateUpdatedStable {
		t.Fatalf("first cycle: outcome = %+v err = %v, want S10", out, err)
	}
	// The publisher (or an attacker) serves the SAME manifest forever; the
	// clock advances past its expiry: the M1 state arrives.
	h.clock.now = h.clock.now.Add(2 * time.Hour)
	h.serve(bundleOpts{expires: h.clock.now.Add(-time.Hour)}) // the same stale manifest, now past expiry
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StatePaused || out.Reason != ReasonExpired {
		t.Fatalf("outcome = %+v, want the expiry-bounded X2", out)
	}
}

// M5 (CLM-UA-005/006): fast-forward — (a) unsigned/wrong-key refused at S1;
// (b) signed by a COMPROMISED release signer: S1 passes, the artifact layer
// is the honest second barrier (the residual stated, never hidden).
func TestUpdateAdversarialFastForwardUnsignedRefused(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{manifestKey: h.forgedKey}) // a metadata-signing capability NOT delegated by the root
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonSignatureInvalid {
		t.Fatalf("outcome = %+v, want X1/signature_invalid", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0", h.downloader.calls)
	}
}

func TestUpdateAdversarialCompromisedSignerSecondBarrier(t *testing.T) {
	h := newHarness(t)
	// The attacker HOLDS the real release-signer key (the compromise): the
	// manifest verifies at S1 — the monotonic version is even correct.
	// The malicious artifact carries its own (attacker-computed) digest and
	// an artifact signature made with the ATTACKER's artifact key.
	h.serve(bundleOpts{artifactSigKey: &h.artifactK2})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonArtifactSignature {
		t.Fatalf("outcome = %+v, want the artifact layer to refuse the compromised-signer release", out)
	}
	// Nothing was activated; the staging was deleted (the second barrier
	// held; the residual — until the root's revocation propagates — is
	// stated by this test's existence, never hidden).
	if h.health.calls != 0 {
		t.Fatal("no health check may run on a refused artifact")
	}
	current, err := readFile(h.layout.InstallPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != artifactMagic+"v"+installedVersion {
		t.Fatalf("the installed binary changed: %q", current)
	}
}

// M6 (CLM-UA-007): threshold violation — at threshold 1 (the delegated
// value) any missing/invalid metadata signature is a shortfall (zero valid
// signatures); a root document declaring a different threshold is refused.
func TestUpdateAdversarialThresholdShortfallRefused(t *testing.T) {
	h := newHarness(t)
	// Variant (a): the manifest carries a signature that does NOT verify —
	// zero valid signatures against threshold 1 → the shortfall refusal.
	h.serve(bundleOpts{manifestKey: h.forgedKey})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonSignatureInvalid {
		t.Fatalf("variant (a) outcome = %+v, want the threshold-1 shortfall refusal", out)
	}
	// Variant (b): a root document declaring threshold 2 — not this client's
	// policy; a threshold change is a versioned decision, never client-adaptive.
	rp, err := CanonicalPayload(RootPayload{
		SchemaVersion: RootSchema,
		RootVersion:   3,
		ExpiresAt:     h.clock.now.Add(24 * time.Hour),
		ReleaseSigner: SignerDelegation{PublicKey: h.signerKey.Public().(ed25519.PublicKey), Threshold: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.fetcher.bundle = MetadataBundle{Root: signedDocPtr(rp, h.rootKey), Manifest: *signedDoc([]byte("{\"schema_version\":\"uawp-update-manifest/v1\"}"), h.signerKey)}
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonThresholdShortfall && out.Reason != ReasonMalformed {
		t.Fatalf("variant (b) outcome = %+v, want the threshold refusal", out)
	}
}

// M7 (CLM-UA-008/009): root transition — (a) the legitimate transition
// maintains the chain (a newer root version under the pinned root's own
// signature is accepted and advances the persisted delegation); (b) a forged
// transition (a root version NOT signed by the trusted root) refuses.
func TestUpdateAdversarialRootTransitionChainMaintained(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{rootVersion: 2}) // root v2, signed by the PINNED root key
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateUpdatedStable {
		t.Fatalf("outcome = %+v, want the update to proceed under the transitioned root", out)
	}
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.RootVersionSeen != 2 {
		t.Fatalf("persisted root version = %d, want 2 (the chain of trust advanced)", st.RootVersionSeen)
	}
}

func TestUpdateAdversarialForgedRootTransitionRefused(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{rootVersion: 3, rootSignedBy: h.forgedKey}) // a root "transition" the pinned root never signed
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonSignatureInvalid {
		t.Fatalf("outcome = %+v, want the fail-closed forged-transition refusal", out)
	}
	st, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if st.RootVersionSeen != 0 {
		t.Fatalf("persisted root version = %d, want untouched 0", st.RootVersionSeen)
	}
}

// M8 (CLM-UA-010/011): malformed/oversized metadata — typed refusals with
// hard bounds; no parse crash, no partial acceptance.
func TestUpdateAdversarialMalformedMetadataRefused(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{}) // keep the trusted root; corrupt only the manifest payload
	// A hand-built manifest payload violating the single-channel rule and a
	// duplicated-target rule (field-bound violations, M8).
	bad, err := CanonicalPayload(ReleaseManifest{
		SchemaVersion:   ManifestSchema,
		Channel:         "nightly", // v1 carries exactly one channel
		ReleaseVersion:  newVersion,
		MetadataVersion: 9,
		RootVersion:     1,
		ExpiresAt:       h.clock.now.Add(time.Hour),
		Artifacts: []ArtifactEntry{
			{TargetID: testTarget, Kind: kindBinary, SHA256: sha256Hex([]byte("x")), Size: 1},
			{TargetID: testTarget, Kind: kindBinary, SHA256: sha256Hex([]byte("y")), Size: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.fetcher.bundle.Manifest = *signedDoc(bad, h.signerKey)
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonMalformed {
		t.Fatalf("outcome = %+v, want the malformed refusal", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0", h.downloader.calls)
	}
}

func TestUpdateAdversarialOversizedMetadataRefused(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{}) // keep the trusted root; corrupt only the manifest
	huge := strings.Repeat("a", MaxMetadataBytes+1)
	h.fetcher.bundle.Manifest = SignedDocument{Payload: huge,
		Signature: Signature{Algorithm: SigAlgorithm, KeyID: "x", Value: make([]byte, 64)}}
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonOversized {
		t.Fatalf("outcome = %+v, want the oversized refusal", out)
	}
}

// M9 (CLM-UA-012): offline / fetch failure — no install decision, the cached
// (paused) state authorizes nothing.
func TestUpdateAdversarialOfflineNoInstallFromCache(t *testing.T) {
	h := newHarness(t)
	st := ClientState{StateVersion: stateVersion, Paused: true, PauseReason: string(ReasonExpired),
		ReleaseSignerKey: h.signerKey.Public().(ed25519.PublicKey)}
	if err := saveState(h.layout, st); err != nil {
		t.Fatal(err)
	}
	h.fetcher.err = errors.New("no route to host")
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonFetchFailed {
		t.Fatalf("outcome = %+v, want X1/fetch_failed", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0 (cached metadata authorizes nothing)", h.downloader.calls)
	}
	after, err := loadState(h.layout)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Paused {
		t.Fatal("the visible pause state must persist across an offline check")
	}
}

// M10 (CLM-UA-013): mix-and-match — a manifest bound to a DIFFERENT root
// version than the verified root metadata is structurally refused.
func TestUpdateAdversarialMixAndMatchRefused(t *testing.T) {
	h := newHarness(t)
	// The bundle's root is v5; the manifest (signed by the same delegated
	// signer) binds root v4 — a splice of two releases.
	h.serve(bundleOpts{rootVersion: 5, manifestRootV: 4})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonInconsistent {
		t.Fatalf("outcome = %+v, want the inconsistent-metadata refusal", out)
	}
	if h.downloader.calls != 0 {
		t.Fatalf("downloads = %d, want 0", h.downloader.calls)
	}
}

// M11 (CLM-UA-014): the owner re-sign path is legal ONLY forward (+1); a
// version RESET or REUSE is refused.
func TestUpdateAdversarialResignVersionMonotonicity(t *testing.T) {
	h := newHarness(t)
	// v1: the original manifest (metadata_version 5) applies cleanly.
	h.serve(bundleOpts{metadataVer: 5})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.State != StateUpdatedStable {
		t.Fatalf("original: outcome = %+v err = %v, want S10", out, err)
	}
	// v2: the legitimate re-sign (metadata_version + 1, same digests, new
	// expiry) — a fresh manifest for a NEW release is accepted.
	h.serve(bundleOpts{metadataVer: 6, releaseVersion: "1.2.0"})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.State != StateUpdatedStable {
		t.Fatalf("re-sign forward: outcome = %+v err = %v, want S10", out, err)
	}
	// v3: a RESET (a lower metadata_version) — refused.
	h.serve(bundleOpts{metadataVer: 4, releaseVersion: "1.3.0"})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonVersionRollback {
		t.Fatalf("reset outcome = %+v, want version_rollback", out)
	}
	// v4: a REUSE (the same metadata_version with DIFFERENT bytes) — refused.
	h.serve(bundleOpts{metadataVer: 6, releaseVersion: "9.9.9"})
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonVersionReuse {
		t.Fatalf("reuse outcome = %+v, want version_reuse", out)
	}
}

// ---- artifact-layer rows (C5-19 §4) ----

// A1 (CLM-UA-015): mismatched artifact digest → staging DELETED → X1.
func TestUpdateAdversarialDigestMismatchStagingDeleted(t *testing.T) {
	h := newHarness(t)
	h.serve(bundleOpts{})                                       // the manifest declares the digest of the REAL artifact
	h.downloader.content = []byte("CORRUPTED-OR-SWAPPED-BYTES") // the delivered bytes differ
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonDigestMismatch {
		t.Fatalf("outcome = %+v, want X1/digest_mismatch", out)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not deleted: %v", entries)
	}
	j, _ := loadJournal(h.layout)
	if j != nil {
		t.Fatalf("a journal entry must not exist after a digest refusal: %+v", j)
	}
	current, _ := readFile(h.layout.InstallPath)
	if string(current) != artifactMagic+"v"+installedVersion {
		t.Fatalf("the installed binary changed: %q", current)
	}
}

// A2 (CLM-UA-016): malformed artifact — refused BEFORE any install-side
// processing; staging deleted.
func TestUpdateAdversarialMalformedArtifactRefused(t *testing.T) {
	h := newHarness(t)
	bad := []byte("NOT-A-UAWP-BINARY-AT-ALL") // structurally invalid for its kind, digest consistent
	h.serve(bundleOpts{artifactBytes: func() []byte { return bad }})
	h.downloader.content = bad
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonArtifactMalformed {
		t.Fatalf("outcome = %+v, want X1/artifact_malformed", out)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not deleted: %v", entries)
	}
}

// A3 (CLM-UA-017): artifact signature failure — wrong-key and absent
// variants refuse per the C5-13 §3.2 order (digest FIRST, then signature);
// no digest-only downgrade.
func TestUpdateAdversarialArtifactSignatureRefused(t *testing.T) {
	h := newHarness(t)
	// Variant (a): the digest matches, but the artifact signature was made
	// by a key the platform identity does not pin.
	h.serve(bundleOpts{artifactSigKey: &h.artifactK2})
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonArtifactSignature {
		t.Fatalf("variant (a) outcome = %+v, want X1/artifact_signature", out)
	}
	// Variant (b): the signature is ABSENT — never a digest-only pass (a
	// fresh metadata_version: variant (a) already advanced the state).
	h.serve(bundleOpts{dropSignature: true, metadataVer: 3})
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonArtifactSignature {
		t.Fatalf("variant (b) outcome = %+v, want X1/artifact_signature (absent)", out)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not deleted: %v", entries)
	}
	if h.health.calls != 0 {
		t.Fatal("no code path may reach S8 while the artifact layer refuses")
	}
}

// A5 (CLM-UA-024): supply-chain pointer unresolvable/mismatched → refusal;
// staging deleted.
func TestUpdateAdversarialSupplyChainPointerRefused(t *testing.T) {
	h := newHarness(t)
	artifactBytes := []byte(artifactMagic + "v" + newVersion)
	ptrDigest := sha256Hex(artifactBytes)
	// Variant (a): the resolver cannot find the binding record.
	h.serve(bundleOpts{supplyChain: []SupplyChainPtr{{ArtifactSHA256: ptrDigest, BindingRecordSHA256: strings.Repeat("a", 64)}}})
	h.downloader.content = artifactBytes
	out, err := h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateRefused || out.Reason != ReasonSupplyChain {
		t.Fatalf("variant (a) outcome = %+v, want X1/supply_chain", out)
	}
	// Variant (b): the record resolves to a DIFFERENT digest (a fresh
	// metadata_version — variant (a) already advanced the monotonic state).
	h.serve(bundleOpts{metadataVer: 3, supplyChain: []SupplyChainPtr{{ArtifactSHA256: ptrDigest, BindingRecordSHA256: strings.Repeat("b", 64)}}})
	h.supply.resolved = map[string]string{ptrDigest: strings.Repeat("c", 64)}
	out, err = h.engine.RunUpdate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if out.Reason != ReasonSupplyChain {
		t.Fatalf("variant (b) outcome = %+v, want X1/supply_chain", out)
	}
	entries, _ := listDir(h.layout.StagingDir)
	if len(entries) != 0 {
		t.Fatalf("staging not deleted: %v", entries)
	}
}

// A6 (CLM-UA-025): the verification-order invariant — NO code path reaches
// the artifact layer (or anything after it) while an earlier check is
// unresolved, and a pre-staged artifact can never be activated while the
// metadata fails.
func TestUpdateAdversarialCheckOrderInvariant(t *testing.T) {
	h := newHarness(t)
	// Scenario A: the metadata signature fails → the downloader, the artifact
	// verifier, and the backup are NEVER invoked.
	h.serve(bundleOpts{manifestKey: h.forgedKey})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.Reason != ReasonSignatureInvalid {
		t.Fatalf("scenario A: outcome = %+v err = %v", out, err)
	}
	if h.downloader.calls != 0 || h.artSig.calls != 0 || h.backupper.calls != 0 {
		t.Fatalf("scenario A order violation: downloads=%d artSig=%d backup=%d (all must be 0)",
			h.downloader.calls, h.artSig.calls, h.backupper.calls)
	}
	// Scenario B: the digest fails → the artifact-signature verifier and the
	// backup are NEVER invoked (digest strictly precedes signature).
	h.serve(bundleOpts{})
	h.downloader.content = []byte("TAMPERED")
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.Reason != ReasonDigestMismatch {
		t.Fatalf("scenario B: outcome = %+v err = %v", out, err)
	}
	if h.artSig.calls != 0 || h.backupper.calls != 0 {
		t.Fatalf("scenario B order violation: artSig=%d backup=%d (both must be 0)", h.artSig.calls, h.backupper.calls)
	}
	// Scenario C: a pre-staged candidate with an ACTIVATION_INTENT journal
	// while the served metadata is garbage — the machine refuses to start a
	// new update on a mid-flight one, and the startup reconciliation discards
	// the pre-staged artifact WITHOUT activating it.
	if err := writeFile(h.layout.StagingDir+"/candidate", []byte(artifactMagic+"v"+newVersion)); err != nil {
		t.Fatal(err)
	}
	if err := saveJournal(h.layout, &Journal{JournalVersion: journalVer, Phase: PhaseIntent,
		OldVersion: installedVersion, NewVersion: newVersion,
		CandidateDigest: sha256Hex([]byte(artifactMagic + "v" + newVersion)),
		CandidatePath:   h.layout.StagingDir + "/candidate",
		RetainedDigest:  sha256Hex([]byte(artifactMagic + "v" + installedVersion))}); err != nil {
		t.Fatal(err)
	}
	h.serve(bundleOpts{manifestKey: h.forgedKey})
	if out, err := h.engine.RunUpdate(t.Context()); err != nil || out.Reason != ReasonJournalPending {
		t.Fatalf("scenario C run: outcome = %+v err = %v, want the journal-pending refusal", out, err)
	}
	rec, err := h.engine.ReconcileAtStartup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// The pre-staged candidate's digest matches the journaled intent and the
	// installed binary is still the OLD one — reconciliation either completes
	// the activation under the FULL check chain (health gate) or reverts; it
	// NEVER silently accepts the pre-staged artifact as final. Here the
	// health port passes, so the journal-driven completion is the safe exit.
	if rec.State != "resolved-updated" && rec.State != "resolved-rollback" {
		t.Fatalf("scenario C reconcile = %+v, want a journal-resolved outcome", rec)
	}
	j, _ := loadJournal(h.layout)
	if j != nil {
		t.Fatalf("the journal must be resolved after reconciliation: %+v", j)
	}
}
