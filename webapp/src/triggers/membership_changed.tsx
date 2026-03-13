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
    },
    "Membership": {
      "Action": "string (joined or left)"
    }
  }
}`;

const membershipChangedTrigger: TriggerConfig = {
    type: 'membership_changed',
    label: 'Membership Changed',

    defaultFormState(): TriggerFormState {
        return {channel_id: '', action: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {
            channel_id: trigger.membership_changed?.channel_id ?? '',
            action: trigger.membership_changed?.action ?? '',
        };
    },

    toTrigger(state: TriggerFormState): Trigger {
        return {membership_changed: {
            channel_id: state.channel_id,
            ...(state.action ? {action: state.action} : {}),
        }};
    },

    formatSummary(trigger: Trigger): string {
        const action = trigger.membership_changed?.action;
        let actionLabel = '';
        if (action === 'joined') {
            actionLabel = ' (joined only)';
        } else if (action === 'left') {
            actionLabel = ' (left only)';
        }
        return `on ${trigger.membership_changed?.channel_id ?? ''}${actionLabel}`;
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
                        htmlFor='trigger-membership-changed-channel-id'
                        style={styles.label}
                    >{'Channel ID'}</label>
                    <input
                        id='trigger-membership-changed-channel-id'
                        style={styles.input}
                        type='text'
                        value={state.channel_id}
                        onChange={(e) => onChange('channel_id', e.target.value)}
                    />
                </div>
                <div style={styles.formGroup}>
                    <label
                        htmlFor='trigger-membership-changed-action'
                        style={styles.label}
                    >{'Action'}</label>
                    <select
                        id='trigger-membership-changed-action'
                        style={styles.input}
                        value={state.action ?? ''}
                        onChange={(e) => onChange('action', e.target.value)}
                    >
                        <option value={''}>{'Both (join and leave)'}</option>
                        <option value={'joined'}>{'Joined only'}</option>
                        <option value={'left'}>{'Left only'}</option>
                    </select>
                </div>
            </>
        );
    },
};

registerTrigger(membershipChangedTrigger);
