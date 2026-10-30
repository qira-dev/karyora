// The composition root of the C5-18 update machine (the sub-milestone 4
// recorded integration step): the `uawp update` surface and every production
// port implementation. The surface is the delegated default of C5-18 §9.3 —
// check on demand, install only by the explicit `apply` invocation, never a
// silent install. The machine (internal/updater) owns every verification and
// transition; this file only wires the environment to its ports:
//
//   - httpMetadataFetcher   — the official endpoint's metadata bundle
//     (C5-18 T1); the endpoint base URL is configuration
//     (update.endpoint_url; C5-17 §4.3 keeps URLs out of the metadata).
//   - httpArtifactDownloader — streams the manifest's artifact into the
//     disposable staging area (C5-18 S3) under the exact declared size.
//   - channelBindingResolver — resolves the manifest's supply_chain[]
//     pointers to the published C5-16 binding records (C5-18 S4/A5).
//   - blockedSignatureVerifier — the C5-13 §3.2 artifact-signature layer,
//     which CANNOT pass while the signing stage is BLOCKED (C5-13 §9.2:
//     the custody/handover-moment-1 items are open): every update refuses
//     fail-closed with the typed artifact_signature reason. This is the
//     honest recorded state — never a digest-only downgrade, never a stub
//     that passes.
//   - storageBackupper — the C5-09-ACCEPTED local backup path (the delivered
//     C2-16/C2-17 mechanism; no new guarantee, no schedule).
//   - ownershipLocker — the delivered single-authority data-directory lock
//     (attach-or-refuse; a running Core means updates refuse).
//   - probeHealthChecker — S8: the swapped-in binary starts under
//     observation as a real process, acquires the data-directory lock
//     itself, applies migrations transactionally, and answers the
//     diagnostics probe (the minimal v1 check set).
//   - updateAuditor — the machine's audit events into the delivered
//     tamper-evident audit chain (C2-15).
//
// The pinned root key is a CONSTRUCTION input loaded from an out-of-band
// anchor file (C5-17 §4.1: trusting a served key would erase the two-key
// recovery property). Until the root ceremony produces that file (BLOCKED
// with the signing stage), updates refuse fail-closed exactly as the
// artifact-signature layer does.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qira-dev/karyora/internal/config"
	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
	"github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/security"
	"github.com/qira-dev/karyora/internal/storage"
	"github.com/qira-dev/karyora/internal/updater"
)

// updateTarget is one C5-11-approved support row (the six-row v1 matrix).
type updateTarget struct {
	id     string // the row id the manifest's artifacts[] entries carry
	goos   string
	goarch string
}

// approvedTargets are exactly the C5-11 §2.3 rows; every other combination
// is unsupported by rule and refused before any network activity.
var approvedTargets = map[string]updateTarget{
	"windows-amd64": {id: "windows-amd64", goos: "windows", goarch: "amd64"},
	"windows-arm64": {id: "windows-arm64", goos: "windows", goarch: "arm64"},
	"macos-amd64":   {id: "macos-amd64", goos: "darwin", goarch: "amd64"},
	"macos-arm64":   {id: "macos-arm64", goos: "darwin", goarch: "arm64"},
	"linux-amd64":   {id: "linux-amd64", goos: "linux", goarch: "amd64"},
	"linux-arm64":   {id: "linux-arm64", goos: "linux", goarch: "arm64"},
}

// targetRowForPlatform resolves the running platform to its approved row;
// an excluded combination (C5-11: Windows 10, macOS 14, musl/Alpine, and
// every unlisted OS/arch) is a typed refusal, never a best-effort match.
func targetRowForPlatform(goos, goarch string) (updateTarget, error) {
	for _, t := range approvedTargets {
		if t.goos == goos && t.goarch == goarch {
			return t, nil
		}
	}
	return updateTarget{}, fmt.Errorf("no approved support row for %s/%s (C5-11: every unlisted combination is unsupported by rule)", goos, goarch)
}

