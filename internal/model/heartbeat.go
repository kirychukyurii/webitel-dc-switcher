package model

import "time"

// ActiveDatacenter represents the currently active datacenter information stored in etcd
type ActiveDatacenter struct {
	Datacenter    string    `json:"datacenter"`
	ActivatedAt   time.Time `json:"activated_at"`
	ActivatedBy   string    `json:"activated_by"` // "api", "startup", "recovery", etc.
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

// HeartbeatInfo represents heartbeat information for a specific datacenter
type HeartbeatInfo struct {
	Datacenter string    `json:"datacenter"`
	LastSeen   time.Time `json:"last_seen"`
}

// IsStale checks if the heartbeat is older than the given threshold
func (a *ActiveDatacenter) IsStale(threshold time.Duration) bool {
	return time.Since(a.LastHeartbeat) > threshold
}

// HeartbeatAge returns the age of the heartbeat
func (a *ActiveDatacenter) HeartbeatAge() time.Duration {
	return time.Since(a.LastHeartbeat)
}
