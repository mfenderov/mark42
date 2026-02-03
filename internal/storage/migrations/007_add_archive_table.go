package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddArchiveTable, downAddArchiveTable)
}

func upAddArchiveTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		-- Archive table for decayed/old observations
		CREATE TABLE IF NOT EXISTS archived_observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			original_entity_id INTEGER NOT NULL,
			entity_name TEXT NOT NULL,
			content TEXT NOT NULL,
			fact_type TEXT DEFAULT 'dynamic',
			importance REAL DEFAULT 1.0,
			archived_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_archived_entity ON archived_observations(entity_name);
		CREATE INDEX IF NOT EXISTS idx_archived_date ON archived_observations(archived_at);
	`)
	return err
}

func downAddArchiveTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
		DROP TABLE IF EXISTS archived_observations;
	`)
	return err
}
