package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kirychukyurii/webitel-dc-switcher/internal/cache"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/concurrent"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/model"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/repository"
)

// HealthChecker defines interface for health check operations
type HealthChecker interface {
	SetActiveRegion(region string)
}

// DatacenterService defines the interface for datacenter operations
type DatacenterService interface {
	ListDatacenters(ctx context.Context) ([]model.Datacenter, error)
	ListRegions(ctx context.Context) ([]model.Region, error)
	GetDatacentersByRegion(ctx context.Context, region string) ([]model.Datacenter, error)
	GetRegionDatacenters(ctx context.Context, region string) (*model.Region, error)
	CheckClusterLeader(ctx context.Context, clusterName string) (bool, error)
	GetNodes(ctx context.Context, dc string) ([]model.Node, error)
	ActivateDatacenter(ctx context.Context, dc string) (*model.ActivationResult, error)
	ActivateRegion(ctx context.Context, region string) (*model.ActivationResult, error)
	DrainAllNodesInRegion(ctx context.Context, region string) error
	EnsureSingleActiveDatacenter(ctx context.Context) error
	PerformStartupReconciliation(ctx context.Context) error
	StartHeartbeat(ctx context.Context)
	StopHeartbeat()
	SetHealthChecker(hc HealthChecker)
	GetJobs(ctx context.Context, dc string) ([]model.Job, error)
	StartJob(ctx context.Context, dc, jobID string) (*model.JobActionResult, error)
	StopJob(ctx context.Context, dc, jobID string) (*model.JobActionResult, error)
}

// datacenterService implements DatacenterService interface
type datacenterService struct {
	repo          repository.NomadRepository
	etcdRepo      repository.EtcdRepository
	cache         cache.Cache
	ttl           time.Duration
	logger        *slog.Logger
	healthChecker HealthChecker
	myDatacenter  string
	heartbeatCfg  config.HeartbeatConfig
	amDrained     bool // Tracks if we intentionally drained our nodes
	stopHeartbeat chan struct{}
}

// clusterNodesInfo stores nodes information for a cluster
type clusterNodesInfo struct {
	clusterName string
	nodes       []model.Node
	region      string
	err         error
}

// NewDatacenterService creates a new datacenter service
func NewDatacenterService(
	repo repository.NomadRepository,
	etcdRepo repository.EtcdRepository,
	cache cache.Cache,
	ttl time.Duration,
	myDatacenter string,
	heartbeatCfg config.HeartbeatConfig,
	logger *slog.Logger,
) DatacenterService {
	return &datacenterService{
		repo:          repo,
		etcdRepo:      etcdRepo,
		cache:         cache,
		ttl:           ttl,
		logger:        logger,
		myDatacenter:  myDatacenter,
		heartbeatCfg:  heartbeatCfg,
		stopHeartbeat: make(chan struct{}),
	}
}

// ListDatacenters returns information about all datacenters
func (s *datacenterService) ListDatacenters(ctx context.Context) ([]model.Datacenter, error) {
	clusterNames := s.repo.GetClusterNames()

	// Fetch datacenter info in parallel
	results := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, name string) (model.Datacenter, error) {
		dc, err := s.getDatacenterInfo(ctx, name)
		if err != nil {
			s.logger.Error("failed to get datacenter info",
				slog.String("datacenter", name),
				slog.String("error", err.Error()),
			)
			// Return error status for this datacenter instead of failing
			return model.Datacenter{
				Name:   name,
				Status: model.DatacenterStatusError,
			}, nil
		}
		return dc, nil
	})

	// Collect all results (errors are already handled above)
	datacenters := make([]model.Datacenter, 0, len(results))
	for _, result := range results {
		datacenters = append(datacenters, result.Value)
	}

	return datacenters, nil
}

// getDatacenterInfo retrieves datacenter information with caching
func (s *datacenterService) getDatacenterInfo(ctx context.Context, name string) (model.Datacenter, error) {
	nodes, err := s.GetNodes(ctx, name)
	if err != nil {
		return model.Datacenter{}, err
	}

	// Get region for this datacenter
	region, err := s.repo.GetClusterRegion(name)
	if err != nil {
		region = "unknown"
	}

	dc := model.Datacenter{
		Name:       name,
		Region:     region,
		NodesTotal: len(nodes),
	}

	// Calculate status and node statistics
	// A node is ready if it's not draining AND is eligible for scheduling
	for _, node := range nodes {
		if node.Drain {
			dc.NodesDraining++
		} else if node.SchedulingEligibility == "eligible" {
			dc.NodesReady++
		} else {
			// Node is not draining but also not eligible (ineligible)
			// Count as draining for status purposes
			dc.NodesDraining++
		}
	}

	// Determine datacenter status
	if dc.NodesDraining == dc.NodesTotal {
		dc.Status = model.DatacenterStatusDraining
	} else if dc.NodesReady > 0 {
		dc.Status = model.DatacenterStatusActive
	} else {
		// All nodes are either draining or ineligible
		dc.Status = model.DatacenterStatusDraining
	}

	// Get jobs statistics
	jobs, err := s.repo.ListJobs(ctx, name)
	if err != nil {
		// Log error but don't fail - jobs stats are optional
		s.logger.Warn("failed to get jobs for datacenter",
			slog.String("datacenter", name),
			slog.String("error", err.Error()),
		)
	} else {
		dc.JobsTotal = len(jobs)
		for _, job := range jobs {
			if job.Status == "running" {
				dc.JobsRunning++
			} else if job.Status == "dead" {
				dc.JobsStopped++
			}
		}
	}

	return dc, nil
}

