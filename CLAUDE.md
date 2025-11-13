# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Webitel DC Switcher is a service for managing multiple Nomad clusters across datacenters and regions. It provides centralized datacenter/region switching by controlling node drain status via Nomad HTTP API.

## Development Commands

### Build and Run
```bash
make build          # Build backend binary to bin/dc-switcher
make build-all      # Build UI + backend (recommended)
make run            # Run with config.yaml
make clean          # Remove Go build artifacts
make clean-all      # Remove all artifacts including UI
```

### UI Development
```bash
make ui-deps        # Install Node dependencies
make ui-build       # Build UI (creates dist/ folder)
make ui-dev         # Run UI dev server on port 3000
```

### Development
```bash
make deps           # Download dependencies
make tidy           # Tidy go modules
make fmt            # Format code
make vet            # Run go vet
make lint           # Run golangci-lint (must be installed)
make test           # Run tests with race detector
make check          # Run fmt + vet + lint + test
```

### Running the Service
```bash
./bin/dc-switcher -config config.yaml
```

## Web UI

The service includes an embedded web UI built with Vue 3:

### Frontend Stack
- **Vue 3** with Composition API
- **Vite** build tool
- **Vue Router** for SPA navigation
- **Pinia** for state management
- **Axios** for HTTP requests
- **@webitel/ui-sdk** (partially integrated for styling)

### UI Structure
- Located in `ui/` directory
- `ui/src/views/` - Page components (RegionsView, DatacentersView)
- `ui/src/api/` - API client for backend communication
- `ui/dist/` - Built files (embedded in Go binary)
- `ui/embed.go` - Go embed directive

### Integration with Backend
- UI is embedded using Go's `//go:embed` directive
- Served on same port as API (default: 8080)
- API routes: `/api/*`
- UI routes: `/*` (catch-all for SPA)
- During development: Vite dev server on port 3000 with API proxy

### Key Features
- **Regions View**: Manage regions with nested datacenter cards
- **Datacenters View**: Detailed DC view with expandable node lists
- **Status Indicators**: Color-coded badges (green=active, orange=partial, red=draining/error)
- **Activation Controls**: One-click buttons with confirmation
- **Real-time Updates**: Manual refresh (auto-refresh not implemented)

### Development Workflow
1. Run backend: `make run` (serves API on :8080)
2. Run UI dev server: `make ui-dev` (Vite on :3000, proxies /api to :8080)
3. Make changes to `ui/src/`
4. Build for production: `make build-all`

## Architecture

The codebase follows a **3-layer clean architecture**:

### 1. Transport Layer (internal/api/)
- HTTP handlers using `chi` router
- Endpoint patterns:
  - Datacenters: `/api/datacenters`, `/api/datacenters/{name}/...`
  - Regions: `/api/regions`, `/api/regions/{name}/...`
  - UI: `/*` (catch-all, serves embedded Vue SPA)
- JSON responses with structured error handling
- Middleware: request logging, recovery, request ID
- UI serving: `ui_handler.go` serves embedded filesystem from `ui/dist`

### 2. Service Layer (internal/service/)
- Business logic orchestration
- **Datacenter activation algorithm** (critical):
  1. Validates target datacenter exists
  2. For ALL clusters: fetch current node states
  3. For non-target clusters: set all nodes to `drain=true`
  4. For target cluster: set all nodes to `drain=false`
  5. **Fail-fast with rollback**: On ANY error, automatically reverts all changes made in current operation
  6. Returns statistics: drained_nodes, un_drained_nodes
- **Region activation algorithm**: Similar to datacenter activation, but operates on all datacenters within a region
  - Drains all nodes in datacenters NOT in target region
  - Un-drains all nodes in datacenters IN target region
- Implements in-memory caching with TTL (configured per-cluster node lists)
- Cache invalidation on drain state changes

### 3. Repository Layer (internal/repository/)
- Wraps Nomad API client (`github.com/hashicorp/nomad/api`)
- Stores cluster metadata including region information
- Key operations:
  - `ListNodes()`: GET /v1/nodes
  - `SetNodeDrain()`: Uses `client.Nodes().UpdateDrain(nodeID, drainSpec, markEligible, nil)`
  - `GetClusterRegion()`: Returns region for a cluster
  - `GetClustersByRegion()`: Returns all clusters in a specific region
  - `GetAllRegions()`: Returns list of unique regions
