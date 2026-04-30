package hooks

import (
	"fmt"
)

func beforeSearchPosts(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	cid := stringArg(args, "channel_id")
	if cid == "" {
		return fmt.Errorf("search_posts requires channel_id when channel guardrails are active; allowed channel_ids: %s", allowed)
	}
	if !channelAllowed(ctx.AllowedCh, cid) {
		return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", cid, allowed)
	}
	return nil
}

func beforeGetChannelInfo(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	chID := stringArg(args, "channel_id")
	if chID == "" {
		return fmt.Errorf("get_channel_info requires channel_id when channel guardrails are active; allowed channel_ids: %s", allowed)
	}
	if !channelAllowed(ctx.AllowedCh, chID) {
		return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", chID, allowed)
	}
	return nil
}

func beforeGetUserChannels(_ HookCtx, _ map[string]any) error {
	return fmt.Errorf("get_user_channels is not permitted when channel guardrails are configured")
}

func beforeReadPost(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	pid := stringArg(args, "post_id")
	if pid == "" {
		return fmt.Errorf("read_post requires post_id; allowed channel_ids: %s", allowed)
	}
	p, appErr := ctx.API.GetPost(pid)
	if appErr != nil || p == nil {
		return fmt.Errorf("read_post: cannot resolve post %q", pid)
	}
	if !channelAllowed(ctx.AllowedCh, p.ChannelId) {
		return fmt.Errorf("post channel %q is not permitted by guardrails; allowed channel_ids: %s", p.ChannelId, allowed)
	}
	return nil
}

func beforeReadChannel(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	cid := stringArg(args, "channel_id")
	if cid == "" {
		return fmt.Errorf("read_channel requires channel_id; allowed channel_ids: %s", allowed)
	}
	if !channelAllowed(ctx.AllowedCh, cid) {
		return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", cid, allowed)
	}
	return nil
}

func beforeAddUserToChannel(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	cid := stringArg(args, "channel_id")
	if cid == "" {
		return fmt.Errorf("add_user_to_channel requires channel_id; allowed channel_ids: %s", allowed)
	}
	if !channelAllowed(ctx.AllowedCh, cid) {
		return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", cid, allowed)
	}
	return nil
}

func beforeGetChannelMembers(ctx HookCtx, args map[string]any) error {
	allowed := formatAllowedChannels(ctx.Guardrails)
	cid := stringArg(args, "channel_id")
	if cid == "" {
		return fmt.Errorf("get_channel_members requires channel_id; allowed channel_ids: %s", allowed)
	}
	if !channelAllowed(ctx.AllowedCh, cid) {
		return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", cid, allowed)
	}
	return nil
}

func beforeCreateChannel(ctx HookCtx, args map[string]any) error {
	if len(ctx.AllowedTeams) == 0 {
		return fmt.Errorf("no team is permitted by guardrails for this automation")
	}
	allowed := formatAllowedTeams(ctx.AllowedTeams)
	tid := stringArg(args, "team_id")
	if tid == "" {
		return fmt.Errorf("create_channel requires team_id when channel guardrails are active; allowed team_ids: %s", allowed)
	}
	if _, ok := ctx.AllowedTeams[tid]; !ok {
		return fmt.Errorf("team_id %q is not permitted by guardrails; allowed team_ids: %s", tid, allowed)
	}
	return nil
}

func beforeGetTeamInfo(ctx HookCtx, args map[string]any) error {
	if len(ctx.AllowedTeams) == 0 {
		return fmt.Errorf("no team is permitted by guardrails for this automation")
	}
	allowed := formatAllowedTeams(ctx.AllowedTeams)
	tid := stringArg(args, "team_id")
	if tid == "" {
		return fmt.Errorf("get_team_info requires team_id when channel guardrails are active; allowed team_ids: %s", allowed)
	}
	if _, ok := ctx.AllowedTeams[tid]; !ok {
		return fmt.Errorf("team_id %q is not permitted by guardrails; allowed team_ids: %s", tid, allowed)
	}
	return nil
}

func beforeGetTeamMembers(ctx HookCtx, args map[string]any) error {
	if len(ctx.AllowedTeams) == 0 {
		return fmt.Errorf("no team is permitted by guardrails for this automation")
	}
	allowed := formatAllowedTeams(ctx.AllowedTeams)
	tid := stringArg(args, "team_id")
	if tid == "" {
		return fmt.Errorf("get_team_members requires team_id; allowed team_ids: %s", allowed)
	}
	if _, ok := ctx.AllowedTeams[tid]; !ok {
		return fmt.Errorf("team_id %q is not permitted by guardrails; allowed team_ids: %s", tid, allowed)
	}
	return nil
}
