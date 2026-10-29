package remotestore

// C5-10 measured-evidence harness for the D-28 W6 shape (CR-5.1
// sub-milestone 3): the LIBRARY-LEVEL object-store put/get round-trip
// (launch-matrix row 14 — excluded from launch claims; library-level
// diagnostics only). It runs against the project's OWN local MinIO live
// target via the throwaway-server pattern and is SKIPPED BY DEFAULT when
// REMOTESTORE_TEST_S3_* is unset (recorded Not Run for that run, never a
// pass) — the same discipline as the C4-36 §4 fixtures above.
//
// Method binding (D-28 Option A): timing WITHOUT -race; every number is
// **Proposed — measured, pending review**; measure-first (no threshold —
// row 14 carries none).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// percentilesC510 reports median/p50/p95 (ms) over the harness's
// within-run samples as benchmark metrics. Every value is Proposed —
// measured, pending review.
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

// BenchmarkW6BlobPutGet is the D-28 W6 OPTIONAL library-level shape:
// put/get round-trip with digest verification against the local live
// MinIO target. Skips (Not Run) when the live target is not configured.
func BenchmarkW6BlobPutGet(b *testing.B) {
	s := testStoreWithBlob(b)
	ctx := context.Background()
	ws := uid("ws-w6")
	seedWorkspace(b, s, ws)
	content := []byte("karyora W6 round-trip fixture " + fmt.Sprint(runStamp))

	b.ResetTimer()
	putSamples := make([]float64, 0, b.N)
	getSamples := make([]float64, 0, b.N)
	digest := ""
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		res, err := s.PutObject(ctx, ws, content)
		if err != nil {
			b.Fatalf("put: %v", err)
		}
		digest = res.Digest
		putSamples = append(putSamples, float64(time.Since(t0).Microseconds())/1000)

		t0 = time.Now()
		got, err := s.GetObject(ctx, ws, digest, 1<<20)
		if err != nil {
			b.Fatalf("get: %v", err)
		}
		if !bytes.Equal(got, content) {
			b.Fatal("round-trip bytes differ")
		}
		getSamples = append(getSamples, float64(time.Since(t0).Microseconds())/1000)
	}
	b.StopTimer()
	percentilesC510(b, "put", putSamples)
	percentilesC510(b, "digest-verified-get", getSamples)
}

// TestFIO1BlobEndpointUnreachable is the FI-O1 FC-4 residual injectable
// here: the object endpoint becomes unreachable (connection refused) — the
// put fails typed, NOTHING dangles (no reference row, no false success),
// and the same content lands on the live target (the recovery leg).
// Skips with the object fixtures when no live target is configured.
func TestFIO1BlobEndpointUnreachable(t *testing.T) {
	if os.Getenv("REMOTESTORE_TEST_S3_ENDPOINT") == "" {
		t.Skip("REMOTESTORE_TEST_S3_ENDPOINT not set — object fixtures Not Run in this run")
	}
	s := testStoreWithBlob(t) // live PostgreSQL + live blob store
	ctx := context.Background()
	ws := uid("ws-dead-endpoint")
	seedWorkspace(t, s, ws)

	// A store attached to an endpoint with no listener (dependency loss).
	dead, err := OpenBlobStore(ctx, BlobConfig{
		Endpoint: "127.0.0.1:1", AccessKey: "x", SecretKey: "y",
		Bucket: "karyora-objects", Secure: false,
	})
	if err != nil {
		t.Fatalf("open dead blob store: %v", err)
	}
	s2 := testStore(t)
	s2.Blob = dead
	content := []byte("unreachable-endpoint fixture " + fmt.Sprint(runStamp))
	_, err = s2.PutObject(ctx, ws, content)
	if err == nil {
		t.Fatal("a put against an unreachable endpoint must fail")
	}
	if errors.Is(err, ErrReferenceCommitFailed) {
		// The typed orphan path — the mark pass must still find nothing
		// dangling, because no bytes and no reference landed.
		if marked, err := s2.MarkStoreOrphans(ctx); err != nil || marked != 0 {
			t.Fatalf("mark after unreachable put: marked=%d err=%v", marked, err)
		}
	}
	var rows int
	if err := s2.pool.QueryRow(ctx,
		`SELECT count(*) FROM object_refs WHERE workspace_id = $1 AND digest = $2`,
		ws, DigestOf(content)).Scan(&rows); err != nil {
		t.Fatalf("probe reference rows: %v", err)
	}
	if rows != 0 {
		t.Fatalf("%d reference rows dangle after the unreachable put", rows)
	}
	// Recovery leg: the live target takes the same content.
	if _, err := s.PutObject(ctx, ws, content); err != nil {
		t.Fatalf("recovery put on the live target: %v", err)
	}
	if got, err := s.GetObject(ctx, ws, DigestOf(content), 1<<20); err != nil || !bytes.Equal(got, content) {
		t.Fatalf("recovery get: %v", err)
	}
}
