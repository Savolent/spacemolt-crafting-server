# Market Data and Pricing Enhancement Design

**Date:** 2026-02-28
**Status:** Draft
**Author:** @rsned

## Overview

This design enhances the SpaceMolt Crafting Server's market data and profit calculation capabilities. The system will support sophisticated pricing statistics, handle flooded markets with outlier prices, integrate MSRP (base_value) for reference, and provide both HTTP and bulk-import mechanisms for market data submission.

## Goals

1. **Accurate market pricing** using statistical methods adapted to different market conditions
2. **Real-time market data submission** via HTTP API for live players and scrapers
3. **Profit calculations** based on actual market data, not just MSRP
4. **Station/empire scoping** to support regional pricing differences
5. **Backward compatibility** with existing MCP tools and database

## Current State

The system already has:
- `market_prices` table storing timestamped aggregate prices
- `market_price_summary` table with 7-day averages (min, max, trend)
- `ProfitAnalysis` type with basic profit metrics
- Import via `-import-market` CLI flag

Limitations:
- Simple average/min/max statistics (sensitive to outliers)
- No order book depth tracking
- No station or empire filtering
- MSRP not exposed to users
- No real-time submission mechanism

## Design

### 1. Enhanced Pricing Architecture

#### Database Schema

**New Table: `market_order_book`**
Stores individual buy/sell orders for advanced statistics:

```sql
CREATE TABLE market_order_book (
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
```

**New Table: `market_price_stats`**
Pre-computed statistics using hybrid pricing approach:

```sql
CREATE TABLE market_price_stats (
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
```

**Modify Existing: `market_prices`**
Add empire scoping:
```sql
ALTER TABLE market_prices ADD COLUMN empire_id TEXT;
```

#### Pricing Strategy Selection

The system automatically selects the best pricing method based on data characteristics:

```
IF sample_count >= 50 AND total_volume >= 10000:
    → volume_weighted (high-volume markets like ores)
ELSE IF sample_count >= 10:
    → second_price (trim top/bottom 10%, average middle 80%)
ELSE IF sample_count >= 3:
    → median (sparse but real data)
ELSE:
    → msrp_only (fallback, flagged as estimated)
```

**Volume-Weighted Average**
Used for flooded markets (ores with high undercutting activity):
- Weight each price by volume available
- Large orders move price more than small orders
- Reflects actual market clearing price

**Second-Price Auction**
Used for normal liquidity markets:
- Sort orders by price
- Exclude top 10% and bottom 10% (outliers)
- Average the middle 80%
- Reduces impact of extreme prices

**Median**
Used for sparse data:
- Statistically robust to outliers
- Works with small sample sizes

**MSRP Fallback**
Used when no market data exists:
- Retrieved from `items.base_value`
- Flagged with `confidence_score: 0.0`
- Shown for reference only, never used in profit calculations when real data exists

#### MSRP Integration

MSRP is **always returned** in pricing responses as a reference field:
- `msrp`: The `base_value` from items table
- `representative_price`: The calculated market price (used for profit calcs)
- Users can compare MSRP vs market value at a glance

### 2. Market Data Submission API

#### HTTP Endpoint

**POST /api/v1/market/submit**

Submit current market order book data:

```json
{
  "station_id": "station_earth_orbit",
  "empire_id": "empire_terran",
  "source": "player_submission | auto_scraper",
  "orders": [
    {
      "item_id": "ore_iron",
      "order_type": "sell",
      "price_per_unit": 1,
      "volume_available": 10000,
      "player_stall": "IronMiner42"
    }
  ],
  "submitted_at": "2026-02-28T10:30:00Z",
  "submitter_id": "optional_agent_or_user_id"
}
```

**Response:**
```json
{
  "batch_id": "batch_20260228_103000_abc123",
  "orders_received": 150,
  "orders_accepted": 142,
  "orders_rejected": 8,
  "errors": ["invalid item_id: fake_item"],
  "estimated_processing_time_ms": 500
}
```

#### Processing Flow

1. **Immediate Validation** (synchronous)
   - Schema validation
   - Item ID lookup in database
   - Price range sanity checks
   - Return errors to client immediately

2. **Batch Insertion** (async)
   - Insert into `market_order_book` with generated `batch_id`
   - Group orders by submission batch

3. **Statistics Recalculation** (background)
   - Trigger recalculation for affected items/stations
   - Update `market_price_stats`

4. **Pruning**
   - Remove orders older than 7 days (configurable)

