// Package errs defines the Core machine-error contract: the owner-accepted
// wire field set (code, retryable, correlationId, bounded data) and the
// Phase 2 code register from the shared-contracts specification (C2-05).
// The package is part of the domain leaf: it imports no other internal
// package and carries no OS, database, or transport types. HTTP maps this
// envelope onto RFC 9457 Problem Details without title/detail; localized
// presentation copy never travels through this package.
package errs

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Code is a stable, language-neutral Core error identifier.
type Code string

// The Phase 2 code register (shared-contracts specification, C2-05).
// CONFLICT_IDEMPOTENCY_KEY joins additively at its implementation (the
// owner's card-3 working decision of 2026-11-02: "the code's C2-05 slice
// is the v1 canonical error envelope register; the recorded unimplemented
// codes join additively as they implement") — the C1-07-recorded code for
// the Supplied provider-connection create idempotency (P3.2 §3.3: same key
// + different body → 409 with fingerprintMatch:false), implemented by the
// connection-record milestone.
const (
	CodeValidationFailed       Code = "VALIDATION_FAILED"
	CodeConfigConflict         Code = "CONFIG_CONFLICT"
	CodeAuthzDenied            Code = "AUTHZ_DENIED"
	CodeNotFound               Code = "NOT_FOUND"
	CodeConflictState          Code = "CONFLICT_STATE"
	CodeConflictIdempotencyKey Code = "CONFLICT_IDEMPOTENCY_KEY"
	CodeSingleInstanceConflict Code = "SINGLE_INSTANCE_CONFLICT"
	CodeStorageUnavailable     Code = "STORAGE_UNAVAILABLE"
	CodeMigrationFailed        Code = "MIGRATION_FAILED"
	CodeStateCorrupt           Code = "STATE_CORRUPT"
	CodeRecoveryRequired       Code = "RECOVERY_REQUIRED"
	CodeRateLimited            Code = "RATE_LIMITED"
	CodeInternal               Code = "INTERNAL"
)

// The Memory family's domain codes join the coded register ADDITIVELY at
// their implementation (the same recorded register-extension class as
// CONFLICT_IDEMPOTENCY_KEY above) — the published MemoryErrorCode register
// carried verbatim (the B2 family; the CF-2 publication act): each riding
// the published ErrorEnvelope with its retryable value and the coded
// retryClass.
const (
	// CodeMemoryScopeViolation — a read/write crossing the forbidden
	// cross-scope default (MB §7.5); retry class never.
	CodeMemoryScopeViolation Code = "MEMORY_SCOPE_VIOLATION"
	// CodeMemorySecretsRejected — a write attempting secret storage; the
	// BINDING secrets law (MB §12.2 law 3); retry class never.
	CodeMemorySecretsRejected Code = "MEMORY_SECRETS_REJECTED"
	// CodeMemoryItemNotFound — the unknown or out-of-scope item, the SAME
	// typed 404 (the no-leak rule); retry class never.
	CodeMemoryItemNotFound Code = "MEMORY_ITEM_NOT_FOUND"
	// CodeMemoryReviewStateConflict — approve/reject against an item not
	// in the required review state; retry class never.
	CodeMemoryReviewStateConflict Code = "MEMORY_REVIEW_STATE_CONFLICT"
	// CodeMemoryConflict — the correction/conflict path's concurrent-edit
	// outcome; reconcile by re-reading the item's authoritative state,
	// never overwrite.
	CodeMemoryConflict Code = "MEMORY_CONFLICT"
	// CodeMemoryNotEditable — the eligibility law's refusal; the Core owns
	// eligibility; retry class never.
	CodeMemoryNotEditable Code = "MEMORY_NOT_EDITABLE"
)

