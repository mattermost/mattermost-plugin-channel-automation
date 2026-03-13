import React from 'react';

import type {Trigger} from 'types';

import type {TriggerConfig, TriggerFormState} from './index';
import {registerTrigger} from './index';

const templateVariables = `{
  "Trigger": {
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
}`;

const channelCreatedTrigger: TriggerConfig = {
    type: 'channel_created',
    label: 'Channel Created',

    defaultFormState(): TriggerFormState {
        return {};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    fromTrigger(trigger: Trigger): TriggerFormState {
        return {};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    toTrigger(state: TriggerFormState): Trigger {
        return {channel_created: {}};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    formatSummary(trigger: Trigger): string {
        return 'on any new channel';
    },

    templateVariables,

    renderFields(
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        state: TriggerFormState,
        // eslint-disable-next-line @typescript-eslint/no-unused-vars
        onChange: (field: string, value: string) => void,
        styles: Record<string, React.CSSProperties>,
    ): React.ReactNode {
        return (
            <div style={styles.formGroup}>
                <p style={{fontSize: 13, color: 'rgba(var(--center-channel-color-rgb), 0.56)', margin: 0}}>
                    {'This trigger fires when any new public channel is created. No additional configuration is needed.'}
                </p>
            </div>
        );
    },
};

registerTrigger(channelCreatedTrigger);
