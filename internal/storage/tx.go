package storage

import (
	"database/sql"
	"encoding/json"
)

// sqlTx wraps one database transaction with typed helpers. Transactions are
// short and database-only (C2-10 §4): no caller may hold one across a
// provider/plugin/network call, filesystem wait, or user interaction — the
// runtime ports and the commit helpers are the only sanctioned users.
type sqlTx struct {
	tx *sql.Tx
}

// Exec runs a statement inside the transaction.
func (t *sqlTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.tx.Exec(query, args...)
}

// QueryRow runs a single-row query inside the transaction.
func (t *sqlTx) QueryRow(query string, args ...any) *sql.Row {
	return t.tx.QueryRow(query, args...)
}

// Query runs a query inside the transaction.
func (t *sqlTx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(query, args...)
}

// JSON marshals v for storage; a marshal failure is a contract violation
// and fails the transaction, never silently degrades.
func JSON(v any) ([]byte, error) {
	return json.Marshal(v)
}
