package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/qira-dev/karyora/internal/plugins"
)

// The enforced plugin host boundary (CR-4.3 C4-25): every advertised quota
// axis is enforced at THIS boundary around the pinned runtime, and every
// host call passes the delivered per-call authorization contract before
// the host function runs. The runtime is never given ambient I/O (no WASI
// is mounted), so a module can reach nothing except host functions it was
// explicitly granted. No instruction fuel-metering claim is made: the
// CPU-millis axis has no tested runtime support; the wall-time Timeout
// axis is the enforced ceiling and is never labeled fuel.

const (
	// pageBytes is one WASM memory page; the declared byte envelope must
	// map onto whole pages (a finer ceiling cannot be enforced, so it is
	// refused instead of silently widened).
	pageBytes int64 = 64 << 10

	// Exit codes the boundary raises through the runtime's documented
	// host-error mechanism (sys.ExitError). Boundary-local audit values.
	ExitQuotaExhausted uint32 = 90
	ExitHostCallDenied uint32 = 91
)

// Outcome vocabulary for receipts (boundary-local audit strings, Proposed).
const (
	OutcomeCompleted = "completed"
	OutcomeCancelled = "cancelled"
	OutcomeTrapped   = "trapped"
	OutcomeExited    = "exited:" // formatted as exited:<code>

	outcomeDenied  = "host_call_denied"
	outcomeQuota   = "quota_exhausted"
	outcomeAdmit   = "result_admitted"
	outcomeReject  = "result_rejected"
	outcomeInvoke  = "invoke"
	outcomeResult  = "result.admit"
	outcomeNoScope = "-"
)

// HostFunction is one host capability an embedder registers under a WASM
// import (module, name). The executor wraps every call with the delivered
// contract: the grant table must carry the capability with a matching
// scope, the record must be enabled with its digest still verified, and
// the per-invocation host-call quota must not be exhausted.
type HostFunction struct {
	Params     []api.ValueType
	Results    []api.ValueType
	Capability string
	Scope      string
	Fn         func(ctx context.Context, stack []uint64)
}

// HostModule pairs a verified plugin record with its verified artifact
// bytes. Instantiation re-verifies the bytes against the record's verified
// digest — the C4-26 verification-order invariant applied at the runtime.
type HostModule struct {
	Record   *plugins.PluginRecord
	Artifact []byte
}

// Executor owns the host-function registry; every Instance gets its OWN
// runtime so the manifest's memory cap is enforced per plugin
// (WithMemoryLimitPages is a per-runtime limit). Host registration freezes
// at the first instantiation; a module importing an unregistered name
// fails to instantiate (missing import = denied surface).
type Executor struct {
	grants *plugins.GrantTable

	mu    sync.Mutex
	hosts map[string]HostFunction // "module.name" -> registration
	built bool
}

// NewExecutor builds the boundary around a deny-by-default grant table.
func NewExecutor(ctx context.Context, grants *plugins.GrantTable) (*Executor, error) {
	if grants == nil {
		return nil, errors.New("pluginhost: a grant table is required (deny-by-default)")
	}
	return &Executor{grants: grants, hosts: map[string]HostFunction{}}, nil
}

