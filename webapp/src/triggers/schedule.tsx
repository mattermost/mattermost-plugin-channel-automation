import React from 'react';

import type {Trigger} from 'types';

import type {TriggerConfig, TriggerFormState} from './index';
import {registerTrigger} from './index';

const templateVariables = `{
  "Trigger": {
    "Schedule": {
      "FiredAt": "number (Unix ms)",
      "Interval": "string"
    }
  }
}`;

function formatUTCStartAt(startAt?: number): string {
    if (!startAt) {
        return '';
    }
    const d = new Date(startAt);
    if (Number.isNaN(d.getTime())) {
        return '';
    }
    const days = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    const pad = (n: number) => String(n).padStart(2, '0');

    return `${days[d.getUTCDay()]}, ${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}-${pad(d.getUTCDate())} ${pad(d.getUTCHours())}:${pad(d.getUTCMinutes())} UTC`;
}

const scheduleTrigger: TriggerConfig = {
    type: 'schedule',
    label: 'Schedule',

    defaultFormState(): TriggerFormState {
        return {channel_id: '', interval: '', start_at: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        let startAt = '';
        if (trigger.schedule?.start_at) {
            const d = new Date(trigger.schedule.start_at);
            const pad = (n: number) => String(n).padStart(2, '0');
            startAt = `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
        }
        return {
            channel_id: trigger.schedule?.channel_id ?? '',
            interval: trigger.schedule?.interval ?? '',
            start_at: startAt,
        };
    },

    toTrigger(state: TriggerFormState): Trigger {
        const params: {channel_id: string; interval: string; start_at?: number} = {channel_id: state.channel_id, interval: state.interval};
        if (state.start_at) {
            const timestamp = new Date(state.start_at).getTime();
            if (!Number.isNaN(timestamp)) {
                params.start_at = timestamp;
            }
        }
        return {schedule: params};
    },

    formatSummary(trigger: Trigger): string {
        const startAt = formatUTCStartAt(trigger.schedule?.start_at);
        if (startAt) {
            return `every ${trigger.schedule?.interval ?? '?'} starting ${startAt}`;
        }
        return `every ${trigger.schedule?.interval ?? '?'} starting immediately`;
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
                        htmlFor='trigger-schedule-channel-id'
                        style={styles.label}
                    >{'Channel ID'}</label>
                    <input
                        id='trigger-schedule-channel-id'
                        style={styles.input}
                        type='text'
                        value={state.channel_id}
                        onChange={(e) => onChange('channel_id', e.target.value)}
                    />
                </div>
                <div style={styles.formGroup}>
                    <label
                        htmlFor='trigger-schedule-interval'
                        style={styles.label}
                    >{'Interval (Go duration, min 1h, e.g. 1h, 24h)'}</label>
                    <input
                        id='trigger-schedule-interval'
                        style={styles.input}
                        type='text'
                        value={state.interval}
                        placeholder={'e.g. 1h'}
                        onChange={(e) => onChange('interval', e.target.value)}
                    />
                </div>
                <div style={styles.formGroup}>
                    <label
                        htmlFor='trigger-schedule-start-at'
                        style={styles.label}
                    >{'Start at (optional, leave empty to start immediately)'}</label>
                    <input
                        id='trigger-schedule-start-at'
                        style={styles.input}
                        type='datetime-local'
                        value={state.start_at}
                        onChange={(e) => onChange('start_at', e.target.value)}
                    />
                </div>
            </>
        );
    },
};

registerTrigger(scheduleTrigger);
