// Package knowledge defines the B1 KnowledgePack family's wire shapes and
// closed registers (the published canonical contract family of
// contracts/openapi.yaml — the Knowledge* schemas, the KnowledgeErrorCode
// register, and the KnowledgeEvent* vocabulary, carried verbatim: never a
// restatement, never an extension). The package is part of the domain leaf:
// it imports no other internal package and carries no transport, database,
// or OS types. The Core-owned identity members and the authoritative state
// register live here; the UI renders Core-returned truth only.
package knowledge

// PackSchemaVersion is the pack/version schema version this implementation
// serves (the pack schema's own version field; the first served version).
const PackSchemaVersion = 1

// Operation kinds (the closed knowledge-operation kind register).
const (
	KindImport = "import"
	KindExport = "export"
)

// ValidKind reports membership in the closed kind register.
func ValidKind(s string) bool {
	return s == KindImport || s == KindExport
}

// The closed knowledge-operation state register (the published register
// carried verbatim). The import stage vocabulary uploading → extracting →
// validating → indexing is emitted from ACTUAL Core stages only as
// import.stage_entered events — progress is never a fabricated percentage
// and no percentage member exists anywhere in Operation.
const (
	StateSubmitted      = "submitted"
	StateUploading      = "uploading"
	StateExtracting     = "extracting"
	StateValidating     = "validating"
	StateIndexing       = "indexing"
	StateGenerating     = "generating"
	StateCompleted      = "completed"
	StateRejected       = "rejected"
	StateFailed         = "failed"
	StateCancelled      = "cancelled"
	StateOutcomeUnknown = "outcome_unknown"
)

// ValidOperationState reports membership in the closed state register.
func ValidOperationState(s string) bool {
	switch s {
	case StateSubmitted, StateUploading, StateExtracting, StateValidating,
		StateIndexing, StateGenerating, StateCompleted, StateRejected,
		StateFailed, StateCancelled, StateOutcomeUnknown:
		return true
	default:
		return false
	}
}

