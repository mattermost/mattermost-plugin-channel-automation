package model

// ExecutionRecord captures the result of a completed flow execution.
type ExecutionRecord struct {
	ID          string                `json:"id"`
	FlowID      string                `json:"flow_id"`
	FlowName    string                `json:"flow_name"`
	Status      string                `json:"status"`
	Error       string                `json:"error,omitempty"`
	Steps       map[string]StepOutput `json:"steps,omitempty"`
	TriggerData TriggerData           `json:"trigger_data"`
	CreatedAt   int64                 `json:"created_at"`
	StartedAt   int64                 `json:"started_at"`
	CompletedAt int64                 `json:"completed_at"`
}

// ExecutionStore persists execution history records.
type ExecutionStore interface {
	Save(record *ExecutionRecord) error
	Get(id string) (*ExecutionRecord, error)
	ListByFlow(flowID string, limit int) ([]*ExecutionRecord, error)
	ListRecent(limit int) ([]*ExecutionRecord, error)
	PurgeFlow(flowID string) error
}
