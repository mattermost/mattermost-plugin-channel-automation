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
        return {team_id: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {team_id: trigger.user_joined_team?.team_id ?? ''};
    },

    toTrigger(state: TriggerFormState): Trigger {
        return {user_joined_team: {team_id: state.team_id}};
    },

    formatSummary(trigger: Trigger): string {
        return `on team ${trigger.user_joined_team?.team_id ?? ''}`;
    },

    templateVariables,

    renderFields(
        state: TriggerFormState,
        onChange: (field: string, value: string) => void,
        styles: Record<string, React.CSSProperties>,
    ): React.ReactNode {
        return (
            <div style={styles.formGroup}>
                <label
                    htmlFor='trigger-user-joined-team-team-id'
                    style={styles.label}
                >{'Team ID'}</label>
                <input
                    id='trigger-user-joined-team-team-id'
                    style={styles.input}
                    type='text'
                    value={state.team_id}
                    onChange={(e) => onChange('team_id', e.target.value)}
                />
            </div>
        );
    },
};

registerTrigger(userJoinedTeamTrigger);
