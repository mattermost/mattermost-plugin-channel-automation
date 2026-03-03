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

const scheduleTrigger: TriggerConfig = {
    type: 'schedule',
    label: 'Schedule',

    defaultFormState(): TriggerFormState {
        return {interval: '', start_at: ''};
    },

    fromTrigger(trigger: Trigger): TriggerFormState {
        return {
            interval: trigger.interval ?? '',
            start_at: trigger.start_at ? new Date(trigger.start_at).toISOString().slice(0, 16) : '',
        };
    },

    toTrigger(state: TriggerFormState): Trigger {
        const trigger: Trigger = {type: 'schedule', interval: state.interval};
        if (state.start_at) {
            trigger.start_at = new Date(state.start_at).getTime();
        }
        return trigger;
    },

    formatSummary(trigger: Trigger): string {
        return `every ${trigger.interval ?? '?'}`;
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
                    <label style={styles.label}>{'Interval (Go duration, e.g. 5m, 1h, 24h)'}</label>
                    <input
                        style={styles.input}
                        type='text'
                        value={state.interval}
                        placeholder={'e.g. 1h'}
                        onChange={(e) => onChange('interval', e.target.value)}
                    />
                </div>
                <div style={styles.formGroup}>
                    <label style={styles.label}>{'Start at (optional, leave empty to start immediately)'}</label>
                    <input
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
