package model

// Node represents a Nomad node
type Node struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Drain                 bool   `json:"drain"`
	SchedulingEligibility string `json:"scheduling_eligibility"` // "eligible" or "ineligible"
	Status                string `json:"status"`
}

// IsReady returns true if node can accept new allocations
// A node is ready when it's not draining AND is eligible for scheduling
func (n *Node) IsReady() bool {
	return !n.Drain && n.SchedulingEligibility == "eligible"
}

// ActivationResult represents the result of datacenter activation
type ActivationResult struct {
	Activated      string   `json:"activated"`
	DrainedNodes   int      `json:"drained_nodes"`
	UnDrainedNodes int      `json:"un_drained_nodes"`
	Errors         []string `json:"errors,omitempty"`
}
