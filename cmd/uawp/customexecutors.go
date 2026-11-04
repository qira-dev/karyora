package main

import (
	"github.com/qira-dev/karyora/internal/app"
	dprovider "github.com/qira-dev/karyora/internal/domain/provider"
	"github.com/qira-dev/karyora/internal/providers"
	"github.com/qira-dev/karyora/internal/security"
)

// customExecutorFactory is the composition root's per-connection
// custom-executor construction (IMPL-18). The coded custom
// connection-protocol adapters (C3-29/C3-30 — NewCustomOpenAIAdapter /
// NewCustomAnthropicAdapter) are parameterized by the connection record's
// own non-secret posture (the explicit base URL, the exact model id), so
// an executor for them cannot pre-exist in the wired provider map: it is
// built per connection, here, at the composition root — the only place
// that may import both the app port and the providers package (the
// dependency map's own rule: internal/app never imports
// internal/providers; the construction crosses the layer boundary through
// the composition-provided app.ConnectionExecutorFactory port).
//
// Every constructed executor rides the SAME posture the statically wired
// executors carry: the local-core egress policy, the OS-keyring credential
// store (D-37 — one store, both faces), and the shared C3-19 registry.
// Construction seeds the adapter's own per-profile data-handling record
// (the honest unknown-base record for custom endpoints: "the pointed-at
// vendor's policy does not transfer to this profile; a per-connection
// dated review is required") so the fail-closed egress gate keys on it
// exactly as for every profile — the gate itself and the
// classification-policy allowlists are untouched (their recorded owner's).
//
// No capability override is supplied: the connection record carries no
// capability-override field (the UI-12 LOCKED Custom API field set), so a
// constructed custom executor claims ONLY what its coded contract states
// (chat) and every unoverridden axis stays unknown/unsupported —
// "compatibility is a tested profile, never a promise of full native
// equivalence" (C3-29's own rule, fail closed).
type customExecutorFactory struct {
	policy  providers.EgressPolicy
	secrets security.SecretStore
	reg     *dprovider.Registry
}

// newCustomExecutorFactory builds the factory over the composition's
// shared posture (the same policy, keyring store, and registry the
// statically wired executors carry — wireJobSlice's own values).
func newCustomExecutorFactory(secrets security.SecretStore, reg *dprovider.Registry) customExecutorFactory {
	return customExecutorFactory{
		policy:  providers.DefaultEgressPolicy(providers.CoreLocal),
		secrets: secrets,
		reg:     reg,
	}
}

// IsConnectionProtocol names the coded custom connection-protocol families
// (the internal/domain/provider contract.go ids): exactly the two adapters
// the providers package codes — no other id is constructible, and the
// statically wired ids never reach this port (the provider map answers
// first).
func (f customExecutorFactory) IsConnectionProtocol(provider string) bool {
	switch provider {
	case dprovider.ProviderCustomOpenAI, dprovider.ProviderCustomAnthropic:
		return true
	}
	return false
}

// ConnectionExecutor builds the per-connection executor for one custom
// posture. ok=false for any id outside the coded custom families (the
// caller's typed wired-check refusal stays for the genuinely-unwired
// case) — a construction failure refuses the same way rather than
// returning a half-built executor.
func (f customExecutorFactory) ConnectionExecutor(provider, baseURL, modelID string) (app.ProviderExecutor, bool) {
	if !f.IsConnectionProtocol(provider) {
		return nil, false
	}
	var a providers.Adapter
	switch provider {
	case dprovider.ProviderCustomOpenAI:
		a = providers.NewCustomOpenAIAdapter(baseURL, modelID, nil)
	case dprovider.ProviderCustomAnthropic:
		a = providers.NewCustomAnthropicAdapter(baseURL, modelID, "", nil)
	default:
		return nil, false
	}
	// The honest per-profile record rides the shared registry (thread-safe
	// upsert; the unknown-base record is idempotent under repeated
	// construction of the same profile).
	f.reg.Upsert(a.SeedDataHandling(a.DefaultProfile(modelID)))
	exec, err := providers.NewExecutor(a, f.policy, f.secrets, f.reg)
	if err != nil {
		return nil, false
	}
	return exec, true
}
