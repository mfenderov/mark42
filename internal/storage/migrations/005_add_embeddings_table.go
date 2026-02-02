package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddEmbeddingsTable, downAddEmbeddingsTable)
}

func upAddEmbeddingsTable(ctx context.Context, tx *sql.Tx) error {
	// CREATE TABLE IF NOT EXISTS is safe - idempotent by design
	_, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS observation_embeddings (
			observation_id INTEGER PRIMARY KEY REFERENCES observations(id) ON DELETE CASCADE,
			embedding BLOB NOT NULL,
			model TEXT NOT NULL,
			dimensions INTEGER NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_embeddings_model ON observation_embeddings(model)`)
	return err
}

func downAddEmbeddingsTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS observation_embeddings`)
	return err
}
