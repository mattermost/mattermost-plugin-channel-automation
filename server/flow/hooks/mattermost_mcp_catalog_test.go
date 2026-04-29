package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMattermostMCPCatalog_DevToolsExcluded(t *testing.T) {
	devOnly := []string{"create_user", "create_post_as_user", "create_team", "add_user_to_team"}
	for _, name := range devOnly {
		_, ok := LookupMattermostMCPTool(name)
		assert.False(t, ok, "dev tool %q must not appear in production catalog", name)
	}
}

func TestMattermostMCPCatalog_ExpectedProductionCount(t *testing.T) {
	assert.Len(t, mattermostMCPServerTools, 20)
}

func TestMattermostMCPCatalog_DisallowedToolsPresentAndDisallowed(t *testing.T) {
	disallowed := []string{"create_post", "dm", "group_message"}
	for _, name := range disallowed {
		known, allowed := IsAllowedMattermostMCPTool(name)
		require.True(t, known, "%q must be present in the catalog", name)
		assert.False(t, allowed, "%q must be marked Allowed=false", name)
	}
}

func TestMattermostMCPCatalog_HookImplsRequireAllowed(t *testing.T) {
	for name, entry := range mattermostMCPServerTools {
		if entry.Before != nil {
			assert.True(t, entry.Allowed, "tool %q has a Before hook but is not Allowed", name)
		}
	}
}

func TestMattermostMCPCatalog_AddUserToChannel_HasBeforeHook(t *testing.T) {
	entry, ok := LookupMattermostMCPTool("add_user_to_channel")
	require.True(t, ok)
	assert.True(t, entry.Allowed)
	assert.NotNil(t, entry.Before, "add_user_to_channel must have a Before hook to enforce channel guardrails")
}

func TestMattermostMCPCatalog_CreateChannel_HasBeforeHook(t *testing.T) {
	entry, ok := LookupMattermostMCPTool("create_channel")
	require.True(t, ok)
	assert.True(t, entry.Allowed)
	assert.NotNil(t, entry.Before, "create_channel must have a Before hook to enforce team guardrails")
}

func TestIsAllowedMattermostMCPTool_UnknownTool(t *testing.T) {
	known, allowed := IsAllowedMattermostMCPTool("not_a_real_tool")
	assert.False(t, known)
	assert.False(t, allowed)
}
