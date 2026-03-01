import {deleteFlow, getFlows, updateFlow} from 'client';
import React, {useCallback, useEffect, useState} from 'react';

import type {Flow} from 'types';

interface Props {
    onCreateFlow: () => void;
    onEditFlow: (id: string) => void;
}

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

const FlowListView: React.FC<Props> = ({onCreateFlow, onEditFlow}) => {
    const [flows, setFlows] = useState<Flow[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const fetchFlows = useCallback(async () => {
        try {
            setLoading(true);
            setError(null);
            const data = await getFlows();
            setFlows(data);
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to load flows');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        fetchFlows();
    }, [fetchFlows]);

    const handleToggleEnabled = useCallback(async (e: React.MouseEvent, flow: Flow) => {
        e.stopPropagation();
        const updated = {...flow, enabled: !flow.enabled};
        setFlows((prev) => prev.map((f) => (f.id === flow.id ? updated : f)));
        try {
            await updateFlow(flow.id, updated);
        } catch (err: unknown) {
            setFlows((prev) => prev.map((f) => (f.id === flow.id ? flow : f)));
            setError(err instanceof Error ? err.message : 'Failed to update flow');
        }
    }, []);

    const handleDelete = useCallback(async (e: React.MouseEvent, flow: Flow) => {
        e.stopPropagation();
        // eslint-disable-next-line no-alert
        if (!window.confirm(`Delete flow "${flow.name}"?`)) {
            return;
        }
        try {
            await deleteFlow(flow.id);
            setFlows((prev) => prev.filter((f) => f.id !== flow.id));
        } catch (err: unknown) {
            setError(err instanceof Error ? err.message : 'Failed to delete flow');
        }
    }, []);

    if (loading) {
        return <p>{'Loading flows...'}</p>;
    }

    return (
        <div>
            <div style={styles.header}>
                <h2>{'Flows'}</h2>
                <button
                    style={styles.btnPrimary}
                    onClick={onCreateFlow}
                >
                    {'Create Flow'}
                </button>
            </div>
            {error && <p style={styles.error}>{error}</p>}
            {flows.length === 0 ? (
                <p>{'No flows yet. Create one to get started.'}</p>
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
                        {flows.map((flow) => (
                            <tr
                                key={flow.id}
                                style={styles.row}
                                onClick={() => onEditFlow(flow.id)}
                            >
                                <td style={styles.td}>{flow.name}</td>
                                <td style={styles.td}>{`${flow.trigger.type} on ${flow.trigger.channel_id}`}</td>
                                <td style={styles.td}>
                                    <input
                                        type='checkbox'
                                        checked={flow.enabled}
                                        onClick={(e) => handleToggleEnabled(e, flow)}
                                        readOnly={true}
                                    />
                                </td>
                                <td style={styles.td}>{(flow.actions ?? []).length}</td>
                                <td style={styles.td}>
                                    <button
                                        style={styles.btnDanger}
                                        onClick={(e) => handleDelete(e, flow)}
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

export default FlowListView;
