package app

import (
	"context"
	"strings"

	"github.com/qira-dev/karyora/internal/domain/errs"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/security"
)

// MaxCapabilityModelRunes bounds the optional model query value the
// discovery read resolves its effective profile through (the
// implementation's own shape; the canonical bound resolves at the
// publication act per the O-1 precedent).
const MaxCapabilityModelRunes = 256

// CapabilityDescriptor is the F2 family's capability descriptor (the
// adopted S10 supply's P-S10-01 contract class as implemented, IMPL-22):
// the Core's OWN coded capability state for one wired provider identity —
// the coded CapabilitySet verbatim (the eleven Feature axes as
// supported/unsupported/unknown three-state facts — "unknown means
// unsupported wherever it gates a call"; the limits members 0 = unknown;
// the refreshable-metadata notes law, contract.go:426-444) plus the
// identity members the read keys on. The implementation serves the Core's
// own coded capability state as the source of truth (the S10 §5 gap 2's
// static-coded class, typed honestly); the canonical source-of-truth
// class and the descriptor's keying (per profile / per Model ID / per
// pair — the three recorded states carried exactly as their records state
// them) resolve at the publication act per the O-1 precedent. The
// descriptor carries FACTS, never promises (C3-29's frozen boundary:
// compatibility is a tested profile, never a promise of full native
// equivalence — every unoverridden axis stays unknown). The twelve-field
// matrix's source/verification-state/freshness members are NOT here: the
// coded vocabulary does not carry them (the S10 §5 gap 3 stays its
// recorded owner's) and each field's presence stays Unverified exactly as
// the P3.2 §5.1 audit verdict records — nothing beyond what the Core's
// coded Capabilities() supplies is populated.
type CapabilityDescriptor struct {
	// Provider is the wired provider identity the read keys on (the same
	// id vocabulary the save/test commands send — the coded adapter
	// register's ids).
	Provider string `json:"provider"`
	// Profile is the effective profile the facts resolve through (the
	// coded ProfileFor resolution — the review field 3's own pattern; the
	// optional model value rides it, the registered default profile when
	// the caller supplies none).
	Profile dprovider.ProfileID `json:"profile"`
	// Capabilities is the coded CapabilitySet verbatim — the same tested
	// claims the fail-closed gate consults (unknown means unsupported).
	Capabilities dprovider.CapabilitySet `json:"capabilities"`
}

// ProviderCapabilities is the F2 family's discovery/status read (the
// adopted S10 supply's P-S10-02 contract class as implemented, IMPL-22):
// a Core, permission-filtered, per-provider read returning the capability
// descriptor WITHOUT running any network probe — the standing/status
// class beside the implemented M-25 test (the probe-bearing discovery
// surface stays exactly as coded; this read is ADDITIVE to it, never a
// replacement, never a re-decision of the M-25 contract). The
// existence-vs-test-only disposition the S10 §5 gap 4 names is typed
// honestly HERE: a real standing query is delivered, not test-only — the
// canonical disposition resolves at the publication act per the O-1
// precedent. The read decides through the one decision port with the
// coded read posture the connection/configuration reads decide through
// (ResourceConfig + ActionDataRead). The no-leak rule binds the refusals:
// a provider outside the wired set is the typed NOT_FOUND — never a
// distinguishing answer, never an invented descriptor, never a
// name-inferred capability (UI-13; the unknown-fields rule).
func (s *Slice) ProviderCapabilities(ctx context.Context, providerID, modelID string) (CapabilityDescriptor, error) {
	corr := errs.NewCorrelationID()
	if err := s.authorize(ctx, corr, security.Resource{Type: security.ResourceConfig, ID: providerID}, security.ActionDataRead); err != nil {
		return CapabilityDescriptor{}, err
	}
	if len([]rune(modelID)) > MaxCapabilityModelRunes {
		return CapabilityDescriptor{}, envelopeErr(errs.CodeValidationFailed, corr, map[string]any{"field": "model"})
	}
	executor, wired := s.deps.Providers[providerID]
	if !wired {
		// The no-leak rule: an identity outside the wired set is the typed
		// NOT_FOUND (the same non-distinguishing phrase the submit path's
		// fail-fast uses), never an invented descriptor and never a
		// capability inferred from the name (UI-13).
		return CapabilityDescriptor{}, envelopeErr(errs.CodeNotFound, corr, map[string]any{"reason": "provider is not wired for this core"})
	}
	model := modelID
	if strings.TrimSpace(model) == "" {
		model = "default"
	}
	profile := executor.ProfileFor(model)
	return CapabilityDescriptor{
		Provider:     providerID,
		Profile:      profile,
		Capabilities: executor.Capabilities(profile),
	}, nil
}
