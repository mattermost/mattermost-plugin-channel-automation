package hooks

import (
	"encoding/json"
	"fmt"

	"github.com/mattermost/mattermost-plugin-agents/public/mcptool"
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
	return mustMarshal(out)
}

func afterGetTeamInfo(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	if ctx.TeamFromFlowErr != "" {
		return nil, fmt.Errorf("%s", ctx.TeamFromFlowErr)
	}
	var out mcptool.TeamInfoOutput
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("invalid get_team_info output: %w", err)
	}
	removed := 0
	keep := out.Teams[:0]
	for _, t := range out.Teams {
		if t == nil || t.Id != ctx.ExpectedTeamID {
			removed++
			continue
		}
		keep = append(keep, t)
	}
	out.Teams = keep
	if len(out.Teams) == 0 {
		return nil, fmt.Errorf("no team in get_team_info output matches this automation's team %q", ctx.ExpectedTeamID)
	}
	if removed > 0 {
		out.PluginAnnotations = append(out.PluginAnnotations,
			fmt.Sprintf("%d team candidate(s) were removed by automation team guardrails", removed))
	}
	return mustMarshal(out)
}

func afterGetTeamMembers(ctx HookCtx, output json.RawMessage) (json.RawMessage, error) {
	if ctx.TeamFromFlowErr != "" {
		return nil, fmt.Errorf("%s", ctx.TeamFromFlowErr)
	}
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
