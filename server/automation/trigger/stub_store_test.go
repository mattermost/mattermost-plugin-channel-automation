package trigger_test

import "github.com/mattermost/mattermost-plugin-channel-automation/server/model"

// stubStore is a minimal model.Store implementation for CandidateAutomationIDs
// tests. Only the index lookup methods each trigger actually calls need to
// be populated; the rest panic if touched to surface unexpected calls.
type stubStore struct {
	automationIDsByChannel           map[string][]string
	automationIDsByMembershipChannel map[string][]string
	channelCreatedAutomationIDs      []string
	automationIDsByTeam              map[string][]string

	err error
}

func (s *stubStore) GetAutomationIDsForChannel(channelID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.automationIDsByChannel[channelID], nil
}

func (s *stubStore) GetAutomationIDsForMembershipChannel(channelID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.automationIDsByMembershipChannel[channelID], nil
}

func (s *stubStore) GetChannelCreatedAutomationIDs() ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.channelCreatedAutomationIDs, nil
}

func (s *stubStore) GetAutomationIDsForUserJoinedTeam(teamID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.automationIDsByTeam[teamID], nil
}

// Remaining Store methods are unused by the trigger candidate-resolution
// path and panic to surface unexpected calls.

func (s *stubStore) Get(string) (*model.Automation, error) {
	panic("stubStore.Get not implemented")
}

func (s *stubStore) List() ([]*model.Automation, error) {
	panic("stubStore.List not implemented")
}

func (s *stubStore) ListByTriggerChannel(string) ([]*model.Automation, error) {
	panic("stubStore.ListByTriggerChannel not implemented")
}

func (s *stubStore) ListScheduled() ([]*model.Automation, error) {
	panic("stubStore.ListScheduled not implemented")
}

func (s *stubStore) Save(*model.Automation) error {
	panic("stubStore.Save not implemented")
}

func (s *stubStore) SaveWithChannelLimit(*model.Automation, int, string) error {
	panic("stubStore.SaveWithChannelLimit not implemented")
}

func (s *stubStore) Delete(string) error {
	panic("stubStore.Delete not implemented")
}

func (s *stubStore) CountByTriggerChannel(string) (int, error) {
	panic("stubStore.CountByTriggerChannel not implemented")
}