// RegisterHost exposes one host capability as the WASM import (module, name).
func (e *Executor) RegisterHost(module, name string, hf HostFunction) error {
	if module == "" || name == "" || hf.Capability == "" || hf.Scope == "" || hf.Fn == nil {
		return fmt.Errorf("pluginhost: host %s.%s: module, name, capability, scope and fn are required", module, name)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.built {
		return errors.New("pluginhost: host functions are frozen after the first instantiation")
	}
	key := module + "." + name
	if _, dup := e.hosts[key]; dup {
		return fmt.Errorf("pluginhost: host %s already registered", key)
	}
	e.hosts[key] = hf
	return nil
}

// Instantiate verifies the record and artifact bytes, maps the memory
// envelope onto pages, and instantiates the module under its manifest cap.
func (e *Executor) Instantiate(ctx context.Context, hm HostModule) (*Instance, error) {
	rec := hm.Record
	if rec == nil {
		return nil, errors.New("pluginhost: a plugin record is required")
	}
	if rec.State != plugins.LifecycleEnabled {
		return nil, fmt.Errorf("pluginhost: plugin %q is %q, not enabled: instantiation refused", rec.Manifest.Name, rec.State)
	}
	if rec.VerifiedDigest != rec.Manifest.Digest {
		return nil, fmt.Errorf("pluginhost: plugin %q digest verification does not match its manifest: instantiation refused", rec.Manifest.Name)
	}
	sum := sha256.Sum256(hm.Artifact)
	if got := "sha256-" + hex.EncodeToString(sum[:]); got != rec.VerifiedDigest {
		return nil, errors.New("pluginhost: artifact bytes do not match the verified digest: instantiation refused")
	}
	pages, err := memoryPages(rec.Manifest.Quota.MaxMemoryBytes)
	if err != nil {
		return nil, err
	}
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().
		WithMemoryLimitPages(pages).
		WithCloseOnContextDone(true))
	if err := e.buildHostModule(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("pluginhost: host module: %w", err)
	}
	mod, err := rt.InstantiateWithConfig(ctx, hm.Artifact,
		wazero.NewModuleConfig().WithName(rec.Manifest.Name))
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("pluginhost: instantiate %q: %w", rec.Manifest.Name, err)
	}
	return &Instance{rt: rt, mod: mod, rec: rec}, nil
}

func (e *Executor) buildHostModule(ctx context.Context, rt wazero.Runtime) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.built {
		return nil
	}
	if len(e.hosts) > 0 {
		hmb := rt.NewHostModuleBuilder("env")
		for key, hf := range e.hosts {
			_, name, _ := strings.Cut(key, ".")
			fn := hf
			wrapper := func(callCtx context.Context, _ api.Module, stack []uint64) {
				st := invocationFrom(callCtx)
				if st == nil {
					panic(sys.NewExitError(ExitHostCallDenied)) // host call outside an enforced invocation
				}
				if err := plugins.AuthorizeHostCall(e.grants, st.rec, fn.Capability, fn.Scope); err != nil {
					st.inst.appendReceipt(fn.Capability, fn.Scope, "", outcomeDenied, 1)
					panic(sys.NewExitError(ExitHostCallDenied))
				}
				if st.calls >= st.rec.Manifest.Quota.MaxHostCalls {
					st.inst.appendReceipt(fn.Capability, fn.Scope, "", outcomeQuota, 1)
					panic(sys.NewExitError(ExitQuotaExhausted))
				}
				st.calls++
				fn.Fn(callCtx, stack)
				st.inst.appendReceipt(fn.Capability, fn.Scope, "", OutcomeCompleted, 1)
			}
			hmb.NewFunctionBuilder().WithGoModuleFunction(api.GoModuleFunc(wrapper), hf.Params, hf.Results).Export(name)
		}
		if _, err := hmb.Instantiate(ctx); err != nil {
			return err
		}
	}
	e.built = true
	return nil
}

func memoryPages(maxBytes int64) (uint32, error) {
	if maxBytes <= 0 || maxBytes%pageBytes != 0 {
		return 0, fmt.Errorf("pluginhost: MaxMemoryBytes %d is not a positive whole number of 64 KiB pages", maxBytes)
	}
	return uint32(maxBytes / pageBytes), nil
}

// invocationState carries the per-invocation record and host-call counter
// to the host wrappers through the call context. One invocation runs at a
// time per Instance, all on the calling goroutine.
type invocationState struct {
	inst  *Instance
	rec   *plugins.PluginRecord
	calls int64
}

type invocationKey struct{}

func invocationFrom(ctx context.Context) *invocationState {
	st, _ := ctx.Value(invocationKey{}).(*invocationState)
	return st
}

// Instance is one instantiated plugin module under its manifest envelope,
// with its own runtime carrying the memory cap.
type Instance struct {
	rt  wazero.Runtime
	mod api.Module
	rec *plugins.PluginRecord

	mu       sync.Mutex
	receipts []plugins.ReceiptRecord
}

