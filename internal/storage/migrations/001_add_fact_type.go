package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddFactType, downAddFactType)
}

func upAddFactType(ctx context.Context, tx *sql.Tx) error {
	// Check if column already exists (idempotent)
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('observations') WHERE name='fact_type'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil // Column already exists
	}

	_, err = tx.ExecContext(ctx, `
		ALTER TABLE observations ADD COLUMN fact_type TEXT DEFAULT 'dynamic'
	`)
	return err
}

func downAddFactType(ctx context.Context, tx *sql.Tx) error {
	return nil
}
