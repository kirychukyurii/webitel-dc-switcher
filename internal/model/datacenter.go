package model

// Datacenter represents a Nomad datacenter with its status and node statistics
type Datacenter struct {
	Name          string `json:"name"`
	Region        string `json:"region"`
	Status        string `json:"status"` // active | draining | error
	NodesTotal    int    `json:"nodes_total"`
	NodesReady    int    `json:"nodes_ready"`
	NodesDraining int    `json:"nodes_draining"`
	JobsTotal     int    `json:"jobs_total"`
	JobsRunning   int    `json:"jobs_running"`
	JobsStopped   int    `json:"jobs_stopped"`
	HeartbeatAge  int64  `json:"heartbeat_age"` // Age of heartbeat in milliseconds (0 if no heartbeat)
	IsMyDC        bool   `json:"is_my_dc"`      // Whether this is the datacenter managed by this switcher instance
}

// DatacenterStatus represents possible datacenter states
const (
	DatacenterStatusActive   = "active"
	DatacenterStatusDraining = "draining"
	DatacenterStatusError    = "error"
)

// Region represents a Nomad region with its datacenters
type Region struct {
	Name        string       `json:"name"`
	Datacenters []Datacenter `json:"datacenters"`
	Status      string       `json:"status"` // active | partial | draining | error
	JobsTotal   int          `json:"jobs_total"`
	JobsRunning int          `json:"jobs_running"`
	JobsStopped int          `json:"jobs_stopped"`
}
