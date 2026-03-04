import React from 'react';

import type {Trigger} from 'types';

import type {TriggerConfig, TriggerFormState} from './index';
import {registerTrigger} from './index';

const templateVariables = `{
  "Trigger": {
    "Post": {
      "Id": "string",
      "ChannelId": "string",
      "ThreadId": "string",
      "Message": "string"
    },
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

const messagePostedTrigger: TriggerConfig = {
    type: 'message_posted',
    label: 'Message Posted',

    defaultFormState(): TriggerFormState {
        return {channel_id: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {channel_id: trigger.message_posted?.channel_id ?? ''};
    },

    toTrigger(state: TriggerFormState): Trigger {
        return {message_posted: {channel_id: state.channel_id}};
    },

    formatSummary(trigger: Trigger): string {
        return `on ${trigger.message_posted?.channel_id ?? ''}`;
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
                    htmlFor='trigger-message-posted-channel-id'
                    style={styles.label}
                >{'Channel ID'}</label>
                <input
                    id='trigger-message-posted-channel-id'
                    style={styles.input}
                    type='text'
                    value={state.channel_id}
                    onChange={(e) => onChange('channel_id', e.target.value)}
                />
            </div>
        );
    },
};

registerTrigger(messagePostedTrigger);
