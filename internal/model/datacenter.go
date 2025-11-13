package model

// Datacenter represents a Nomad datacenter with its status and node statistics
type Datacenter struct {
	Name          string `json:"name"`
	Region        string `json:"region"`
	Status        string `json:"status"` // active | draining | error
	NodesTotal    int    `json:"nodes_total"`
	NodesReady    int    `json:"nodes_ready"`
	NodesDraining int    `json:"nodes_draining"`
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
}
