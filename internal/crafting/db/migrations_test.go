package db

import (
	"context"
	"database/sql"
	"testing"
)

func TestMigrationTracker(t *testing.T) {
	ctx := context.Background()

	// Create an in-memory database for testing
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize schema first
	if err := InitSchema(ctx, db.DB); err != nil {
		t.Fatalf("initializing schema: %v", err)
	}

	// Create migration tracker
	tracker := NewMigrationTracker(db)

	// First migration should not be applied
	applied, err := tracker.IsApplied(ctx, "001_test_migration")
	if err != nil {
		t.Fatalf("checking if migration applied: %v", err)
	}
	if applied {
		t.Error("first migration should not be applied yet")
	}

	// Record migration as applied
	if err := tracker.RecordApplied(ctx, "001_test_migration"); err != nil {
		t.Fatalf("recording migration: %v", err)
	}

	// Now migration should be applied
	applied, err = tracker.IsApplied(ctx, "001_test_migration")
	if err != nil {
		t.Fatalf("checking if migration applied: %v", err)
	}
	if !applied {
		t.Error("migration should now be applied")
	}

	// Get current version - should be the migration we just applied
	version, err := tracker.GetCurrentVersion(ctx)
	if err != nil {
		t.Fatalf("getting current version: %v", err)
	}
	if version != "001_test_migration" {
		t.Errorf("expected version 001_test_migration, got %s", version)
	}
}

func TestMigrator(t *testing.T) {
	ctx := context.Background()

	// Create an in-memory database for testing
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize schema first
	if err := InitSchema(ctx, db.DB); err != nil {
		t.Fatalf("initializing schema: %v", err)
	}

	// Create migrator
	migrator := NewMigrator(db)

	// Create a test migration
	migration := &Migration{
		ID:   "001_create_test_table",
		UpSQL: `
			CREATE TABLE IF NOT EXISTS test_table (
				id INTEGER PRIMARY KEY,
				name TEXT NOT NULL
			);
		`,
		DownSQL: `DROP TABLE IF EXISTS test_table;`,
	}

	// Run migration
	if err := migrator.Apply(ctx, migration); err != nil {
		t.Fatalf("applying migration: %v", err)
	}

	// Verify migration was recorded
	tracker := NewMigrationTracker(db)
	applied, err := tracker.IsApplied(ctx, "001_create_test_table")
	if err != nil {
		t.Fatalf("checking if migration applied: %v", err)
	}
	if !applied {
		t.Error("migration should be applied")
	}

	// Verify table exists
	var tableName string
	err = db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='table' AND name='test_table'`,
	).Scan(&tableName)
	if err != nil {
		t.Errorf("test_table should exist: %v", err)
	}

	// Running the same migration again should be idempotent (no error)
	if err := migrator.Apply(ctx, migration); err != nil {
		t.Errorf("re-applying migration should not error: %v", err)
	}
}

func TestMigration005MarketDataEnhancement(t *testing.T) {
	ctx := context.Background()

	// Create an in-memory database for testing
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Initialize schema first
	if err := InitSchema(ctx, db.DB); err != nil {
		t.Fatalf("initializing schema: %v", err)
	}

	// Insert test data to verify migration compatibility
	_, err = db.ExecContext(ctx, `
		INSERT INTO items (id, name, base_value) VALUES
			('ore_iron', 'Iron Ore', 10),
			('comp_steel', 'Steel Component', 100);

		INSERT INTO market_prices (item_id, station_id, price_type, price, volume_24h, recorded_at)
		VALUES ('ore_iron', 'station_1', 'sell', 5, 1000, datetime('now'));
	`)
	if err != nil {
		t.Fatalf("inserting test data: %v", err)
	}

	// Apply migration 005 with special handling
	if err := ApplyMigration005(ctx, db); err != nil {
		t.Fatalf("applying migration 005: %v", err)
	}

	// Verify market_order_book table exists
	var tableExists int
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='market_order_book'`,
	).Scan(&tableExists)
	if err != nil || tableExists != 1 {
		t.Error("market_order_book table should exist")
	}

	// Verify market_price_stats table exists
	err = db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='market_price_stats'`,
	).Scan(&tableExists)
	if err != nil || tableExists != 1 {
		t.Error("market_price_stats table should exist")
	}

	// Verify empire_id column added to market_prices
	var empireID sql.NullString
	err = db.QueryRowContext(ctx, `SELECT empire_id FROM market_prices LIMIT 1`).Scan(&empireID)
	if err != nil {
		t.Errorf("empire_id column should exist in market_prices: %v", err)
	}

	// Verify MSRP backfilled in market_price_stats
	var msrp int
	err = db.QueryRowContext(ctx,
		`SELECT representative_price FROM market_price_stats WHERE item_id = 'ore_iron' AND stat_method = 'msrp_only'`,
	).Scan(&msrp)
	if err != nil || msrp != 10 {
		t.Errorf("MSRP should be backfilled for ore_iron: got %d, err %v", msrp, err)
	}
}
