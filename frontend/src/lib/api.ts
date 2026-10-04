import { pb } from './pocketbase';
import type {
  ActivityReport,
  BoardRecord,
  BoardState,
  Cadence,
  CurrentPeriods,
  MoveBoardResult,
  MutationResult,
  NavigationOrder,
  Report,
  TeamRecord,
} from './types';

/**
 * Typed wrappers around the backend endpoints.
 *
 * Recording work goes through the purpose-built /api/kamishibai routes rather
 * than writing to the occurrences collection, because the server is what decides
 * which period is open and who the work is attributed to. The client cannot send
 * either, by design: the only thing these calls carry is an optional note.
 */
export const api = {
  /**
   * Which period each cadence is currently in, according to the server.
   *
   * The browser never computes period keys itself. ISO week numbering and
   * daylight-saving boundaries are fiddly enough that a second implementation
   * would eventually disagree with the first, and then the board would show a
   * different day's work to different people.
   */
  currentPeriods(): Promise<CurrentPeriods> {
    return pb.send<CurrentPeriods>('/api/kamishibai/periods/current', { method: 'GET' });
  },

  /** A board, its cards, and each card's status for the period it is in. */
  boardState(boardId: string, options?: { includeArchived?: boolean }): Promise<BoardState> {
    return pb.send<BoardState>(`/api/kamishibai/boards/${encodeURIComponent(boardId)}/state`, {
      method: 'GET',
      query: options?.includeArchived ? { includeArchived: 'true' } : undefined,
    });
  },

  /** Completion history for a board and cadence. */
  report(boardId: string, cadence: Cadence, periods = 12): Promise<Report> {
    return pb.send<Report>(`/api/kamishibai/boards/${encodeURIComponent(boardId)}/report`, {
      method: 'GET',
      query: { cadence, periods },
    });
  },

  /**
   * Completions per calendar day for a board, for the heatmap.
   *
   * The server buckets by day in the board's timezone; the response is every day
   * on record, so there are no query parameters.
   */
  activity(boardId: string): Promise<ActivityReport> {
    return pb.send<ActivityReport>(
      `/api/kamishibai/boards/${encodeURIComponent(boardId)}/activity`,
      { method: 'GET' },
    );
  },

  start(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'start', notes);
  },

  complete(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'complete', notes);
  },

  reopen(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'reopen', notes);
  },

  /**
   * Teams the signed-in user can see, in sidebar order. Admins see all of them,
   * archived ones included: callers filter on `archived_at`.
   */
  teams(): Promise<TeamRecord[]> {
    return pb.collection('teams').getFullList<TeamRecord>({ sort: 'sort_order,name' });
  },

  /** Active boards for a team, or every board (archived too) with `includeArchived`. */
  boards(teamId: string, options?: { includeArchived?: boolean }): Promise<BoardRecord[]> {
    const filter = options?.includeArchived
      ? pb.filter('team = {:team}', { team: teamId })
      : pb.filter('team = {:team} && archived_at = ""', { team: teamId });
    return pb.collection('boards').getFullList<BoardRecord>({ filter, sort: 'sort_order,name' });
  },

  // The calls below are admin-only on the server. The sidebar editor is the only
  // caller and is itself admin-only, but the server is what actually enforces it.

  /** Create a team. `sortOrder` places it in the sidebar (omit for the default). */
  createTeam(name: string, description: string, sortOrder?: number): Promise<TeamRecord> {
    return pb.collection('teams').create<TeamRecord>({
      name,
      description,
      ...(sortOrder === undefined ? {} : { sort_order: sortOrder }),
    });
  },

  updateTeam(id: string, input: { name: string; description: string }): Promise<TeamRecord> {
    return pb.collection('teams').update<TeamRecord>(id, {
      name: input.name,
      description: input.description,
    });
  },

  /** Archive a team. Its boards stay as they are and reappear if it is restored. */
  archiveTeam(id: string): Promise<unknown> {
    return pb.collection('teams').update(id, { archived_at: archivedStamp() });
  },

  restoreTeam(id: string): Promise<unknown> {
    return pb.collection('teams').update(id, { archived_at: '' });
  },

  /** Create a board. `sortOrder` should place it after the team's other boards. */
  createBoard(
    teamId: string,
    name: string,
    description: string,
    sortOrder?: number,
  ): Promise<BoardRecord> {
    return pb.collection('boards').create<BoardRecord>({
      team: teamId,
      name,
      description,
      ...(sortOrder === undefined ? {} : { sort_order: sortOrder }),
    });
  },

  /** Rename or re-describe a board. `team` is not sent: use moveBoard. */
  updateBoard(id: string, input: { name: string; description: string }): Promise<BoardRecord> {
    return pb.collection('boards').update<BoardRecord>(id, {
      name: input.name,
      description: input.description,
    });
  },

  archiveBoard(id: string): Promise<unknown> {
    return pb.collection('boards').update(id, { archived_at: archivedStamp() });
  },

  restoreBoard(id: string): Promise<unknown> {
    return pb.collection('boards').update(id, { archived_at: '' });
  },

  /**
   * Move a board, and everything recorded against it, to another team.
   *
   * This is an endpoint rather than a PATCH of `team` because the board's cards,
   * occurrences and rollups each carry their own copy of the team id, and all of
   * them have to change in one transaction. The generic update refuses a team
   * change for exactly that reason.
   */
  moveBoard(boardId: string, teamId: string): Promise<MoveBoardResult> {
    return pb.send<MoveBoardResult>(
      `/api/kamishibai/boards/${encodeURIComponent(boardId)}/move`,
      { method: 'POST', body: { team: teamId } },
    );
  },

  /** Persist sidebar order: sort_order becomes each id's index in its array. */
  saveOrder(order: NavigationOrder): Promise<unknown> {
    return pb.send('/api/kamishibai/navigation/order', { method: 'POST', body: order });
  },

  /**
   * Create a card.
   *
   * `team` is deliberately not sent. The backend derives it from the board and
   * ignores any client-supplied value, so sending one would be misleading.
   */
  createCard(input: {
    boardId: string;
    title: string;
    summary: string;
    cadence: Cadence;
    instructions: string;
    links: { label: string; url: string }[];
    checklist: { text: string }[];
    sortOrder: number;
  }): Promise<{ id: string }> {
    return pb.collection('cards').create<{ id: string }>({
      board: input.boardId,
      title: input.title,
      summary: input.summary,
      cadence: input.cadence,
      instructions: input.instructions,
      links: input.links,
      checklist: input.checklist,
      sort_order: input.sortOrder,
    });
  },

  /** Archive a card. Any team member may do this. */
  archiveCard(cardId: string): Promise<unknown> {
    return pb.collection('cards').update(cardId, {
      archived_at: archivedStamp(),
    });
  },

  /** Restore an archived card. The backend permits this for admins only. */
  restoreCard(cardId: string): Promise<unknown> {
    return pb.collection('cards').update(cardId, { archived_at: '' });
  },
};

/** The timestamp format PocketBase date fields accept ("2026-09-15 12:00:00.000Z"). */
function archivedStamp(): string {
  return new Date().toISOString().replace('T', ' ');
}

function mutate(cardId: string, action: string, notes?: string): Promise<MutationResult> {
  return pb.send<MutationResult>(
    `/api/kamishibai/cards/${encodeURIComponent(cardId)}/${action}`,
    { method: 'POST', body: notes ? { notes } : {} },
  );
}

/**
 * Turns an unknown thrown value into something worth showing a person.
 *
 * PocketBase rejects with a ClientResponseError carrying the server's message,
 * which is more useful than a generic failure notice because the backend already
 * explains itself ("already complete for this period", "archived and read-only").
 */
export function errorMessage(error: unknown, fallback = 'Something went wrong.'): string {
  if (typeof error === 'object' && error !== null) {
    const candidate = error as { response?: { message?: string }; message?: string };
    const fromResponse = candidate.response?.message;
    if (typeof fromResponse === 'string' && fromResponse.length > 0) {
      return fromResponse;
    }
    if (typeof candidate.message === 'string' && candidate.message.length > 0) {
      return candidate.message;
    }
  }
  return fallback;
}
