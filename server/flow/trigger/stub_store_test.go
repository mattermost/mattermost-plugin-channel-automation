package trigger_test

import "github.com/mattermost/mattermost-plugin-channel-automation/server/model"

// stubStore is a minimal model.Store implementation for CandidateFlowIDs
// tests. Only the index lookup methods each trigger actually calls need to
// be populated; the rest panic if touched to surface unexpected calls.
type stubStore struct {
	flowIDsByChannel           map[string][]string
	flowIDsByMembershipChannel map[string][]string
	channelCreatedFlowIDs      []string
	flowIDsByTeam              map[string][]string

	err error
}

func (s *stubStore) GetFlowIDsForChannel(channelID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.flowIDsByChannel[channelID], nil
}

func (s *stubStore) GetFlowIDsForMembershipChannel(channelID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.flowIDsByMembershipChannel[channelID], nil
}

func (s *stubStore) GetChannelCreatedFlowIDs() ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.channelCreatedFlowIDs, nil
}

func (s *stubStore) GetFlowIDsForUserJoinedTeam(teamID string) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.flowIDsByTeam[teamID], nil
}

// Remaining Store methods are unused by the trigger candidate-resolution
// path and panic to surface unexpected calls.

func (s *stubStore) Get(string) (*model.Flow, error) {
	panic("stubStore.Get not implemented")
}

func (s *stubStore) List() ([]*model.Flow, error) {
	panic("stubStore.List not implemented")
}

func (s *stubStore) ListByTriggerChannel(string) ([]*model.Flow, error) {
	panic("stubStore.ListByTriggerChannel not implemented")
}

func (s *stubStore) ListScheduled() ([]*model.Flow, error) {
	panic("stubStore.ListScheduled not implemented")
}

func (s *stubStore) Save(*model.Flow) error {
	panic("stubStore.Save not implemented")
}

func (s *stubStore) Delete(string) error {
	panic("stubStore.Delete not implemented")
}

func (s *stubStore) CountByTriggerChannel(string) (int, error) {
	panic("stubStore.CountByTriggerChannel not implemented")
}