// The Context family's domain codes (the published ContextErrorCode
// register carried verbatim; the CF-3 publication act) join the coded
// register ADDITIVELY at their implementation (the same recorded
// register-extension class as the Memory codes above) — each riding the
// published ErrorEnvelope with its PUBLISHED retryable value and coded
// retryClass (the per-code values are the publication act's assignments
// following the CF-3 draft §3(i) retryClass proposals). The
// idempotency/state conflicts ride the PUBLISHED codes exactly
// (CONFLICT_IDEMPOTENCY_KEY / CONFLICT_STATE) — no Context-specific
// conflict code exists.
const (
	// CodeContextBudgetExhausted — the budget-exhaustion outcome producing
	// the explicit omission/partial-context record (the
	// no-silent-truncation budget law, MB §18); retry class never (a
	// repeat with the same budget re-exhausts; the omission record is the
	// explicit outcome, not a transient failure).
	CodeContextBudgetExhausted Code = "CONTEXT_BUDGET_EXHAUSTED"
	// CodeContextOmissionReasonUnavailable — the typed carrier for the
	// honest reason-unavailable state: the Core reports an omission
	// without a reason and the absence is displayed as the absence, never
	// a synthesized reason (P4.6 ST-3); retry class safe-retry.
	CodeContextOmissionReasonUnavailable Code = "CONTEXT_OMISSION_REASON_UNAVAILABLE"
	// CodeContextCompilationUnavailable — the compilation/preview cannot
	// be served; the honest unavailable state renders, never a fabricated
	// state (P4.6 ST-9/C7); retry class safe-retry.
	CodeContextCompilationUnavailable Code = "CONTEXT_COMPILATION_UNAVAILABLE"
	// CodeContextCompareUnsupported — compare attempted without stable
	// Core comparison support; safe to re-issue as a read-only
	// effect-free read while the outcome persists until support exists
	// (the GC-4 law); never a fabricated diff; retry class safe-retry.
	CodeContextCompareUnsupported Code = "CONTEXT_COMPARE_UNSUPPORTED"
	// CodeContextSnapshotNotFound — the unknown snapshot/operation
	// identity, the SAME typed 404 for unknown and out-of-scope (the
	// no-leak rule; the typed-404 pattern is the published precedent);
	// retry class never.
	CodeContextSnapshotNotFound Code = "CONTEXT_SNAPSHOT_NOT_FOUND"
)

// The Workflow family's domain codes (the published WorkflowErrorCode
// register carried verbatim; the CF-4 publication act) join the coded
// register ADDITIVELY at their implementation (the same recorded
// register-extension class as the Memory and Context codes above) — each
// riding the published ErrorEnvelope with its PUBLISHED retryable value
// and coded retryClass (the publication act's assignments: every
// Workflow domain code is `never`). The idempotency/state conflicts ride
// the PUBLISHED codes exactly (CONFLICT_IDEMPOTENCY_KEY / CONFLICT_STATE)
// — no Workflow-specific conflict code exists.
const (
	// CodeWorkflowValidationFailed — the Core-owned typed blocking-error
	// outcome (the save's authoritative re-check refusing a draft that
	// carries blocking findings); the result is a fact about the evaluated
	// object, not a transient failure; the exact-object labeling law
	// applies; retry class never.
	CodeWorkflowValidationFailed Code = "WORKFLOW_VALIDATION_FAILED"
	// CodeWorkflowWarningUnacknowledged — a save or run attempted while
	// identified non-blocking warnings requiring acknowledgment are
	// unacknowledged; the acknowledgment is the user's explicit act —
	// never a timeout, never implicit, never a dismissed toast; a retry
	// without acknowledgment re-fails deterministically; retry class
	// never.
	CodeWorkflowWarningUnacknowledged Code = "WORKFLOW_WARNING_UNACKNOWLEDGED"
	// CodeWorkflowRevisionConflict — the concurrent-revision outcome; the
	// interim conflict rule governs (no overwrite; no merge without an
	// approved policy; both sides survive; the conflict state persists
	// until user resolution — never auto-resolved, never timed out);
	// retry class never.
	CodeWorkflowRevisionConflict Code = "WORKFLOW_REVISION_CONFLICT"
	// CodeWorkflowUnknownNodeType — a node type the active Core does not
	// contract (the palette source is ONLY the Core-contracted node
	// vocabulary; no placeholder node can be saved or run); rides the
	// validation issue's typed `code` member; retry class never.
	CodeWorkflowUnknownNodeType Code = "WORKFLOW_UNKNOWN_NODE_TYPE"
	// CodeWorkflowNodePermissionDenied — a node requiring a
	// capability/permission the current principal lacks (the Core's
	// validation and the single decision port re-check authoritatively);
	// retry class never.
	CodeWorkflowNodePermissionDenied Code = "WORKFLOW_NODE_PERMISSION_DENIED"
	// CodeWorkflowNotSaved — an operation attempted against a draft that
	// is not a Core-confirmed saved revision (the no-silent-save law);
	// retry class never.
	CodeWorkflowNotSaved Code = "WORKFLOW_NOT_SAVED"

	// The Plugin family's domain codes (the published PluginErrorCode
	// register; the CF-5 publication act) — each riding the published
	// ErrorEnvelope with its PUBLISHED retryable assignment (the
	// publication act's assignments following the CF-5 draft §3(k)
	// retryClass routing), each naming the admission/lifecycle check whose
	// typed outcome it carries. THE SHAPE DECISION (recorded at the
	// 2026-11-09 resolution act, binding): the three typed INVALID
	// outcomes stay three DISTINCT codes — facts about different input
	// aspects (the bytes' signature, the bytes' digest, the manifest's
	// structure); the Core does not collapse them into one typed `data`
	// payload, exactly as the contract publishes them. The idempotency/state conflicts
	// ride the PUBLISHED codes exactly (CONFLICT_IDEMPOTENCY_KEY /
	// CONFLICT_STATE) — no Plugin-specific conflict code exists.
	//
	// CodePluginTrustUnknown — the UNKNOWN admission state's outcome:
	// publisher/trust root unverified or not determinable by the Core; the
	// state can change only Core-side — reconcile through the
	// authoritative state read before any retry, never a blind resubmit;
	// retry class reconcile.
	CodePluginTrustUnknown Code = "PLUGIN_TRUST_UNKNOWN"
	// CodePluginSignatureInvalid — the signature-verification failure; a
	// DISTINCT typed INVALID outcome; a fact about the submitted bytes; a
	// retry of the same bytes re-fails deterministically; retry class
	// never.
	CodePluginSignatureInvalid Code = "PLUGIN_SIGNATURE_INVALID"
	// CodePluginDigestMismatch — the digest mismatch; a DISTINCT typed
	// INVALID outcome; a fact about the submitted bytes; retry class
	// never.
	CodePluginDigestMismatch Code = "PLUGIN_DIGEST_MISMATCH"
	// CodePluginManifestInvalid — the malformed-manifest outcome; a
	// DISTINCT typed INVALID outcome; a fact about the submitted manifest;
	// retry class never.
	CodePluginManifestInvalid Code = "PLUGIN_MANIFEST_INVALID"
	// CodePluginABIIncompatible — the INCOMPATIBLE state's outcome: the
	// ABI/runtime mismatch; update availability is rendered where the Core
	// reports it; retry class never.
	CodePluginABIIncompatible Code = "PLUGIN_ABI_INCOMPATIBLE"
	// CodePluginTrustRevoked — the REVOKED state's outcome: trust revoked;
	// FUTURE invocations blocked and the current invocation's behavior
	// Core-reported — never guessed; retry class never.
	CodePluginTrustRevoked Code = "PLUGIN_TRUST_REVOKED"
	// CodePluginCapabilityDenied — the DENIED state's outcome: policy
	// denies a required capability, or the user denies it in the review; a
	// retry never grants — the user's explicit act governs; retry class
	// never.
	CodePluginCapabilityDenied Code = "PLUGIN_CAPABILITY_DENIED"
)

