package model

import "time"

// ServiceStatus represents the current status of the dc-switcher service
type ServiceStatus struct {
	MyDatacenter      string    `json:"my_datacenter"`      // Name of the datacenter this instance manages
	AmDrained         bool      `json:"am_drained"`         // Whether this instance has drained its nodes
	EtcdConnected     bool      `json:"etcd_connected"`     // Whether connected to etcd
	ActiveDatacenter  string    `json:"active_datacenter"`  // Which datacenter is active according to etcd
	HeartbeatAge      int64     `json:"heartbeat_age"`      // Age of the heartbeat in milliseconds
	LastHeartbeat     time.Time `json:"last_heartbeat"`     // Last heartbeat time
	ActivatedAt       time.Time `json:"activated_at"`       // When the active datacenter was activated
	ActivatedBy       string    `json:"activated_by"`       // Who/what activated the datacenter
	HeartbeatInterval int64     `json:"heartbeat_interval"` // Heartbeat update interval in milliseconds
	StaleThreshold    int64     `json:"stale_threshold"`    // Heartbeat stale threshold in milliseconds
}
