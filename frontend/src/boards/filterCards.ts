import type { Card } from '../lib/types';

/**
 * Whether a card matches the board's text filter.
 *
 * Only the title and summary are searched: they are what the tile shows, so a
 * match is always something the person can see on the board. The instructions
 * are left out on purpose, because a word buried in a long runbook would surface
 * cards that look unrelated to the query. The comparison is a case-insensitive
 * substring match, and a blank query matches every card.
 */
export function matchesQuery(card: Card, query: string): boolean {
  const needle = query.trim().toLocaleLowerCase();
  if (needle === '') return true;

  return (
    card.title.toLocaleLowerCase().includes(needle) ||
    card.summary.toLocaleLowerCase().includes(needle)
  );
}
