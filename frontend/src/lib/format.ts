import type { Cadence, Period, Status } from './types';

/** Human labels for the five cadences. */
const CADENCE_LABELS: Record<Cadence, string> = {
  daily: 'Daily',
  weekly: 'Weekly',
  monthly: 'Monthly',
  quarterly: 'Quarterly',
  annual: 'Annual',
};

export function cadenceLabel(cadence: Cadence): string {
  return CADENCE_LABELS[cadence] ?? cadence;
}

/** What a card of this cadence resets on, phrased for a tooltip. */
const CADENCE_RESET: Record<Cadence, string> = {
  daily: 'Resets every day at midnight',
  weekly: 'Resets every Monday',
  monthly: 'Resets on the 1st of each month',
  quarterly: 'Resets on the 1st of January, April, July and October',
  annual: 'Resets on the 1st of January',
};

export function cadenceReset(cadence: Cadence): string {
  return CADENCE_RESET[cadence] ?? '';
}

const STATUS_LABELS: Record<Status, string> = {
  not_started: 'Not started',
  in_progress: 'In progress',
  done: 'Done',
};

export function statusLabel(status: Status): string {
  return STATUS_LABELS[status] ?? status;
}

/**
 * Turns a period key into something readable.
 *
 * The keys are canonical and sortable rather than pretty (2026-W36, 2026-Q3), so
 * they are expanded for display. Parsing is deliberately tolerant: an unknown
 * shape is shown as-is rather than throwing, because a label is never worth
 * breaking a screen over.
 *
 * A key names a calendar day, week or month, not an instant, so no viewer's time
 * zone may move it. The date is therefore built and formatted in UTC: building it
 * in local time and formatting in local time happens to round-trip, but any
 * formatting zone other than the one it was built in (for example one a long way
 * east, where midnight UTC is already tomorrow) would shift the day.
 */
export function periodKeyLabel(key: string): string {
  const daily = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
  if (daily) {
    const [, year, month, day] = daily;
    const date = new Date(Date.UTC(Number(year), Number(month) - 1, Number(day)));
    return date.toLocaleDateString(undefined, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
      timeZone: 'UTC',
    });
  }

  const weekly = /^(\d{4})-W(\d{2})$/.exec(key);
  if (weekly) {
    return `Week ${Number(weekly[2])}, ${weekly[1]}`;
  }

  const monthly = /^(\d{4})-M(\d{2})$/.exec(key);
  if (monthly) {
    const date = new Date(Date.UTC(Number(monthly[1]), Number(monthly[2]) - 1, 1));
    return date.toLocaleDateString(undefined, { month: 'long', year: 'numeric', timeZone: 'UTC' });
  }

  const quarterly = /^(\d{4})-Q([1-4])$/.exec(key);
  if (quarterly) {
    return `Q${quarterly[2]} ${quarterly[1]}`;
  }

  const annual = /^(\d{4})-Y$/.exec(key);
  if (annual) {
    return annual[1] ?? key;
  }

  return key;
}

/** A short label for chart axes, where space is tight. */
export function periodKeyShortLabel(key: string): string {
  const daily = /^\d{4}-(\d{2})-(\d{2})$/.exec(key);
  if (daily) {
    return `${daily[2]}/${daily[1]}`;
  }
  const weekly = /^\d{4}-W(\d{2})$/.exec(key);
  if (weekly) {
    return `W${weekly[1]}`;
  }
  const monthly = /^(\d{4})-M(\d{2})$/.exec(key);
  if (monthly) {
    return `${monthly[2]}/${monthly[1]?.slice(2)}`;
  }
  return periodKeyLabel(key);
}

/** Formats a rate in the range 0..1 as a whole percentage. */
export function percent(rate: number): string {
  if (!Number.isFinite(rate)) return '0%';
  return `${Math.round(rate * 100)}%`;
}

