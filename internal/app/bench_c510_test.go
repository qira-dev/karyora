package app

// C5-10 measured-evidence harnesses for the durable-slice shapes of the
// CR-5.1 sub-milestone 3 (under the owner's 2026-10-29 Go):
//
//   - BenchmarkW3JobLifecycle — the D-28 W3 shape: the delivered
//     relay+storage slice (enqueue → claim → run → atomic commit → result)
//     through internal/app (the Core API surface) with the MOCK provider
//     job class (no external provider call — provider latency is excluded
//     per the D-28 W3 boundary). Reports enqueue→start and end-to-end
//     p50/p95 within each run.
//   - BenchmarkW3InterruptedAttemptRecovery — the interrupted-attempt
//     recovery timing of the same shape: attempt 1 holds a lease and never
//     completes (the crash window), the recovery sweep quarantines it, and
//     the delivered settlement path lands the job on a fresh attempt.
//   - BenchmarkW7ResourceCeilings — the W7 shape: process memory/CPU
//     ceilings under a fixed batch of settled jobs (RSS, Go heap/sys, GC
//     cycles, CPU seconds per completed job).
//
// Method binding (D-28 Option A, accepted 2026-10-29): this machine pinned
// (Intel i5-6500 4 cores, 15 GiB RAM, NVMe ext4, kernel 7.0.0-42-generic,
// go1.25.14); timing WITHOUT -race; >= 5 full runs; medians and p50/p95;
// every number is **Proposed — measured, pending review** — never a
// threshold verdict. Benchmarks run only under -bench; the normal suite
// (including CI) executes none of them.

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qira-dev/karyora/internal/aijobrelay"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
)

// percentiles reports median/p50/p95 (ms) over the harness's within-run
// samples as benchmark metrics. Every value is Proposed — measured,
// pending review.
func percentilesC510(b *testing.B, name string, samples []float64) {
	if len(samples) == 0 {
		return
	}
	sorted := append([]float64(nil), samples...)
	sort.Float64s(sorted)
	pick := func(p float64) float64 {
		return sorted[int(p*float64(len(sorted)-1))]
	}
	var sum float64
	for _, s := range samples {
		sum += s
	}
	b.ReportMetric(sum/float64(len(samples)), name+"-mean-ms")
	b.ReportMetric(pick(0.50), name+"-p50-ms")
	b.ReportMetric(pick(0.95), name+"-p95-ms")
}

// benchWorker is the W3 worker capability: the mock provider is the only
// wired provider, so the measured job class is the mock text-completion
// job — no external provider call exists in the measured path.
func benchWorker(id string) aijobrelay.WorkerCapability {
	return aijobrelay.WorkerCapability{
		WorkerID: id, ProviderIDs: []string{dprovider.ProviderMock}, MaxMemoryMB: 1024,
	}
}

// BenchmarkW3JobLifecycle is the D-28 W3 shape through the delivered
// slice: submit → claim (enqueue→start) → the delivered relay handler
// (compile → gated provider call → atomic commit) → result read. The job
// class is the MOCK-provider text-completion job (fixture classification;
// provider latency excluded per D-28).
func BenchmarkW3JobLifecycle(b *testing.B) {
	env := newTestEnv(b)
	handler := NewRelayHandler(env.deps)
	worker := benchWorker("bench-w3")
	ctx := env.ctx()
	nonce := time.Now().UnixNano()

	enqSamples := make([]float64, 0, b.N)
	e2eSamples := make([]float64, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := baseRequest()
		req.IdempotencyKey = fmt.Sprintf("idem-w3-%d-%d", nonce, i)
		t0 := time.Now()
		sub, err := env.slice.Submit(ctx, req)
		if err != nil {
			b.Fatalf("submit: %v", err)
		}
		claim, ok, err := env.store.ClaimNext(ctx, worker, time.Minute)
		if err != nil || !ok {
			b.Fatalf("claim: %v %v", ok, err)
		}
		enqSamples = append(enqSamples, float64(time.Since(t0).Microseconds())/1000)
		commit, err := handler.Handle(ctx, *claim)
		if err != nil {
			b.Fatalf("handler: %v", err)
		}
		if err := env.store.CommitAttempt(ctx, commit); err != nil {
			b.Fatalf("commit: %v", err)
		}
		job, err := env.store.LoadJob(context.Background(), sub.Job.JobID)
		if err != nil || job.State != aijobrelay.JobSucceeded {
			b.Fatalf("job did not succeed: %+v %v", job, err)
		}
		if _, err := env.slice.Result(ctx, sub.Job.JobID); err != nil {
			b.Fatalf("result read: %v", err)
		}
		e2eSamples = append(e2eSamples, float64(time.Since(t0).Microseconds())/1000)
	}
	b.StopTimer()
	percentilesC510(b, "enqueue-to-start", enqSamples)
	percentilesC510(b, "end-to-end", e2eSamples)
}

