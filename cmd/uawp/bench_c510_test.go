package main

// C5-10 measured-evidence harnesses (CR-5.1 sub-milestone 3, under the
// owner's 2026-10-29 Go). This file carries the D-28 W1/W2 shapes as Go
// benchmarks over the BUILT CLI binary and the crash-window failure
// injections (the FI-S4/FI-R3 residuals of the C5-05 matrix) executed at
// the REAL process boundary with SIGKILL.
//
// Method binding (D-28 Option A, accepted 2026-10-29): this machine pinned
// (Intel i5-6500 4 cores, 15 GiB RAM, NVMe ext4, kernel 7.0.0-42-generic,
// go1.25.14 linux/amd64); timing WITHOUT -race; >= 5 full runs; medians
// and p50/p95 reported; every number these harnesses produce is
// **Proposed — measured, pending review** — never a threshold verdict.
//
// Benchmarks run only under -bench, so the normal suite (including CI)
// executes only the injection tests below. os/exec is test-only harness
// code in the composition root: the dependency-map rules govern non-test
// imports, and cmd/uawp is the documented composition root.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	"github.com/qira-dev/karyora/internal/platform"
	coreRuntime "github.com/qira-dev/karyora/internal/runtime"
	"github.com/qira-dev/karyora/internal/storage"
)

// benchW2SeedBytes is the W2/W3 seeded-store tier of the approved D-28 v1
// set (Proposed ~100 MB). Overridable per run for tier sweeps.
func benchSeedBytes() int64 {
	if v := os.Getenv("UAWP_BENCH_SEED_BYTES"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 100 << 20
}

func benchSeedObjectSize() int {
	if v := os.Getenv("UAWP_BENCH_SEED_OBJECT_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 1 << 20
}

// benchXDG isolates one harness run's environment (XDG roots under one
// temp dir) so the class directories keep the delivered 0700 posture —
// the same isolation the conformance harness (newHarness) applies. The
// subprocesses inherit the environment. It returns the isolation root.
func benchXDG(tb testing.TB) string {
	tb.Helper()
	tmp := tb.TempDir()
	tb.Setenv("XDG_RUNTIME_DIR", filepath.Join(tmp, "rt"))
	tb.Setenv("HOME", tmp)
	tb.Setenv("XDG_DATA_HOME", filepath.Join(tmp, "data"))
	tb.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	tb.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))
	tb.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	return tmp
}

// benchDataDir creates the harness's data dir with the delivered 0700
// posture explicitly (testing.TempDir creates its directories broader —
// 0777&~umask — which the delivered doctor correctly refuses; the harness
// must reproduce a real install's posture, not fight it).
func benchDataDir(tb testing.TB, root string) string {
	tb.Helper()
	dir := filepath.Join(root, "data0700")
	if err := os.Mkdir(dir, 0o700); err != nil {
		tb.Fatalf("create 0700 data dir: %v", err)
	}
	return dir
}

// moduleRootDir walks up to the go.mod so the harness can build the CLI.
func moduleRootDir(tb testing.TB) string {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

// goCacheHome captures the REAL user home at process init (before any
// harness Setenv overrides HOME) so buildCoreBinary can pin the go build
// caches outside the harness temp dirs.
var goCacheHome = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}()

// buildCoreBinary builds the delivered CLI surface once per harness run;
// W1/W2 time THIS artifact (the built binary, not the in-process run()).
func buildCoreBinary(tb testing.TB) string {
	tb.Helper()
	bin := filepath.Join(tb.TempDir(), "uawp-c510")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/uawp")
	cmd.Dir = moduleRootDir(tb)
	if goCacheHome != "" {
		// Keep the go caches out of the (overridden) HOME so the harness
		// temp dirs stay clean and removable.
		cmd.Env = append(os.Environ(),
			"GOCACHE="+filepath.Join(goCacheHome, ".cache", "go-build"),
			"GOMODCACHE="+filepath.Join(goCacheHome, "go", "pkg", "mod"))
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		tb.Fatalf("build core binary: %v\n%s", err, out)
	}
	return bin
}

