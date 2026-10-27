package workflow

import (
	"fmt"
	"strings"
)

// Declared capabilities and the policy boundary (C4-06). The core rule:
// A REQUESTED CAPABILITY IS NOT A GRANT. Definitions may only declare what
// they need; Core policy resolves every request deny-by-default at
// execution time. Grants cannot come from inside a definition, from model
// output, or from retrieved content — structurally, they only arrive
// through the PolicyInput parameter owned by Core.

// workCapabilityPrefix is refused unconditionally (D-12/D-33): Work-scoped
// capabilities stay out of Core Phase 4 until the owner opens that gate.
const workCapabilityPrefix = "work."

// Denial records one refused capability request with its safe reason.
type Denial struct {
	Node       string `json:"node"`
	Capability string `json:"capability"`
	Reason     string `json:"reason"`
}

// PolicyInput is the Core-owned resolution input: explicit grants for
// named capabilities. Nothing else can authorize.
type PolicyInput struct {
	// Grants maps capability name -> allowed. Absent means denied.
	Grants map[string]bool
	// Approver records which Core policy version/input produced this
	// resolution (audit trail; never model output).
	Approver string
}

// ResolveCapabilities walks every node's declared requests and returns the
// per-node granted set plus every denial. Deny-by-default: an unlisted
// capability, an unknown name, and anything Work-scoped is denied.
func ResolveCapabilities(d *Definition, policy PolicyInput) (granted map[string][]CapabilityRequest, denied []Denial) {
	granted = map[string][]CapabilityRequest{}
	for _, n := range d.Nodes {
		for _, req := range n.RequiredCapabilities {
			switch {
			case strings.HasPrefix(req.Name, workCapabilityPrefix):
				denied = append(denied, Denial{Node: n.Name, Capability: req.Name,
					Reason: "work-scoped capability is excluded pending D-12/D-33"})
			case policy.Grants[req.Name]:
				granted[n.Name] = append(granted[n.Name], req)
			default:
				reason := "deny-by-default: no policy grant for this capability"
				if req.DataClassification != "" && len(policy.Grants) == 0 {
					reason = "deny-by-default: policy input carries no grants at all"
				}
				denied = append(denied, Denial{Node: n.Name, Capability: req.Name, Reason: reason})
			}
		}
	}
	return granted, denied
}

// AssertPolicyBoundary is a compile-time honesty check used by Compile: a
// resolution with an empty approver is not a policy decision and is
// refused, so grants can never silently appear from nowhere.
func AssertPolicyBoundary(policy PolicyInput) error {
	if strings.TrimSpace(policy.Approver) == "" {
		return fmt.Errorf("%w: capability resolution requires a recorded Core policy approver (grants never originate in definitions or model output)", ErrCapabilityDenied)
	}
	return nil
}
