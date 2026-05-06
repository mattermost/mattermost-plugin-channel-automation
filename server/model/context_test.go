package model

import (
	"strconv"
	"testing"

	mmmodel "github.com/mattermost/mattermost/server/public/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSafePost_NilInput(t *testing.T) {
	assert.Nil(t, NewSafePost(nil, nil))
}

func TestNewSafePost_SetsThreadId(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", ChannelId: "ch1", Message: "hello"}
	safe := NewSafePost(post, nil)
	require.NotNil(t, safe)
	assert.Equal(t, "post1", safe.ThreadId, "ThreadId should equal post Id when RootId is empty")
}

func TestNewSafePost_PreservesRootId(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", RootId: "root1", ChannelId: "ch1", Message: "hello"}
	safe := NewSafePost(post, nil)
	require.NotNil(t, safe)
	assert.Equal(t, "root1", safe.ThreadId, "ThreadId should equal RootId when set")
}

func TestNewSafePost_PopulatesFallbackAuthorAndCreateAt(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", UserId: "user1", CreateAt: 1234}
	safe := NewSafePost(post, nil)
	require.NotNil(t, safe)
	assert.Equal(t, "user1", safe.User.Id)
	assert.Equal(t, int64(1234), safe.CreateAt)
}

func TestNewSafePost_PopulatesResolvedAuthor(t *testing.T) {
	post := &mmmodel.Post{Id: "post1", UserId: "user1"}
	safe := NewSafePost(post, &SafeUser{Id: "user1", Username: "alice"})
	require.NotNil(t, safe)
	assert.Equal(t, SafeUser{Id: "user1", Username: "alice"}, safe.User)
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
	assert.Equal(t, "name-u1", st.Messages[0].User.Username)
	assert.Equal(t, "name-u2", st.Messages[1].User.Username)
	assert.Equal(t, "name-u1", st.Messages[2].User.Username)
	// u1 appears twice but lookup happens once per distinct user.
	assert.Equal(t, 2, calls, "userFor should dedupe lookups per user ID")
}

func TestNewSafeThread_OverridesThreadIdAndPreservesMessage(t *testing.T) {
	// NewSafePost would set a root post's ThreadId to its own Id;
	// NewSafeThread must override it to the resolved root for consistency
	// across all messages in the thread. The post Message must survive
	// the round trip unchanged.
	root := &mmmodel.Post{Id: "p1", UserId: "u1", Message: "raw body", CreateAt: 1}
	list := &mmmodel.PostList{
		Order: []string{"p1"},
		Posts: map[string]*mmmodel.Post{"p1": root},
	}
	st := NewSafeThread(list, "p1", func(_ string) *SafeUser { return &SafeUser{Id: "u1", Username: "alice"} })
	require.NotNil(t, st)
	require.Len(t, st.Messages, 1)
	assert.Equal(t, "p1", st.Messages[0].ThreadId)
	assert.Equal(t, "raw body", st.Messages[0].Message)
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
	assert.Equal(t, "u1", st.Messages[0].User.Id)
	assert.Empty(t, st.Messages[0].User.Username)
}

