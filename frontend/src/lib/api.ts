import { pb } from './pocketbase';
import type {
  BoardRecord,
  BoardState,
  Cadence,
  CurrentPeriods,
  MutationResult,
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

  start(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'start', notes);
  },

  complete(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'complete', notes);
  },

  reopen(cardId: string, notes?: string): Promise<MutationResult> {
    return mutate(cardId, 'reopen', notes);
  },

  /** Teams the signed-in user can see. Admins see all of them. */
  teams(): Promise<TeamRecord[]> {
    return pb.collection('teams').getFullList<TeamRecord>({ sort: 'name' });
  },

  /** Active boards for a team. */
  boards(teamId: string): Promise<BoardRecord[]> {
    return pb.collection('boards').getFullList<BoardRecord>({
      filter: pb.filter('team = {:team} && archived_at = ""', { team: teamId }),
      sort: 'sort_order,name',
    });
  },

  createBoard(teamId: string, name: string, description: string): Promise<BoardRecord> {
    return pb.collection('boards').create<BoardRecord>({
      team: teamId,
      name,
      description,
    });
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
      archived_at: new Date().toISOString().replace('T', ' ').replace('Z', 'Z'),
    });
  },

  /** Restore an archived card. The backend permits this for admins only. */
  restoreCard(cardId: string): Promise<unknown> {
    return pb.collection('cards').update(cardId, { archived_at: '' });
  },
};

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