#### Deduplication Strategy

**Within a batch:**
- Deduplicate by: `(item_id, station_id, order_type, price_per_unit, volume, player_stall_name)`
- Prevents same submitter from accidentally sending identical data twice
- Does NOT combine different orders at the same price (legitimate market depth)

**Cross-batch management:**
- Keep all orders for 7 days regardless of price
- Evict oldest orders after 7 days
- If total orders per item/station exceed threshold (500), evict oldest

#### CLI Bulk Import

Enhanced existing flag:
```bash
crafting-server -db crafting.db -import-market scraped_market.json
```

Accepts both old (aggregate) and new (order book) JSON formats. Processes synchronously with progress reporting.

### 3. MCP Tool Integration

#### Updated Tool Parameters

All profit-aware tools get optional scoping:

```go
type CraftingRequest struct {
    Components    []ComponentQuantity
    Skills        map[string]SkillProgress
    Limit         int

    // NEW: Market scoping
    StationID     string `json:"station_id,omitempty"`
    EmpireID      string `json:"empire_id,omitempty"`
    RequireMarket bool   `json:"require_market"`
}
```

#### Profit Calculation Flow

```go
func (e *Engine) calculateProfit(ctx, recipe, stationID, empireID) (*ProfitAnalysis, error) {
    // 1. Get market price stats for output
    stats, err := e.market.GetPriceStats(ctx, outputItemID, stationID, empireID, "sell")

    if err != nil || stats == nil {
        return &ProfitAnalysis{
            OutputSellPrice: 0,
            MarketStatus: "no_market_data",
            MSRP: getMSRP(outputItemID),
        }, nil
    }

    // 2. Sum input costs
    var totalInputCost int
    for _, input := range recipe.Inputs {
        inputStats := e.market.GetPriceStats(ctx, input.ItemID, stationID, empireID, "buy")
        totalInputCost += inputStats.RepresentativePrice * input.Quantity
    }

    // 3. Calculate profit
    outputPrice := stats.RepresentativePrice * outputQuantity
    profit := outputPrice - totalInputCost

    return &ProfitAnalysis{
        OutputSellPrice:    outputPrice,
        InputCost:          totalInputCost,
        ProfitPerUnit:      profit,
        ProfitMarginPct:    float64(profit) / float64(outputPrice) * 100,
        MarketStatus:       stats.ConfidenceScore,
        PricingMethod:      stats.StatMethod,
        MSRP:               getMSRP(outputItemID),
        TotalVolume24h:     stats.TotalVolume,
        PriceTrend:         stats.PriceTrend,
    }, nil
}
```

#### Response Format

```json
{
  "recipe": {...},
  "profit_analysis": {
    "output_sell_price": 5000,
    "input_cost": 3200,
    "profit_per_unit": 1800,
    "profit_margin_pct": 36.0,

    // NEW fields
    "msrp": 4500,
    "market_status": "high_confidence",
    "pricing_method": "volume_weighted",
    "sample_count": 247,
    "price_trend": "falling",
    "total_volume_24h": 125000
  }
}
```

### 4. HTTP Server Implementation

#### Package Structure

```
internal/
  ├── api/               # NEW
  │   ├── server.go      # HTTP server setup
  │   ├── handlers.go    # Request handlers
  │   └── middleware.go  # Auth, logging, rate limiting
  ├── crafting/engine/
  └── db/
```

#### Server Configuration

```bash
crafting-server -db crafting.db -http :8080
```

Flags:
- `-http`: Enable HTTP server (default: off, MCP mode)
- `-http-addr`: Bind address (default: `:8080`)
- `-http-auth`: Require Bearer token
- `-rate-limit`: Requests per minute (default: 60)
- `-cors`: Enable CORS

#### Endpoints

```
POST   /api/v1/market/submit           # Submit order book
GET    /api/v1/market/price/:item_id   # Query price stats
GET    /api/v1/health                  # Health check
POST   /api/v1/admin/recalc            # Trigger recalc (auth)
GET    /api/v1/admin/stats             # DB stats (auth)
```

#### Rate Limiting

- Per-IP: 60 requests/minute
- Per-station: 1 submission/minute per station_id
- Burst allowance for bulk imports

#### Authentication (Optional)

```bash
curl -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json" \
     -d @market.json \
     http://localhost:8080/api/v1/market/submit
```

### 5. Background Statistics & Data Management

#### Stats Calculator

