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

func TestNewSafeThread_NilInput(t *testing.T) {
	assert.Nil(t, NewSafeThread(nil, "root", nil))
}

func TestNewSafeThread_EmptyList(t *testing.T) {
	list := &mmmodel.PostList{Order: []string{}, Posts: map[string]*mmmodel.Post{}}
	st := NewSafeThread(list, "root1", nil)
	require.NotNil(t, st)
	assert.Equal(t, "root1", st.RootID)
	assert.Equal(t, 0, st.PostCount)
	assert.Nil(t, st.Messages)
}

func TestNewSafeThread_SortsOldestFirstAndDedupesUserLookups(t *testing.T) {
	list := &mmmodel.PostList{
		// Order is intentionally newest-first (the server convention) to
		// verify we do not rely on it and instead sort by CreateAt.
		Order: []string{"p3", "p1", "p2"},
		Posts: map[string]*mmmodel.Post{
			"p1": {Id: "p1", UserId: "u1", Message: "first", CreateAt: 100},
			"p2": {Id: "p2", UserId: "u2", Message: "second", CreateAt: 200},
			"p3": {Id: "p3", UserId: "u1", Message: "third", CreateAt: 300},
		},
	}
	calls := 0
	userFor := func(uid string) *SafeUser {
		calls++
		return &SafeUser{Id: uid, Username: "name-" + uid, FirstName: "First-" + uid, LastName: "Last-" + uid}
	}
	st := NewSafeThread(list, "p1", userFor)
	require.NotNil(t, st)
	assert.Equal(t, 3, st.PostCount)
	require.Len(t, st.Messages, 3)
	assert.Equal(t, "p1", st.Messages[0].Id)
	assert.Equal(t, "p2", st.Messages[1].Id)
	assert.Equal(t, "p3", st.Messages[2].Id)
	require.NotNil(t, st.Messages[0].User)
	assert.Equal(t, "name-u1", st.Messages[0].User.Username)
	assert.Equal(t, "name-u2", st.Messages[1].User.Username)
	assert.Equal(t, "name-u1", st.Messages[2].User.Username)
	// u1 appears twice but lookup happens once per distinct user.
	assert.Equal(t, 2, calls, "userFor should dedupe lookups per user ID")
}

func TestNewSafeThread_OverridesThreadIdAndKeepsRawMessage(t *testing.T) {
	// NewSafePost would set the root post's ThreadId to its own Id;
	// NewSafeThread must override it to the resolved root for consistency
	// across all messages in the thread. Message must remain raw — no
	// attachment flattening.
	root := &mmmodel.Post{Id: "p1", UserId: "u1", Message: "raw body", CreateAt: 1}
	mmmodel.ParseSlackAttachment(root, []*mmmodel.SlackAttachment{{Title: "Alert", Text: "details"}})
	list := &mmmodel.PostList{
		Order: []string{"p1"},
		Posts: map[string]*mmmodel.Post{"p1": root},
	}
	st := NewSafeThread(list, "p1", func(_ string) *SafeUser { return &SafeUser{Id: "u1", Username: "alice"} })
	require.NotNil(t, st)
	require.Len(t, st.Messages, 1)
	assert.Equal(t, "p1", st.Messages[0].ThreadId)
	assert.Equal(t, "raw body", st.Messages[0].Message, "Message must mirror NewSafePost (raw, no attachment flattening)")
}

func TestNewSafeThread_NilUsernameResolverKeepsUserIdFallback(t *testing.T) {
	list := &mmmodel.PostList{
		Order: []string{"p1"},
		Posts: map[string]*mmmodel.Post{
			"p1": {Id: "p1", UserId: "u1", Message: "hi", CreateAt: 1},
		},
	}
	st := NewSafeThread(list, "p1", nil)
	require.NotNil(t, st)
	require.Len(t, st.Messages, 1)
	require.NotNil(t, st.Messages[0].User)
	assert.Equal(t, "u1", st.Messages[0].User.Id)
	assert.Empty(t, st.Messages[0].User.Username)
}

