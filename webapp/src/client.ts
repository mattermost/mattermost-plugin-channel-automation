import {Client4} from '@mattermost/client';

import type {AIBotInfo, AIToolInfo, Flow} from 'types';

const BASE_URL = '/plugins/com.mattermost.channel-automation/api/v1';

const client = new Client4();

async function doFetch<T>(url: string, options: {method?: string; body?: string} = {}): Promise<T> {
    const fetchOptions = client.getOptions({
        method: options.method || 'get',
        headers: options.body ? {'Content-Type': 'application/json'} : {},
        body: options.body,
    });
    const resp = await fetch(url, fetchOptions);
    if (!resp.ok) {
        let message = resp.statusText;
        try {
            const text = await resp.text();
            try {
                const body = JSON.parse(text);
                if (body && body.error) {
                    message = body.error;
                }
            } catch {
                // Not JSON — use the plain text response body directly
                if (text.trim()) {
                    message = text.trim();
                }
            }
        } catch {
            // ignore read errors
        }
        throw new Error(message);
    }
    if (resp.status === 204) {
        return undefined as unknown as T;
    }
    return resp.json() as Promise<T>;
}

export async function getFlows(): Promise<Flow[]> {
    const flows = await doFetch<Flow[] | null>(`${BASE_URL}/flows`);
    return flows ?? [];
}

export async function getFlow(id: string): Promise<Flow> {
    return doFetch<Flow>(`${BASE_URL}/flows/${id}`);
}

export async function createFlow(data: Partial<Flow>): Promise<Flow> {
    return doFetch<Flow>(`${BASE_URL}/flows`, {
        method: 'POST',
        body: JSON.stringify(data),
    });
}

export async function updateFlow(id: string, data: Partial<Flow>): Promise<Flow> {
    return doFetch<Flow>(`${BASE_URL}/flows/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    });
}

export async function deleteFlow(id: string): Promise<void> {
    await doFetch<void>(`${BASE_URL}/flows/${id}`, {
        method: 'DELETE',
    });
}

export async function getAIBots(): Promise<AIBotInfo[]> {
    const resp = await doFetch<{bots: AIBotInfo[]}>('/plugins/mattermost-ai/ai_bots');
    return resp.bots ?? [];
}

export async function getAgentTools(agentId: string): Promise<AIToolInfo[]> {
    const tools = await doFetch<AIToolInfo[] | null>(`${BASE_URL}/agents/${agentId}/tools`);
    return tools ?? [];
}

export interface TeamBotInfo {
    bot_user_id: string;
    team_id: string;
    username: string;
}

export async function getTeamBot(teamId: string): Promise<TeamBotInfo> {
    return doFetch<TeamBotInfo>(`${BASE_URL}/teams/${teamId}/bot`);
}

export interface ClientConfig {
    enable_ui: boolean;
}

export async function getConfig(): Promise<ClientConfig> {
    return doFetch<ClientConfig>(`${BASE_URL}/config`);
}