func TestNewSafeThread_TruncatesPreservingRootAndRecentReplies(t *testing.T) {
	// Build a thread with the root + (MaxThreadReplies + 5) replies. We
	// expect Messages to be capped at root + the most recent
	// MaxThreadReplies replies (= MaxThreadReplies + 1 total), with
	// PostCount reflecting the original full count and Truncated=true.
	totalReplies := MaxThreadReplies + 5
	posts := make(map[string]*mmmodel.Post, totalReplies+1)
	order := make([]string, 0, totalReplies+1)
	posts["root"] = &mmmodel.Post{Id: "root", UserId: "u1", Message: "topic", CreateAt: 0}
	order = append(order, "root")
	for i := 1; i <= totalReplies; i++ {
		id := "r" + strconv.Itoa(i)
		posts[id] = &mmmodel.Post{Id: id, UserId: "u1", Message: "reply " + strconv.Itoa(i), CreateAt: int64(i)}
		order = append(order, id)
	}
	list := &mmmodel.PostList{Order: order, Posts: posts}

	st := NewSafeThread(list, "root", func(_ string) *SafeUser { return &SafeUser{Username: "alice"} })
	require.NotNil(t, st)
	assert.True(t, st.Truncated)
	assert.Equal(t, totalReplies+1, st.PostCount, "PostCount must reflect the original full thread size")
	require.Len(t, st.Messages, MaxThreadReplies+1, "kept root + MaxThreadReplies replies")

	// Root preserved at the head.
	assert.Equal(t, "root", st.Messages[0].Id)
	// Tail = the most recent MaxThreadReplies replies, in CreateAt order.
	// First retained reply after the root has CreateAt = totalReplies - MaxThreadReplies + 1.
	firstRetainedReplyCreateAt := int64(totalReplies - MaxThreadReplies + 1)
	assert.Equal(t, firstRetainedReplyCreateAt, st.Messages[1].CreateAt, "oldest retained reply should follow the gap")
	// Last retained reply is the newest in the thread.
	assert.Equal(t, int64(totalReplies), st.Messages[len(st.Messages)-1].CreateAt)
}

func TestNewSafeThread_DoesNotTruncateAtCapBoundary(t *testing.T) {
	// Exactly MaxThreadReplies+1 messages (root + MaxThreadReplies replies)
	// is the largest size that still fits without truncation.
	posts := make(map[string]*mmmodel.Post, MaxThreadReplies+1)
	order := make([]string, 0, MaxThreadReplies+1)
	posts["root"] = &mmmodel.Post{Id: "root", UserId: "u1", Message: "root", CreateAt: 0}
	order = append(order, "root")
	for i := 1; i <= MaxThreadReplies; i++ {
		id := "r" + strconv.Itoa(i)
		posts[id] = &mmmodel.Post{Id: id, UserId: "u1", Message: "x", CreateAt: int64(i)}
		order = append(order, id)
	}
	list := &mmmodel.PostList{Order: order, Posts: posts}

	st := NewSafeThread(list, "root", func(_ string) *SafeUser { return &SafeUser{Username: "alice"} })
	require.NotNil(t, st)
	assert.False(t, st.Truncated, "exactly MaxThreadReplies+1 messages must not trip truncation")
	assert.Equal(t, MaxThreadReplies+1, st.PostCount)
	require.Len(t, st.Messages, MaxThreadReplies+1)
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
	t.Run("renders posts separated by blank lines", func(t *testing.T) {
		st := &SafeThread{Messages: []SafePost{
			{User: SafeUser{Username: "alice", FirstName: "Alice", LastName: "Smith"}, Message: "hi"},
			{User: SafeUser{Id: "u2"}, Message: "fallback id"},
		}}
		assert.Equal(t, "@alice (Alice Smith): hi\n\nu2: fallback id", st.TranscriptDisplay())
	})
	t.Run("multi-line message bodies survive verbatim", func(t *testing.T) {
		// The blank-line separator means a multi-line post stays as one
		// readable block rather than blurring into the next post.
		st := &SafeThread{Messages: []SafePost{
			{User: SafeUser{Username: "alice"}, Message: "line 1\nline 2\nline 3"},
			{User: SafeUser{Username: "bob"}, Message: "reply"},
		}}
		assert.Equal(t, "@alice: line 1\nline 2\nline 3\n\n@bob: reply", st.TranscriptDisplay())
	})
	t.Run("trailing newline in final message preserved", func(t *testing.T) {
		// TrimSuffix removes only the loop-appended "\n\n" delimiter, so a
		// final message that legitimately ends in "\n" keeps that newline.
		st := &SafeThread{Messages: []SafePost{
			{User: SafeUser{Username: "alice"}, Message: "ends with newline\n"},
		}}
		assert.Equal(t, "@alice: ends with newline\n", st.TranscriptDisplay())
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
