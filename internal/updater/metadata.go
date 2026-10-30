package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// The metadata trust layer (C5-17 §4): the two-key structure (offline root
// delegating an online release signer, threshold 1 per role — C5-17 §14),
// the release manifest fields of C5-17 §4.2, and the exact-bytes signature
// envelope. The signed bytes are the payload's own canonical serialization
// (Go encoding/json output of the fixed-field-order structs below — no maps
// anywhere in the payloads, so the encoding is deterministic), embedded in
// the published document as an exact-byte JSON string and signed with
// Ed25519. The client verifies over those exact bytes and never
// re-canonicalizes; any splice of documents from different releases (M10)
// fails byte-exact verification or the root-version binding check.

const (
	ManifestSchema = "uawp-update-manifest/v1"
	RootSchema     = "uawp-update-root/v1"
	SigAlgorithm   = "ed25519"

	// Hard size/field bounds — implementation constants with recorded
	// rationale (C5-17 §5 row 8; C5-19 M8: "the bounds are implementation
	// constants with recorded rationale, never silent"). A manifest for the
	// six C5-11 rows with supply-chain pointers and a migration manifest is
	// a few KiB; 1 MiB is a generous ceiling that still makes an
	// accidental-or-malicious oversized document impossible to parse.
	MaxMetadataBytes   = 1 << 20 // 1 MiB for any metadata document
	MaxArtifacts       = 64      // six rows + future formats
	MaxSupplyChainPtrs = 64      // one per artifact
	MaxMigrationSteps  = 256     // a release's schema migrations
	MaxVersionLen      = 64      // release/min-supported version strings
	MaxChannelLen      = 32      // v1 carries exactly one channel: stable
)

// Signature is an algorithm-qualified detached signature over the exact
// payload bytes (C5-17 §4.2's "algorithm-qualified" requirement).
type Signature struct {
	Algorithm string `json:"algorithm"` // SigAlgorithm; anything else is refused
	KeyID     string `json:"key_id"`    // hex sha256 fingerprint of the signing public key
	Value     []byte `json:"value"`     // the Ed25519 signature bytes
}

// SignedDocument is the published wrapper: the payload's exact canonical
// bytes as a JSON string plus the detached signature over those bytes.
type SignedDocument struct {
	Payload   string    `json:"payload"`
	Signature Signature `json:"signature"`
}

// PayloadBytes returns the exact signed bytes of the document.
func (d SignedDocument) PayloadBytes() []byte { return []byte(d.Payload) }

// SignerDelegation is the root's delegation of the release signer
// (C5-17 §4.1): the signer's Ed25519 public key and the signature threshold
// (threshold 1 per the delegated value).
type SignerDelegation struct {
	PublicKey []byte `json:"public_key"`
	Threshold int    `json:"threshold"`
}

// RootPayload is the root metadata (C5-17 §4.1): its own monotonic version,
// its expiry (per-release expiry applies to the root metadata too — C5-17
// §9.1's binding rule), and the delegation of the release signer. The ROOT
// document is signed by the client's pinned root key (the canonical anchor
// page's fingerprint — C5-13 §3.1's publication rule).
type RootPayload struct {
	SchemaVersion string           `json:"schema_version"`
	RootVersion   uint64           `json:"root_version"`
	ExpiresAt     time.Time        `json:"expires_at"`
	ReleaseSigner SignerDelegation `json:"release_signer"`
}

// ArtifactEntry is one target's artifact identity (C5-14 §3's digest half)
// plus the detached artifact signature published by the C5-13 identity
// layer. The signature pointer is optional in the STRUCT but the machine
// refuses any artifact whose signature is absent (A3's signature-absent
// variant — C5-13 §3.3: no digest-only downgrade).
type ArtifactEntry struct {
	TargetID  string     `json:"target_id"`
	Kind      string     `json:"kind"`
	SHA256    string     `json:"sha256"`
	Size      int64      `json:"size"`
	Signature *Signature `json:"signature,omitempty"`
}

