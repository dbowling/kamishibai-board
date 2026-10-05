// Sample data for stories.
//
// Every date is fixed and every period has already ended, so nothing here depends
// on the clock: cards read "closing now", and useBoardState never arms a timer to
// refetch at a period boundary.
import type {
  ActivityDay,
  ActivityReport,
  Board,
  BoardRecord,
  BoardState,
  Cadence,
  Card,
  CardState,
  Period,
  Report,
  ReportPoint,
  TeamRecord,
  UserRef,
} from '../lib/types';

export const USERS = {
  admin: { id: 'uadmin', name: 'Avery Admin', email: 'avery@example.test', role: 'admin' },
  dana: { id: 'udana', name: 'Dana Ops', email: 'dana@example.test', role: 'user' },
} as const;

export const DANA: UserRef = { id: USERS.dana.id, name: USERS.dana.name, email: USERS.dana.email };
export const AVERY: UserRef = {
  id: USERS.admin.id,
  name: USERS.admin.name,
  email: USERS.admin.email,
};

export const TEAMS: TeamRecord[] = [
  { id: 't1', name: 'Platform', description: '', members: [USERS.dana.id], sort_order: 1, archived_at: '' },
  // Support runs on its own clock, so moving a board between Platform (which
  // inherits the instance default) and Support crosses time zones.
  {
    id: 't2',
    name: 'Support',
    description: '',
    members: [USERS.dana.id],
    sort_order: 2,
    archived_at: '',
    timezone: 'Asia/Tokyo',
  },
  // Archived: never in the sidebar, only under "Archived" while an admin edits it.
  {
    id: 't3',
    name: 'Legacy',
    description: '',
    members: [],
    sort_order: 3,
    archived_at: '2026-08-01 00:00:00.000Z',
  },
];

export const BOARDS_BY_TEAM: Record<string, BoardRecord[]> = {
  t1: [
    {
      id: 'b1',
      team: 't1',
      name: 'Platform triage',
      description: 'The daily and weekly rounds.',
      sort_order: 1,
      archived_at: '',
    },
    { id: 'b2', team: 't1', name: 'Security checks', description: '', sort_order: 2, archived_at: '' },
    {
      id: 'b4',
      team: 't1',
      name: 'Retired checks',
      description: '',
      sort_order: 3,
      archived_at: '2026-08-01 00:00:00.000Z',
    },
  ],
  t2: [{ id: 'b3', team: 't2', name: 'Support rota', description: '', sort_order: 1, archived_at: '' }],
  t3: [{ id: 'b5', team: 't3', name: 'Old rota', description: '', sort_order: 1, archived_at: '' }],
};

export const BOARD: Board = {
  id: 'b1',
  teamId: 't1',
  name: 'Platform triage',
  description: 'The daily and weekly rounds.',
  archived: false,
};

export const TIMEZONE = 'America/New_York';
export const SERVER_AT = '2026-09-15T12:00:00Z';

const PERIODS: Record<Cadence, Period> = {
  daily: { cadence: 'daily', key: '2026-09-15', start: '2026-09-15T04:00:00Z', end: '2026-09-16T04:00:00Z', label: 'Daily' },
  weekly: { cadence: 'weekly', key: '2026-W38', start: '2026-09-14T04:00:00Z', end: '2026-09-21T04:00:00Z', label: 'Weekly' },
  monthly: { cadence: 'monthly', key: '2026-M09', start: '2026-09-01T04:00:00Z', end: '2026-10-01T04:00:00Z', label: 'Monthly' },
  quarterly: { cadence: 'quarterly', key: '2026-Q3', start: '2026-07-01T04:00:00Z', end: '2026-10-01T04:00:00Z', label: 'Quarterly' },
  // Ended in the past like the rest, so no boundary timer is ever armed.
  annual: { cadence: 'annual', key: '2025-Y', start: '2025-01-01T05:00:00Z', end: '2026-01-01T05:00:00Z', label: 'Annual' },
};