// resolveDataDir resolves the storage-class roots for a --data-dir value
// exactly as the CLI does.
func resolveDataDir(tb testing.TB, dir string) platform.DirSet {
	tb.Helper()
	ds, err := platform.Resolve(&platform.Overrides{DataDir: dir})
	if err != nil {
		tb.Fatalf("resolve data dir: %v", err)
	}
	return ds
}

// productionStoreOptions mirrors buildOptions (the composition root's
// translation of the effective configuration) for harness seeding and the
// crash-window fixtures.
func productionStoreOptions(tb testing.TB, ds platform.DirSet) storage.Options {
	tb.Helper()
	corr := "c510-harness"
	cfg, f := coreRuntime.LoadEffectiveConfig(corr, ds.Config, nil, os.Getenv)
	if f != nil {
		tb.Fatalf("load effective config: %+v", f.Envelope)
	}
	return buildOptions(cfg, ds.Data)
}

// openSharedStore opens the store at a CLI data dir (harness/fixture use).
func openSharedStore(tb testing.TB, dir string) *storage.Store {
	tb.Helper()
	ds := resolveDataDir(tb, dir)
	if err := platform.EnsureLayout(ds); err != nil {
		tb.Fatalf("ensure layout: %v", err)
	}
	st, err := storage.Open(context.Background(), productionStoreOptions(tb, ds))
	if err != nil {
		tb.Fatalf("open store at %s: %v", ds.Data, err)
	}
	return st
}

// shaDigest returns a grammar-valid sha256 digest for fixture content.
func shaDigest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// seedCLIStore fills the CLI's own store to the proposed tier: durable
// CAS-backed objects (referenced rows, so GC never collects them) plus a
// small settled job history.
func seedCLIStore(tb testing.TB, dir string) {
	tb.Helper()
	st := openSharedStore(tb, dir)
	defer st.Close()
	ctx := context.Background()
	objectSize := benchSeedObjectSize()
	count := int(benchSeedBytes() / int64(objectSize))
	payload := make([]byte, objectSize)
	for i := 0; i < count; i++ {
		for j := range payload {
			payload[j] = byte(uint32(i)*31 + uint32(j))
		}
		if _, err := st.PutObject(ctx, payload, "c510.bench", fmt.Sprintf("obj-%d", i)); err != nil {
			tb.Fatalf("seed object %d: %v", i, err)
		}
	}
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("job-seed-%d", i)
		if _, err := st.SubmitJob(ctx, aijobrelay.Job{
			JobID: id, IdempotencyKey: "key-" + id, InputDigest: shaDigest(id),
			State: aijobrelay.JobCreated, Version: 1,
			CorrelationID: "corr-" + id, CreatedAt: time.Now().UTC(),
		}); err != nil {
			tb.Fatalf("seed job %d: %v", i, err)
		}
	}
}

// startCore starts the built binary and returns the process.
func startCore(tb testing.TB, bin, dir string) *exec.Cmd {
	tb.Helper()
	cmd := exec.Command(bin, "start", "--json", "--data-dir", dir)
	if err := cmd.Start(); err != nil {
		tb.Fatalf("start core: %v", err)
	}
	return cmd
}