// SupplyChainPtr points at the C5-16 binding record for one artifact
// (pointers only — C5-16 §8 rule 4: the records are separate evidence
// layers; C5-19 A5: they must resolve and match).
type SupplyChainPtr struct {
	ArtifactSHA256      string `json:"artifact_sha256"`
	BindingRecordSHA256 string `json:"binding_record_sha256"`
}

// MigrationStep is one declared migration (C5-18 §6.1): its identity and
// the honestly-declared reversibility class.
type MigrationStep struct {
	ID                 string `json:"id"`
	ReversibilityClass string `json:"reversibility_class"`
}

// MigrationManifest is the release's migration declaration (C5-18 §6.1):
// the schema range it migrates, the rollback-compatibility declaration (the
// refuse-incompatible gate's datum — §6.3's RECOMMENDED policy, delegated
// as REFUSE), and the per-step declarations.
type MigrationManifest struct {
	FromSchema         int             `json:"from_schema"`
	ToSchema           int             `json:"to_schema"`
	RollbackCompatible bool            `json:"rollback_compatible"`
	Steps              []MigrationStep `json:"steps"`
}

// ReleaseManifest is the per-release metadata (C5-17 §4.2's field design;
// root_version and migrations added per C5-18's consumption of it).
type ReleaseManifest struct {
	SchemaVersion      string            `json:"schema_version"`
	Channel            string            `json:"channel"`
	ReleaseVersion     string            `json:"release_version"`
	MetadataVersion    uint64            `json:"metadata_version"`
	RootVersion        uint64            `json:"root_version"`
	PublishedAt        time.Time         `json:"published_at"`
	ExpiresAt          time.Time         `json:"expires_at"`
	Artifacts          []ArtifactEntry   `json:"artifacts"`
	SupplyChain        []SupplyChainPtr  `json:"supply_chain"`
	MinSupportedClient string            `json:"min_supported_client"`
	Migrations         MigrationManifest `json:"migrations"`
}

// MetadataBundle is what the metadata fetcher serves: the current root
// metadata (optional — absent means "no root change; use the persisted
// delegation") and the release manifest.
type MetadataBundle struct {
	Root     *SignedDocument `json:"root,omitempty"`
	Manifest SignedDocument  `json:"manifest"`
}

