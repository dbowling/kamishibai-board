// An in-memory stand-in for the backend, wired onto the mocked `api`.
//
// .storybook/preview.tsx mocks src/lib/api.ts in spy mode. Under Vitest that
// leaves every method a mock for good. In the Storybook dev server it does not:
// `api` is an object rather than a set of top-level exports, so its methods are
// ordinary spies, and Storybook restores those to the real functions before every
// story. stub() covers both, reusing the mock when there is one and spying on the
// method again when there is not. The preview installs the default backend before
// every story; a story that needs something different calls
// installFakeBackend(seed) or mocks one method in its own beforeEach with
// mocked(api.x), which runs afterwards and so wins.
import { isMockFunction, mocked, spyOn } from 'storybook/test';
import { api } from '../lib/api';
import type {
  ActivityDay,
  Board,
  BoardRecord,
  BoardState,
  Cadence,
  Card,
  CurrentPeriods,
  MutationResult,
  Report,
  TeamRecord,
} from '../lib/types';
import {
  ACTIVITY_DAYS,
  ARCHIVED_CARD,
  AVERY,
  BOARD,
  BOARDS_BY_TEAM,
  CARDS,
  DANA,
  REPORT_WEEKLY,
  SERVER_AT,
  TEAMS,
  TIMEZONE,
  makeActivity,
  makeBoardState,
  makeCard,
  periodFor,
} from './fixtures';

export interface FakeBoard {
  board: Board;
  cards: Card[];
  activity: ActivityDay[];
}

export interface Seed {
  teams: TeamRecord[];
  boardsByTeam: Record<string, BoardRecord[]>;
  boards: Record<string, FakeBoard>;
  /** Report returned for a cadence; cadences not listed reuse the weekly figures. */
  reports: Partial<Record<Cadence, Report>>;
}

export function defaultSeed(): Seed {
  const secondBoard = (id: string, teamId: string, name: string): FakeBoard => ({
    board: { id, teamId, name, description: '', archived: false },
    cards: [],
    activity: [],
  });

  return {
    teams: TEAMS,
    boardsByTeam: BOARDS_BY_TEAM,
    boards: {
      b1: { board: BOARD, cards: [...CARDS, ARCHIVED_CARD], activity: ACTIVITY_DAYS },
      b2: secondBoard('b2', 't1', 'Security checks'),
      b3: secondBoard('b3', 't2', 'Support rota'),
    },
    reports: {},
  };
}

/** A promise that never settles, for the loading states. */
export function pending<T>(): Promise<T> {
  return new Promise<T>(() => {});
}

/** A rejection shaped like PocketBase's, which errorMessage() knows how to read. */
export function failure(message: string): Promise<never> {
  return Promise.reject({ response: { message } });
}

type Api = typeof api;

function stub<K extends keyof Api>(method: K) {
  return isMockFunction(api[method]) ? mocked(api[method]) : spyOn(api, method);
}

function notFound(): Promise<never> {
  return failure('The requested resource wasn’t found.');
}

export function installFakeBackend(seed: Seed = defaultSeed()) {
  const db = structuredClone(seed);

  const find = (cardId: string) => {
    for (const entry of Object.values(db.boards)) {
      const card = entry.cards.find((candidate) => candidate.id === cardId);
      if (card) return card;
    }
    return undefined;
  };

  // Mutations flip the card in the database and answer like the real endpoints, so
  // the board's refetch afterwards shows the new state.
  const transition = (action: 'start' | 'complete' | 'reopen') =>
    async (cardId: string, notes?: string): Promise<MutationResult> => {
      const card = find(cardId);
      if (!card) return notFound();
      if (card.archived) return failure('This card is archived and read-only.');

      const now = SERVER_AT;
      if (action === 'start') {
        card.state = { ...card.state, status: 'in_progress', startedBy: DANA, startedAt: now };
      } else if (action === 'complete') {
        card.state = {
          ...card.state,
          status: 'done',
          startedBy: card.state.startedBy ?? DANA,
          startedAt: card.state.startedAt ?? now,
          completedBy: AVERY,
          completedAt: now,
        };
      } else {
        card.state = { periodKey: card.state.periodKey, status: 'not_started' };
      }
      if (notes) card.state.notes = notes;

      return { cardId, period: card.period, state: card.state };
    };

  stub('currentPeriods').mockImplementation(
    async (): Promise<CurrentPeriods> => ({
      timezone: TIMEZONE,
      serverAt: SERVER_AT,
      periods: { daily: periodFor('daily'), weekly: periodFor('weekly') },
    }),
  );

  stub('teams').mockImplementation(async () => db.teams);

  stub('boards').mockImplementation(async (teamId) => db.boardsByTeam[teamId] ?? []);

  stub('boardState').mockImplementation(async (boardId, options): Promise<BoardState> => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    const cards = options?.includeArchived
      ? entry.cards
      : entry.cards.filter((card) => !card.archived);
    return makeBoardState(cards, entry.board);
  });

  stub('report').mockImplementation(async (boardId, cadence): Promise<Report> => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    const report = db.reports[cadence] ?? { ...REPORT_WEEKLY, cadence };
    return { ...report, board: entry.board };
  });

  stub('activity').mockImplementation(async (boardId) => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    return makeActivity(entry.board, entry.activity);
  });

  stub('start').mockImplementation(transition('start'));
  stub('complete').mockImplementation(transition('complete'));
  stub('reopen').mockImplementation(transition('reopen'));

  stub('archiveCard').mockImplementation(async (cardId) => {
    const card = find(cardId);
    if (!card) return notFound();
    card.archived = true;
    card.archivedAt = SERVER_AT;
    return card;
  });

  stub('restoreCard').mockImplementation(async (cardId) => {
    const card = find(cardId);
    if (!card) return notFound();
    card.archived = false;
    delete card.archivedAt;
    return card;
  });

  stub('createCard').mockImplementation(async (input) => {
    const entry = db.boards[input.boardId];
    if (!entry) return notFound();
    const id = `new${entry.cards.length + 1}`;
    entry.cards.push(
      makeCard({
        id,
        boardId: input.boardId,
        title: input.title,
        summary: input.summary,
        cadence: input.cadence,
        instructions: input.instructions,
        links: input.links,
        checklist: input.checklist,
        sortOrder: input.sortOrder,
      }),
    );
    return { id };
  });

  return db;
}
