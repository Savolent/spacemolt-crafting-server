# SpaceMolt Crafting Query MCP Server

A Model Context Protocol (MCP) server that provides intelligent crafting queries for SpaceMolt AI agents to cut down on context usage and token burn.

## Features

### 6 Useful MCP Tools

1. **`craft_query`** - "What can I craft with my inventory?"
2. **`craft_path_to`** - "How do I craft this specific item?"
3. **`recipe_lookup`** - "Tell me about this recipe"
4. **`skill_craft_paths`** - "Which skills unlock new recipes?"
5. **`component_uses`** - "What can I do with this item?"
6. **`bill_of_materials`** - "What raw materials do I need?"

## Quick Start

### Installation

```bash
# Clone the repository
git clone https://github.com/rsned/spacemolt-crafting-server.git
cd spacemolt-crafting-server

# Build the server
go build -o bin/crafting-server ./cmd/crafting-server

# (Optional) Install to PATH
cp bin/crafting-server ~/go/bin/
```

### Usage

#### As an MCP Server

```bash
# Run the server (communicates via stdin/stdout)
./bin/crafting-server -db crafting.db
```

#### Database Snapshot

A pre-built database snapshot is available in the `database/` directory, containing all items, recipes, and skills already imported. You can use it directly:

```bash
# Copy the pre-built database
cp database/crafting.db ./

# Or run the server with the snapshot directly
./bin/crafting-server -db database/crafting.db
```

#### Starting From Scratch

To create and populate a fresh database from the game catalog JSON files:

```bash
# Build the server
go build -o bin/crafting-server ./cmd/crafting-server

# Import all data into a new database (the DB file is created automatically)
./bin/crafting-server -db crafting.db \
  -import-items /path/to/catalog_items.json \
  -import-recipes /path/to/catalog_recipes.json \
  -import-skills /path/to/catalog_skills.json \
  -verbose
```

The catalog JSON files use a `{"items": [...]}` envelope format, which the importer handles automatically. You can also import each file separately:

```bash
# Import items first (provides item metadata for names, categories, etc.)
./bin/crafting-server -db crafting.db -import-items catalog_items.json

# Import recipes (with inputs, outputs, and skill requirements)
./bin/crafting-server -db crafting.db -import-recipes catalog_recipes.json

# Import skills (with prerequisites and XP thresholds)
./bin/crafting-server -db crafting.db -import-skills catalog_skills.json

# (Optional) Import market data for profit calculations
./bin/crafting-server -db crafting.db -import-market market.json
```

#### Verifying the Import

After importing, you can verify the data with SQLite:

```bash
sqlite3 crafting.db "
  SELECT 'items', COUNT(*) FROM items
  UNION ALL SELECT 'recipes', COUNT(*) FROM recipes
  UNION ALL SELECT 'skills', COUNT(*) FROM skills
  UNION ALL SELECT 'recipe_inputs', COUNT(*) FROM recipe_inputs
  UNION ALL SELECT 'recipe_outputs', COUNT(*) FROM recipe_outputs
  UNION ALL SELECT 'recipe_skills', COUNT(*) FROM recipe_skills
  UNION ALL SELECT 'skill_levels', COUNT(*) FROM skill_levels
  UNION ALL SELECT 'skill_prerequisites', COUNT(*) FROM skill_prerequisites;
"
```

Expected counts: ~476 items, 394 recipes, 138 skills, plus populated junction tables.

## Claude Code Integration

To use this MCP server with Claude Code, add it to your Claude Code configuration file.

### Configuration

