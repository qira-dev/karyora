package plugins

import (
	"fmt"
	"time"
)

// Result validation and auditable receipts (C4-27): plugin output is
// UNTRUSTED. It is never inlined into Core state on faith — it arrives
// digest-addressed with a bounded size, and every host interaction leaves
// a receipt for the audit trail.

// MaxPluginResultBytes is the model bound for a result admitted to the
// CAS path; larger outputs are rejected before any storage work.
const MaxPluginResultBytes = 8 << 20 // 8 MiB

// PluginResult is the declared shape of a plugin's output.
type PluginResult struct {
	Plugin       string `json:"plugin"`
	Version      string `json:"version"`
	ABIVersion   string `json:"abi_version"`
	OutputDigest string `json:"output_digest"` // sha256- of the result bytes (CAS-addressed)
	OutputBytes  int64  `json:"output_bytes"`
}

// ValidateResult rejects malformed or oversized untrusted output (C4-27):
// known ABI, digest-addressed result, bounded size. Validation proves
// SHAPE only — plugin results are still untrusted content and must pass
// the same inspection any external input receives downstream.
func ValidateResult(r *PluginResult, maxBytes int64) error {
	if r.ABIVersion != ABIVersion {
		return fmt.Errorf("%w: unknown ABI version %q", ErrResultUntrusted, r.ABIVersion)
	}
	if r.Plugin == "" || r.Version == "" {
		return fmt.Errorf("%w: plugin and version are required", ErrResultUntrusted)
	}
	if !sha256Pattern.MatchString(r.OutputDigest) {
		return fmt.Errorf("%w: output must be digest-addressed (sha256-<64 hex>)", ErrResultUntrusted)
	}
	if r.OutputBytes <= 0 || r.OutputBytes > maxBytes {
		return fmt.Errorf("%w: output size %d outside the admitted bound (%d bytes)", ErrResultUntrusted, r.OutputBytes, maxBytes)
	}
	return nil
}

// ReceiptRecord is the durable audit line for one completed host
// interaction (C4-27): who ran, under which ABI/digest, with which
// granted capability and scope, producing which result digest.
type ReceiptRecord struct {
	Plugin       string    `json:"plugin"`
	Version      string    `json:"version"`
	ABIVersion   string    `json:"abi_version"`
	Artifact     string    `json:"artifact_digest"`
	Capability   string    `json:"capability"`
	Scope        string    `json:"scope"`
	ResultDigest string    `json:"result_digest"`
	Outcome      string    `json:"outcome"`
	HostCalls    int64     `json:"host_calls"`
	At           time.Time `json:"at"`
}

// BuildReceipt assembles the receipt after a host interaction completes;
// the caller supplies the verified facts (outcome strings come from the
// future runtime; nothing here is inferred).
func BuildReceipt(rec *PluginRecord, capability, scope, resultDigest, outcome string, hostCalls int64, at time.Time) ReceiptRecord {
	return ReceiptRecord{
		Plugin:       rec.Manifest.Name,
		Version:      rec.Manifest.Version,
		ABIVersion:   rec.Manifest.ABIVersion,
		Artifact:     rec.VerifiedDigest,
		Capability:   capability,
		Scope:        scope,
		ResultDigest: resultDigest,
		Outcome:      outcome,
		HostCalls:    hostCalls,
		At:           at.UTC(),
	}
}
