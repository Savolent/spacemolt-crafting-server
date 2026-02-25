package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// VersionInfo represents the database version information.
type VersionInfo struct {
	GameVersion string
	ImportedAt  time.Time
	UpdatedAt   time.Time
}

// GetVersion retrieves the version information from the database.
func (db *DB) GetVersion(ctx context.Context) (*VersionInfo, error) {
	var gameVersion, importedAt, updatedAt string
	err := db.QueryRowContext(ctx,
		`SELECT game_version, imported_at, updated_at FROM version WHERE id = 1`,
	).Scan(&gameVersion, &importedAt, &updatedAt)

	if err == sql.ErrNoRows {
		// No version info yet
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying version: %w", err)
	}

	importedTime, err := time.Parse(time.RFC3339, importedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing imported_at: %w", err)
	}

	updatedTime, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing updated_at: %w", err)
	}

	return &VersionInfo{
		GameVersion: gameVersion,
		ImportedAt:  importedTime,
		UpdatedAt:   updatedTime,
	}, nil
}

// SetVersion sets or updates the version information.
// If a version row already exists, it updates the game_version and updated_at.
// If no version row exists, it creates a new one.
func (db *DB) SetVersion(ctx context.Context, gameVersion string) error {
	// Try update first
	result, err := db.ExecContext(ctx,
		`UPDATE version SET game_version = ?, updated_at = ? WHERE id = 1`,
		gameVersion, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("updating version: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}

	// If no rows were updated, insert a new row
	if rows == 0 {
		now := time.Now().Format(time.RFC3339)
		_, err = db.ExecContext(ctx,
			`INSERT INTO version (id, game_version, imported_at, updated_at) VALUES (1, ?, ?, ?)`,
			gameVersion, now, now,
		)
		if err != nil {
			return fmt.Errorf("inserting version: %w", err)
		}
	}

	return nil
}

// UpdateVersionTimestamp updates only the updated_at timestamp to the current time.
// This should be called when data is re-imported or updated.
func (db *DB) UpdateVersionTimestamp(ctx context.Context) error {
	_, err := db.ExecContext(ctx,
		`UPDATE version SET updated_at = ? WHERE id = 1`,
		time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("updating version timestamp: %w", err)
	}
	return nil
}
