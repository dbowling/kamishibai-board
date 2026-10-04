import { describe, expect, it } from 'vitest';
import type { BoardRecord, TeamRecord } from '../lib/types';
import {
  boardItemId,
  boardOrder,
  boardOrderPayload,
  dropBoard,
  dropItemId,
  moveBoardToEnd,
  nextSortOrder,
  parseItemId,
  reorderTeams,
  teamItemId,
  teamOfBoard,
  teamOrderPayload,
} from './order';
import type { Layout } from './order';

const team = (id: string, sort_order: number): TeamRecord => ({
  id,
  name: id.toUpperCase(),
  description: '',
  members: [],
  sort_order,
  archived_at: '',
});

const board = (id: string, teamId: string, sort_order: number): BoardRecord => ({
  id,
  team: teamId,
  name: id,
  description: '',
  sort_order,
  archived_at: '',
});

function layout(): Layout {
  return [
    { team: team('t1', 1), boards: [board('b1', 't1', 1), board('b2', 't1', 2), board('b3', 't1', 3)] },
    { team: team('t2', 2), boards: [board('b4', 't2', 1)] },
    { team: team('t3', 3), boards: [] },
  ];
}

const ids = (l: Layout, teamId: string) => boardOrder(l, teamId);

describe('item ids', () => {
  it('round-trips and rejects unknown prefixes', () => {
    expect(parseItemId(teamItemId('abc'))).toEqual({ kind: 'team', id: 'abc' });
    expect(parseItemId(boardItemId('x:y'))).toEqual({ kind: 'board', id: 'x:y' });
    expect(parseItemId(dropItemId('t'))).toEqual({ kind: 'drop', id: 't' });
    expect(parseItemId('nope')).toBeNull();
    expect(parseItemId('card:1')).toBeNull();
  });
});

describe('reorderTeams', () => {
  it('moves a team and renumbers sort_order', () => {
    const next = reorderTeams(layout(), 't1', 't3');
    expect(next?.map((entry) => entry.team.id)).toEqual(['t2', 't3', 't1']);
    expect(next?.map((entry) => entry.team.sort_order)).toEqual([0, 1, 2]);
    expect(teamOrderPayload(next!)).toEqual({ teams: ['t2', 't3', 't1'] });
  });

  it('returns null when nothing moved or an id is unknown', () => {
    expect(reorderTeams(layout(), 't1', 't1')).toBeNull();
    expect(reorderTeams(layout(), 't1', 'nope')).toBeNull();
  });
});

describe('dropBoard within a team', () => {
  it('takes the position of the board it lands on', () => {
    const result = dropBoard(layout(), 'b1', { kind: 'board', id: 'b3' });
    expect(result?.move).toBeNull();
    expect(ids(result!.layout, 't1')).toEqual(['b2', 'b3', 'b1']);
    expect(result?.layout[0]?.boards.map((b) => b.sort_order)).toEqual([0, 1, 2]);
  });

  it('moves up as well as down', () => {
    const result = dropBoard(layout(), 'b3', { kind: 'board', id: 'b1' });
    expect(ids(result!.layout, 't1')).toEqual(['b3', 'b1', 'b2']);
  });

  it('is a no-op when dropped on itself or on its own team', () => {
    expect(dropBoard(layout(), 'b1', { kind: 'board', id: 'b1' })).toBeNull();
    expect(dropBoard(layout(), 'b1', { kind: 'team', id: 't1' })).toBeNull();
  });
});

describe('dropBoard across teams', () => {
  it('inserts before the board it lands on and reports the move', () => {
    const result = dropBoard(layout(), 'b2', { kind: 'board', id: 'b4' });
    expect(result?.move).toEqual({ boardId: 'b2', fromTeamId: 't1', toTeamId: 't2' });
    expect(ids(result!.layout, 't2')).toEqual(['b2', 'b4']);
    expect(ids(result!.layout, 't1')).toEqual(['b1', 'b3']);
    expect(teamOfBoard(result!.layout, 'b2')).toBe('t2');
    expect(result?.layout[1]?.boards[0]?.team).toBe('t2');
  });

  it('inserts after when released on the lower half', () => {
    const result = dropBoard(layout(), 'b2', { kind: 'board', id: 'b4', after: true });
    expect(ids(result!.layout, 't2')).toEqual(['b4', 'b2']);
  });

  it('appends to a team dropped on, including an empty one', () => {
    const result = dropBoard(layout(), 'b1', { kind: 'team', id: 't3' });
    expect(result?.move?.toTeamId).toBe('t3');
    expect(ids(result!.layout, 't3')).toEqual(['b1']);
  });

  it('builds the order payload for just the target team', () => {
    const result = dropBoard(layout(), 'b2', { kind: 'board', id: 'b4' });
    expect(boardOrderPayload(result!.layout, ['t2'])).toEqual({ boards: { t2: ['b2', 'b4'] } });
  });

  it('does not mutate its input', () => {
    const before = layout();
    dropBoard(before, 'b2', { kind: 'board', id: 'b4' });
    expect(ids(before, 't1')).toEqual(['b1', 'b2', 'b3']);
  });

  it('returns null for unknown ids', () => {
    expect(dropBoard(layout(), 'nope', { kind: 'team', id: 't2' })).toBeNull();
    expect(dropBoard(layout(), 'b1', { kind: 'team', id: 'nope' })).toBeNull();
    expect(dropBoard(layout(), 'b1', { kind: 'board', id: 'nope' })).toBeNull();
  });
});

describe('moveBoardToEnd', () => {
  it('appends to the target team', () => {
    const result = moveBoardToEnd(layout(), 'b1', 't2');
    expect(ids(result!.layout, 't2')).toEqual(['b4', 'b1']);
    expect(result?.move).toEqual({ boardId: 'b1', fromTeamId: 't1', toTeamId: 't2' });
  });
});

describe('nextSortOrder', () => {
  it('goes after the highest value, whatever the numbering', () => {
    expect(nextSortOrder([])).toBe(0);
    expect(nextSortOrder([1, 2, 5])).toBe(6);
    expect(nextSortOrder([0])).toBe(1);
  });
});
