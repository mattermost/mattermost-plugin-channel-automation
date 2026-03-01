import React, {useEffect} from 'react';
import {Redirect, Route, Switch, useRouteMatch} from 'react-router-dom';

import FlowEditorView from 'components/flow_editor_view';
import FlowListView from 'components/flow_list_view';
import Sidebar from 'components/sidebar';

import './layout.scss';

const MainApp: React.FC = () => {
    useEffect(() => {
        document.body.classList.add('app__body');

        return () => {
            document.body.classList.remove('app__body');
        };
    }, []);

    const {path, url} = useRouteMatch();

    return (
        <div className='channel-automation-root'>
            <Sidebar baseUrl={url}/>
            <div className='channel-automation-content'>
                <div className='channel-automation-content__inner'>
                    <Switch>
                        <Route
                            exact={true}
                            path={`${path}/workflows`}
                            component={FlowListView}
                        />
                        <Route
                            exact={true}
                            path={`${path}/workflows/add`}
                            component={FlowEditorView}
                        />
                        <Route
                            exact={true}
                            path={`${path}/workflows/:id/edit`}
                            component={FlowEditorView}
                        />
                        <Redirect to={`${url}/workflows`}/>
                    </Switch>
                </div>
            </div>
        </div>
    );
};

export default MainApp;
