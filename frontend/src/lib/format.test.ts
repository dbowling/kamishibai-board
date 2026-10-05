import { afterEach, describe, expect, it } from 'vitest';
import {
  cadenceLabel,
  clockTime,
  dateTime,
  msUntilNextBoundary,
  percent,
  periodKeyLabel,
  periodKeyShortLabel,
  statusLabel,
  timeUntil,
  timeZoneOptions,
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

describe('period key labels are calendar labels, not instants', () => {
  // Kiritimati is UTC+14, the furthest east of any zone. Formatting a day in it
  // is where a label built from an instant would slip to the next day.
  const KIRITIMATI = 'Pacific/Kiritimati';
  const PAGO_PAGO = 'Pacific/Pago_Pago'; // UTC-11, the furthest west

  const realFormat = Date.prototype.toLocaleDateString;
  afterEach(() => {
    Date.prototype.toLocaleDateString = realFormat;
  });

  /** Runs the label with the viewer's zone forced to `zone`, as if the browser were there. */
  function labelAsSeenFrom(zone: string, key: string): string {
    Date.prototype.toLocaleDateString = function (
      this: Date,
      locales?: Intl.LocalesArgument,
      options?: Intl.DateTimeFormatOptions,
    ) {
      // A caller that pins its own timeZone is immune to the viewer's; one that
      // leaves it unset picks up the viewer's zone, which is what is simulated.
      return realFormat.call(this, locales, { timeZone: zone, ...options });
    } as typeof Date.prototype.toLocaleDateString;
    return periodKeyLabel(key);
  }

  it('labels a daily key as that day whatever zone the viewer is in', () => {
    for (const zone of [KIRITIMATI, PAGO_PAGO, 'UTC', 'America/New_York']) {
      const label = labelAsSeenFrom(zone, '2026-10-05');
      expect(label, zone).toContain('5');
      expect(label, zone).toContain('Oct');
      expect(label, zone).toMatch(/Mon/);
    }
  });

  it('labels a monthly key as that month whatever zone the viewer is in', () => {
    for (const zone of [KIRITIMATI, PAGO_PAGO]) {
      expect(labelAsSeenFrom(zone, '2026-M10'), zone).toContain('October');
    }
  });
});

describe('dateTime', () => {
  const instant = '2026-10-05T20:00:00-04:00'; // 09:00 on the 6th in Tokyo

  it('renders the instant in the zone it is given', () => {
    const tokyo = dateTime(instant, 'Asia/Tokyo');
    expect(tokyo).toContain('6');
    expect(tokyo).toContain('09:00');

    const newYork = dateTime(instant, 'America/New_York');
    expect(newYork).toContain('5');
    // 12-hour or 24-hour depending on the test locale.
    expect(newYork).toMatch(/(08:00 PM|20:00)/);
  });

  it('is the same instant either way, only written differently', () => {
    expect(dateTime(instant, 'Asia/Tokyo')).not.toBe(dateTime(instant, 'America/New_York'));
  });

  it('falls back to the browser zone for a zone the runtime rejects', () => {
    expect(dateTime(instant, 'Mars/Base')).toBe(dateTime(instant));
  });

  it('is empty for nothing or nonsense', () => {
    expect(dateTime(undefined, 'Asia/Tokyo')).toBe('');
    expect(dateTime('not a date', 'Asia/Tokyo')).toBe('');
  });
});

describe('clockTime', () => {
  it('gives midnight in one zone as the right wall clock in another', () => {
    // Midnight ending 2026-10-05 in New York (EDT) is 13:00 in Tokyo.
    const end = '2026-10-06T00:00:00-04:00';
    expect(clockTime(end, 'America/New_York')).toBe('00:00');
    expect(clockTime(end, 'Asia/Tokyo')).toBe('13:00');
  });
});

describe('timeZoneOptions', () => {
  it('is sorted, includes UTC, and always keeps the stored value', () => {
    const zones = timeZoneOptions('Mars/Base');
    expect(zones).toContain('UTC');
    expect(zones).toContain('Mars/Base');
    expect(zones).toEqual([...zones].sort((a, b) => a.localeCompare(b)));
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