// awaitReady polls `status --json` until the core reports live+ready and
// returns the launch→ready wall time.
func awaitReady(tb testing.TB, bin, dir string, deadline time.Duration) (time.Duration, error) {
	tb.Helper()
	t0 := time.Now()
	deadlineAt := time.Now().Add(deadline)
	for time.Now().Before(deadlineAt) {
		out, err := exec.Command(bin, "status", "--json", "--data-dir", dir).Output()
		if err == nil {
			var report struct {
				Live   bool `json:"live"`
				Health struct {
					Process struct {
						State string `json:"state"`
					} `json:"process"`
				} `json:"health"`
			}
			if json.Unmarshal(out, &report) == nil && report.Live && report.Health.Process.State == "ready" {
				return time.Since(t0), nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return 0, fmt.Errorf("core at %s did not reach ready within %s", dir, deadline)
}

// stopGracefully sends SIGTERM (the same graceful-stop request a signal
// carries) and waits for the drain to complete.
func stopGracefully(tb testing.TB, cmd *exec.Cmd) {
	tb.Helper()
	if cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		tb.Fatalf("signal core: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		tb.Fatal("core did not drain within 30s of SIGTERM")
	}
}

// processRSSMiB reads VmRSS from /proc (linux only; other GOOS report 0
// and the metric is recorded as Not Run for that run).
func processRSSMiB(pid int) float64 {
	if goruntime.GOOS != "linux" {
		return 0
	}
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, err := strconv.ParseFloat(fields[1], 64)
				if err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

// latencyStats reports median/p50/p95 (ms) over the harness's within-run
// samples as benchmark metrics. Every value is Proposed — measured,
// pending review.
func latencyStats(b *testing.B, samples []float64) {
	if len(samples) == 0 {
		return
	}
	sorted := append([]float64(nil), samples...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	pick := func(p float64) float64 {
		idx := int(p * float64(len(sorted)-1))
		return sorted[idx]
	}
	var sum float64
	for _, s := range samples {
		sum += s
	}
	b.ReportMetric(sum/float64(len(samples)), "mean-ms")
	b.ReportMetric(pick(0.50), "p50-ms")
	b.ReportMetric(pick(0.95), "p95-ms")
}

// BenchmarkW1ColdStartStatus is the D-28 W1 shape: process launch to first
// correct command output on an initialized local data dir. Each iteration
// launches the built binary cold (a fresh process each time).
func BenchmarkW1ColdStartStatus(b *testing.B) {
	root := benchXDG(b)
	bin := buildCoreBinary(b)
	dir := benchDataDir(b, root)
	// Initialize the data dir exactly as the delivered lifecycle does:
	// one start → ready → graceful drain cycle creates and migrates the
	// store; every timed iteration then launches on the initialized dir.
	stopCmd := startCore(b, bin, dir)
	if _, err := awaitReady(b, bin, dir, 30*time.Second); err != nil {
		_ = stopCmd.Process.Kill()
		b.Fatal(err)
	}
	stopGracefully(b, stopCmd)

	b.ResetTimer()
	samples := make([]float64, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		out, err := exec.Command(bin, "status", "--json", "--data-dir", dir).Output()
		if err != nil {
			b.Fatalf("status: %v\n%s", err, out)
		}
		var report map[string]any
		if json.Unmarshal(out, &report) != nil {
			b.Fatalf("status output is not machine JSON: %s", out)
		}
		if _, ok := report["live"]; !ok {
			b.Fatalf("status output lacks the live key: %s", out)
		}
		samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
	}
	b.StopTimer()
	latencyStats(b, samples)
}

// BenchmarkW1StartToReady measures the delivered lifecycle ordering:
// process start → ready on the initialized data dir, then the graceful
// drain. It also records the running process's idle RSS (the W7 idle
// ceiling at the real process boundary).
func BenchmarkW1StartToReady(b *testing.B) {
	root := benchXDG(b)
	bin := buildCoreBinary(b)
	dir := benchDataDir(b, root)
	stopCmd := startCore(b, bin, dir)
	if _, err := awaitReady(b, bin, dir, 30*time.Second); err != nil {
		_ = stopCmd.Process.Kill()
		b.Fatal(err)
	}
	stopGracefully(b, stopCmd)

	readySamples := make([]float64, 0, b.N)
	rssSamples := make([]float64, 0, b.N)
	for i := 0; i < b.N; i++ {
		cmd := startCore(b, bin, dir)
		ready, err := awaitReady(b, bin, dir, 30*time.Second)
		if err != nil {
			_ = cmd.Process.Kill()
			b.Fatal(err)
		}
		readySamples = append(readySamples, float64(ready.Microseconds())/1000)
		if rss := processRSSMiB(cmd.Process.Pid); rss > 0 {
			rssSamples = append(rssSamples, rss)
		}
		stopGracefully(b, cmd)
	}
	latencyStats(b, readySamples)
	if len(rssSamples) > 0 {
		var sum float64
		for _, s := range rssSamples {
			sum += s
		}
		b.ReportMetric(sum/float64(len(rssSamples)), "idle-rss-mib")
	} else {
		b.Log("idle-rss-mib: Not Run (non-linux process metrics unavailable in this run)")
	}
}

// BenchmarkW2CommandLatency is the D-28 W2 shape: repeated local CLI
// commands (status, doctor) against the seeded local store (~100 MB tier),
// one cold process per invocation.
func BenchmarkW2CommandLatency(b *testing.B) {
	root := benchXDG(b)
	bin := buildCoreBinary(b)
	dir := benchDataDir(b, root)
	stopCmd := startCore(b, bin, dir)
	if _, err := awaitReady(b, bin, dir, 30*time.Second); err != nil {
		_ = stopCmd.Process.Kill()
		b.Fatal(err)
	}
	stopGracefully(b, stopCmd)
	seedCLIStore(b, dir)

	for _, command := range []string{"status", "doctor"} {
		b.Run(command, func(b *testing.B) {
			b.ResetTimer()
			samples := make([]float64, 0, b.N)
			for i := 0; i < b.N; i++ {
				t0 := time.Now()
				out, err := exec.Command(bin, command, "--json", "--data-dir", dir).Output()
				if err != nil {
					b.Fatalf("%s: %v\n%s", command, err, out)
				}
				samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
			}
			b.StopTimer()
			latencyStats(b, samples)
		})
	}
}

// ---- Crash-window failure injections (FI-S4/FI-R3 residuals) ----

// The re-exec helper env names (fixture-only; nothing product-facing).
const (
	envCrashHelper  = "UAWP_C510_CRASH_HELPER"
	envCrashWindow  = "UAWP_C510_CRASH_WINDOW"
	envCrashDataDir = "UAWP_C510_CRASH_DATA_DIR"
	envCrashJobID   = "UAWP_C510_CRASH_JOB_ID"
	envCrashLeaseMS = "UAWP_C510_CRASH_LEASE_MS"
)

// TestC510CrashHelper is the child process of the crash-window fixtures:
// it opens the shared store, drives one Job to the requested crash window
// (queued, or claimed with a live lease), announces readiness on stdout,
// and blocks until the parent SIGKILLs it — a real process death with no
// graceful drain, no checkpoint request, and no lock release.
func TestC510CrashHelper(t *testing.T) {
	if os.Getenv(envCrashHelper) == "" {
		t.Skip("crash-window helper process")
	}
	dir := os.Getenv(envCrashDataDir)
	jobID := os.Getenv(envCrashJobID)
	window := os.Getenv(envCrashWindow)
	st := openSharedStore(t, dir)
	ctx := context.Background()
	job := aijobrelay.Job{
		JobID: jobID, IdempotencyKey: "key-" + jobID, InputDigest: shaDigest(jobID),
		State: aijobrelay.JobCreated, Version: 1,
		CorrelationID: "corr-" + jobID, CreatedAt: time.Now().UTC(),
	}
	if _, err := st.SubmitJob(ctx, job); err != nil {
		fmt.Fprintf(os.Stderr, "crash helper: submit: %v\n", err)
		os.Exit(1)
	}
	if window == "claimed" {
		leaseMS, _ := strconv.Atoi(os.Getenv(envCrashLeaseMS))
		if leaseMS <= 0 {
			leaseMS = 1000
		}
		claim, ok, err := st.ClaimNext(ctx,
			aijobrelay.WorkerCapability{WorkerID: "crash-worker", MaxMemoryMB: 1024},
			time.Duration(leaseMS)*time.Millisecond)
		if err != nil || !ok {
			fmt.Fprintf(os.Stderr, "crash helper: claim: ok=%v err=%v\n", ok, err)
			os.Exit(1)
		}
		_ = claim
	}
	fmt.Println("READY")
	select {} // blocked until SIGKILL
}

// runCrashChild starts the re-exec helper, waits for its READY line, then
// SIGKILLs it (FC-1: a real process crash mid-operation).
func runCrashChild(t *testing.T, dir, jobID, window string, leaseMS int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestC510CrashHelper$", "-test.timeout=60s")
	cmd.Env = append(os.Environ(),
		envCrashHelper+"=1",
		envCrashWindow+"="+window,
		envCrashDataDir+"="+dir,
		envCrashJobID+"="+jobID,
		envCrashLeaseMS+"="+strconv.Itoa(leaseMS),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start crash child: %v", err)
	}
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err != nil {
			ready <- err
			return
		}
		if strings.TrimSpace(line) != "READY" {
			ready <- fmt.Errorf("unexpected child line %q", line)
			return
		}
		ready <- nil
	}()
	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("crash child never reached the window: %v", err)
		}
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("crash child did not announce readiness within 30s")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("SIGKILL the crash child: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("the crash child exited cleanly — the kill did not land")
	}
}

// TestFIS4CrashWindowQueuedRestart injects a process crash while the Job
// sits Queued (submitted, never claimed) and asserts the restart recovery:
// the durable truth survives (no loss, no false success) and the delivered
// path settles the job on a fresh attempt.
func TestFIS4CrashWindowQueuedRestart(t *testing.T) {
	dir := t.TempDir()
	st := openSharedStore(t, dir)
	ctx := context.Background()
	jobID := fmt.Sprintf("job-crash-queued-%d", time.Now().UnixNano())
	st.Close()

	runCrashChild(t, dir, jobID, "queued", 0)

	// The restart path mirrors the runner's startup steps 4–5: reconcile,
	// then the recovery generation bumps (the fencing token).
	reopened := openSharedStore(t, dir)
	defer reopened.Close()
	if err := reopened.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile after crash: %v", err)
	}
	gen, err := reopened.BumpGeneration(ctx)
	if err != nil || gen == 0 {
		t.Fatalf("recovery generation after crash: %d %v", gen, err)
	}
	job, err := reopened.LoadJob(ctx, jobID)
	if err != nil {
		t.Fatalf("load job after crash: %v", err)
	}
	if job.State != aijobrelay.JobQueued {
		t.Fatalf("queued job state after crash = %s, want queued (no loss, no rewrite)", job.State)
	}
	if job.ResultDigest != "" {
		t.Fatal("an unclaimed job must never carry a result after a crash")
	}
	if _, err := reopened.LoadCurrentAttempt(ctx, jobID); err == nil {
		t.Fatal("a queued job must have no attempt after a crash")
	}
	// The delivered path settles it: claim + commit land exactly once.
	claim, ok, err := reopened.ClaimNext(ctx,
		aijobrelay.WorkerCapability{WorkerID: "restart-worker", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim after crash-restart: %v %v", ok, err)
	}
	if claim.Job.JobID != jobID {
		t.Fatalf("claim returned %s, want %s", claim.Job.JobID, jobID)
	}
	commit := aijobrelay.Commit{
		JobID: jobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: claim.Attempt.WorkerID, LeaseID: claim.Attempt.LeaseID,
		LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:    aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: shaDigest("recovered result"), ReceiptID: "rc-crash-queued",
		CorrelationID: job.CorrelationID,
	}
	if err := reopened.CommitAttempt(ctx, commit); err != nil {
		t.Fatalf("commit after crash-restart: %v", err)
	}
	final, err := reopened.LoadJob(ctx, jobID)
	if err != nil || final.State != aijobrelay.JobSucceeded || final.ReceiptID != "rc-crash-queued" {
		t.Fatalf("recovered job: %+v %v", final, err)
	}
}

// TestFIS4CrashWindowMidAttemptRestart injects a process crash while an
// Attempt holds a live lease mid-flight and asserts the FI-R3 recovery:
// lease expiry requeues honestly (no false success), the quarantined
// attempt's commit is refused after the fencing bump, and a fresh attempt
// settles the job.
func TestFIS4CrashWindowMidAttemptRestart(t *testing.T) {
	dir := t.TempDir()
	st := openSharedStore(t, dir)
	ctx := context.Background()
	jobID := fmt.Sprintf("job-crash-mid-%d", time.Now().UnixNano())
	st.Close()

	const leaseMS = 700
	runCrashChild(t, dir, jobID, "claimed", leaseMS)

	reopened := openSharedStore(t, dir)
	defer reopened.Close()
	if err := reopened.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile after crash: %v", err)
	}
	gen, err := reopened.BumpGeneration(ctx)
	if err != nil || gen == 0 {
		t.Fatalf("recovery generation after crash: %d %v", gen, err)
	}
	job, err := reopened.LoadJob(ctx, jobID)
	if err != nil {
		t.Fatalf("load job after crash: %v", err)
	}
	if job.State != aijobrelay.JobRunning {
		t.Fatalf("mid-attempt job state after crash = %s, want running (durable truth preserved)", job.State)
	}
	if job.ResultDigest != "" {
		t.Fatal("an interrupted attempt must never carry a result after a crash")
	}
	attempt, err := reopened.LoadCurrentAttempt(ctx, jobID)
	if err != nil || attempt.Outcome != aijobrelay.AttemptRunning {
		t.Fatalf("interrupted attempt after crash: %+v %v", attempt, err)
	}
	staleVersion := job.Version

	// The dead worker's lease expires: the recovery sweep quarantines the
	// interrupted attempt — requeued honestly, fencing version bumped.
	time.Sleep(time.Duration(leaseMS)*time.Millisecond + 100*time.Millisecond)
	requeued, err := reopened.ReleaseExpiredLeases(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("lease recovery sweep: %v", err)
	}
	if len(requeued) != 1 || requeued[0].JobID != jobID || requeued[0].State != aijobrelay.JobQueued {
		t.Fatalf("interrupted attempt was not requeued: %+v", requeued)
	}
	if requeued[0].Version != staleVersion+1 {
		t.Fatalf("requeue did not bump the fencing version: %d → %d", staleVersion, requeued[0].Version)
	}
	// The quarantined attempt's commit is now refused (stale version +
	// stale lease epoch): no false success can survive the crash window.
	stale := aijobrelay.Commit{
		JobID: jobID, JobVersion: staleVersion, AttemptID: attempt.AttemptID,
		WorkerID: attempt.WorkerID, LeaseID: attempt.LeaseID, LeaseEpoch: attempt.LeaseEpoch,
		ToState: aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: shaDigest("ghost result"), ReceiptID: "rc-ghost",
	}
	if err := reopened.CommitAttempt(ctx, stale); err == nil {
		t.Fatal("the quarantined attempt's commit was accepted after the crash — a false success")
	}
	// A fresh attempt settles the job exactly once.
	claim, ok, err := reopened.ClaimNext(ctx,
		aijobrelay.WorkerCapability{WorkerID: "restart-worker", MaxMemoryMB: 1024}, time.Minute)
	if err != nil || !ok || claim.Job.JobID != jobID {
		t.Fatalf("fresh claim after quarantine: %v %v", ok, err)
	}
	commit := aijobrelay.Commit{
		JobID: jobID, JobVersion: claim.Job.Version, AttemptID: claim.Attempt.AttemptID,
		WorkerID: claim.Attempt.WorkerID, LeaseID: claim.Attempt.LeaseID,
		LeaseEpoch: claim.Attempt.LeaseEpoch,
		ToState:    aijobrelay.JobSucceeded, AttemptOutcome: aijobrelay.AttemptSucceeded,
		ResultDigest: shaDigest("recovered result"), ReceiptID: "rc-crash-mid",
		CorrelationID: job.CorrelationID,
	}
	if err := reopened.CommitAttempt(ctx, commit); err != nil {
		t.Fatalf("commit after recovery: %v", err)
	}
	final, err := reopened.LoadJob(ctx, jobID)
	if err != nil || final.State != aijobrelay.JobSucceeded || final.ReceiptID != "rc-crash-mid" {
		t.Fatalf("recovered job: %+v %v", final, err)
	}
}
