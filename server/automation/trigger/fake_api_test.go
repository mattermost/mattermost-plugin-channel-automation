package trigger_test

import (
	"fmt"
	"net/http"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
)

// fakeTriggerAPI is a minimal in-memory implementation of model.TriggerAPI
// for unit-testing BuildTriggerData. Lookups for unregistered IDs return a
// 500 AppError. Note that some triggers treat lookup failures as best-effort
// (logging a warning and returning partial data), so tests must explicitly
// assert the downstream behavior rather than relying on the AppError to
// fail the test outright.
type fakeTriggerAPI struct {
	channels        map[string]*mmmodel.Channel
	channelErrors   map[string]*mmmodel.AppError
	users           map[string]*mmmodel.User
	userErrors      map[string]*mmmodel.AppError
	teams           map[string]*mmmodel.Team
	teamErrors      map[string]*mmmodel.AppError
	channelByName   map[string]*mmmodel.Channel
	channelByNameEr map[string]*mmmodel.AppError
	postThreads     map[string]*mmmodel.PostList
	postThreadErrs  map[string]*mmmodel.AppError

	warnCalls []string
}

func newFakeTriggerAPI() *fakeTriggerAPI {
	return &fakeTriggerAPI{
		channels:        map[string]*mmmodel.Channel{},
		channelErrors:   map[string]*mmmodel.AppError{},
		users:           map[string]*mmmodel.User{},
		userErrors:      map[string]*mmmodel.AppError{},
		teams:           map[string]*mmmodel.Team{},
		teamErrors:      map[string]*mmmodel.AppError{},
		channelByName:   map[string]*mmmodel.Channel{},
		channelByNameEr: map[string]*mmmodel.AppError{},
		postThreads:     map[string]*mmmodel.PostList{},
		postThreadErrs:  map[string]*mmmodel.AppError{},
	}
}

func (f *fakeTriggerAPI) GetChannel(id string) (*mmmodel.Channel, *mmmodel.AppError) {
	if err, ok := f.channelErrors[id]; ok {
		return nil, err
	}
	if c, ok := f.channels[id]; ok {
		return c, nil
	}
	return nil, mmmodel.NewAppError("fake", "not_found", nil, fmt.Sprintf("channel %s not registered in fake", id), http.StatusInternalServerError)
}

func (f *fakeTriggerAPI) GetChannelByName(teamID, name string, _ bool) (*mmmodel.Channel, *mmmodel.AppError) {
	key := teamID + "/" + name
	if err, ok := f.channelByNameEr[key]; ok {
		return nil, err
	}
	if c, ok := f.channelByName[key]; ok {
		return c, nil
	}
	return nil, mmmodel.NewAppError("fake", "not_found", nil, fmt.Sprintf("channel %s not registered in fake", key), http.StatusInternalServerError)
}

func (f *fakeTriggerAPI) GetUser(id string) (*mmmodel.User, *mmmodel.AppError) {
	if err, ok := f.userErrors[id]; ok {
		return nil, err
	}
	if u, ok := f.users[id]; ok {
		return u, nil
	}
	return nil, mmmodel.NewAppError("fake", "not_found", nil, fmt.Sprintf("user %s not registered in fake", id), http.StatusInternalServerError)
}

func (f *fakeTriggerAPI) GetTeam(id string) (*mmmodel.Team, *mmmodel.AppError) {
	if err, ok := f.teamErrors[id]; ok {
		return nil, err
	}
	if t, ok := f.teams[id]; ok {
		return t, nil
	}
	return nil, mmmodel.NewAppError("fake", "not_found", nil, fmt.Sprintf("team %s not registered in fake", id), http.StatusInternalServerError)
}

func (f *fakeTriggerAPI) GetPostThread(postID string) (*mmmodel.PostList, *mmmodel.AppError) {
	if err, ok := f.postThreadErrs[postID]; ok {
		return nil, err
	}
	if pl, ok := f.postThreads[postID]; ok {
		return pl, nil
	}
	return nil, mmmodel.NewAppError("fake", "not_found", nil, fmt.Sprintf("post thread %s not registered in fake", postID), http.StatusInternalServerError)
}

func (f *fakeTriggerAPI) LogWarn(msg string, _ ...any) {
	f.warnCalls = append(f.warnCalls, msg)
}
