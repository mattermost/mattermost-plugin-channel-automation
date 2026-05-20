import type {Action} from 'types';

jest.mock('client', () => ({
    createAutomation: jest.fn(),
    getAgentTools: jest.fn(),
    getAIBots: jest.fn(),
    getAutomation: jest.fn(),
    updateAutomation: jest.fn(),
}));

jest.mock('react-router-dom', () => ({
    useHistory: jest.fn(),
    useParams: jest.fn(),
    useRouteMatch: jest.fn(),
}), {virtual: true});

import {actionFormsToActions, actionToForm} from './automation_editor_view';

describe('automation editor ai_prompt mapping', () => {
    it('loads use_agent_system_prompt into the action form', () => {
        const action: Action = {
            id: 'ask',
            ai_prompt: {
                prompt: 'summarize',
                provider_type: 'agent',
                provider_id: 'agent-id',
                use_agent_system_prompt: true,
            },
        };

        expect(actionToForm(action).use_agent_system_prompt).toBe(true);
    });

    it('serializes use_agent_system_prompt when enabled', () => {
        const [action] = actionFormsToActions([{
            id: 'ask',
            type: 'ai_prompt',
            channel_id: '',
            reply_to_post_id: '',
            as_bot_id: '',
            body: '',
            system_prompt: '',
            use_agent_system_prompt: true,
            prompt: 'summarize',
            provider_id: 'agent-id',
            allowed_tool_refs: [],
            request_as: '',
            guardrail_channel_ids: [],
        }]);

        expect(action.ai_prompt?.use_agent_system_prompt).toBe(true);
    });

    it('omits use_agent_system_prompt when disabled', () => {
        const [action] = actionFormsToActions([{
            id: 'ask',
            type: 'ai_prompt',
            channel_id: '',
            reply_to_post_id: '',
            as_bot_id: '',
            body: '',
            system_prompt: '',
            use_agent_system_prompt: false,
            prompt: 'summarize',
            provider_id: 'agent-id',
            allowed_tool_refs: [],
            request_as: '',
            guardrail_channel_ids: [],
        }]);

        expect(action.ai_prompt).not.toHaveProperty('use_agent_system_prompt');
    });
});
