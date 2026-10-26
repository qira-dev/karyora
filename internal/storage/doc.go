// Package storage implements the storage ports for the locked local stack:
// embedded SQLite (proposed driver D-09 with CGO, one authority per data
// directory, WAL never on a network filesystem) and the local CAS with
// digest verification. It is an adapter package: it depends inward on
// domain, events, and module-package ports; database drivers are imported
// HERE only.
package storage
