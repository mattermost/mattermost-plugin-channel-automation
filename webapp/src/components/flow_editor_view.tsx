import {createFlow, getAgentTools, getAIBots, getFlow, updateFlow} from 'client';
import React, {useCallback, useEffect, useState} from 'react';
import {useHistory, useParams, useRouteMatch} from 'react-router-dom';
import type {TriggerFormState} from 'triggers';
import {getAllTriggerConfigs, getTriggerConfig} from 'triggers';

import 'triggers/channel_created';
import 'triggers/membership_changed';
import 'triggers/message_posted';
import 'triggers/schedule';
import type {AIBotInfo, AIToolInfo, Action, AllowedToolRef, TeamBotConfig} from 'types';
import {getTriggerType} from 'types';

interface ActionForm {
    id: string;
    type: string;
    channel_id: string;
    reply_to_post_id: string;
    as_bot_id: string;
    body: string;
    system_prompt: string;
    prompt: string;
    provider_id: string;
    allowed_tool_refs: AllowedToolRef[];
    execution_mode: string;
}

const styles = {
    formGroup: {
        marginBottom: 16,
    } as React.CSSProperties,
    label: {
        display: 'block',
        fontWeight: 600,
        marginBottom: 4,
    } as React.CSSProperties,
    input: {
        width: '100%',
        padding: '6px 8px',
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.2)',
        borderRadius: 4,
        fontSize: 14,
        boxSizing: 'border-box' as const,
        color: 'var(--center-channel-color)',
        backgroundColor: 'var(--center-channel-bg)',
    },
    textarea: {
        width: '100%',
        padding: '6px 8px',
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.2)',
        borderRadius: 4,
        fontSize: 14,
        minHeight: 80,
        boxSizing: 'border-box' as const,
        color: 'var(--center-channel-color)',
        backgroundColor: 'var(--center-channel-bg)',
    },
    select: {
        width: '100%',
        padding: '6px 8px',
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.2)',
        borderRadius: 4,
        fontSize: 14,
        color: 'var(--center-channel-color)',
        backgroundColor: 'var(--center-channel-bg)',
    } as React.CSSProperties,
    btnPrimary: {
        padding: '8px 16px',
        backgroundColor: 'var(--button-bg)',
        color: 'var(--button-color)',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 14,
        marginRight: 8,
    } as React.CSSProperties,
    btnSecondary: {
        padding: '8px 16px',
        backgroundColor: 'var(--center-channel-bg)',
        color: 'var(--center-channel-color)',
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.2)',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 14,
        marginRight: 8,
    } as React.CSSProperties,
    btnDanger: {
        padding: '4px 10px',
        backgroundColor: 'var(--error-text)',
        color: '#fff',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 13,
    } as React.CSSProperties,
    actionItem: {
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.15)',
        borderRadius: 4,
        padding: 12,
        marginBottom: 8,
    } as React.CSSProperties,
    actionHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: 8,
    } as React.CSSProperties,
    error: {
        color: 'var(--error-text)',
        marginBottom: 12,
    } as React.CSSProperties,
    warning: {
        padding: '8px 12px',
        borderRadius: 4,
        backgroundColor: 'rgba(var(--error-text-color-rgb, 210, 75, 78), 0.08)',
        color: 'var(--error-text)',
        fontSize: 13,
        marginBottom: 12,
    } as React.CSSProperties,
    header: {
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        marginBottom: 20,
    } as React.CSSProperties,
    actionId: {
        fontSize: 12,
        color: 'rgba(var(--center-channel-color-rgb), 0.56)',
        fontFamily: 'monospace',
        userSelect: 'all',
    } as React.CSSProperties,
    details: {
        marginTop: 8,
        marginBottom: 12,
    } as React.CSSProperties,
    summary: {
        fontSize: 13,
        color: 'rgba(var(--center-channel-color-rgb), 0.56)',
        cursor: 'pointer',
    } as React.CSSProperties,
    pre: {
        fontSize: 12,
        fontFamily: 'monospace',
        backgroundColor: 'rgba(var(--center-channel-color-rgb), 0.05)',
        borderRadius: 4,
        padding: 8,
        overflowX: 'auto',
        margin: '4px 0 0',
    } as React.CSSProperties,
    toolList: {
        border: '1px solid rgba(var(--center-channel-color-rgb), 0.15)',
        borderRadius: 4,
        maxHeight: 200,
        overflowY: 'auto',
        padding: 4,
    } as React.CSSProperties,
    toolItem: {
        display: 'flex',
        alignItems: 'flex-start',
        gap: 6,
        padding: '4px 8px',
        fontSize: 13,
    } as React.CSSProperties,
    toolDescription: {
        fontSize: 12,
        color: 'rgba(var(--center-channel-color-rgb), 0.56)',
    } as React.CSSProperties,
};

