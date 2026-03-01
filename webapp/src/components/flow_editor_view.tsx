import {createFlow, getFlow, updateFlow} from 'client';
import React, {useCallback, useEffect, useState} from 'react';

import type {Action} from 'types';

interface Props {
    flowId: string | null;
    onBack: () => void;
}

interface ActionForm {
    id: string;
    name: string;
    type: string;
    channel_id: string;
    reply_to_post_id: string;
    body: string;
    prompt: string;
    provider_type: string;
    provider_id: string;
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
};

function newActionForm(): ActionForm {
    return {id: '', name: '', type: 'send_message', channel_id: '', reply_to_post_id: '', body: '', prompt: '', provider_type: 'agent', provider_id: ''};
}

function actionToForm(a: Action): ActionForm {
    if (a.type === 'ai_prompt') {
        return {
            id: a.id,
            name: a.name,
            type: a.type,
            channel_id: '',
            body: '',
            prompt: a.config?.prompt ?? '',
            provider_type: a.config?.provider_type ?? 'agent',
            provider_id: a.config?.provider_id ?? '',
        };
    }
    return {id: a.id, name: a.name, type: a.type, channel_id: a.channel_id, reply_to_post_id: a.reply_to_post_id ?? '', body: a.body, prompt: '', provider_type: 'agent', provider_id: ''};
}

const FlowEditorView: React.FC<Props> = ({flowId, onBack}) => {
    const [name, setName] = useState('');
    const [enabled, setEnabled] = useState(false);
    const [triggerType, setTriggerType] = useState('message_posted');
    const [triggerChannelId, setTriggerChannelId] = useState('');
    const [actions, setActions] = useState<ActionForm[]>([]);
    const [loading, setLoading] = useState(Boolean(flowId));
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState<string | null>(null);

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
                setTriggerType(flow.trigger.type);
                setTriggerChannelId(flow.trigger.channel_id);
                setActions((flow.actions ?? []).map(actionToForm));
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
    }, [flowId]);

    const handleAddAction = useCallback(() => {
        setActions((prev) => [...prev, newActionForm()]);
    }, []);

    const handleRemoveAction = useCallback((index: number) => {
        setActions((prev) => prev.filter((_, i) => i !== index));
    }, []);

    const handleActionChange = useCallback((index: number, field: keyof ActionForm, value: string) => {
        setActions((prev) => prev.map((a, i) => (i === index ? {...a, [field]: value} : a)));
    }, []);

    const handleSave = useCallback(async () => {
        setSaving(true);
        setError(null);
        const data = {
            name,
            enabled,
            trigger: {type: triggerType, channel_id: triggerChannelId},
            actions: actions.map((a) => {
                if (a.type === 'ai_prompt') {
                    return {
                        id: a.id,
                        name: a.name,
                        type: a.type,
                        channel_id: '',
                        body: '',
                        config: {
                            prompt: a.prompt,
                            provider_type: a.provider_type,
                            provider_id: a.provider_id,
                        },
                    };
                }
                const action: Action = {
                    id: a.id,
                    name: a.name,
                    type: a.type,
                    channel_id: a.channel_id,
                    body: a.body,
                };
                if (a.reply_to_post_id) {
                    action.reply_to_post_id = a.reply_to_post_id;
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
            onBack();
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to save flow');
            setSaving(false);
        }
    }, [flowId, name, enabled, triggerType, triggerChannelId, actions, onBack]);

    if (loading) {
        return <p>{'Loading...'}</p>;
    }

    return (
        <div>
            <div style={styles.header}>
                <button
                    style={styles.btnSecondary}
                    onClick={onBack}
                >
                    {'\u2190 Back'}
                </button>
                <h2>{flowId ? 'Edit Flow' : 'Create Flow'}</h2>
            </div>
            {error && <p style={styles.error}>{error}</p>}
            <div style={styles.formGroup}>
                <label style={styles.label}>{'Name'}</label>
                <input
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
            <h3>{'Trigger'}</h3>
            <details style={styles.details}>
                <summary style={styles.summary}>{'Available template variables'}</summary>
                <pre style={styles.pre}>{`{
  "Trigger": {
    "Post": {
      "Id": "string",
      "ChannelId": "string",
      "Message": "string"
    },
    "Channel": {
      "Id": "string",
      "Name": "string",
      "DisplayName": "string"
    },
    "User": {
      "Id": "string",
      "Username": "string",
      "FirstName": "string",
      "LastName": "string"
    }
  }
}`}</pre>
            </details>
            <div style={styles.formGroup}>
                <label style={styles.label}>{'Type'}</label>
                <select
                    style={styles.select}
                    value={triggerType}
                    onChange={(e) => setTriggerType(e.target.value)}
                >
                    <option value='message_posted'>{'message_posted'}</option>
                </select>
            </div>
            <div style={styles.formGroup}>
                <label style={styles.label}>{'Channel ID'}</label>
                <input
                    style={styles.input}
                    type='text'
                    value={triggerChannelId}
                    onChange={(e) => setTriggerChannelId(e.target.value)}
                />
            </div>
            <h3>{'Actions'}</h3>
            {actions.map((action, index) => (
                <div
                    key={index}
                    style={styles.actionItem}
                >
                    <div style={styles.actionHeader}>
                        <div>
                            <strong>{`Action ${index + 1}`}</strong>
                            {action.id && (
                                <span style={styles.actionId}>{` ID: ${action.id}`}</span>
                            )}
                        </div>
                        <button
                            style={styles.btnDanger}
                            onClick={() => handleRemoveAction(index)}
                        >
                            {'Remove'}
                        </button>
                    </div>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>{'Name'}</label>
                        <input
                            style={styles.input}
                            type='text'
                            value={action.name}
                            onChange={(e) => handleActionChange(index, 'name', e.target.value)}
                        />
                    </div>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>{'Type'}</label>
                        <select
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
                                <label style={styles.label}>{'Channel ID'}</label>
                                <input
                                    style={styles.input}
                                    type='text'
                                    value={action.channel_id}
                                    onChange={(e) => handleActionChange(index, 'channel_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Reply to Post ID (Go template, optional)'}</label>
                                <input
                                    style={styles.input}
                                    type='text'
                                    value={action.reply_to_post_id}
                                    placeholder={'e.g. {{.Trigger.Post.Id}}'}
                                    onChange={(e) => handleActionChange(index, 'reply_to_post_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Body (Go template)'}</label>
                                <textarea
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
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Provider Type'}</label>
                                <select
                                    style={styles.select}
                                    value={action.provider_type}
                                    onChange={(e) => handleActionChange(index, 'provider_type', e.target.value)}
                                >
                                    <option value='agent'>{'agent'}</option>
                                    <option value='service'>{'service'}</option>
                                </select>
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Provider ID'}</label>
                                <input
                                    style={styles.input}
                                    type='text'
                                    placeholder={action.provider_type === 'agent' ? 'Bot username' : 'Service name'}
                                    value={action.provider_id}
                                    onChange={(e) => handleActionChange(index, 'provider_id', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>{'Prompt (Go template)'}</label>
                                <textarea
                                    style={styles.textarea}
                                    value={action.prompt}
                                    onChange={(e) => handleActionChange(index, 'prompt', e.target.value)}
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
                    onClick={onBack}
                >
                    {'Cancel'}
                </button>
            </div>
        </div>
    );
};

export default FlowEditorView;
