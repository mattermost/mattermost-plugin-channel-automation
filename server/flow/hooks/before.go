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
	if chID != "" {
		if !channelAllowed(ctx.AllowedCh, chID) {
			return fmt.Errorf("channel_id %q is not permitted by guardrails; allowed channel_ids: %s", chID, allowed)
		}
		return nil
	}
	name := stringArg(args, "channel_name")
	teamID := stringArg(args, "team_id")
	if name == "" || teamID == "" {
		return nil
	}
	ch, appErr := ctx.API.GetChannelByName(teamID, name, false)
	if appErr != nil {
		return nil
	}
	if ch != nil && ch.Id != "" && !channelAllowed(ctx.AllowedCh, ch.Id) {
		return fmt.Errorf("resolved channel %q (id %s) is not permitted by guardrails; allowed channel_ids: %s", name, ch.Id, allowed)
	}
	return nil
}

func beforeGetUserChannels(_ HookCtx, _ map[string]any) error {
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