// artifactName is the release channel's binary naming convention (the
// release-build pipeline's artifact name: uawp-<goos>-<goarch>).
func (t updateTarget) artifactName() string {
	return "uawp-" + t.goos + "-" + t.goarch
}

// updateLayout builds the machine's filesystem layout (C5-15 §5.0's binding
// data-directory contract): the install/retain paths live in the app-owned
// install location beside the running binary; staging is the rebuildable
// cache class; the journal/client state is the append-only state class —
// never the user-data classes.
func buildUpdateLayout(ds platform.DirSet) (updater.Layout, error) {
	self, err := os.Executable()
	if err != nil {
		return updater.Layout{}, fmt.Errorf("resolve the running binary: %w", err)
	}
	layout := updater.Layout{
		InstallPath: self,
		RetainPath:  self + ".previous",
		StagingDir:  filepath.Join(ds.Cache, "update-staging"),
		StateDir:    filepath.Join(ds.State, "update"),
	}
	for _, dir := range []string{layout.StagingDir, layout.StateDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return updater.Layout{}, fmt.Errorf("prepare the update directories: %w", err)
		}
	}
	return layout, nil
}

// pinnedRootAnchorFile is the out-of-band trust anchor (C5-17 §4.1): the
// root public key whose fingerprint the canonical anchor page publishes,
// placed on the machine by the release documentation's documented step —
// NEVER fetched from the update channel.
const pinnedRootAnchorFile = "pinned-root.json"

type pinnedRootAnchor struct {
	Key         string `json:"key"`         // hex-encoded Ed25519 public key
	Fingerprint string `json:"fingerprint"` // hex sha256 of the key (cross-check)
}

// loadPinnedRoot reads the out-of-band anchor; a missing file is the honest
// pre-ceremony state (zero key — the machine then refuses every update
// fail-closed), while a corrupt or self-inconsistent one is a hard failure.
func loadPinnedRoot(stateDir string) (updater.PinnedRoot, error) {
	raw, err := os.ReadFile(filepath.Join(stateDir, pinnedRootAnchorFile))
	if err != nil {
		if os.IsNotExist(err) {
			// No anchor yet: the zero key makes every verification refuse
			// (C5-17 §4.1 — the anchor arrives out of band, or not at all).
			return updater.PinnedRoot{}, nil
		}
		return updater.PinnedRoot{}, fmt.Errorf("read the pinned root anchor: %w", err)
	}
	var a pinnedRootAnchor
	if err := json.Unmarshal(raw, &a); err != nil {
		return updater.PinnedRoot{}, fmt.Errorf("the pinned root anchor is not valid JSON: %w", err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(a.Key))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return updater.PinnedRoot{}, fmt.Errorf("the pinned root anchor's key is not a %d-byte hex Ed25519 public key", ed25519.PublicKeySize)
	}
	if fp := updater.Fingerprint(ed25519.PublicKey(key)); a.Fingerprint != "" && fp != strings.ToLower(strings.TrimSpace(a.Fingerprint)) {
		return updater.PinnedRoot{}, fmt.Errorf("the pinned root anchor's fingerprint does not match its key (%s != %s)", fp, a.Fingerprint)
	}
	return updater.PinnedRoot{Key: ed25519.PublicKey(key)}, nil
}

// fetchBound reads a response body under a hard byte cap (the resource-abuse
// row: oversized/malformed channel content is refused before it can exhaust
// the client — C5-17 §5 row 8).
func fetchBound(resp *http.Response, capBytes int64) ([]byte, error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the channel answered HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, capBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read the channel response: %w", err)
	}
	if int64(len(body)) > capBytes {
		return nil, fmt.Errorf("the channel response exceeds the %d-byte bound", capBytes)
	}
	return body, nil
}

// metadataRawCap bounds the raw bundle: two documents (root + manifest) at
// the machine's own 1 MiB per-payload bound plus the JSON envelope overhead.
const metadataRawCap = 4 << 20

// bindingRecordCap bounds one C5-16 binding record (a small evidence record).
const bindingRecordCap = 1 << 20

