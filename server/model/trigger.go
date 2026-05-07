package model

import mmmodel "github.com/mattermost/mattermost/server/public/model"

// Trigger type constants.
const (
	TriggerTypeMessagePosted     = "message_posted"
	TriggerTypeSchedule          = "schedule"
	TriggerTypeMembershipChanged = "membership_changed"
	TriggerTypeChannelCreated    = "channel_created"
	TriggerTypeUserJoinedTeam    = "user_joined_team"
)

// TriggerAPI is the narrow subset of plugin.API that trigger handlers need
// for building TriggerData. Keeping it separate from plugin.API makes
// handlers easy to unit-test.
type TriggerAPI interface {
	GetChannel(channelID string) (*mmmodel.Channel, *mmmodel.AppError)
	GetChannelByName(teamID, name string, includeDeleted bool) (*mmmodel.Channel, *mmmodel.AppError)
	GetUser(userID string) (*mmmodel.User, *mmmodel.AppError)
	GetTeam(teamID string) (*mmmodel.Team, *mmmodel.AppError)
	GetPostThread(postID string) (*mmmodel.PostList, *mmmodel.AppError)
	LogWarn(msg string, keyValuePairs ...any)
}

// TriggerHandler owns the lifecycle of a single trigger type: config
// validation, matching events, resolving candidate flows, and building the
// TriggerData passed to automation execution.
type TriggerHandler interface {
	// Type returns the trigger type string (e.g. "message_posted").
	Type() string

	// Matches reports whether the trigger configuration matches the event.
	// Called after CandidateAutomationIDs has already narrowed down the set.
	Matches(trigger *Trigger, event *Event) bool

	// Validate checks the per-type configuration (required fields, enum
	// values, interval bounds, etc.). The existing trigger is passed on
	// update so fields like Schedule.StartAt can be validated only when
	// they change. Mutual-exclusion of trigger types is validated by the
	// caller before Validate is invoked.
	Validate(trigger *Trigger, existing *Trigger) error

	// CandidateAutomationIDs returns the automation IDs that could potentially match
	// this event, using whatever store index is most selective for the
	// trigger type. Returning nil is valid ("no candidates").
	CandidateAutomationIDs(store Store, event *Event) ([]string, error)

	// BuildTriggerData fetches auxiliary Mattermost data (channel, user,
	// team, default channel) and returns the TriggerData payload attached
	// to every work item enqueued for this event. Called once per event,
	// after at least one automation has matched (i.e. CandidateAutomationIDs returned
	// a non-empty set and Matches returned true for at least one of them).
	// An error aborts dispatch for the event.
	BuildTriggerData(api TriggerAPI, event *Event) (TriggerData, error)
}
