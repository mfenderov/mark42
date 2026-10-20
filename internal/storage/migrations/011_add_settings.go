package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddSettings, downAddSettings)
}

func upAddSettings(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func downAddSettings(ctx context.Context, tx *sql.Tx) error {
	// SQLite does not support DROP TABLE in a cascading manner similar to other databases.
	// For safety and consistency with other migrations, down is intentionally a no-op.
	return nil
}
