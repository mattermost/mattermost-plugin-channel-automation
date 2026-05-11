import React, {useEffect} from 'react';
import {Redirect, Route, Switch, useRouteMatch} from 'react-router-dom';

import AutomationEditorView from 'components/automation_editor_view';
import AutomationListView from 'components/automation_list_view';
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
                            path={`${path}/automations`}
                            component={AutomationListView}
                        />
                        <Route
                            exact={true}
                            path={`${path}/automations/add`}
                            component={AutomationEditorView}
                        />
                        <Route
                            exact={true}
                            path={`${path}/automations/:id/edit`}
                            component={AutomationEditorView}
                        />
                        <Redirect to={`${url}/automations`}/>
                    </Switch>
                </div>
            </div>
        </div>
    );
};

export default MainApp;