// httpMetadataFetcher implements updater.MetadataFetcher over the official
// endpoint: GET <base>/update.json, bounded, decoded; the machine owns every
// verification beyond the decode.
type httpMetadataFetcher struct {
	base *url.URL
}

func newHTTPMetadataFetcher(base string) (*httpMetadataFetcher, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("the update endpoint %q is not an absolute http(s) URL", base)
	}
	return &httpMetadataFetcher{base: u}, nil
}

func (f *httpMetadataFetcher) resolve(ref string) string {
	u := *f.base
	u.Path = strings.TrimSuffix(u.Path, "/") + ref
	return u.String()
}

// Fetch implements updater.MetadataFetcher.
func (f *httpMetadataFetcher) Fetch(ctx context.Context) (updater.MetadataBundle, error) {
	var bundle updater.MetadataBundle
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.resolve("/update.json"), nil)
	if err != nil {
		return bundle, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return bundle, fmt.Errorf("the metadata endpoint is unreachable: %w", err)
	}
	raw, err := fetchBound(resp, metadataRawCap)
	if err != nil {
		return bundle, err
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		return bundle, fmt.Errorf("the metadata bundle is not valid JSON: %w", err)
	}
	return bundle, nil
}

// httpArtifactDownloader implements updater.ArtifactDownloader: it streams
// the manifest's artifact for this target into the staging area under the
// EXACT declared size (the machine's digest check is the real gate; the
// downloader's contract is "write the bytes, report the failure" — a partial
// or oversized result is refused and the disposable staging rule discards
// it). The staged file is written with the delivered temp-file + fsync +
// rename discipline and made executable (it becomes the installed binary).
type httpArtifactDownloader struct {
	fetcher *httpMetadataFetcher
	target  updateTarget
}

