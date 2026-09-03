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
 */
export function periodKeyLabel(key: string): string {
  const daily = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
  if (daily) {
    const [, year, month, day] = daily;
    const date = new Date(Number(year), Number(month) - 1, Number(day));
    return date.toLocaleDateString(undefined, {
      weekday: 'short',
      day: 'numeric',
      month: 'short',
      year: 'numeric',
    });
  }

  const weekly = /^(\d{4})-W(\d{2})$/.exec(key);
  if (weekly) {
    return `Week ${Number(weekly[2])}, ${weekly[1]}`;
  }

  const monthly = /^(\d{4})-M(\d{2})$/.exec(key);
  if (monthly) {
    const date = new Date(Number(monthly[1]), Number(monthly[2]) - 1, 1);
    return date.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
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

/** Formats an ISO timestamp as a local date and time, or '' when absent. */
export function dateTime(iso: string | undefined): string {
  if (!iso) return '';
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return '';
  return date.toLocaleString(undefined, {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
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
