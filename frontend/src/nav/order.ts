import { arrayMove } from '@dnd-kit/sortable';
import type { TeamWithBoards } from '../boards/useBoards';
import type { NavigationOrder } from '../lib/types';

/**
 * Pure logic behind sidebar drag and drop.
 *
 * The editor turns a finished drag into a new layout here, so the rules (what
 * counts as a move between teams, where the board lands, what is sent to the
 * server) can be tested without rendering anything or simulating a drag.
 */

export type Layout = TeamWithBoards[];

// dnd-kit identifies draggables by one id space, and teams and boards are
// different tables whose ids could in principle collide, so each id is prefixed
// with what it is.
export type ItemKind = 'team' | 'board' | 'drop';

export const teamItemId = (id: string) => `team:${id}`;
export const boardItemId = (id: string) => `board:${id}`;
/** The drop target of a team with no boards, so a board can be dragged into it. */
export const dropItemId = (teamId: string) => `drop:${teamId}`;

export function parseItemId(raw: string | number): { kind: ItemKind; id: string } | null {
  const text = String(raw);
  const separator = text.indexOf(':');
  if (separator === -1) return null;
  const kind = text.slice(0, separator);
  if (kind !== 'team' && kind !== 'board' && kind !== 'drop') return null;
  return { kind, id: text.slice(separator + 1) };
}

/** The team that currently holds a board, if any. */
export function teamOfBoard(layout: Layout, boardId: string): string | undefined {
  return layout.find((entry) => entry.boards.some((board) => board.id === boardId))?.team.id;
}

/**
 * Gives every team and board a sort_order equal to its position, which is what
 * the server will store once the order is saved. Keeping the local copy in step
 * means the next "add board" computes its position from current numbers.
 */
function renumber(layout: Layout): Layout {
  return layout.map((entry, teamIndex) => ({
    team: { ...entry.team, sort_order: teamIndex },
    boards: entry.boards.map((board, boardIndex) => ({
      ...board,
      team: entry.team.id,
      sort_order: boardIndex,
    })),
  }));
}

/** A new team order after dragging one team onto another. Null when nothing changed. */
export function reorderTeams(layout: Layout, activeTeamId: string, overTeamId: string): Layout | null {
  const from = layout.findIndex((entry) => entry.team.id === activeTeamId);
  const to = layout.findIndex((entry) => entry.team.id === overTeamId);
  if (from === -1 || to === -1 || from === to) return null;
  return renumber(arrayMove(layout, from, to));
}

export interface BoardMove {
  boardId: string;
  fromTeamId: string;
  toTeamId: string;
}

export interface BoardDrop {
  layout: Layout;
  /** Set when the board changed team; it needs confirming and the move endpoint. */
  move: BoardMove | null;
}

/** Where a dragged board was released. */
export type DropTarget =
  | {
      kind: 'board';
      id: string;
      /**
       * Insert after the board it was released on rather than before. Only used
       * when changing team: within a team, array-move semantics already decide.
       */
      after?: boolean;
    }
  | { kind: 'team'; id: string };

/**
 * A new layout after dragging a board. Null when nothing changed.
 *
 * Within one team the board takes the position of the one it was dropped on.
 * Into another team it is inserted at that position (or appended when dropped on
 * the team itself) and `move` is set, because a team change is the one edit that
 * must go through the move endpoint.
 */
export function dropBoard(layout: Layout, boardId: string, target: DropTarget): BoardDrop | null {
  const fromTeamId = teamOfBoard(layout, boardId);
  if (!fromTeamId) return null;

  const toTeamId =
    target.kind === 'team' ? target.id : teamOfBoard(layout, target.id);
  if (!toTeamId) return null;

  const source = layout.find((entry) => entry.team.id === fromTeamId);
  const destination = layout.find((entry) => entry.team.id === toTeamId);
  const board = source?.boards.find((candidate) => candidate.id === boardId);
  if (!source || !destination || !board) return null;

  if (fromTeamId === toTeamId) {
    if (target.kind !== 'board' || target.id === boardId) return null;
    const from = source.boards.findIndex((candidate) => candidate.id === boardId);
    const to = source.boards.findIndex((candidate) => candidate.id === target.id);
    if (from === to) return null;
    const next = layout.map((entry) =>
      entry.team.id === fromTeamId ? { ...entry, boards: arrayMove(entry.boards, from, to) } : entry,
    );
    return { layout: renumber(next), move: null };
  }

  let index = destination.boards.length;
  if (target.kind === 'board') {
    index = destination.boards.findIndex((candidate) => candidate.id === target.id);
    if (index === -1) return null;
    if (target.after) index += 1;
  }

  const next = layout.map((entry) => {
    if (entry.team.id === fromTeamId) {
      return { ...entry, boards: entry.boards.filter((candidate) => candidate.id !== boardId) };
    }
    if (entry.team.id === toTeamId) {
      const boards = [...entry.boards];
      boards.splice(index, 0, board);
      return { ...entry, boards };
    }
    return entry;
  });

  return { layout: renumber(next), move: { boardId, fromTeamId, toTeamId } };
}

/** Moves a board to the end of another team: the result of the board dialog's Team select. */
export function moveBoardToEnd(layout: Layout, boardId: string, toTeamId: string): BoardDrop | null {
  return dropBoard(layout, boardId, { kind: 'team', id: toTeamId });
}

export function teamOrder(layout: Layout): string[] {
  return layout.map((entry) => entry.team.id);
}

export function boardOrder(layout: Layout, teamId: string): string[] {
  return layout.find((entry) => entry.team.id === teamId)?.boards.map((board) => board.id) ?? [];
}

/** The body for saveOrder after a team reorder. */
export function teamOrderPayload(layout: Layout): NavigationOrder {
  return { teams: teamOrder(layout) };
}

/** The body for saveOrder after boards changed in the given teams. */
export function boardOrderPayload(layout: Layout, teamIds: string[]): NavigationOrder {
  const boards: Record<string, string[]> = {};
  for (const teamId of teamIds) boards[teamId] = boardOrder(layout, teamId);
  return { boards };
}

/** The sort_order a new item should take to land after the existing ones. */
export function nextSortOrder(existing: number[]): number {
  return existing.length === 0 ? 0 : Math.max(...existing) + 1;
}