// RetryClass is the operation-level retry classification. On the wire it
// travels inside Data under the reserved "retryClass" key so the accepted
// envelope field set stays exactly the owner-approved four fields (D-35).
type RetryClass string

const (
	RetryNever     RetryClass = "never"
	RetrySafe      RetryClass = "safe-retry"
	RetryBackoff   RetryClass = "backoff"
	RetryReconcile RetryClass = "reconcile"
)

type codeDefault struct {
	retryable bool
	class     RetryClass
}

// register is the single source of Phase 2 code semantics.
var register = map[Code]codeDefault{
	CodeValidationFailed:          {retryable: false, class: RetryNever},
	CodeConfigConflict:            {retryable: false, class: RetryNever},
	CodeAuthzDenied:               {retryable: false, class: RetryNever},
	CodeNotFound:                  {retryable: false, class: RetryNever},
	CodeConflictState:             {retryable: false, class: RetryNever},
	CodeConflictIdempotencyKey:    {retryable: false, class: RetryNever},
	CodeSingleInstanceConflict:    {retryable: true, class: RetryBackoff},
	CodeStorageUnavailable:        {retryable: true, class: RetryBackoff},
	CodeMigrationFailed:           {retryable: true, class: RetryReconcile},
	CodeStateCorrupt:              {retryable: true, class: RetryReconcile},
	CodeRecoveryRequired:          {retryable: true, class: RetryReconcile},
	CodeRateLimited:               {retryable: true, class: RetryBackoff},
	CodeInternal:                  {retryable: true, class: RetrySafe},
	CodeMemoryScopeViolation:      {retryable: false, class: RetryNever},
	CodeMemorySecretsRejected:     {retryable: false, class: RetryNever},
	CodeMemoryItemNotFound:        {retryable: false, class: RetryNever},
	CodeMemoryReviewStateConflict: {retryable: false, class: RetryNever},
	CodeMemoryConflict:            {retryable: false, class: RetryReconcile},
	CodeMemoryNotEditable:         {retryable: false, class: RetryNever},
	// The Context family's domain codes (the published ContextErrorCode
	// register; the CF-3 publication act) — each with its PUBLISHED
	// retryable value and coded retryClass (the publication act's
	// assignments).
	CodeContextBudgetExhausted:           {retryable: false, class: RetryNever},
	CodeContextOmissionReasonUnavailable: {retryable: true, class: RetrySafe},
	CodeContextCompilationUnavailable:    {retryable: true, class: RetrySafe},
	CodeContextCompareUnsupported:        {retryable: true, class: RetrySafe},
	CodeContextSnapshotNotFound:          {retryable: false, class: RetryNever},
	// The Workflow family's domain codes (the published WorkflowErrorCode
	// register; the CF-4 publication act) — each with its PUBLISHED
	// retryable value (`never`) and coded retryClass.
	CodeWorkflowValidationFailed:      {retryable: false, class: RetryNever},
	CodeWorkflowWarningUnacknowledged: {retryable: false, class: RetryNever},
	CodeWorkflowRevisionConflict:      {retryable: false, class: RetryNever},
	CodeWorkflowUnknownNodeType:       {retryable: false, class: RetryNever},
	CodeWorkflowNodePermissionDenied:  {retryable: false, class: RetryNever},
	CodeWorkflowNotSaved:              {retryable: false, class: RetryNever},
	// The Plugin family's domain codes (the published PluginErrorCode
	// register; the CF-5 publication act) join additively at their
	// implementation per the same recorded register-extension class —
	// each with its PUBLISHED retryable value and coded retryClass
	// (PLUGIN_TRUST_UNKNOWN rides reconcile — the state can change only
	// Core-side; the remaining six ride never, per the publication act's
	// assignments).
	CodePluginTrustUnknown:     {retryable: true, class: RetryReconcile},
	CodePluginSignatureInvalid: {retryable: false, class: RetryNever},
	CodePluginDigestMismatch:   {retryable: false, class: RetryNever},
	CodePluginManifestInvalid:  {retryable: false, class: RetryNever},
	CodePluginABIIncompatible:  {retryable: false, class: RetryNever},
	CodePluginTrustRevoked:     {retryable: false, class: RetryNever},
	CodePluginCapabilityDenied: {retryable: false, class: RetryNever},
}

