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
      "LastName": "string",
      "IsGuest": "boolean"
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
        return {team_id: '', user_type: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {
            team_id: trigger.user_joined_team?.team_id ?? '',
            user_type: trigger.user_joined_team?.user_type ?? '',
        };
    },

    toTrigger(state: TriggerFormState): Trigger {
        return {user_joined_team: {
            team_id: state.team_id,
            ...(state.user_type ? {user_type: state.user_type} : {}),
        }};
    },

    formatSummary(trigger: Trigger): string {
        const userType = trigger.user_joined_team?.user_type;
        let userTypeLabel = '';
        if (userType === 'user') {
            userTypeLabel = ' (users only)';
        } else if (userType === 'guest') {
            userTypeLabel = ' (guests only)';
        }
        return `on team ${trigger.user_joined_team?.team_id ?? ''}${userTypeLabel}`;
    },

    templateVariables,

    renderFields(
        state: TriggerFormState,
        onChange: (field: string, value: string) => void,
        styles: Record<string, React.CSSProperties>,
    ): React.ReactNode {
        return (
            <>
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
                <div style={styles.formGroup}>
                    <label
                        htmlFor='trigger-user-joined-team-user-type'
                        style={styles.label}
                    >{'User Type'}</label>
                    <select
                        id='trigger-user-joined-team-user-type'
                        style={styles.input}
                        value={state.user_type ?? ''}
                        onChange={(e) => onChange('user_type', e.target.value)}
                    >
                        <option value={''}>{'All users'}</option>
                        <option value={'user'}>{'Users only'}</option>
                        <option value={'guest'}>{'Guests only'}</option>
                    </select>
                </div>
            </>
        );
    },
};

registerTrigger(userJoinedTeamTrigger);
