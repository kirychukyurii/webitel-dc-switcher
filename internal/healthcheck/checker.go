package healthcheck

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/service"
)

// Checker performs periodic health checks on the active region
type Checker struct {
	cfg            *config.HealthCheckConfig
	dcService      service.DatacenterService
	logger         *slog.Logger
	stopCh         chan struct{}
	wg             sync.WaitGroup
	activeRegion   string         // Currently active region to monitor
	failureCounter map[string]int // region -> consecutive failure count
	mu             sync.RWMutex
}

// NewChecker creates a new health checker
func NewChecker(
	cfg *config.HealthCheckConfig,
	dcService service.DatacenterService,
	logger *slog.Logger,
) *Checker {
	return &Checker{
		cfg:            cfg,
		dcService:      dcService,
		logger:         logger,
		stopCh:         make(chan struct{}),
		failureCounter: make(map[string]int),
	}
}

// Start begins the health check loop in a background goroutine
func (c *Checker) Start(ctx context.Context) {
	if !c.cfg.Enabled {
		c.logger.Info("health check is disabled")
		return
	}

	// Determine initial active region
	activeRegion, err := c.detectActiveRegion(ctx)
	if err != nil {
		c.logger.Error("failed to detect initial active region",
			slog.String("error", err.Error()),
		)
	} else if activeRegion != "" {
		c.mu.Lock()
		c.activeRegion = activeRegion
		c.mu.Unlock()

		c.logger.Info("initial active region detected",
			slog.String("region", activeRegion),
		)
	} else {
		c.logger.Info("no active region at startup")
	}

	c.logger.Info("starting health checker",
		slog.Duration("interval", c.cfg.Interval),
		slog.Int("failed_threshold", c.cfg.FailedThreshold),
	)

	c.wg.Add(1)
	go c.run(ctx)
}

// SetActiveRegion updates the currently monitored active region
// This should be called when a region is activated via UI
func (c *Checker) SetActiveRegion(region string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldRegion := c.activeRegion
	c.activeRegion = region

	// Reset failure counter when changing regions
	c.failureCounter = make(map[string]int)

	if oldRegion != region {
		c.logger.Info("active region changed",
			slog.String("old_region", oldRegion),
			slog.String("new_region", region),
		)
	}
}

// Stop gracefully stops the health checker
func (c *Checker) Stop() {
	if !c.cfg.Enabled {
		return
	}

	c.logger.Info("stopping health checker")
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("health checker stopped")
}

// run is the main health check loop
func (c *Checker) run(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.cfg.Interval)
	defer ticker.Stop()

	// Perform initial check after a short delay
	c.logger.Info("waiting 5 seconds before first health check")
	time.Sleep(5 * time.Second)
	c.logger.Info("performing initial health check")
	c.performCheck(ctx)

	for {
		select {
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.logger.Info("health check timer triggered")
			c.performCheck(ctx)
		}
	}
}

// performCheck executes a single health check cycle
func (c *Checker) performCheck(ctx context.Context) {
	// Sync active region with actual state before checking
	realActiveRegion, err := c.detectActiveRegion(ctx)
	if err != nil {
		c.logger.Warn("failed to detect actual active region",
			slog.String("error", err.Error()),
		)
		// Continue with current activeRegion if detection fails
	} else if realActiveRegion != "" {
		c.mu.Lock()
		currentActiveRegion := c.activeRegion
		if currentActiveRegion != realActiveRegion {
			c.activeRegion = realActiveRegion
			// Reset failure counter when active region changes externally
			c.failureCounter = make(map[string]int)
			c.logger.Info("active region changed externally, syncing healthcheck",
				slog.String("old_region", currentActiveRegion),
				slog.String("new_region", realActiveRegion),
			)
		}
		c.mu.Unlock()
	}

	// Get currently monitored active region
	c.mu.RLock()
	activeRegion := c.activeRegion
	c.mu.RUnlock()

	if activeRegion == "" {
		c.logger.Info("no active region configured, skipping health check")
		return
	}

	c.logger.Info("checking health of active region",
		slog.String("region", activeRegion),
	)

	// Check if region has a leader
	hasLeader, err := c.checkRegionLeader(ctx, activeRegion)
	if err != nil {
		c.logger.Warn("health check failed",
			slog.String("region", activeRegion),
			slog.String("error", err.Error()),
		)
		c.handleFailure(ctx, activeRegion)
		return
	}

	if !hasLeader {
		c.logger.Warn("region has no leader",
			slog.String("region", activeRegion),
		)
		c.handleFailure(ctx, activeRegion)
		return
	}

	// Health check passed - reset failure counter
	c.mu.Lock()
	previousFailures := c.failureCounter[activeRegion]
	c.failureCounter[activeRegion] = 0
	c.mu.Unlock()

	if previousFailures > 0 {
		c.logger.Info("region health check passed - health restored",
			slog.String("region", activeRegion),
			slog.Int("previous_failures", previousFailures),
		)
	} else {
		c.logger.Info("region health check passed",
			slog.String("region", activeRegion),
		)
	}
}

