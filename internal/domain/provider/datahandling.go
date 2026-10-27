package provider

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// Data-handling review rules (C3-19): a profile's claims are usable only
// while a dated official-source review exists and is fresh. The adapters
// seed each direct-provider profile with the facts its vendor policy
// states as of the workspace's dated review round (2026-10-28); facts the
// reviewed sources do not state stay unknown — and unknown profiles BLOCK
// protected data egress. No provider policy claim is fabricated here.
const (
	// ProfileRecheckInterval is the freshness window after which a verified
	// profile goes stale and blocks again (Proposed, pending owner review).
	ProfileRecheckInterval = 180 * 24 * time.Hour

	ReviewVerified = "verified"
	ReviewUnknown  = "unknown"

	// Deployment forms (C3-19): the deployment participates in the claim.
	DeploymentCloudPublic    = "cloud_public"
	DeploymentCloudDedicated = "cloud_dedicated"
	DeploymentLocal          = "local"
	DeploymentFixture        = "fixture" // recorded/mock harness endpoints only

	// Fact vocabulary: unknown is a legal, honest value — it always blocks
	// protected data.
	FactUnknown = "unknown"
	FactNo      = "none"
	FactYes     = "yes"

	// ClassificationTestFixture is the workspace's own recorded
	// classification for recorded/mock fixtures. Only the mock adapter's
	// profile allows it; no real provider does (no dated source check).
	ClassificationTestFixture = "test-fixture"

	// ClassificationNonPersonal is the v1 egress label for content the user
	// affirmatively asserts carries no personal data and no
	// third-party-confidential material (owner decision 2026-10-28, Option
	// A: "Core Egress Classification Policy Decision Record"). Only a
	// profile with a current dated review, all six facts known, and
	// training_use=none may allow it; `personal`, unlabeled, and
	// unrecognized labels never egress to a third-party cloud provider in
	// v1 (the empty/other allowlists keep them fail-closed).
	ClassificationNonPersonal = "non-personal"
)

// DataHandlingProfile is the per-profile provider data-handling record
// (C3-19): deployment, endpoint, model, feature scope, region, account/tier
// assumptions, training use, abuse retention, application-state TTL,
// subprocessors, deletion/ZDR eligibility, policy source/version, review
// date, and the allowed workspace classifications. Vendor changes
// invalidate the affected profile: a new review overwrites the record.
type DataHandlingProfile struct {
	Profile                ProfileID `json:"profile"`
	FeatureScope           Feature   `json:"feature_scope,omitempty"` // "" = whole profile
	Deployment             string    `json:"deployment"`              // cloud_public | cloud_dedicated | local | unknown
	AccountTier            string    `json:"account_tier,omitempty"`
	TrainingUse            string    `json:"training_use"`          // none | yes | unknown
	AbuseRetention         string    `json:"abuse_retention"`       // none | bounded | unknown
	ApplicationStateTTL    string    `json:"application_state_ttl"` // none | bounded | unknown
	Subprocessors          string    `json:"subprocessors"`         // none | listed | unknown
	DeletionEligibility    string    `json:"deletion_eligibility"`  // zdr | deletion_supported | unknown
	PolicyURL              string    `json:"policy_url,omitempty"`
	PolicyVersion          string    `json:"policy_version,omitempty"`
	ReviewDate             time.Time `json:"review_date,omitempty"` // zero = never reviewed here
	Reviewer               string    `json:"reviewer,omitempty"`
	AllowedClassifications []string  `json:"allowed_classifications"`
	Notes                  string    `json:"notes,omitempty"`
}

// Review reports the honest review status of the record at time now:
// verified only when a dated review exists and is inside the recheck
// window; unknown otherwise (never reviewed here, or stale).
func (p *DataHandlingProfile) Review(now time.Time) string {
	if p.ReviewDate.IsZero() || p.Reviewer == "" {
		return ReviewUnknown
	}
	if now.Sub(p.ReviewDate) > ProfileRecheckInterval {
		return ReviewUnknown // stale profiles block exactly like unknown ones
	}
	return ReviewVerified
}

