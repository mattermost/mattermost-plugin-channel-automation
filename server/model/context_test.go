package model

import (
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSafePost_NilInput(t *testing.T) {
	assert.Nil(t, NewSafePost(nil))
}

func TestNewSafePost_SetsThreadId(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", ChannelId: "ch1", Message: "hello"}
	safe := NewSafePost(post)
	require.NotNil(t, safe)
	assert.Equal(t, "post1", safe.ThreadId, "ThreadId should equal post Id when RootId is empty")
}

func TestNewSafePost_PreservesRootId(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", RootId: "root1", ChannelId: "ch1", Message: "hello"}
	safe := NewSafePost(post)
	require.NotNil(t, safe)
	assert.Equal(t, "root1", safe.ThreadId, "ThreadId should equal RootId when set")
}

func TestNewSafeUser_NilInput(t *testing.T) {
	assert.Nil(t, NewSafeUser(nil))
}

func TestNewSafeUser_StripsSensitiveFields(t *testing.T) {
	authData := "auth-data-secret"
	user := &mmmodel.User{
		Id:          "u1",
		Username:    "testuser",
		FirstName:   "Test",
		LastName:    "User",
		Email:       "test@example.com",
		AuthData:    &authData,
		Password:    "supersecret",
		NotifyProps: mmmodel.StringMap{"push": "all"},
	}
	safe := NewSafeUser(user)
	require.NotNil(t, safe)

	// Sensitive fields must not be accessible — SafeUser has no such fields.
	assert.Equal(t, "u1", safe.Id)
	assert.Equal(t, "testuser", safe.Username)
	assert.Equal(t, "Test", safe.FirstName)
	assert.Equal(t, "User", safe.LastName)
}

func TestNewSafeUser_PreservesDisplayFields(t *testing.T) {
	user := &mmmodel.User{
		Id:        "u1",
		Username:  "alice",
		FirstName: "Alice",
		LastName:  "Smith",
	}
	safe := NewSafeUser(user)
	require.NotNil(t, safe)
	assert.Equal(t, "u1", safe.Id)
	assert.Equal(t, "alice", safe.Username)
	assert.Equal(t, "Alice", safe.FirstName)
	assert.Equal(t, "Smith", safe.LastName)
}

func TestNewSafeChannel_NilInput(t *testing.T) {
	assert.Nil(t, NewSafeChannel(nil))
}

func TestNewSafeChannel_PreservesFields(t *testing.T) {
	ch := &mmmodel.Channel{
		Id:          "ch1",
		Name:        "test-channel",
		DisplayName: "Test Channel",
	}
	safe := NewSafeChannel(ch)
	require.NotNil(t, safe)
	assert.Equal(t, "ch1", safe.Id)
	assert.Equal(t, "test-channel", safe.Name)
	assert.Equal(t, "Test Channel", safe.DisplayName)
}
