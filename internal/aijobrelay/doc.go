// Package aijobrelay owns execution truth: the separate Job, Attempt, Lease,
// Checkpoint, side-effect intent, receipt, and authority-transfer state
// machines defined by the Phase 1 lifecycle contract (contract v1, LV-01 to
// LV-15). It may import internal/domain and internal/events only. Authority
// transfer stays unsupported behind D-38 and full D-42 evidence; Workflow
// Run execution stays behind D-40.
package aijobrelay
