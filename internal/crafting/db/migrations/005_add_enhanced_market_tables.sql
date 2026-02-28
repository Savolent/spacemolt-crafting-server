-- Migration 005: Add Enhanced Market Data Tables
-- This migration adds support for order book tracking and advanced pricing statistics

-- Add empire_id column to existing market_prices table
-- Note: SQLite doesn't support ALTER TABLE with IF NOT EXISTS, so we check if column exists first
-- This will be handled in Go code

-- Create market_order_book table (stores individual orders)
CREATE TABLE IF NOT EXISTS market_order_book (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    batch_id        TEXT NOT NULL,
    item_id         TEXT NOT NULL,
    station_id      TEXT NOT NULL,
    empire_id       TEXT,
    order_type      TEXT NOT NULL CHECK (order_type IN ('buy', 'sell')),
    price_per_unit  INTEGER NOT NULL,
    volume_available INTEGER NOT NULL,
    player_stall_name TEXT,
    recorded_at     TEXT NOT NULL,
    submitter_id    TEXT,
    created_at      TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_order_book_lookup ON market_order_book(item_id, station_id, order_type, recorded_at);
CREATE INDEX IF NOT EXISTS idx_order_book_batch ON market_order_book(batch_id);
CREATE INDEX IF NOT EXISTS idx_order_book_stale ON market_order_book(recorded_at);

-- Create market_price_stats table (computed statistics)
CREATE TABLE IF NOT EXISTS market_price_stats (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    item_id             TEXT NOT NULL,
    station_id          TEXT NOT NULL,
    empire_id           TEXT,
    order_type          TEXT NOT NULL CHECK (order_type IN ('buy', 'sell')),
    stat_method         TEXT NOT NULL,
    representative_price INTEGER NOT NULL,
    sample_count        INTEGER NOT NULL,
    total_volume        INTEGER NOT NULL,
    min_price           INTEGER NOT NULL,
    max_price           INTEGER NOT NULL,
    stddev              REAL,
    confidence_score    REAL NOT NULL,
    price_trend         TEXT CHECK (price_trend IN ('rising', 'falling', 'stable')),
    last_updated        TEXT NOT NULL,
    UNIQUE(item_id, station_id, empire_id, order_type),
    FOREIGN KEY (item_id) REFERENCES items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_price_stats_lookup ON market_price_stats(item_id, station_id, order_type);

-- Backfill MSRP for all items into market_price_stats (as fallback)
INSERT OR IGNORE INTO market_price_stats (
    item_id, station_id, empire_id, order_type, stat_method,
    representative_price, sample_count, total_volume, min_price,
    max_price, stddev, confidence_score, last_updated
)
SELECT
    i.id as item_id,
    'global' as station_id,
    'global' as empire_id,
    'sell' as order_type,
    'msrp_only' as stat_method,
    i.base_value as representative_price,
    0 as sample_count,
    0 as total_volume,
    i.base_value as min_price,
    i.base_value as max_price,
    0.0 as stddev,
    0.0 as confidence_score,
    CURRENT_TIMESTAMP as last_updated
FROM items i
WHERE i.base_value > 0;
