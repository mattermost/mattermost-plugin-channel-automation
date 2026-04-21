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

func beforeReadPost(_ HookCtx, _ map[string]any) error {
	return nil
}

func beforeGetTeamInfo(ctx HookCtx, args map[string]any) error {
	if ctx.TeamFromFlowErr != "" {
		return fmt.Errorf("%s", ctx.TeamFromFlowErr)
	}
	tid := stringArg(args, "team_id")
	if tid == "" {
		return fmt.Errorf("get_team_info requires team_id when channel guardrails are active; automation team_id: %s", ctx.ExpectedTeamID)
	}
	if tid != ctx.ExpectedTeamID {
		return fmt.Errorf("team_id %q does not match this automation's team %q", tid, ctx.ExpectedTeamID)
	}
	return nil
}

func beforeGetTeamMembers(ctx HookCtx, args map[string]any) error {
	if ctx.TeamFromFlowErr != "" {
		return fmt.Errorf("%s", ctx.TeamFromFlowErr)
	}
	tid := stringArg(args, "team_id")
	if tid == "" {
		return fmt.Errorf("get_team_members requires team_id; automation team_id: %s", ctx.ExpectedTeamID)
	}
	if tid != ctx.ExpectedTeamID {
		return fmt.Errorf("team_id %q does not match this automation's team %q", tid, ctx.ExpectedTeamID)
	}
	return nil
}
