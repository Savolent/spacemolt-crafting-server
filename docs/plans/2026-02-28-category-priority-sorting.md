# Category Priority Ordering Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add category priority tier sorting to all recipe queries so high-value categories (Shipbuilding, Legendary) appear before lower-priority ones (Weapons, Refining, etc.)

**Architecture:** Add a `category_priorities` database table to store tier mappings (1-6), load these into memory at engine initialization, and modify all recipe sorting functions to use category tier as the primary sort key before applying existing optimization strategies.

**Tech Stack:** Go 1.24, SQLite, database/sql, existing codebase patterns

---

## Task 1: Add Database Schema for Category Priorities

**Files:**
- Modify: `internal/crafting/db/schema.sql`

**Step 1: Add the category_priorities table to schema**

Add this table definition to the end of `internal/crafting/db/schema.sql`:

```sql
-- ============================================
-- CATEGORY PRIORITY DATA
-- ============================================

CREATE TABLE IF NOT EXISTS category_priorities (
    category TEXT PRIMARY KEY,
    priority_tier INTEGER NOT NULL CHECK (priority_tier BETWEEN 1 AND 6),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_category_priorities_tier ON category_priorities(priority_tier);
```

**Step 2: Verify the schema file is valid**

Run: `sqlite3 < /dev/null` (just to verify sqlite3 is available)
Expected: No output (sqlite3 available)

**Step 3: Commit the schema change**

```bash
git add internal/crafting/db/schema.sql
git commit -m "feat: add category_priorities table schema

Add table for storing recipe category priority tiers (1-6).
Enables priority-based sorting of recipe query results.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 2: Create Category Priority Store (Data Access Layer)

**Files:**
- Create: `internal/crafting/db/categories.go`

**Step 1: Write failing tests for CategoryPriorityStore**

Create `internal/crafting/db/categories_test.go`:

```go
package db

import (
	"context"
	"testing"
)

func TestGetPriorityTier_KnownCategory(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()

	store := NewCategoryPriorityStore(db)

	// Initialize defaults
	err := store.InitializeDefaultPriorities(ctx)
	if err != nil {
		t.Fatalf("InitializeDefaultPriorities failed: %v", err)
	}

	// Test known high-priority category
	tier, err := store.GetPriorityTier(ctx, "Shipbuilding")
	if err != nil {
		t.Fatalf("GetPriorityTier failed: %v", err)
	}
	if tier != 1 {
		t.Errorf("Expected tier 1 for Shipbuilding, got %d", tier)
	}
}

func TestGetPriorityTier_UnknownCategory(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()

	store := NewCategoryPriorityStore(db)

	// Initialize defaults
	err := store.InitializeDefaultPriorities(ctx)
	if err != nil {
		t.Fatalf("InitializeDefaultPriorities failed: %v", err)
	}

	// Test unknown category returns default tier 6
	tier, err := store.GetPriorityTier(ctx, "UnknownCategory")
	if err != nil {
		t.Fatalf("GetPriorityTier failed: %v", err)
	}
	if tier != 6 {
		t.Errorf("Expected tier 6 for unknown category, got %d", tier)
	}
}

func TestGetAllCategories(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()

	store := NewCategoryPriorityStore(db)

	// Initialize defaults
	err := store.InitializeDefaultPriorities(ctx)
	if err != nil {
		t.Fatalf("InitializeDefaultPriorities failed: %v", err)
	}

	// Get all categories
	categories, err := store.GetAllCategories(ctx)
	if err != nil {
		t.Fatalf("GetAllCategories failed: %v", err)
	}

	// Verify expected categories exist
	expectedCategories := []string{
		"Shipbuilding", "Legendary", "Utility", "Mining",
		"Components", "Weapons", "Refining",
	}

	for _, cat := range expectedCategories {
		tier, ok := categories[cat]
		if !ok {
			t.Errorf("Category %s not found in map", cat)
		}
		if tier < 1 || tier > 6 {
			t.Errorf("Invalid tier %d for category %s", tier, cat)
		}
	}
}

