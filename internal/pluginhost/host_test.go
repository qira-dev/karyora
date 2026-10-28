package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/sys"

	"github.com/qira-dev/karyora/internal/plugins"
)

// Adversarial enforcement fixtures for C4-25: the quota envelope, the
// per-call authorization contract, cancellation, and fault containment
// are exercised against the REAL runtime boundary with hand-assembled
// minimal WASM modules. Plugin execution is exercised ONLY here; no
// product surface reaches this package (D-14 stays open).

var fixtureNow = time.Date(2026, 10, 29, 14, 0, 0, 0, time.UTC)

// --- minimal WASM assembly (binary encoding, no external tooling) ---

var wasmHeader = []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00}

const vtI32 = byte(0x7F) // value type i32 (matches api.ValueTypeI32)

func cat(bs ...[]byte) []byte {
	var out []byte
	for _, b := range bs {
		out = append(out, b...)
	}
	return out
}

func uleb(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

func sleb(v int64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

func section(id byte, body []byte) []byte {
	return append([]byte{id}, append(uleb(uint32(len(body))), body...)...)
}

func wasmName(s string) []byte { return append(uleb(uint32(len(s))), s...) }

func vec(items ...[]byte) []byte {
	out := uleb(uint32(len(items)))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

func functype(params, results []byte) []byte {
	out := []byte{0x60, byte(len(params))}
	out = append(out, params...)
	out = append(out, byte(len(results)))
	return append(out, results...)
}

func exportFunc(nm string, idx uint32) []byte {
	return cat(wasmName(nm), []byte{0x00}, uleb(idx))
}

func importEntry(mod, field string, typeIdx uint32) []byte {
	return cat(wasmName(mod), wasmName(field), []byte{0x00}, uleb(typeIdx))
}

func codeEntry(body []byte) []byte {
	full := append(uleb(0), body...) // no local declarations
	return append(uleb(uint32(len(full))), full...)
}

// modSpin exports run() = loop { br 0 } — never returns.
func modSpin() []byte {
	return cat(wasmHeader,
		section(1, vec(functype(nil, nil))),
		section(3, vec(uleb(0))),
		section(7, vec(exportFunc("run", 0))),
		section(10, vec(codeEntry([]byte{0x03, 0x40, 0x0C, 0x00, 0x0B, 0x0B}))),
	)
}

// modHostLoop imports env.host_tick and exports run() = loop { host_tick }.
func modHostLoop() []byte {
	return cat(wasmHeader,
		section(1, vec(functype(nil, nil))),
		section(2, vec(importEntry("env", "host_tick", 0))),
		section(3, vec(uleb(0))),
		section(7, vec(exportFunc("run", 1))),
		section(10, vec(codeEntry([]byte{0x03, 0x40, 0x10, 0x00, 0x0C, 0x00, 0x0B, 0x0B}))),
	)
}

// modBlocked imports env.host_tick and exports run() = host_tick(), where
// the host implementation blocks until its context is done.
func modBlocked() []byte {
	return cat(wasmHeader,
		section(1, vec(functype(nil, nil))),
		section(2, vec(importEntry("env", "host_tick", 0))),
		section(3, vec(uleb(0))),
		section(7, vec(exportFunc("run", 1))),
		section(10, vec(codeEntry([]byte{0x10, 0x00, 0x0B}))),
	)
}

// modGrow declares (memory 1) and exports grow(delta:i32)->i32 = memory.grow.
func modGrow() []byte {
	return cat(wasmHeader,
		section(1, vec(functype([]byte{vtI32}, []byte{vtI32}))),
		section(3, vec(uleb(0))),
		section(5, vec([]byte{0x00, 0x01})),
		section(7, vec(exportFunc("grow", 0))),
		section(10, vec(codeEntry([]byte{0x20, 0x00, 0x40, 0x00, 0x0B}))),
	)
}

// modOob declares (memory 1) and exports store() = i32.store at 1 GiB.
func modOob() []byte {
	return cat(wasmHeader,
		section(1, vec(functype(nil, nil))),
		section(3, vec(uleb(0))),
		section(5, vec([]byte{0x00, 0x01})),
		section(7, vec(exportFunc("store", 0))),
		section(10, vec(codeEntry([]byte{0x41, 0x80, 0x80, 0x80, 0x80, 0x04, 0x41, 0x00, 0x36, 0x00, 0x00, 0x0B}))),
	)
}

// modBoom exports boom() = unreachable.
func modBoom() []byte {
	return cat(wasmHeader,
		section(1, vec(functype(nil, nil))),
		section(3, vec(uleb(0))),
		section(7, vec(exportFunc("boom", 0))),
		section(10, vec(codeEntry([]byte{0x00, 0x0B}))),
	)
}

// modWasi imports wasi_snapshot_preview1.fd_write — which this boundary
// never provides.
func modWasi() []byte {
	return cat(wasmHeader,
		section(1, vec(functype([]byte{vtI32, vtI32, vtI32, vtI32}, []byte{vtI32}))),
		section(2, vec(importEntry("wasi_snapshot_preview1", "fd_write", 0))),
	)
}

// --- fixture scaffolding ---

func digestOf(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256-" + hex.EncodeToString(s[:])
}

func verifiedRecord(t *testing.T, name string, art []byte, hostCalls int64, timeout time.Duration) *plugins.PluginRecord {
	t.Helper()
	m := plugins.PluginManifest{
		Name: name, Version: "1.0.0", ABIVersion: plugins.ABIVersion,
		Capabilities: []plugins.CapabilityScope{{Capability: "test.tick", Scope: "tick/*"}},
		Quota:        plugins.Quota{MaxMemoryBytes: 65536, MaxCPUMillis: 1000, Timeout: timeout, MaxHostCalls: hostCalls},
		Digest:       digestOf(art),
	}
	rec, err := plugins.NewPluginRecord(m, fixtureNow)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := plugins.MarkVerified(rec, m.Digest, fixtureNow); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if err := plugins.TransitionPlugin(rec, plugins.LifecycleEnabled, fixtureNow); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return rec
}

func newExecutor(t *testing.T) *Executor {
	t.Helper()
	e, err := NewExecutor(context.Background(), plugins.NewGrantTable())
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	return e
}

// newExecutorWithTick registers env.host_tick (granted for the record) and
// hands the fixture closure to it.
func newExecutorWithTick(t *testing.T, rec *plugins.PluginRecord, fn func(context.Context, []uint64)) *Executor {
	t.Helper()
	table := plugins.NewGrantTable()
	table.Grant(plugins.HostGrant{Plugin: rec.Manifest.Name, Capability: "test.tick", Scope: "tick/*"})
	e, err := NewExecutor(context.Background(), table)
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	if err := e.RegisterHost("env", "host_tick", HostFunction{Capability: "test.tick", Scope: "tick/*", Fn: fn}); err != nil {
		t.Fatalf("register host: %v", err)
	}
	return e
}

func mustInstantiate(t *testing.T, e *Executor, rec *plugins.PluginRecord, art []byte) *Instance {
	t.Helper()
	inst, err := e.Instantiate(context.Background(), HostModule{Record: rec, Artifact: art})
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	return inst
}

func tickReceipts(inst *Instance) []plugins.ReceiptRecord {
	var out []plugins.ReceiptRecord
	for _, r := range inst.Receipts() {
		if r.Capability == "test.tick" {
			out = append(out, r)
		}
	}
	return out
}

// --- enforcement fixtures ---

func TestHostCallQuotaExhausted(t *testing.T) {
	art := modHostLoop()
	rec := verifiedRecord(t, "quota", art, 3, 10*time.Second)
	var ran int64
	e := newExecutorWithTick(t, rec, func(context.Context, []uint64) { ran++ })
	inst := mustInstantiate(t, e, rec, art)

	_, outcome, callErr := inst.Invoke(context.Background(), "run")
	var exitErr *sys.ExitError
	if !errors.As(callErr, &exitErr) || exitErr.ExitCode() != ExitQuotaExhausted {
		t.Fatalf("quota exhaustion must surface as exit code %d, got %v", ExitQuotaExhausted, callErr)
	}
	if outcome != "exited:90" {
		t.Fatalf("outcome = %q, want exited:90", outcome)
	}
	if ran != 3 {
		t.Fatalf("host ran %d times, want exactly the 3 granted calls", ran)
	}
	var got []string
	for _, r := range tickReceipts(inst) {
		got = append(got, r.Outcome)
	}
	want := []string{OutcomeCompleted, OutcomeCompleted, OutcomeCompleted, outcomeQuota}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("host receipts = %v, want %v", got, want)
	}
}

func TestWallTimeCancellationIsNotFuel(t *testing.T) {
	art := modSpin()
	rec := verifiedRecord(t, "spin", art, 1000, 80*time.Millisecond)
	inst := mustInstantiate(t, newExecutor(t), rec, art)

	start := time.Now()
	_, outcome, callErr := inst.Invoke(context.Background(), "run")
	elapsed := time.Since(start)
	if outcome != OutcomeCancelled {
		t.Fatalf("outcome = %q, want %q (err %v)", outcome, OutcomeCancelled, callErr)
	}
	if elapsed < 70*time.Millisecond {
		t.Fatalf("cancelled at %v — before the declared wall-time ceiling", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v — not prompt", elapsed)
	}
	// Honest boundary: this is the wall-time Timeout axis, NOT instruction
	// fuel; no fuel-metering claim is made anywhere in this boundary.
}

func TestBlockedHostCallInterruptedCooperatively(t *testing.T) {
	art := modBlocked()
	rec := verifiedRecord(t, "blocked", art, 5, 100*time.Millisecond)
	observed := false
	e := newExecutorWithTick(t, rec, func(ctx context.Context, _ []uint64) {
		<-ctx.Done()
		observed = true
		panic(sys.NewExitError(91)) // the HOST cooperates with its context
	})
	inst := mustInstantiate(t, e, rec, art)

	start := time.Now()
	_, outcome, callErr := inst.Invoke(context.Background(), "run")
	elapsed := time.Since(start)
	if !observed {
		t.Fatal("the blocked host must observe context cancellation")
	}
	var exitErr *sys.ExitError
	if !errors.As(callErr, &exitErr) || exitErr.ExitCode() != 91 {
		t.Fatalf("cooperative interruption must surface as exit 91, got %v", callErr)
	}
	if outcome != "exited:91" {
		t.Fatalf("outcome = %q, want exited:91", outcome)
	}
	if elapsed < 90*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("interruption at %v — must land at the wall-time ceiling", elapsed)
	}
	// Honest boundary: a host function that ignores its context cannot be
	// preempted by the runtime; cooperation is part of the contract.
}

func TestMemoryCapEnforced(t *testing.T) {
	art := modGrow() // manifest declares 65536 bytes = 1 page
	rec := verifiedRecord(t, "grow", art, 5, 10*time.Second)
	inst := mustInstantiate(t, newExecutor(t), rec, art)

	res, _, err := inst.Invoke(context.Background(), "grow", 0)
	if err != nil || res[0] != 1 {
		t.Fatalf("grow(0) = %d (err %v), want current size 1 page", res, err)
	}
	res, _, err = inst.Invoke(context.Background(), "grow", 2)
	if err != nil {
		t.Fatalf("grow beyond the cap must fail via the -1 signal, not a host error: %v", err)
	}
	if int32(res[0]) != -1 {
		t.Fatalf("grow(2) = %d, want -1 (cap enforced)", int32(res[0]))
	}
}

func TestNonPageMemoryEnvelopeRefused(t *testing.T) {
	art := modGrow()
	rec := verifiedRecord(t, "pages", art, 5, 10*time.Second)
	rec.Manifest.Quota.MaxMemoryBytes = 65537 // not a whole page
	if _, err := newExecutor(t).Instantiate(context.Background(), HostModule{Record: rec, Artifact: art}); err == nil {
		t.Fatal("a byte envelope finer than one page must be refused, not silently widened")
	}
}

func TestInstantiationRefusals(t *testing.T) {
	art := modSpin()
	e := newExecutor(t)
	ctx := context.Background()

	if _, err := e.Instantiate(ctx, HostModule{Record: verifiedRecord(t, "d1", art, 5, time.Second), Artifact: art}); err != nil {
		t.Fatalf("matching bytes must instantiate: %v", err)
	}
	mutated := append([]byte(nil), art...)
	mutated[len(mutated)-1] ^= 0xFF
	rec := verifiedRecord(t, "d2", art, 5, time.Second)
	if _, err := e.Instantiate(ctx, HostModule{Record: rec, Artifact: mutated}); err == nil {
		t.Fatal("artifact bytes mismatching the verified digest must be refused")
	}
	// Digest swapped after verification.
	rec = verifiedRecord(t, "d3", art, 5, time.Second)
	rec.Manifest.Digest = "sha256-" + strings.Repeat("b", 64)
	if _, err := e.Instantiate(ctx, HostModule{Record: rec, Artifact: art}); err == nil {
		t.Fatal("a digest swapped after verification must be refused")
	}
	// Disabled record.
	rec = verifiedRecord(t, "d4", art, 5, time.Second)
	if err := plugins.TransitionPlugin(rec, plugins.LifecycleDisabled, fixtureNow); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := e.Instantiate(ctx, HostModule{Record: rec, Artifact: art}); err == nil {
		t.Fatal("a disabled record must be refused")
	}
}

func TestUnauthorizedHostCallDenied(t *testing.T) {
	art := modHostLoop()
	rec := verifiedRecord(t, "unauth", art, 10, 10*time.Second)
	// Deny-by-default: the grant table is EMPTY.
	e, err := NewExecutor(context.Background(), plugins.NewGrantTable())
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	if err := e.RegisterHost("env", "host_tick", HostFunction{Capability: "test.tick", Scope: "tick/*", Fn: func(context.Context, []uint64) {}}); err != nil {
		t.Fatalf("register host: %v", err)
	}
	inst := mustInstantiate(t, e, rec, art)

	_, outcome, callErr := inst.Invoke(context.Background(), "run")
	var exitErr *sys.ExitError
	if !errors.As(callErr, &exitErr) || exitErr.ExitCode() != ExitHostCallDenied {
		t.Fatalf("ungranted host call must be denied with exit %d, got %v", ExitHostCallDenied, callErr)
	}
	if outcome != "exited:91" {
		t.Fatalf("outcome = %q, want exited:91", outcome)
	}
	if got := tickReceipts(inst); len(got) != 1 || got[0].Outcome != outcomeDenied {
		t.Fatalf("receipts = %+v, want exactly one %q line", got, outcomeDenied)
	}
}

func TestRevocationMidRunDeniesNextCall(t *testing.T) {
	art := modHostLoop()
	rec := verifiedRecord(t, "revoke", art, 100, 10*time.Second)
	var ran int64
	e := newExecutorWithTick(t, rec, func(context.Context, []uint64) {
		ran++
		if ran == 2 {
			if err := plugins.Revoke(rec, time.Now()); err != nil {
				t.Errorf("revoke: %v", err)
			}
		}
	})
	inst := mustInstantiate(t, e, rec, art)

	_, _, callErr := inst.Invoke(context.Background(), "run")
	var exitErr *sys.ExitError
	if !errors.As(callErr, &exitErr) || exitErr.ExitCode() != ExitHostCallDenied {
		t.Fatalf("post-revocation call must be denied, got %v", callErr)
	}
	if ran != 2 {
		t.Fatalf("host ran %d times, want exactly 2 before revocation", ran)
	}
	var got []string
	for _, r := range tickReceipts(inst) {
		got = append(got, r.Outcome)
	}
	want := []string{OutcomeCompleted, OutcomeCompleted, outcomeDenied}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("receipts = %v, want %v", got, want)
	}
}

func TestTrapContainment(t *testing.T) {
	e := newExecutor(t)
	ctx := context.Background()

	boomRec := verifiedRecord(t, "boom", modBoom(), 5, time.Second)
	boom := mustInstantiate(t, e, boomRec, modBoom())
	_, outcome, callErr := boom.Invoke(ctx, "boom")
	if outcome != OutcomeTrapped || callErr == nil {
		t.Fatalf("unreachable must trap, got outcome %q err %v", outcome, callErr)
	}
	if got := boom.Receipts(); len(got) == 0 || got[0].Outcome != OutcomeTrapped {
		t.Fatalf("trap must be receipted, got %+v", got)
	}

	oobRec := verifiedRecord(t, "oob", modOob(), 5, time.Second)
	oob := mustInstantiate(t, e, oobRec, modOob())
	if _, oobOutcome, _ := oob.Invoke(ctx, "store"); oobOutcome != OutcomeTrapped {
		t.Fatalf("out-of-bounds store must trap, got %q", oobOutcome)
	}

	// The host process and executor survive both traps: a fresh module runs.
	growRec := verifiedRecord(t, "after", modGrow(), 5, time.Second)
	grow := mustInstantiate(t, e, growRec, modGrow())
	res, _, err := grow.Invoke(ctx, "grow", 0)
	if err != nil || res[0] != 1 {
		t.Fatalf("executor unusable after traps: grow(0) = %d (err %v)", res, err)
	}
}

func TestNoAmbientWASI(t *testing.T) {
	art := modWasi()
	rec := verifiedRecord(t, "wasi", art, 5, time.Second)
	if _, err := newExecutor(t).Instantiate(context.Background(), HostModule{Record: rec, Artifact: art}); err == nil {
		t.Fatal("a module importing WASI must fail to instantiate: no ambient I/O exists")
	}
}

func TestResultAdmissionAndMemoryPath(t *testing.T) {
	art := modGrow()
	rec := verifiedRecord(t, "result", art, 5, 10*time.Second)
	inst := mustInstantiate(t, newExecutor(t), rec, art)

	if err := inst.WriteMemory(0, []byte("karyora")); err != nil {
		t.Fatalf("write memory: %v", err)
	}
	out, err := inst.ReadResult(0, 7)
	if err != nil || string(out) != "karyora" {
		t.Fatalf("read result = %q (err %v), want karyora", out, err)
	}
	r, err := inst.AdmitResult(out)
	if err != nil {
		t.Fatalf("valid result refused: %v", err)
	}
	if r.OutputBytes != 7 || r.OutputDigest != digestOf(out) {
		t.Fatalf("admitted result mismatch: %+v", r)
	}
	if _, err := inst.AdmitResult(make([]byte, plugins.MaxPluginResultBytes+1)); err == nil {
		t.Fatal("an oversized result must be rejected before any storage work")
	}
	var admitted, rejected bool
	for _, rc := range inst.Receipts() {
		switch rc.Outcome {
		case outcomeAdmit:
			admitted = true
		case outcomeReject:
			rejected = true
		}
	}
	if !admitted || !rejected {
		t.Fatalf("admission/rejection receipts missing: %+v", inst.Receipts())
	}
}

func TestMissingExportAndRegistrationGuards(t *testing.T) {
	art := modSpin()
	rec := verifiedRecord(t, "misc", art, 5, time.Second)
	inst := mustInstantiate(t, newExecutor(t), rec, art)
	if _, _, err := inst.Invoke(context.Background(), "nope"); err == nil {
		t.Fatal("a missing export must be an error")
	}
	// Duplicate registration is refused; registration freezes after the
	// first instantiation.
	e := newExecutor(t)
	if err := e.RegisterHost("env", "host_tick", HostFunction{Capability: "test.tick", Scope: "tick/*", Fn: func(context.Context, []uint64) {}}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := e.RegisterHost("env", "host_tick", HostFunction{Capability: "test.tick", Scope: "tick/*", Fn: func(context.Context, []uint64) {}}); err == nil {
		t.Fatal("duplicate registration must be refused")
	}
	r2 := verifiedRecord(t, "misc2", art, 5, time.Second)
	if _, err := e.Instantiate(context.Background(), HostModule{Record: r2, Artifact: art}); err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	if err := e.RegisterHost("env", "host_other", HostFunction{Capability: "test.tick", Scope: "tick/*", Fn: func(context.Context, []uint64) {}}); err == nil {
		t.Fatal("registration must freeze after the first instantiation")
	}
}
