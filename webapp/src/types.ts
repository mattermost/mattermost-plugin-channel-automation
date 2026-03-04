export interface MessagePostedTriggerParams {
    channel_id: string;
}

export interface ScheduleTriggerParams {
    channel_id: string;
    interval: string;
    start_at?: number;
}

export interface Trigger {
    message_posted?: MessagePostedTriggerParams;
    schedule?: ScheduleTriggerParams;
}

export function getTriggerType(trigger: Trigger): string {
    if (trigger.message_posted) {
        return 'message_posted';
    }
    if (trigger.schedule) {
        return 'schedule';
    }
    return '';
}

export interface SendMessageActionParams {
    channel_id: string;
    reply_to_post_id?: string;
    body: string;
}

export interface AIPromptActionParams {
    prompt: string;
    provider_type: string;
    provider_id: string;
}

export interface Action {
    id: string;
    name: string;
    send_message?: SendMessageActionParams;
    ai_prompt?: AIPromptActionParams;
}

export interface AIBotInfo {
    id: string;
    displayName: string;
    username: string;
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
