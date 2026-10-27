package knowledgepack

import (
	"context"
	"fmt"
	"time"
)

// Pack lifecycle statuses in the store (C3-06): a quarantined pack is
// preserved and inspectable but never silently served to retrieval.
const (
	PackStatusActive      = "active"
	PackStatusQuarantined = "quarantined"
	PackStatusRejected    = "rejected"
)

// StoredPack is one immutable stored pack version plus its disposition
// (C3-06). The manifest and entries never change after save; only the
// status/reason disposition moves, and every move is visible.
type StoredPack struct {
	Manifest     *Manifest
	RootDigest   string
	Status       string
	StatusReason string
	StoredAt     time.Time
	Entries      map[string][]byte
}

// ValidateStored checks the disposition invariants before persistence.
func (p *StoredPack) ValidateStored() error {
	if p.Manifest == nil {
		return fmt.Errorf("knowledgepack: stored manifest is nil")
	}
	if err := p.Manifest.Validate(); err != nil {
		return err
	}
	switch p.Status {
	case PackStatusActive, PackStatusQuarantined, PackStatusRejected:
	default:
		return fmt.Errorf("knowledgepack: illegal pack status %q", p.Status)
	}
	if p.Status != PackStatusActive && p.StatusReason == "" {
		return fmt.Errorf("knowledgepack: a %s pack requires a visible reason", p.Status)
	}
	return nil
}

// StorePort is the persistence contract for pack versions (C3-06). The
// storage package implements it; the domain package never imports a
// database. Implementations must persist manifest, entries, and disposition
// atomically per version and must treat (pack_id, version) as immutable.
type StorePort interface {
	// SavePackVersion stores one new immutable version. Saving a version
	// that already exists is a conflict error, never an overwrite.
	SavePackVersion(ctx context.Context, pack StoredPack) error
	// LoadPackVersion loads exactly one stored version.
	LoadPackVersion(ctx context.Context, packID string, version int) (StoredPack, error)
	// LatestPackVersion returns the highest stored version (0 = none).
	LatestPackVersion(ctx context.Context, packID string) (int, error)
	// SetPackStatus moves the visible disposition of one version. The
	// manifest and entries are never touched by a status move.
	SetPackStatus(ctx context.Context, packID string, version int, status, reason string) error
	// ListPackVersions returns the stored versions in ascending order.
	ListPackVersions(ctx context.Context, packID string) ([]int, error)
}
