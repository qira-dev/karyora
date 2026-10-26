package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qira-dev/karyora/internal/domain/errs"
	"github.com/qira-dev/karyora/internal/platform"
)

// CAS is the local content-addressed store (C2-12): algorithm-qualified
// `sha256:<lowercase-hex>` object names over the canonical bytes, fan-out
// layout `objects/<2>/<2>/<full-hex>` under the data directory, a separate
// staging directory for in-flight writes, fail-closed reads, and
// reference-aware mark/sweep garbage collection. Product data owners
// register their reference types here but bring none of their semantics
// into Phase 2.
type CAS struct {
	root           string // <data>/cas
	staging        string // <data>/cas/staging
	objects        string // <data>/cas/objects
	maxObjectBytes int64
	maxObjects     int64
	grace          time.Duration

	count atomic.Int64

	mu    sync.Mutex
	roots map[string]func(ctx context.Context) (map[string]bool, error)
}

// DigestPrefix is the only algorithm in the additive naming scheme (C2-12
// §1); a future algorithm change adds a prefix, never reinterprets old IDs.
const DigestPrefix = "sha256:"

// DigestOf returns the content-addressed ID for the canonical bytes.
func DigestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return DigestPrefix + hex.EncodeToString(sum[:])
}

// NewCAS builds the store under <data>/cas and walks the object count once
// (bounded by the local store size at startup).
func NewCAS(root string, maxObjectBytes, maxObjects int64, grace time.Duration) *CAS {
	c := &CAS{
		root:           root,
		staging:        filepath.Join(root, "staging"),
		objects:        filepath.Join(root, "objects"),
		maxObjectBytes: maxObjectBytes,
		maxObjects:     maxObjects,
		grace:          grace,
		roots:          map[string]func(ctx context.Context) (map[string]bool, error){},
	}
	return c
}

// Init creates the layout with least-privilege modes (C2-06/C2-12 §1: 0700
// dirs, 0600 objects; no traversal, no symlinks).
func (c *CAS) Init() error {
	for _, d := range []string{c.root, c.staging, c.objects} {
		if err := platform.EnsureDir(d, 0o700); err != nil {
			return err
		}
	}
	return c.recount()
}

func (c *CAS) recount() error {
	var count int64
	err := filepath.WalkDir(c.objects, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			count++
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.count.Store(count)
	return nil
}

// objectPath maps a digest to its fan-out path, validating the grammar so
// no traversal or foreign name can reach the filesystem.
func (c *CAS) objectPath(digest string) (string, error) {
	if !eventsDigestGrammar(digest) {
		return "", errStoragef(errs.CodeValidationFailed, "digest %q does not match the grammar", digest)
	}
	hexPart := strings.TrimPrefix(digest, DigestPrefix)
	return filepath.Join(c.objects, hexPart[0:2], hexPart[2:4], hexPart), nil
}

// eventsDigestGrammar mirrors the additive digest grammar (kept local so
// the CAS does not import the events package for one predicate).
func eventsDigestGrammar(s string) bool {
	if !strings.HasPrefix(s, DigestPrefix) {
		return false
	}
	h := strings.TrimPrefix(s, DigestPrefix)
	if len(h) != 64 {
		return false
	}
	for _, ch := range h {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return false
		}
	}
	return true
}

