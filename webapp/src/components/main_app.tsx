import React, {useCallback, useState} from 'react';

import FlowEditorView from 'components/flow_editor_view';
import FlowListView from 'components/flow_list_view';

type View = 'list' | 'editor';

const containerStyle: React.CSSProperties = {
    maxWidth: 960,
    margin: '0 auto',
    padding: 20,
    color: 'var(--center-channel-color)',
    backgroundColor: 'var(--center-channel-bg)',
    minHeight: '100vh',
};

const MainApp: React.FC = () => {
    const [view, setView] = useState<View>('list');
    const [editingFlowId, setEditingFlowId] = useState<string | null>(null);

    const handleCreateFlow = useCallback(() => {
        setEditingFlowId(null);
        setView('editor');
    }, []);

    const handleEditFlow = useCallback((id: string) => {
        setEditingFlowId(id);
        setView('editor');
    }, []);

    const handleBack = useCallback(() => {
        setEditingFlowId(null);
        setView('list');
    }, []);

    return (
        <div style={containerStyle}>
            {view === 'list' ? (
                <FlowListView
                    onCreateFlow={handleCreateFlow}
                    onEditFlow={handleEditFlow}
                />
            ) : (
                <FlowEditorView
                    flowId={editingFlowId}
                    onBack={handleBack}
                />
            )}
        </div>
    );
};

export default MainApp;