export function periodFor(cadence: Cadence): Period {
  return { ...PERIODS[cadence] };
}

export const INSTRUCTIONS_HTML =
  '<p>Open the <strong>backup dashboard</strong> and confirm last night&rsquo;s run finished.</p>' +
  '<ul><li>All jobs green</li><li>Storage under 80%</li></ul>' +
  '<p>If a job failed, re-run it and post in the on-call channel.</p>';

/** Instructions carrying payloads the sanitiser must neutralise. */
export const UNSAFE_INSTRUCTIONS_HTML =
  '<p>Safe paragraph.</p>' +
  '<script>window.__pwned = true</script>' +
  '<img src="x" onerror="window.__pwned = true">' +
  '<a href="javascript:window.__pwned = true">bad link</a>';

export function makeCard(overrides: Partial<Card> = {}, state: Partial<CardState> = {}): Card {
  const cadence = overrides.cadence ?? 'daily';
  const period = overrides.period ?? periodFor(cadence);
  return {
    id: 'c1',
    boardId: 'b1',
    title: 'Verify backups',
    summary: 'Confirm last night’s backups completed.',
    cadence,
    instructions: INSTRUCTIONS_HTML,
    links: [
      { label: 'Backup dashboard', url: 'https://grafana.example.test/d/backups' },
      { label: 'On-call runbook', url: 'https://wiki.example.test/runbooks/backups' },
    ],
    checklist: [
      { text: 'Check the backup job logs' },
      { text: 'Confirm storage capacity' },
      { text: 'Spot-check one restore' },
    ],
    sortOrder: 1,
    archived: false,
    ...overrides,
    period,
    state: { status: 'not_started', periodKey: period.key, ...state },
  };
}

export const NOT_STARTED_CARD = makeCard();

export const IN_PROGRESS_CARD = makeCard(
  {
    id: 'c2',
    title: 'Rotate on-call',
    summary: 'Hand the pager to next week’s owner.',
    cadence: 'weekly',
    instructions: '<p>Swap the pager schedule and tell the incoming engineer.</p>',
    links: [],
    checklist: [],
    sortOrder: 2,
  },
  { status: 'in_progress', startedBy: DANA, startedAt: '2026-09-15T13:30:00Z' },
);

export const DONE_CARD = makeCard(
  {
    id: 'c3',
    title: 'Patch review',
    summary: 'Review pending OS and dependency patches.',
    cadence: 'monthly',
    instructions: '<p>Triage the patch queue and schedule anything urgent.</p>',
    links: [],
    checklist: [],
    sortOrder: 3,
  },
  {
    status: 'done',
    startedBy: DANA,
    startedAt: '2026-09-02T14:00:00Z',
    completedBy: AVERY,
    completedAt: '2026-09-02T16:45:00Z',
    notes: 'Two patches deferred to next sprint.',
  },
);

export const QUARTERLY_CARD = makeCard({
  id: 'c4',
  title: 'Quarterly access audit',
  summary: 'Review who has access to what.',
  cadence: 'quarterly',
  instructions: '<p>Export the access list and have each owner confirm it.</p>',
  links: [],
  checklist: [],
  sortOrder: 4,
});

export const ARCHIVED_CARD = makeCard({
  id: 'c5',
  title: 'Retired VPN check',
  summary: 'No longer needed after the migration.',
  cadence: 'weekly',
  instructions: '<p>Kept for its history.</p>',
  links: [],
  checklist: [],
  sortOrder: 5,
  archived: true,
  archivedAt: '2026-08-20T15:00:00Z',
});

export const CARDS: Card[] = [NOT_STARTED_CARD, IN_PROGRESS_CARD, DONE_CARD, QUARTERLY_CARD];

