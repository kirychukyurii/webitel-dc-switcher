package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	nomad "github.com/hashicorp/nomad/api"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/config"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/model"
	"github.com/kirychukyurii/webitel-dc-switcher/internal/util"
)

// NomadRepository defines the interface for Nomad API operations
type NomadRepository interface {
	ListNodes(ctx context.Context, clusterName string) ([]model.Node, error)
	SetNodeDrain(ctx context.Context, clusterName, nodeID string, drain bool) error
	CheckLeader(ctx context.Context, clusterName string) (bool, error)
	GetClusterNames() []string
	GetClusterRegion(clusterName string) (string, error)
	GetClustersByRegion(region string) []string
	GetAllRegions() []string
	TriggerJobEvaluations(ctx context.Context, clusterName string) error
	ListJobs(ctx context.Context, clusterName string) ([]model.Job, error)
	StartJob(ctx context.Context, clusterName, jobID string) error
	StopJob(ctx context.Context, clusterName, jobID string) error
}

// nodeCache stores cached information about a node for direct API access
type nodeCache struct {
	HTTPAddr string
	Name     string
}

// clusterMetadata stores metadata about a cluster
type clusterMetadata struct {
	name       string
	region     string
	client     *nomad.Client
	httpClient *http.Client          // HTTP client with TLS config for direct API calls
	nodeCache  map[string]*nodeCache // nodeID -> nodeCache
}

// nomadRepository implements NomadRepository interface
type nomadRepository struct {
	clusters map[string]*clusterMetadata
	logger   *slog.Logger
}

// NewNomadRepository creates a new Nomad repository with clients for each cluster
func NewNomadRepository(cfg *config.Config, logger *slog.Logger) (NomadRepository, error) {
	clusters := make(map[string]*clusterMetadata)
	var initErrors []string

	for i, cluster := range cfg.Clusters {
		client, httpClient, err := createNomadClient(cluster)
		if err != nil {
			return nil, fmt.Errorf("failed to create client for cluster at index %d: %w", i, err)
		}

		// Check cluster health and connectivity
		logger.Info("checking cluster health",
			slog.String("address", cluster.Address),
		)

		healthy, healthErr := checkClusterHealth(client)
		if !healthy {
			if cfg.SkipUnhealthyClusters {
				logger.Warn("skipping unhealthy cluster",
					slog.String("address", cluster.Address),
					slog.String("error", healthErr.Error()),
				)
				continue
			} else {
				logger.Error("cluster health check failed",
					slog.String("address", cluster.Address),
					slog.String("error", healthErr.Error()),
				)
				return nil, fmt.Errorf("cluster at %s is not healthy or unreachable: %w", cluster.Address, healthErr)
			}
		}

		// Auto-detect name and region from Nomad API if not specified
		name := cluster.Name
		region := cluster.Region

		if name == "" || region == "" {
			detectedName, detectedRegion, err := detectClusterInfo(client)
			if err != nil {
				logger.Warn("failed to auto-detect cluster info, using fallback values",
					slog.String("address", cluster.Address),
					slog.String("error", err.Error()),
				)
				// Use fallback values
				if name == "" {
					name = fmt.Sprintf("cluster-%d", i)
				}
				if region == "" {
					region = "global"
				}
			} else {
				if name == "" {
					name = detectedName
				}
				if region == "" {
					region = detectedRegion
				}
			}
		}

		// Check if cluster with this name already exists
		// If so, use name-region format to ensure uniqueness
		clusterKey := name
		if _, exists := clusters[name]; exists {
			clusterKey = fmt.Sprintf("%s-%s", name, region)
			logger.Warn("cluster name already exists, using name-region format",
				slog.String("original_name", name),
				slog.String("unique_key", clusterKey),
				slog.String("region", region),
			)
		}

		logger.Info("initialized cluster",
			slog.String("name", name),
			slog.String("key", clusterKey),
			slog.String("region", region),
			slog.String("address", cluster.Address),
			slog.Bool("healthy", true),
		)

		metadata := &clusterMetadata{
			name:       clusterKey, // Use unique key as name
			region:     region,
			client:     client,
			httpClient: httpClient,
			nodeCache:  make(map[string]*nodeCache),
		}

		// Cache node addresses for fallback direct API access
		if err := cacheNodeAddresses(metadata, logger); err != nil {
			logger.Warn("failed to cache node addresses, direct fallback will not be available",
				slog.String("cluster", clusterKey),
				slog.String("error", err.Error()),
			)
		}

		clusters[clusterKey] = metadata
	}

	if len(clusters) == 0 {
		return nil, fmt.Errorf("no healthy clusters available")
	}

	if len(initErrors) > 0 {
		logger.Warn("some clusters failed initialization but were skipped",
			slog.Int("failed_count", len(initErrors)),
		)
	}

	return &nomadRepository{
		clusters: clusters,
		logger:   logger,
	}, nil
}

