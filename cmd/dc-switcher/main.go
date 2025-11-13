package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/api"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/cache"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/healthcheck"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/logger"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/repository"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/service"
	"github.com/kirychukyurii/webitel-dc-switcher/pkg/httpserver"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	// Initialize logger
	log := logger.New()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("failed to load configuration",
			"error", err.Error(),
		)
		os.Exit(1)
	}

	log.Info("configuration loaded",
		"clusters", len(cfg.Clusters),
	)

	// Create cache
	appCache := cache.New(cfg.Cache.TTL)

	// Create Nomad repository
	repo, err := repository.NewNomadRepository(cfg, log)
	if err != nil {
		log.Error("failed to create nomad repository",
			"error", err.Error(),
		)
		os.Exit(1)
	}

	log.Info("nomad clients initialized",
		"clusters", len(cfg.Clusters),
	)

	// Create service
	svc := service.NewDatacenterService(repo, appCache, cfg.Cache.TTL, log)

	// Ensure only one datacenter is active at startup
	log.Info("performing startup datacenter state synchronization")
	if err := svc.EnsureSingleActiveDatacenter(context.Background()); err != nil {
		log.Error("failed to sync datacenter state at startup",
			"error", err.Error(),
		)
		// Don't exit - continue with startup but log the error
	}

	// Create and start health checker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	healthChecker := healthcheck.NewChecker(&cfg.HealthCheck, svc, log)
	svc.SetHealthChecker(healthChecker) // Link service with health checker for region change notifications
	healthChecker.Start(ctx)

	// Create HTTP handler
	handler := api.NewHandler(svc, cfg.Server.BasePath, log)

	// Setup signal handling for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Create HTTP server
	srv := httpserver.New(
		cfg.Server.Addr,
		handler.Router(),
		cfg.Server.ReadTimeout,
		cfg.Server.WriteTimeout,
		log,
	)

	log.Info("starting dc-switcher service")

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		// Use internal server start method (without signal handling)
		log.Info("starting http server",
			"addr", cfg.Server.Addr,
		)
		if err := srv.Run(); err != nil {
			serverErrors <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		log.Error("server error",
			"error", err.Error(),
		)
	case sig := <-quit:
		log.Info("received shutdown signal",
			"signal", sig.String(),
		)
	}

	// Graceful shutdown
	log.Info("shutting down health checker")
	cancel() // Cancel context for health checker
	healthChecker.Stop()

	log.Info("shutdown complete")
}