func TestInitializeDefaultPriorities(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	defer db.Close()

	store := NewCategoryPriorityStore(db)

	// First call should insert defaults
	err := store.InitializeDefaultPriorities(ctx)
	if err != nil {
		t.Fatalf("First InitializeDefaultPriorities failed: %v", err)
	}

	// Second call should be idempotent (no error on duplicate keys)
	err = store.InitializeDefaultPriorities(ctx)
	if err != nil {
		t.Fatalf("Second InitializeDefaultPriorities failed: %v", err)
	}

	// Verify a specific category was inserted
	tier, err := store.GetPriorityTier(ctx, "Legendary")
	if err != nil {
		t.Fatalf("GetPriorityTier failed: %v", err)
	}
	if tier != 1 {
		t.Errorf("Expected tier 1 for Legendary, got %d", tier)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v ./internal/crafting/db/... -run TestGetPriorityTier`
Expected: FAIL with "undefined: NewCategoryPriorityStore"

**Step 3: Implement CategoryPriorityStore**

Create `internal/crafting/db/categories.go`:

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
)

// CategoryPriorityStore handles category priority data access.
type CategoryPriorityStore struct {
	db *DB
}

// NewCategoryPriorityStore creates a new CategoryPriorityStore.
func NewCategoryPriorityStore(db *DB) *CategoryPriorityStore {
	return &CategoryPriorityStore{db: db}
}

// GetPriorityTier returns the priority tier for a category (1-6).
// Returns 6 (lowest) for unlisted categories.
func (s *CategoryPriorityStore) GetPriorityTier(ctx context.Context, category string) (int, error) {
	var tier sql.NullInt64

	err := s.db.QueryRowContext(ctx, `
		SELECT priority_tier FROM category_priorities WHERE category = ?
	`, category).Scan(&tier)

	if err == sql.ErrNoRows {
		// Unlisted category gets default tier 6
		return 6, nil
	}
	if err != nil {
		return 6, fmt.Errorf("querying category tier: %w", err)
	}

	if !tier.Valid {
		return 6, nil
	}

	return int(tier.Int64), nil
}

// GetAllCategories returns all categories with their priority tiers.
func (s *CategoryPriorityStore) GetAllCategories(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT category, priority_tier FROM category_priorities
	`)
	if err != nil {
		return nil, fmt.Errorf("querying all categories: %w", err)
	}
	defer func() { _ = rows.Close() }()

	categories := make(map[string]int)
	for rows.Next() {
		var category string
		var tier int
		if err := rows.Scan(&category, &tier); err != nil {
			return nil, fmt.Errorf("scanning category: %w", err)
		}
		categories[category] = tier
	}

	return categories, rows.Err()
}