// Put runs the staged-write protocol (C2-12 §2): stage → verify → publish
// → (reference commit happens only after publish, in the caller's
// transaction). Crash windows: stage (orphan staged file, collected after
// a bounded grace), publish (orphan object, collectible by GC), reference
// (impossible by invariant).
func (c *CAS) Put(ctx context.Context, data []byte) (string, error) {
	if int64(len(data)) > c.maxObjectBytes {
		return "", errStoragef(errs.CodeRateLimited, "object exceeds the configured maximum (%d bytes)", c.maxObjectBytes)
	}
	if c.maxObjects > 0 && c.count.Load() >= c.maxObjects {
		return "", errStoragef(errs.CodeStorageUnavailable, "object count bound reached")
	}
	digest := DigestOf(data)
	if err := platform.EnsureDir(c.staging, 0o700); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "staging dir: %v", err)
	}
	// Step 1 — stage: full bytes to staging/<uuid>.tmp (0600), fsync the
	// file. A crash here leaves an orphaned staged file; startup
	// reconciliation collects stale staging after a bounded grace.
	tmp, err := os.CreateTemp(c.staging, "*.tmp")
	if err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "stage: %v", err)
	}
	stagedPath := tmp.Name()
	defer os.Remove(stagedPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", errStoragef(errs.CodeStorageUnavailable, "stage write: %v", err)
	}
	if err := platform.SyncFile(tmp); err != nil {
		tmp.Close()
		return "", errStoragef(errs.CodeStorageUnavailable, "stage fsync: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "stage close: %v", err)
	}

	// Step 2 — verify: recompute the digest over the staged bytes.
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "stage read-back: %v", err)
	}
	if DigestOf(staged) != digest {
		// Mismatch deletes the staged file and returns a
		// STATE_CORRUPT-family error (C2-12 §2 step 2).
		return "", errStoragef(errs.CodeStateCorrupt, "staged bytes do not verify against the digest")
	}

	// Step 3 — publish: atomic rename on the same filesystem/volume, then
	// per-target durability (POSIX: fsync the parent directory).
	target, err := c.objectPath(digest)
	if err != nil {
		return "", err
	}
	if err := platform.EnsureDir(filepath.Dir(target), 0o700); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "fan-out dir: %v", err)
	}
	if err := os.Chmod(stagedPath, 0o600); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "stage chmod: %v", err)
	}
	if _, err := os.Stat(target); err == nil {
		// Concurrent writes of the same digest converge idempotently:
		// publish is idempotent by content address (C2-12 §5).
		return digest, nil
	}
	if err := os.Rename(stagedPath, target); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "publish: %v", err)
	}
	if err := platform.SyncDir(filepath.Dir(target)); err != nil {
		return "", errStoragef(errs.CodeStorageUnavailable, "publish dir fsync: %v", err)
	}
	c.count.Add(1)
	return digest, nil
}

// Read re-verifies the digest before returning the bytes: a missing or
// mismatched referenced object is STATE_CORRUPT — surfaced for the recovery
// path, never silently replaced or ignored (C2-12 §3).
func (c *CAS) Read(ctx context.Context, digest string) ([]byte, error) {
	path, err := c.objectPath(digest)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errStoragef(errs.CodeStateCorrupt, "referenced object %s is missing", digest)
		}
		return nil, errStoragef(errs.CodeStorageUnavailable, "open object: %v", err)
	}
	defer f.Close()
	limited := io.LimitReader(f, c.maxObjectBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, errStoragef(errs.CodeStorageUnavailable, "read object: %v", err)
	}
	if int64(len(data)) > c.maxObjectBytes {
		return nil, errStoragef(errs.CodeStateCorrupt, "object %s exceeds the configured maximum", digest)
	}
	if DigestOf(data) != digest {
		return nil, errStoragef(errs.CodeStateCorrupt, "object %s fails digest verification", digest)
	}
	return data, nil
}

// Stat reports whether an object exists.
func (c *CAS) Stat(digest string) (bool, error) {
	path, err := c.objectPath(digest)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// RegisterReferenceType registers a reachability root owner (C2-12 §4):
// unregistered reachability is not collectible, so every future module
// owner must register before its data can ever be collected.
func (c *CAS) RegisterReferenceType(name string, list func(ctx context.Context) (map[string]bool, error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.roots[name] = list
}

// ReconcileStaging collects stale staged files after a crash (C2-12 §2
// crash window 1: an orphaned staged file is collected after a bounded
// grace). A staged file never carries an intended digest — verification is
// protocol step 2, which the dead writer never reached — so partial bytes
// are never published as objects; they are discarded and the next real
// Put re-runs the full staged-write protocol. Staged files inside the
// write window (recent mtime) are left alone so a live writer is never
// disturbed. The documented staged-upload DB-state alternative for large
// streams carries its own expected digest and completes through that
// record; Phase 2 has no such records yet.
func (c *CAS) ReconcileStaging(ctx context.Context) (published, discarded int, err error) {
	entries, err := os.ReadDir(c.staging)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, errStoragef(errs.CodeStorageUnavailable, "staging scan: %v", err)
	}
	const liveWindow = 5 * time.Minute
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(c.staging, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) < liveWindow {
			continue // possibly a live writer's staged file
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			continue
		}
		discarded++
	}
	_ = published
	return published, discarded, nil
}

