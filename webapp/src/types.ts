export interface Trigger {
    type: string;
    channel_id?: string;
    interval?: string;
    start_at?: number;
}

export interface Action {
    id: string;
    name: string;
    type: string;
    channel_id: string;
    reply_to_post_id?: string;
    body: string;
    config?: Record<string, string>;
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