// createNomadClient creates a Nomad API client for a cluster
func createNomadClient(cluster config.ClusterConfig) (*nomad.Client, *http.Client, error) {
	nomadConfig := nomad.DefaultConfig()
	nomadConfig.Address = cluster.Address

	// Set region if specified (used for API calls)
	if cluster.Region != "" {
		nomadConfig.Region = cluster.Region
	}

	// Configure TLS if provided
	var httpClient *http.Client
	if cluster.TLS != nil {
		tlsConfig, err := util.LoadTLSConfig(cluster.TLS)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load TLS config: %w", err)
		}

		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
			Timeout: 30 * time.Second,
		}

		nomadConfig.HttpClient = httpClient
	} else {
		// Create default HTTP client
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	client, err := nomad.NewClient(nomadConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}

	return client, httpClient, nil
}

// checkClusterHealth checks if Nomad cluster is healthy and reachable
func checkClusterHealth(client *nomad.Client) (bool, error) {
	// Try to get the leader - this is a simple health check
	status := client.Status()
	leader, err := status.Leader()
	if err != nil {
		return false, fmt.Errorf("failed to get leader: %w", err)
	}

	if leader == "" {
		return false, fmt.Errorf("no leader elected")
	}

	// Additionally check agent health
	agent := client.Agent()
	health, err := agent.Health()
	if err != nil {
		return false, fmt.Errorf("failed to get agent health: %w", err)
	}

	// Check if agent is alive
	if health == nil {
		return false, fmt.Errorf("agent health response is nil")
	}

	// Check client health if present
	if health.Client != nil && !health.Client.Ok {
		return false, fmt.Errorf("client health check failed")
	}

	// Check server health if present
	if health.Server != nil && !health.Server.Ok {
		return false, fmt.Errorf("server health check failed")
	}

	return true, nil
}

