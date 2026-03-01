package db

import (
	"context"
	"database/sql"
	"fmt"
)

// Station represents a station with its empire affiliation.
type Station struct {
	ID     string
	Name   string
	PoiID  string
	Empire string
}

// UpsertStation inserts or updates a station record.
func (db *DB) UpsertStation(ctx context.Context, s Station) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO stations (id, name, poi_id, empire)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			poi_id = excluded.poi_id,
			empire = excluded.empire
	`, s.ID, s.Name, s.PoiID, s.Empire)
	if err != nil {
		return fmt.Errorf("upserting station: %w", err)
	}
	return nil
}

// ResolveStation looks up a station by trying station_id, poi_id, and name
// in that order. Returns nil if no match is found.
func (db *DB) ResolveStation(ctx context.Context, identifier string) (*Station, error) {
	var s Station
	err := db.QueryRowContext(ctx, `
		SELECT id, name, COALESCE(poi_id, ''), empire FROM stations
		WHERE id = ? OR poi_id = ? OR name = ?
		LIMIT 1
	`, identifier, identifier, identifier).Scan(&s.ID, &s.Name, &s.PoiID, &s.Empire)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolving station: %w", err)
	}
	return &s, nil
}

// GetStation retrieves a station by ID.
func (db *DB) GetStation(ctx context.Context, id string) (*Station, error) {
	var s Station
	err := db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(poi_id, ''), empire FROM stations WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &s.PoiID, &s.Empire)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying station by id: %w", err)
	}
	return &s, nil
}

// GetStationByName retrieves a station by name.
func (db *DB) GetStationByName(ctx context.Context, name string) (*Station, error) {
	var s Station
	err := db.QueryRowContext(ctx,
		`SELECT id, name, COALESCE(poi_id, ''), empire FROM stations WHERE name = ?`, name,
	).Scan(&s.ID, &s.Name, &s.PoiID, &s.Empire)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying station by name: %w", err)
	}
	return &s, nil
}

// ListStations returns all stations.
func (db *DB) ListStations(ctx context.Context) ([]Station, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name, COALESCE(poi_id, ''), empire FROM stations ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing stations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stations []Station
	for rows.Next() {
		var s Station
		if err := rows.Scan(&s.ID, &s.Name, &s.PoiID, &s.Empire); err != nil {
			return nil, fmt.Errorf("scanning station: %w", err)
		}
		stations = append(stations, s)
	}
	return stations, rows.Err()
}

// ListStationsByEmpire returns all stations belonging to an empire.
func (db *DB) ListStationsByEmpire(ctx context.Context, empire string) ([]Station, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, COALESCE(poi_id, ''), empire FROM stations WHERE empire = ? ORDER BY name`, empire,
	)
	if err != nil {
		return nil, fmt.Errorf("listing stations by empire: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var stations []Station
	for rows.Next() {
		var s Station
		if err := rows.Scan(&s.ID, &s.Name, &s.PoiID, &s.Empire); err != nil {
			return nil, fmt.Errorf("scanning station: %w", err)
		}
		stations = append(stations, s)
	}
	return stations, rows.Err()
}