// Fingerprint returns the hex sha256 fingerprint of an Ed25519 public key
// (the KeyID form; never key material in records — C5-13 §9.1).
func Fingerprint(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// validateSignedDocument bound-checks one in-memory document and returns the
// exact signed payload bytes. The fetcher port delivers DECODED documents —
// the JSON wrapper ({"payload": "<exact bytes>", "signature": …}) is decoded
// by the production fetcher itself, and a Go string round-trips JSON
// byte-exactly, so the signature still binds the exact payload bytes.
// Fail-closed on: empty/oversized payload, missing/unknown algorithm, and a
// malformed signature value.
func validateSignedDocument(doc *SignedDocument) ([]byte, error) {
	if doc == nil {
		return nil, fail(ReasonMalformed, "metadata document absent")
	}
	payload := []byte(doc.Payload)
	if len(payload) == 0 {
		return nil, fail(ReasonMalformed, "metadata document carries no payload")
	}
	if len(payload) > MaxMetadataBytes {
		return nil, fail(ReasonOversized, "metadata payload is %d bytes (bound %d)", len(payload), MaxMetadataBytes)
	}
	if doc.Signature.Algorithm != SigAlgorithm {
		return nil, fail(ReasonSignatureInvalid, "signature algorithm %q is not %q", doc.Signature.Algorithm, SigAlgorithm)
	}
	if len(doc.Signature.Value) != ed25519.SignatureSize {
		return nil, fail(ReasonSignatureInvalid, "signature is %d bytes (Ed25519 requires %d)", len(doc.Signature.Value), ed25519.SignatureSize)
	}
	return payload, nil
}

// verifySignature is the threshold-1 Ed25519 check over the exact payload
// bytes (C5-17 §14's per-role threshold; M6's shortfall = any missing,
// wrong-key, or invalid signature).
func verifySignature(payload []byte, sig Signature, pub ed25519.PublicKey, wantKeyID string) error {
	if len(pub) != ed25519.PublicKeySize {
		return fail(ReasonSignatureInvalid, "delegated public key is %d bytes (Ed25519 requires %d)", len(pub), ed25519.PublicKeySize)
	}
	if wantKeyID != "" && sig.KeyID != wantKeyID {
		return fail(ReasonSignatureInvalid, "signature key %q is not the expected key %q", sig.KeyID, wantKeyID)
	}
	if !ed25519.Verify(pub, payload, sig.Value) {
		return fail(ReasonSignatureInvalid, "Ed25519 verification failed over the exact payload bytes")
	}
	return nil
}

// parseRootPayload decodes and bound-checks the root payload.
func parseRootPayload(payload []byte) (RootPayload, error) {
	var rp RootPayload
	if err := json.Unmarshal(payload, &rp); err != nil {
		return RootPayload{}, fail(ReasonMalformed, "root payload is not valid JSON: %v", err)
	}
	if rp.SchemaVersion != RootSchema {
		return RootPayload{}, fail(ReasonMalformed, "root schema_version %q is not %q", rp.SchemaVersion, RootSchema)
	}
	if rp.RootVersion == 0 {
		return RootPayload{}, fail(ReasonMalformed, "root_version must be a positive monotonic integer")
	}
	if rp.ReleaseSigner.Threshold != 1 {
		// Threshold 1 is the delegated value (C5-17 §14); any other count in
		// a served root is a malformed document for THIS client, not a new
		// policy (M6's typed refusal; a threshold change is a versioned
		// decision, never client-adaptive).
		return RootPayload{}, fail(ReasonThresholdShortfall, "root delegates threshold %d; this client requires 1", rp.ReleaseSigner.Threshold)
	}
	if len(rp.ReleaseSigner.PublicKey) != ed25519.PublicKeySize {
		return RootPayload{}, fail(ReasonMalformed, "delegated release-signer key is %d bytes", len(rp.ReleaseSigner.PublicKey))
	}
	return rp, nil
}

// parseManifestPayload decodes and bound-checks the manifest payload (the
// M8 field bounds; an absurd artifacts[]/supply_chain[]/steps count is a
// typed refusal, never a parse crash or partial acceptance).
func parseManifestPayload(payload []byte) (ReleaseManifest, error) {
	var rm ReleaseManifest
	if err := json.Unmarshal(payload, &rm); err != nil {
		return ReleaseManifest{}, fail(ReasonMalformed, "manifest payload is not valid JSON: %v", err)
	}
	if rm.SchemaVersion != ManifestSchema {
		return ReleaseManifest{}, fail(ReasonMalformed, "manifest schema_version %q is not %q", rm.SchemaVersion, ManifestSchema)
	}
	if rm.Channel != "stable" {
		// v1 carries exactly one official channel (C5-17 §4.2; Blueprint
		// §15.2) — anything else is refused, never silently accepted.
		return ReleaseManifest{}, fail(ReasonMalformed, "manifest channel %q is not the single official channel", rm.Channel)
	}
	if rm.ReleaseVersion == "" || len(rm.ReleaseVersion) > MaxVersionLen {
		return ReleaseManifest{}, fail(ReasonMalformed, "release_version missing or longer than %d bytes", MaxVersionLen)
	}
	if rm.MinSupportedClient != "" && len(rm.MinSupportedClient) > MaxVersionLen {
		return ReleaseManifest{}, fail(ReasonMalformed, "min_supported_client longer than %d bytes", MaxVersionLen)
	}
	if rm.MetadataVersion == 0 {
		return ReleaseManifest{}, fail(ReasonMalformed, "metadata_version must be a positive monotonic integer")
	}
	if rm.ExpiresAt.IsZero() {
		return ReleaseManifest{}, fail(ReasonMalformed, "expires_at is mandatory (per-release expiry, C5-17 §9.1)")
	}
	if len(rm.Artifacts) == 0 || len(rm.Artifacts) > MaxArtifacts {
		return ReleaseManifest{}, fail(ReasonMalformed, "artifacts count %d outside [1,%d]", len(rm.Artifacts), MaxArtifacts)
	}
	seen := map[string]bool{}
	for i, a := range rm.Artifacts {
		if len(a.TargetID) == 0 || len(a.TargetID) > MaxVersionLen {
			return ReleaseManifest{}, fail(ReasonMalformed, "artifact %d target_id missing or oversized", i)
		}
		if seen[a.TargetID] {
			return ReleaseManifest{}, fail(ReasonMalformed, "artifact %d duplicates target %q", i, a.TargetID)
		}
		seen[a.TargetID] = true
		if !isHex64(a.SHA256) {
			return ReleaseManifest{}, fail(ReasonMalformed, "artifact %d sha256 is not a sha256 hex digest", i)
		}
		if a.Size <= 0 {
			return ReleaseManifest{}, fail(ReasonMalformed, "artifact %d size must be positive", i)
		}
	}
	if len(rm.SupplyChain) > MaxSupplyChainPtrs {
		return ReleaseManifest{}, fail(ReasonOversized, "supply_chain count %d exceeds %d", len(rm.SupplyChain), MaxSupplyChainPtrs)
	}
	for i, p := range rm.SupplyChain {
		if !isHex64(p.ArtifactSHA256) || !isHex64(p.BindingRecordSHA256) {
			return ReleaseManifest{}, fail(ReasonMalformed, "supply_chain pointer %d carries a non-digest field", i)
		}
	}
	if len(rm.Migrations.Steps) > MaxMigrationSteps {
		return ReleaseManifest{}, fail(ReasonOversized, "migration steps %d exceed %d", len(rm.Migrations.Steps), MaxMigrationSteps)
	}
	return rm, nil
}

func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// VerifyRoot verifies a root document against the client's pinned root key
// (the out-of-band trust anchor): threshold-1 signature over the exact
// payload bytes, then the payload's own bounds. Returns the parsed payload.
func VerifyRoot(root *SignedDocument, pinnedRoot ed25519.PublicKey) (RootPayload, error) {
	if root == nil {
		return RootPayload{}, fail(ReasonMalformed, "root metadata absent where required")
	}
	payload, err := validateSignedDocument(root)
	if err != nil {
		return RootPayload{}, err
	}
	if err := verifySignature(payload, root.Signature, pinnedRoot, Fingerprint(pinnedRoot)); err != nil {
		return RootPayload{}, err
	}
	return parseRootPayload(payload)
}

// VerifyManifest verifies a manifest document under the given release-signer
// key (the root's threshold-1 delegation): signature over the exact payload
// bytes, then the payload bounds. The FRESHNESS checks (expiry, monotonic
// version) belong to the machine, never to this parser — C5-17 §7: a valid
// signature alone is not freshness.
func VerifyManifest(manifest *SignedDocument, signer ed25519.PublicKey) (ReleaseManifest, []byte, error) {
	if manifest == nil {
		return ReleaseManifest{}, nil, fail(ReasonMalformed, "manifest absent")
	}
	payload, err := validateSignedDocument(manifest)
	if err != nil {
		return ReleaseManifest{}, nil, err
	}
	if err := verifySignature(payload, manifest.Signature, signer, Fingerprint(signer)); err != nil {
		return ReleaseManifest{}, nil, err
	}
	rm, err := parseManifestPayload(payload)
	if err != nil {
		return ReleaseManifest{}, nil, err
	}
	return rm, payload, nil
}

// CanonicalPayload marshals a payload struct to its canonical bytes: the
// Go encoding/json output of a fixed-field-order struct (no maps anywhere
// in the payload types, so the serialization is deterministic). Publishers
// sign exactly these bytes; the envelope records the algorithm.
func CanonicalPayload(payload any) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("canonical payload marshal: %w", err)
	}
	return b, nil
}
