package bot

import (
	"encoding/json"
	"fmt"
	"strings"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	kvTeamBotPrefix        = "teambot:"
	kvTeamBotReversePrefix = "teambot_rev:"
	botUsernamePrefix      = "automation-"
	maxUsernameLen         = 64
)

// Manager handles creation and lookup of team-scoped automation bots.
// Each team gets at most one bot, restricted to public channels via plugin hooks.
type Manager struct {
	api plugin.API
}

// NewManager creates a Manager backed by the given plugin API.
func NewManager(api plugin.API) *Manager {
	return &Manager{api: api}
}

// EnsureTeamBot returns the bot user ID for the given team, creating the bot
// if it does not already exist. The bot's team membership is always verified
// and re-added if missing (CreateTeamMember is idempotent).
func (m *Manager) EnsureTeamBot(teamID string) (string, error) {
	botUserID, _ := m.loadTeamBot(teamID)

	if botUserID == "" {
		team, appErr := m.api.GetTeam(teamID)
		if appErr != nil {
			return "", fmt.Errorf("failed to get team %s: %w", teamID, appErr)
		}

		username := buildBotUsername(team.Name)
		bot, appErr := m.api.CreateBot(&mmmodel.Bot{
			Username:    username,
			DisplayName: fmt.Sprintf("Automation (%s)", team.DisplayName),
			Description: fmt.Sprintf("Team-scoped automation bot for %s. Restricted to public channels.", team.DisplayName),
		})
		if appErr != nil {
			return "", fmt.Errorf("failed to create team bot for team %s: %w", teamID, appErr)
		}

		botUserID = bot.UserId
		if err := m.saveTeamBot(teamID, botUserID); err != nil {
			return "", err
		}
	}

	if _, appErr := m.api.CreateTeamMember(teamID, botUserID); appErr != nil {
		return "", fmt.Errorf("failed to add team bot to team %s: %w", teamID, appErr)
	}

	return botUserID, nil
}

// GetTeamForBot returns the team ID that owns the given bot user ID.
// Returns empty string if the user is not a managed team bot.
func (m *Manager) GetTeamForBot(botUserID string) (string, error) {
	data, appErr := m.api.KVGet(kvTeamBotReversePrefix + botUserID)
	if appErr != nil {
		return "", fmt.Errorf("failed to look up team for bot %s: %w", botUserID, appErr)
	}
	if data == nil {
		return "", nil
	}
	return string(data), nil
}

// IsTeamBot returns true if the given user ID is a managed team bot.
func (m *Manager) IsTeamBot(botUserID string) bool {
	teamID, err := m.GetTeamForBot(botUserID)
	if err != nil {
		return false
	}
	return teamID != ""
}

// AddBotToChannels ensures the bot is a member of each specified channel.
// Errors on individual channels are logged but do not stop processing.
func (m *Manager) AddBotToChannels(botUserID string, channelIDs []string) error {
	var firstErr error
	for _, chID := range channelIDs {
		if _, appErr := m.api.AddChannelMember(chID, botUserID); appErr != nil {
			m.api.LogWarn("Failed to add team bot to channel",
				"bot_user_id", botUserID,
				"channel_id", chID,
				"err", appErr.Error(),
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to add bot to channel %s: %w", chID, appErr)
			}
		}
	}
	return firstErr
}

func (m *Manager) loadTeamBot(teamID string) (string, error) {
	data, appErr := m.api.KVGet(kvTeamBotPrefix + teamID)
	if appErr != nil {
		return "", fmt.Errorf("failed to load team bot mapping: %w", appErr)
	}
	if data == nil {
		return "", nil
	}

	var mapping struct {
		BotUserID string `json:"bot_user_id"`
	}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return "", fmt.Errorf("failed to unmarshal team bot mapping: %w", err)
	}
	return mapping.BotUserID, nil
}

func (m *Manager) saveTeamBot(teamID, botUserID string) error {
	mapping := struct {
		BotUserID string `json:"bot_user_id"`
	}{BotUserID: botUserID}

	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal team bot mapping: %w", err)
	}

	if appErr := m.api.KVSet(kvTeamBotPrefix+teamID, data); appErr != nil {
		return fmt.Errorf("failed to save team bot mapping: %w", appErr)
	}
	if appErr := m.api.KVSet(kvTeamBotReversePrefix+botUserID, []byte(teamID)); appErr != nil {
		return fmt.Errorf("failed to save reverse team bot mapping: %w", appErr)
	}
	return nil
}

func buildBotUsername(teamName string) string {
	name := strings.ToLower(teamName)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-_.")

	username := botUsernamePrefix + name
	if len(username) > maxUsernameLen {
		username = username[:maxUsernameLen]
	}
	return username
}