/** The browser's own IANA zone, which is what a person sees with no preference set. */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/**
 * Runs a formatter in `timeZone`, falling back to the browser's zone if the
 * runtime rejects the name. A stale or unsupported stored preference should cost
 * someone their preferred zone, not break the screen that shows a timestamp.
 */
function inZone(timeZone: string | undefined, format: (zone: string | undefined) => string): string {
  if (!timeZone) return format(undefined);
  try {
    return format(timeZone);
  } catch (error) {
    if (error instanceof RangeError) return format(undefined);
    throw error;
  }
}

/**
 * Formats an ISO timestamp as a date and time, or '' when absent.
 *
 * `timeZone` is the viewer's display zone. It only changes how the instant is
 * written; omit it for the browser's own zone. It is not for period keys, which
 * are calendar labels (see periodKeyLabel).
 */
export function dateTime(iso: string | undefined, timeZone?: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return inZone(timeZone, (zone) =>
    date.toLocaleString(undefined, {
      day: 'numeric',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
      timeZone: zone,
    }),
  );
}

/** Formats just the clock time of an ISO timestamp (24-hour, "09:00") in `timeZone`. */
export function clockTime(iso: string | undefined, timeZone?: string): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return inZone(timeZone, (zone) =>
    date.toLocaleTimeString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
      timeZone: zone,
    }),
  );
}

/** A small fallback for runtimes without Intl.supportedValuesOf. */
const COMMON_TIME_ZONES = [
  'UTC',
  'America/Los_Angeles',
  'America/Denver',
  'America/Chicago',
  'America/New_York',
  'America/Sao_Paulo',
  'Europe/London',
  'Europe/Paris',
  'Europe/Berlin',
  'Africa/Johannesburg',
  'Asia/Dubai',
  'Asia/Kolkata',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Sydney',
  'Pacific/Auckland',
];

/**
 * The IANA zones to offer in a picker, alphabetically, always including `keep`
 * (the value already stored) so editing a record never silently drops its zone.
 *
 * `Intl.supportedValuesOf` is missing from older browsers and some test
 * runtimes, hence the guard. "UTC" is added because some engines leave it out of
 * the list although the backend accepts it.
 */
export function timeZoneOptions(keep?: string): string[] {
  let zones: string[] = [];
  try {
    const supported = (Intl as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf;
    zones = supported ? supported('timeZone') : [];
  } catch {
    zones = [];
  }
  if (zones.length === 0) zones = COMMON_TIME_ZONES;

  const all = new Set(zones);
  all.add('UTC');
  if (keep) all.add(keep);
  return [...all].sort((a, b) => a.localeCompare(b));
}

/**
 * How long until the period closes, in words.
 *
 * This is the "you have until" hint on a card. It rounds down and stays vague on
 * purpose; a live countdown would imply more precision than the ritual needs.
 */
export function timeUntil(iso: string, now: Date = new Date()): string {
  const end = new Date(iso).getTime();
  if (Number.isNaN(end)) return '';

  const ms = end - now.getTime();
  if (ms <= 0) return 'closing now';

  const minutes = Math.floor(ms / 60_000);
  if (minutes < 60) return `${minutes}m left`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h left`;

  const days = Math.floor(hours / 24);
  if (days < 14) return `${days}d left`;

  const weeks = Math.floor(days / 7);
  if (weeks < 9) return `${weeks}w left`;

  const months = Math.floor(days / 30);
  return `${months}mo left`;
}

/** Milliseconds until the soonest period boundary among the given periods. */
export function msUntilNextBoundary(
  periods: Iterable<Period>,
  now: Date = new Date(),
): number | null {
  let soonest = Number.POSITIVE_INFINITY;

  for (const period of periods) {
    const end = new Date(period.end).getTime();
    if (Number.isNaN(end)) continue;
    const delta = end - now.getTime();
    if (delta > 0 && delta < soonest) {
      soonest = delta;
    }
  }

  return Number.isFinite(soonest) ? soonest : null;
}