// RetryClassOf reports the registered default classification for a code and
// whether the code is known at all.
func RetryClassOf(code Code) (RetryClass, bool) {
	d, ok := register[code]
	return d.class, ok
}

// Bounds enforce the bounded-data rule of the contract: an envelope never
// carries unbounded content.
const (
	MaxDataKeys       = 16
	MaxDataValueBytes = 512
)

// reservedDataKeys are set by the register, never by callers.
var reservedDataKeys = map[string]bool{
	"retryClass": true,
}

// Envelope is the machine error contract. JSON field names are the
// owner-accepted set; Data carries bounded typed members only (field/reason
// codes — never prose, secrets, stack traces, or raw upstream bodies).
type Envelope struct {
	Code          Code           `json:"code"`
	Retryable     bool           `json:"retryable"`
	CorrelationID string         `json:"correlationId"`
	Data          map[string]any `json:"data"`
}

// New builds a validated envelope for a known code. It fails closed on
// unknown codes, empty correlation IDs, reserved data keys, or unbounded
// data rather than emitting a contract violation.
func New(code Code, correlationID string, data map[string]any) (Envelope, error) {
	d, ok := register[code]
	if !ok {
		return Envelope{}, fmt.Errorf("errs: unknown code %q", string(code))
	}
	if correlationID == "" {
		return Envelope{}, fmt.Errorf("errs: correlationId is required for code %q", string(code))
	}
	clean := make(map[string]any, len(data)+1)
	for k, v := range data {
		if reservedDataKeys[k] {
			return Envelope{}, fmt.Errorf("errs: data key %q is reserved", k)
		}
		b, err := json.Marshal(v)
		if err != nil {
			return Envelope{}, fmt.Errorf("errs: data[%q] is not encodable", k)
		}
		if len(b) > MaxDataValueBytes {
			return Envelope{}, fmt.Errorf("errs: data[%q] exceeds %d bytes", k, MaxDataValueBytes)
		}
		clean[k] = v
	}
	clean["retryClass"] = string(d.class)
	if len(clean) > MaxDataKeys {
		return Envelope{}, fmt.Errorf("errs: data exceeds %d keys", MaxDataKeys)
	}
	return Envelope{Code: code, Retryable: d.retryable, CorrelationID: correlationID, Data: clean}, nil
}

// NewCorrelationID returns a fresh random correlation identifier, or an
// empty string when the platform entropy source is unavailable; callers
// fail closed through New on an empty ID.
func NewCorrelationID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
