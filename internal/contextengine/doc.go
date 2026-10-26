// Package contextengine owns compiled context truth: the retrieval pipeline
// and immutable ContextSnapshot records with selections, omissions, and
// provenance. It may import internal/domain and internal/events only. It
// must never present compiled context as canonical knowledge truth.
package contextengine