- Supports per-cluster TLS client certificates
- Sets Nomad region in client config per cluster

## Key Components

### Configuration (internal/config/)
- Uses `koanf` for YAML parsing
- Structure: `server` + `cache` + `clusters[]` with optional name, region, and TLS
- Validation on load (required field: cluster address only)
- Name and region are **auto-detected** from Nomad API if not specified
- Manual values override auto-detection

### Caching (internal/cache/)
- Uses `github.com/patrickmn/go-cache`
- Keys: `{clusterName}:nodes`
- Configurable TTL (typically 30s)
- Force-invalidate on drain updates

### TLS (internal/util/)
- Loads client certificates for mTLS to Nomad API
- Requires: CA cert, client cert, client key (all file paths)

### HTTP Server (pkg/httpserver/)
- Graceful shutdown with 30s timeout
- Listens for SIGINT/SIGTERM
- Configurable read/write timeouts

### Models (internal/model/)
- `Datacenter`: name, region, status (active/draining/error), node counts
  - NodesReady counts nodes that are NOT draining AND eligible
  - NodesDraining counts nodes that are draining OR ineligible
- `Region`: name, status (active/partial/draining/error), list of datacenters
- `Node`: id, name, drain, scheduling_eligibility, status
  - `scheduling_eligibility`: "eligible" or "ineligible"
  - `IsReady()`: returns true if node can accept allocations (not drain + eligible)
- `ActivationResult`: activated (name), statistics + errors + rollback_applied flag

### Logging (internal/logger/)
- Uses Go's standard `log/slog` (structured JSON logging)
- Log levels: Info (default), Error

## Common Patterns

### Adding a New Endpoint
1. Define handler in `internal/api/datacenter_handler.go`, `internal/api/region_handler.go`, or new handler file
2. Register route in `internal/api/handler.go` Router() method
3. Call service layer method
4. Return JSON via `h.respondJSON()` or error via `h.respondError()`

### Adding Nomad API Operations
1. Add method to `NomadRepository` interface in `internal/repository/nomad_client.go`
2. Implement using `client.Nodes()` or other Nomad API clients
3. Update service layer to use new repository method

### Modifying Activation Logic
- Core logic in `datacenterService.ActivateDatacenter()` (internal/service/datacenter_service.go:123)
- **Critical**: Maintain fail-fast behavior by checking errors immediately
- **Critical**: Track all changes in `[]nodeChange` slice for rollback
- Call `s.rollbackChanges()` on any failure before returning error

### Error Handling Philosophy
- **Fail-fast**: Stop immediately on first error
- **Rollback**: Revert all changes made before failure
- Return detailed error information to API client
- Log all errors with structured context (cluster, node_id, operation)

## Dependencies

Key external packages:
- `github.com/hashicorp/nomad/api` - Nomad API client
- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/knadh/koanf/v2` - Configuration
- `github.com/patrickmn/go-cache` - In-memory cache
- `log/slog` - Structured logging (stdlib)

## Testing

When adding tests:
- Place in `*_test.go` files alongside implementation
- Use table-driven tests
- Mock repository layer for service tests
- Mock service layer for API tests

## Configuration Notes

- `config.yaml` is gitignored (contains secrets)
- Use `config.yaml.example` as template
- **Required**: Only `address` is required per cluster
- **Optional**: `name`, `region`, `tls`, `skip_unhealthy_clusters`, and `base_path`
- **Base Path**: Optional `server.base_path` for hosting behind reverse proxy (e.g., `/dc-switcher`)
  - When set, UI is available at `http://host{base_path}/` and API at `http://host{base_path}/api/`
  - Both Vue Router and Axios automatically use this base path via `window._BASE_PATH`
  - Injected dynamically into index.html on each request
  - See `nginx.conf.example` for reverse proxy configuration
- **Health Checks**: Each cluster is checked during initialization
  - Verifies leader election (`/v1/status/leader`)
  - Checks agent health (`/v1/agent/health`)
  - If `skip_unhealthy_clusters: false` (default): fails startup on unhealthy clusters
  - If `skip_unhealthy_clusters: true`: skips unhealthy clusters and continues