// Download implements updater.ArtifactDownloader.
func (d *httpArtifactDownloader) Download(ctx context.Context, entry updater.ArtifactEntry, destPath string) error {
	if entry.Size <= 0 {
		return fmt.Errorf("the manifest declares a non-positive artifact size (%d)", entry.Size)
	}
	u := d.fetcher.resolve("/artifacts/" + d.target.id + "/" + d.target.artifactName())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("the artifact endpoint is unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the artifact endpoint answered HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".updstage-*")
	if err != nil {
		return fmt.Errorf("create the staging temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	// Copy EXACTLY the declared size; one extra byte is a refusal.
	if _, err := io.CopyN(tmp, resp.Body, entry.Size); err != nil {
		tmp.Close()
		return fmt.Errorf("the artifact stream did not deliver the declared %d bytes: %w", entry.Size, err)
	}
	var extra [1]byte
	if n, _ := io.ReadFull(resp.Body, extra[:]); n == 1 {
		tmp.Close()
		return fmt.Errorf("the artifact stream carries more than the declared %d bytes", entry.Size)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("fsync the staged artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("mark the staged artifact executable: %w", err)
	}
	if err := os.Rename(tmpName, destPath); err != nil {
		return fmt.Errorf("move the staged artifact into place: %w", err)
	}
	return nil
}

// channelBindingResolver implements updater.SupplyChainVerifier over the
// release channel's published C5-16 binding records: the record for an
// artifact digest lives at <base>/bindings/<artifact sha256>.json; the
// resolver returns the EXACT served bytes' sha256 (the machine compares it
// against the manifest's signed pointer). The deeper per-release record
// verification (C5-16 §7's verifier policy) is the release-time evidence
// work — this port resolves presence and digest-exactness, fail-closed.
type channelBindingResolver struct {
	fetcher *httpMetadataFetcher
}

// Resolve implements updater.SupplyChainVerifier.
func (r *channelBindingResolver) Resolve(ctx context.Context, ptr updater.SupplyChainPtr) (string, error) {
	u := r.fetcher.resolve("/bindings/" + ptr.ArtifactSHA256 + ".json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("the binding-record endpoint is unreachable: %w", err)
	}
	raw, err := fetchBound(resp, bindingRecordCap)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// blockedSignatureVerifier is the production artifact-signature layer while
// the signing stage is BLOCKED (C5-13 §9.2): it ALWAYS refuses with the
// typed artifact_signature reason naming the block. Updates therefore
// refuse fail-closed until the custody items close and a real verifier
// replaces it — the honest recorded state, never a passing stub.
type blockedSignatureVerifier struct{}

// Verify implements updater.ArtifactSignatureVerifier.
func (blockedSignatureVerifier) Verify(_ context.Context, _ string, _ *updater.Signature, _ string) error {
	return &updater.Error{Reason: updater.ReasonArtifactSignature,
		Detail: "the artifact-signature layer cannot verify: the C5-13 signing stage is BLOCKED (custody is handover moment 1); updates stay refused until signing exists"}
}

// storageBackupper adapts the delivered C5-09 backup path (C2-16/C2-17) to
// the machine's Backupper port: one user-initiated-by-the-update-flow
// snapshot; the reference is the delivered backup generation the journal
// records (T6). No schedule, no RPO/RTO, no cloud claim.
type storageBackupper struct {
	opts          storage.Options
	coreVersion   string
	correlationID string
}

// CommitBackup implements updater.Backupper.
func (b *storageBackupper) CommitBackup(ctx context.Context) (string, error) {
	s, err := storage.Connect(b.opts)
	if err != nil {
		return "", fmt.Errorf("open the store for the pre-update backup: %w", err)
	}
	defer s.Close()
	manifest, err := s.Backup(ctx, b.correlationID)
	if err != nil {
		return "", fmt.Errorf("the pre-update backup did not commit: %w", err)
	}
	return manifest.BackupGeneration, nil
}

// ownershipLocker adapts the delivered single-authority data-directory lock
// (attach-or-refuse) to the machine's Locker port. A running Core holds the
// lock, so updates refuse while the Core runs — the single-writer rule.
type ownershipLocker struct {
	ds      platform.DirSet
	dataDir string
	version string
	mu      sync.Mutex
	owned   *platform.Ownership
}

// Acquire implements updater.Locker.
func (l *ownershipLocker) Acquire(_ context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owned != nil {
		return nil // already the single authority (reconcile + the command share one acquisition)
	}
	own, _, err := platform.AcquireOwnership(l.ds, l.dataDir, l.version)
	if err != nil {
		return err
	}
	l.owned = own
	return nil
}

// release drops the lock (best-effort at command exit; required before the
// health probe can acquire it itself — C5-18 S8's own lock acquisition).
func (l *ownershipLocker) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owned != nil {
		_ = l.owned.Release()
		l.owned = nil
	}
}

// reacquire takes the lock back after the health probe released it; a
// failure here means someone else took the data directory — the caller
// reports it as a failed health check (fail-closed, never assume).
func (l *ownershipLocker) reacquire() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owned != nil {
		return nil
	}
	own, _, err := platform.AcquireOwnership(l.ds, l.dataDir, l.version)
	if err != nil {
		return err
	}
	l.owned = own
	return nil
}

// healthProbeTimeout bounds one S8 observation window (the minimal v1 set:
// process start, the data-directory lock, the database, transactional
// migrations, the schema range, diagnostics). A bounded window keeps a hung
// candidate from hanging the update — the timeout is a typed health FAILURE
// (the machine rolls back), never a silent pass.
const healthProbeTimeout = 2 * time.Minute

// probeHealthChecker implements updater.HealthChecker: it spawns the
// swapped-in binary as a real process in the internal health-probe mode
// (`uawp update health-probe`), which acquires the data-directory lock,
// applies migrations transactionally, and reports the landed schema. The
// updater's own lock is released for the probe's lifetime (the probe must
// prove it can own the data directory — never a second authority) and
// re-taken afterwards; a re-take failure is a failed check (fail-closed).
type probeHealthChecker struct {
	binaryPath string
	dataDir    string
	locker     *ownershipLocker
}

type healthProbeReport struct {
	Passed        bool   `json:"passed"`
	Reason        string `json:"reason,omitempty"`
	Detail        string `json:"detail,omitempty"`
	SchemaVersion int64  `json:"schema_version"`
	Version       string `json:"version"`
}

// Check implements updater.HealthChecker. The lock is released for the
// probe's lifetime and re-taken BEFORE the verdict returns: a probe PASS
// without a re-acquired lock is a FAILED check (fail-closed — the machine
// rolls back rather than settling an update while the data directory is
// unowned).
func (h *probeHealthChecker) Check(ctx context.Context, spec updater.HealthCheckSpec) (bool, string) {
	h.locker.release()
	passed, reason := h.probe(ctx, spec)
	if err := h.locker.reacquire(); err != nil {
		return false, "the data directory could not be re-locked after the health probe: " + err.Error()
	}
	if !passed {
		return false, reason
	}
	return true, "pass"
}

func (h *probeHealthChecker) probe(ctx context.Context, spec updater.HealthCheckSpec) (bool, string) {
	runCtx, cancel := context.WithTimeout(ctx, healthProbeTimeout)
	defer cancel()
	cmdArgs := []string{"update", "health-probe",
		"--data-dir", h.dataDir,
		"--schema-to", strconv.Itoa(spec.MigrationTo),
		"--json"}
	cmd := exec.CommandContext(runCtx, h.binaryPath, cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		return false, "the health probe did not complete cleanly: " + boundedProbeErr(err, out)
	}
	var report healthProbeReport
	if err := json.Unmarshal(out, &report); err != nil {
		return false, "the health probe's report is not valid JSON: " + boundedProbeErr(err, out)
	}
	if !report.Passed {
		if report.Reason != "" {
			return false, report.Reason
		}
		return false, "the health probe reported failure"
	}
	if report.SchemaVersion != int64(spec.MigrationTo) {
		return false, fmt.Sprintf("the landed schema %d does not match the declared range target %d", report.SchemaVersion, spec.MigrationTo)
	}
	return true, "pass"
}

func boundedProbeErr(err error, out []byte) string {
	s := err.Error()
	if len(out) > 0 {
		clip := string(out)
		if len(clip) > 200 {
			clip = clip[:200]
		}
		s += "; output: " + clip
	}
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// updateAuditor adapts the delivered tamper-evident audit chain (C2-15) to
// the machine's Auditor port: identifiers, reasons, and version references
// only — never key material or credentials (C5-13 §9.1).
type updateAuditor struct {
	audit         *security.AuditService
	correlationID string
}

// Record implements updater.Auditor.
func (a *updateAuditor) Record(_ context.Context, e updater.Event) {
	rec := security.AuditRecord{
		SchemaVersion:   security.AuditSchemaVersion,
		Actor:           "core-updater",
		Action:          e.Name,
		Scope:           "core:update",
		PolicyDecision:  firstNonEmptyCmd(e.Reason, "info"),
		PolicyReference: e.Version,
		Outcome:         outcomeWord(e.Reason),
		CorrelationID:   a.correlationID,
	}
	_ = a.audit.Record(rec) // the audit service's own bounded-buffer semantics govern delivery failures
}

func outcomeWord(reason string) string {
	if reason == "" || reason == "ok" || reason == "pass" {
		return "success"
	}
	return "failure"
}

func firstNonEmptyCmd(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// cmdUpdate is the `uawp update` entry point: check | apply | status |
// health-probe (the last is the internal S8 mechanism, not a user workflow).
// parseArgs hands the command's words WITHOUT the command itself, so
// ordered[0] is the subcommand.
func cmdUpdate(ctx context.Context, flags cliFlags, out *printer, ordered []string) int {
	sub := ""
	if len(ordered) > 0 {
		sub = ordered[0]
	}
	switch sub {
	case "check", "apply":
		return cmdUpdateCheckApply(ctx, flags, out, sub == "apply")
	case "status":
		return cmdUpdateStatus(flags, out)
	case "health-probe":
		rest := []string{}
		if len(ordered) > 1 {
			rest = ordered[1:]
		}
		return cmdUpdateHealthProbe(ctx, flags, out, rest)
	default:
		fmt.Fprintf(os.Stderr, "uawp: unknown update subcommand %q (expected check, apply, or status)\n", sub)
		out.text(out.loc.Text(runtime.KeyHelpUsage))
		return int(runtime.ExitFailure)
	}
}

// cmdUpdateCheckApply runs the machine: check = S1/S2 only (never downloads);
// apply = the caller's explicit consent for the full T1–T10 cycle.
func cmdUpdateCheckApply(ctx context.Context, flags cliFlags, out *printer, apply bool) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	corr := errs.NewCorrelationID()
	cfg, f := runtime.LoadEffectiveConfig(corr, ds.Config, flags.overrides, os.Getenv)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	target, err := targetRowForPlatform(goruntime.GOOS, goruntime.GOARCH)
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitFailure)
	}
	// The data and runtime classes are prepared before the machine acquires
	// the ownership (the delivered composition-root creation discipline; a
	// fresh install has empty classes).
	for _, dir := range []string{ds.Data, ds.Runtime} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			out.text("update: prepare the data directories: " + err.Error())
			return int(runtime.ExitPermission)
		}
	}
	layout, err := buildUpdateLayout(ds)
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitInternal)
	}
	pinned, err := loadPinnedRoot(layout.StateDir)
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitFailure)
	}
	wired := newUpdateEngine(updateEngineParams{
		cfg:           cfg,
		ds:            ds,
		layout:        layout,
		pinned:        pinned,
		target:        target,
		correlationID: corr,
		installed:     version,
	})
	defer wired.locker.release()

	reconciled, err := wired.engine.ReconcileAtStartup(ctx)
	if err != nil {
		out.text("update: startup reconciliation failed: " + err.Error())
		return int(runtime.ExitFailure)
	}
	// The reconciliation's own resolution is part of the honest report: a
	// resolved mid-flight update or a completed revert is visible, never silent.
	if reconciled.State != "clean" {
		out.text("update: startup reconciliation: " + reconciled.State + " (" + string(reconciled.Reason) + ") " + reconciled.Detail)
	}

	// The store opens only after the machine owns the data directory (the
	// delivered lazy discipline); it backs the audit chain. The C5-09
	// backup adapter opens its own short-lived connection at S5.
	if err := wired.store.connect(ctx, version); err != nil {
		out.text("update: open the store: " + err.Error())
		return int(runtime.ExitInternal)
	}
	defer wired.store.close()
	defer wired.audit.Flush()

	var outcome updater.Outcome
	if apply {
		outcome, err = wired.engine.RunUpdate(ctx)
	} else {
		outcome, err = wired.engine.CheckForUpdate(ctx)
	}
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitInternal)
	}
	outUpdateOutcome(out, outcome)
	switch outcome.State {
	case updater.StateNoUpdate, updater.StateUpdatedStable:
		return int(runtime.ExitOK)
	default:
		return int(runtime.ExitFailure)
	}
}

