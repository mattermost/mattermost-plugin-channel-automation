package model

// Flow represents a trigger-action workflow.
type Flow struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Trigger   Trigger  `json:"trigger"`
	Actions   []Action `json:"actions"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
	CreatedBy string   `json:"created_by"`
}

// Trigger defines when a flow should fire.
type Trigger struct {
	Type      string `json:"type"`       // "message_posted"
	ChannelID string `json:"channel_id"` // channel to watch
}

// Action defines a single step in a flow.
type Action struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`                        // "send_message", "ai_prompt"
	ChannelID      string         `json:"channel_id"`                  // target channel
	ReplyToPostID  string         `json:"reply_to_post_id,omitempty"`  // post ID to reply to (creates a thread)
	Body           string         `json:"body"`                        // Go text/template string
	Config         map[string]any `json:"config,omitempty"`            // action-type-specific configuration
}
