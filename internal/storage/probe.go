package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"path/filepath"

	"github.com/mattn/go-sqlite3"
)

// readonlyConnector opens one read-only connection for the doctor probe.
type readonlyConnector struct {
	dsn string
}

func (c readonlyConnector) Connect(ctx context.Context) (driver.Conn, error) {
	d := &sqlite3.SQLiteDriver{}
	return d.Open(c.dsn)
}

func (c readonlyConnector) Driver() driver.Driver { return &sqlite3.SQLiteDriver{} }

// ProbeReadonly runs doctor's read-only storage check: open the database
// read-only, run PRAGMA integrity_check, and report the applied schema
// version. doctor never repairs; corruption surfaces as an error for the
// quarantine path (C2-07 §2, C2-11 §2).
func ProbeReadonly(ctx context.Context, path string) (schemaVersion int64, integrity string, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return 0, "", fmt.Errorf("storage: resolve %s: %w", path, err)
	}
	db := sql.OpenDB(readonlyConnector{dsn: "file:" + abs + "?mode=ro"})
	defer db.Close()
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return 0, "", fmt.Errorf("storage: integrity check: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return 0, "", fmt.Errorf("storage: integrity check: %w", err)
		}
		if result != "ok" {
			return 0, result, fmt.Errorf("storage: integrity_check reported %q", result)
		}
		integrity = result
	}
	if err := rows.Err(); err != nil {
		return 0, "", fmt.Errorf("storage: integrity check: %w", err)
	}
	row := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`)
	var hasTable int
	if err := row.Scan(&hasTable); err != nil {
		return 0, integrity, fmt.Errorf("storage: schema probe: %w", err)
	}
	if hasTable == 0 {
		return 0, integrity, nil
	}
	err = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE state = 'applied'`).Scan(&schemaVersion)
	if err != nil {
		return 0, integrity, fmt.Errorf("storage: schema version: %w", err)
	}
	return schemaVersion, integrity, nil
}
