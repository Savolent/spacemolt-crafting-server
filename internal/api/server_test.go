package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rsned/spacemolt-crafting-server/internal/crafting/db"
)

func TestServerStartup(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("opening database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Initialize schema
	if err := db.InitSchema(ctx, database.DB); err != nil {
		t.Fatalf("initializing schema: %v", err)
	}

	// Create server with random port to avoid conflicts
	server := NewServer(database, Config{
		Addr:            "127.0.0.1:0", // Let OS assign port
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    5 * time.Second,
		ShutdownTimeout: 5 * time.Second,
	})

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Start()
	}()

	// Wait a moment for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Check if server started successfully
	select {
	case err := <-serverErr:
		t.Fatalf("server failed to start: %v", err)
	default:
		// Server is running, continue
	}

	// Shutdown server
	if err := server.Shutdown(ctx); err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	// Wait for server to actually shut down
	select {
	case err := <-serverErr:
		if err != http.ErrServerClosed {
			t.Errorf("unexpected server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}
}