// GCResult is the observable GC outcome (C2-12 §4: candidate/deleted counts
// and bytes surface to health).
type GCResult struct {
	Candidates int   `json:"candidates"`
	Deleted    int   `json:"deleted"`
	Bytes      int64 `json:"bytes"`
}

// Collect runs one bounded mark/sweep pass: mark from registered roots →
// mark unreferenced objects → recheck reachability immediately before
// deletion for objects past the grace period → delete. GC is idempotent,
// batch-bounded, and never touches staged/in-flight objects or anything
// whose owning module has not registered a reference type.
func (c *CAS) Collect(ctx context.Context, batch int) (GCResult, error) {
	reachable, err := c.reachable(ctx)
	if err != nil {
		return GCResult{}, err
	}
	result := GCResult{}
	entries, err := os.ReadDir(c.objects)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return result, errStoragef(errs.CodeStorageUnavailable, "objects scan: %v", err)
	}
	for _, shard1 := range entries {
		if result.Deleted >= batch {
			break
		}
		if !shard1.IsDir() || len(shard1.Name()) != 2 {
			continue
		}
		shard2s, err := os.ReadDir(filepath.Join(c.objects, shard1.Name()))
		if err != nil {
			continue
		}
		for _, shard2 := range shard2s {
			if !shard2.IsDir() || len(shard2.Name()) != 2 {
				continue
			}
			files, err := os.ReadDir(filepath.Join(c.objects, shard1.Name(), shard2.Name()))
			if err != nil {
				continue
			}
			for _, f := range files {
				if f.IsDir() || !eventsDigestGrammar(DigestPrefix+f.Name()) {
					continue
				}
				digest := DigestPrefix + f.Name()
				if reachable[digest] {
					continue
				}
				result.Candidates++
				info, err := f.Info()
				if err != nil {
					continue
				}
				if time.Since(info.ModTime()) < c.grace {
					continue // inside the grace window: never collected
				}
				// Second pass: reachability is rechecked immediately
				// before deletion (C2-12 §4).
				fresh, err := c.reachable(ctx)
				if err != nil {
					return result, err
				}
				if fresh[digest] {
					continue
				}
				path, _ := c.objectPath(digest)
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					continue
				}
				result.Deleted++
				result.Bytes += info.Size()
				c.count.Add(-1)
				if result.Deleted >= batch {
					break
				}
			}
		}
	}
	return result, nil
}

// reachable unions every registered root's reference set.
func (c *CAS) reachable(ctx context.Context) (map[string]bool, error) {
	c.mu.Lock()
	roots := make(map[string]func(ctx context.Context) (map[string]bool, error), len(c.roots))
	for k, v := range c.roots {
		roots[k] = v
	}
	c.mu.Unlock()
	set := map[string]bool{}
	for name, list := range roots {
		refs, err := list(ctx)
		if err != nil {
			return nil, fmt.Errorf("cas: reachability root %q: %w", name, err)
		}
		for r := range refs {
			set[r] = true
		}
	}
	return set, nil
}

// IsNotExistError reports whether err is the missing-object corruption path.
func IsNotExistError(err error) bool {
	var se *Error
	if errors.As(err, &se) {
		return se.Code == errs.CodeStateCorrupt && strings.Contains(se.Detail, "missing")
	}
	return false
}