Edit your Claude Code config file (typically `~/.config/claude/claude_desktop_config.json` on Linux/macOS or `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "spacemolt-crafting": {
      "command": "/path/to/spacemolt-crafting-server/bin/crafting-server",
      "args": [
        "-db",
        "/path/to/spacemolt-crafting-server/database/crafting.db"
      ]
    }
  }
}
```

**Important:** Update the paths to match your actual installation directory.

### Restart Claude Code

After updating the configuration, restart Claude Code to load the MCP server. The server will then be available to assist with crafting queries.

### Available Tools

Once configured, you can use these tools in Claude Code:

- **`craft_query`** - Find what you can craft with your current inventory and skills
- **`craft_path_to`** - Get the crafting path for a specific item
- **`recipe_lookup`** - Look up details about a specific recipe
- **`skill_craft_paths`** - Discover which skills unlock new crafting recipes
- **`component_uses`** - Find all uses for a specific item
- **`bill_of_materials`** - Calculate total raw materials needed for a recipe

## Database

The server uses SQLite for fast, efficient recipe and skill queries:

- **Items:** 476 item definitions from the game catalog
- **Recipes:** 394 recipes from SpaceMolt
- **Skills:** 138 skill definitions
- **Database Size:** ~500KB
- **Query Performance:** 1-5ms typical

### Schema

- `items` - Item metadata (name, category, rarity, value)
- `recipes` - Recipe metadata
- `recipe_inputs` - Required input items (inverted index)
- `recipe_outputs` - Recipe output items (supports multiple outputs)
- `recipe_skills` - Skill requirements per recipe
- `skills` - Skill definitions
- `skill_prerequisites` - Skill dependencies
- `skill_levels` - XP thresholds per level
- `market_prices` - Historical price data
- `market_price_summary` - Aggregated price summaries

## Example Queries

### What Can I Craft?

```json
{
  "method": "tools/call",
  "params": {
    "name": "craft_query",
    "arguments": {
      "components": [
        {"id": "ore_copper", "quantity": 50}
      ],
      "skills": {
        "crafting_basic": 1
      },
      "limit": 10
    }
  }
}
```

### Bill of Materials

```json
{
  "method": "tools/call",
  "params": {
    "name": "bill_of_materials",
    "arguments": {
      "recipe_id": "craft_scanner_1",
      "quantity": 1
    }
  }
}
```

## Architecture

```
cmd/crafting-server/    # Main entry point
pkg/crafting/           # Public domain types
internal/
  ├── db/              # Database layer (SQLite)
  ├── engine/          # Query business logic
  ├── mcp/             # MCP protocol
  └── sync/            # Data import from catalog JSON
```

## Dependencies

- **Go:** 1.24 or later
- **External:** `modernc.org/sqlite` (pure Go SQLite driver)
- **Standard Library:** context, database/sql, encoding/json, log/slog

## Development

### Build

```bash
go build ./cmd/crafting-server
```

### Test

```bash
go test ./...
```

### Lint

```bash
golangci-lint run
```

## Configuration

Command-line options:

```
-db string
    Path to SQLite database (default "data/crafting/crafting.db")
-import-items string
    Import items from JSON file
-import-recipes string
    Import recipes from JSON file
-import-skills string
    Import skills from JSON file
-import-market string
    Import market data from JSON file
-migrate
    Migrate database from v1 to v2 schema
-verbose
    Enable verbose logging
```

## Data Format

The importer accepts both flat JSON arrays and catalog envelope format (`{"items": [...], "total": N}`).

### Item JSON (Catalog Format)

```json
{
  "items": [
    {
      "id": "ore_copper",
      "name": "Copper Ore",
      "description": "Common metallic ore.",
      "category": "ore",
      "rarity": "common",
      "size": 1,
      "base_value": 10,
      "stackable": true,
      "tradeable": true
    }
  ]
}
```

### Recipe JSON (Catalog Format)

```json
{
  "items": [
    {
      "id": "craft_engine_core",
      "name": "Assemble Engine Core",
      "description": "Build propulsion system cores.",
      "category": "Components",
      "crafting_time": 10,
      "base_quality": 40,
      "skill_quality_mod": 6,
      "inputs": [
        {"item_id": "refined_alloy", "quantity": 3},
        {"item_id": "ore_cobalt", "quantity": 4}
      ],
      "outputs": [
        {"item_id": "comp_engine_core", "quantity": 1, "quality_mod": true}
      ],
      "required_skills": {
        "crafting_advanced": 2
      }
    }
  ]
}
```

### Skill JSON (Catalog Format)

```json
{
  "items": [
    {
      "id": "armor_advanced",
      "name": "Advanced Armor",
      "description": "Expert armor plating.",
      "category": "Combat",
      "max_level": 10,
      "training_source": "Take hull damage in combat.",
      "required_skills": {"armor": 5},
      "xp_per_level": [500, 1500, 3000, 5000, 8000, 12000, 17000, 23000, 30000, 40000],
      "bonus_per_level": {"armorEffectiveness": 2, "hullHP": 3}
    }
  ]
}
```

## Performance

- **Query Speed:** 1-5ms for typical queries
- **Database Size:** ~500KB (476 items, 394 recipes, 138 skills)
- **Binary Size:** ~10MB
- **Memory Usage:** ~5MB typical

## License

MIT License - see [LICENSE](LICENSE) file

## Related Projects

- [SpaceMolt](https://www.spacemolt.com) - The game
- [spacemolt](https://github.com/rsned/spacemolt) - My main spacemolt working space

## Author

@rsned

## Acknowledgments

Extracted from the [spacemolt](https://github.com/rsned/spacemolt) project for better modularity and independent maintenance.