// updateEngineParams bundles everything the composition root computes before
// the engine exists (tests inject their own values instead of process state).
type updateEngineParams struct {
	cfg           config.Config
	ds            platform.DirSet
	layout        updater.Layout
	pinned        updater.PinnedRoot
	target        updateTarget
	correlationID string
	installed     string
}

// wiredUpdateEngine is the assembled root: the machine plus the ports it
// needs from this environment.
type wiredUpdateEngine struct {
	engine *updater.Engine
	locker *ownershipLocker
	store  *lazyStore
	audit  *security.AuditService
}

// newUpdateEngine wires the production ports. The endpoint's absence is not
// a wiring error: the fetcher is still wired and its refusal (fetch_failed:
// no endpoint configured) is the honest typed outcome.
func newUpdateEngine(p updateEngineParams) *wiredUpdateEngine {
	locker := &ownershipLocker{ds: p.ds, dataDir: p.ds.Data, version: p.installed}
	store := &lazyStore{opts: buildOptions(p.cfg, p.ds.Data)}
	auditSvc := security.NewAuditService(lazyAuditStore{l: store}, p.cfg.AuditPendingMax, uint64(p.cfg.AuditCheckpointInterval))

	w := &wiredUpdateEngine{locker: locker, store: store, audit: auditSvc}
	e := &updater.Engine{
		InstalledVersion: p.installed,
		TargetID:         p.target.id,
		PinnedRoot:       p.pinned,
		Layout:           p.layout,
		Clock:            time.Now,
		Locker:           locker,
		Auditor:          &updateAuditor{audit: auditSvc, correlationID: p.correlationID},
	}
	if fetcher, err := newHTTPMetadataFetcher(p.cfg.UpdateEndpointURL); err == nil {
		e.Fetcher = fetcher
		e.Downloader = &httpArtifactDownloader{fetcher: fetcher, target: p.target}
		e.SupplyChain = &channelBindingResolver{fetcher: fetcher}
	} else {
		// An unparseable endpoint (impossible through the validated config,
		// possible through direct construction) wires the honest refusing
		// fetcher so the machine's typed refusal records the cause.
		e.Fetcher = refusingFetcher{cause: err}
	}
	// The C5-13 artifact-signature layer is REQUIRED and BLOCKED: the wired
	// implementation refuses every update (never nil, never a pass-through).
	e.ArtifactSig = blockedSignatureVerifier{}
	// The activator is the per-OS implementation of the same port.
	e.Activator = updater.DefaultActivator()
	e.Health = &probeHealthChecker{binaryPath: p.layout.InstallPath, dataDir: p.ds.Data, locker: locker}
	e.Backupper = &storageBackupper{opts: store.opts, coreVersion: p.installed, correlationID: p.correlationID}
	w.engine = e
	return w
}