// cacheNodeAddresses fetches and caches node addresses for direct client API access
func cacheNodeAddresses(meta *clusterMetadata, logger *slog.Logger) error {
	// List all nodes first
	nodeStubs, _, err := meta.client.Nodes().List(nil)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// Fetch full node info for each node to get HTTPAddr
	cachedCount := 0
	for _, stub := range nodeStubs {
		// Get full node info which includes HTTPAddr
		node, _, err := meta.client.Nodes().Info(stub.ID, nil)
		if err != nil {
			logger.Warn("failed to get node info, skipping",
				slog.String("cluster", meta.name),
				slog.String("node_id", stub.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		if node.HTTPAddr != "" {
			meta.nodeCache[node.ID] = &nodeCache{
				HTTPAddr: node.HTTPAddr,
				Name:     node.Name,
			}
			cachedCount++
		}
	}

	logger.Info("cached node addresses for direct API access",
		slog.String("cluster", meta.name),
		slog.Int("total_nodes", len(nodeStubs)),
		slog.Int("cached_nodes", cachedCount),
	)

	return nil
}

// detectClusterInfo queries Nomad API to detect cluster name (datacenter) and region
func detectClusterInfo(client *nomad.Client) (string, string, error) {
	// Get agent self information
	agent := client.Agent()
	self, err := agent.Self()
	if err != nil {
		return "", "", fmt.Errorf("failed to query agent self: %w", err)
	}

	// The Self() method returns *AgentSelf which has Config as map[string]interface{}
	if self.Config == nil {
		return "", "", fmt.Errorf("config section not found in agent self response")
	}

	// Extract datacenter (this is the cluster name in Nomad)
	datacenterVal, ok := self.Config["Datacenter"]
	if !ok {
		return "", "", fmt.Errorf("datacenter not found in config")
	}
	datacenter, ok := datacenterVal.(string)
	if !ok || datacenter == "" {
		return "", "", fmt.Errorf("datacenter is not a valid string")
	}

	// Extract region
	region := "global" // Default Nomad region
	if regionVal, ok := self.Config["Region"]; ok {
		if regionStr, ok := regionVal.(string); ok && regionStr != "" {
			region = regionStr
		}
	}

	return datacenter, region, nil
}

// ListNodes returns all nodes in the specified cluster
func (r *nomadRepository) ListNodes(ctx context.Context, clusterName string) ([]model.Node, error) {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found", clusterName)
	}

	nodes, _, err := clusterMeta.client.Nodes().List(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	result := make([]model.Node, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, model.Node{
			ID:                    n.ID,
			Name:                  n.Name,
			Drain:                 n.Drain,
			SchedulingEligibility: n.SchedulingEligibility,
			Status:                n.Status,
		})
	}

	r.logger.Info("listed nodes",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
		slog.Int("count", len(result)),
	)

	return result, nil
}

// SetNodeDrain sets the drain status for a specific node
// First tries via Server API, falls back to direct Client API if server is unavailable
func (r *nomadRepository) SetNodeDrain(ctx context.Context, clusterName, nodeID string, drain bool) error {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	var drainSpec *nomad.DrainSpec
	if drain {
		drainSpec = &nomad.DrainSpec{
			Deadline: -1, // Infinite deadline
		}
	}

	// markEligible is the opposite of drain
	// When enabling drain (drain=true), node becomes ineligible (markEligible=false)
	// When disabling drain (drain=false), node becomes eligible (markEligible=true)
	markEligible := !drain

	// Try via Server API first
	_, err := clusterMeta.client.Nodes().UpdateDrain(nodeID, drainSpec, markEligible, nil)
	if err == nil {
		r.logger.Info("updated node drain status via Server API",
			slog.String("cluster", clusterName),
			slog.String("region", clusterMeta.region),
			slog.String("node_id", nodeID),
			slog.Bool("drain", drain),
		)
		return nil
	}

	// Server API failed - try direct Client API fallback
	r.logger.Warn("Server API failed, attempting direct Client API fallback",
		slog.String("cluster", clusterName),
		slog.String("node_id", nodeID),
		slog.String("server_error", err.Error()),
	)

	fallbackErr := r.setNodeDrainDirect(ctx, clusterMeta, nodeID, drain, markEligible)
	if fallbackErr != nil {
		return fmt.Errorf("both Server API and Client API failed: server_error=%w, client_error=%v", err, fallbackErr)
	}

	r.logger.Info("updated node drain status via direct Client API fallback",
		slog.String("cluster", clusterName),
		slog.String("node_id", nodeID),
		slog.Bool("drain", drain),
	)

	return nil
}

// setNodeDrainDirect sets drain status by making direct HTTP request to Nomad Client API
func (r *nomadRepository) setNodeDrainDirect(ctx context.Context, meta *clusterMetadata, nodeID string, drain bool, markEligible bool) error {
	// Get cached node address
	nodeInfo, ok := meta.nodeCache[nodeID]
	if !ok {
		return fmt.Errorf("node %s not found in cache", nodeID)
	}

	if nodeInfo.HTTPAddr == "" {
		return fmt.Errorf("node %s has no cached HTTP address", nodeID)
	}

	// Build drain request payload
	var drainSpec *nomad.DrainSpec
	if drain {
		drainSpec = &nomad.DrainSpec{
			Deadline: -1, // Infinite deadline
		}
	}

	payload := map[string]interface{}{
		"DrainSpec":    drainSpec,
		"MarkEligible": markEligible,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal drain request: %w", err)
	}

	// Determine protocol (http or https based on server address)
	protocol := "http"
	if strings.HasPrefix(meta.client.Address(), "https://") {
		protocol = "https"
	}

	// Build direct client API URL
	// Using /v1/node/self/drain since we're making request directly to the node
	url := fmt.Sprintf("%s://%s/v1/node/self/drain", protocol, nodeInfo.HTTPAddr)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Make request using the HTTP client with TLS config
	resp, err := meta.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("client API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	r.logger.Info("direct client API drain request succeeded",
		slog.String("cluster", meta.name),
		slog.String("node_id", nodeID),
		slog.String("node_name", nodeInfo.Name),
		slog.String("url", url),
	)

	return nil
}

// CheckLeader checks if the cluster has an elected leader
func (r *nomadRepository) CheckLeader(ctx context.Context, clusterName string) (bool, error) {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return false, fmt.Errorf("cluster %s not found", clusterName)
	}

	// Get leader from Nomad Status API
	status := clusterMeta.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return false, fmt.Errorf("failed to get leader: %w", err)
	}

	hasLeader := leader != ""

	r.logger.Debug("checked cluster leader",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
		slog.Bool("has_leader", hasLeader),
		slog.String("leader", leader),
	)

	return hasLeader, nil
}

