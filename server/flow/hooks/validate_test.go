package hooks

import (
	"testing"

	"github.com/mattermost/mattermost-plugin-agents/public/bridgeclient"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-plugin-channel-automation/server/model"
)

type stubLister struct {
	tools []bridgeclient.BridgeToolInfo
	err   error
}

func (s *stubLister) GetAgentTools(_, _ string) ([]bridgeclient.BridgeToolInfo, error) {
	return s.tools, s.err
}

func mmEmbeddedTool(name string) bridgeclient.BridgeToolInfo {
	return bridgeclient.BridgeToolInfo{Name: name, ServerOrigin: EmbeddedMattermostMCPOrigin}
}

func flowWithTools(tools []string, guardrails *model.Guardrails) *model.Flow {
	return &model.Flow{
		Actions: []model.Action{
			{
				ID: "ai1",
				AIPrompt: &model.AIPromptActionConfig{
					ProviderID:   "agent-1",
					AllowedTools: tools,
					Guardrails:   guardrails,
				},
			},
		},
	}
}

func TestValidateAllowedTools_GetUserChannels_AllowedWithoutGuardrails(t *testing.T) {
	bridge := &stubLister{tools: []bridgeclient.BridgeToolInfo{mmEmbeddedTool("get_user_channels")}}
	f := flowWithTools([]string{"get_user_channels"}, nil)

	require.NoError(t, ValidateAllowedTools(f, "user1", bridge))
}

func TestValidateAllowedTools_GetUserChannels_AllowedWithEmptyGuardrails(t *testing.T) {
	bridge := &stubLister{tools: []bridgeclient.BridgeToolInfo{mmEmbeddedTool("get_user_channels")}}
	f := flowWithTools([]string{"get_user_channels"}, &model.Guardrails{Channels: nil})

	require.NoError(t, ValidateAllowedTools(f, "user1", bridge))
}

func TestValidateAllowedTools_GetUserChannels_RejectedWithGuardrails(t *testing.T) {
	bridge := &stubLister{tools: []bridgeclient.BridgeToolInfo{mmEmbeddedTool("get_user_channels")}}
	guardrails := &model.Guardrails{Channels: []model.GuardrailChannel{
		{ChannelID: mmmodel.NewId(), TeamID: mmmodel.NewId()},
	}}
	f := flowWithTools([]string{"get_user_channels"}, guardrails)

	err := ValidateAllowedTools(f, "user1", bridge)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get_user_channels is not permitted when channel guardrails are configured")
}
