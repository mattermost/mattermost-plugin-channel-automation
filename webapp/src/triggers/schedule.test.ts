import {getTriggerConfig} from './index';
import './schedule';

describe('schedule trigger', () => {
    const config = getTriggerConfig('schedule');

    it('formats interval-only schedules as starting immediately', () => {
        expect(config).toBeDefined();
        expect(config!.formatSummary({
            schedule: {
                channel_id: 'channel-id',
                interval: '168h',
            },
        })).toBe('every 168h starting immediately');
    });

    it('formats schedules with the exact UTC start time', () => {
        expect(config).toBeDefined();
        expect(config!.formatSummary({
            schedule: {
                channel_id: 'channel-id',
                interval: '168h',
                start_at: Date.UTC(2026, 0, 2, 3, 4),
            },
        })).toBe('every 168h starting Friday, 2026-01-02 03:04 UTC');
    });
});
