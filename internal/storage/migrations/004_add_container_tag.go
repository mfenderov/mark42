package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddContainerTag, downAddContainerTag)
}

func upAddContainerTag(ctx context.Context, tx *sql.Tx) error {
	// Check if column exists
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('entities') WHERE name='container_tag'
	`).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		_, err = tx.ExecContext(ctx, `ALTER TABLE entities ADD COLUMN container_tag TEXT`)
		if err != nil {
			return err
		}
	}

	// Create index if not exists (always safe)
	_, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_entities_container ON entities(container_tag)`)
	return err
}

func downAddContainerTag(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_entities_container`)
	return err
}