// GetClusterNames returns the list of all configured cluster names (sorted alphabetically)
func (r *nomadRepository) GetClusterNames() []string {
	names := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetClusterRegion returns the region for a specific cluster
func (r *nomadRepository) GetClusterRegion(clusterName string) (string, error) {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return "", fmt.Errorf("cluster %s not found", clusterName)
	}
	return clusterMeta.region, nil
}

// GetClustersByRegion returns all cluster names in a specific region (sorted alphabetically)
func (r *nomadRepository) GetClustersByRegion(region string) []string {
	var clusters []string
	for _, meta := range r.clusters {
		if meta.region == region {
			clusters = append(clusters, meta.name)
		}
	}
	sort.Strings(clusters)
	return clusters
}

// GetAllRegions returns the list of all unique regions (sorted alphabetically)
func (r *nomadRepository) GetAllRegions() []string {
	regionMap := make(map[string]bool)
	for _, meta := range r.clusters {
		regionMap[meta.region] = true
	}

	regions := make([]string, 0, len(regionMap))
	for region := range regionMap {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	return regions
}

// TriggerJobEvaluations triggers evaluations for all jobs in the cluster
// This forces Nomad scheduler to re-evaluate job placements, which is useful
// after un-draining nodes to redistribute allocations
func (r *nomadRepository) TriggerJobEvaluations(ctx context.Context, clusterName string) error {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	r.logger.Info("triggering job evaluations",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
	)

	// List all jobs in the cluster
	jobs, _, err := clusterMeta.client.Jobs().List(nil)
	if err != nil {
		return fmt.Errorf("failed to list jobs: %w", err)
	}

	r.logger.Info("found jobs to evaluate",
		slog.String("cluster", clusterName),
		slog.Int("job_count", len(jobs)),
	)

	// Track evaluation results
	successCount := 0
	errorCount := 0
	var errors []string

	// Trigger evaluation for each job
	for _, job := range jobs {
		// Skip jobs that are stopped/dead
		if job.Status == "dead" {
			r.logger.Debug("skipping dead job",
				slog.String("cluster", clusterName),
				slog.String("job_id", job.ID),
				slog.String("status", job.Status),
			)
			continue
		}

		// Force evaluation for this job
		evalID, _, err := clusterMeta.client.Jobs().ForceEvaluate(job.ID, nil)
		if err != nil {
			errorCount++
			errMsg := fmt.Sprintf("job %s: %v", job.ID, err)
			errors = append(errors, errMsg)
			r.logger.Warn("failed to trigger evaluation for job",
				slog.String("cluster", clusterName),
				slog.String("job_id", job.ID),
				slog.String("error", err.Error()),
			)
			continue
		}

		successCount++
		r.logger.Debug("triggered evaluation for job",
			slog.String("cluster", clusterName),
			slog.String("job_id", job.ID),
			slog.String("eval_id", evalID),
		)
	}

	r.logger.Info("job evaluations triggered",
		slog.String("cluster", clusterName),
		slog.Int("total_jobs", len(jobs)),
		slog.Int("success", successCount),
		slog.Int("errors", errorCount),
	)

	// Return error if all evaluations failed
	if errorCount > 0 && successCount == 0 {
		return fmt.Errorf("all job evaluations failed: %v", errors)
	}

	// Log warnings if some failed but continue
	if errorCount > 0 {
		r.logger.Warn("some job evaluations failed",
			slog.String("cluster", clusterName),
			slog.Int("error_count", errorCount),
			slog.Any("errors", errors),
		)
	}

	return nil
}

// ListJobs returns all jobs in the specified cluster
func (r *nomadRepository) ListJobs(ctx context.Context, clusterName string) ([]model.Job, error) {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return nil, fmt.Errorf("cluster %s not found", clusterName)
	}

	// List all jobs
	jobs, _, err := clusterMeta.client.Jobs().List(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	result := make([]model.Job, 0, len(jobs))
	for _, j := range jobs {
		// Get job summary for allocation counts
		summary, _, err := clusterMeta.client.Jobs().Summary(j.ID, nil)
		if err != nil {
			r.logger.Warn("failed to get job summary, using basic info",
				slog.String("cluster", clusterName),
				slog.String("job_id", j.ID),
				slog.String("error", err.Error()),
			)
			// Continue with basic info if summary fails
			result = append(result, model.Job{
				ID:          j.ID,
				Name:        j.Name,
				Type:        j.Type,
				Status:      j.Status,
				Priority:    j.Priority,
				SubmitTime:  j.SubmitTime,
				Datacenters: j.Datacenters,
			})
			continue
		}

		// Calculate total allocations across all task groups
		var running, desired, failed int
		if summary != nil && summary.Summary != nil {
			for _, tg := range summary.Summary {
				running += tg.Running
				desired += tg.Queued + tg.Starting + tg.Running
				failed += tg.Failed + tg.Lost
			}
		}

		result = append(result, model.Job{
			ID:          j.ID,
			Name:        j.Name,
			Type:        j.Type,
			Status:      j.Status,
			Running:     running,
			Desired:     desired,
			Failed:      failed,
			Priority:    j.Priority,
			SubmitTime:  j.SubmitTime,
			Datacenters: j.Datacenters,
		})
	}

	r.logger.Info("listed jobs",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
		slog.Int("count", len(result)),
	)

	return result, nil
}

// StartJob starts (registers) a stopped job
func (r *nomadRepository) StartJob(ctx context.Context, clusterName, jobID string) error {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	// Get the job definition first
	job, _, err := clusterMeta.client.Jobs().Info(jobID, nil)
	if err != nil {
		return fmt.Errorf("failed to get job info: %w", err)
	}

	// Set Stop to false to start the job
	stop := false
	job.Stop = &stop

	// Register the job (this will start it)
	_, _, err = clusterMeta.client.Jobs().Register(job, nil)
	if err != nil {
		return fmt.Errorf("failed to start job: %w", err)
	}

	r.logger.Info("started job",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
		slog.String("job_id", jobID),
	)

	return nil
}

// StopJob stops (deregisters) a running job
func (r *nomadRepository) StopJob(ctx context.Context, clusterName, jobID string) error {
	clusterMeta, ok := r.clusters[clusterName]
	if !ok {
		return fmt.Errorf("cluster %s not found", clusterName)
	}

	// Deregister the job (purge=false keeps it in the system)
	_, _, err := clusterMeta.client.Jobs().Deregister(jobID, false, nil)
	if err != nil {
		return fmt.Errorf("failed to stop job: %w", err)
	}

	r.logger.Info("stopped job",
		slog.String("cluster", clusterName),
		slog.String("region", clusterMeta.region),
		slog.String("job_id", jobID),
	)

	return nil
}
