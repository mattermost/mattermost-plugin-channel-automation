package model

import "encoding/json"

// GuardrailChannel is the runtime representation of one allowed channel and
// the team it belongs to. The TeamID is never serialized — it is resolved on
// demand by the hooks layer (channel -> team is immutable in Mattermost, so a
// process-wide cache keeps lookups cheap).
type GuardrailChannel struct {
	ChannelID string `json:"-"`
	TeamID    string `json:"-"`
}

// Guardrails constrains MCP tool calls for an ai_prompt action (opt-in).
// Additional fields may be added over time without breaking callers.
//
// The JSON wire shape is {"channel_ids": ["..."]}: callers do not need to
// know about teams, and storage round-trips through the same shape. Internal
// code uses Channels and may populate TeamID for fast lookup.
type Guardrails struct {
	Channels []GuardrailChannel `json:"-"`
}

// guardrailsJSON is the on-the-wire shape for Guardrails.
type guardrailsJSON struct {
	ChannelIDs []string `json:"channel_ids,omitempty"`
}

// MarshalJSON emits {"channel_ids": ["..."]}, hiding any resolved team IDs.
func (g Guardrails) MarshalJSON() ([]byte, error) {
	if len(g.Channels) == 0 {
		return json.Marshal(guardrailsJSON{})
	}
	ids := make([]string, 0, len(g.Channels))
	for _, c := range g.Channels {
		ids = append(ids, c.ChannelID)
	}
	return json.Marshal(guardrailsJSON{ChannelIDs: ids})
}

// UnmarshalJSON accepts {"channel_ids": ["..."]} and populates Channels with
// empty TeamIDs. Resolution happens on demand at hook time.
func (g *Guardrails) UnmarshalJSON(data []byte) error {
	var aux guardrailsJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	g.Channels = nil
	if len(aux.ChannelIDs) == 0 {
		return nil
	}
	g.Channels = make([]GuardrailChannel, 0, len(aux.ChannelIDs))
	for _, id := range aux.ChannelIDs {
		g.Channels = append(g.Channels, GuardrailChannel{ChannelID: id})
	}
	return nil
}
