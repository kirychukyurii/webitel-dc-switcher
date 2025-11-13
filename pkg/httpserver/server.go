package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Server represents an HTTP server with graceful shutdown
type Server struct {
	server *http.Server
	logger *slog.Logger
}

// New creates a new HTTP server
func New(addr string, handler http.Handler, readTimeout, writeTimeout time.Duration, logger *slog.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		logger: logger,
	}
}

// Run starts the HTTP server and handles graceful shutdown
func (s *Server) Run() error {
	// Channel to listen for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Channel to notify when server has shut down
	serverErrors := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		s.logger.Info("starting http server",
			slog.String("addr", s.server.Addr),
		)
		serverErrors <- s.server.ListenAndServe()
	}()

	// Block until we receive a signal or an error
	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case sig := <-quit:
		s.logger.Info("received shutdown signal",
			slog.String("signal", sig.String()),
		)

		// Create a context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := s.server.Shutdown(ctx); err != nil {
			s.logger.Error("graceful shutdown failed, forcing shutdown",
				slog.String("error", err.Error()),
			)
			if err := s.server.Close(); err != nil {
				return err
			}
		}

		s.logger.Info("server stopped gracefully")
	}

	return nil
}
