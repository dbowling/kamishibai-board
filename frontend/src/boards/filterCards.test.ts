import { describe, expect, it } from 'vitest';
import { matchesQuery } from './filterCards';
import type { Card } from '../lib/types';

function makeCard(overrides: Partial<Card> = {}): Card {
  return {
    id: 'card1',
    boardId: 'board1',
    title: 'Verify Backups',
    summary: 'Confirm last night’s runs completed.',
    cadence: 'daily',
    instructions: '<p>Open the restore console.</p>',
    links: [],
    checklist: [],
    sortOrder: 1,
    archived: false,
    period: {
      cadence: 'daily',
      key: '2026-09-03',
      start: '2026-09-03T04:00:00Z',
      end: '2099-01-01T05:00:00Z',
      label: 'Daily',
    },
    state: { status: 'not_started', periodKey: '2026-09-03' },
    ...overrides,
  };
}

describe('matchesQuery', () => {
  it('matches everything for an empty query', () => {
    expect(matchesQuery(makeCard(), '')).toBe(true);
  });

  it('matches everything for a whitespace-only query', () => {
    expect(matchesQuery(makeCard(), '   ')).toBe(true);
  });

  it('matches part of the title', () => {
    expect(matchesQuery(makeCard(), 'backu')).toBe(true);
  });

  it('matches part of the summary', () => {
    expect(matchesQuery(makeCard(), 'night')).toBe(true);
  });

  it('ignores case in the query and the card', () => {
    expect(matchesQuery(makeCard(), 'VERIFY')).toBe(true);
    expect(matchesQuery(makeCard(), 'backups')).toBe(true);
  });

  it('ignores whitespace around the query', () => {
    expect(matchesQuery(makeCard(), '  backups  ')).toBe(true);
  });

  it('does not match text that is only in the instructions', () => {
    expect(matchesQuery(makeCard(), 'console')).toBe(false);
  });

  it('does not match an unrelated query', () => {
    expect(matchesQuery(makeCard(), 'pager')).toBe(false);
  });
});