// Invoke runs one exported function under the manifest envelope. The
// wall-time ceiling is the manifest Timeout (context cancellation — never
// fuel); the outcome is classified from the runtime error and receipted.
func (i *Instance) Invoke(ctx context.Context, export string, args ...uint64) ([]uint64, string, error) {
	fn := i.mod.ExportedFunction(export)
	if fn == nil {
		i.appendReceipt(outcomeInvoke, export, "", OutcomeTrapped, 0)
		return nil, OutcomeTrapped, fmt.Errorf("pluginhost: no export %q", export)
	}
	callCtx, cancel := context.WithTimeout(ctx, i.rec.Manifest.Quota.Timeout)
	defer cancel()
	st := &invocationState{inst: i, rec: i.rec}
	res, callErr := fn.Call(context.WithValue(callCtx, invocationKey{}, st), args...)
	outcome, code := classify(callErr)
	if code != 0 {
		outcome = fmt.Sprintf("%s%d", OutcomeExited, code)
	}
	i.appendReceipt(outcomeInvoke, export, "", outcome, st.calls)
	return res, outcome, callErr
}

// classify maps a runtime error onto the audit vocabulary. Every
// sys.ExitError inside this boundary originates either from the boundary
// itself (quota 90 / denial 91 — a module has no other exit path: WASI is
// not mounted and traps are plain errors) or from the runtime closing the
// module on a done context (WithCloseOnContextDone; sentinel close code,
// message "module closed with context …") — that is a cancellation, never
// a host exit. A trapped module is never mistaken for a clean result.
func classify(err error) (string, uint32) {
	if err == nil {
		return OutcomeCompleted, 0
	}
	var exitErr *sys.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case ExitQuotaExhausted, ExitHostCallDenied:
			return OutcomeExited, exitErr.ExitCode()
		default:
			return OutcomeCancelled, 0
		}
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return OutcomeCancelled, 0
	}
	return OutcomeTrapped, 0
}

// AdmitResult validates and digest-addresses untrusted output (C4-27):
// the digest is computed from the bytes themselves, the size is bounded,
// and every admission or rejection leaves a receipt. Invalid output is
// rejected before anything downstream sees it.
func (i *Instance) AdmitResult(out []byte) (plugins.PluginResult, error) {
	r := plugins.PluginResult{
		Plugin:      i.rec.Manifest.Name,
		Version:     i.rec.Manifest.Version,
		ABIVersion:  i.rec.Manifest.ABIVersion,
		OutputBytes: int64(len(out)),
	}
	if len(out) > 0 {
		sum := sha256.Sum256(out)
		r.OutputDigest = "sha256-" + hex.EncodeToString(sum[:])
	}
	outcome := outcomeAdmit
	if err := plugins.ValidateResult(&r, plugins.MaxPluginResultBytes); err != nil {
		outcome = outcomeReject
		i.appendReceipt(outcomeResult, outcomeNoScope, "", outcome, 0)
		return r, err
	}
	i.appendReceipt(outcomeResult, outcomeNoScope, r.OutputDigest, outcome, 0)
	return r, nil
}

// ReadResult reads a bounded region of the module's linear memory — the
// result-collection path future product wiring will use.
func (i *Instance) ReadResult(offset, size uint32) ([]byte, error) {
	b, ok := i.mod.Memory().Read(offset, size)
	if !ok {
		return nil, fmt.Errorf("pluginhost: memory region [%d,%d) is outside the module's memory", offset, size)
	}
	return append([]byte(nil), b...), nil
}

// WriteMemory writes host-provided bytes into the module's linear memory.
func (i *Instance) WriteMemory(offset uint32, b []byte) error {
	if !i.mod.Memory().Write(offset, b) {
		return fmt.Errorf("pluginhost: memory write at %d (%d bytes) is outside the module's memory", offset, len(b))
	}
	return nil
}

// Receipts returns a copy of the audit lines accumulated so far.
func (i *Instance) Receipts() []plugins.ReceiptRecord {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]plugins.ReceiptRecord, len(i.receipts))
	copy(out, i.receipts)
	return out
}

func (i *Instance) appendReceipt(capability, scope, resultDigest, outcome string, hostCalls int64) {
	line := plugins.BuildReceipt(i.rec, capability, scope, resultDigest, outcome, hostCalls, time.Now().UTC())
	i.mu.Lock()
	defer i.mu.Unlock()
	i.receipts = append(i.receipts, line)
}

// Close releases the module and its runtime.
func (i *Instance) Close(ctx context.Context) error {
	_ = i.mod.Close(ctx)
	return i.rt.Close(ctx)
}