// refusingFetcher reports a configuration-caused unavailability as the
// machine's typed fetch failure.
type refusingFetcher struct{ cause error }

// Fetch implements updater.MetadataFetcher.
func (f refusingFetcher) Fetch(_ context.Context) (updater.MetadataBundle, error) {
	return updater.MetadataBundle{}, fmt.Errorf("no update endpoint is configured: %w", f.cause)
}

// outUpdateOutcome renders one machine outcome in the selected form.
func outUpdateOutcome(out *printer, o updater.Outcome) {
	if out.json {
		out.report(o)
		return
	}
	switch {
	case o.State == updater.StateNoUpdate && strings.HasPrefix(o.Detail, "an update to "):
		out.text(strings.ReplaceAll(out.loc.Text(runtime.KeyUpdateAvailable), "{{version}}", o.Version))
	case o.State == updater.StateNoUpdate:
		out.text(out.loc.Text(runtime.KeyUpdateCurrent))
	default:
		line := out.loc.Text(runtime.KeyUpdateOutcome)
		line = strings.ReplaceAll(line, "{{state}}", o.State)
		line = strings.ReplaceAll(line, "{{reason}}", string(o.Reason))
		line = strings.ReplaceAll(line, "{{version}}", o.Version)
		line = strings.ReplaceAll(line, "{{detail}}", o.Detail)
		out.text(line)
	}
}

