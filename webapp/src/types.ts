export interface MessagePostedTriggerParams {
    channel_id: string;
    include_thread_replies?: boolean; // when omitted/false, thread replies do not trigger the flow
}

export interface ScheduleTriggerParams {
    channel_id: string;
    interval: string;
    start_at?: number;
}

export interface MembershipChangedTriggerParams {
    channel_id: string;
    action?: string; // "joined", "left", or undefined (both)
}

export interface ChannelCreatedTriggerParams {
    [key: string]: never;
}

export interface UserJoinedTeamTriggerParams {
    team_id: string;
    user_type?: string; // "user", "guest", or undefined (both)
}

export interface Trigger {
    message_posted?: MessagePostedTriggerParams;
    schedule?: ScheduleTriggerParams;
    membership_changed?: MembershipChangedTriggerParams;
    channel_created?: ChannelCreatedTriggerParams;
    user_joined_team?: UserJoinedTeamTriggerParams;
}

export function getTriggerType(trigger: Trigger): string {
    if (trigger.message_posted) {
        return 'message_posted';
    }
    if (trigger.schedule) {
        return 'schedule';
    }
    if (trigger.membership_changed) {
        return 'membership_changed';
    }
    if (trigger.channel_created) {
        return 'channel_created';
    }
    if (trigger.user_joined_team) {
        return 'user_joined_team';
    }
    return '';
}

export interface SendMessageActionParams {
    channel_id: string;
    reply_to_post_id?: string;
    as_bot_id?: string;
    body: string;
}

/** Matches AI bridge allowed_tools entries (server_origin + name from agent tools discovery). */
export interface AllowedToolRef {
    server_origin: string;
    name: string;
}

export interface AIPromptActionParams {
    system_prompt?: string;
    prompt: string;
    provider_type: string;
    provider_id: string;
    allowed_tools?: AllowedToolRef[];
}

export interface Action {
    id: string;
    send_message?: SendMessageActionParams;
    ai_prompt?: AIPromptActionParams;
}

export interface AIBotInfo {
    id: string;
    display_name: string;
    username: string;
}

export interface AIToolInfo {
    name: string;
    description: string;
    server_origin?: string;
}

export interface Flow {
    id: string;
    name: string;
    enabled: boolean;
    trigger: Trigger;
    actions: Action[];
    created_at: number;
    updated_at: number;
    created_by: string;
}
