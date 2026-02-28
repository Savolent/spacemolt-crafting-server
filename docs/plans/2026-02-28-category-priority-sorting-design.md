# Category Priority Ordering for Recipe Queries

**Date:** 2026-02-28
**Status:** Approved
**Author:** Design Review

## Overview

Add a category priority system where recipes are first sorted by their category tier (1-6), then by the existing optimization strategy within each tier. Priority tiers will be stored in a database table for runtime configuration without recompilation.

## Requirements

### Functional Requirements

1. **Primary Sort by Category Tier**: Recipes must be sorted first by category priority tier (1=highest, 6=lowest)
2. **Secondary Sort by Strategy**: Within each tier, apply existing optimization strategies (profit, volume, etc.)
3. **Database Storage**: Priority tiers stored in database table for runtime updates
4. **Backward Compatibility**: Unlisted categories default to tier 6 (lowest priority)
5. **All Recipe Lists**: Apply sorting to all tools returning recipe lists

### Priority Tier Specification

| Tier | Categories |
|------|------------|
| 1 (highest) | Shipbuilding, Legendary |
| 2 | Utility, Mining, Gas Processing, Ice Refining, Equipment |
| 3 | Components |
| 4 | Weapons, Drones, Electronic Warfare, Defense, Stealth |
| 5 | Refining |
| 6 (lowest) | All other categories (default) |

## Architecture

### Database Schema

```sql
CREATE TABLE IF NOT EXISTS category_priorities (
    category TEXT PRIMARY KEY,
    priority_tier INTEGER NOT NULL CHECK (priority_tier BETWEEN 1 AND 6),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_category_priorities_tier
    ON category_priorities(priority_tier);
```

**Initial Data:**
```sql
INSERT OR IGNORE INTO category_priorities (category, priority_tier) VALUES
    ('Shipbuilding', 1),
    ('Legendary', 1),
    ('Utility', 2),
    ('Mining', 2),
    ('Gas Processing', 2),
    ('Ice Refining', 2),
    ('Equipment', 2),
    ('Components', 3),
    ('Weapons', 4),
    ('Drones', 4),
    ('Electronic Warfare', 4),
    ('Defense', 4),
    ('Stealth', 4),
    ('Refining', 5);
```

### Component Structure

```
internal/crafting/
├── db/
│   ├── categories.go          # NEW: CategoryPriorityStore
│   ├── db.go                  # MODIFIED: Add CategoryPriorityStore
│   └── schema.sql             # MODIFIED: Add category_priorities table
└── engine/
    ├── engine.go              # MODIFIED: Add priority cache
    ├── craft_query.go         # MODIFIED: Use tier-aware sorting
    ├── component_uses.go      # MODIFIED: Add sorting
    ├── skill_paths.go         # MODIFIED: Add sorting
    └── recipe_lookup.go       # MODIFIED: Add sorting
```

### Data Access Layer

**New file: `internal/crafting/db/categories.go`**

```go
package db

type CategoryPriorityStore struct {
    db *DB
}

// GetPriorityTier returns the priority tier for a category (1-6).
// Returns 6 (lowest) for unlisted categories.
func (s *CategoryPriorityStore) GetPriorityTier(
    ctx context.Context, category string) (int, error)

// GetAllCategories returns all categories with their priority tiers.
func (s *CategoryPriorityStore) GetAllCategories(
    ctx context.Context) (map[string]int, error)

// InitializeDefaultPriorities populates the table with default
// priority tiers if empty.
func (s *CategoryPriorityStore) InitializeDefaultPriorities(
    ctx context.Context) error
```

### Query Engine Changes

**Modified: `internal/crafting/engine/engine.go`**

```go
type Engine struct {
    recipes *RecipeStore
    skills  *SkillStore
    market  *MarketStore
    catPri  *CategoryPriorityStore  // NEW

    // Cached priority map for fast lookups
    categoryPriorities map[string]int  // NEW
}

func New(...) *Engine {
    // Initialize priority cache on engine creation
    priorities, _ := catPri.GetAllCategories(ctx)
    return &Engine{
        categoryPriorities: priorities,
        // ...
    }
}
```

**Sorting Logic Pattern (applied to all sort functions):**

```go
func sortCraftable(
    matches []CraftableMatch,
    strategy OptimizationStrategy,
    catPriorities map[string]int) {  // NEW parameter
    sort.Slice(matches, func(i, j int) bool {
        // Primary: category tier
        tierI := getTier(catPriorities, matches[i].Recipe.Category)
        tierJ := getTier(catPriorities, matches[j].Recipe.Category)
        if tierI != tierJ {
            return tierI < tierJ  // Lower number = higher priority
        }

        // Secondary: existing strategy
        switch strategy {
        case StrategyMaximizeProfit:
            // ... existing logic
        // ...
        }
    })
}

func getTier(priorities map[string]int, category string) int {
    if tier, ok := priorities[category]; ok {
        return tier
    }
    return 6  // Default to lowest priority
}
```

### Affected Tools

1. **craft_query**: Sort craftable, partial, and blocked lists
2. **component_uses**: Sort used_in recipe list
3. **skill_craft_paths**: Sort recipes_unlocked lists
4. **recipe_lookup**: Sort search_results

## Data Flow

```
1. Engine Initialization
   ↓
2. Load category priorities into memory map
   ↓
3. Query executes (craft_query, component_uses, etc.)
   ↓
4. Recipes matched and filtered
   ↓
5. Sorting applied:
   - Primary: Category tier (1-6)
   - Secondary: Optimization strategy
   ↓
6. Results returned to client
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Unlisted category | Return tier 6, no error |
| Priority table missing | Auto-create with defaults |
| DB error on load | Log warning, use tier 6 for all |
| Invalid tier value | Database constraint enforcement |

## Testing Strategy

### Unit Tests

- `TestGetPriorityTier`: Returns correct tier for known/unknown categories
- `TestInitializeDefaultPriorities`: Populates table correctly
- `TestSortCraftableWithTiers`: Orders by tier first, then strategy
- `TestGetTier`: Handles missing categories correctly

### Integration Tests

- `TestCraftQueryRespectsTiers`: Results sorted by category tier
- `TestUnlistedCategoryLast`: Unknown categories appear at end
- `TestWithinTierStrategy`: Secondary sort applies within tier
- `TestAllToolsUseSorting`: All 4 tools apply tier sorting

### Migration Tests

- Table creation on fresh database
- Default data insertion
- Priority updates take effect

## Migration Path

### Phase 1: Schema Addition
1. Add `category_priorities` table to `schema.sql`
2. Existing databases unaffected (new table only)

### Phase 2: Code Changes
1. Add `categories.go` with `CategoryPriorityStore`
2. Update `db.go` to initialize store
3. Update engine files with new sorting
4. Update engine initialization

### Phase 3: Deployment
1. Deploy binary with new schema
2. On first run, table auto-created
3. Default priorities inserted
4. Existing queries immediately benefit from sorting

### Rollback Plan
- Remove priority parameters from sort functions
- Keep `category_priorities` table (harmless if unused)
- Revert to original sorting logic

## Success Criteria

1. All recipe-list tools return results sorted by category tier
2. Within each tier, existing optimization strategies apply
3. Unlisted categories appear last in results
4. Priority tiers can be updated via SQL without recompilation
5. No performance regression (< 1ms overhead per query)
6. All existing tests pass plus new category-priority tests

## Future Enhancements (Out of Scope)

- Admin API for updating priorities without SQL
- Per-user priority customization
- Priority inheritance (parent/child categories)
- Priority weights instead of discrete tiers