const allTriggers = getAllTriggerConfigs();
const defaultTriggerType = allTriggers[0]?.type ?? 'message_posted';

function toolRefKey(r: {server_origin?: string; name: string}): string {
    return JSON.stringify({server_origin: r.server_origin ?? '', name: r.name});
}

// Accept (string | AllowedToolRef)[] at runtime for backward compatibility with
// legacy JSON payloads where allowed_tools was a plain string array.
function normalizeAllowedToolsFromFlow(raw: unknown): AllowedToolRef[] {
    if (!raw || !Array.isArray(raw)) {
        return [];
    }
    const out: AllowedToolRef[] = [];
    for (const item of raw as Array<string | AllowedToolRef>) {
        if (typeof item === 'string') {
            if (item.trim() !== '') {
                out.push({server_origin: '', name: item.trim()});
            }
            continue;
        }
        if (item && typeof item === 'object' && typeof item.name === 'string' && item.name !== '') {
            out.push({server_origin: item.server_origin ?? '', name: item.name});
        }
    }
    return out;
}

function newActionForm(): ActionForm {
    return {id: '', type: 'send_message', channel_id: '', reply_to_post_id: '', as_bot_id: '', body: '', system_prompt: '', prompt: '', provider_id: '', allowed_tool_refs: [], execution_mode: ''};
}

function actionToForm(a: Action): ActionForm {
    if (a.ai_prompt) {
        return {
            id: a.id,
            type: 'ai_prompt',
            channel_id: '',
            reply_to_post_id: '',
            as_bot_id: '',
            body: '',
            system_prompt: a.ai_prompt.system_prompt ?? '',
            prompt: a.ai_prompt.prompt ?? '',
            provider_id: a.ai_prompt.provider_id ?? '',
            allowed_tool_refs: normalizeAllowedToolsFromFlow(a.ai_prompt.allowed_tools),
            execution_mode: a.ai_prompt.execution_mode ?? '',
        };
    }
    if (a.send_message) {
        return {
            id: a.id,
            type: 'send_message',
            channel_id: a.send_message.channel_id,
            reply_to_post_id: a.send_message.reply_to_post_id ?? '',
            as_bot_id: a.send_message.as_bot_id ?? '',
            body: a.send_message.body,
            system_prompt: '',
            prompt: '',
            provider_id: '',
            allowed_tool_refs: [],
            execution_mode: '',
        };
    }
    return {id: a.id, type: '', channel_id: '', reply_to_post_id: '', as_bot_id: '', body: '', system_prompt: '', prompt: '', provider_id: '', allowed_tool_refs: [], execution_mode: ''};
}

const hintStyle: React.CSSProperties = {fontSize: 13, color: 'rgba(var(--center-channel-color-rgb), 0.56)', margin: 0};

interface ToolSelectorProps {
    action: ActionForm;
    index: number;
    tools: AIToolInfo[] | undefined;
    onToggle: (index: number, tool: AIToolInfo, checked: boolean) => void;
}

const ToolSelector: React.FC<ToolSelectorProps> = ({action, index, tools, onToggle}) => {
    if (!action.provider_id) {
        return <p style={hintStyle}>{'Select an agent to see available tools.'}</p>;
    }
    if (tools === undefined) {
        return <p style={hintStyle}>{'Loading tools...'}</p>;
    }
    if (tools.length === 0) {
        return <p style={hintStyle}>{'No tools available for this agent.'}</p>;
    }
    const selected = new Set(action.allowed_tool_refs.map((r) => toolRefKey(r)));
    return (
        <div style={styles.toolList}>
            {tools.map((tool) => (
                <label
                    key={toolRefKey(tool)}
                    style={styles.toolItem}
                >
                    <input
                        type='checkbox'
                        checked={selected.has(toolRefKey(tool))}
                        onChange={(e) => onToggle(index, tool, e.target.checked)}
                    />
                    <span>
                        <strong>{tool.name}</strong>
                        {tool.server_origin ? (
                            <span style={styles.toolDescription}>{` \u2014 ${tool.server_origin}`}</span>
                        ) : null}
                        {tool.description ? (
                            <span style={styles.toolDescription}>{` \u2014 ${tool.description}`}</span>
                        ) : null}
                    </span>
                </label>
            ))}
        </div>
    );
};

