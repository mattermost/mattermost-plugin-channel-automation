export interface MessagePostedTriggerParams {
    channel_id: string;
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

export interface Trigger {
    message_posted?: MessagePostedTriggerParams;
    schedule?: ScheduleTriggerParams;
    membership_changed?: MembershipChangedTriggerParams;
    channel_created?: ChannelCreatedTriggerParams;
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
    execution_mode?: string; // "team_bot" | "creator"
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

export interface TeamBotConfig {
    team_id: string;
    channel_ids?: string[];
}

export interface Flow {
    id: string;
    name: string;
    enabled: boolean;
    trigger: Trigger;
    actions: Action[];
    team_bot_config?: TeamBotConfig;
    created_at: number;
    updated_at: number;
    created_by: string;
}
