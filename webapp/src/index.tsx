import manifest from 'manifest';
import type {Store} from 'redux';

import type {GlobalState} from '@mattermost/types/store';

import HeaderCenter from 'components/header_center';
import {iconSVG} from 'components/icon';
import MainApp from 'components/main_app';

import type {PluginRegistry} from 'types/mattermost-webapp';

export default class Plugin {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars, @typescript-eslint/no-empty-function
    public async initialize(registry: PluginRegistry, store: Store<GlobalState>) {
        registry.registerProduct(
            `/plug/${manifest.id}`,
            iconSVG,
            'Channel Automation',
            `/plug/${manifest.id}`,
            MainApp,
            HeaderCenter,
            undefined,
            false,
        );
    }
}

declare global {
    interface Window {
        registerPlugin(pluginId: string, plugin: Plugin): void;
    }
}

window.registerPlugin(manifest.id, new Plugin());
