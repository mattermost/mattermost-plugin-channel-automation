package notifier

import (
	"strings"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/mock"
)

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
	expectedOpts := mmmodel.PluginKVSetOptions{
		Atomic:          true,
		OldValue:        nil,
		ExpireInSeconds: 3600,
	}
	api.On("KVSetWithOptions", "flow_failure_notify_flow1", []byte{1}, expectedOpts).Return(true, nil)
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

	n := NewCreatorNotifier(api, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
}

func TestNotifyFailure_SkipsWhenCooldownActive(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

	n := NewCreatorNotifier(api, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_SkipsOnKVError(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).
		Return(false, mmmodel.NewAppError("KVSetWithOptions", "kv.fail", nil, "boom", 500))
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	api.AssertNotCalled(t, "GetDirectChannel", mock.Anything, mock.Anything)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_NoOpOnMissingFields(t *testing.T) {
	api := &plugintest.API{}
	n := NewCreatorNotifier(api, "bot1")

	n.NotifyFailure(FailureDetails{FlowID: "f", CreatedBy: ""}) // no creator
	api.AssertNotCalled(t, "KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything)

	n2 := NewCreatorNotifier(api, "")
	n2.NotifyFailure(sampleDetails()) // no bot
	api.AssertNotCalled(t, "KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything)
}

func TestNotifyFailure_LogsOnDMOpenFailure(t *testing.T) {
	api := &plugintest.API{}
	api.On("KVSetWithOptions", mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	api.On("GetDirectChannel", "creator1", "bot1").
		Return((*mmmodel.Channel)(nil), mmmodel.NewAppError("GetDirectChannel", "dm.fail", nil, "boom", 500))
	api.On("KVDelete", mock.Anything).Return(nil)
	api.On("LogError", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()

	n := NewCreatorNotifier(api, "bot1")
	n.NotifyFailure(sampleDetails())

	api.AssertExpectations(t)
	api.AssertNotCalled(t, "CreatePost", mock.Anything)
}

func TestNotifyFailure_NilSafe(t *testing.T) {
	var n *CreatorNotifier
	n.NotifyFailure(sampleDetails()) // must not panic
}
