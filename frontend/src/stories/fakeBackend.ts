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
      b4: { ...secondBoard('b4', 't1', 'Retired checks'), board: { id: 'b4', teamId: 't1', name: 'Retired checks', description: '', archived: true } },
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

  // Navigation. The real server sorts by sort_order then name and enforces the
  // rules below; the fake mirrors the ones a story can reasonably hit.
  const bySortOrder = <T extends { sort_order: number; name: string }>(a: T, b: T) =>
    a.sort_order - b.sort_order || a.name.localeCompare(b.name);
  const findBoardRecord = (id: string) => {
    for (const list of Object.values(db.boardsByTeam)) {
      const record = list.find((candidate) => candidate.id === id);
      if (record) return record;
    }
    return undefined;
  };
  const findTeam = (id: string) => db.teams.find((team) => team.id === id);
  // Records are replaced rather than mutated: the app holds on to what it was given.
  const patchBoard = (id: string, patch: Partial<BoardRecord>) => {
    const record = findBoardRecord(id);
    if (!record) return undefined;
    const list = db.boardsByTeam[record.team] ?? [];
    const next = { ...record, ...patch };
    db.boardsByTeam[record.team] = list.map((candidate) => (candidate.id === id ? next : candidate));
    const entry = db.boards[id];
    if (entry) {
      entry.board = {
        ...entry.board,
        name: next.name,
        description: next.description,
        archived: Boolean(next.archived_at),
      };
    }
    return next;
  };
  const patchTeam = (id: string, patch: Partial<TeamRecord>) => {
    const team = findTeam(id);
    if (!team) return undefined;
    const next = { ...team, ...patch };
    db.teams = db.teams.map((candidate) => (candidate.id === id ? next : candidate));
    return next;
  };
  let created = 0;

  stub('teams').mockImplementation(async () => [...db.teams].sort(bySortOrder));

  stub('boards').mockImplementation(async (teamId, options) =>
    (db.boardsByTeam[teamId] ?? [])
      .filter((board) => options?.includeArchived || !board.archived_at)
      .sort(bySortOrder),
  );

  stub('createTeam').mockImplementation(async (name, description, sortOrder) => {
    const team: TeamRecord = {
      id: `nt${++created}`,
      name,
      description,
      members: [],
      sort_order: sortOrder ?? 0,
      archived_at: '',
    };
    db.teams = [...db.teams, team];
    db.boardsByTeam[team.id] = [];
    return team;
  });

  stub('updateTeam').mockImplementation(async (id, input) => patchTeam(id, input) ?? notFound());
  stub('archiveTeam').mockImplementation(
    async (id) => patchTeam(id, { archived_at: SERVER_AT }) ?? notFound(),
  );
  stub('restoreTeam').mockImplementation(async (id) => patchTeam(id, { archived_at: '' }) ?? notFound());

  stub('createBoard').mockImplementation(async (teamId, name, description, sortOrder) => {
    if (!findTeam(teamId)) return failure('team: invalid team.');
    const id = `nb${++created}`;
    const record: BoardRecord = {
      id,
      team: teamId,
      name,
      description,
      sort_order: sortOrder ?? 0,
      archived_at: '',
    };
    db.boardsByTeam[teamId] = [...(db.boardsByTeam[teamId] ?? []), record];
    db.boards[id] = { board: { id, teamId, name, description, archived: false }, cards: [], activity: [] };
    return record;
  });

  stub('updateBoard').mockImplementation(async (id, input) => patchBoard(id, input) ?? notFound());
  stub('archiveBoard').mockImplementation(
    async (id) => patchBoard(id, { archived_at: SERVER_AT }) ?? notFound(),
  );
  stub('restoreBoard').mockImplementation(async (id) => patchBoard(id, { archived_at: '' }) ?? notFound());

  stub('moveBoard').mockImplementation(async (boardId, teamId) => {
    const board = findBoardRecord(boardId);
    if (!board) return notFound();
    const target = findTeam(teamId);
    const source = findTeam(board.team);
    if (!target) return failure('Target team not found.');
    if (target.id === board.team) return failure('The board is already on that team.');
    if (board.archived_at) return failure('This board is archived.');
    if (target.archived_at || source?.archived_at) return failure('The team is archived.');
    const targetBoards = db.boardsByTeam[teamId] ?? [];
    if (targetBoards.some((other) => !other.archived_at && other.name === board.name)) {
      return failure('A board with that name already exists on the target team.');
    }
    db.boardsByTeam[board.team] = (db.boardsByTeam[board.team] ?? []).filter(
      (candidate) => candidate.id !== boardId,
    );
    const last = Math.max(0, ...targetBoards.map((other) => other.sort_order));
    db.boardsByTeam[teamId] = [...targetBoards, { ...board, team: teamId, sort_order: last + 1 }];
    const entry = db.boards[boardId];
    if (entry) entry.board = { ...entry.board, teamId };
    return {
      boardId,
      teamId,
      moved: { cards: db.boards[boardId]?.cards.length ?? 0, occurrences: 0, rollups: 0 },
    };
  });

  stub('saveOrder').mockImplementation(async (order) => {
    // Validate everything first: the real endpoint writes nothing on a 400.
    for (const id of order.teams ?? []) {
      if (!findTeam(id)) return failure(`Unknown team ${id}.`);
    }
    for (const [teamId, ids] of Object.entries(order.boards ?? {})) {
      for (const id of ids) {
        if (findBoardRecord(id)?.team !== teamId) return failure(`Board ${id} is not on team ${teamId}.`);
      }
    }
    (order.teams ?? []).forEach((id, index) => patchTeam(id, { sort_order: index }));
    for (const ids of Object.values(order.boards ?? {})) {
      ids.forEach((id, index) => patchBoard(id, { sort_order: index }));
    }
    return undefined;
  });

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