// GetNodes returns all nodes for a specific datacenter
func (s *datacenterService) GetNodes(ctx context.Context, dc string) ([]model.Node, error) {
	cacheKey := fmt.Sprintf("%s:nodes", dc)

	// Try to get from cache
	if cached, ok := s.cache.Get(cacheKey); ok {
		if nodes, ok := cached.([]model.Node); ok {
			s.logger.Debug("nodes retrieved from cache",
				slog.String("datacenter", dc),
				slog.Int("count", len(nodes)),
			)
			return nodes, nil
		}
	}

	// Fetch from repository
	nodes, err := s.repo.ListNodes(ctx, dc)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Store in cache
	s.cache.Set(cacheKey, nodes, s.ttl)

	return nodes, nil
}

// ActivateDatacenter activates the specified datacenter and drains all datacenters in other regions
// Uses continue-on-error approach: collects errors but continues with other clusters/nodes
func (s *datacenterService) ActivateDatacenter(ctx context.Context, targetDC string) (*model.ActivationResult, error) {
	s.logger.Info("starting datacenter activation",
		slog.String("target_datacenter", targetDC),
	)

	result := &model.ActivationResult{
		Activated: targetDC,
		Errors:    []string{},
	}

	clusterNames := s.repo.GetClusterNames()

	// Verify target datacenter exists and get its region
	targetRegion, err := s.repo.GetClusterRegion(targetDC)
	if err != nil {
		return nil, fmt.Errorf("target datacenter %s not found: %w", targetDC, err)
	}

	s.logger.Info("activating datacenter in region",
		slog.String("target_datacenter", targetDC),
		slog.String("target_region", targetRegion),
	)

	// OPTIMIZATION: Fetch nodes from all clusters in parallel
	clusterNodesResults := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, clusterName string) (clusterNodesInfo, error) {
		clusterRegion, err := s.repo.GetClusterRegion(clusterName)
		if err != nil {
			s.logger.Warn("failed to get cluster region",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesInfo{clusterName: clusterName, err: err}, nil
		}

		// Skip datacenters in the same region (except target) - preserve their state
		if clusterRegion == targetRegion && clusterName != targetDC {
			s.logger.Debug("skipping datacenter in same region",
				slog.String("cluster", clusterName),
				slog.String("region", clusterRegion),
			)
			return clusterNodesInfo{clusterName: clusterName, region: clusterRegion}, nil // Skip but no error
		}

		nodes, err := s.GetNodes(ctx, clusterName)
		if err != nil {
			s.logger.Error("failed to get nodes",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesInfo{clusterName: clusterName, err: err}, nil
		}

		return clusterNodesInfo{
			clusterName: clusterName,
			nodes:       nodes,
			region:      clusterRegion,
		}, nil
	})

	// Process all datacenters - continue on error, collect errors
	for _, clusterResult := range clusterNodesResults {
		clusterInfo := clusterResult.Value

		// If error fetching this cluster, add to errors and continue
		if clusterInfo.err != nil {
			errMsg := fmt.Sprintf("cluster %s: failed to fetch nodes: %v", clusterInfo.clusterName, clusterInfo.err)
			result.Errors = append(result.Errors, errMsg)
			s.logger.Warn("skipping cluster due to error",
				slog.String("cluster", clusterInfo.clusterName),
				slog.String("error", clusterInfo.err.Error()),
			)
			continue
		}

		// Skip if this cluster was marked to skip (same region, not target)
		if len(clusterInfo.nodes) == 0 && clusterInfo.err == nil {
			continue
		}

		clusterName := clusterInfo.clusterName
		nodes := clusterInfo.nodes

		// Drain if in different region, activate if target datacenter
		shouldDrain := clusterName != targetDC
		shouldBeEligible := !shouldDrain // When not draining, node should be eligible

		// Collect nodes that need changes
		type nodeToChange struct {
			node           model.Node
			nodeIsEligible bool
			alreadyCorrect bool
		}

		nodesToChange := make([]nodeToChange, 0, len(nodes))
		for _, node := range nodes {
			nodeIsEligible := node.SchedulingEligibility == "eligible"
			alreadyCorrect := (node.Drain == shouldDrain) && (nodeIsEligible == shouldBeEligible)

			nodesToChange = append(nodesToChange, nodeToChange{
				node:           node,
				nodeIsEligible: nodeIsEligible,
				alreadyCorrect: alreadyCorrect,
			})
		}

		// OPTIMIZATION: Apply changes to nodes in parallel
		type nodeResult struct {
			nodeID  string
			success bool
		}

		nodeResults := concurrent.ParallelMap(ctx, nodesToChange, func(ctx context.Context, ntc nodeToChange) (nodeResult, error) {
			if ntc.alreadyCorrect {
				return nodeResult{nodeID: "", success: true}, nil // Skip, already correct
			}

			// Apply the change
			err := s.repo.SetNodeDrain(ctx, clusterName, ntc.node.ID, shouldDrain)
			if err != nil {
				s.logger.Error("failed to set node drain",
					slog.String("cluster", clusterName),
					slog.String("node_id", ntc.node.ID),
					slog.Bool("drain", shouldDrain),
					slog.String("error", err.Error()),
				)
				return nodeResult{nodeID: ntc.node.ID, success: false}, err
			}

			return nodeResult{nodeID: ntc.node.ID, success: true}, nil
		})

		// Collect errors and update counters - CONTINUE on error
		for _, nr := range nodeResults {
			if nr.Error != nil {
				// Add error but continue with other nodes
				errMsg := fmt.Sprintf("cluster %s, node %s: %v", clusterName, nr.Value.nodeID, nr.Error)
				result.Errors = append(result.Errors, errMsg)
			} else if nr.Value.success && nr.Value.nodeID != "" {
				// Update counters only for successful changes
				if shouldDrain {
					result.DrainedNodes++
				} else {
					result.UnDrainedNodes++
				}
			}
		}

		// Invalidate cache for this cluster
		s.cache.Delete(fmt.Sprintf("%s:nodes", clusterName))
	}

	s.logger.Info("datacenter activation completed",
		slog.String("activated", targetDC),
		slog.Int("drained_nodes", result.DrainedNodes),
		slog.Int("un_drained_nodes", result.UnDrainedNodes),
		slog.Int("errors_count", len(result.Errors)),
	)

	// Trigger job evaluations for the activated datacenter to redistribute allocations
	if result.UnDrainedNodes > 0 {
		s.logger.Info("triggering job evaluations for activated datacenter",
			slog.String("datacenter", targetDC),
		)
		if err := s.repo.TriggerJobEvaluations(ctx, targetDC); err != nil {
			// Log error but don't fail the activation
			errMsg := fmt.Sprintf("failed to trigger job evaluations for %s: %v", targetDC, err)
			result.Errors = append(result.Errors, errMsg)
			s.logger.Warn("failed to trigger job evaluations",
				slog.String("datacenter", targetDC),
				slog.String("error", err.Error()),
			)
		} else {
			s.logger.Info("job evaluations triggered successfully",
				slog.String("datacenter", targetDC),
			)
		}
	}

	// Write active datacenter info to etcd
	activeInfo := &model.ActiveDatacenter{
		Datacenter:    targetDC,
		ActivatedAt:   time.Now(),
		ActivatedBy:   "api",
		LastHeartbeat: time.Now(),
	}
	if err := s.etcdRepo.WriteActiveDatacenter(ctx, activeInfo); err != nil {
		s.logger.Error("failed to write active datacenter to etcd",
			"datacenter", targetDC,
			"error", err.Error())
		// Add to errors but don't fail activation
		result.Errors = append(result.Errors, fmt.Sprintf("failed to write to etcd: %v", err))
	} else {
		s.logger.Info("wrote active datacenter to etcd", "datacenter", targetDC)
		// Update local state
		s.amDrained = false
	}

	// Update health checker to monitor the region of the newly activated datacenter
	if s.healthChecker != nil {
		s.healthChecker.SetActiveRegion(targetRegion)
	}

	return result, nil
}

