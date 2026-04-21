package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMattermostMCPCatalog_DevToolsExcluded(t *testing.T) {
	devOnly := []string{"create_user", "create_post_as_user", "create_team", "add_user_to_team"}
	for _, name := range devOnly {
		assert.False(t, isMattermostMCPServerTool(name), "dev tool %q must not appear in production catalog", name)
	}
}

func TestMattermostMCPCatalog_ExpectedProductionCount(t *testing.T) {
	assert.Len(t, mattermostMCPServerToolNames, 20)
}

func TestMattermostMCPCatalog_EveryGuardrailToolIsCatalogued(t *testing.T) {
	for name := range toolRegistry {
		require.True(t, isMattermostMCPServerTool(name),
			"toolRegistry contains %q which is missing from mattermostMCPServerToolNames — sync with agents plugin MCP tools", name)
	}
}
