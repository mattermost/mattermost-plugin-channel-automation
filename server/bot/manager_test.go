package bot

import (
	"encoding/json"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuildBotUsername(t *testing.T) {
	tests := []struct {
		teamName string
		expected string
	}{
		{"engineering", "automation-engineering"},
		{"My Team!", "automation-my-team"},
		{"a", "automation-a"},
		{"UPPER", "automation-upper"},
		{"with spaces", "automation-with-spaces"},
	}
	for _, tc := range tests {
		t.Run(tc.teamName, func(t *testing.T) {
			got := buildBotUsername(tc.teamName)
			assert.Equal(t, tc.expected, got)
			assert.LessOrEqual(t, len(got), maxUsernameLen)
		})
	}
}

func TestBuildBotUsername_LongTeamName(t *testing.T) {
	long := ""
	for range 100 {
		long += "a"
	}
	got := buildBotUsername(long)
	assert.LessOrEqual(t, len(got), maxUsernameLen)
	assert.True(t, len(got) > 0)
}

func TestEnsureTeamBot_CreatesNewBot(t *testing.T) {
	api := &plugintest.API{}

	api.On("KVGet", kvTeamBotPrefix+"team1").Return(nil, nil)
	api.On("GetTeam", "team1").Return(&mmmodel.Team{
		Id:          "team1",
		Name:        "myteam",
		DisplayName: "My Team",
	}, nil)
	api.On("CreateBot", mock.MatchedBy(func(b *mmmodel.Bot) bool {
		return b.Username == "automation-myteam"
	})).Return(&mmmodel.Bot{UserId: "bot-user-1", Username: "automation-myteam"}, nil)

	mapping, _ := json.Marshal(struct {
		BotUserID string `json:"bot_user_id"`
	}{BotUserID: "bot-user-1"})
	api.On("KVSet", kvTeamBotPrefix+"team1", mapping).Return(nil)
	api.On("KVSet", kvTeamBotReversePrefix+"bot-user-1", []byte("team1")).Return(nil)
	api.On("CreateTeamMember", "team1", "bot-user-1").Return(&mmmodel.TeamMember{}, nil)

	m := NewManager(api)
	botID, err := m.EnsureTeamBot("team1")
	require.NoError(t, err)
	assert.Equal(t, "bot-user-1", botID)
}

func TestEnsureTeamBot_ReturnsExistingBot(t *testing.T) {
	api := &plugintest.API{}

	mapping, _ := json.Marshal(struct {
		BotUserID string `json:"bot_user_id"`
	}{BotUserID: "bot-user-existing"})
	api.On("KVGet", kvTeamBotPrefix+"team1").Return(mapping, nil)
	api.On("CreateTeamMember", "team1", "bot-user-existing").Return(&mmmodel.TeamMember{}, nil)

	m := NewManager(api)
	botID, err := m.EnsureTeamBot("team1")
	require.NoError(t, err)
	assert.Equal(t, "bot-user-existing", botID)

	api.AssertNotCalled(t, "CreateBot", mock.Anything)
	api.AssertCalled(t, "CreateTeamMember", "team1", "bot-user-existing")
}

func TestGetTeamForBot(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVGet", kvTeamBotReversePrefix+"bot-user-1").Return([]byte("team1"), nil)

	m := NewManager(api)
	teamID, err := m.GetTeamForBot("bot-user-1")
	require.NoError(t, err)
	assert.Equal(t, "team1", teamID)
}

func TestGetTeamForBot_NotATeamBot(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVGet", kvTeamBotReversePrefix+"random-user").Return(nil, nil)

	m := NewManager(api)
	teamID, err := m.GetTeamForBot("random-user")
	require.NoError(t, err)
	assert.Equal(t, "", teamID)
}

func TestIsTeamBot(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVGet", kvTeamBotReversePrefix+"bot-user-1").Return([]byte("team1"), nil)
	api.On("KVGet", kvTeamBotReversePrefix+"regular-user").Return(nil, nil)

	m := NewManager(api)
	assert.True(t, m.IsTeamBot("bot-user-1"))
	assert.False(t, m.IsTeamBot("regular-user"))
}

func TestAddBotToChannels(t *testing.T) {
	api := &plugintest.API{}
	api.On("AddChannelMember", "ch1", "bot-user-1").Return(&mmmodel.ChannelMember{}, nil)
	api.On("AddChannelMember", "ch2", "bot-user-1").Return(&mmmodel.ChannelMember{}, nil)

	m := NewManager(api)
	err := m.AddBotToChannels("bot-user-1", []string{"ch1", "ch2"})
	require.NoError(t, err)
}

func TestAddBotToChannels_PartialFailure(t *testing.T) {
	api := &plugintest.API{}
	api.On("AddChannelMember", "ch1", "bot-user-1").Return(nil,
		mmmodel.NewAppError("AddChannelMember", "test", nil, "", 400))
	api.On("AddChannelMember", "ch2", "bot-user-1").Return(&mmmodel.ChannelMember{}, nil)
	api.On("LogWarn", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	m := NewManager(api)
	err := m.AddBotToChannels("bot-user-1", []string{"ch1", "ch2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ch1")
}
