package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddImportanceDecay, downAddImportanceDecay)
}

func upAddImportanceDecay(ctx context.Context, tx *sql.Tx) error {
	columns := []string{"importance", "forget_after", "last_accessed"}

	for _, col := range columns {
		var count int
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pragma_table_info('observations') WHERE name=?
		`, col).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue // Column already exists
		}

		var ddl string
		switch col {
		case "importance":
			ddl = `ALTER TABLE observations ADD COLUMN importance REAL DEFAULT 1.0`
		case "forget_after":
			ddl = `ALTER TABLE observations ADD COLUMN forget_after TIMESTAMP`
		case "last_accessed":
			ddl = `ALTER TABLE observations ADD COLUMN last_accessed TIMESTAMP`
		}

		if _, err := tx.ExecContext(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func downAddImportanceDecay(ctx context.Context, tx *sql.Tx) error {
	return nil
}