// BlockingReason returns why protected data may not egress under this
// profile, or "" when the profile is current and complete for protected
// data. Every unknown fact blocks (fail closed, C3-19).
func (p *DataHandlingProfile) BlockingReason(now time.Time) string {
	if p.Review(now) != ReviewVerified {
		return "profile_has_no_current_dated_review"
	}
	for name, fact := range map[string]string{
		"deployment":            p.Deployment,
		"training_use":          p.TrainingUse,
		"abuse_retention":       p.AbuseRetention,
		"application_state_ttl": p.ApplicationStateTTL,
		"subprocessors":         p.Subprocessors,
		"deletion_eligibility":  p.DeletionEligibility,
	} {
		if fact == "" || fact == FactUnknown {
			return "profile_fact_" + name + "_unknown"
		}
	}
	if p.Deployment == DeploymentLocal && !p.Profile.ProviderLocalForm() {
		return "profile_deployment_conflict"
	}
	return ""
}

// ProviderLocalForm reports whether the profile belongs to an explicit
// local provider family (C3-30). The mock fixture family also reports true.
func (p ProfileID) ProviderLocalForm() bool {
	switch p.Provider {
	case ProviderLocal, ProviderMock:
		return true
	}
	return false
}

// EgressAllowed is the fail-closed gate (C3-19 acceptance): protected data
// egress requires a CURRENT profile whose facts are known AND whose
// allowed classifications include the data's classification. An empty
// classification is treated as protected. Unknown, stale, or conflicting
// profiles block egress with a typed reason.
func (p *DataHandlingProfile) EgressAllowed(now time.Time, classification string) error {
	if reason := p.BlockingReason(now); reason != "" {
		return &ProviderError{Kind: ErrEgressPolicy, Provider: p.Profile.Provider, Detail: reason}
	}
	if classification == "" {
		classification = "protected-unlabeled"
	}
	for _, c := range p.AllowedClassifications {
		if c == classification {
			return nil
		}
	}
	return &ProviderError{Kind: ErrEgressPolicy, Provider: p.Profile.Provider,
		Detail: "classification_not_allowed_by_profile"}
}

// Registry is the per-profile data-handling registry (C3-19). It is
// seeded by the adapters with their dated official-source records (or
// honest unknown records where no review exists) and updated only through
// recorded dated reviews.
type Registry struct {
	mu   sync.Mutex
	byID map[string][]*DataHandlingProfile
	now  func() time.Time
}

// NewRegistry builds an empty registry over the given clock.
func NewRegistry(now func() time.Time) *Registry {
	if now == nil {
		now = time.Now
	}
	return &Registry{byID: map[string][]*DataHandlingProfile{}, now: now}
}

// Upsert records (or re-records after a vendor change) one profile. The
// newest review date wins for the same (profile key, feature scope).
func (r *Registry) Upsert(p DataHandlingProfile) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := p.Profile.Key() + "|" + string(p.FeatureScope)
	list := r.byID[key]
	for i, existing := range list {
		if !p.ReviewDate.After(existing.ReviewDate) {
			// keep the existing (newer or equal) record
			return
		}
		list = append(list[:i], list[i+1:]...)
		break
	}
	r.byID[key] = append(list, &p)
}

// Lookup returns the governing record for a profile and feature scope
// (feature-specific records take precedence over whole-profile records).
func (r *Registry) Lookup(p ProfileID, feature Feature) (*DataHandlingProfile, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if list, ok := r.byID[p.Key()+"|"+string(feature)]; ok && len(list) > 0 {
		return list[0], true
	}
	if list, ok := r.byID[p.Key()+"|"]; ok && len(list) > 0 {
		return list[0], true
	}
	return nil, false
}

// EgressAllowed consults the governing record and applies the fail-closed
// gate. An unknown profile blocks (C3-19 acceptance: unknown profiles
// block protected data egress).
func (r *Registry) EgressAllowed(p ProfileID, feature Feature, classification string) error {
	rec, ok := r.Lookup(p, feature)
	if !ok {
		return &ProviderError{Kind: ErrEgressPolicy, Provider: p.Provider,
			Detail: "no_data_handling_profile_recorded"}
	}
	return rec.EgressAllowed(r.now(), classification)
}

// Profiles returns every recorded profile sorted by key (report stability).
func (r *Registry) Profiles() []*DataHandlingProfile {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*DataHandlingProfile, 0, len(r.byID))
	for _, list := range r.byID {
		out = append(out, list...)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Profile.Key(), out[j].Profile.Key()
		if a == b {
			return string(out[i].FeatureScope) < string(out[j].FeatureScope)
		}
		return strings.Compare(a, b) < 0
	})
	return out
}
