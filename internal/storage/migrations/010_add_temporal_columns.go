package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddTemporalColumns, downAddTemporalColumns)
}

func upAddTemporalColumns(ctx context.Context, tx *sql.Tx) error {
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('observations') WHERE name='valid_from'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = tx.ExecContext(ctx, `ALTER TABLE observations ADD COLUMN valid_from TIMESTAMP DEFAULT NULL`)
		if err != nil {
			return err
		}
	}

	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pragma_table_info('observations') WHERE name='valid_until'
	`).Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = tx.ExecContext(ctx, `ALTER TABLE observations ADD COLUMN valid_until TIMESTAMP DEFAULT NULL`)
		if err != nil {
			return err
		}
	}

	return nil
}

func downAddTemporalColumns(ctx context.Context, tx *sql.Tx) error {
	// SQLite does not support DROP COLUMN — down migration is intentionally a no-op.
	return nil
}
