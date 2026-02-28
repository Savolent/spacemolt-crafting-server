package db

import (
	"context"
	"database/sql"
	"embed"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migration represents a database schema migration.
type Migration struct {
	ID      string
	UpSQL   string
	DownSQL string
}

// MigrationTracker tracks which database migrations have been applied.
type MigrationTracker struct {
	db *DB
}

// NewMigrationTracker creates a new migration tracker.
func NewMigrationTracker(db *DB) *MigrationTracker {
	return &MigrationTracker{db: db}
}

// IsApplied checks if a migration has been applied.
func (m *MigrationTracker) IsApplied(ctx context.Context, migrationID string) (bool, error) {
	var count int
	err := m.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations WHERE migration_id = ?`,
		migrationID,
	).Scan(&count)

	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// RecordApplied records that a migration has been applied.
func (m *MigrationTracker) RecordApplied(ctx context.Context, migrationID string) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO schema_migrations (migration_id, applied_at) VALUES (?, datetime('now'))`,
		migrationID,
	)
	return err
}

// GetCurrentVersion returns the most recently applied migration ID.
func (m *MigrationTracker) GetCurrentVersion(ctx context.Context) (string, error) {
	var version string
	err := m.db.QueryRowContext(ctx,
		`SELECT migration_id FROM schema_migrations ORDER BY applied_at DESC LIMIT 1`,
	).Scan(&version)

	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return version, nil
}

// Migrator applies database migrations.
type Migrator struct {
	db      *DB
	tracker *MigrationTracker
}

// NewMigrator creates a new migrator.
func NewMigrator(db *DB) *Migrator {
	return &Migrator{
		db:      db,
		tracker: NewMigrationTracker(db),
	}
}

// Apply applies a migration if it hasn't been applied yet.
func (m *Migrator) Apply(ctx context.Context, migration *Migration) error {
	// Check if already applied
	applied, err := m.tracker.IsApplied(ctx, migration.ID)
	if err != nil {
		return err
	}
	if applied {
		// Already applied, skip (idempotent)
		return nil
	}

	// Apply migration in a transaction
	return m.db.InTransaction(ctx, func(tx *sql.Tx) error {
		// Execute the Up migration
		if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
			return err
		}

		// Record as applied
		_, err = tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (migration_id, applied_at) VALUES (?, datetime('now'))`,
			migration.ID,
		)
		return err
	})
}

// GetMigration005 returns the market data enhancement migration.
func GetMigration005() (*Migration, error) {
	data, err := migrationFS.ReadFile("migrations/005_add_enhanced_market_tables.sql")
	if err != nil {
		return nil, err
	}

	return &Migration{
		ID:    "005_add_enhanced_market_tables",
		UpSQL: string(data),
		DownSQL: `
			DROP TABLE IF EXISTS market_order_book;
			DROP TABLE IF EXISTS market_price_stats;
		`,
	}, nil
}

// ApplyMigration005 applies migration 005 with special handling for empire_id column.
func ApplyMigration005(ctx context.Context, db *DB) error {
	migration, err := GetMigration005()
	if err != nil {
		return err
	}

	// Check if migration already applied
	tracker := NewMigrationTracker(db)
	applied, err := tracker.IsApplied(ctx, migration.ID)
	if err != nil {
		return err
	}
	if applied {
		return nil // Already applied
	}

	return db.InTransaction(ctx, func(tx *sql.Tx) error {
		// First, add empire_id column if it doesn't exist
		// Check if column exists using PRAGMA
		var hasColumn bool
		rows, err := tx.QueryContext(ctx, `PRAGMA table_info(market_prices)`)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()

		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue any
		for rows.Next() {
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
				return err
			}
			if name == "empire_id" {
				hasColumn = true
				break
			}
		}

		if !hasColumn {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE market_prices ADD COLUMN empire_id TEXT`); err != nil {
				return err
			}
		}

		// Now execute the rest of the migration
		if _, err := tx.ExecContext(ctx, migration.UpSQL); err != nil {
			return err
		}

		// Record as applied
		_, err = tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (migration_id, applied_at) VALUES (?, datetime('now'))`,
			migration.ID,
		)
		return err
	})
}
