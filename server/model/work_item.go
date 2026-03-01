package model

// WorkItemStatus represents the state of a work item in the queue.
type WorkItemStatus string

const (
	WorkItemStatusPending WorkItemStatus = "pending"
	WorkItemStatusRunning WorkItemStatus = "running"
	WorkItemStatusFailed  WorkItemStatus = "failed"
)

// WorkItem represents a single unit of work in the persistent queue.
// Successful items are deleted from the KV store; only pending, running,
// and failed items are persisted.
type WorkItem struct {
	ID          string         `json:"id"`
	FlowID      string         `json:"flow_id"`
	FlowName    string         `json:"flow_name"`
	TriggerData TriggerData    `json:"trigger_data"`
	Status      WorkItemStatus `json:"status"`
	CreatedAt   int64          `json:"created_at"`
	StartedAt   int64          `json:"started_at,omitempty"`
	Error       string         `json:"error,omitempty"`
	RetryCount  int            `json:"retry_count"`
}
