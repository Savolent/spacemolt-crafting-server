package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rsned/spacemolt-crafting-server/internal/crafting/db"
)

// Config holds server configuration.
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Server handles HTTP requests for market data API.
type Server struct {
	db     *db.DB
	config Config
	server *http.Server
	addr   string // Actual address server is listening on
}

// NewServer creates a new HTTP server.
func NewServer(database *db.DB, cfg Config) *Server {
	return &Server{
		db:     database,
		config: cfg,
	}
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	listener, err := net.Listen("tcp", s.config.Addr)
	if err != nil {
		return err
	}

	s.addr = listener.Addr().String()

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	return s.server.Serve(listener)
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	return s.server.Shutdown(shutdownCtx)
}

// handleHealth returns server health status.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, `{"status":"healthy"}`)
}