// ListRegions returns information about all regions with their datacenters
func (s *datacenterService) ListRegions(ctx context.Context) ([]model.Region, error) {
	regionNames := s.repo.GetAllRegions()

	// Fetch region info in parallel
	results := concurrent.ParallelMap(ctx, regionNames, func(ctx context.Context, regionName string) (model.Region, error) {
		region, err := s.getRegionInfo(ctx, regionName)
		if err != nil {
			s.logger.Error("failed to get region info",
				slog.String("region", regionName),
				slog.String("error", err.Error()),
			)
			// Return error status for this region instead of failing
			return model.Region{
				Name:        regionName,
				Datacenters: []model.Datacenter{},
				Status:      model.DatacenterStatusError,
			}, nil
		}
		return region, nil
	})

	// Collect all results (errors are already handled above)
	regions := make([]model.Region, 0, len(results))
	for _, result := range results {
		regions = append(regions, result.Value)
	}

	return regions, nil
}

// getRegionInfo retrieves region information including all its datacenters
func (s *datacenterService) getRegionInfo(ctx context.Context, regionName string) (model.Region, error) {
	clusterNames := s.repo.GetClustersByRegion(regionName)

	// Fetch datacenter info in parallel
	results := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, name string) (model.Datacenter, error) {
		dc, err := s.getDatacenterInfo(ctx, name)
		if err != nil {
			s.logger.Error("failed to get datacenter info",
				slog.String("datacenter", name),
				slog.String("region", regionName),
				slog.String("error", err.Error()),
			)
			// Return error status for this datacenter instead of failing
			return model.Datacenter{
				Name:   name,
				Region: regionName,
				Status: model.DatacenterStatusError,
			}, nil
		}
		return dc, nil
	})

	// Collect results and count statuses
	datacenters := make([]model.Datacenter, 0, len(results))
	activeCount := 0
	drainingCount := 0
	errorCount := 0
	totalJobs := 0
	runningJobs := 0
	stoppedJobs := 0

	for _, result := range results {
		dc := result.Value
		datacenters = append(datacenters, dc)

		// Count status
		switch dc.Status {
		case model.DatacenterStatusActive:
			activeCount++
		case model.DatacenterStatusDraining:
			drainingCount++
		case model.DatacenterStatusError:
			errorCount++
		}

		// Aggregate jobs statistics
		totalJobs += dc.JobsTotal
		runningJobs += dc.JobsRunning
		stoppedJobs += dc.JobsStopped
	}

	// Determine region status
	regionStatus := model.DatacenterStatusActive
	if errorCount > 0 {
		regionStatus = model.DatacenterStatusError
	} else if drainingCount == len(clusterNames) {
		regionStatus = model.DatacenterStatusDraining
	} else if activeCount > 0 && drainingCount > 0 {
		regionStatus = "partial" // Some DCs active, some draining
	}

	return model.Region{
		Name:        regionName,
		Datacenters: datacenters,
		Status:      regionStatus,
		JobsTotal:   totalJobs,
		JobsRunning: runningJobs,
		JobsStopped: stoppedJobs,
	}, nil
}

