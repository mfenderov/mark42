package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddAccessCount, downAddAccessCount)
}

func upAddAccessCount(ctx context.Context, tx *sql.Tx) error {
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('observations') WHERE name='access_count'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Column already exists
	}

	_, err = tx.ExecContext(ctx, `ALTER TABLE observations ADD COLUMN access_count INTEGER DEFAULT 0`)
	return err
}

func downAddAccessCount(ctx context.Context, tx *sql.Tx) error {
	// SQLite does not support DROP COLUMN — down migration is intentionally a no-op.
	return nil
}
