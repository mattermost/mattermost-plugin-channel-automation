import {deleteAutomation, getAutomations, updateAutomation} from 'client';
import React, {useCallback, useEffect, useState} from 'react';
import {useHistory, useRouteMatch} from 'react-router-dom';
import {getTriggerConfig} from 'triggers';

import 'triggers/channel_created';
import 'triggers/membership_changed';
import 'triggers/message_posted';
import 'triggers/schedule';
import type {Automation} from 'types';
import {getTriggerType} from 'types';

const styles = {
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: 16,
    } as React.CSSProperties,
    table: {
        width: '100%',
        borderCollapse: 'collapse' as const,
    },
    th: {
        textAlign: 'left' as const,
        padding: '8px 12px',
        borderBottom: '2px solid rgba(var(--center-channel-color-rgb), 0.2)',
        fontWeight: 600,
    },
    td: {
        padding: '8px 12px',
        borderBottom: '1px solid rgba(var(--center-channel-color-rgb), 0.1)',
    },
    row: {
        cursor: 'pointer',
    } as React.CSSProperties,
    btnPrimary: {
        padding: '8px 16px',
        backgroundColor: 'var(--button-bg)',
        color: 'var(--button-color)',
        border: 'none',
        borderRadius: 4,
        cursor: 'pointer',
        fontSize: 14,
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
    error: {
        color: 'var(--error-text)',
        marginBottom: 12,
    } as React.CSSProperties,
};

function formatTrigger(automation: Automation): string {
    const tt = getTriggerType(automation.trigger);
    const config = getTriggerConfig(tt);
    if (config) {
        return `${config.label} ${config.formatSummary(automation.trigger)}`;
    }
    return tt;
}

const AutomationListView: React.FC = () => {
    const history = useHistory();
    const {url} = useRouteMatch();
    const [automations, setAutomations] = useState<Automation[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const fetchAutomations = useCallback(async () => {
        try {
            setLoading(true);
            setError(null);
            const data = await getAutomations();
            setAutomations(data);
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to load automations');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchAutomations();
    }, [fetchAutomations]);

    const handleToggleEnabled = useCallback(async (e: React.MouseEvent, automation: Automation) => {
        e.stopPropagation();
        const updated = {...automation, enabled: !automation.enabled};
        setAutomations((prev) => prev.map((f) => (f.id === automation.id ? updated : f)));
        try {
            await updateAutomation(automation.id, updated);
        } catch (err: unknown) {
            setAutomations((prev) => prev.map((f) => (f.id === automation.id ? automation : f)));
            setError(err instanceof Error ? err.message : 'Failed to update automation');
        }
    }, []);

    const handleDelete = useCallback(async (e: React.MouseEvent, automation: Automation) => {
        e.stopPropagation();
        // eslint-disable-next-line no-alert
        if (!window.confirm(`Delete automation "${automation.name}"?`)) {
            return;
        }
        try {
            await deleteAutomation(automation.id);
            setAutomations((prev) => prev.filter((f) => f.id !== automation.id));
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to delete automation');
        }
    }, []);

    if (loading) {
        return <p>{'Loading automations...'}</p>;
    }

    return (
        <div>
            <div style={styles.header}>
                <h2>{'Automations'}</h2>
                <button
                    style={styles.btnPrimary}
                    onClick={() => history.push(`${url}/add`)}
                >
                    {'Create Automation'}
                </button>
            </div>
            {error && <p style={styles.error}>{error}</p>}
            {automations.length === 0 ? (
                <p>{'No automations yet. Create one to get started.'}</p>
            ) : (
                <table style={styles.table}>
                    <thead>
                        <tr>
                            <th style={styles.th}>{'Name'}</th>
                            <th style={styles.th}>{'Trigger'}</th>
                            <th style={styles.th}>{'Enabled'}</th>
                            <th style={styles.th}>{'Actions'}</th>
                            <th style={styles.th}>{'Controls'}</th>
                        </tr>
                    </thead>
                    <tbody>
                        {automations.map((automation) => (
                            <tr
                                key={automation.id}
                                style={styles.row}
                                onClick={() => history.push(`${url}/${automation.id}/edit`)}
                            >
                                <td style={styles.td}>{automation.name}</td>
                                <td style={styles.td}>{formatTrigger(automation)}</td>
                                <td style={styles.td}>
                                    <input
                                        type='checkbox'
                                        checked={automation.enabled}
                                        onClick={(e) => handleToggleEnabled(e, automation)}
                                        readOnly={true}
                                    />
                                </td>
                                <td style={styles.td}>{(automation.actions ?? []).length}</td>
                                <td style={styles.td}>
                                    <button
                                        style={styles.btnDanger}
                                        onClick={(e) => handleDelete(e, automation)}
                                    >
                                        {'Delete'}
                                    </button>
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            )}
        </div>
    );
};

export default AutomationListView;