```go
type StatsCalculator struct {
    db *DB
}

func (sc *StatsCalculator) RecalculateItemStats(ctx, itemID, stationID) error {
    orders := sc.fetchRecentOrders(ctx, itemID, stationID, 7)

    for _, orderType := range []string{"buy", "sell"} {
        typeOrders := filterByType(orders, orderType)
        if len(typeOrders) == 0 {
            continue
        }

        method := sc.choosePricingMethod(typeOrders)
        stats := sc.calculateStats(typeOrders, method)
        sc.saveStats(ctx, stats)
    }

    return nil
}
```

#### Recalculation Triggers

1. **After import**: Trigger for affected items/stations
2. **Scheduled**: Hourly recalc for items with recent orders
3. **On-demand**: Admin API endpoint
4. **Lazy**: On query if stats are stale (>1 hour old)

#### Data Retention

```go
// Run daily
func (sc *StatsCalculator) PruneOldData(ctx) error {
    // Remove orders older than 7 days
    DELETE FROM market_order_book
    WHERE recorded_at < datetime('now', '-7 days')

    // Remove stale stats
    DELETE FROM market_price_stats
    WHERE last_updated < datetime('now', '-30 days')
}
```

### 6. Migration Strategy

#### Migration Script

`internal/db/migrations/005_add_enhanced_market_tables.sql`

1. Create `market_order_book` table
2. Create `market_price_stats` table
3. Add `empire_id` to `market_prices`
4. Migrate existing `market_prices` to `market_order_book` (as "synthetic" orders)
5. Backfill MSRP for all items into `market_price_stats`

#### Migration Command

```bash
# Explicit migration
crafting-server -db crafting.db migrate

# Or automatic on startup
crafting-server -db crafting.db -auto-migrate
```

#### Backward Compatibility

- All new parameters are optional
- Existing tools work without modification
- Migration preserves all existing data
- Rollback script provided

## Implementation Phases

**Phase 1: Core Infrastructure (Week 1)**
- Database migration script
- New tables and indexes
- Migration tool in CLI
- Basic StatsCalculator

**Phase 2: HTTP Server (Week 1-2)**
- `internal/api` package
- Market submit endpoint
- Validation and rate limiting
- CLI flag: `-http :8080`

**Phase 3: Enhanced Profit Calculations (Week 2)**
- Update ProfitAnalysis type
- Modify calculateProfit() method
- Add station/empire parameters
- Update all profit-aware tools

**Phase 4: Background Processing (Week 2-3)**
- Auto-recalc triggers
- Hourly scheduled jobs
- Daily pruning
- Admin endpoints

**Phase 5: Bulk Import Enhancement (Week 3)**
- Update -import-market flag
- Support order book format
- Batch processing
- Progress reporting

**Phase 6: Testing & Polish (Week 3-4)**
- Unit tests for pricing methods
- Integration tests
- Load testing
- Documentation

**Phase 7: Deployment (Week 4)**
- Migration testing
- Gradual rollout
- Monitoring setup

## Testing Strategy

### Unit Tests
- Volume-weighted average calculation
- Second-price auction (trim logic)
- Median calculation
- MSRP fallback
- Outlier detection

### Integration Tests
- Full pipeline: submit → validate → store → recalc → query
- Multiple orders at same price (no deduplication)
- Station/empire filtering
- Market data absence handling

### Load Tests
- HTTP endpoint: 1000 concurrent submissions
- Large bulk imports: 10,000+ orders
- Stats recalculation performance

## Performance Considerations

- **Order book**: 7-day retention keeps DB size bounded
- **Stats pre-calculation**: Queries use pre-computed values, not raw orders
- **Indexes**: Composite indexes on (item_id, station_id, order_type, recorded_at)
- **Background jobs**: Non-blocking async processing
- **Rate limiting**: Prevents abuse and spikes

## Security Considerations

- **Input validation**: All item IDs validated against database
- **Price limits**: Reject clearly invalid prices (negative, unreasonably high)
- **Rate limiting**: Per-IP and per-station limits
- **Optional auth**: Bearer token for protected endpoints
- **SQL injection**: Parameterized queries throughout

## Future Enhancements

- **Price prediction models**: ML for trend forecasting
- **Arbitrage detection**: Find profit opportunities across stations
- **Historical price charts**: Track price movements over time
- **Market alerts**: Notify users of significant price changes
- **Player reputation**: Track reliable vs unreliable data submitters

## References

- Second-price auction theory
- Interquartile range (IQR) for outlier detection
- Volume-weighted average price (VWAP)
