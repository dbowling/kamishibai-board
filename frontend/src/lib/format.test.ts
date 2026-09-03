import { describe, expect, it } from 'vitest';
import {
  cadenceLabel,
  msUntilNextBoundary,
  percent,
  periodKeyLabel,
  periodKeyShortLabel,
  statusLabel,
  timeUntil,
} from './format';
import type { Period } from './types';

describe('periodKeyLabel', () => {
  it('expands each canonical key shape', () => {
    // Daily keys go through toLocaleDateString, so assert on the parts rather
    // than an exact locale-dependent string.
    const daily = periodKeyLabel('2026-09-03');
    expect(daily).toContain('2026');
    expect(daily).not.toBe('2026-09-03');

    expect(periodKeyLabel('2026-W36')).toBe('Week 36, 2026');
    expect(periodKeyLabel('2026-Q3')).toBe('Q3 2026');
    expect(periodKeyLabel('2026-Y')).toBe('2026');

    const monthly = periodKeyLabel('2026-M09');
    expect(monthly).toContain('2026');
    expect(monthly).not.toBe('2026-M09');
  });

  it('strips leading zeros from week numbers', () => {
    expect(periodKeyLabel('2026-W01')).toBe('Week 1, 2026');
  });

  it('passes an unrecognised key through rather than throwing', () => {
    expect(periodKeyLabel('nonsense')).toBe('nonsense');
    expect(periodKeyLabel('')).toBe('');
  });
});

describe('periodKeyShortLabel', () => {
  it('abbreviates for tight spaces', () => {
    expect(periodKeyShortLabel('2026-09-03')).toBe('03/09');
    expect(periodKeyShortLabel('2026-W36')).toBe('W36');
    expect(periodKeyShortLabel('2026-M09')).toBe('09/26');
  });
});

describe('percent', () => {
  it('renders a 0..1 rate as a whole percentage', () => {
    expect(percent(0)).toBe('0%');
    expect(percent(1)).toBe('100%');
    expect(percent(0.5)).toBe('50%');
    expect(percent(1 / 3)).toBe('33%');
  });

  it('survives non-finite input', () => {
    expect(percent(Number.NaN)).toBe('0%');
    expect(percent(Number.POSITIVE_INFINITY)).toBe('0%');
  });
});

describe('timeUntil', () => {
  const now = new Date('2026-09-03T14:00:00Z');

  it('scales the unit to the distance', () => {
    expect(timeUntil('2026-09-03T14:30:00Z', now)).toBe('30m left');
    expect(timeUntil('2026-09-03T20:00:00Z', now)).toBe('6h left');
    expect(timeUntil('2026-09-06T14:00:00Z', now)).toBe('3d left');
    expect(timeUntil('2026-09-24T14:00:00Z', now)).toBe('3w left');
    expect(timeUntil('2026-12-03T14:00:00Z', now)).toBe('3mo left');
  });

  it('reports a period that has already ended as closing now', () => {
    expect(timeUntil('2026-09-03T13:00:00Z', now)).toBe('closing now');
    expect(timeUntil('2026-09-03T14:00:00Z', now)).toBe('closing now');
  });

  it('returns nothing for an unparseable timestamp', () => {
    expect(timeUntil('not a date', now)).toBe('');
  });
});

describe('msUntilNextBoundary', () => {
  const now = new Date('2026-09-03T14:00:00Z');

  const period = (key: string, end: string): Period => ({
    cadence: 'daily',
    key,
    start: '2026-09-03T04:00:00Z',
    end,
    label: 'Daily',
  });

  it('finds the soonest future boundary', () => {
    const result = msUntilNextBoundary(
      [
        period('annual', '2027-01-01T05:00:00Z'),
        period('daily', '2026-09-04T04:00:00Z'),
        period('weekly', '2026-09-07T04:00:00Z'),
      ],
      now,
    );
    // The daily boundary is 14 hours away.
    expect(result).toBe(14 * 60 * 60 * 1000);
  });

  it('ignores boundaries already in the past', () => {
    const result = msUntilNextBoundary(
      [period('stale', '2026-09-01T04:00:00Z'), period('daily', '2026-09-04T04:00:00Z')],
      now,
    );
    expect(result).toBe(14 * 60 * 60 * 1000);
  });

  it('returns null when there is nothing upcoming', () => {
    expect(msUntilNextBoundary([], now)).toBeNull();
    expect(msUntilNextBoundary([period('stale', '2026-01-01T00:00:00Z')], now)).toBeNull();
    expect(msUntilNextBoundary([period('bad', 'not a date')], now)).toBeNull();
  });
});

describe('labels', () => {
  it('names cadences and statuses', () => {
    expect(cadenceLabel('quarterly')).toBe('Quarterly');
    expect(statusLabel('not_started')).toBe('Not started');
    expect(statusLabel('in_progress')).toBe('In progress');
    expect(statusLabel('done')).toBe('Done');
  });
});