func TestNewSafeThread_FailedUserLookupKeepsUserIdFallback(t *testing.T) {
	list := &mmmodel.PostList{
		Order: []string{"p1"},
		Posts: map[string]*mmmodel.Post{
			"p1": {Id: "p1", UserId: "u1", Message: "hi", CreateAt: 1},
		},
	}
	st := NewSafeThread(list, "p1", func(_ string) *SafeUser { return nil })
	require.NotNil(t, st)
	require.Len(t, st.Messages, 1)
	require.NotNil(t, st.Messages[0].User)
	assert.Equal(t, "u1", st.Messages[0].User.Id)
}

func TestSafeUser_AuthorDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   *SafeUser
		want string
	}{
		{"both", &SafeUser{Username: "alice", FirstName: "Alice", LastName: "Smith"}, "@alice (Alice Smith)"},
		{"username only", &SafeUser{Username: "alice"}, "@alice"},
		{"name only", &SafeUser{FirstName: "Alice", LastName: "Smith"}, "(Alice Smith)"},
		{"first name only", &SafeUser{FirstName: "Alice"}, "(Alice)"},
		{"userid fallback", &SafeUser{Id: "u123"}, "u123"},
		{"empty user", &SafeUser{}, "unknown"},
		{"nil receiver", nil, "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.in.AuthorDisplay())
		})
	}
}

func TestSafeThread_TranscriptDisplay(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var st *SafeThread
		assert.Empty(t, st.TranscriptDisplay())
	})
	t.Run("empty messages", func(t *testing.T) {
		st := &SafeThread{}
		assert.Empty(t, st.TranscriptDisplay())
	})
	t.Run("renders one line per message", func(t *testing.T) {
		st := &SafeThread{Messages: []SafePost{
			{User: &SafeUser{Username: "alice", FirstName: "Alice", LastName: "Smith"}, Message: "hi"},
			{User: &SafeUser{Id: "u2"}, Message: "fallback id"},
		}}
		assert.Equal(t, "@alice (Alice Smith): hi\nu2: fallback id", st.TranscriptDisplay())
	})
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

func TestNewSafeTeam_NilInputReturnsPlaceholder(t *testing.T) {
	safe := NewSafeTeam(nil)
	require.NotNil(t, safe)
	assert.Empty(t, safe.Id)
	assert.Equal(t, "[unknown team]", safe.Name)
	assert.Equal(t, "[unknown team]", safe.DisplayName)
	assert.Empty(t, safe.DefaultChannelId)
}

func TestNewSafeTeam_PreservesFields(t *testing.T) {
	team := &mmmodel.Team{
		Id:          "team1",
		Name:        "test-team",
		DisplayName: "Test Team",
	}
	safe := NewSafeTeam(team)
	require.NotNil(t, safe)
	assert.Equal(t, "team1", safe.Id)
	assert.Equal(t, "test-team", safe.Name)
	assert.Equal(t, "Test Team", safe.DisplayName)
	assert.Empty(t, safe.DefaultChannelId, "DefaultChannelId should be empty — populated by the caller")
}

func TestNewSafeTeam_ExcludesSensitiveFields(t *testing.T) {
	team := &mmmodel.Team{
		Id:              "team1",
		Name:            "test-team",
		DisplayName:     "Test Team",
		Email:           "team@example.com",
		AllowedDomains:  "example.com",
		InviteId:        "secret-invite-id",
		CompanyName:     "Acme Corp",
		AllowOpenInvite: true,
	}
	safe := NewSafeTeam(team)
	require.NotNil(t, safe)

	// Only Id, Name, DisplayName should be copied.
	assert.Equal(t, "team1", safe.Id)
	assert.Equal(t, "test-team", safe.Name)
	assert.Equal(t, "Test Team", safe.DisplayName)
}
