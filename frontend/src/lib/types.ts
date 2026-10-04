// Shapes returned by the backend.
//
// These mirror the DTOs in backend/internal/api/dto.go. Keeping them narrow and
// explicit means a backend change that drops a field shows up as a type error
// here rather than as undefined at runtime.

export type Cadence = 'daily' | 'weekly' | 'monthly' | 'quarterly' | 'annual';

export const CADENCES: readonly Cadence[] = [
  'daily',
  'weekly',
  'monthly',
  'quarterly',
  'annual',
] as const;

/**
 * A card's state within one period.
 *
 * `not_started` is never stored by the backend; it is the absence of an
 * occurrence row, which is how the board flips without anything being written.
 */
export type Status = 'not_started' | 'in_progress' | 'done';

export interface Period {
  cadence: Cadence;
  key: string;
  start: string;
  end: string;
  label: string;
}

export interface UserRef {
  id: string;
  name: string;
  email: string;
}

export interface CardState {
  status: Status;
  periodKey: string;
  startedBy?: UserRef;
  startedAt?: string;
  completedBy?: UserRef;
  completedAt?: string;
  notes?: string;
}

export interface CardLink {
  label: string;
  url: string;
}

export interface ChecklistItem {
  text: string;
}

export interface Card {
  id: string;
  boardId: string;
  title: string;
  summary: string;
  cadence: Cadence;
  /** Rich text from the editor field. Must be sanitised before rendering. */
  instructions: string;
  links: CardLink[];
  checklist: ChecklistItem[];
  sortOrder: number;
  archived: boolean;
  archivedAt?: string;
  period: Period;
  state: CardState;
}

export interface Board {
  id: string;
  teamId: string;
  name: string;
  description: string;
  archived: boolean;
}

export interface BoardTally {
  total: number;
  done: number;
  inProgress: number;
  notStarted: number;
  completionRate: number;
}

export interface BoardState {
  board: Board;
  periods: Partial<Record<Cadence, Period>>;
  cards: Card[];
  timezone: string;
  serverAt: string;
  summary: BoardTally;
}

export interface CurrentPeriods {
  timezone: string;
  serverAt: string;
  periods: Partial<Record<Cadence, Period>>;
}

export interface MutationResult {
  cardId: string;
  period: Period;
  state: CardState;
}

/** Where a report figure came from: a frozen snapshot, or computed on the fly. */
export type ReportSource = 'rollup' | 'live';

export interface ReportPoint {
  periodKey: string;
  periodStart: string;
  periodEnd: string;
  total: number;
  done: number;
  inProgress: number;
  notStarted: number;
  completionRate: number;
  source: ReportSource;
}

export interface CardReport {
  cardId: string;
  title: string;
  cadence: Cadence;
  archived: boolean;
  expected: number;
  done: number;
  completionRate: number;
}

export interface ReportTotals {
  periods: number;
  expected: number;
  done: number;
  completionRate: number;
}

export interface Report {
  board: Board;
  cadence: Cadence;
  timezone: string;
  series: ReportPoint[];
  current?: ReportPoint;
  cards: CardReport[];
  totals: ReportTotals;
}

/**
 * One (day, cadence) bucket of the completions heatmap.
 *
 * `date` is YYYY-MM-DD in the board's timezone, already bucketed by the server.
 * The browser must not re-bucket timestamps itself.
 */
export interface ActivityDay {
  date: string;
  cadence: Cadence;
  completed: number;
}

/** Every day on record for a board, for the completions heatmap. */
export interface ActivityReport {
  board: Board;
  timezone: string;
  days: ActivityDay[];
  total: number;
}

// ---------------------------------------------------------------------------
// Collection records, as returned by PocketBase's generated API
// ---------------------------------------------------------------------------

export interface TeamRecord {
  id: string;
  name: string;
  description: string;
  members: string[];
  /** Ascending display order in the sidebar. */
  sort_order: number;
  archived_at: string;
}

export interface BoardRecord {
  id: string;
  team: string;
  name: string;
  description: string;
  sort_order: number;
  archived_at: string;
}

/** Response of POST /api/kamishibai/boards/{id}/move. */
export interface MoveBoardResult {
  boardId: string;
  teamId: string;
  /** How many child rows were re-pointed at the new team. */
  moved: { cards: number; occurrences: number; rollups: number };
}

/** Body of POST /api/kamishibai/navigation/order. Both keys are optional. */
export interface NavigationOrder {
  /** Team ids in display order. */
  teams?: string[];
  /** Board ids in display order, keyed by the team that currently owns them. */
  boards?: Record<string, string[]>;
}

export interface CurrentUser {
  id: string;
  name: string;
  email: string;
  role: string;
}

export function isAdmin(user: CurrentUser | null): boolean {
  return user?.role === 'admin';
}
