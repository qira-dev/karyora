// Package events owns event journal, transactional outbox, and opaque cursor
// semantics: the event record fields, per-aggregate monotonic sequences,
// at-least-once delivery with idempotent consumers, and cursor expiry/gap
// behavior. It may import internal/domain only. Persistence implementations
// live under internal/storage.
package events
