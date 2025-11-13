# Webitel DC Switcher

DC Switcher is a service for managing multiple Nomad clusters across different datacenters. It provides a centralized HTTP API to switch active datacenters by controlling node drain status across all clusters.

## Features

- **Multi-cluster Management**: Control multiple Nomad clusters from a single service
- **Datacenter Switching**: Activate one datacenter while draining all others
- **TLS Support**: Secure communication with Nomad clusters using client certificates
- **Caching**: In-memory caching with TTL to reduce load on Nomad API
- **Graceful Shutdown**: Clean shutdown handling via context
- **Structured Logging**: JSON-formatted logs using Go's standard `slog` package
- **Fail-fast with Rollback**: Automatic rollback on activation failures

## Architecture

The service follows a clean 3-layer architecture:

- **Transport Layer** (`internal/api`): HTTP handlers and routing
- **Service Layer** (`internal/service`): Business logic and orchestration
- **Repository Layer** (`internal/repository`): Nomad API client operations

## Requirements

- Go 1.22+
- Access to Nomad cluster(s) HTTP API
- (Optional) TLS certificates for Nomad authentication

## Installation

### Build from source

```bash
make build
```

The binary will be created at `bin/dc-switcher`.

### Run directly

```bash
make run
```

## Configuration

Create a `config.yaml` file (see `config.yaml.example`):

```yaml
server:
  addr: ":8080"
  read_timeout: 5s
  write_timeout: 10s

cache:
  ttl: 30s

clusters:
  # Minimal - name and region auto-detected from Nomad API
  - address: https://nomad-dc1.example.com:4646
    tls:
      ca: /etc/nomad/ca.crt
      cert: /etc/nomad/client.crt
      key: /etc/nomad/client.key

  # Explicit configuration (overrides auto-detection)
  - name: dc2
    region: us-east
    address: https://nomad-dc2.example.com:4646
```

### Configuration Options

- `server.addr`: HTTP server listen address
- `server.read_timeout`: HTTP read timeout
- `server.write_timeout`: HTTP write timeout
- `cache.ttl`: Time-to-live for cached node information
- `skip_unhealthy_clusters`: **Optional** (default: `false`) - Health check behavior
  - `false`: Fail startup if any cluster is unhealthy or unreachable
  - `true`: Skip unhealthy clusters and continue with healthy ones
- `clusters`: List of Nomad clusters to manage
  - `address`: **Required** - Nomad API address
  - `name`: **Optional** - Cluster/datacenter name (auto-detected from Nomad API if not specified)
  - `region`: **Optional** - Nomad region (auto-detected from Nomad API if not specified)
  - `tls`: **Optional** - TLS configuration for mTLS
    - `ca`: Path to CA certificate
    - `cert`: Path to client certificate
    - `key`: Path to client private key

**Health Checks**: During initialization, the service verifies each cluster:
- Checks if Nomad leader is elected
- Verifies agent health status
- Validates connectivity

**Auto-detection**: The service automatically queries Nomad API (`/v1/agent/self`) to determine:
- **Datacenter name** (used as cluster name)
- **Region** (Nomad region)

This eliminates manual configuration and reduces errors.

## Web UI

The service includes a built-in web UI for managing datacenters and regions. The UI is embedded in the binary and served on the same port as the HTTP API.

### Features

- **Dashboard View**: Visual overview of all regions and datacenters
- **Real-time Status**: Live status indicators for each datacenter and node
- **One-Click Activation**: Easy activation of datacenters and regions
- **Node Details**: Detailed information about node drain status and scheduling eligibility
- **Responsive Design**: Works on desktop and mobile devices

### Access

Once the service is running, access the web UI at:

```
http://localhost:8080/
```

The UI provides two main views:
- **Regions**: Manage regions and their datacenters
- **Datacenters**: View individual datacenters and their nodes

## Usage

### Start the service

```bash
./bin/dc-switcher -config config.yaml
```

### API Endpoints

#### List Datacenters

Get status of all configured datacenters.

```bash
GET /api/datacenters
```

**Response:**

```json
[
  {
    "name": "dc1",
    "region": "us-east",
    "status": "active",
    "nodes_total": 12,
    "nodes_ready": 11,
    "nodes_draining": 1
  },
  {
    "name": "dc2",
    "region": "us-east",
    "status": "draining",
    "nodes_total": 10,
    "nodes_ready": 0,
    "nodes_draining": 10
  }
]
```

**Status values:**
- `active`: At least one node is not draining
- `draining`: All nodes are draining
- `error`: Cluster is unreachable

#### Get Nodes

Get all nodes for a specific datacenter.

```bash
GET /api/datacenters/{name}/nodes
```

**Response:**

```json
[
  {
    "id": "node-1-id",
    "name": "node-1",
    "drain": false,
    "scheduling_eligibility": "eligible",
    "status": "ready"
  },
  {
    "id": "node-2-id",
    "name": "node-2",
    "drain": true,
    "scheduling_eligibility": "ineligible",
    "status": "ready"
  }
]
```

**Node fields:**
- `drain`: Whether the node is draining allocations
- `scheduling_eligibility`: Can be `"eligible"` or `"ineligible"`
- A node is considered **ready** only when `drain=false` AND `scheduling_eligibility="eligible"`

#### Activate Datacenter

Activate a specific datacenter and drain all others.