// InitializeDefaultPriorities populates the table with default priority tiers.
// Uses INSERT OR IGNORE to be idempotent.
func (s *CategoryPriorityStore) InitializeDefaultPriorities(ctx context.Context) error {
	// Default priority tiers as specified in design
	defaults := map[string]int{
		"Shipbuilding":       1,
		"Legendary":          1,
		"Utility":            2,
		"Mining":             2,
		"Gas Processing":     2,
		"Ice Refining":       2,
		"Equipment":          2,
		"Components":         3,
		"Weapons":            4,
		"Drones":             4,
		"Electronic Warfare": 4,
		"Defense":            4,
		"Stealth":            4,
		"Refining":           5,
	}

	return s.db.InTransaction(ctx, func(tx *sql.Tx) error {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT OR IGNORE INTO category_priorities (category, priority_tier)
			VALUES (?, ?)
		`)
		if err != nil {
			return fmt.Errorf("preparing statement: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for category, tier := range defaults {
			_, err := stmt.ExecContext(ctx, category, tier)
			if err != nil {
				return fmt.Errorf("inserting category %s: %w", category, err)
			}
		}

		return nil
	})
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -v ./internal/crafting/db/... -run "TestGetPriorityTier|TestGetAllCategories|TestInitializeDefaultPriorities"`
Expected: PASS for all tests

**Step 5: Commit the implementation**

```bash
git add internal/crafting/db/categories.go internal/crafting/db/categories_test.go
git commit -m "feat: add CategoryPriorityStore for tier-based recipe sorting

Implement data access layer for category priority management.
- GetPriorityTier: Returns tier 1-6, defaults to 6 for unknown categories
- GetAllCategories: Returns full category->tier map
- InitializeDefaultPriorities: Idempotent default data insertion

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 3: Integrate CategoryPriorityStore into DB Initialization

**Files:**
- Modify: `internal/crafting/db/db.go`

**Step 1: Read current db.go to understand structure**

Run: `head -50 internal/crafting/db/db.go`
Expected: See DB struct and initialization pattern

**Step 2: Add CategoryPriorityStore to DB struct**

In `internal/crafting/db/db.go`, add the CategoryPriorityStore field to the DB struct:

```go
// DB provides access to all database stores.
type DB struct {
    *sql.DB
    recipes   *RecipeStore
    skills    *SkillStore
    market    *MarketStore
    catPri    *CategoryPriorityStore  // ADD THIS LINE
}
```

**Step 3: Initialize CategoryPriorityStore in Open function**

In the `Open` function of `internal/crafting/db/db.go`, after the market store initialization:

```go
// Add after: db.market = NewMarketStore(db)

db.catPri = NewCategoryPriorityStore(db)
```

**Step 4: Initialize default priorities after schema creation**

In the `Open` function, after `InitSchema(ctx, db)`:

```go
// Add after InitSchema call:
// Initialize category priorities
if err := db.catPri.InitializeDefaultPriorities(ctx); err != nil {
    _ = db.Close()
    return nil, fmt.Errorf("initializing category priorities: %w", err)
}
```

**Step 5: Add getter method for CategoryPriorityStore**

Add this method to the DB struct:

```go
// CategoryPriorities returns the category priority store.
func (db *DB) CategoryPriorities() *CategoryPriorityStore {
    return db.catPri
}
```

**Step 6: Run existing tests to ensure no breakage**

Run: `go test -v ./internal/crafting/db/...`
Expected: All existing tests still pass

**Step 7: Commit the integration**

```bash
git add internal/crafting/db/db.go
git commit -m "feat: integrate CategoryPriorityStore into DB initialization

- Add catPri field to DB struct
- Initialize default priorities on database open
- Add CategoryPriorities() getter method

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 4: Add Priority Cache to Engine

**Files:**
- Modify: `internal/crafting/engine/engine.go`

**Step 1: Add categoryPriorities field to Engine struct**

In `internal/crafting/engine/engine.go`, add the cache field:

```go
type Engine struct {
    recipes   *db.RecipeStore
    skills    *db.SkillStore
    market    *db.MarketStore
    catPri    *db.CategoryPriorityStore

    // Cached priority map for fast lookups (NEW)
    categoryPriorities map[string]int
}
```

**Step 2: Load priorities in New function**

Modify the `New` function to load and cache priorities:

```go
func New(db *db.DB) *Engine {
    // Load category priorities into memory for fast access
    priorities, err := db.CategoryPriorities().GetAllCategories(context.Background())
    if err != nil {
        // Log warning but continue - will use tier 6 (default) for all
        log.Printf("WARNING: Failed to load category priorities: %v", err)
        priorities = make(map[string]int)
    }

    return &Engine{
        recipes:            db.Recipes(),
        skills:             db.Skills(),
        market:             db.Market(),
        catPri:             db.CategoryPriorities(),
        categoryPriorities: priorities,
    }
}
```

Note: You may need to add `import "log"` at the top if not already present.

**Step 3: Add helper function to get tier with default**

Add this helper method to the Engine struct:

```go
// getCategoryTier returns the priority tier for a category.
// Returns 6 (lowest) for unlisted categories.
func (e *Engine) getCategoryTier(category string) int {
    if tier, ok := e.categoryPriorities[category]; ok {
        return tier
    }
    return 6  // Default to lowest priority
}
```

**Step 4: Run engine tests to verify no breakage**

Run: `go test -v ./internal/crafting/engine/...`
Expected: All existing tests pass

**Step 5: Commit the engine changes**

```bash
git add internal/crafting/engine/engine.go
git commit -m "feat: add category priority cache to Engine

- Load category priorities into memory on initialization
- Add getCategoryTier helper with default tier 6
- Cache provides fast lookups during sorting

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 5: Update CraftQuery Sorting to Use Category Tiers

**Files:**
- Modify: `internal/crafting/engine/craft_query.go`

**Step 1: Modify sortCraftable to use tier-aware sorting**

Replace the `sortCraftable` function in `craft_query.go`:

```go
// sortCraftable sorts craftable matches based on optimization strategy.
// Primary sort: Category tier (1-6), Secondary sort: Strategy.
func (e *Engine) sortCraftable(matches []crafting.CraftableMatch, strategy crafting.OptimizationStrategy) {
    sort.Slice(matches, func(i, j int) bool {
        // Primary: sort by category tier
        tierI := e.getCategoryTier(matches[i].Recipe.Category)
        tierJ := e.getCategoryTier(matches[j].Recipe.Category)
        if tierI != tierJ {
            return tierI < tierJ
        }

        // Secondary: apply strategy within same tier
        switch strategy {
        case crafting.StrategyMaximizeProfit:
            pi := profitPerUnit(matches[i].ProfitAnalysis)
            pj := profitPerUnit(matches[j].ProfitAnalysis)
            return pi > pj

        case crafting.StrategyMaximizeVolume:
            return matches[i].CanCraftQuantity > matches[j].CanCraftQuantity

        case crafting.StrategyUseInventoryFirst:
            return matches[i].CanCraftQuantity > matches[j].CanCraftQuantity

        case crafting.StrategyMinimizeAcquisition:
            return matches[i].CanCraftQuantity > matches[j].CanCraftQuantity

        case crafting.StrategyOptimizeCraftPath:
            return len(matches[i].Recipe.Inputs) < len(matches[j].Recipe.Inputs)

        default:
            return matches[i].CanCraftQuantity > matches[j].CanCraftQuantity
        }
    })
}
```

**Step 2: Modify sortPartial to use tier-aware sorting**

Replace the `sortPartial` function in `craft_query.go`:

```go
// sortPartial sorts partial matches based on optimization strategy.
// Primary sort: Category tier (1-6), Secondary sort: Strategy.
func (e *Engine) sortPartial(matches []crafting.PartialComponentMatch, strategy crafting.OptimizationStrategy) {
    sort.Slice(matches, func(i, j int) bool {
        // Primary: sort by category tier
        tierI := e.getCategoryTier(matches[i].Recipe.Category)
        tierJ := e.getCategoryTier(matches[j].Recipe.Category)
        if tierI != tierJ {
            return tierI < tierJ
        }

        // Secondary: apply strategy within same tier
        switch strategy {
        case crafting.StrategyMaximizeProfit:
            pi := profitPerUnit(matches[i].ProfitAnalysis)
            pj := profitPerUnit(matches[j].ProfitAnalysis)
            return pi > pj

        case crafting.StrategyMaximizeVolume:
            return matches[i].MatchRatio > matches[j].MatchRatio

        case crafting.StrategyUseInventoryFirst:
            return matches[i].MatchRatio > matches[j].MatchRatio

        case crafting.StrategyMinimizeAcquisition:
            return len(matches[i].InputsMissing) < len(matches[j].InputsMissing)

        case crafting.StrategyOptimizeCraftPath:
            return len(matches[i].Recipe.Inputs) < len(matches[j].Recipe.Inputs)

        default:
            return matches[i].MatchRatio > matches[j].MatchRatio
        }
    })
}
```

**Step 3: Update CraftQuery method to use engine receivers**

In the `CraftQuery` method, change the sort calls:

Find:
```go
sortCraftable(craftable, req.Strategy)
sortPartial(partialComponents, req.Strategy)
sortPartial(blockedBySkills, req.Strategy)
```

Replace with:
```go
e.sortCraftable(craftable, req.Strategy)
e.sortPartial(partialComponents, req.Strategy)
e.sortPartial(blockedBySkills, req.Strategy)
```

**Step 4: Run existing craft_query tests**

Run: `go test -v ./internal/crafting/engine/... -run TestCraftQuery`
Expected: Tests pass (behavior preserved, just sorting order changes)

**Step 5: Commit the sorting changes**

```bash
git add internal/crafting/engine/craft_query.go
git commit -m "feat: apply category tier sorting to craft_query results

- sortCraftable and sortPartial now use Engine receiver
- Primary sort by category tier (1-6)
- Secondary sort by optimization strategy within tier
- Higher priority categories (Shipbuilding, Legendary) appear first

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 6: Update ComponentUses Sorting

**Files:**
- Modify: `internal/crafting/engine/component_uses.go`

**Step 1: Read current component_uses.go to find sorting logic**

Run: `grep -n "sort" internal/crafting/engine/component_uses.go`
Expected: See sort.Sort or sort.Slice calls

**Step 2: Add tier-aware sorting to ComponentUses results**

Add sorting after results are collected (before returning):

```go
// Sort results by category tier first, then by optimization strategy
sort.Slice(result.UsedIn, func(i, j int) bool {
    tierI := e.getCategoryTier(result.UsedIn[i].Recipe.Category)
    tierJ := e.getCategoryTier(result.UsedIn[j].Recipe.Category)
    if tierI != tierJ {
        return tierI < tierJ
    }

    // Within tier, apply strategy
    switch req.Strategy {
    case crafting.StrategyMaximizeProfit:
        pi := profitPerUnit(result.UsedIn[i].ProfitAnalysis)
        pj := profitPerUnit(result.UsedIn[j].ProfitAnalysis)
        return pi > pj
    default:
        // Default to recipe name for consistency
        return result.UsedIn[i].Recipe.Name < result.UsedIn[j].Recipe.Name
    }
})
```

**Step 3: Run component_uses tests**

Run: `go test -v ./internal/crafting/engine/... -run TestComponentUses`
Expected: Tests pass

**Step 4: Commit the component_uses changes**

```bash
git add internal/crafting/engine/component_uses.go
git commit -m "feat: apply category tier sorting to component_uses results

- Sort recipe list by category tier first
- Secondary sort by optimization strategy
- Ensures consistent priority ordering across all tools

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 7: Update SkillPaths Sorting

**Files:**
- Modify: `internal/crafting/engine/skill_paths.go`

**Step 1: Find where recipes are sorted in skill_paths.go**

Run: `grep -n "RecipesUnlocked\|sort" internal/crafting/engine/skill_paths.go`
Expected: See where recipe lists are built

**Step 2: Add tier-aware sorting for each skill's unlocked recipes**

For each `SkillUnlockPath` in results, sort its `RecipesUnlocked` slice:

```go
// Sort each skill's unlocked recipes by category tier
for i := range result.SkillPaths {
    sort.Slice(result.SkillPaths[i].RecipesUnlocked, func(j, k int) bool {
        // Need to look up recipe categories from the recipe store
        recipeJ, errJ := e.recipes.GetRecipe(ctx, result.SkillPaths[i].RecipesUnlocked[j])
        recipeK, errK := e.recipes.GetRecipe(ctx, result.SkillPaths[i].RecipesUnlocked[k])
        if errJ != nil || errK != nil {
            // If we can't get recipe info, compare by recipe ID
            return result.SkillPaths[i].RecipesUnlocked[j] < result.SkillPaths[i].RecipesUnlocked[k]
        }

        tierJ := e.getCategoryTier(recipeJ.Category)
        tierK := e.getCategoryTier(recipeK.Category)
        if tierJ != tierK {
            return tierJ < tierK
        }

        // Within tier, sort by recipe name
        return recipeJ.Name < recipeK.Name
    })
}
```

**Step 8: Run skill_paths tests**

Run: `go test -v ./internal/crafting/engine/... -run TestSkillCraftPaths`
Expected: Tests pass

**Step 9: Commit the skill_paths changes**

```bash
git add internal/crafting/engine/skill_paths.go
git commit -m "feat: apply category tier sorting to skill_craft_paths results

- Sort each skill's unlocked recipes by category tier
- Ensures high-priority recipe categories appear first in skill unlock lists
- Secondary sort by recipe name within tier

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 8: Update RecipeLookup Search Results Sorting

**Files:**
- Modify: `internal/crafting/engine/recipe_lookup.go`

**Step 1: Find search results sorting logic**

Run: `grep -n "SearchResults\|sort" internal/crafting/engine/recipe_lookup.go`
Expected: See where search results are returned

**Step 2: Add tier-aware sorting to search results**

Add sorting before returning search results:

```go
// Sort search results by category tier
sort.Slice(response.SearchResults, func(i, j int) bool {
    tierI := e.getCategoryTier(response.SearchResults[i].Category)
    tierJ := e.getCategoryTier(response.SearchResults[j].Category)
    if tierI != tierJ {
        return tierI < tierJ
    }

    // Within tier, sort by name
    return response.SearchResults[i].Name < response.SearchResults[j].Name
})
```

**Step 3: Run recipe_lookup tests**

Run: `go test -v ./internal/crafting/engine/... -run TestRecipeLookup`
Expected: Tests pass

**Step 4: Commit the recipe_lookup changes**

```bash
git add internal/crafting/engine/recipe_lookup.go
git commit -m "feat: apply category tier sorting to recipe_lookup search results

- Sort search results by category tier first
- Secondary sort by recipe name within tier
- Consistent priority ordering across all search queries

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 9: Add Integration Tests for Category Sorting

**Files:**
- Create: `internal/crafting/engine/category_sorting_test.go`

**Step 1: Write integration test for craft_query sorting**

Create comprehensive integration tests:

```go
package engine

import (
	"context"
	"testing"

	"github.com/rsned/spacemolt-crafting-server/pkg/crafting"
)

func TestCraftQuery_CategoryPrioritySorting(t *testing.T) {
	ctx := context.Background()
	engine := setupTestEngine(t)
	defer engine.Close()

	// Create test recipes with different categories
	recipes := []crafting.Recipe{
		{ID: "recipe_refining", Name: "Refining Recipe", Category: "Refining"},
		{ID: "recipe_ship", Name: "Ship Recipe", Category: "Shipbuilding"},
		{ID: "recipe_weapon", Name: "Weapon Recipe", Category: "Weapons"},
		{ID: "recipe_legendary", Name: "Legendary Recipe", Category: "Legendary"},
	}

	// Insert test recipes
	if err := engine.recipes.BulkInsertRecipes(ctx, recipes); err != nil {
		t.Fatalf("Failed to insert recipes: %v", err)
	}

	// Query for craftable items
	req := crafting.CraftQueryRequest{
		Components: []crafting.Component{},
		Skills:     map[string]int{},
		Limit:      10,
	}

	resp, err := engine.CraftQuery(ctx, req)
	if err != nil {
		t.Fatalf("CraftQuery failed: %v", err)
	}

	// Verify ordering: Shipbuilding/Legendary (tier 1) before Weapons (tier 4) before Refining (tier 5)
	var categories []string
	for _, match := range resp.Craftable {
		categories = append(categories, match.Recipe.Category)
	}

	// Tier 1 should appear before tier 4
	shipIdx := indexOf(categories, "Shipbuilding")
	weaponIdx := indexOf(categories, "Weapons")
	if shipIdx != -1 && weaponIdx != -1 && shipIdx > weaponIdx {
		t.Errorf("Shipbuilding (tier 1) should appear before Weapons (tier 4)")
	}

	// Tier 1 should appear before tier 5
	refiningIdx := indexOf(categories, "Refining")
	if shipIdx != -1 && refiningIdx != -1 && shipIdx > refiningIdx {
		t.Errorf("Shipbuilding (tier 1) should appear before Refining (tier 5)")
	}
}

func TestUnknownCategory_LastInResults(t *testing.T) {
	ctx := context.Background()
	engine := setupTestEngine(t)
	defer engine.Close()

	// Create test recipes with known and unknown categories
	recipes := []crafting.Recipe{
		{ID: "recipe_ship", Name: "Ship Recipe", Category: "Shipbuilding"},
		{ID: "recipe_unknown", Name: "Unknown Recipe", Category: "CompletelyUnknownCategory"},
	}

	if err := engine.recipes.BulkInsertRecipes(ctx, recipes); err != nil {
		t.Fatalf("Failed to insert recipes: %v", err)
	}

	req := crafting.CraftQueryRequest{
		Components: []crafting.Component{},
		Skills:     map[string]int{},
		Limit:      10,
	}

	resp, err := engine.CraftQuery(ctx, req)
	if err != nil {
		t.Fatalf("CraftQuery failed: %v", err)
	}

	// Unknown category should appear last
	var categories []string
	for _, match := range resp.Craftable {
		categories = append(categories, match.Recipe.Category)
	}

	unknownIdx := indexOf(categories, "CompletelyUnknownCategory")
	shipIdx := indexOf(categories, "Shipbuilding")

	if unknownIdx != -1 && shipIdx != -1 && unknownIdx < shipIdx {
		t.Errorf("Unknown category (tier 6) should appear after Shipbuilding (tier 1)")
	}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}
```

**Step 2: Run integration tests**

Run: `go test -v ./internal/crafting/engine/... -run "TestCraftQuery_CategoryPrioritySorting|TestUnknownCategory_LastInResults"`
Expected: Tests pass, verifying tier-based ordering

**Step 3: Commit integration tests**

```bash
git add internal/crafting/engine/category_sorting_test.go
git commit -m "test: add integration tests for category priority sorting

- TestCraftQuery_CategoryPrioritySorting: Verifies tier 1 before tier 4/5
- TestUnknownCategory_LastInResults: Verifies unknown categories appear last
- Validates end-to-end sorting behavior

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 10: Run Full Test Suite and Linting

**Step 1: Run all tests**

Run: `go test -v ./...`
Expected: All tests pass (existing + new category sorting tests)

**Step 2: Run golangci-lint**

Run: `golangci-lint run`
Expected: No new findings introduced

**Step 3: Fix any lint issues**

If golangci-lint reports issues, fix them:

Run: `golangci-lint run --fix`
Or manually fix the reported issues

**Step 4: Commit any lint fixes**

```bash
git add -A
git commit -m "fix: resolve golangci-lint findings

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 11: Update Documentation

**Files:**
- Modify: `README.md` (optional, if user-facing behavior needs documentation)

**Step 1: Document the category priority feature**

Add a section to README.md explaining the new sorting behavior:

```markdown
## Category Priority Sorting

Recipe query results are now sorted by category priority tier (1-6) before applying optimization strategies. This ensures high-value categories like Shipbuilding and Legendary recipes appear first in results.

### Priority Tiers

| Tier | Categories |
|------|------------|
| 1 (highest) | Shipbuilding, Legendary |
| 2 | Utility, Mining, Gas Processing, Ice Refining, Equipment |
| 3 | Components |
| 4 | Weapons, Drones, Electronic Warfare, Defense, Stealth |
| 5 | Refining |
| 6 (lowest) | All other categories |

### Customizing Priorities

To adjust category priorities, update the `category_priorities` table directly:

```bash
sqlite3 crafting.db "INSERT OR REPLACE INTO category_priorities (category, priority_tier) VALUES ('MyCategory', 2);"
```

Priority changes take effect immediately on the next query.
```

**Step 2: Commit documentation updates**

```bash
git add README.md
git commit -m "docs: document category priority sorting feature

Add explanation of tier-based recipe sorting and customization options.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 12: Verify End-to-End with Real Database

**Step 1: Test with actual database**

Run: `./bin/crafting-server -db database/crafting.db -version`
Expected: Server starts successfully, category_priorities table exists

**Step 2: Verify category_priorities table exists**

Run: `sqlite3 database/crafting.db "SELECT category, priority_tier FROM category_priorities ORDER BY priority_tier, category;"`
Expected: See all default categories with their tiers

**Step 3: Test craft_query tool manually**

Use the MCP tool or test-tools to verify sorting works with real data:

Run: `./cmd/test-tools/main.go -db database/crafting.db -v`
Expected: Results show Shipbuilding/Legendary recipes first

**Step 4: Final commit with verification notes**

```bash
git add -A
git commit -m "feat: complete category priority ordering implementation

All recipe query tools now sort by category tier (1-6) as primary key,
with existing optimization strategies as secondary sort within tiers.

Verified:
- category_priorities table created and populated
- All 4 recipe-list tools use tier-aware sorting
- Unknown categories default to tier 6 (lowest)
- Integration tests pass with real database

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Verification Checklist

Before considering this feature complete, verify:

- [ ] All unit tests pass (`go test -v ./...`)
- [ ] No golangci-lint findings (`golangci-lint run`)
- [ ] Integration tests verify tier-based ordering
- [ ] Unknown categories appear last in results
- [ ] All 4 recipe-list tools use priority sorting:
  - [ ] craft_query
  - [ ] component_uses
  - [ ] skill_craft_paths
  - [ ] recipe_lookup
- [ ] Category priorities load correctly from database
- [ ] Manual testing with real database confirms behavior
- [ ] Documentation updated (if applicable)

---

## Rollback Plan (If Issues Arise)

To revert category priority sorting:

1. Remove `categoryPriorities map[string]int` field from Engine
2. Remove `getCategoryTier` method from Engine
3. Revert sort functions to non-receiver versions
4. Remove category tier logic from sort functions
5. Keep `category_priorities` table (harmless if unused)

The database schema change is backward compatible and can be left in place.
