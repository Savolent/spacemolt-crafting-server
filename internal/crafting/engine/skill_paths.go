package engine

import (
	"context"

	"github.com/rsned/spacemolt-crafting-server/pkg/crafting"
)

// SkillCraftPaths executes the skill_craft_paths tool logic.
// Since v0.226.0 removed recipe-level skill gates, this returns skill
// progression info without recipe unlock data.
func (e *Engine) SkillCraftPaths(ctx context.Context, req crafting.SkillCraftPathsRequest) (*crafting.SkillCraftPathsResponse, error) {
	// Apply defaults
	if req.Limit <= 0 {
		req.Limit = 10
	}

	// Get total recipe count
	totalRecipes, err := e.recipes.CountRecipes(ctx)
	if err != nil {
		return nil, err
	}

	// All recipes are unlocked since crafting gates were removed in v0.226.0
	return &crafting.SkillCraftPathsResponse{
		Summary: crafting.SkillCraftPathsSummary{
			TotalRecipes:    totalRecipes,
			RecipesUnlocked: totalRecipes,
			RecipesLocked:   0,
		},
	}, nil
}
