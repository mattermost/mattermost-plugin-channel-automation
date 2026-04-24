package notifier

import (
	"errors"
	"strings"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// fakeCooldownStore is a hand-rolled mock for CooldownStore. Using mock.Mock
// directly keeps the test wiring lightweight and avoids generated mocks for a
// two-method interface.
type fakeCooldownStore struct {
	mock.Mock
}

func (f *fakeCooldownStore) Claim(flowID string) (bool, error) {
	args := f.Called(flowID)
	return args.Bool(0), args.Error(1)
}

func (f *fakeCooldownStore) Release(flowID string) error {
	return f.Called(flowID).Error(0)
}

func sampleDetails() FailureDetails {
	return FailureDetails{
		FlowID:      "flow1",
		FlowName:    "My Flow",
		CreatedBy:   "creator1",
		ActionID:    "ai_step",
		ActionType:  "ai_prompt",
		ErrorMsg:    "AI completion failed: tool not allowed",
		ExecutionID: "exec1",
	}
}

func TestNotifyFailure_SendsDMOnFirstClaim(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(true, nil)
	api.On("GetDirectChannel", "creator1", "bot1").Return(&mmmodel.Channel{Id: "dm1"}, nil)
	api.On("CreatePost", mock.MatchedBy(func(p *mmmodel.Post) bool {
		return p.UserId == "bot1" &&
			p.ChannelId == "dm1" &&
			strings.Contains(p.Message, "My Flow") &&
			strings.Contains(p.Message, "ai_step") &&
			strings.Contains(p.Message, "ai_prompt") &&
			strings.Contains(p.Message, "AI completion failed: tool not allowed") &&
			strings.Contains(p.Message, "exec1")
	})).Return(&mmmodel.Post{Id: "p1"}, nil)

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	cooldown.AssertExpectations(t)
	cooldown.AssertNotCalled(t, "Release", mock.Anything)
}

func TestNotifyFailure_SkipsWhenCooldownActive(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(false, nil)

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	cooldown.AssertExpectations(t)
	cooldown.AssertNotCalled(t, "Release", mock.Anything)
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_SkipsOnClaimError(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(false, errors.New("kv boom"))
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	cooldown.AssertExpectations(t)
	cooldown.AssertNotCalled(t, "Release", mock.Anything)
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_NoOpOnMissingFields(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(FailureDetails{FlowID: "f", CreatedBy: ""}) // no creator

	n2 := NewCreatorNotifier(api, cooldown, "")
	n2.NotifyFailure(sampleDetails()) // no bot

	cooldown.AssertNotCalled(t, "Claim", mock.Anything)
	cooldown.AssertNotCalled(t, "Release", mock.Anything)
}

func TestNotifyFailure_ReleasesCooldownOnDMOpenFailure(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(true, nil)
	cooldown.On("Release", "flow1").Return(nil)
	api.On("GetDirectChannel", "creator1", "bot1").
		Return((*mmmodel.Channel)(nil), mmmodel.NewAppError("GetDirectChannel", "dm.fail", nil, "boom", 500))
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	cooldown.AssertExpectations(t)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_ReleasesCooldownOnCreatePostFailure(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(true, nil)
	cooldown.On("Release", "flow1").Return(nil)
	api.On("GetDirectChannel", "creator1", "bot1").Return(&mmmodel.Channel{Id: "dm1"}, nil)
	api.On("CreatePost", mock.Anything).
		Return((*mmmodel.Post)(nil), mmmodel.NewAppError("CreatePost", "post.fail", nil, "boom", 500))
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	cooldown.AssertExpectations(t)
}

func TestNotifyFailure_LogsWhenReleaseFails(t *testing.T) {
	api := &plugintest.API{}
	cooldown := &fakeCooldownStore{}
	cooldown.On("Claim", "flow1").Return(true, nil)
	cooldown.On("Release", "flow1").Return(errors.New("kv del boom"))
	api.On("GetDirectChannel", "creator1", "bot1").
		Return((*mmmodel.Channel)(nil), mmmodel.NewAppError("GetDirectChannel", "dm.fail", nil, "boom", 500))
	// Two LogError calls: one for the DM-open failure, one for the release failure.
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, cooldown, "bot1")
	n.NotifyFailure(sampleDetails())

	cooldown.AssertExpectations(t)
	// Confirm both log lines fired.
	logs := 0
	for _, call := range api.Calls {
		if call.Method == "LogError" {
			logs++
		}
	}
	assert.Equal(t, 2, logs)
}

func TestNotifyFailure_NilSafe(t *testing.T) {
	var n *CreatorNotifier
	n.NotifyFailure(sampleDetails()) // must not panic
}

func TestNotifyFailure_NoOpWhenCooldownStoreMissing(t *testing.T) {
	api := &plugintest.API{}
	n := NewCreatorNotifier(api, nil, "bot1")
	n.NotifyFailure(sampleDetails()) // must not panic, must not touch api
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
}
