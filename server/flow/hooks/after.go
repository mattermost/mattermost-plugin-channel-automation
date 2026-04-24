package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost-plugin-agents/public/mcptool"
	mmmodel "github.com/mattermost/mattermost/server/public/model"
)

func afterSearchPosts(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.SearchPostsOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid search_posts output: %w", err)
	}
	removedSem := filterSearchResults(&out.SemanticResults, ctx.AllowedCh)
	removedKw := filterSearchResults(&out.KeywordResults, ctx.AllowedCh)
	if removedSem+removedKw > 0 {
		out.PluginAnnotations = append(out.PluginAnnotations,
			fmt.Sprintf("%d search_posts result(s) were removed by channel guardrails (%d semantic, %d keyword)",
				removedSem+removedKw, removedSem, removedKw))
	}
	return mustMarshal(out)
}

func filterSearchResults(rows *[]mcptool.SearchPostResult, allowed map[string]struct{}) int {
	if rows == nil {
		return 0
	}
	removed := 0
	keep := (*rows)[:0]
	for _, r := range *rows {
		if r.Post == nil || !channelAllowed(allowed, r.Post.ChannelId) {
			removed++
			continue
		}
		keep = append(keep, r)
	}
	*rows = keep
	return removed
}

func afterGetChannelInfo(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.ChannelInfoOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_channel_info output: %w", err)
	}
	removed := 0
	keep := out.Channels[:0]
	for _, ch := range out.Channels {
		if ch == nil || !channelAllowed(ctx.AllowedCh, ch.Id) {
			removed++
			continue
		}
		keep = append(keep, ch)
	}
	out.Channels = keep
	keepCh, keepTeams := keysFromChannels(out.Channels)
	pruneMap(out.TeamByID, keepTeams)
	pruneMap(out.MemberCountByChannelID, keepCh)
	pruneMap(out.ChannelRoleByID, keepCh)
	if removed > 0 {
		out.PluginAnnotations = append(out.PluginAnnotations,
			fmt.Sprintf("%d channel(s) were removed by channel guardrails", removed))
	}
	return mustMarshal(out)
}

func afterGetUserChannels(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.UserChannelsOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_user_channels output: %w", err)
	}
	before := len(out.Channels)
	keep := out.Channels[:0]
	for _, ch := range out.Channels {
		if ch == nil || !channelAllowed(ctx.AllowedCh, ch.Id) {
			continue
		}
		keep = append(keep, ch)
	}
	out.Channels = keep
	_, keepTeams := keysFromChannels(out.Channels)
	pruneMap(out.TeamInfoByID, keepTeams)
	removed := before - len(out.Channels)
	if removed > 0 {
		out.PluginAnnotations = append(out.PluginAnnotations,
			fmt.Sprintf("%d channel(s) were removed by channel guardrails (page_info counts reflect the upstream page before filtering)", removed))
	}
	return mustMarshal(out)
}

func afterReadChannel(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.ReadChannelOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid read_channel output: %w", err)
	}
	if out.Channel == nil || out.Channel.Id == "" {
		return nil, fmt.Errorf("read_channel output missing channel")
	}
	if !channelAllowed(ctx.AllowedCh, out.Channel.Id) {
		return nil, fmt.Errorf("channel %q is not permitted by guardrails; allowed channel_ids: %s",
			out.Channel.Id, formatAllowedChannels(ctx.Guardrails))
	}
	return mustMarshal(out)
}

func afterGetChannelMembers(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.ChannelMembersOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_channel_members output: %w", err)
	}
	if out.Channel == nil || out.Channel.Id == "" {
		return nil, fmt.Errorf("get_channel_members output missing channel")
	}
	if !channelAllowed(ctx.AllowedCh, out.Channel.Id) {
		return nil, fmt.Errorf("channel %q is not permitted by guardrails; allowed channel_ids: %s",
			out.Channel.Id, formatAllowedChannels(ctx.Guardrails))
	}
	return mustMarshal(out)
}

func afterReadPost(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	var out mcptool.ReadPostOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid read_post output: %w", err)
	}
	if len(out.Posts) == 0 {
		return output, nil
	}
	first := out.Posts[0]
	if first == nil || first.ChannelId == "" {
		return nil, fmt.Errorf("read_post output missing post channel")
	}
	if !channelAllowed(ctx.AllowedCh, first.ChannelId) {
		return nil, fmt.Errorf("post channel %q is not permitted by guardrails; allowed channel_ids: %s",
			first.ChannelId, formatAllowedChannels(ctx.Guardrails))
	}
	// Defensive: ensure all posts share the same channel as the first.
	for _, p := range out.Posts[1:] {
		if p == nil {
			continue
		}
		if p.ChannelId != first.ChannelId {
			return nil, fmt.Errorf("read_post output spans multiple channels (%q and %q); guardrails require a single channel",
				first.ChannelId, p.ChannelId)
		}
	}
	return mustMarshal(out)
}

func afterGetTeamInfo(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	if len(ctx.AllowedTeams) == 0 {
		return nil, fmt.Errorf("no team is permitted by guardrails for this automation")
	}
	var out mcptool.TeamInfoOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_team_info output: %w", err)
	}
	allowed := formatAllowedTeams(ctx.AllowedTeams)
	removed := 0
	keep := out.Teams[:0]
	for _, t := range out.Teams {
		if t == nil {
			removed++
			continue
		}
		if _, ok := ctx.AllowedTeams[t.Id]; !ok {
			removed++
			continue
		}
		keep = append(keep, t)
	}
	out.Teams = keep
	if len(out.Teams) == 0 {
		return nil, fmt.Errorf("no team in get_team_info output is permitted by guardrails; allowed team_ids: %s", allowed)
	}
	if removed > 0 {
		out.PluginAnnotations = append(out.PluginAnnotations,
			fmt.Sprintf("%d team candidate(s) were removed by guardrails", removed))
	}
	return mustMarshal(out)
}

func afterGetTeamMembers(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	if len(ctx.AllowedTeams) == 0 {
		return nil, fmt.Errorf("no team is permitted by guardrails for this automation")
	}
	// Note: TeamMembersOutput intentionally has no team identifier — only
	// per-row user + scheme flags. The team_id is enforced on the request
	// side by beforeGetTeamMembers, which is the security boundary for this
	// tool. There is nothing in the response to cross-check against
	// ctx.AllowedTeams, so we only validate that the body is well-formed.
	var out mcptool.TeamMembersOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_team_members output: %w", err)
	}
	return mustMarshal(out)
}

func mustMarshal(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal output: %w", err)
	}
	return b, nil
}

// pruneMap removes any entry from m whose key is not in keep. A nil map is
// left untouched.
func pruneMap[V any](m map[string]V, keep map[string]struct{}) {
	if m == nil {
		return
	}
	for k := range m {
		if _, ok := keep[k]; !ok {
			delete(m, k)
		}
	}
}

// keysFromChannels returns the set of channel IDs and the set of team IDs
// referenced by the given channels. nil entries are skipped.
func keysFromChannels(chs []*mmmodel.Channel) (map[string]struct{}, map[string]struct{}) {
	keepCh := make(map[string]struct{}, len(chs))
	keepTeams := make(map[string]struct{}, len(chs))
	for _, ch := range chs {
		if ch == nil {
			continue
		}
		if ch.Id != "" {
			keepCh[ch.Id] = struct{}{}
		}
		if ch.TeamId != "" {
			keepTeams[ch.TeamId] = struct{}{}
		}
	}
	return keepCh, keepTeams
}
