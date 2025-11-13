# DC Switcher UI

Web UI for managing Nomad datacenters and regions built with Vue 3 and Vite.

## Features

- **Regions View**: View and manage all regions with their datacenters
- **Datacenters View**: Detailed view of all datacenters with node information
- **Node Details**: Expandable node lists showing drain status and scheduling eligibility
- **Activation Controls**: One-click activation for regions and datacenters
- **Real-time Status**: Live status indicators (active, draining, partial, error)

## Tech Stack

- **Vue 3** - Progressive JavaScript framework
- **Vite** - Next generation frontend tooling
- **Pinia** - State management
- **Vue Router** - Official router for Vue.js
- **Axios** - HTTP client
- **@webitel/ui-sdk** - Webitel UI component library

## Development

### Prerequisites

- Node.js 22+ (recommended)
- npm 10+

### Install Dependencies

```bash
npm install
```

### Development Server

Run the development server with hot-reload:

```bash
npm run dev
```

The UI will be available at http://localhost:3000 with API proxy to http://localhost:8080

### Build for Production

```bash
npm run build
```

The built files will be in the `dist/` directory, which gets embedded into the Go binary.

### Preview Production Build

```bash
npm run preview
```

## Project Structure

```
ui/
├── src/
│   ├── api/           # API client
│   ├── components/    # Vue components
│   ├── views/         # Page views
│   ├── App.vue        # Root component
│   ├── main.js        # Application entry point
│   └── router.js      # Router configuration
├── dist/              # Built files (embedded in Go binary)
├── index.html         # HTML template
├── vite.config.js     # Vite configuration
├── package.json       # Dependencies
└── embed.go           # Go embed directive

## API Integration

The UI communicates with the backend API at `/api`:

- `GET /api/regions` - List all regions
- `GET /api/regions/{name}/datacenters` - Get datacenters by region
- `POST /api/regions/{name}/activate` - Activate region
- `GET /api/datacenters` - List all datacenters
- `GET /api/datacenters/{name}/nodes` - Get nodes for datacenter
- `POST /api/datacenters/{name}/activate` - Activate datacenter

## Styling

The UI uses custom CSS with a clean, modern design inspired by Consul and Nomad interfaces. Status colors:

- **Green** - Active/Ready
- **Orange** - Partial/Warning
- **Red** - Draining/Error
- **Blue** - Primary actions

## Building with Go

The UI is automatically embedded into the Go binary during build:

```bash
# From project root
make ui-build   # Build UI only
make build-all  # Build UI + backend
```

The `embed.go` file uses Go's `//go:embed` directive to include the `dist/` folder in the compiled binary.
