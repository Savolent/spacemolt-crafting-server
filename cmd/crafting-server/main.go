// SpaceMolt Crafting Query MCP Server
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/rsned/spacemolt-crafting-server/internal/crafting/db"
	"github.com/rsned/spacemolt-crafting-server/internal/crafting/engine"
	"github.com/rsned/spacemolt-crafting-server/internal/crafting/mcp"
	"github.com/rsned/spacemolt-crafting-server/internal/crafting/sync"
)

func main() {
	// Parse flags
	dbPath := flag.String("db", "data/crafting/crafting.db", "Path to SQLite database")
	importItems := flag.String("import-items", "", "Import items from JSON file")
	importRecipes := flag.String("import-recipes", "", "Import recipes from JSON file")
	importSkills := flag.String("import-skills", "", "Import skills from JSON file")
	importMarket := flag.String("import-market", "", "Import market data from JSON file")
	gameVersion := flag.String("game-version", "", "Game server version (e.g., 'v0.142.7')")
	showVersion := flag.Bool("version", false, "Show database version information and exit")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("shutting down...")
		cancel()
	}()

	// Open database
	database, err := db.OpenAndInit(ctx, *dbPath)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = database.Close() }()

	// Handle version query
	if *showVersion {
		version, err := database.GetVersion(ctx)
		if err != nil {
			logger.Error("failed to get version", "error", err)
			os.Exit(1)
		}
		if version == nil {
			fmt.Println("No version information available in database")
			os.Exit(0)
		}
		fmt.Printf("Game Version: %s\n", version.GameVersion)
		fmt.Printf("Imported At: %s\n", version.ImportedAt.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("Updated At:  %s\n", version.UpdatedAt.Format("2006-01-02 15:04:05 MST"))
		os.Exit(0)
	}

	// Handle import commands
	if *importItems != "" || *importRecipes != "" || *importSkills != "" || *importMarket != "" {
		syncer := sync.NewSyncer(database)

		// Track if any imports happened
		imported := false

		if *importItems != "" {
			logger.Info("importing items", "file", *importItems)
			if err := syncer.ImportItemsFromFile(ctx, *importItems); err != nil {
				logger.Error("failed to import items", "error", err)
				os.Exit(1)
			}
			logger.Info("items imported successfully")
			imported = true
		}

		if *importRecipes != "" {
			logger.Info("importing recipes", "file", *importRecipes)
			if err := syncer.ImportRecipesFromFile(ctx, *importRecipes); err != nil {
				logger.Error("failed to import recipes", "error", err)
				os.Exit(1)
			}
			logger.Info("recipes imported successfully")
			imported = true
		}

		if *importSkills != "" {
			logger.Info("importing skills", "file", *importSkills)
			if err := syncer.ImportSkillsFromFile(ctx, *importSkills); err != nil {
				logger.Error("failed to import skills", "error", err)
				os.Exit(1)
			}
			logger.Info("skills imported successfully")
			imported = true
		}

		if *importMarket != "" {
			logger.Info("importing market data", "file", *importMarket)
			if err := syncer.ImportMarketDataFromFile(ctx, *importMarket); err != nil {
				logger.Error("failed to import market data", "error", err)
				os.Exit(1)
			}
			logger.Info("market data imported successfully")
			imported = true
		}

		// Update version info if game-version was provided
		if imported && *gameVersion != "" {
			logger.Info("setting version", "game_version", *gameVersion)
			if err := database.SetVersion(ctx, *gameVersion); err != nil {
				logger.Warn("failed to set version", "error", err)
			} else {
				logger.Info("version set successfully")
			}
		} else if imported {
			// Just update the timestamp if no version specified
			logger.Info("updating version timestamp")
			if err := database.UpdateVersionTimestamp(ctx); err != nil {
				logger.Warn("failed to update version timestamp", "error", err)
			}
		}

		// If only doing imports, exit
		if flag.NArg() == 0 {
			return
		}
	}

	// Create engine and server
	eng := engine.New(database)
	server := mcp.NewServer(eng, logger)

	// Run MCP server
	logger.Info("starting MCP server", "db", *dbPath)
	if err := server.Run(ctx); err != nil && ctx.Err() == nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "server stopped")
}