```bash
POST /api/datacenters/{name}/activate
```

**Response (Success):**

```json
{
  "activated": "dc2",
  "drained_nodes": 24,
  "un_drained_nodes": 8
}
```

**Response (Failure with Rollback):**

```json
{
  "activated": "dc2",
  "drained_nodes": 0,
  "un_drained_nodes": 0,
  "errors": ["activation failed: failed to set node drain: ...; rollback successful"],
  "rollback_applied": true
}
```

#### List Regions

Get status of all regions with their datacenters.

```bash
GET /api/regions
```

**Response:**

```json
[
  {
    "name": "us-east",
    "status": "active",
    "datacenters": [
      {
        "name": "dc1",
        "region": "us-east",
        "status": "active",
        "nodes_total": 12,
        "nodes_ready": 12,
        "nodes_draining": 0
      },
      {
        "name": "dc2",
        "region": "us-east",
        "status": "active",
        "nodes_total": 8,
        "nodes_ready": 8,
        "nodes_draining": 0
      }
    ]
  },
  {
    "name": "eu-west",
    "status": "draining",
    "datacenters": [...]
  }
]
```

**Region status values:**
- `active`: All datacenters are active
- `draining`: All datacenters are draining
- `partial`: Some datacenters active, some draining
- `error`: At least one datacenter has errors

#### Get Datacenters by Region

Get all datacenters in a specific region.

```bash
GET /api/regions/{name}/datacenters
```

#### Activate Region

Activate all datacenters in a specific region and drain all others.

```bash
POST /api/regions/{name}/activate
```

**Response:** Same format as datacenter activation.

### Example Usage

```bash
# List all datacenters
curl http://localhost:8080/api/datacenters

# List all regions
curl http://localhost:8080/api/regions

# Get datacenters in us-east region
curl http://localhost:8080/api/regions/us-east/datacenters

# Get nodes for dc1
curl http://localhost:8080/api/datacenters/dc1/nodes

# Activate dc2 (drain all other datacenters)
curl -X POST http://localhost:8080/api/datacenters/dc2/activate

# Activate us-east region (drain all other regions)
curl -X POST http://localhost:8080/api/regions/us-east/activate
```

## Error Handling

The service implements a **fail-fast with rollback** strategy:

1. When activating a datacenter, if any node update fails, the operation stops immediately
2. All previously applied changes are automatically rolled back
3. The system returns to its state before the activation attempt
4. Detailed error information is returned in the response

## Development

### Available Make Commands

```bash
make help          # Show all available commands
make deps          # Download Go dependencies
make build         # Build the backend application
make run           # Run the application
make test          # Run tests
make lint          # Run linter
make fmt           # Format code
make vet           # Run go vet
make check         # Run all checks (fmt, vet, lint, test)
make clean         # Clean Go build artifacts
make clean-all     # Clean all artifacts including UI

# UI commands
make ui-deps       # Install UI dependencies
make ui-build      # Build UI (creates dist/ folder)
make ui-dev        # Run UI in development mode (port 3000)
make build-all     # Build both UI and backend
```

### Building with UI

To build the complete application with embedded UI:

```bash
# Build everything (UI + backend)
make build-all

# The binary will include the embedded UI
./bin/dc-switcher -config config.yaml
```

The UI is automatically embedded in the binary using Go's `embed` package. The `ui/dist` folder is included at compile time.

### Project Structure

```
dc-switcher/
├── cmd/
│   └── dc-switcher/        # Application entry point
├── internal/
│   ├── api/                # HTTP handlers and routing
│   ├── service/            # Business logic
│   ├── repository/         # Nomad API client
│   ├── config/             # Configuration management
│   ├── cache/              # Caching implementation
│   ├── model/              # Data models
│   ├── logger/             # Logger setup
│   └── util/               # Utilities (TLS, etc.)
├── pkg/
│   └── httpserver/         # HTTP server with graceful shutdown
├── ui/                     # Web UI (Vue 3)
│   ├── src/                # UI source code
│   │   ├── api/            # API client
│   │   ├── views/          # Page views
│   │   ├── App.vue         # Root component
│   │   └── main.js         # Entry point
│   ├── dist/               # Built UI files (embedded)
│   ├── embed.go            # Go embed directive
│   ├── package.json        # Node dependencies
│   └── vite.config.js      # Vite configuration
├── config.yaml.example     # Example configuration
├── Makefile               # Build automation
└── README.md              # This file
```

## Reverse Proxy Configuration

DC Switcher supports deployment behind a reverse proxy (e.g., Nginx) with custom base path.

### Configuration

In `config.yaml`, set the base path:

```yaml
server:
  addr: ":4647"
  base_path: "/dc-switcher"  # Optional: leave empty or omit for root path
```

### Nginx Example

See `nginx.conf.example` for complete examples. Basic configuration:

```nginx
# Host at custom path /dc-switcher
server {
    listen 80;
    server_name example.com;

    location /dc-switcher/ {
        proxy_pass http://localhost:4647/dc-switcher/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

After configuration:
- **UI**: `http://example.com/dc-switcher/`
- **API**: `http://example.com/dc-switcher/api/`

The UI and API client automatically adjust to the configured base path.

## License

[Your License Here]

## Contributing

Contributions are welcome! Please submit pull requests or open issues for bugs and feature requests.
