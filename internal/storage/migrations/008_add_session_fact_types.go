package migrations

import (
	"context"
	"database/sql"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upAddSessionFactTypes, downAddSessionFactTypes)
}

func upAddSessionFactTypes(_ context.Context, _ *sql.Tx) error {
	// No schema changes needed — fact_type is TEXT, new values
	// (session_event, session_summary) work without migration.
	// This migration documents the new fact types for auditability.
	return nil
}

func downAddSessionFactTypes(_ context.Context, _ *sql.Tx) error {
	return nil
}
