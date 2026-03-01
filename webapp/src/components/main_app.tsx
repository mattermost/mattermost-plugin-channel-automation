import React, {useCallback, useEffect, useState} from 'react';

import FlowEditorView from 'components/flow_editor_view';
import FlowListView from 'components/flow_list_view';
import Sidebar from 'components/sidebar';

import './layout.scss';

type View = 'list' | 'editor';

const MainApp: React.FC = () => {
    useEffect(() => {
        document.body.classList.add('app__body');

        return () => {
            document.body.classList.remove('app__body');
        };
    }, []);
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

    const handleSidebarItemClick = useCallback((id: string) => {
        if (id === 'flows') {
            setEditingFlowId(null);
            setView('list');
        }
    }, []);

    return (
        <div className='channel-automation-root'>
            <Sidebar
                activeItem='flows'
                onItemClick={handleSidebarItemClick}
            />
            <div className='channel-automation-content'>
                <div className='channel-automation-content__inner'>
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
            </div>
        </div>
    );
};

export default MainApp;
