package plugins

import (
	"fmt"
	"sync"
)

// Scoped host-capability grants and per-call authorization (C4-23/C4-24):
// EVERY host call from a plugin passes through AuthorizeHostCall. The
// model is deny-by-default twice over — the manifest must DECLARE the
// capability, and the grant table must GRANT it with a matching scope;
// and the plugin record must be ENABLED with its artifact digest still
// verified. Revocation, disablement, or a digest mismatch denies
// everything instantly.

// HostGrant is one Core-granted (capability, scope) pair for one plugin.
type HostGrant struct {
	Plugin     string `json:"plugin"`
	Capability string `json:"capability"`
	Scope      string `json:"scope"`
}

// GrantTable is the Core-owned grant store. Grants are policy decisions,
// never plugin input.
type GrantTable struct {
	mu   sync.Mutex
	byID map[string]map[string]string // plugin -> capability -> scope
}

func NewGrantTable() *GrantTable {
	return &GrantTable{byID: map[string]map[string]string{}}
}

// Grant records one explicit scoped grant.
func (t *GrantTable) Grant(g HostGrant) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.byID[g.Plugin] == nil {
		t.byID[g.Plugin] = map[string]string{}
	}
	t.byID[g.Plugin][g.Capability] = g.Scope
}

// RevokeAll drops every grant for one plugin.
func (t *GrantTable) RevokeAll(plugin string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byID, plugin)
}

// scopeAllowed matches the granted scope: the explicit wildcard "*" grants
// a capability without namespace restriction, a trailing "/*" grants the
// whole subtree under the prefix (e.g. "/workspace/exports/*"), and
// anything else matches exactly.
func scopeAllowed(granted, requested string) bool {
	switch {
	case granted == "*":
		return true
	case len(granted) > 1 && granted[len(granted)-1] == '*' && granted[len(granted)-2] == '/':
		return len(requested) >= len(granted)-1 && requested[:len(granted)-1] == granted[:len(granted)-1]
	default:
		return granted == requested
	}
}

// AuthorizeHostCall checks ONE host call (C4-24). Order matters and is
// deny-by-default at every step: the record must be enabled with a
// verified artifact, the grant table must carry the capability for this
// plugin, and the requested scope must match the granted scope.
func AuthorizeHostCall(t *GrantTable, rec *PluginRecord, capability, scope string) error {
	if rec.State != LifecycleEnabled {
		return fmt.Errorf("%w: plugin %q is %q, not enabled", ErrHostCallDenied, rec.Manifest.Name, rec.State)
	}
	if rec.VerifiedDigest != rec.Manifest.Digest {
		return fmt.Errorf("%w: plugin %q artifact digest no longer matches its verification", ErrHostCallDenied, rec.Manifest.Name)
	}
	t.mu.Lock()
	grantedScope, ok := t.byID[rec.Manifest.Name][capability]
	t.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: no grant for capability %q", ErrHostCallDenied, capability)
	}
	if !scopeAllowed(grantedScope, scope) {
		return fmt.Errorf("%w: scope %q is outside the granted scope for %q", ErrHostCallDenied, scope, capability)
	}
	return nil
}