- **Auto-detection**: Name (datacenter) and region are automatically detected from Nomad API
  - Uses `/v1/agent/self` endpoint
  - Falls back to `cluster-{index}` for name and `global` for region if detection fails
  - Manual configuration overrides auto-detection
- TLS is optional per-cluster (omit `tls` section for HTTP)
- Multiple clusters with same Nomad address are allowed (different names)
- Clusters are grouped by region for region-level operations

## Nomad API Specifics

- API calls use `nil` for QueryOptions/WriteOptions (context not passed via these)
- `UpdateDrain()` signature: `(nodeID, *DrainSpec, markEligible bool, *WriteOptions)`
- `DrainSpec.Deadline = -1` means infinite drain deadline
- `markEligible = true` when un-draining (makes node eligible for scheduling)
- `markEligible = false` when draining (makes node ineligible)
- Region is set in Nomad client config (`nomadConfig.Region`)
- Each cluster can target a different Nomad region

## Node Eligibility

Nomad has two separate controls for node scheduling:
1. **Drain**: Controls whether existing allocations should be migrated off the node
2. **Eligibility**: Controls whether new allocations can be placed on the node

**Combined states:**
- `drain=false, eligible=true` → Node is **ready** (accepts new allocations)
- `drain=true, eligible=false` → Node is **draining** (existing jobs migrate, no new jobs)
- `drain=false, eligible=false` → Node is **ineligible** (keeps existing jobs, no new jobs)
- `drain=true, eligible=true` → Invalid state (not used)

**Service behavior:**
- When activating a datacenter: sets `drain=false, eligible=true` on nodes
- When draining a datacenter: sets `drain=true, eligible=false` on nodes
- Datacenter status considers both: only nodes with `eligible=true AND drain=false` count as ready

## API Endpoints

### Datacenter Endpoints
- `GET /api/datacenters` - List all datacenters with status
- `GET /api/datacenters/{name}/nodes` - Get nodes for a datacenter
- `POST /api/datacenters/{name}/activate` - Activate datacenter (drain all others)

### Region Endpoints
- `GET /api/regions` - List all regions with their datacenters
- `GET /api/regions/{name}/datacenters` - Get all datacenters in a region
- `POST /api/regions/{name}/activate` - Activate region (drain all other regions)

## Region Support

Regions provide a hierarchical grouping above datacenters:
- Each cluster/datacenter belongs to one region
- Region activation drains ALL datacenters outside the target region
- Region status is computed from datacenter statuses:
  - `active`: All DCs active
  - `draining`: All DCs draining
  - `partial`: Mixed state
  - `error`: At least one DC has errors

## Auto-Detection

The service automatically discovers cluster metadata from Nomad:

### How It Works
1. During initialization, for each cluster without explicit `name` or `region`
2. Queries Nomad API: `GET /v1/agent/self`
3. Extracts from response:
   - `config.Datacenter` → used as cluster name
   - `config.Region` → used as region
4. Falls back to defaults if API call fails:
   - Name: `cluster-{index}` (e.g., `cluster-0`, `cluster-1`)
   - Region: `global`

### Benefits
- Reduces configuration errors
- Automatically adapts to Nomad cluster changes
- Simplifies multi-cluster setup
- Still allows manual override when needed

### Implementation
- `detectClusterInfo()` in `internal/repository/nomad_client.go`
- Called during `NewNomadRepository()` initialization
- Logs detected values at INFO level

## Health Checks

The service performs comprehensive health checks during initialization:

### How It Works
1. For each configured cluster
2. Checks leader election: `GET /v1/status/leader`
   - Ensures cluster has an elected leader
3. Checks agent health: `GET /v1/agent/health`
   - Verifies both client and server are OK
4. On failure:
   - If `skip_unhealthy_clusters: false` → fails startup with error
   - If `skip_unhealthy_clusters: true` → logs warning and continues

### Benefits
- Early detection of connectivity issues
- Prevents adding unreachable clusters
- Clear error messages at startup
- Option to continue with partial cluster set

### Implementation
- `checkClusterHealth()` in `internal/repository/nomad_client.go`
- Called before adding cluster to repository
- Logs health status for each cluster
