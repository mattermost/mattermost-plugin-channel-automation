import type React from 'react';

import type {Trigger} from 'types';

/**
 * TriggerFormState holds the form-level state for a trigger type.
 * Each trigger implementation defines what fields it needs.
 */
export type TriggerFormState = Record<string, string>;

/**
 * TriggerConfig describes the UI behavior for a trigger type.
 * Each trigger type registers one implementation.
 */
export interface TriggerConfig {

    /** Unique trigger type key, e.g. "message_posted". */
    type: string;

    /** Human-readable label shown in dropdowns and list view. */
    label: string;

    /** Returns the initial (empty) form state for this trigger. */
    defaultFormState(): TriggerFormState;

    /** Populates form state from a persisted Trigger object. */
    fromTrigger(trigger: Trigger): TriggerFormState;

    /** Serializes form state back into a Trigger for the API. */
    toTrigger(state: TriggerFormState): Trigger;

    /** Short description for the flow list table, e.g. "on #channel". */
    formatSummary(trigger: Trigger): string;

    /** Template variables JSON string shown in the editor. */
    templateVariables: string;

    /**
     * Renders the trigger-specific form fields.
     * Uses the shared styles from the editor.
     */
    renderFields(
        state: TriggerFormState,
        onChange: (field: string, value: string) => void,
        styles: Record<string, React.CSSProperties>,
    ): React.ReactNode;
}

// Registry of all known trigger configurations.
const triggerRegistry = new Map<string, TriggerConfig>();

export function registerTrigger(config: TriggerConfig): void {
    triggerRegistry.set(config.type, config);
}

export function getTriggerConfig(type: string): TriggerConfig | undefined {
    return triggerRegistry.get(type);
}

export function getAllTriggerConfigs(): TriggerConfig[] {
    return Array.from(triggerRegistry.values());
}