// GetDatacentersByRegion returns all datacenters in a specific region
func (s *datacenterService) GetDatacentersByRegion(ctx context.Context, region string) ([]model.Datacenter, error) {
	clusterNames := s.repo.GetClustersByRegion(region)
	if len(clusterNames) == 0 {
		return nil, fmt.Errorf("region %s not found or has no datacenters", region)
	}

	// Fetch datacenter info in parallel
	results := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, name string) (model.Datacenter, error) {
		dc, err := s.getDatacenterInfo(ctx, name)
		if err != nil {
			s.logger.Error("failed to get datacenter info",
				slog.String("datacenter", name),
				slog.String("region", region),
				slog.String("error", err.Error()),
			)
			// Return error status for this datacenter instead of failing
			return model.Datacenter{
				Name:   name,
				Region: region,
				Status: model.DatacenterStatusError,
			}, nil
		}
		return dc, nil
	})

	// Collect all results (errors are already handled above)
	datacenters := make([]model.Datacenter, 0, len(results))
	for _, result := range results {
		datacenters = append(datacenters, result.Value)
	}

	return datacenters, nil
}

// ActivateRegion activates all datacenters in a specific region and drains all others
// Uses continue-on-error approach: collects errors but continues with other clusters/nodes
func (s *datacenterService) ActivateRegion(ctx context.Context, targetRegion string) (*model.ActivationResult, error) {
	s.logger.Info("starting region activation",
		slog.String("target_region", targetRegion),
	)

	// Verify target region exists
	targetClusters := s.repo.GetClustersByRegion(targetRegion)
	if len(targetClusters) == 0 {
		return nil, fmt.Errorf("region %s not found or has no datacenters", targetRegion)
	}

	result := &model.ActivationResult{
		Activated: targetRegion,
		Errors:    []string{},
	}

	allClusters := s.repo.GetClusterNames()

	// OPTIMIZATION: Fetch nodes from all clusters in parallel
	clusterNodesResults := concurrent.ParallelMap(ctx, allClusters, func(ctx context.Context, clusterName string) (clusterNodesInfo, error) {
		clusterRegion, err := s.repo.GetClusterRegion(clusterName)
		if err != nil {
			s.logger.Error("failed to get cluster region",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesInfo{clusterName: clusterName, err: err}, nil
		}

		nodes, err := s.GetNodes(ctx, clusterName)
		if err != nil {
			s.logger.Error("failed to get nodes",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesInfo{clusterName: clusterName, err: err}, nil
		}

		return clusterNodesInfo{
			clusterName: clusterName,
			nodes:       nodes,
			region:      clusterRegion,
		}, nil
	})

	// Process all datacenters - continue on error, collect errors
	for _, clusterResult := range clusterNodesResults {
		clusterInfo := clusterResult.Value

		// If error fetching this cluster, add to errors and continue
		if clusterInfo.err != nil {
			errMsg := fmt.Sprintf("cluster %s: failed to fetch nodes: %v", clusterInfo.clusterName, clusterInfo.err)
			result.Errors = append(result.Errors, errMsg)
			s.logger.Warn("skipping cluster due to error",
				slog.String("cluster", clusterInfo.clusterName),
				slog.String("error", clusterInfo.err.Error()),
			)
			continue
		}

		clusterName := clusterInfo.clusterName
		nodes := clusterInfo.nodes
		clusterRegion := clusterInfo.region

		// Determine if nodes should be drained (drain all except target region)
		shouldDrain := clusterRegion != targetRegion
		shouldBeEligible := !shouldDrain // When not draining, node should be eligible

		// Collect nodes that need changes
		type nodeToChange struct {
			node           model.Node
			nodeIsEligible bool
			alreadyCorrect bool
		}

		nodesToChange := make([]nodeToChange, 0, len(nodes))
		for _, node := range nodes {
			nodeIsEligible := node.SchedulingEligibility == "eligible"
			alreadyCorrect := (node.Drain == shouldDrain) && (nodeIsEligible == shouldBeEligible)

			nodesToChange = append(nodesToChange, nodeToChange{
				node:           node,
				nodeIsEligible: nodeIsEligible,
				alreadyCorrect: alreadyCorrect,
			})
		}

		// OPTIMIZATION: Apply changes to nodes in parallel
		type nodeResult struct {
			nodeID  string
			success bool
		}

		nodeResults := concurrent.ParallelMap(ctx, nodesToChange, func(ctx context.Context, ntc nodeToChange) (nodeResult, error) {
			if ntc.alreadyCorrect {
				return nodeResult{nodeID: "", success: true}, nil // Skip, already correct
			}

			// Apply the change
			err := s.repo.SetNodeDrain(ctx, clusterName, ntc.node.ID, shouldDrain)
			if err != nil {
				s.logger.Error("failed to set node drain",
					slog.String("cluster", clusterName),
					slog.String("node_id", ntc.node.ID),
					slog.Bool("drain", shouldDrain),
					slog.String("error", err.Error()),
				)
				return nodeResult{nodeID: ntc.node.ID, success: false}, err
			}

			return nodeResult{nodeID: ntc.node.ID, success: true}, nil
		})

		// Collect errors and update counters - CONTINUE on error
		for _, nr := range nodeResults {
			if nr.Error != nil {
				// Add error but continue with other nodes
				errMsg := fmt.Sprintf("cluster %s, node %s: %v", clusterName, nr.Value.nodeID, nr.Error)
				result.Errors = append(result.Errors, errMsg)
			} else if nr.Value.success && nr.Value.nodeID != "" {
				// Update counters only for successful changes
				if shouldDrain {
					result.DrainedNodes++
				} else {
					result.UnDrainedNodes++
				}
			}
		}

		// Invalidate cache for this cluster
		s.cache.Delete(fmt.Sprintf("%s:nodes", clusterName))
	}

	s.logger.Info("region activation completed",
		slog.String("activated", targetRegion),
		slog.Int("drained_nodes", result.DrainedNodes),
		slog.Int("un_drained_nodes", result.UnDrainedNodes),
		slog.Int("errors_count", len(result.Errors)),
	)

	// Trigger job evaluations for all datacenters in the activated region
	if result.UnDrainedNodes > 0 {
		s.logger.Info("triggering job evaluations for activated region",
			slog.String("region", targetRegion),
			slog.Int("datacenters", len(targetClusters)),
		)

		// Trigger evaluations for all clusters in the region in parallel
		evalErrors := []string{}
		for _, clusterName := range targetClusters {
			if err := s.repo.TriggerJobEvaluations(ctx, clusterName); err != nil {
				errMsg := fmt.Sprintf("datacenter %s: %v", clusterName, err)
				evalErrors = append(evalErrors, errMsg)
				s.logger.Warn("failed to trigger job evaluations",
					slog.String("datacenter", clusterName),
					slog.String("error", err.Error()),
				)
			} else {
				s.logger.Info("job evaluations triggered successfully",
					slog.String("datacenter", clusterName),
				)
			}
		}

		if len(evalErrors) > 0 {
			result.Errors = append(result.Errors, evalErrors...)
		}
	}

	// Write active datacenter info to etcd (choose first DC in region as active)
	if len(targetClusters) > 0 {
		activeDatacenter := targetClusters[0]
		activeInfo := &model.ActiveDatacenter{
			Datacenter:    activeDatacenter,
			ActivatedAt:   time.Now(),
			ActivatedBy:   "api-region",
			LastHeartbeat: time.Now(),
		}
		if err := s.etcdRepo.WriteActiveDatacenter(ctx, activeInfo); err != nil {
			s.logger.Error("failed to write active datacenter to etcd",
				"datacenter", activeDatacenter,
				"region", targetRegion,
				"error", err.Error())
			// Add to errors but don't fail activation
			result.Errors = append(result.Errors, fmt.Sprintf("failed to write to etcd: %v", err))
		} else {
			s.logger.Info("wrote active datacenter to etcd",
				"datacenter", activeDatacenter,
				"region", targetRegion)
			// Update local state if this is my datacenter
			if activeDatacenter == s.myDatacenter {
				s.amDrained = false
			}
		}
	}

	// Update health checker to monitor the newly activated region
	if s.healthChecker != nil {
		s.healthChecker.SetActiveRegion(targetRegion)
	}

	return result, nil
}

// EnsureSingleActiveDatacenter ensures only one region is active at startup
// If multiple regions have active datacenters, it keeps the first region active and drains all others
func (s *datacenterService) EnsureSingleActiveDatacenter(ctx context.Context) error {
	s.logger.Info("checking region states at startup")

	clusterNames := s.repo.GetClusterNames()

	// OPTIMIZATION: Fetch nodes from all clusters in parallel
	type clusterNodesWithRegion struct {
		clusterName string
		nodes       []model.Node
		region      string
		err         error
	}

	clusterNodesResults := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, clusterName string) (clusterNodesWithRegion, error) {
		nodes, err := s.GetNodes(ctx, clusterName)
		if err != nil {
			s.logger.Warn("failed to get nodes during startup check",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesWithRegion{clusterName: clusterName, err: err}, nil
		}

		region, err := s.repo.GetClusterRegion(clusterName)
		if err != nil {
			s.logger.Warn("failed to get cluster region during startup check",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return clusterNodesWithRegion{clusterName: clusterName, nodes: nodes, err: err}, nil
		}

		return clusterNodesWithRegion{
			clusterName: clusterName,
			nodes:       nodes,
			region:      region,
		}, nil
	})

	// Map: region -> list of active datacenters in that region
	activeDatacentersByRegion := make(map[string][]string)

	// Find all datacenters with ready nodes, grouped by region
	for _, result := range clusterNodesResults {
		info := result.Value
		if info.err != nil {
			continue
		}

		// Check if datacenter has any ready nodes
		hasReadyNodes := false
		for _, node := range info.nodes {
			if !node.Drain && node.SchedulingEligibility == "eligible" {
				hasReadyNodes = true
				break
			}
		}

		if hasReadyNodes {
			activeDatacentersByRegion[info.region] = append(activeDatacentersByRegion[info.region], info.clusterName)
		}
	}

	// If 0 or 1 active regions, no action needed
	if len(activeDatacentersByRegion) <= 1 {
		if len(activeDatacentersByRegion) == 1 {
			for region, dcs := range activeDatacentersByRegion {
				s.logger.Info("found single active region at startup",
					slog.String("region", region),
					slog.Int("active_datacenters", len(dcs)),
					slog.Any("datacenters", dcs),
				)
			}
		} else {
			s.logger.Info("no active regions found at startup")
		}
		return nil
	}

	// Multiple active regions found - keep first one, drain others
	var activeRegions []string
	for region := range activeDatacentersByRegion {
		activeRegions = append(activeRegions, region)
	}

	keepActiveRegion := activeRegions[0]
	drainRegions := activeRegions[1:]

	s.logger.Warn("multiple active regions detected at startup",
		slog.String("keeping_active_region", keepActiveRegion),
		slog.Int("draining_regions_count", len(drainRegions)),
		slog.Any("draining_regions", drainRegions),
	)

	// Drain all datacenters in other regions
	for _, region := range drainRegions {
		datacenters := activeDatacentersByRegion[region]
		s.logger.Info("draining region",
			slog.String("region", region),
			slog.Any("datacenters", datacenters),
		)

		// Collect all nodes to drain across all datacenters in this region
		type nodeToDrain struct {
			clusterName string
			node        model.Node
		}
		var nodesToDrain []nodeToDrain

		for _, clusterName := range datacenters {
			// Find nodes from our earlier fetch
			for _, result := range clusterNodesResults {
				if result.Value.clusterName == clusterName && result.Value.err == nil {
					for _, node := range result.Value.nodes {
						// Only update nodes that are currently ready
						if !node.Drain && node.SchedulingEligibility == "eligible" {
							nodesToDrain = append(nodesToDrain, nodeToDrain{
								clusterName: clusterName,
								node:        node,
							})
						}
					}
					break
				}
			}
		}

		// OPTIMIZATION: Drain all nodes in parallel
		drainResults := concurrent.ParallelMap(ctx, nodesToDrain, func(ctx context.Context, ntd nodeToDrain) (string, error) {
			err := s.repo.SetNodeDrain(ctx, ntd.clusterName, ntd.node.ID, true)
			if err != nil {
				s.logger.Error("failed to drain node during startup sync",
					slog.String("cluster", ntd.clusterName),
					slog.String("node_id", ntd.node.ID),
					slog.String("error", err.Error()),
				)
				return "", err
			}

			s.logger.Info("drained node during startup sync",
				slog.String("cluster", ntd.clusterName),
				slog.String("node_id", ntd.node.ID),
			)
			return ntd.clusterName, nil
		})

		// Collect unique cluster names that were modified to invalidate cache
		modifiedClusters := make(map[string]bool)
		for _, result := range drainResults {
			if result.Value != "" {
				modifiedClusters[result.Value] = true
			}
		}

		// Invalidate cache for modified clusters
		for clusterName := range modifiedClusters {
			s.cache.Delete(fmt.Sprintf("%s:nodes", clusterName))
		}
	}

	s.logger.Info("startup region sync completed",
		slog.String("active_region", keepActiveRegion),
		slog.Int("drained_regions", len(drainRegions)),
	)

	return nil
}

// GetRegionDatacenters returns detailed information about a specific region and its datacenters
func (s *datacenterService) GetRegionDatacenters(ctx context.Context, region string) (*model.Region, error) {
	regionInfo, err := s.getRegionInfo(ctx, region)
	if err != nil {
		return nil, fmt.Errorf("failed to get region info: %w", err)
	}
	return &regionInfo, nil
}

// CheckClusterLeader checks if the specified cluster has an elected leader
func (s *datacenterService) CheckClusterLeader(ctx context.Context, clusterName string) (bool, error) {
	hasLeader, err := s.repo.CheckLeader(ctx, clusterName)
	if err != nil {
		return false, fmt.Errorf("failed to check leader: %w", err)
	}
	return hasLeader, nil
}

// SetHealthChecker sets the health checker instance for notifying about region changes
func (s *datacenterService) SetHealthChecker(hc HealthChecker) {
	s.healthChecker = hc
}

// DrainAllNodesInRegion drains all nodes in all datacenters in the specified region
func (s *datacenterService) DrainAllNodesInRegion(ctx context.Context, region string) error {
	// Get all clusters in this region
	clusterNames := s.repo.GetClustersByRegion(region)
	if len(clusterNames) == 0 {
		return fmt.Errorf("no clusters found in region %s", region)
	}

	s.logger.Info("draining all nodes in region",
		slog.String("region", region),
		slog.Int("cluster_count", len(clusterNames)),
	)

	var errors []string

	// Drain nodes in all clusters in parallel
	drainResults := concurrent.ParallelMap(ctx, clusterNames, func(ctx context.Context, clusterName string) (int, error) {
		// Get nodes for this cluster
		nodes, err := s.GetNodes(ctx, clusterName)
		if err != nil {
			s.logger.Error("failed to get nodes for draining",
				slog.String("cluster", clusterName),
				slog.String("error", err.Error()),
			)
			return 0, err
		}

		drainedCount := 0
		// Drain each node that is not already drained
		for _, node := range nodes {
			if !node.Drain {
				err := s.repo.SetNodeDrain(ctx, clusterName, node.ID, true)
				if err != nil {
					s.logger.Error("failed to drain node",
						slog.String("cluster", clusterName),
						slog.String("node_id", node.ID),
						slog.String("node_name", node.Name),
						slog.String("error", err.Error()),
					)
					continue
				}
				drainedCount++
			}
		}

		s.logger.Info("drained nodes in cluster",
			slog.String("cluster", clusterName),
			slog.Int("drained_count", drainedCount),
			slog.Int("total_nodes", len(nodes)),
		)

		return drainedCount, nil
	})

	// Collect results and errors
	totalDrained := 0
	for _, result := range drainResults {
		if result.Error != nil {
			errors = append(errors, fmt.Sprintf("cluster drain error: %v", result.Error))
		} else {
			totalDrained += result.Value
		}
	}

	// Invalidate cache for all clusters in region
	for _, clusterName := range clusterNames {
		s.cache.Delete(fmt.Sprintf("%s:nodes", clusterName))
	}

	s.logger.Info("completed draining region",
		slog.String("region", region),
		slog.Int("total_drained_nodes", totalDrained),
		slog.Int("error_count", len(errors)),
	)

	if len(errors) > 0 {
		return fmt.Errorf("some drain operations failed: %v", errors)
	}

	return nil
}

// GetJobs returns all jobs for a specific datacenter
func (s *datacenterService) GetJobs(ctx context.Context, dc string) ([]model.Job, error) {
	jobs, err := s.repo.ListJobs(ctx, dc)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	return jobs, nil
}

// StartJob starts a stopped job in the specified datacenter
func (s *datacenterService) StartJob(ctx context.Context, dc, jobID string) (*model.JobActionResult, error) {
	s.logger.Info("starting job",
		slog.String("datacenter", dc),
		slog.String("job_id", jobID),
	)

	result := &model.JobActionResult{
		JobID:   jobID,
		Action:  "start",
		Success: false,
		Errors:  []string{},
	}

	err := s.repo.StartJob(ctx, dc, jobID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to start job %s: %v", jobID, err)
		result.Errors = append(result.Errors, errMsg)
		s.logger.Error("failed to start job",
			slog.String("datacenter", dc),
			slog.String("job_id", jobID),
			slog.String("error", err.Error()),
		)
		return result, err
	}

	result.Success = true
	s.logger.Info("job started successfully",
		slog.String("datacenter", dc),
		slog.String("job_id", jobID),
	)

	return result, nil
}

// StopJob stops a running job in the specified datacenter
func (s *datacenterService) StopJob(ctx context.Context, dc, jobID string) (*model.JobActionResult, error) {
	s.logger.Info("stopping job",
		slog.String("datacenter", dc),
		slog.String("job_id", jobID),
	)

	result := &model.JobActionResult{
		JobID:   jobID,
		Action:  "stop",
		Success: false,
		Errors:  []string{},
	}

	err := s.repo.StopJob(ctx, dc, jobID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to stop job %s: %v", jobID, err)
		result.Errors = append(result.Errors, errMsg)
		s.logger.Error("failed to stop job",
			slog.String("datacenter", dc),
			slog.String("job_id", jobID),
			slog.String("error", err.Error()),
		)
		return result, err
	}

	result.Success = true
	s.logger.Info("job stopped successfully",
		slog.String("datacenter", dc),
		slog.String("job_id", jobID),
	)

	return result, nil
}

// PerformStartupReconciliation reads active datacenter from etcd and reconciles local state
func (s *datacenterService) PerformStartupReconciliation(ctx context.Context) error {
	s.logger.Info("performing startup reconciliation with etcd")

	// Read active datacenter from etcd
	activeInfo, err := s.etcdRepo.ReadActiveDatacenter(ctx)
	if err != nil {
		s.logger.Warn("no active datacenter found in etcd", "error", err.Error())
		// No active datacenter in etcd - stay drained for safety
		s.logger.Info("no active datacenter in etcd, draining my nodes for safety")
		if err := s.drainMyNodes(ctx); err != nil {
			return fmt.Errorf("failed to drain nodes: %w", err)
		}
		s.amDrained = true
		return nil
	}

	s.logger.Info("found active datacenter in etcd",
		"datacenter", activeInfo.Datacenter,
		"activated_at", activeInfo.ActivatedAt,
		"heartbeat_age", activeInfo.HeartbeatAge(),
	)

	// Check if I should be active
	if activeInfo.Datacenter != s.myDatacenter {
		// Another DC is active
		s.logger.Info("another datacenter is active, ensuring my nodes are drained",
			"active_dc", activeInfo.Datacenter)
		if err := s.drainMyNodes(ctx); err != nil {
			return fmt.Errorf("failed to drain nodes: %w", err)
		}
		s.amDrained = true
		return nil
	}

	// I should be active - check heartbeat freshness
	if activeInfo.IsStale(s.heartbeatCfg.StaleThreshold) {
		age := activeInfo.HeartbeatAge()
		s.logger.Warn("I am marked as active but heartbeat is stale, staying drained for safety",
			"heartbeat_age", age,
			"threshold", s.heartbeatCfg.StaleThreshold)
		if err := s.drainMyNodes(ctx); err != nil {
			return fmt.Errorf("failed to drain nodes: %w", err)
		}
		s.amDrained = true
		return nil
	}

	// Fresh heartbeat exists but I'm starting up
	// This means another instance might be running!
	age := activeInfo.HeartbeatAge()
	if age < s.heartbeatCfg.StaleThreshold {
		s.logger.Error("fresh heartbeat exists but I'm starting up - another instance might be running!",
			"heartbeat_age", age,
			"action", "draining nodes for safety")
		if err := s.drainMyNodes(ctx); err != nil {
			return fmt.Errorf("failed to drain nodes: %w", err)
		}
		s.amDrained = true
		return fmt.Errorf("another instance of this datacenter might be running (fresh heartbeat found)")
	}

	// Heartbeat is old enough - safe to continue as active
	s.logger.Info("resuming as active datacenter")
	s.amDrained = false
	return nil
}

// drainMyNodes drains all nodes in my datacenter
func (s *datacenterService) drainMyNodes(ctx context.Context) error {
	s.logger.Info("draining all nodes in my datacenter", "datacenter", s.myDatacenter)

	nodes, err := s.GetNodes(ctx, s.myDatacenter)
	if err != nil {
		return fmt.Errorf("failed to get nodes: %w", err)
	}

	for _, node := range nodes {
		if node.Drain || node.SchedulingEligibility != "eligible" {
			// Already drained or ineligible
			continue
		}

		if err := s.repo.SetNodeDrain(ctx, s.myDatacenter, node.ID, true); err != nil {
			s.logger.Error("failed to drain node",
				"node_id", node.ID,
				"node_name", node.Name,
				"error", err.Error())
			return fmt.Errorf("failed to drain node %s: %w", node.ID, err)
		}

		s.logger.Info("drained node",
			"node_id", node.ID,
			"node_name", node.Name)
	}

	// Invalidate cache
	s.cache.Delete(s.myDatacenter + ":nodes")

	return nil
}

// StartHeartbeat starts the heartbeat update goroutine
func (s *datacenterService) StartHeartbeat(ctx context.Context) {
	go s.heartbeatLoop(ctx)
}

// StopHeartbeat stops the heartbeat update goroutine
func (s *datacenterService) StopHeartbeat() {
	close(s.stopHeartbeat)
}

// heartbeatLoop periodically updates heartbeat in etcd with fail-safe logic
func (s *datacenterService) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(s.heartbeatCfg.UpdateInterval)
	defer ticker.Stop()

	consecutiveFailures := 0

	s.logger.Info("started heartbeat updater",
		"interval", s.heartbeatCfg.UpdateInterval,
		"max_failures", s.heartbeatCfg.MaxFailures)

	for {
		select {
		case <-s.stopHeartbeat:
			s.logger.Info("stopping heartbeat updater")
			return
		case <-ticker.C:
			// Read active datacenter from etcd
			activeInfo, err := s.etcdRepo.ReadActiveDatacenter(ctx)
			if err != nil {
				consecutiveFailures++
				s.logger.Warn("failed to read active datacenter from etcd",
					"failures", consecutiveFailures,
					"error", err.Error())
				continue
			}

			// Check if another DC is now active
			if activeInfo.Datacenter != s.myDatacenter {
				s.logger.Info("another datacenter is now active, draining my nodes",
					"active_dc", activeInfo.Datacenter)
				if !s.amDrained {
					if err := s.drainMyNodes(ctx); err != nil {
						s.logger.Error("failed to drain nodes", "error", err.Error())
					} else {
						s.amDrained = true
					}
				}
				consecutiveFailures = 0
				continue
			}

			// Check for fresh heartbeat from another instance
			heartbeatAge := activeInfo.HeartbeatAge()
			if heartbeatAge < s.heartbeatCfg.StaleThreshold && s.amDrained {
				s.logger.Error("fresh heartbeat exists but I'm drained - another instance running?",
					"heartbeat_age", heartbeatAge)
				// Stay drained, don't update heartbeat
				continue
			}

			// Try to update heartbeat
			activeInfo.LastHeartbeat = time.Now()
			err = s.etcdRepo.WriteActiveDatacenter(ctx, activeInfo)
			if err != nil {
				consecutiveFailures++
				s.logger.Error("failed to update heartbeat in etcd",
					"failures", consecutiveFailures,
					"max_failures", s.heartbeatCfg.MaxFailures,
					"error", err.Error())

				if consecutiveFailures >= s.heartbeatCfg.MaxFailures && !s.amDrained {
					s.logger.Error("lost etcd quorum - draining nodes to prevent split-brain",
						"failures", consecutiveFailures)
					if err := s.drainMyNodes(ctx); err != nil {
						s.logger.Error("failed to drain nodes during etcd failure", "error", err.Error())
					} else {
						s.amDrained = true
					}
				}
			} else {
				// Success
				if consecutiveFailures > 0 {
					s.logger.Info("reconnected to etcd after failures",
						"failures", consecutiveFailures)
				}
				consecutiveFailures = 0

				// Log warning if we're successfully writing but are drained
				if s.amDrained {
					s.logger.Warn("successfully writing to etcd but nodes are drained",
						"action_required", "manual activation via API needed")
				}
			}
		}
	}
}