// cmdUpdateStatus reports the persisted update state (both forms from the
// same report object — C2-07 §5).
func cmdUpdateStatus(flags cliFlags, out *printer) int {
	ds, f := resolvePaths(flags)
	if f != nil {
		out.failure(f)
		return int(f.Exit)
	}
	layout, err := buildUpdateLayout(ds)
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitInternal)
	}
	st, j, err := updater.ReadStatus(layout)
	if err != nil {
		out.text("update: " + err.Error())
		return int(runtime.ExitFailure)
	}
	if out.json {
		report := struct {
			State   updater.ClientState `json:"state"`
			Journal *updater.Journal    `json:"journal,omitempty"`
		}{State: st, Journal: j}
		out.report(report)
		return int(runtime.ExitOK)
	}
	lkg := st.LastKnownGood.Version
	if lkg == "" {
		lkg = "(none recorded)"
	}
	paused := out.loc.Text(runtime.KeyWordNo)
	if st.Paused {
		paused = st.PauseReason
	}
	line := out.loc.Text(runtime.KeyUpdateStatusLine)
	line = strings.ReplaceAll(line, "{{lkg}}", lkg)
	line = strings.ReplaceAll(line, "{{paused}}", paused)
	line = strings.ReplaceAll(line, "{{metadata}}", strconv.FormatUint(st.HighestSeenMetadataVersion, 10))
	out.text(line)
	if j != nil {
		out.text("update: mid-flight journal present: phase=" + j.Phase + " " + j.OldVersion + " → " + j.NewVersion)
	}
	return int(runtime.ExitOK)
}

