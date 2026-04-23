package hooks

import (
	"errors"
	"fmt"

	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

// ErrToolDiscovery wraps failures to discover the agent's tool catalog
// (bridge unavailable, HTTP error, etc.). These are runtime/dependency
// failures rather than client validation errors, so callers should map
// them to a 5xx response instead of 400.
var ErrToolDiscovery = errors.New("tool discovery failed")

// AgentToolsLister fetches the set of tools an agent exposes for a given user
// from the Mattermost agents bridge. Implemented by *bridgeclient.Client; kept
// as an interface so the validator can be unit-tested without live HTTP.
type AgentToolsLister interface {
	GetAgentTools(agentID, userID string) ([]bridgeclient.BridgeToolInfo, error)
}

// ValidateAllowedTools verifies that every entry in each ai_prompt action's
// allowed_tools is genuinely available to userID via the action's agent, and
// that any embedded Mattermost MCP tool is present in the supported catalog
// with Allowed=true. The bridge is queried once per distinct provider_id.
//
// Returns nil when there are no ai_prompt actions or none have allowed_tools,
// without making any bridge calls.
func ValidateAllowedTools(f *model.Flow, userID string, bridge AgentToolsLister) error {
	if f == nil {
		return nil
	}

	// Cache results per agent so a flow with multiple ai_prompt actions
	// pointing at the same agent only triggers one bridge call.
	cache := make(map[string]map[string]bridgeclient.BridgeToolInfo)

	for i, a := range f.Actions {
		if a.AIPrompt == nil || len(a.AIPrompt.AllowedTools) == 0 {
			continue
		}
		if userID == "" {
			return fmt.Errorf("action %d: cannot validate allowed_tools: missing user id", i)
		}
		agentID := a.AIPrompt.ProviderID
		if agentID == "" {
			return fmt.Errorf("action %d: ai_prompt allowed_tools requires provider_id", i)
		}

		available, ok := cache[agentID]
		if !ok {
			if bridge == nil {
				return errors.Join(fmt.Errorf("action %d: cannot validate allowed_tools: bridge client unavailable", i), ErrToolDiscovery)
			}
			tools, err := bridge.GetAgentTools(agentID, userID)
			if err != nil {
				return errors.Join(fmt.Errorf("action %d: failed to list tools for agent %q: %w", i, agentID, err), ErrToolDiscovery)
			}
			available = make(map[string]bridgeclient.BridgeToolInfo, len(tools))
			for _, t := range tools {
				available[t.Name] = t
			}
			cache[agentID] = available
		}

		for _, name := range a.AIPrompt.AllowedTools {
			info, ok := available[name]
			if !ok {
				return fmt.Errorf("action %d: tool %q is not available to the automation owner for agent %q", i, name, agentID)
			}
			if info.ServerOrigin != EmbeddedMattermostMCPOrigin {
				continue
			}
			known, allowed := IsAllowedMattermostMCPTool(name)
			if !known {
				return fmt.Errorf("action %d: Mattermost MCP tool %q is not in the supported catalog", i, name)
			}
			if !allowed {
				return fmt.Errorf("action %d: Mattermost MCP tool %q is not permitted in automations", i, name)
			}
		}
	}
	return nil
}
