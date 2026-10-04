// An in-memory stand-in for the backend, wired onto the mocked `api`.
//
// .storybook/preview.tsx mocks src/lib/api.ts in spy mode, so each method is a
// real spy that a story (or this file) can give an implementation. The preview
// installs the default backend before every story; a story that needs something
// different calls installFakeBackend(seed) or mocks one method in its own
// beforeEach, which runs afterwards and so wins.
import { mocked } from 'storybook/test';
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

  mocked(api.currentPeriods).mockImplementation(
    async (): Promise<CurrentPeriods> => ({
      timezone: TIMEZONE,
      serverAt: SERVER_AT,
      periods: { daily: periodFor('daily'), weekly: periodFor('weekly') },
    }),
  );

  mocked(api.teams).mockImplementation(async () => db.teams);

  mocked(api.boards).mockImplementation(async (teamId) => db.boardsByTeam[teamId] ?? []);

  mocked(api.boardState).mockImplementation(async (boardId, options): Promise<BoardState> => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    const cards = options?.includeArchived
      ? entry.cards
      : entry.cards.filter((card) => !card.archived);
    return makeBoardState(cards, entry.board);
  });

  mocked(api.report).mockImplementation(async (boardId, cadence): Promise<Report> => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    const report = db.reports[cadence] ?? { ...REPORT_WEEKLY, cadence };
    return { ...report, board: entry.board };
  });

  mocked(api.activity).mockImplementation(async (boardId) => {
    const entry = db.boards[boardId];
    if (!entry) return notFound();
    return makeActivity(entry.board, entry.activity);
  });

  mocked(api.start).mockImplementation(transition('start'));
  mocked(api.complete).mockImplementation(transition('complete'));
  mocked(api.reopen).mockImplementation(transition('reopen'));

  mocked(api.archiveCard).mockImplementation(async (cardId) => {
    const card = find(cardId);
    if (!card) return notFound();
    card.archived = true;
    card.archivedAt = SERVER_AT;
    return card;
  });

  mocked(api.restoreCard).mockImplementation(async (cardId) => {
    const card = find(cardId);
    if (!card) return notFound();
    card.archived = false;
    delete card.archivedAt;
    return card;
  });

  mocked(api.createCard).mockImplementation(async (input) => {
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
