package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// GracefulServer wraps an HTTP server with graceful shutdown capability
type GracefulServer struct {
	server *http.Server
}

// NewGracefulServer creates a new server with graceful shutdown support
func NewGracefulServer(addr string, handler http.Handler) *GracefulServer {
	return &GracefulServer{
		server: &http.Server{
			Addr:    addr,
			Handler: handler,
		},
	}
}

// Start starts the server and sets up signal handling for graceful shutdown
func (s *GracefulServer) Start() error {
	// Channel to listen for errors coming from the listener.
	serverErrors := make(chan error, 1)
	
	// Start the server
	go func() {
		slog.Info("Starting server", slog.String("address", s.server.Addr))
		serverErrors <- s.server.ListenAndServe()
	}()

	// Channel to listen for an interrupt or terminate signal from the OS.
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking main and waiting for shutdown or server errors.
	select {
	case err := <-serverErrors:
		return err

	case <-shutdown:
		slog.Info("Shutdown signal received, shutting down gracefully...")
		
		// Give outstanding requests a deadline for completion.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		// Asking listener to shut down and shed load.
		if err := s.server.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			slog.Error("Graceful shutdown failed", slog.String("error", err.Error()))
			
			// Force close now
			if err := s.server.Close(); err != nil {
				slog.Error("Error during forced server close", slog.String("error", err.Error()))
			}
			
			return err
		}

		slog.Info("Graceful shutdown complete")
		return nil
	}
}