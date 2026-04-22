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
      "LastName": "string",
      "AuthorDisplay": "method — renders '@username (First Last)' with graceful fallbacks"
    },
    "Thread": {
      "RootID": "string — only present when include_thread_context is on and the post is a reply",
      "PostCount": "number",
      "Summary": "string — populated by ai_prompt when PostCount >= 5, otherwise empty",
      "TranscriptDisplay": "string — plaintext transcript, one 'user.AuthorDisplay: message' line per post",
      "Messages": "array of Post {Id, ChannelId, ThreadId, Message, User: {Id, Username, FirstName, LastName, AuthorDisplay}, CreateAt} — ordered oldest first. Emptied after summarization."
    }
  }
}`;

const messagePostedTrigger: TriggerConfig = {
    type: 'message_posted',
    label: 'Message Posted',

    defaultFormState(): TriggerFormState {
        return {channel_id: '', include_thread_context: 'false'};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {
            channel_id: trigger.message_posted?.channel_id ?? '',
            include_thread_context: trigger.message_posted?.include_thread_context ? 'true' : 'false',
        };
    },

    toTrigger(state: TriggerFormState): Trigger {
        return {
            message_posted: {
                channel_id: state.channel_id,
                include_thread_context: state.include_thread_context === 'true',
            },
        };
    },

    formatSummary(trigger: Trigger): string {
        const base = `on ${trigger.message_posted?.channel_id ?? ''}`;
        return trigger.message_posted?.include_thread_context ? `${base} (with thread context)` : base;
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
                <div style={styles.formGroup}>
                    <label
                        htmlFor='trigger-message-posted-include-thread-context'
                        style={styles.label}
                    >
                        <input
                            id='trigger-message-posted-include-thread-context'
                            type='checkbox'
                            checked={state.include_thread_context === 'true'}
                            onChange={(e) => onChange('include_thread_context', e.target.checked ? 'true' : 'false')}
                        />
                        {' Include thread context when post is a reply'}
                    </label>
                </div>
            </>
        );
    },
};

registerTrigger(messagePostedTrigger);
