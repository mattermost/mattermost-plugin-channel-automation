import React from 'react';

import type {Trigger} from 'types';

import type {TriggerConfig, TriggerFormState} from './index';
import {registerTrigger} from './index';

const templateVariables = `{
  "Trigger": {
    "User": {
      "Id": "string",
      "Username": "string",
      "FirstName": "string",
      "LastName": "string"
    },
    "Team": {
      "Id": "string",
      "Name": "string",
      "DisplayName": "string",
      "DefaultChannelId": "string"
    }
  }
}`;

const userJoinedTeamTrigger: TriggerConfig = {
    type: 'user_joined_team',
    label: 'User Joined Team',

    defaultFormState(): TriggerFormState {
        return {};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    fromTrigger(trigger: Trigger): TriggerFormState {
        return {};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    toTrigger(state: TriggerFormState): Trigger {
        return {user_joined_team: {}};
    },

    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    formatSummary(trigger: Trigger): string {
        return 'on any team';
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
                    {'This trigger fires when a user joins any team. Bot users are excluded. The team\'s default channel ID is available in the template context.'}
                </p>
            </div>
        );
    },
};

registerTrigger(userJoinedTeamTrigger);
