package db

import (
	"context"
	"database/sql"
	"time"
)

// IllegalRecipe represents a recipe that is illegal to craft privately
type IllegalRecipe struct {
	RecipeID      string
	BanReason     string
	LegalLocation string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IllegalRecipesStore handles illegal recipe operations
type IllegalRecipesStore struct {
	db *DB
}

// NewIllegalRecipesStore creates a new illegal recipes store
func NewIllegalRecipesStore(db *DB) *IllegalRecipesStore {
	return &IllegalRecipesStore{db: db}
}

// IsIllegal checks if a recipe is illegal to craft privately
func (s *IllegalRecipesStore) IsIllegal(ctx context.Context, recipeID string) (bool, *IllegalRecipe, error) {
	query := `
		SELECT recipe_id, ban_reason, legal_location, created_at, updated_at
		FROM illegal_recipes
		WHERE recipe_id = ?
	`

	var ir IllegalRecipe
	err := s.db.QueryRowContext(ctx, query, recipeID).Scan(
		&ir.RecipeID,
		&ir.BanReason,
		&ir.LegalLocation,
		&ir.CreatedAt,
		&ir.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	return true, &ir, nil
}

// MarkIllegal marks a recipe as illegal to craft privately
func (s *IllegalRecipesStore) MarkIllegal(ctx context.Context, recipeID, banReason, legalLocation string) error {
	query := `
		INSERT INTO illegal_recipes (recipe_id, ban_reason, legal_location)
		VALUES (?, ?, ?)
		ON CONFLICT(recipe_id) DO UPDATE SET
			ban_reason = excluded.ban_reason,
			legal_location = excluded.legal_location,
			updated_at = CURRENT_TIMESTAMP
	`
	_, err := s.db.ExecContext(ctx, query, recipeID, banReason, legalLocation)
	return err
}

// MarkLegal removes illegal status from a recipe
func (s *IllegalRecipesStore) MarkLegal(ctx context.Context, recipeID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM illegal_recipes WHERE recipe_id = ?", recipeID)
	return err
}