// BenchmarkW3InterruptedAttemptRecovery measures the recovery of an
// interrupted attempt: attempt 1 claims with a short lease and never
// completes (the crash window), the delivered recovery sweep quarantines
// it, and RunToSettlement — the delivered settlement path — lands the job
// on a fresh attempt. The metric is sweep-start → settled-success.
func BenchmarkW3InterruptedAttemptRecovery(b *testing.B) {
	env := newTestEnv(b)
	worker := benchWorker("bench-w3-interrupted")
	ctx := env.ctx()
	nonce := time.Now().UnixNano()

	samples := make([]float64, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := baseRequest()
		req.IdempotencyKey = fmt.Sprintf("idem-w3r-%d-%d", nonce, i)
		sub, err := env.slice.Submit(ctx, req)
		if err != nil {
			b.Fatalf("submit: %v", err)
		}
		// Attempt 1: claimed, never completed — the interrupted attempt.
		if _, ok, err := env.store.ClaimNext(ctx, worker, 25*time.Millisecond); err != nil || !ok {
			b.Fatalf("interrupted claim: %v %v", ok, err)
		}
		time.Sleep(40 * time.Millisecond) // the lease expires (the crash window)
		t0 := time.Now()
		requeued, err := env.store.ReleaseExpiredLeases(ctx, time.Now())
		if err != nil {
			b.Fatalf("recovery sweep: %v", err)
		}
		if len(requeued) != 1 || requeued[0].JobID != sub.Job.JobID {
			b.Fatalf("the interrupted attempt was not quarantined: %+v", requeued)
		}
		final, err := RunToSettlement(ctx, env.deps, "bench-w3-recovery", 1, sub.Job.JobID, 64)
		if err != nil || final.State != aijobrelay.JobSucceeded {
			b.Fatalf("recovery did not settle the job: %+v %v", final, err)
		}
		if final.AttemptCount < 2 {
			b.Fatalf("recovery must run on a NEW attempt, got %d", final.AttemptCount)
		}
		samples = append(samples, float64(time.Since(t0).Microseconds())/1000)
	}
	b.StopTimer()
	percentilesC510(b, "interrupted-attempt-recovery", samples)
}

// resourceSample captures the W7 process metrics: VmRSS from /proc (linux
// only), Go heap/sys and GC counters from runtime, and process CPU from
// /proc/self/stat (linux only; utime+stime in clock ticks at 100 Hz).
type resourceSample struct {
	rssMiB    float64
	heapMiB   float64
	sysMiB    float64
	gcCycles  uint32
	cpuTicks  float64
	procTicks float64
}

func readResourceSample() resourceSample {
	var s resourceSample
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	s.heapMiB = float64(ms.HeapAlloc) / (1 << 20)
	s.sysMiB = float64(ms.Sys) / (1 << 20)
	s.gcCycles = ms.NumGC
	if runtime.GOOS == "linux" {
		if raw, err := os.ReadFile("/proc/self/stat"); err == nil {
			// pid (comm) state ppid ... utime(14) stime(15)
			fields := strings.Fields(string(raw))
			utime, _ := strconv.ParseFloat(fields[13], 64)
			stime, _ := strconv.ParseFloat(fields[14], 64)
			s.cpuTicks = utime + stime
		}
		if raw, err := os.ReadFile("/proc/self/status"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "VmRSS:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, _ := strconv.ParseFloat(fields[1], 64)
						s.rssMiB = kb / 1024
					}
				}
			}
		}
	}
	return s
}

// BenchmarkW7ResourceCeilings is the D-28 W7 shape: the process resource
// ceilings of the delivered runtime under a fixed batch of settled jobs
// (the approved load level is the W3 concurrency; this harness measures a
// bounded sequential batch through the delivered slice). Metrics: RSS,
// Go heap/sys, GC cycles, CPU seconds per completed job, wall ms per job.
func BenchmarkW7ResourceCeilings(b *testing.B) {
	// Keep GC behavior deterministic for the report window.
	defer debug.SetGCPercent(debug.SetGCPercent(100))
	env := newTestEnv(b)
	ctx := env.ctx()
	nonce := time.Now().UnixNano()

	const jobs = 50
	before := readResourceSample()
	runtime.GC()
	t0 := time.Now()
	for i := 0; i < jobs; i++ {
		req := baseRequest()
		req.IdempotencyKey = fmt.Sprintf("idem-w7-%d-%d", nonce, i)
		sub, err := env.slice.Submit(ctx, req)
		if err != nil {
			b.Fatalf("submit: %v", err)
		}
		final, err := RunToSettlement(ctx, env.deps, "bench-w7", 2, sub.Job.JobID, 64)
		if err != nil || final.State != aijobrelay.JobSucceeded {
			b.Fatalf("job %d did not succeed: %+v %v", i, final, err)
		}
	}
	wall := time.Since(t0)
	after := readResourceSample()

	b.ReportMetric(after.rssMiB, "rss-mib")
	b.ReportMetric(after.heapMiB, "heap-alloc-mib")
	b.ReportMetric(after.sysMiB, "go-sys-mib")
	b.ReportMetric(float64(after.gcCycles-before.gcCycles), "gc-cycles")
	if after.cpuTicks > 0 || before.cpuTicks > 0 {
		b.ReportMetric((after.cpuTicks-before.cpuTicks)/100/jobs, "cpu-sec-per-job")
	} else {
		b.Log("cpu-sec-per-job: Not Run (non-linux process CPU metrics unavailable in this run)")
	}
	b.ReportMetric(wall.Seconds()*1000/jobs, "wall-ms-per-job")
}