// Terminal reports whether the state is terminal (completed | rejected |
// failed | cancelled). Terminal states accept no further transitions; a
// terminal state's timestamp IS that transition's instant.
func Terminal(state string) bool {
	switch state {
	case StateCompleted, StateRejected, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// The import stage sequence in processing order (the recorded UI stage
// vocabulary; each stage is emitted from the actual Core stage only).
var ImportStages = []string{StateUploading, StateExtracting, StateValidating, StateIndexing}

// The closed claim-status register (the published six-value list).
const (
	ClaimCandidate  = "candidate"
	ClaimActive     = "active"
	ClaimContested  = "contested"
	ClaimSuperseded = "superseded"
	ClaimRevoked    = "revoked"
	ClaimArchived   = "archived"
)

// The closed evidence verification-state register (the published four-value
// list). A verified state is content-identity evidence only — never
// publisher authorization, authenticity, license compliance, factual truth,
// or freedom from prompt injection.
const (
	VerificationUnsigned    = "unsigned"
	VerificationVerified    = "verified"
	VerificationRevoked     = "revoked"
	VerificationQuarantined = "quarantined"
)

// The closed duplicate-mode register (the published Core-exposed digest-match
// choices). Displayed and accepted ONLY where the Core defines the
// corresponding operation — this implementation defines none, so the field
// is refused non-empty at the service boundary (the register's own law).
const (
	DuplicateLogicalReference  = "logical_reference"
	DuplicateDeduplication     = "deduplication"
	DuplicateDistinctReference = "distinct_reference"
)

// The KnowledgeErrorCode register (the published family codes carried
// verbatim — each riding the published ErrorEnvelope with its retryable
// value and the coded retryClass). The MB §6.3 explicit-failure list first;
// the import rejection vocabulary second.
const (
	FailDependencyCycle   = "KNOWLEDGE_DEPENDENCY_CYCLE"
	FailSignatureInvalid  = "KNOWLEDGE_SIGNATURE_INVALID"
	FailSchemaUnsupported = "KNOWLEDGE_SCHEMA_UNSUPPORTED"
	FailTrustRevoked      = "KNOWLEDGE_TRUST_REVOKED"
	FailDigestMismatch    = "KNOWLEDGE_DIGEST_MISMATCH"
	FailLicensePolicyFail = "KNOWLEDGE_LICENSE_POLICY_FAIL"

	FailImportTypeUnsupported     = "IMPORT_TYPE_UNSUPPORTED"
	FailImportSizeOverLimit       = "IMPORT_SIZE_OVER_LIMIT"
	FailImportMalformed           = "IMPORT_MALFORMED"
	FailImportDuplicateDigest     = "IMPORT_DUPLICATE_DIGEST"
	FailImportPermissionDenied    = "IMPORT_PERMISSION_DENIED"
	FailImportNetworkInterrupted  = "IMPORT_NETWORK_INTERRUPTED"
	FailImportPolicyBlocked       = "IMPORT_POLICY_BLOCKED"
	FailImportIntegritySigFailure = "IMPORT_INTEGRITY_SIGNATURE_FAILURE"
)

// The F10-shaped event aggregate types (the published register carried
// verbatim; events ride the frozen F10 EventRecord field set and are never
// the source of truth — the authoritative state endpoint is).
const (
	AggregateKnowledgePack            = "knowledgePack"
	AggregateKnowledgePackVersion     = "knowledgePackVersion"
	AggregateKnowledgeImportOperation = "knowledgeImportOperation"
	AggregateKnowledgeExportOperation = "knowledgeExportOperation"
)

// The F10-shaped event_type vocabulary (the published register carried
// verbatim; no type outside this list is ever emitted — the register is
// closed, and no import.failed type exists while export.failed does).
const (
	EventPackCreated           = "pack.created"
	EventPackVersionCreated    = "pack_version.created"
	EventPackVersionSuperseded = "pack_version.superseded"
	EventImportSubmitted       = "import.submitted"
	EventImportStageEntered    = "import.stage_entered"
	EventImportCompleted       = "import.completed"
	EventImportRejected        = "import.rejected"
	EventImportCancelled       = "import.cancelled"
	EventExportSubmitted       = "export.submitted"
	EventExportCompleted       = "export.completed"
	EventExportFailed          = "export.failed"
)

// UntrustedLabel is the one coded value of the published untrusted-content
// label register; every imported Knowledge source is untrusted — content is
// data, never instruction.
const UntrustedLabel = "untrusted"

// The identity-create intent encodings (the duplicate-create layer's
// fingerprint class members). PackCreate is the destination-absent intent;
// VersionCreatePrefix + the named pack is the destination-present intent.
const (
	CreateIntentPack          = "pack-create"
	CreateIntentVersionPrefix = "version-create:"
)

// Pack is one durable knowledge pack (the published KnowledgePack schema's
// members carried verbatim; JSON tags are the contract member names). The
// pack's identity members and its current-version reference; indexes are
// rebuildable, never canonical.
type Pack struct {
	PackID           string `json:"pack_id"`
	SchemaVersion    int    `json:"schema_version"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
	CurrentVersionID string `json:"current_version_id"`
}

// PackList is one bounded pack page (the keyset list semantics; the
// cursor's absence marks the final page).
type PackList struct {
	Packs  []Pack `json:"packs"`
	Cursor string `json:"cursor,omitempty"`
}

// PackVersion is one immutable pack version (the published
// KnowledgePackVersion schema's members carried verbatim). Prior versions
// are immutable — a source-snapshot change creates a new version or an
// explicit supersession, never edits old evidence; immutability is
// Core-enforced. PackRootDigest carries the algorithm-qualified digest FORM
// sha256:<lowercase-hex> as a value format only — the D-41 portable profile
// stays behind its owner gate and no algorithm is approved by its carriage.
type PackVersion struct {
	PackID         string   `json:"pack_id"`
	VersionID      string   `json:"version_id"`
	SchemaVersion  int      `json:"schema_version"`
	PackRootDigest string   `json:"pack_root_digest"`
	EntryCount     int      `json:"entry_count,omitempty"`
	ClaimCount     int      `json:"claim_count,omitempty"`
	EvidenceCount  int      `json:"evidence_count,omitempty"`
	Supersedes     []string `json:"supersedes,omitempty"`
	SupersededBy   string   `json:"superseded_by,omitempty"`
	License        string   `json:"license,omitempty"`
	LicenseStatus  string   `json:"license_status,omitempty"`
	CreatedAt      string   `json:"created_at"`
}

// PackVersionList is one bounded pack-version page (the keyset list
// semantics).
type PackVersionList struct {
	Versions []PackVersion `json:"versions"`
	Cursor   string        `json:"cursor,omitempty"`
}

// Claim is one knowledge claim (the published KnowledgeClaim schema's
// members carried verbatim; MB §6.3's model). Conflicting claims are never
// hidden — shown via the explicit relations.
type Claim struct {
	ClaimID        string   `json:"claim_id"`
	Subject        string   `json:"subject"`
	Predicate      string   `json:"predicate"`
	Object         string   `json:"object"`
	Scope          string   `json:"scope"`
	Status         string   `json:"status"`
	ValidFrom      string   `json:"valid_from,omitempty"`
	ValidUntil     string   `json:"valid_until,omitempty"`
	ObservedAt     string   `json:"observed_at,omitempty"`
	SourceRefs     []string `json:"source_refs,omitempty"`
	EvidenceRefs   []string `json:"evidence_refs,omitempty"`
	DerivedFrom    []string `json:"derived_from,omitempty"`
	Supersedes     []string `json:"supersedes,omitempty"`
	ConflictsWith  []string `json:"conflicts_with,omitempty"`
	Language       string   `json:"language,omitempty"`
	ValidationNote string   `json:"validation_note,omitempty"`
}

// Evidence is one evidence record (the published KnowledgeEvidence schema's
// members carried verbatim). The digest establishes content identity
// relative to compared bytes only.
type Evidence struct {
	EvidenceID        string `json:"evidence_id"`
	SourceID          string `json:"source_id"`
	Locator           string `json:"locator"`
	Digest            string `json:"digest,omitempty"`
	SourceSnapshotRef string `json:"source_snapshot_ref,omitempty"`
	CapturedAt        string `json:"captured_at"`
	VerificationState string `json:"verification_state"`
}

// Source is one source identity (the published KnowledgePackSource schema's
// members carried verbatim).
type Source struct {
	SourceID           string `json:"source_id"`
	Kind               string `json:"kind"`
	Locator            string `json:"locator"`
	CapturedAt         string `json:"captured_at"`
	ObservedAt         string `json:"observed_at,omitempty"`
	License            string `json:"license,omitempty"`
	LicenseStatus      string `json:"license_status,omitempty"`
	FreshnessPolicyRef string `json:"freshness_policy_ref,omitempty"`
}

// ImportSourceSpec is one served supported-source registry member (the
// published KnowledgeImportSourceSpec schema's members carried verbatim) —
// the Core's own supported kinds and per-kind size bounds as queryable
// contract data. URLPolicy is carried only where the kind fetches URLs.
type ImportSourceSpec struct {
	SourceKind string `json:"source_kind"`
	MaxBytes   int64  `json:"max_bytes"`
	URLPolicy  string `json:"url_policy,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// Operation is one knowledge operation's authoritative record (the
// published KnowledgeOperation schema's members carried verbatim) — the
// state endpoint's payload. No percentage member exists anywhere; the
// stage vocabulary rides the closed state register only.
type Operation struct {
	OperationID    string           `json:"operation_id"`
	Kind           string           `json:"kind"`
	State          string           `json:"state"`
	CorrelationID  string           `json:"correlation_id"`
	FailureCode    string           `json:"failure_code,omitempty"`
	FailureReason  string           `json:"failure_reason,omitempty"`
	Result         *OperationResult `json:"result,omitempty"`
	UntrustedLabel string           `json:"untrusted_label,omitempty"`
	CreatedAt      string           `json:"created_at"`
	UpdatedAt      string           `json:"updated_at"`
}

// OperationResult is one operation's Core-confirmed result (the published
// KnowledgeOperationResult schema's members carried verbatim) — what
// persists; no erasure claim beyond Core-confirmed scope. Present only once
// the operation carries it.
type OperationResult struct {
	Kind      string `json:"kind"`
	PackID    string `json:"pack_id,omitempty"`
	VersionID string `json:"version_id,omitempty"`
	ExportRef string `json:"export_ref,omitempty"`
	Digest    string `json:"digest,omitempty"`
}

// SubmitResult is one submit's accepted outcome (the published
// KnowledgeOperationSubmitResult schema's members carried verbatim):
// replayed=true on the M-9 same-digest replay.
type SubmitResult struct {
	Operation Operation `json:"operation"`
	Replayed  bool      `json:"replayed"`
}

// EntryRecord is one stored pack entry's full record — the persistence form
// behind the version's entry set (the bytes ride the durable entry storage;
// the digest grammar is the additive version-qualified CAS grammar).
type EntryRecord struct {
	Path        string
	ContentType string
	SizeBytes   int64
	Digest      string
	Content     []byte
}

// ImportIntent is the import submit's durable intent: the operation record
// to store (state submitted; the store owns the durable wall-clock values)
// with the M-9 keying members and the identity-create intent.
type ImportIntent struct {
	Operation      Operation
	IdempotencyKey string
	SourceDigest   string
	CreateIntent   string
}

// ImportCommit is the import completion's atomic durable payload: the
// pack/version rows, the entries, and the terminal operation state with its
// result — committed in ONE transaction with the family's events.
type ImportCommit struct {
	OperationID   string
	CorrelationID string
	Pack          Pack
	PackIsNew     bool
	Version       PackVersion
	Entries       []EntryRecord
	Result        OperationResult
}

// ExportCommit is the export completion's atomic durable payload: the
// artifact bytes (CAS-published), the result, and the terminal state —
// committed in ONE transaction with the export event.
type ExportCommit struct {
	OperationID   string
	CorrelationID string
	Result        OperationResult
	Artifact      []byte
}

// ListQuery is one keyset list request: the bounded page size and the
// opaque cursor position (the last page's sort-key values).
type ListQuery struct {
	Limit          int
	AfterCreatedAt string
	AfterID        string
}

// PackPage is one pack list page: the rows plus whether more exist beyond
// the page (the cursor's mint is the service's; absence marks the final
// page).
type PackPage struct {
	Packs []Pack
	More  bool
}

// VersionPage is one version list page.
type VersionPage struct {
	Versions []PackVersion
	More     bool
}
