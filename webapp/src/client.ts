import {Client4} from '@mattermost/client';

import type {AIBotInfo, AIToolInfo, Automation} from 'types';

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

export async function getAutomations(): Promise<Automation[]> {
    const automations = await doFetch<Automation[] | null>(`${BASE_URL}/automations`);
    return automations ?? [];
}

export async function getAutomation(id: string): Promise<Automation> {
    return doFetch<Automation>(`${BASE_URL}/automations/${id}`);
}

export async function createAutomation(data: Partial<Automation>): Promise<Automation> {
    return doFetch<Automation>(`${BASE_URL}/automations`, {
        method: 'POST',
        body: JSON.stringify(data),
    });
}

export async function updateAutomation(id: string, data: Partial<Automation>): Promise<Automation> {
    return doFetch<Automation>(`${BASE_URL}/automations/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
    });
}

export async function deleteAutomation(id: string): Promise<void> {
    await doFetch<void>(`${BASE_URL}/automations/${id}`, {
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

export interface ClientConfig {
    enable_ui: boolean;
}

export async function getConfig(): Promise<ClientConfig> {
    return doFetch<ClientConfig>(`${BASE_URL}/config`);
}
