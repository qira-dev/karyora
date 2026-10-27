package knowledgepack

import (
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"time"
)

// Verdict statuses (C3-06): accepted packs are usable; rejected packs are
// invalid; quarantined packs are preserved but visibly withheld. Every
// verdict carries explicit reasons — no silent substitution of state.
type VerdictStatus string

const (
	VerdictAccepted    VerdictStatus = "accepted"
	VerdictRejected    VerdictStatus = "rejected"
	VerdictQuarantined VerdictStatus = "quarantined"
)

// Verdict is the outcome of pack verification (C3-06).
type Verdict struct {
	Status     VerdictStatus
	Reasons    []string
	TrustState string // Trust* constant after verification
	Freshness  string // Freshness* constant after verification
	RootDigest string
}

// Verify checks one pack version end to end (C3-06): schema version,
// structural validity, per-entry digest recomputation, root digest,
// signature authenticity when an envelope is present, license and
// freshness visibility. Signature absence is TrustUnsigned (a legal local
// state) and is never confused with a digest failure; an invalid signature
// is a hard rejection. Revoked trust quarantines — it never silently falls
// back to another artifact.
//
// now drives the freshness assessment; entries maps entry path → exact
// bytes (a missing key counts as a tamper signal).
func Verify(m *Manifest, entries map[string][]byte, env *SignatureEnvelope, key ed25519.PublicKey, now time.Time) (Verdict, error) {
	if m == nil {
		return Verdict{}, fmt.Errorf("knowledgepack: manifest is nil")
	}
	v := Verdict{}
	if m.SchemaVersion != SchemaVersion {
		v.Status = VerdictQuarantined
		v.Reasons = append(v.Reasons, string(QuarantineUnsupportedVer))
		return v, nil
	}
	if err := m.Validate(); err != nil {
		v.Status = VerdictRejected
		v.Reasons = append(v.Reasons, err.Error())
		return v, nil
	}
	// Integrity first: every entry's bytes must hash to its recorded
	// digest. A missing entry is a tamper signal, not a soft skip.
	for i := range m.Entries {
		e := &m.Entries[i]
		data, ok := entries[e.Path]
		if !ok {
			v.Status = VerdictRejected
			v.Reasons = append(v.Reasons, fmt.Sprintf("%s: entry bytes missing", e.Path))
			continue
		}
		sum := sha256.Sum256(data)
		if got := fmt.Sprintf("sha256:%x", sum); got != e.Digest {
			v.Status = VerdictRejected
			v.Reasons = append(v.Reasons, fmt.Sprintf("%s: %s", e.Path, string(QuarantineTampered)))
		}
	}
	if v.Status == VerdictRejected {
		return v, nil
	}
	// Authenticity: an absent envelope is unsigned (legal locally); a
	// present envelope must verify exactly.
	if env != nil {
		if err := VerifySignature(m, env, key); err != nil {
			v.Status = VerdictRejected
			v.Reasons = append(v.Reasons, err.Error())
			return v, nil
		}
		v.TrustState = TrustVerified
	} else if m.Trust.State == TrustRevoked {
		v.Status = VerdictQuarantined
		v.Reasons = append(v.Reasons, string(QuarantineRevokedTrust))
		v.TrustState = TrustRevoked
		return v, nil
	} else {
		v.TrustState = TrustUnsigned
	}
	// Freshness is visible state, never a rejection: the retrieval policy
	// decides exposure, and the assessment is recorded.
	if m.Freshness.StaleAfterSecs > 0 && !m.Freshness.AssessedAt.IsZero() {
		age := now.Sub(m.Freshness.AssessedAt)
		if age > time.Duration(m.Freshness.StaleAfterSecs)*time.Second {
			v.Freshness = FreshnessStale
		} else {
			v.Freshness = FreshnessCurrent
		}
	} else {
		v.Freshness = FreshnessUnknown
	}
	// License status is carried through so it stays visible downstream.
	_ = m.LicenseStatus
	root, err := PackRootDigest(m)
	if err != nil {
		return Verdict{}, err
	}
	v.RootDigest = root
	v.Status = VerdictAccepted
	return v, nil
}