const FlowEditorView: React.FC = () => {
    const {id: flowId} = useParams<{id?: string}>();
    const history = useHistory();
    const {url} = useRouteMatch();

    // Derive the workflows list URL by stripping the current route suffix
    const workflowsUrl = url.replace(/\/workflows\/.*$/, '/workflows');

    const [name, setName] = useState('');
    const [enabled, setEnabled] = useState(false);
    const [triggerType, setTriggerType] = useState(defaultTriggerType);
    const [triggerState, setTriggerState] = useState<TriggerFormState>(
        () => getTriggerConfig(defaultTriggerType)?.defaultFormState() ?? {},
    );
    const [actions, setActions] = useState<ActionForm[]>([]);
    const [teamBotTeamId, setTeamBotTeamId] = useState('');
    const [teamBotChannelIds, setTeamBotChannelIds] = useState('');
    const [agents, setAgents] = useState<AIBotInfo[]>([]);
    const [agentsError, setAgentsError] = useState<string | null>(null);
    const [agentTools, setAgentTools] = useState<Map<number, AIToolInfo[]>>(new Map());
    const [loading, setLoading] = useState(Boolean(flowId));
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const triggerConfig = getTriggerConfig(triggerType);

    useEffect(() => {
        let cancelled = false;
        getAIBots().then((bots) => {
            if (cancelled) {
                return;
            }
            setAgents(bots);
            if (bots.length === 0) {
                setAgentsError('No AI agents found. Please configure agents in the AI plugin.');
            }
        }).catch((err: unknown) => {
            if (cancelled) {
                return;
            }
            setAgents([]);
            const msg = err instanceof Error ? err.message : 'unknown error';
            setAgentsError(`Could not load AI agents: ${msg}`);
        });
        return () => {
            cancelled = true;
        };
    }, []);

    const fetchToolsForAction = useCallback((index: number, agentId: string) => {
        if (!agentId) {
            setAgentTools((prev) => {
                const next = new Map(prev);
                next.delete(index);
                return next;
            });
            return;
        }
        getAgentTools(agentId).then((tools) => {
            setAgentTools((prev) => {
                const next = new Map(prev);
                next.set(index, tools);
                return next;
            });
        }).catch((err: unknown) => {
            // eslint-disable-next-line no-console
            console.error('Failed to fetch agent tools:', err);
            setAgentTools((prev) => {
                const next = new Map(prev);
                next.set(index, []);
                return next;
            });
        });
    }, []);

    useEffect(() => {
        if (!flowId) {
            return undefined;
        }
        let cancelled = false;
        (async () => {
            try {
                const flow = await getFlow(flowId);
                if (cancelled) {
                    return;
                }
                setName(flow.name);
                setEnabled(flow.enabled);
                if (flow.team_bot_config) {
                    setTeamBotTeamId(flow.team_bot_config.team_id ?? '');
                    setTeamBotChannelIds((flow.team_bot_config.channel_ids ?? []).join(', '));
                }
                const tt = getTriggerType(flow.trigger);
                setTriggerType(tt);
                const config = getTriggerConfig(tt);
                setTriggerState(config?.fromTrigger(flow.trigger) ?? {});
                const forms = (flow.actions ?? []).map(actionToForm);
                setActions(forms);
                forms.forEach((a, idx) => {
                    if (a.type === 'ai_prompt' && a.provider_id) {
                        fetchToolsForAction(idx, a.provider_id);
                    }
                });
            } catch (err: unknown) {
                if (!cancelled) {
                    setError(err instanceof Error ? err.message : 'Failed to load flow');
                }
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [flowId, fetchToolsForAction]);

    const handleTriggerTypeChange = useCallback((newType: string) => {
        setTriggerType(newType);
        const config = getTriggerConfig(newType);
        setTriggerState(config?.defaultFormState() ?? {});
    }, []);

    const handleTriggerFieldChange = useCallback((field: string, value: string) => {
        setTriggerState((prev) => ({...prev, [field]: value}));
    }, []);

    const handleAddAction = useCallback(() => {
        setActions((prev) => [...prev, newActionForm()]);
    }, []);

    const handleRemoveAction = useCallback((index: number) => {
        setActions((prev) => prev.filter((_, i) => i !== index));
        setAgentTools((prev) => {
            const next = new Map<number, AIToolInfo[]>();
            for (const [k, v] of prev) {
                if (k < index) {
                    next.set(k, v);
                } else if (k > index) {
                    next.set(k - 1, v);
                }
            }
            return next;
        });
    }, []);

    const handleActionChange = useCallback((index: number, field: keyof ActionForm, value: string) => {
        setActions((prev) => prev.map((a, i) => (i === index ? {...a, [field]: value} : a)));
    }, []);

    const handleAgentChange = useCallback((index: number, agentId: string) => {
        handleActionChange(index, 'provider_id', agentId);
        fetchToolsForAction(index, agentId);
    }, [handleActionChange, fetchToolsForAction]);

    const handleToolToggle = useCallback((index: number, tool: AIToolInfo, checked: boolean) => {
        setActions((prev) => prev.map((a, i) => {
            if (i !== index) {
                return a;
            }
            const k = toolRefKey(tool);
            const ref: AllowedToolRef = {server_origin: tool.server_origin ?? '', name: tool.name};
            const next = a.allowed_tool_refs.filter((r) => toolRefKey(r) !== k);
            if (checked) {
                return {...a, allowed_tool_refs: [...next, ref]};
            }
            return {...a, allowed_tool_refs: next};
        }));
    }, []);

    const handleSave = useCallback(async () => {
        setSaving(true);
        setError(null);

        const trigger = triggerConfig?.toTrigger(triggerState) ?? {};

        let teamBotConfig: TeamBotConfig | undefined;
        if (teamBotTeamId.trim()) {
            const channelIds = teamBotChannelIds.split(',').map((s) => s.trim()).filter(Boolean);
            teamBotConfig = {team_id: teamBotTeamId.trim()};
            if (channelIds.length > 0) {
                teamBotConfig.channel_ids = channelIds;
            }
        }

        const data = {
            name,
            enabled,
            trigger,
            team_bot_config: teamBotConfig,
            actions: actions.map((a): Action => {
                if (a.type === 'ai_prompt') {
                    const action: Action = {
                        id: a.id,
                        ai_prompt: {
                            prompt: a.prompt,
                            provider_type: 'agent',
                            provider_id: a.provider_id,
                        },
                    };
                    if (a.system_prompt.trim() && action.ai_prompt) {
                        action.ai_prompt.system_prompt = a.system_prompt;
                    }
                    if (a.allowed_tool_refs.length > 0 && action.ai_prompt) {
                        action.ai_prompt.allowed_tools = a.allowed_tool_refs;
                    }
                    if (a.execution_mode && action.ai_prompt) {
                        action.ai_prompt.execution_mode = a.execution_mode;
                    }
                    return action;
                }
                const action: Action = {
                    id: a.id,
                    send_message: {
                        channel_id: a.channel_id,
                        body: a.body,
                    },
                };
                if (a.reply_to_post_id && action.send_message) {
                    action.send_message.reply_to_post_id = a.reply_to_post_id;
                }
                if (a.as_bot_id && action.send_message) {
                    action.send_message.as_bot_id = a.as_bot_id;
                }
                return action;
            }),
        };
        try {
            if (flowId) {
                await updateFlow(flowId, data);
            } else {
                await createFlow(data);
            }
            history.push(workflowsUrl);
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to save flow');
            setSaving(false);
        }
    }, [flowId, name, enabled, triggerType, triggerState, triggerConfig, actions, teamBotTeamId, teamBotChannelIds, history, workflowsUrl]);

    if (loading) {
        return <p>{'Loading...'}</p>;
    }

    return (
        <div>
            <div style={styles.header}>
                <button
                    style={styles.btnSecondary}
                    onClick={() => history.push(workflowsUrl)}
                >
                    {'\u2190 Back'}
                </button>
                <h2>{flowId ? 'Edit Flow' : 'Create Flow'}</h2>
            </div>
            {error && <p style={styles.error}>{error}</p>}
            <div style={styles.formGroup}>
                <label
                    htmlFor='flow-name'
                    style={styles.label}
                >{'Name'}</label>
                <input
                    id='flow-name'
                    style={styles.input}
                    type='text'
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                />
            </div>
            <div style={styles.formGroup}>
                <label style={styles.label}>
                    <input
                        type='checkbox'
                        checked={enabled}
                        onChange={(e) => setEnabled(e.target.checked)}
                    />
                    {' Enabled'}
                </label>
            </div>
            <h3>{'Team Bot (optional)'}</h3>
            <p style={hintStyle}>
                {'Configure a team-scoped bot to run AI actions with restricted permissions (public channels only). Leave Team ID empty to skip.'}
            </p>
            <div style={styles.formGroup}>
                <label
                    htmlFor='team-bot-team-id'
                    style={styles.label}
                >{'Team ID'}</label>
                <input
                    id='team-bot-team-id'
                    style={styles.input}
                    type='text'
                    placeholder='Team ID for the automation bot'
                    value={teamBotTeamId}
                    onChange={(e) => setTeamBotTeamId(e.target.value)}
                />
            </div>
            {teamBotTeamId.trim() && (
                <div style={styles.formGroup}>
                    <label
                        htmlFor='team-bot-channel-ids'
                        style={styles.label}
                    >{'Public Channel IDs (comma-separated)'}</label>
                    <input
                        id='team-bot-channel-ids'
                        style={styles.input}
                        type='text'
                        placeholder='ch-id-1, ch-id-2'
                        value={teamBotChannelIds}
                        onChange={(e) => setTeamBotChannelIds(e.target.value)}
                    />
                    <p style={hintStyle}>{'Channels the bot will be added to before executing.'}</p>
                </div>
            )}
            <h3>{'Trigger'}</h3>
            {triggerConfig && (
                <details style={styles.details}>
                    <summary style={styles.summary}>{'Available template variables'}</summary>
                    <pre style={styles.pre}>{triggerConfig.templateVariables}</pre>
                </details>
            )}
            <div style={styles.formGroup}>
                <label
                    htmlFor='trigger-type'
                    style={styles.label}
                >{'Type'}</label>
                <select
                    id='trigger-type'
                    style={styles.select}
                    value={triggerType}
                    onChange={(e) => handleTriggerTypeChange(e.target.value)}
                >
                    {allTriggers.map((t) => (
                        <option
                            key={t.type}
                            value={t.type}
                        >
                            {t.label}
                        </option>
                    ))}
                </select>
            </div>
            {triggerConfig?.renderFields(triggerState, handleTriggerFieldChange, styles)}
            <h3>{'Actions'}</h3>
            {actions.map((action, index) => (
                <div
                    key={index}
                    style={styles.actionItem}
                >
                    <div style={styles.actionHeader}>
                        <strong>{`Action ${index + 1}`}</strong>
                        <button
                            style={styles.btnDanger}
                            onClick={() => handleRemoveAction(index)}
                        >
                            {'Remove'}
                        </button>
                    </div>
                    <div style={styles.formGroup}>
                        <label
                            htmlFor={`action-${index}-id`}
                            style={styles.label}
                        >{'ID'}</label>
                        <input
                            id={`action-${index}-id`}
                            style={styles.input}
                            type='text'
                            placeholder='e.g. send-greeting'
                            value={action.id}
                            onChange={(e) => handleActionChange(index, 'id', e.target.value)}
                        />
                    </div>
                    <div style={styles.formGroup}>
                        <label
                            htmlFor={`action-${index}-type`}
                            style={styles.label}
                        >{'Type'}</label>
                        <select
                            id={`action-${index}-type`}
                            style={styles.select}
                            value={action.type}
                            onChange={(e) => handleActionChange(index, 'type', e.target.value)}
                        >
                            <option value='send_message'>{'send_message'}</option>
                            <option value='ai_prompt'>{'ai_prompt'}</option>
                        </select>
                    </div>
                    {action.type === 'send_message' && (
                        <>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-channel-id`}
                                    style={styles.label}
                                >{'Channel ID'}</label>
                                <input
                                    id={`action-${index}-channel-id`}
                                    style={styles.input}
                                    type='text'
                                    value={action.channel_id}
                                    onChange={(e) => handleActionChange(index, 'channel_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-reply-to-post-id`}
                                    style={styles.label}
                                >{'Reply to Post ID (Go template, optional)'}</label>
                                <input
                                    id={`action-${index}-reply-to-post-id`}
                                    style={styles.input}
                                    type='text'
                                    value={action.reply_to_post_id}
                                    placeholder={'e.g. {{.Trigger.Post.ThreadId}}'}
                                    onChange={(e) => handleActionChange(index, 'reply_to_post_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-as-bot-id`}
                                    style={styles.label}
                                >{'Bot User ID (optional, overrides default bot)'}</label>
                                <input
                                    id={`action-${index}-as-bot-id`}
                                    style={styles.input}
                                    type='text'
                                    value={action.as_bot_id}
                                    placeholder={'Bot user ID'}
                                    onChange={(e) => handleActionChange(index, 'as_bot_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-body`}
                                    style={styles.label}
                                >{'Body (Go template)'}</label>
                                <textarea
                                    id={`action-${index}-body`}
                                    style={styles.textarea}
                                    value={action.body}
                                    onChange={(e) => handleActionChange(index, 'body', e.target.value)}
                                />
                            </div>
                            <details style={styles.details}>
                                <summary style={styles.summary}>{'Output of this action'}</summary>
                                <pre style={styles.pre}>{`{
  "PostID": "string",
  "ChannelID": "string",
  "Message": "string"
}

Usage: {{(index .Steps "${action.id || '<action_id>'}").Message}}`}</pre>
                            </details>
                        </>
                    )}
                    {action.type === 'ai_prompt' && (
                        <>
                            {agentsError && <p style={styles.warning}>{agentsError}</p>}
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-agent`}
                                    style={styles.label}
                                >{'Agent'}</label>
                                <select
                                    id={`action-${index}-agent`}
                                    style={styles.select}
                                    value={action.provider_id}
                                    onChange={(e) => handleAgentChange(index, e.target.value)}
                                >
                                    <option value=''>{'Select an agent...'}</option>
                                    {agents.map((bot) => (
                                        <option
                                            key={bot.id}
                                            value={bot.id}
                                        >
                                            {`${bot.display_name} (@${bot.username})`}
                                        </option>
                                    ))}
                                </select>
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Execution Mode'}</label>
                                <div style={{display: 'flex', gap: 16}}>
                                    <label>
                                        <input
                                            type='radio'
                                            name={`action-${index}-exec-mode`}
                                            value=''
                                            checked={!action.execution_mode || action.execution_mode === 'creator'}
                                            onChange={() => handleActionChange(index, 'execution_mode', '')}
                                        />
                                        {' Creator'}
                                    </label>
                                    <label>
                                        <input
                                            type='radio'
                                            name={`action-${index}-exec-mode`}
                                            value='team_bot'
                                            checked={action.execution_mode === 'team_bot'}
                                            disabled={!teamBotTeamId.trim()}
                                            onChange={() => handleActionChange(index, 'execution_mode', 'team_bot')}
                                        />
                                        {' Team Bot'}
                                    </label>
                                </div>
                                <p style={hintStyle}>
                                    {action.execution_mode === 'team_bot' ?
                                        'Runs as the team automation bot (public channels only).' :
                                        'Runs with your identity and MCP connections.'}
                                </p>
                            </div>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-system-prompt`}
                                    style={styles.label}
                                >{'System Prompt (Go template, optional)'}</label>
                                <textarea
                                    id={`action-${index}-system-prompt`}
                                    style={styles.textarea}
                                    placeholder={'e.g. You are a helpful assistant.'}
                                    value={action.system_prompt}
                                    onChange={(e) => handleActionChange(index, 'system_prompt', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label
                                    htmlFor={`action-${index}-prompt`}
                                    style={styles.label}
                                >{'Prompt (Go template)'}</label>
                                <textarea
                                    id={`action-${index}-prompt`}
                                    style={styles.textarea}
                                    value={action.prompt}
                                    onChange={(e) => handleActionChange(index, 'prompt', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Allowed Tools (optional)'}</label>
                                <ToolSelector
                                    action={action}
                                    index={index}
                                    tools={agentTools.get(index)}
                                    onToggle={handleToolToggle}
                                />
                            </div>
                            <details style={styles.details}>
                                <summary style={styles.summary}>{'Output of this action'}</summary>
                                <pre style={styles.pre}>{`{
  "Message": "string"
}

Usage: {{(index .Steps "${action.id || '<action_id>'}").Message}}`}</pre>
                            </details>
                        </>
                    )}
                </div>
            ))}
            <div style={{marginBottom: 20}}>
                <button
                    style={styles.btnSecondary}
                    onClick={handleAddAction}
                >
                    {'+ Add Action'}
                </button>
            </div>
            <div>
                <button
                    style={styles.btnPrimary}
                    onClick={handleSave}
                    disabled={saving}
                >
                    {saving ? 'Saving...' : 'Save'}
                </button>
                <button
                    style={styles.btnSecondary}
                    onClick={() => history.push(workflowsUrl)}
                >
                    {'Cancel'}
                </button>
            </div>
        </div>
    );
};

export default FlowEditorView;
