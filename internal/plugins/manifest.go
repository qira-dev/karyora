package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Plugin ABI host contracts (CR-4.3). Third-party plugin execution stays
// DISABLED until the D-14 runtime evidence gate passes: this package
// specifies the manifest, trust lifecycle, scoped host grants, and result
// receipts — it never executes anything, and no fuel-metering or
// containment claim is made without documented runtime support.

// ABIVersion pins the language-neutral host-call envelope (C4-20). A
// manifest written against any other ABI fails closed.
const ABIVersion = "plugin-abi/v1"

var sha256Pattern = regexp.MustCompile(`^sha256-[0-9a-f]{64}$`)

// Sentinel errors (typed, inspectable).
var (
	ErrManifestInvalid = errors.New("plugins: invalid manifest")
	ErrLifecycle       = errors.New("plugins: illegal lifecycle transition")
	ErrHostCallDenied  = errors.New("plugins: host call denied")
	ErrResultUntrusted = errors.New("plugins: plugin result rejected")
)

// Quota is the declared resource envelope (C4-25 model). ENFORCEMENT
// belongs to the future D-14-proven runtime; the manifest merely declares
// the bounds, and validation refuses non-positive/absent envelopes.
type Quota struct {
	MaxMemoryBytes int64         `json:"max_memory_bytes"`
	MaxCPUMillis   int64         `json:"max_cpu_millis"`
	Timeout        time.Duration `json:"timeout"`
	MaxHostCalls   int64         `json:"max_host_calls"`
}

// CapabilityScope is one declared host capability with its exact scope
// string (e.g. capability "fs.read", scope "/workspace/exports/*"). A
// manifest declares REQUESTS; grants are separate (C4-06/C4-23).
type CapabilityScope struct {
	Capability string `json:"capability"`
	Scope      string `json:"scope"`
}

// PluginManifest is the versioned, digest-addressed plugin descriptor
// (C4-22). The digest pins the artifact bytes; it is NOT proof of
// publisher trust by itself.
type PluginManifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	ABIVersion   string            `json:"abi_version"`
	Capabilities []CapabilityScope `json:"capabilities"`
	Quota        Quota             `json:"quota"`
	Digest       string            `json:"digest"`
}

// Plugin lifecycle states (C4-26). Revoked is terminal: a revoked plugin
// can never be enabled again — a re-install is a NEW record.
const (
	LifecycleRegistered = "registered"
	LifecycleVerified   = "verified"
	LifecycleEnabled    = "enabled"
	LifecycleDisabled   = "disabled"
	LifecycleRevoked    = "revoked"
)

var lifecycleTransitions = map[string]map[string]bool{
	LifecycleRegistered: {LifecycleVerified: true, LifecycleRevoked: true},
	LifecycleVerified:   {LifecycleEnabled: true, LifecycleRevoked: true},
	LifecycleEnabled:    {LifecycleDisabled: true, LifecycleRevoked: true},
	LifecycleDisabled:   {LifecycleEnabled: true, LifecycleRevoked: true},
	LifecycleRevoked:    {},
}

// PluginRecord is one plugin through its trust lifecycle (C4-26).
type PluginRecord struct {
	Manifest       PluginManifest `json:"manifest"`
	State          string         `json:"state"`
	VerifiedDigest string         `json:"verified_digest,omitempty"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// NewPluginRecord registers a manifest.
func NewPluginRecord(m PluginManifest, now time.Time) (*PluginRecord, error) {
	if err := ValidateManifest(&m); err != nil {
		return nil, err
	}
	return &PluginRecord{Manifest: m, State: LifecycleRegistered, UpdatedAt: now.UTC()}, nil
}

// ValidateManifest enforces the contract (C4-20/C4-22): known ABI version,
// named capabilities with scopes, a positive resource envelope, and a
// well-formed content digest. Unknown ABI versions fail closed.
func ValidateManifest(m *PluginManifest) error {
	if m.Name == "" || m.Version == "" {
		return fmt.Errorf("%w: name and version are required", ErrManifestInvalid)
	}
	if m.ABIVersion != ABIVersion {
		return fmt.Errorf("%w: unknown ABI version %q (expected %s)", ErrManifestInvalid, m.ABIVersion, ABIVersion)
	}
	if len(m.Capabilities) == 0 {
		return fmt.Errorf("%w: at least one declared capability is required", ErrManifestInvalid)
	}
	for _, c := range m.Capabilities {
		if c.Capability == "" || c.Scope == "" {
			return fmt.Errorf("%w: capability %q: both capability and scope are required", ErrManifestInvalid, c.Capability)
		}
	}
	if m.Quota.MaxMemoryBytes <= 0 || m.Quota.MaxCPUMillis <= 0 || m.Quota.Timeout <= 0 || m.Quota.MaxHostCalls <= 0 {
		return fmt.Errorf("%w: quota envelope must be positive in every axis", ErrManifestInvalid)
	}
	if !sha256Pattern.MatchString(m.Digest) {
		return fmt.Errorf("%w: artifact digest must be sha256-<64 hex>", ErrManifestInvalid)
	}
	return nil
}

// MarkVerified records a completed artifact-integrity verification
// (C4-22): the verified digest must equal the manifest digest, or the
// verification is refused.
func MarkVerified(rec *PluginRecord, digest string, now time.Time) error {
	if digest != rec.Manifest.Digest {
		return fmt.Errorf("%w: verified digest does not match the manifest artifact digest", ErrLifecycle)
	}
	rec.VerifiedDigest = digest
	return TransitionPlugin(rec, LifecycleVerified, now)
}

// TransitionPlugin applies one lifecycle transition under the accepted
// table. Revocation is terminal; an ENABLE transition is refused unless
// the artifact digest has actually been verified (C4-26).
func TransitionPlugin(rec *PluginRecord, to string, now time.Time) error {
	allowed, ok := lifecycleTransitions[rec.State]
	if !ok || !allowed[to] {
		return fmt.Errorf("%w: %s -> %s", ErrLifecycle, rec.State, to)
	}
	if to == LifecycleEnabled && rec.VerifiedDigest != rec.Manifest.Digest {
		return fmt.Errorf("%w: enable requires a completed artifact verification", ErrLifecycle)
	}
	rec.State = to
	rec.UpdatedAt = now.UTC()
	return nil
}

// Revoke is the terminal trust action (C4-26): after this, every host call
// is denied and the record can never return to service.
func Revoke(rec *PluginRecord, now time.Time) error {
	return TransitionPlugin(rec, LifecycleRevoked, now)
}
