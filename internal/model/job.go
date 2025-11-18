package model

// Job represents a Nomad job with its status
type Job struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`        // service | batch | system
	Status      string   `json:"status"`      // running | pending | dead
	Running     int      `json:"running"`     // number of running allocations
	Desired     int      `json:"desired"`     // desired number of allocations
	Failed      int      `json:"failed"`      // number of failed allocations
	SubmitTime  int64    `json:"submit_time"` // unix timestamp
	Priority    int      `json:"priority"`    // job priority
	Datacenters []string `json:"datacenters"` // list of datacenters job is targeting
}

// JobAction represents an action to perform on a job
type JobAction struct {
	Action string `json:"action"` // start | stop
}

// JobActionResult represents the result of a job action
type JobActionResult struct {
	JobID   string   `json:"job_id"`
	Action  string   `json:"action"`
	Success bool     `json:"success"`
	Errors  []string `json:"errors,omitempty"`
}
