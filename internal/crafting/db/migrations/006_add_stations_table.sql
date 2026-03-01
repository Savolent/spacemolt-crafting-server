-- Migration 006: Add stations table for station-to-empire mapping.

CREATE TABLE IF NOT EXISTS stations (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    poi_id          TEXT,
    empire          TEXT NOT NULL
);
