package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddVersioning, downAddVersioning)
}

func upAddVersioning(ctx context.Context, tx *sql.Tx) error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"supersedes_id", `ALTER TABLE entities ADD COLUMN supersedes_id INTEGER REFERENCES entities(id)`},
		{"is_latest", `ALTER TABLE entities ADD COLUMN is_latest BOOLEAN DEFAULT 1`},
		{"version", `ALTER TABLE entities ADD COLUMN version INTEGER DEFAULT 1`},
	}

	for _, col := range columns {
		var count int
		err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pragma_table_info('entities') WHERE name=?
		`, col.name).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue // Column already exists
		}

		if _, err := tx.ExecContext(ctx, col.ddl); err != nil {
			return err
		}
	}
	return nil
}

func downAddVersioning(ctx context.Context, tx *sql.Tx) error {
	return nil
}