// cmdUpdateHealthProbe is the internal S8 mechanism the probeHealthChecker
// spawns: a real startup of the swapped-in binary — acquire the data
// directory, apply migrations transactionally, answer the diagnostics probe,
// report. It is not a user workflow; its JSON is the machine's evidence.
func cmdUpdateHealthProbe(ctx context.Context, flags cliFlags, out *printer, rest []string) int {
	schemaTo := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == "--schema-to" && i+1 < len(rest) {
			n, err := strconv.Atoi(rest[i+1])
			if err != nil {
				out.text(`{"passed":false,"reason":"the --schema-to flag is not an integer"}`)
				return int(runtime.ExitFailure)
			}
			schemaTo = n
			i++
		}
	}
	if schemaTo < 0 {
		out.text(`{"passed":false,"reason":"the health probe requires --schema-to"}`)
		return int(runtime.ExitFailure)
	}
	ds, f := resolvePaths(flags)
	if f != nil {
		out.text(`{"passed":false,"reason":"the health probe could not resolve the data directory"}`)
		return int(runtime.ExitFailure)
	}
	corr := errs.NewCorrelationID()
	cfg, f := runtime.LoadEffectiveConfig(corr, ds.Config, flags.overrides, os.Getenv)
	if f != nil {
		out.text(`{"passed":false,"reason":"the health probe could not load the configuration"}`)
		return int(runtime.ExitFailure)
	}
	// The data and runtime classes are prepared before the probe acquires
	// the ownership (the delivered composition-root creation discipline).
	for _, dir := range []string{ds.Data, ds.Runtime} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			out.text(`{"passed":false,"reason":"the health probe could not prepare the data directories"}`)
			return int(runtime.ExitPermission)
		}
	}
	own, _, err := platform.AcquireOwnership(ds, ds.Data, version)
	if err != nil {
		out.text(`{"passed":false,"reason":"the data-directory lock is held elsewhere"}`)
		return int(runtime.ExitFailure)
	}
	defer own.Release()
	store := &lazyStore{opts: buildOptions(cfg, ds.Data)}
	if err := store.connect(ctx, version); err != nil {
		out.text(`{"passed":false,"reason":"the database did not open under the new binary","detail":` + jsonString(storeConnectDetail(err)) + `}`)
		return int(runtime.ExitFailure)
	}
	defer store.close()
	schema, integrity, err := storage.ProbeReadonly(ctx, store.opts.Path)
	report := healthProbeReport{
		SchemaVersion: schema,
		Version:       version,
	}
	if err != nil {
		report.Reason = "the diagnostics probe failed: " + bounded(err)
		out.report(report)
		return int(runtime.ExitFailure)
	}
	report.Detail = "integrity=" + integrity
	if schema != int64(schemaTo) {
		report.Reason = fmt.Sprintf("the landed schema %d does not match the declared range target %d", schema, schemaTo)
		out.report(report)
		return int(runtime.ExitFailure)
	}
	report.Passed = true
	out.report(report)
	return int(runtime.ExitOK)
}

func storeConnectDetail(err error) string {
	s := err.Error()
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