/** A board's state for the given cards. Archived cards are left out of the tally. */
export function makeBoardState(cards: Card[] = CARDS, board: Board = BOARD): BoardState {
  const live = cards.filter((card) => !card.archived);
  const done = live.filter((card) => card.state.status === 'done').length;
  const inProgress = live.filter((card) => card.state.status === 'in_progress').length;

  const periods: BoardState['periods'] = {};
  for (const card of live) periods[card.cadence] = card.period;

  return {
    board,
    periods,
    cards,
    timezone: TIMEZONE,
    serverAt: SERVER_AT,
    summary: {
      total: live.length,
      done,
      inProgress,
      notStarted: live.length - done - inProgress,
      completionRate: live.length === 0 ? 0 : done / live.length,
    },
  };
}

function point(week: number, done: number, total: number, source: ReportPoint['source'] = 'rollup'): ReportPoint {
  const start = new Date(Date.UTC(2026, 7, 3 + (week - 32) * 7));
  const end = new Date(start.getTime() + 7 * 24 * 3600 * 1000);
  return {
    periodKey: `2026-W${String(week).padStart(2, '0')}`,
    periodStart: start.toISOString(),
    periodEnd: end.toISOString(),
    total,
    done,
    inProgress: Math.min(1, total - done),
    notStarted: Math.max(0, total - done - 1),
    completionRate: total === 0 ? 0 : done / total,
    source,
  };
}

export const REPORT_WEEKLY: Report = {
  board: BOARD,
  cadence: 'weekly',
  timezone: TIMEZONE,
  series: [point(32, 3, 4), point(33, 4, 4), point(34, 2, 4), point(35, 3, 4), point(36, 4, 4), point(37, 1, 4)],
  current: point(38, 2, 4, 'live'),
  // Least reliable first, as the API returns them.
  cards: [
    { cardId: 'c2', title: 'Rotate on-call', cadence: 'weekly', archived: false, expected: 7, done: 4, completionRate: 4 / 7 },
    { cardId: 'c5', title: 'Retired VPN check', cadence: 'weekly', archived: true, expected: 4, done: 3, completionRate: 0.75 },
    { cardId: 'c1', title: 'Verify backups', cadence: 'weekly', archived: false, expected: 7, done: 7, completionRate: 1 },
  ],
  totals: { periods: 7, expected: 18, done: 14, completionRate: 14 / 18 },
};

export const REPORT_EMPTY: Report = {
  board: BOARD,
  cadence: 'weekly',
  timezone: TIMEZONE,
  series: [],
  cards: [],
  totals: { periods: 0, expected: 0, done: 0, completionRate: 0 },
};

/**
 * Completions per day. The first rows match the heatmap unit tests: 2026-09-03 is
 * 3 daily + 1 weekly (4 in all) and 2026-09-04 is 2 daily.
 */
export const ACTIVITY_DAYS: ActivityDay[] = [
  { date: '2026-09-03', cadence: 'daily', completed: 3 },
  { date: '2026-09-03', cadence: 'weekly', completed: 1 },
  { date: '2026-09-04', cadence: 'daily', completed: 2 },
  { date: '2026-09-07', cadence: 'daily', completed: 1 },
  { date: '2026-09-08', cadence: 'daily', completed: 4 },
  { date: '2026-09-10', cadence: 'weekly', completed: 2 },
  { date: '2026-09-11', cadence: 'daily', completed: 6 },
  { date: '2026-09-14', cadence: 'daily', completed: 9 },
  { date: '2026-08-12', cadence: 'monthly', completed: 1 },
  { date: '2026-08-20', cadence: 'daily', completed: 2 },
  { date: '2026-07-01', cadence: 'quarterly', completed: 1 },
];

export function makeActivity(board: Board = BOARD, days: ActivityDay[] = ACTIVITY_DAYS): ActivityReport {
  return {
    board,
    timezone: TIMEZONE,
    days,
    total: days.reduce((sum, day) => sum + day.completed, 0),
  };
}