// detectActiveRegion determines which region is currently active (has un-drained DCs)
// This is called only once during startup
func (c *Checker) detectActiveRegion(ctx context.Context) (string, error) {
	regions, err := c.dcService.ListRegions(ctx)
	if err != nil {
		return "", err
	}

	// Find region with status "active" or "partial" (has some un-drained DCs)
	for _, region := range regions {
		if region.Status == "active" || region.Status == "partial" {
			return region.Name, nil
		}
	}

	return "", nil
}

// checkRegionLeader checks if any cluster in the region has an elected leader
func (c *Checker) checkRegionLeader(ctx context.Context, region string) (bool, error) {
	// Get region details to access datacenters
	regionDetails, err := c.dcService.GetRegionDatacenters(ctx, region)
	if err != nil {
		return false, err
	}

	if regionDetails == nil || len(regionDetails.Datacenters) == 0 {
		c.logger.Warn("region has no datacenters",
			slog.String("region", region),
		)
		return false, nil
	}

	// Check leader on first datacenter (all DCs in region share same Nomad Server cluster)
	firstDC := regionDetails.Datacenters[0]

	hasLeader, err := c.dcService.CheckClusterLeader(ctx, firstDC.Name)
	if err != nil {
		c.logger.Warn("failed to check leader",
			slog.String("region", region),
			slog.String("datacenter", firstDC.Name),
			slog.String("error", err.Error()),
		)
		return false, err
	}

	return hasLeader, nil
}

// handleFailure increments failure counter and drains region if threshold is reached
func (c *Checker) handleFailure(ctx context.Context, region string) {
	c.mu.Lock()
	c.failureCounter[region]++
	currentFailures := c.failureCounter[region]
	c.mu.Unlock()

	c.logger.Warn("region health check failure",
		slog.String("region", region),
		slog.Int("consecutive_failures", currentFailures),
		slog.Int("threshold", c.cfg.FailedThreshold),
	)

	// Check if threshold is reached
	if currentFailures >= c.cfg.FailedThreshold {
		c.logger.Error("region health check threshold reached, draining region",
			slog.String("region", region),
			slog.Int("failures", currentFailures),
		)

		// Drain the region
		if err := c.drainRegion(ctx, region); err != nil {
			c.logger.Error("failed to drain unhealthy region",
				slog.String("region", region),
				slog.String("error", err.Error()),
			)
		} else {
			c.logger.Info("successfully drained unhealthy region",
				slog.String("region", region),
			)
			// Reset counter after successful drain
			c.mu.Lock()
			c.failureCounter[region] = 0
			c.mu.Unlock()
		}
	}
}

// drainRegion drains all datacenters in the region by setting all nodes to drain
func (c *Checker) drainRegion(ctx context.Context, region string) error {
	c.logger.Info("draining unhealthy region",
		slog.String("region", region),
	)

	err := c.dcService.DrainAllNodesInRegion(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to drain region: %w", err)
	}

	c.logger.Info("successfully drained unhealthy region",
		slog.String("region", region),
	)

	return nil
}
